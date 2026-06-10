package app

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liwnn/redisterm/config"
	"github.com/liwnn/redisterm/datasource"
	"github.com/liwnn/redisterm/datasource/mongo"
	"github.com/liwnn/redisterm/datasource/mysql"
	"github.com/liwnn/redisterm/datasource/redisapi"
	"github.com/liwnn/redisterm/tlog"
	"github.com/liwnn/redisterm/tui/model"
	"github.com/liwnn/redisterm/tui/view"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// connState is the connection state shown as a right-aligned glyph on a tree's
// root row.
type connState int

const (
	connConnecting connState = iota
	connOK
	connFailed
)

// applyConnState maps a connState to the root-row status glyph + color. The
// failed glyph (⚠) is also what Failed() detects for click-to-reconnect.
func applyConnState(tree *view.Tree, s connState) {
	switch s {
	// Use explicit RGB rather than ANSI palette colors (ColorRed/etc.), which the
	// terminal theme can render as washed-out pastels.
	case connConnecting:
		tree.SetRootStatus('◌', tcell.NewRGBColor(0xE6, 0xB4, 0x22)) // amber
	case connOK:
		tree.SetRootStatus('●', tcell.NewRGBColor(0x2E, 0xCC, 0x71)) // green
	case connFailed:
		tree.SetRootStatus('✖', tcell.NewRGBColor(0xE7, 0x4C, 0x3C)) // red
	}
}

// treeController is the per-connection tree shown in the left pane. Redis and
// mongo provide different implementations dispatched by config kind.
type treeController interface {
	TreeView() *view.Tree
	PreviewFlex() *tview.Flex
	Preview() *view.Preview
	Index() int
	Cmd(w io.Writer, cmd string, params ...string) error
	Connect() error               // 建立后端连接(可能阻塞在网络上),与建树分离以便异步
	Expand()                      // 加载并展开第一层(redis db / mongo database)
	SetConnState(s connState)     // 设置根节点右侧的连接状态图标
	Failed() bool                 // 根节点当前是否处于连接失败态(用于点击重连判定)
	SetRootClickFunc(func() bool) // 根节点被点击时的拦截:返回 true 表示已处理(如重连),不再正常展开
	Filter(term string)
	Close()
}

// App app
type App struct {
	cfg  *config.Config
	main *view.MainView
	tree treeController

	trees map[string]treeController

	// connecting marks cacheKeys with a background connect attempt in flight, so
	// repeated root clicks don't kick off concurrent dials.
	connecting map[string]bool

	searchMu    sync.Mutex
	searchTimer *time.Timer
}

// NewApp new
func NewApp(cfgFile string) *App {
	cfg, err := config.NewConfig(cfgFile)
	if err != nil {
		panic(err)
	}
	a := &App{
		main:       view.NewMainView(),
		trees:      make(map[string]treeController),
		connecting: make(map[string]bool),
		cfg:        cfg,
	}
	a.init()
	return a
}

