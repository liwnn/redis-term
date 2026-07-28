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
	"github.com/liwnn/redisterm/datasource/redis"
	"github.com/liwnn/redisterm/datasource/zookeeper"
	"github.com/liwnn/redisterm/tlog"
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
	Pollable() bool               // 后端是否支持后台健康轮询(redis 支持)
	Ping() error                  // 健康检查(走独立连接);nil 表示连接存活
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

	// pollers maps a cacheKey to the stop channel of its background health-poll
	// goroutine, so the poll can be torn down when the connection is invalidated,
	// deleted, or transitions to the failed state.
	pollers map[string]chan struct{}

	// dropPrevFocus is the primitive that held focus when Ctrl-P opened the
	// connection dropdown, so Esc (abort) can return focus there instead of
	// stranding it on the closed dropdown field.
	dropPrevFocus tview.Primitive

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
		pollers:    make(map[string]chan struct{}),
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
			a.dropPrevFocus = a.main.GetFocus()
			a.main.GetOpLine().OpenSelectDrop(func(p tview.Primitive) { a.main.SetFocus(p) })
			return nil
		}
		// Esc while the connection dropdown holds focus aborts selection and
		// returns focus to wherever it was when Ctrl-P opened the dropdown,
		// instead of stranding it on the closed dropdown field.
		if event.Key() == tcell.KeyEscape && a.main.GetOpLine().SelectDropPrimitive().HasFocus() {
			a.main.GetOpLine().CloseSelectDrop()
			if a.dropPrevFocus != nil {
				a.main.SetFocus(a.dropPrevFocus)
				a.dropPrevFocus = nil
			}
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
			if a.treeFocused() {
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
			if a.treeFocused() {
				return nil
			}
		}
		// Navigation keys inside the connection dropdown: vim j/k/g/G plus n/p
		// (next/previous) translate to arrow/home/end so the embedded list moves.
		// Gated on the dropdown holding focus (true whether closed or its list is
		// open). Every other rune is swallowed so stray letters can't trigger
		// type-ahead jumps — the list is navigated only by these keys.
		if event.Key() == tcell.KeyRune && a.main.GetOpLine().SelectDropPrimitive().HasFocus() {
			switch event.Rune() {
			case 'j', 'n', 'N':
				return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
			case 'k', 'p', 'P':
				return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
			case 'g':
				return tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone)
			case 'G':
				return tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone)
			default:
				return nil // block all other runes (no type-ahead)
			}
		}
		// vim h/l mirror the arrow jumps between tree and table, but only when
		// the tree or table actually holds focus — otherwise the rune must reach
		// the query box / input fields so the user can type 'h'/'l' normally.
		if event.Key() == tcell.KeyRune && a.tree != nil && a.tree.Preview() != nil {
			focused := a.main.GetFocus()
			switch event.Rune() {
			case 'l':
				if a.treeFocused() && a.focusPreviewFromTree() {
					return nil
				}
			case 'h':
				if (focused == a.tree.Preview().TablePrimitive() || focused == a.tree.Preview().TextPrimitive()) && a.focusTreeFromPreview() {
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

	for key := range a.trees {
		a.stopPoll(key)
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
		case "zookeeper":
			t = a.newZKTree(conf)
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
	a.main.SetPreviewOpBar(a.tree.Preview().OpBar())              // host this tree's op row in the top bar
	a.main.SetCliVisible(conf.Kind == "" || conf.Kind == "redis") // redis-cli only for redis
	a.main.GetCmd().SetPromt(conf.Name, a.tree.Index())
	a.cfg.SaveLastSelected(index)
	// Defer the focus to the tree: when Show runs from the connection dropdown's
	// selected callback, tview restores focus to the dropdown AFTER the callback
	// returns, which would override a synchronous SetFocus here and leave the
	// arrow keys driving the dropdown instead of the tree. Queuing it runs the
	// focus change after the dropdown settles.
	tree := a.tree.TreeView()
	go a.main.QueueUpdateDraw(func() { a.main.SetFocus(tree) })
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
	a.stopPoll(cacheKey) // a fresh connect supersedes any prior poll
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
			a.startPoll(t, cacheKey, name)
		})
	}()
}

