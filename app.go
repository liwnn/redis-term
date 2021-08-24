package redisterm

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"time"

	"github.com/rivo/tview"

	"redisterm/tlog"
	"redisterm/ui"
)

// Reference referenct
type Reference struct {
	Name  string
	Index int
	Data  *DataNode
}

// DBTree tree.
type DBTree struct {
	tree    *ui.Tree
	preview *ui.Preview

	data *Data
}

// NewDBTree new
func NewDBTree(tree *ui.Tree, preview *ui.Preview) *DBTree {
	dbTree := &DBTree{
		tree:    tree,
		preview: preview,
	}

	tree.SetSelectedFunc(dbTree.OnSelected)
	tree.SetChangedFunc(dbTree.OnChanged)

	return dbTree
}

// SetData change db data.
func (t *DBTree) SetData(name string, data *Data) {
	t.tree.GetRoot().ClearChildren()
	t.tree.GetRoot().SetText(name)
	t.data = data
}

// OnSelected on select
func (t *DBTree) OnSelected(node *tview.TreeNode) {
	typ := t.getReference(node)
	err := t.data.Select(typ.Index)
	if err != nil {
		if err := t.data.Connect(); err != nil {
			tlog.Log("[OnSelected] %v", err)
			return
		}
	}
	tlog.Log("OnSelected: name[%v] index[%v]", typ.Name, typ.Index)
	if typ.Data != nil && typ.Data.HasChild() {
		t.tree.SetNodeText(typ.Data.name)
	}
	childen := node.GetChildren()
	if len(childen) == 0 {
		var dataNodes []*DataNode
		switch typ.Name {
		case "db":
			dbs, err := t.data.GetDatabases()
			if err != nil {
				if err := t.data.Connect(); err == nil {
					dbs, _ = t.data.GetDatabases()
				} else {
					tlog.Log("[OnSelected] db %v", err)
					return
				}
			}
			for i, dataNode := range dbs {
				t.tree.AddNode(dataNode.name, &Reference{
					Name:  "index",
					Index: i,
					Data:  dataNode,
				})
			}
			t.addNode(node, dataNodes)
		case "index":
			dataNodes, err = t.data.ScanAllKeys()
			if err != nil {
				if err := t.data.Connect(); err == nil {
					dataNodes, _ = t.data.ScanAllKeys()
				} else {
					tlog.Log("[OnSelected] index %v", err)
					return
				}
			}
			t.addNode(node, dataNodes)
		case "dir":
			dataNodes = t.data.GetChildren(typ.Data)
			t.addNode(node, dataNodes)
		}
	}
}

func (t *DBTree) addNode(node *tview.TreeNode, dataNodes []*DataNode) {
	typ := t.getReference(node)
	for _, dataNode := range dataNodes {
		r := &Reference{
			Index: typ.Index,
			Data:  dataNode,
		}
		if dataNode.HasChild() {
			r.Name = "dir"
			t.tree.AddNode("▶ "+dataNode.name, r)
		} else {
			r.Name = "key"
			t.tree.AddNode(dataNode.name, r)
		}
	}
}

// OnChanged on change
func (t *DBTree) OnChanged(node *tview.TreeNode) {
	typ := t.getReference(node)
	if typ.Name == "db" {
		tlog.Log("OnChanged: db %v", typ.Name)
		t.preview.SetOpBtnVisible(false)
	} else {
		if typ.Name == "index" {
			tlog.Log("OnChanged: %v - %v", typ.Name, typ.Index)
		} else {
			tlog.Log("OnChanged: %v - %v", typ.Name, typ.Data.key)
		}
		t.preview.SetOpBtnVisible(true)
	}

	if typ.Name == "key" {
		if !typ.Data.removed {
			t.data.Select(typ.Index)
			begin := time.Now()
			o := t.data.GetValue(typ.Data.key)
			tlog.Log("redis value time cost %v", time.Since(begin))
			t.updatePreview(o, true)
		} else {
			t.updatePreview(fmt.Sprintf("%v was removed", typ.Data.key), false)
		}
		t.preview.SetDeleteText("Delete")
		t.preview.SetKey(typ.Data.key)
	} else {
		if typ.Name == "index" {
			t.preview.SetDeleteText("Flush")
		} else {

			t.preview.SetDeleteText("Delete")
		}
		t.updatePreview("", false)
		t.preview.SetKey("")
	}
}