func (a *App) init() {
	tlog.SetLogger(a.main.GetOutput())

	a.main.GetOpLine().SetEditClickFunc(func() {
		index := a.main.GetOpLine().GetSelect()
		config := a.cfg.GetConfig(index)
		kind := config.Kind
		if kind == "" {
			kind = "redis"
		}
		setting := view.Setting{
			Name: config.Name,
			Kind: kind,
			Host: config.Host,
			Port: strconv.Itoa(config.Port),
			User: config.User,
			Auth: config.Auth,
			URI:  config.URI,
			DB:   config.DB,
		}
		a.main.ShowConnSetting(setting, true)
		tlog.Log("[App] init Edit Click: %v", setting)
	})

	a.main.GetConnSetting().SetTestHandler(func(s view.Setting) error {
		return probeConn(settingToConfig(s))
	})

	a.main.GetOpLine().SetDeleteClickFunc(func() {
		// Delete the currently-selected connection (toolbar ✕ button).
		index := a.main.GetOpLine().GetSelect()
		if index < 0 || index >= a.cfg.Count() {
			return
		}
		name := a.cfg.GetConfig(index).Name
		a.main.ShowModal(fmt.Sprintf("Delete connection %q?", name), func() {
			a.deleteConn(index)
		})
	})

	a.main.GetConnSetting().SetOKHandler(func(s view.Setting, edit bool) {
		a.main.HideConnSetting()
		if s.Name == "" {
			return
		}
		conf := settingToConfig(s)
		if edit {
			lastIndex := a.main.GetOpLine().GetSelect()
			a.invalidate(lastIndex)
			a.cfg.Update(conf, lastIndex)
			a.main.RefreshOpLine(a.cfg.GetDbNames(), a.Show)
			a.main.GetOpLine().Select(lastIndex)
		} else {
			a.cfg.Add(conf)
			a.main.RefreshOpLine(a.cfg.GetDbNames(), a.Show)
			a.main.GetOpLine().Select(a.main.GetOpLine().GetOptionCount() - 1)
		}
		if err := a.cfg.Save(); err != nil {
			panic(err)
		}
	})
	a.main.GetCmd().SetEnterHandler(a.onCmdLineEnter)
	a.main.GetSearch().SetChangedFunc(a.onSearchChanged)
	a.main.GetSearch().SetDoneFunc(func(key tcell.Key) {
		// Enter/Esc/Tab from the search box drop focus into the tree so the user
		// can navigate the filtered results immediately.
		if a.tree != nil {
			a.main.SetFocus(a.tree.TreeView())
		}
	})
	a.main.SetGlobalInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlP {
			a.main.SetFocus(a.main.GetOpLine().SelectDropPrimitive())
			return nil
		}
		if event.Key() == tcell.KeyCtrlF {
			a.main.SetFocus(a.main.GetSearch())
			return nil
		}
		if event.Key() == tcell.KeyTab && a.toggleFocus() {
			return nil
		}
		if event.Key() == tcell.KeyRight {
			if a.focusPreviewFromTree() {
				return nil
			}
			// when the tree already holds focus but no jump happened (e.g. the
			// preview is a plain text view, not a table/query), swallow Right so
			// it doesn't move the selection down (tview maps Right→down).
			if a.tree != nil && a.main.GetFocus() == a.tree.TreeView() {
				return nil
			}
		}
		if event.Key() == tcell.KeyLeft {
			if a.focusTreeFromPreview() {
				return nil
			}
			// when the tree already holds focus, swallow Left so it doesn't move
			// the selection up (tview maps Left→up); Left is reserved for the
			// preview→tree jump only.
			if a.tree != nil && a.main.GetFocus() == a.tree.TreeView() {
				return nil
			}
		}
		// vim j/k/g/G inside the connection dropdown: translate to arrow/home/end
		// so the embedded list navigates. Gated on the dropdown holding focus
		// (true whether closed or its list is open) so type-ahead still works
		// elsewhere. Letters that aren't nav keys fall through to type-ahead.
		if event.Key() == tcell.KeyRune && a.main.GetOpLine().SelectDropPrimitive().HasFocus() {
			switch event.Rune() {
			case 'j':
				return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
			case 'k':
				return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
			case 'g':
				return tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone)
			case 'G':
				return tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)
			}
		}
		// vim h/l mirror the arrow jumps between tree and table, but only when
		// the tree or table actually holds focus — otherwise the rune must reach
		// the query box / input fields so the user can type 'h'/'l' normally.
		if event.Key() == tcell.KeyRune && a.tree != nil && a.tree.Preview() != nil {
			focused := a.main.GetFocus()
			switch event.Rune() {
			case 'l':
				if focused == a.tree.TreeView() && a.focusPreviewFromTree() {
					return nil
				}
			case 'h':
				if focused == a.tree.Preview().TablePrimitive() && a.focusTreeFromPreview() {
					return nil
				}
			}
		}
		return event
	})
	a.main.RefreshOpLine(a.cfg.GetDbNames(), a.Show)
}

