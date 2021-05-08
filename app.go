package redisterm

import (
	"fmt"
	"log"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

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

	pageDelta int
}

// NewDBTree new
func NewDBTree(tree *ui.Tree, preview *ui.Preview) *DBTree {
	dbTree := &DBTree{
		tree:      tree,
		preview:   preview,
		pageDelta: 1000,
	}

	tree.SetSelectedFunc(dbTree.OnSelected)
	tree.SetChangedFunc(dbTree.OnChanged)
	tree.SetMouseCapture(dbTree.onCapture)

	preview.SetReloadFunc(dbTree.reloadSelectKey)

	return dbTree
}

func (t *DBTree) onCapture(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
	return action, event
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
	Log("OnSelected: %v %v", typ.Name, typ.Index)

	t.data.Select(typ.Index)
	childen := node.GetChildren()
	if len(childen) == 0 {
		var dataNodes []*DataNode
		switch typ.Name {
		case "db":
			for i, dataNode := range t.data.GetDatabases() {
				t.tree.AddNode(dataNode.name, &Reference{
					Name:  "index",
					Index: i,
					Data:  dataNode,
				})
			}
		case "index":
			//dataNodes := t.data.GetKeys()
			dataNodes = t.data.ScanAllKeys()
		case "dir":
			dataNodes = t.data.GetChildren(typ.Data)
		}
		for _, dataNode := range dataNodes {
			r := &Reference{
				Index: typ.Index,
				Data:  dataNode,
			}
			if dataNode.CanExpand() {
				r.Name = "dir"
				t.tree.AddNode("▶ "+dataNode.name, r)
			} else {
				r.Name = "key"
				t.tree.AddNode(dataNode.name, r)
			}
		}
	}
	if typ.Data != nil && typ.Data.CanExpand() {
		t.tree.SetNodeText(typ.Data.name)
	}
}