func (t *DBTree) updatePreview(o interface{}, valid bool) {
	p := t.preview
	p.Clear()
	switch h := o.(type) {
	case string:
		text := o.(string)
		p.ShowText(text, valid)
		if valid {
			p.SetSizeText(fmt.Sprintf("Size: %d bytes", len(text)))
		}
	case []string:
		title := []ui.TablePageTitle{
			{
				Name:      "row",
				Expansion: 1,
			},
			{
				Name:      "value",
				Expansion: 20,
			},
		}

		rows := make([]ui.Row, 0, len(h))
		for _, v := range h {
			rows = append(rows, ui.Row{v})
		}
		p.ShowTable(title, rows)
	case []KVText:
		title := []ui.TablePageTitle{
			{
				Name:      "row",
				Expansion: 1,
			},
			{
				Name:      "key",
				Expansion: 3,
			},
			{
				Name:      "value",
				Expansion: 24,
			},
		}
		rows := make([]ui.Row, 0, len(h))
		for _, v := range h {
			rows = append(rows, ui.Row{v.Key, v.Value})
		}
		p.ShowTable(title, rows)
	}
}

func (t *DBTree) getReference(node *tview.TreeNode) *Reference {
	if node == nil {
		return nil
	}

	reference := node.GetReference()
	if reference == nil {
		return nil
	}
	typ, ok := reference.(*Reference)
	if !ok {
		log.Fatalf("reference \n")
	}
	return typ
}

func (t *DBTree) getCurrentNode() *tview.TreeNode {
	return t.tree.GetCurrentNode()
}