// Run run
func (a *App) Run() {
	a.main.GetOpLine().Select(a.cfg.LastSelectedIndex())

	if err := a.main.Run(); err != nil {
		panic(err)
	}

	for _, client := range a.trees {
		client.Close()
	}
}

// Show displays the connection at index. A first-time selection builds the tree's
// UI skeleton synchronously (tview nodes must be created on the UI goroutine) and
// then opens the backend connection in the background, so an unreachable host
// can't freeze the UI. Once connected, Expand runs back on the UI goroutine via
// QueueUpdateDraw to populate the first level.
func (a *App) Show(index int) {
	conf := a.cfg.GetConfig(index)
	cacheKey := strconv.Itoa(index)
	t, ok := a.trees[cacheKey]
	if !ok {
		switch conf.Kind {
		case "mongo":
			t = a.newMongoTree(conf)
		case "mysql":
			t = a.newMySQLTree(conf)
		default:
			t = a.newRedisTree(conf)
		}
		a.trees[cacheKey] = t
		// Clicking the root after a failed connect retries it.
		t.SetRootClickFunc(func() bool { return a.reconnect(t, cacheKey, conf.Name) })
		a.connectAsync(t, cacheKey, conf.Name)
	}

	a.tree = t
	a.main.SetTree(a.tree.TreeView())
	a.main.GetSearch().SetText("") // clear the filter when switching connections
	a.main.SetPreview(a.tree.PreviewFlex())
	a.main.SetPreviewOpBar(a.tree.Preview().OpBar()) // host this tree's op row in the top bar
	a.main.SetCliVisible(conf.Kind == "" || conf.Kind == "redis") // redis-cli only for redis
	a.main.GetCmd().SetPromt(conf.Name, a.tree.Index())
	a.cfg.SaveLastSelected(index)
	a.main.SetFocus(a.tree.TreeView())
}

// connectAsync opens t's backend connection off the UI goroutine, then expands
// the first level on the UI goroutine. On failure the tree's root node shows a
// short failed status (the full error goes to the console log); the tree stays
// visible so the user can switch away. While connecting it shows a hint. The
// cacheKey gates concurrent attempts so a flurry of root clicks dials only once.
func (a *App) connectAsync(t treeController, cacheKey, name string) {
	if a.connecting[cacheKey] {
		return
	}
	a.connecting[cacheKey] = true
	t.SetConnState(connConnecting)
	go func() {
		err := t.Connect()
		a.main.QueueUpdateDraw(func() {
			delete(a.connecting, cacheKey)
			if err != nil {
				tlog.Log("[Show] connect %q failed: %v", name, err)
				t.SetConnState(connFailed)
				return
			}
			t.SetConnState(connOK)
			t.Expand()
		})
	}()
}

// reconnect retries a connection from a root-node click. It acts only in the
// failed state; otherwise it returns false so the click falls through to normal
// expand/collapse. A retry already in flight is swallowed so concurrent clicks
// dial only once.
func (a *App) reconnect(t treeController, cacheKey, name string) bool {
	if a.connecting[cacheKey] {
		return true // an attempt is mid-flight; swallow the click
	}
	if !t.Failed() {
		return false // not in a failed state; let normal handling run
	}
	a.connectAsync(t, cacheKey, name)
	return true
}

// onSearchChanged debounces key-search input: each keystroke resets a 200ms
// timer; when it fires we filter the active tree on the UI goroutine. Debouncing
// avoids a full key rescan on every keystroke (redis re-SCANs the db).
func (a *App) onSearchChanged(term string) {
	a.searchMu.Lock()
	defer a.searchMu.Unlock()
	if a.searchTimer != nil {
		a.searchTimer.Stop()
	}
	a.searchTimer = time.AfterFunc(200*time.Millisecond, func() {
		a.main.QueueUpdateDraw(func() {
			if a.tree != nil {
				a.tree.Filter(term)
			}
		})
	})
}