// OnChanged on change
func (t *DBTree) OnChanged(node *tview.TreeNode) {
	typ := t.getReference(node)
	if typ.Name == "db" {
		Log("OnChanged: db %v", typ.Name)
		t.preview.SetOpBtnVisible(false)
	} else {
		if typ.Name == "index" {
			Log("OnChanged: %v - %v", typ.Name, typ.Index)
		} else {
			Log("OnChanged: %v - %v", typ.Name, typ.Data.key)
		}
		t.preview.SetOpBtnVisible(true)
	}

	if typ.Name == "key" {
		if !typ.Data.removed {
			t.data.Select(typ.Index)
			begin := time.Now()
			o := t.data.GetValue(typ.Data.key)
			Log("redis value time cost %v", time.Since(begin))
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

	var count int
	switch o.(type) {
	case string:
		text := o.(string)
		page := ui.NewTextPage(text)
		p.AddPage(page)
		if valid {
			p.SetSizeText(fmt.Sprintf("Size: %d bytes", len(text)))
		}
	case []string:
		h := o.([]string)
		count = len(h)
		pageCount := len(h) / t.pageDelta
		if len(h)%t.pageDelta > 0 {
			pageCount++
		}
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
		for i := 0; i < (pageCount - 1); i++ {
			rowData := h[i*t.pageDelta : (i+1)*t.pageDelta]
			var rows = make([][]string, 0, len(rowData))
			for _, data := range rowData {
				rows = append(rows, []string{data})
			}
			offset := i*t.pageDelta + 1
			page := ui.NewTablePage(title, rows, offset)
			p.AddPage(page)
		}
		rowData := h[(pageCount-1)*t.pageDelta:]
		var rows = make([][]string, 0, len(rowData))
		for _, data := range rowData {
			rows = append(rows, []string{data})
		}
		offset := (pageCount-1)*t.pageDelta + 1
		page := ui.NewTablePage(title, rows, offset)
		p.AddPage(page)
	case []KVText:
		h := o.([]KVText)
		count = len(h)
		pageCount := len(h) / t.pageDelta
		if len(h)%t.pageDelta > 0 {
			pageCount++
		}
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
		for i := 0; i < pageCount-1; i++ {
			rowData := h[i*t.pageDelta : (i+1)*t.pageDelta]
			var rows = make([][]string, 0, len(rowData))
			for _, data := range rowData {
				rows = append(rows, []string{data.Key, data.Value})
			}

			offset := i*t.pageDelta + 1
			page := ui.NewTablePage(title, rows, offset)
			p.AddPage(page)
		}
		rowData := h[(pageCount-1)*t.pageDelta:]
		var rows = make([][]string, 0, len(rowData))
		for _, data := range rowData {
			rows = append(rows, []string{data.Key, data.Value})
		}
		offset := (pageCount-1)*t.pageDelta + 1
		page := ui.NewTablePage(title, rows, offset)
		p.AddPage(page)
	}

	p.SetContent(count)
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
		Log("delete %v", typ.Data.key)
		t.data.Delete(typ.Data)
		t.getCurrentNode().SetText(typ.Data.key + " (Removed)")
		t.getCurrentNode().SetColor(tcell.ColorGray)
		t.updatePreview(fmt.Sprintf("%v was removed", typ.Data.key), false)
	case "index":
		t.data.FlushDB(typ.Data)
		t.getCurrentNode().ClearChildren()
		t.getCurrentNode().SetText(typ.Data.name)
	case "dir":
		t.data.Delete(typ.Data)
		t.getCurrentNode().SetText(typ.Data.name + " (Removed)")
		t.getCurrentNode().SetColor(tcell.ColorGray)
		t.getCurrentNode().ClearChildren()
		t.updatePreview("", false)
	default:
		Log("delete %v not implement", typ.Name)
	}
}

func (t *DBTree) reloadSelectKey() {
	reference := t.getReference(t.getCurrentNode())
	if reference == nil {
		return
	}
	Log("reload %v", reference.Data.key)

	if reference.Name == "key" {
		t.data.Select(reference.Index)
		o := t.data.GetValue(reference.Data.key)
		if o == nil {
			reference.Data.removed = true
			t.getCurrentNode().SetText(reference.Data.name + " (Removed)")
			t.updatePreview(fmt.Sprintf("%v was removed", reference.Data.key), false)
			t.preview.SetDeleteText("Delete")
		} else {
			t.updatePreview(o, true)
		}
		return
	}

	t.getCurrentNode().ClearChildren()
	t.data.Reload(reference.Data)

	childen := reference.Data.GetChildren()
	for _, dataNode := range childen {
		r := &Reference{
			Index: reference.Index,
			Data:  dataNode,
		}
		if dataNode.CanExpand() {
			r.Name = "dir"
			t.tree.AddNode("▶ "+dataNode.name, r)
		} else {
			r.Name = "key"
			t.tree.AddNode(dataNode.name, r)
		}
	}

	if reference.Data.removed {
		t.getCurrentNode().SetText(reference.Data.name + " (Removed)")
		t.getCurrentNode().SetColor(tcell.ColorGray)
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

	tree   *DBShow
	dbTree map[string]*DBShow

	configs []RedisConfig
}

// NewApp new
func NewApp() *App {
	return &App{
		dbTree: make(map[string]*DBShow),
	}
}

// Run run
func (a *App) Run(configs ...RedisConfig) {
	main := ui.NewMainView()
	a.main = main
	main.InitLayout()

	a.configs = configs

	for _, config := range a.configs {
		main.AddSelect(config.Host)
	}
	main.SetSelectedFunc(func(index int) {
		a.Show(a.configs[index])
	})
	SetLogger(main.GetOutput())
	main.Show(0)
	main.SetCmdLineEnter(a.onCmdLineEnter)

	if err := main.Run(); err != nil {
		panic(err)
	}

	for _, client := range a.dbTree {
		client.Close()
	}
}

// Show show
func (a *App) Show(config RedisConfig) {
	address := fmt.Sprintf("%v:%v", config.Host, config.Port)
	t, ok := a.dbTree[address]
	if !ok {
		client := NewRedis(address, config.Auth)
		data := NewData(client)

		tree := ui.NewTree("")
		tree.GetRoot().SetReference(&Reference{
			Name: "db",
		})
		preview := ui.NewPreview()

		t = &DBShow{
			DBTree:  NewDBTree(tree, preview),
			Preview: preview,
		}
		t.SetData(config.Host, data)
		a.dbTree[address] = t

		preview.SetRenameFunc(a.renameSelectKey)
		preview.SetDeleteFunc(a.deleteKey)
	}

	a.tree = t

	a.main.SetTree(a.tree.tree.TreeView)
	a.main.SetPreview(a.tree.preview.FlexBox())
}

func (a *App) onCmdLineEnter(text string) {
	view := a.main.GetCmdWriter()
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

		Log("rename %v %v", reference.Data.key, a.tree.Preview.GetKey())
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
		t.deleteSelectKey(typ)
	})
}