func (t *DBTree) deleteSelectKey(typ *Reference) {
	switch typ.Name {
	case "key":
		tlog.Log("delete %v", typ.Data.key)
		if err := t.data.Delete(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.tree.SetNodeRemoved()
		t.updatePreview(fmt.Sprintf("%v was removed", typ.Data.key), false)
	case "index":
		if err := t.data.FlushDB(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.getCurrentNode().ClearChildren()
		t.getCurrentNode().SetText(typ.Data.name)
	case "dir":
		tlog.Log("delete %v", typ.Data.key)
		if err := t.data.Delete(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.tree.SetNodeRemoved()
		t.updatePreview("", false)
	default:
		tlog.Log("delete %v not implement", typ.Name)
	}
}

// Close close
func (t *DBTree) Close() {
	t.data.Close()
}

// RedisConfig config
type RedisConfig struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Auth string `json:"auth"`
}

type DBShow struct {
	*DBTree
	*ui.Preview
}

// App app
type App struct {
	main *ui.MainView

	config RedisConfig
	tree   *DBShow
	dbTree map[string]*DBShow

	configPath string
	configs    []RedisConfig
}

// NewApp new
func NewApp(config string) *App {
	a := &App{
		main:       ui.NewMainView(),
		dbTree:     make(map[string]*DBShow),
		configPath: config,
	}
	a.loadConfig()
	a.init()
	return a
}

func (a *App) loadConfig() error {
	b, err := ioutil.ReadFile(a.configPath)
	if err != nil {
		return err
	}

	var configs []RedisConfig
	if err := json.Unmarshal(b, &configs); err != nil {
		return err
	}
	a.configs = configs
	return nil
}

func (a *App) saveConfig() error {
	b, err := json.MarshalIndent(a.configs, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(a.configPath, b, 0)
}

func (a *App) init() {
	a.main.GetConfig = a.GetConfig
	a.main.GetOpLine().SetSelectedFunc(a.Show)
	a.main.GetCmd().SetEnterHandler(a.onCmdLineEnter)
	tlog.SetLogger(a.main.GetOutput())
	for _, config := range a.configs {
		a.main.GetOpLine().AddSelect(config.Name)
	}
	a.main.OnAdd = func(s ui.Setting) {
		if s.Name == "" {
			return
		}
		port, _ := strconv.Atoi(s.Port)
		conf := RedisConfig{
			Name: s.Name,
			Host: s.Host,
			Port: port,
			Auth: s.Auth,
		}
		var old bool
		for i, v := range a.configs {
			if v.Host == conf.Host && v.Port == conf.Port {
				a.configs[i] = conf
				old = true
				break
			}
		}
		if old {
			a.main.GetOpLine().ClearAllSelect()
			for _, config := range a.configs {
				a.main.GetOpLine().AddSelect(config.Name)
			}
			a.main.GetOpLine().SetSelectedFunc(a.Show)
		} else {
			a.configs = append(a.configs, conf)
			a.main.GetOpLine().AddSelect(conf.Name)
		}
		if err := a.saveConfig(); err != nil {
			panic(err)
		}
		a.config = conf
	}
}

// Run run
func (a *App) Run() {
	a.main.GetOpLine().Select(0)

	if err := a.main.Run(); err != nil {
		panic(err)
	}

	for _, client := range a.dbTree {
		client.Close()
	}
}

// Show show
func (a *App) Show(index int) {
	config := a.configs[index]
	address := fmt.Sprintf("%v:%v", config.Host, config.Port)
	t, ok := a.dbTree[address]
	if !ok {
		tree := ui.NewTree("")
		tree.GetRoot().SetReference(&Reference{
			Name: "db",
		})
		preview := ui.NewPreview()

		t = &DBShow{
			DBTree:  NewDBTree(tree, preview),
			Preview: preview,
		}
		data := NewData(address, config.Auth)
		if err := data.Connect(); err != nil {
			tlog.Log("[Show] %v", err)
		}
		t.SetData(config.Host, data)
		a.dbTree[address] = t

		preview.SetRenameFunc(a.renameSelectKey)
		preview.SetDeleteFunc(a.deleteKey)
		preview.SetReloadFunc(a.reloadSelectKey)
		preview.SetSaveFunc(a.saveKey)
	}

	a.tree = t
	a.config = config

	a.main.SetTree(a.tree.tree.TreeView)
	a.main.SetPreview(a.tree.preview.FlexBox())
	a.onCmdLineEnter("")
}

func (a *App) onCmdLineEnter(text string) {
	view := a.main.GetCmd()
	fmt.Fprintf(view, "[#00aa00]redis%v> [blue]", a.tree.data.index)
	fmt.Fprintln(view, text)
	fmt.Fprintf(view, "[white]")
	a.tree.data.Cmd(view, text)
}

func (a *App) renameSelectKey() {
	reference := a.tree.getReference(a.tree.getCurrentNode())
	if reference == nil {
		return
	}
	if reference.Name != "key" {
		return
	}

	notice := fmt.Sprintf("Rename %v->%v", reference.Data.key, a.tree.Preview.GetKey())
	a.main.ShowModal(notice, func() {
		if reference.Data.key == a.tree.Preview.GetKey() {
			return
		}

		tlog.Log("rename %v %v", reference.Data.key, a.tree.Preview.GetKey())
		a.tree.data.Rename(reference.Data, a.tree.Preview.GetKey())
		a.tree.getCurrentNode().SetText(reference.Data.name)
	})
}

func (a *App) deleteKey() {
	t := a.tree.DBTree
	typ := t.getReference(t.getCurrentNode())
	if typ == nil {
		return
	}
	var notice string
	switch typ.Name {
	case "key":
		notice = "Delete " + typ.Data.key + " ?"
	case "index":
		notice = fmt.Sprintf("FlushDB index:%v?", typ.Index)
	case "dir":
		notice = "Delete " + typ.Data.key + "* ?"
	}
	a.main.ShowModal(notice, func() {
		go t.deleteSelectKey(typ)
	})
}

func (a *App) reloadSelectKey() {
	t := a.tree.DBTree
	node := t.getCurrentNode()

	reference := t.getReference(node)
	if reference == nil {
		return
	}
	tlog.Log("reload %v", reference.Data.key)

	if reference.Name == "key" {
		t.data.Select(reference.Index)
		o := t.data.GetValue(reference.Data.key)
		if o == nil {
			reference.Data.removed = true
			t.tree.SetNodeRemoved()
			t.updatePreview(fmt.Sprintf("%v was removed", reference.Data.key), false)
			t.preview.SetDeleteText("Delete")
		} else {
			t.updatePreview(o, true)
		}
		return
	}

	node.ClearChildren()
	if err := t.data.Reload(reference.Data); err != nil {
		tlog.Log("[App] err %v", err)
		node.SetExpanded(false)
		node.SetText(reference.Data.name)
	}

	childen := reference.Data.GetChildren()
	for _, dataNode := range childen {
		r := &Reference{
			Index: reference.Index,
			Data:  dataNode,
		}
		if dataNode.HasChild() {
			r.Name = "dir"
			t.tree.AddNode("▶ "+dataNode.name, r)
		} else {
			r.Name = "key"
			t.tree.AddNode(dataNode.name, r)
		}
	}

	if reference.Data.removed {
		t.tree.SetNodeRemoved()
	}
}

func (a *App) saveKey(oldValue, newValue string) {
	if oldValue == newValue {
		a.main.ShowModalOK("Nothing to save")
		return
	}
	t := a.tree.DBTree
	typ := t.getReference(t.getCurrentNode())
	if typ == nil {
		return
	}
	switch typ.Name {
	case "key":
		if err := t.data.SetValue(typ.Data, newValue); err == nil {
			t.preview.ShowText(newValue, true)
			a.main.ShowModalOK("Value was updated!")
		} else {
			tlog.Log("saveKey %v", err)
		}
	}
}

func (a *App) GetConfig() ui.Setting {
	return ui.Setting{
		Name: a.config.Name,
		Host: a.config.Host,
		Port: strconv.Itoa(a.config.Port),
		Auth: a.config.Auth,
	}
}