// toggleFocus moves keyboard focus between the key tree, the optional query box
// and the table preview so the user can reach table cells to edit them or type a
// query. It reports whether it acted; when focus is elsewhere (e.g. a form), it
// returns false so Tab passes through.
func (a *App) toggleFocus() bool {
	if a.tree == nil {
		return false
	}
	preview := a.tree.Preview()
	if preview == nil {
		return false
	}
	tree := a.tree.TreeView()
	hasQuery := preview.IsQueryShown()
	hasTable := preview.IsTableShown()
	if !hasQuery && !hasTable {
		return false
	}
	focused := a.main.GetFocus()
	query := preview.QueryPrimitive()
	table := preview.TablePrimitive()

	switch focused {
	case tree:
		if hasQuery {
			a.main.SetFocus(query)
		} else {
			a.main.SetFocus(table)
		}
		return true
	case query:
		if hasTable {
			a.main.SetFocus(table)
		} else {
			a.main.SetFocus(tree)
		}
		return true
	case table:
		a.main.SetFocus(tree)
		return true
	}
	return false
}

// focusPreviewFromTree handles Right-arrow when the key tree is focused: it
// jumps focus into the right-hand preview's table so the user lands on the data.
// The query box is reachable via Tab, not Right. It reports whether it acted; if
// the tree isn't focused or there's no table target, it returns false so Right
// keeps its normal meaning.
func (a *App) focusPreviewFromTree() bool {
	if a.tree == nil {
		return false
	}
	preview := a.tree.Preview()
	if preview == nil {
		return false
	}
	if a.main.GetFocus() != a.tree.TreeView() {
		return false
	}
	if preview.IsTableShown() {
		a.main.SetFocus(preview.TablePrimitive())
		return true
	}
	return false
}

// focusTreeFromPreview handles Left-arrow when the preview is focused, mirroring
// Right's tree→preview jump. From the table it returns to the tree only when the
// selection is on the first data column (inner columns keep moving left). From
// the query box it returns only when the box is empty (otherwise Left edits
// text). Reports whether it acted.
func (a *App) focusTreeFromPreview() bool {
	if a.tree == nil {
		return false
	}
	preview := a.tree.Preview()
	if preview == nil {
		return false
	}
	focused := a.main.GetFocus()
	switch focused {
	case preview.TablePrimitive():
		// in the table, Left only leaves when on the first data column;
		// otherwise it moves the selection one column left.
		if !preview.TableAtLeftEdge() {
			return false
		}
	case preview.QueryPrimitive():
		// in the query box, Left edits text; only leave when it's empty so we
		// never swallow a cursor move (InputField exposes no cursor position).
		if preview.QueryText() != "" {
			return false
		}
	default:
		return false
	}
	a.main.SetFocus(a.tree.TreeView())
	return true
}

// focus sets keyboard focus to a primitive (adapts Application.SetFocus, which
// returns the app, to the view layer's no-result focus setter).
func (a *App) focus(p tview.Primitive) {
	a.main.SetFocus(p)
}

// invalidate drops the cached tree for a connection index so the next Show
// reconnects with the latest config. Used after editing a connection.
func (a *App) invalidate(index int) {
	key := strconv.Itoa(index)
	if t, ok := a.trees[key]; ok {
		t.Close()
		delete(a.trees, key)
	}
}

// deleteConn removes the connection at index. Because the tree cache is keyed by
// connection index, a deletion shifts every later index, so the whole cache is
// closed and dropped; surviving trees lazily reconnect on the next Show. After
// removal it reselects an adjacent connection (or clears the panes if none remain).
func (a *App) deleteConn(index int) {
	if index < 0 || index >= a.cfg.Count() {
		return
	}
	for _, t := range a.trees {
		t.Close()
	}
	a.trees = make(map[string]treeController)
	a.connecting = make(map[string]bool)
	a.tree = nil

	a.cfg.Remove(index)
	if err := a.cfg.Save(); err != nil {
		panic(err)
	}

	a.main.RefreshOpLine(a.cfg.GetDbNames(), a.Show)
	if a.cfg.Count() == 0 {
		return
	}
	next := index
	if next >= a.cfg.Count() {
		next = a.cfg.Count() - 1
	}
	a.main.GetOpLine().Select(next)
	a.Show(next) // Select may not re-fire when the numeric index is unchanged
}