// pollInterval is how often a live connection is health-checked in the
// background, so a server that dies while the user is idle flips the tree's
// status glyph to red within a few seconds.
const pollInterval = 3 * time.Second

// startPoll launches a background health-check loop for a freshly-connected
// tree that supports polling. On the first failed ping it flips the glyph to
// the failed state (enabling click-to-reconnect) and stops; the reconnect path
// restarts polling on success. Must run on the UI goroutine (it mutates
// a.pollers, which connectAsync/stopPoll also touch there).
func (a *App) startPoll(t treeController, cacheKey, name string) {
	if !t.Pollable() {
		return
	}
	a.stopPoll(cacheKey) // never run two pollers for one connection
	stop := make(chan struct{})
	a.pollers[cacheKey] = stop
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if err := t.Ping(); err != nil {
					a.main.QueueUpdateDraw(func() {
						// Ignore a stale failure: if this poller was already replaced or
						// stopped, leave the current state alone.
						if a.pollers[cacheKey] != stop {
							return
						}
						tlog.Log("[Poll] %q health check failed: %v", name, err)
						t.SetConnState(connFailed)
						delete(a.pollers, cacheKey)
					})
					return
				}
			}
		}
	}()
}

// stopPoll tears down the background health poll for a connection, if any. Must
// run on the UI goroutine.
func (a *App) stopPoll(cacheKey string) {
	if stop, ok := a.pollers[cacheKey]; ok {
		close(stop)
		delete(a.pollers, cacheKey)
	}
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

// toggleFocus cycles keyboard focus through the visible panels in a fixed ring:
// key tree → optional query box → table/text preview → bottom panel (CONSOLE /
// redis-cli) → back to the tree. This lets the user reach table cells to edit
// them, type a query, and read or copy from the console without the mouse. It
// reports whether it acted; when focus is elsewhere (e.g. a form), it returns
// false so Tab passes through.
func (a *App) toggleFocus() bool {
	if a.tree == nil {
		return false
	}
	preview := a.tree.Preview()
	if preview == nil {
		return false
	}

	// Build the focus ring in order. The tree and bottom panel are always present;
	// the query box and the table/text preview are added only when shown.
	ring := []tview.Primitive{a.tree.TreeView()}
	if preview.IsQueryShown() {
		ring = append(ring, preview.QueryPrimitive())
	}
	if preview.IsTableShown() {
		ring = append(ring, preview.TablePrimitive())
	} else if preview.IsTextShown() {
		ring = append(ring, preview.TextPrimitive())
	}
	ring = append(ring, a.main.BottomPanelPrimitive())

	// Match by HasFocus() rather than comparing GetFocus() pointers: a mouse click
	// focuses the embedded primitive (e.g. the inner *tview.TreeView), a different
	// pointer than the wrapper in the ring, so strict equality would miss it and
	// Tab would appear to stop working after a click.
	for i, p := range ring {
		if p.HasFocus() {
			a.main.SetFocus(ring[(i+1)%len(ring)])
			return true
		}
	}
	// Focus is on none of the ring members (e.g. a form, the search box or the
	// connection dropdown holds focus): don't act, so Tab keeps its normal meaning
	// there.
	return false
}

// focusPreviewFromTree handles Right-arrow when the key tree is focused: it
// jumps focus into the right-hand preview's table so the user lands on the data.
// The query box is reachable via Tab, not Right. It reports whether it acted; if
// the tree isn't focused or there's no table target, it returns false so Right
// keeps its normal meaning.
// treeFocused reports whether the key tree currently holds focus. It uses
// HasFocus() rather than comparing GetFocus() to the *view.Tree wrapper: a mouse
// click on a node focuses the embedded *tview.TreeView, a different pointer than
// the wrapper, so strict equality would wrongly report "not focused" and let
// Left/Right leak through to the tree's own up/down navigation.
func (a *App) treeFocused() bool {
	return a.tree != nil && a.tree.TreeView().HasFocus()
}

func (a *App) focusPreviewFromTree() bool {
	if a.tree == nil {
		return false
	}
	preview := a.tree.Preview()
	if preview == nil {
		return false
	}
	if !a.treeFocused() {
		return false
	}
	// Land on the table when shown (Show() makes an empty table non-selectable, so
	// focusing it is safe), else the text view, else the query box.
	if preview.IsTableShown() {
		a.main.SetFocus(preview.TablePrimitive())
		return true
	}
	if preview.IsTextShown() {
		a.main.SetFocus(preview.TextPrimitive())
		return true
	}
	if preview.IsQueryShown() {
		a.main.SetFocus(preview.QueryPrimitive())
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
	case preview.TextPrimitive():
		// in the text box, Left leaves for the tree only in read-only mode; while
		// editing it must reach the textarea so the cursor can move.
		if preview.IsTextEditing() {
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
		a.stopPoll(key)
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
	for key := range a.trees {
		a.stopPoll(key)
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
func settingToConfig(s view.Setting) config.Conn {
	port, _ := strconv.Atoi(s.Port)
	kind := s.Kind
	if kind == "redis" {
		kind = ""
	}
	return config.Conn{
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
func probeConn(conf config.Conn) error {
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
	case "zookeeper":
		src := zookeeper.NewZKSource(conf.Host, conf.Port)
		if err := src.Open(); err != nil {
			return err
		}
		src.Close()
	default: // redis
		src := redis.NewRedisSource(fmt.Sprintf("%v:%v", conf.Host, conf.Port), conf.Auth)
		if err := src.Open(); err != nil {
			return err
		}
		src.Close()
	}
	return nil
}

// newRedisTree builds the redis-backed tree (db0..dbN > keys split into a ':'
// prefix tree). The connection is not opened here; Show calls Connect()
// asynchronously.
func (a *App) newRedisTree(conf config.Conn) treeController {
	address := fmt.Sprintf("%v:%v", conf.Host, conf.Port)
	t := NewDSTree(conf.Name, redis.NewRedisSource(address, conf.Auth))
	t.SetKeyTree(":")
	t.SetQueueUpdate(func(fn func()) { a.main.QueueUpdateDraw(fn) })
	t.preview.SetTableFocusFunc(a.focus)
	t.preview.SetTextFocusFunc(a.focus)
	t.preview.SetClipboardFunc(a.main.Clipboard)
	t.ShowModal = a.main.ShowModal
	t.ShowModalOK = a.main.ShowModalOK
	return t
}

// newMongoTree builds the mongo-backed read-only tree (database > collection > docs).
func (a *App) newMongoTree(conf config.Conn) treeController {
	return a.newDSTree(conf.Name, mongo.NewMongoSource(conf.MongoURI()))
}

// newMySQLTree builds the mysql-backed tree (database > table > rows).
func (a *App) newMySQLTree(conf config.Conn) treeController {
	return a.newDSTree(conf.Name, mysql.NewMySQLSource(conf.Host, conf.Port, conf.User, conf.Auth))
}

// newZKTree builds the zookeeper-backed tree. Znode paths nest into a folder
// tree split on "/", mirroring the redis key tree.
func (a *App) newZKTree(conf config.Conn) treeController {
	t := NewDSTree(conf.Name, zookeeper.NewZKSource(conf.Host, conf.Port))
	t.SetNested("/", "znode", "znode")
	t.preview.SetTableFocusFunc(a.focus)
	t.preview.SetTextFocusFunc(a.focus)
	t.preview.SetClipboardFunc(a.main.Clipboard)
	t.ShowModal = a.main.ShowModal
	t.ShowModalOK = a.main.ShowModalOK
	return t
}

// newDSTree wraps a datasource in a DSTree and wires the shared UI callbacks.
// Used by the query-capable backends (mongo / mysql), so it also enables the
// Query filter box; redis and zookeeper build DSTree directly without it.
func (a *App) newDSTree(name string, src datasource.Datasource) treeController {
	t := NewDSTree(name, src)
	t.preview.SetTableFocusFunc(a.focus)
	t.preview.SetTextFocusFunc(a.focus)
	t.preview.SetClipboardFunc(a.main.Clipboard)
	t.preview.SetQueryFunc(t.runQuery)
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