// settingToConfig converts a connection-setting form value into a stored config.
// Redis keeps an empty Kind for backward compatibility (configs predate the field).
func settingToConfig(s view.Setting) redisapi.RedisConfig {
	port, _ := strconv.Atoi(s.Port)
	kind := s.Kind
	if kind == "redis" {
		kind = ""
	}
	return redisapi.RedisConfig{
		Name: s.Name,
		Host: s.Host,
		Port: port,
		User: s.User,
		Auth: s.Auth,
		Kind: kind,
		URI:  s.URI,
		DB:   s.DB,
	}
}

// probeConn opens a throwaway connection to verify the config is reachable, then
// closes it. Returns nil on success. Used by the setting form's Test button.
func probeConn(conf redisapi.RedisConfig) error {
	switch conf.Kind {
	case "mongo":
		src := mongo.NewMongoSource(conf.MongoURI())
		if err := src.Open(); err != nil {
			return err
		}
		src.Close()
	case "mysql":
		src := mysql.NewMySQLSource(conf.Host, conf.Port, conf.User, conf.Auth)
		if err := src.Open(); err != nil {
			return err
		}
		src.Close()
	default: // redis
		data := model.NewData(fmt.Sprintf("%v:%v", conf.Host, conf.Port), conf.Auth)
		if err := data.Connect(); err != nil {
			return err
		}
		data.Close()
	}
	return nil
}

// newRedisTree builds the redis-backed tree (db0..dbN > keys). The connection is
// not opened here; Show calls Connect() asynchronously.
func (a *App) newRedisTree(conf redisapi.RedisConfig) treeController {
	address := fmt.Sprintf("%v:%v", conf.Host, conf.Port)
	tree := view.NewTree("db")
	tree.GetRoot().SetReference(&Reference{Name: "db"})
	preview := view.NewPreview()

	t := NewDBTree(tree, preview)
	t.ShowModalOK = a.main.ShowModalOK
	t.ShowModal = a.main.ShowModal
	preview.SetTableFocusFunc(a.focus)
	t.SetData(address, model.NewData(address, conf.Auth))
	return t
}

// newMongoTree builds the mongo-backed read-only tree (database > collection > docs).
func (a *App) newMongoTree(conf redisapi.RedisConfig) treeController {
	return a.newDSTree(conf.Name, mongo.NewMongoSource(conf.MongoURI()))
}

// newMySQLTree builds the mysql-backed tree (database > table > rows).
func (a *App) newMySQLTree(conf redisapi.RedisConfig) treeController {
	return a.newDSTree(conf.Name, mysql.NewMySQLSource(conf.Host, conf.Port, conf.User, conf.Auth))
}

// newDSTree wraps a datasource in a DSTree and wires the shared UI callbacks.
func (a *App) newDSTree(name string, src datasource.Datasource) treeController {
	t := NewDSTree(name, src)
	t.preview.SetTableFocusFunc(a.focus)
	t.ShowModal = a.main.ShowModal
	t.ShowModalOK = a.main.ShowModalOK
	return t
}

func (a *App) onCmdLineEnter(text string) {
	args := strings.Fields(text)
	if len(args) == 0 {
		return
	}
	cmd := args[0]
	view := a.main.GetCmd()
	if err := a.tree.Cmd(view, cmd, args[1:]...); err != nil {
		fmt.Fprintln(view, err)
	} else {
		switch strings.ToUpper(cmd) {
		case "SELECT":
			index, err := strconv.Atoi(args[1])
			if err != nil {
				fmt.Fprintln(view, err)
			} else {
				view.SetIndex(index)
			}
		}
	}
}
