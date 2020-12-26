package redisterm

import (
	"fmt"
	"log"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	pages *tview.Pages
	modal *tview.Modal
)

// Reference referenct
type Reference struct {
	Name  string
	Index int
	Data  *DataNode
}

// DBTree tree.
type DBTree struct {
	tree     *tview.TreeView
	data     *Data
	lastNode *tview.TreeNode
	preview  *Preview
}

// NewDBTree new
func NewDBTree(tree *tview.TreeView, preview *Preview) *DBTree {
	dbTree := &DBTree{
		tree:    tree,
		preview: preview,
	}

	tree.SetSelectedFunc(dbTree.OnSelected)
	tree.SetChangedFunc(dbTree.OnChanged)

	preview.SetDeleteFunc(dbTree.deleteKey)
	preview.SetReloadFunc(dbTree.reloadSelectKey)
	preview.SetRenameFunc(dbTree.renameSelectKey)

	return dbTree
}

// SetData change db data.
func (t *DBTree) SetData(name string, data *Data) {
	t.tree.GetRoot().ClearChildren()
	t.tree.GetRoot().SetText(name)
	t.data = data
}

// AddNode add node
func (t *DBTree) AddNode(target *tview.TreeNode, name string, reference *Reference) {
	node := tview.NewTreeNode(name).SetSelectable(true)
	if reference != nil {
		node.SetReference(reference)
	}
	node.SetColor(tcell.ColorGreen)
	target.AddChild(node)
}

// OnSelected on select
func (t *DBTree) OnSelected(node *tview.TreeNode) {
	reference := node.GetReference()
	if reference == nil {
		return
	}
	typ, ok := reference.(*Reference)
	if !ok {
		log.Fatalf("reference \n")
	}
	Log("OnSelected: %v %v", typ.Name, typ.Index)

	t.data.Select(typ.Index)
	childen := node.GetChildren()
	if len(childen) == 0 {
		switch typ.Name {
		case "db":
			for i, dataNode := range t.data.GetDatabases() {
				t.AddNode(node, dataNode.name, &Reference{
					Name:  "index",
					Index: i,
					Data:  dataNode,
				})
			}
		case "index":
			//dataNodes := t.data.GetKeys()
			dataNodes := t.data.ScanAllKeys()
			for _, dataNode := range dataNodes {
				r := &Reference{
					Index: typ.Index,
					Data:  dataNode,
				}
				if dataNode.CanExpand() {
					r.Name = "dir"
					t.AddNode(node, "▶ "+dataNode.name, r)
				} else {
					r.Name = "key"
					t.AddNode(node, dataNode.name, r)
				}
			}
		case "dir":
			dataNodes := t.data.GetChildren(typ.Data)
			for _, dataNode := range dataNodes {
				r := &Reference{
					Index: typ.Index,
					Data:  dataNode,
				}
				if dataNode.CanExpand() {
					r.Name = "dir"
					t.AddNode(node, "▶ "+dataNode.name, r)
				} else {
					r.Name = "key"
					t.AddNode(node, dataNode.name, r)
				}
			}
		}
	} else {
		if t.tree.GetCurrentNode() != t.lastNode && node.IsExpanded() {
			t.lastNode = node
			return
		}
		node.SetExpanded(!node.IsExpanded())
	}
	if typ.Data != nil && typ.Data.CanExpand() {
		if node.IsExpanded() {
			node.SetText("▼ " + typ.Data.name)
		} else {
			node.SetText("▶ " + typ.Data.name)
		}
	}
	t.lastNode = node
}

// OnChanged on change
func (t *DBTree) OnChanged(node *tview.TreeNode) {
	reference := node.GetReference()
	if reference == nil {
		return
	}
	typ, ok := reference.(*Reference)
	if !ok {
		log.Fatalf("reference \n")
	}
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
			t.preview.SetContent(o, true)
		} else {
			t.preview.SetContent(fmt.Sprintf("%v was removed", typ.Data.key), false)
		}
		t.preview.SetDeleteText("Delete")
		t.preview.SetKey(typ.Data.key)
	} else {
		if typ.Name == "index" {
			t.preview.SetDeleteText("Flush")
		} else {
			t.preview.SetDeleteText("Delete")
		}
		t.preview.SetContent("", false)
		t.preview.SetKey("")
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
		Log("delete %v", typ.Data.key)
		t.data.Delete(typ.Data)
		t.getCurrentNode().SetText(typ.Data.key + " (Removed)")
		t.getCurrentNode().SetColor(tcell.ColorGray)
		t.preview.SetContent(fmt.Sprintf("%v was removed", typ.Data.key), false)
	case "index":
		t.data.FlushDB(typ.Data)
		t.getCurrentNode().ClearChildren()
		t.getCurrentNode().SetText(typ.Data.name)
	case "dir":
		t.data.Delete(typ.Data)
		t.getCurrentNode().SetText(typ.Data.name + " (Removed)")
		t.getCurrentNode().SetColor(tcell.ColorGray)
		t.getCurrentNode().ClearChildren()
		t.preview.SetContent("", false)
	default:
		Log("delete %v not implement", typ.Name)
	}
}

func (t *DBTree) renameSelectKey() {
	reference := t.getReference(t.getCurrentNode())
	if reference == nil {
		return
	}
	if reference.Name != "key" {
		return
	}

	notice := fmt.Sprintf("Rename %v->%v", reference.Data.key, t.preview.GetKey())
	ShowModal(notice, func() {
		if reference.Data.key == t.preview.GetKey() {
			return
		}

		Log("rename %v %v", reference.Data.key, t.preview.GetKey())
		t.data.Rename(reference.Data, t.preview.GetKey())
		t.getCurrentNode().SetText(reference.Data.name)
	})
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
			t.preview.SetContent(fmt.Sprintf("%v was removed", reference.Data.key), false)
			t.preview.SetDeleteText("Delete")
		} else {
			t.preview.SetContent(o, true)
		}
		return
	}

	t.getCurrentNode().ClearChildren()
	t.data.Reload(reference.Data)

	node := t.getCurrentNode()
	childen := reference.Data.GetChildren()
	for _, dataNode := range childen {
		r := &Reference{
			Index: reference.Index,
			Data:  dataNode,
		}
		if dataNode.CanExpand() {
			r.Name = "dir"
			t.AddNode(node, "▶ "+dataNode.name, r)
		} else {
			r.Name = "key"
			t.AddNode(node, dataNode.name, r)
		}
	}

	if reference.Data.removed {
		t.getCurrentNode().SetText(reference.Data.name + " (Removed)")
		t.getCurrentNode().SetColor(tcell.ColorGray)
	}
}

func (t *DBTree) deleteKey() {
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
	ShowModal(notice, func() {
		t.deleteSelectKey(typ)
	})
}

// Close close
func (t *DBTree) Close() {
	t.data.Close()
}

// ShowModal show modal
func ShowModal(text string, okFunc func()) {
	modal.SetText(text).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				okFunc()
			}
			pages.HidePage("modal")
		})
	pages.ShowPage("modal")
}

// RedisConfig config
type RedisConfig struct {
	Name string
	Host string
	Port int
	Auth string
}

// App app
type App struct {
	tree         *DBTree
	leftFlexBox  *tview.Flex
	rightFlexBox *tview.Flex
	bottomPanel  tview.Primitive
	selectDrop   *tview.DropDown

	dbTree map[string]*DBTree
}

// NewApp new
func NewApp() *App {
	return &App{
		dbTree: make(map[string]*DBTree),
	}
}

// Run run
func (a *App) Run(configs ...RedisConfig) {
	a.selectDrop = a.createSelectDB(configs...)
	a.leftFlexBox = tview.NewFlex().SetDirection(tview.FlexRow)
	a.bottomPanel = a.createBottom()
	a.rightFlexBox = tview.NewFlex().SetDirection(tview.FlexRow)

	mainFlexBox := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.leftFlexBox, 0, 1, true).
		AddItem(a.rightFlexBox, 0, 4, false)

	modal := a.createModal()

	pages = tview.NewPages()
	pages.AddPage("main", mainFlexBox, true, true)
	pages.AddPage("modal", modal, true, false)

	a.selectDrop.SetCurrentOption(0)

	if err := tview.NewApplication().SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
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
		preview := NewPreview()
		t = NewDBTree(a.createTree(""), preview)
		t.SetData(config.Host, data)

		a.dbTree[address] = t
	}

	a.tree = t

	a.leftFlexBox.Clear()
	a.leftFlexBox.AddItem(a.selectDrop, 1, 0, false)
	a.leftFlexBox.AddItem(a.tree.tree, 0, 1, true)

	a.rightFlexBox.Clear()
	a.rightFlexBox.AddItem(a.tree.preview.flexBox, 0, 3, false)
	a.rightFlexBox.AddItem(a.bottomPanel, 0, 1, false)
}

func (a *App) createTree(rootName string) *tview.TreeView {
	root := tview.NewTreeNode(rootName).SetColor(tcell.ColorYellow)
	root.SetReference(&Reference{
		Name: "db",
	})
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	tree.SetBorder(true)
	tree.SetTitle("KEYS")

	return tree
}

func (a *App) createSelectDB(configs ...RedisConfig) *tview.DropDown {
	dbSel := tview.NewDropDown().SetLabel("Select server:")
	for _, config := range configs {
		t := config
		dbSel.AddOption(t.Host, func() {
			a.Show(t)
		})
	}
	return dbSel
}

func (a *App) createModal() *tview.Modal {
	modal = tview.NewModal().
		AddButtons([]string{"Ok", "Cancel"})
	return modal
}

func (a *App) createBottom() tview.Primitive {
	pages := tview.NewPages()

	info := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false).
		SetHighlightedFunc(func(added, removed, remaining []string) {
			pages.SwitchToPage(added[0])
		})

	{
		title := "CONSOLE"
		console := tview.NewTextView()
		console.
			SetScrollable(true).
			SetTitle(title).
			SetBorder(true)
		SetLogger(console)
		pages.AddPage(title, console, true, true)
		fmt.Fprintf(info, `["%v"][slategrey]%s[white][""] `, title, title)
	}

	{
		title := "redis-cli"
		cmdLine := tview.NewInputField()
		cmdLine.SetPlaceholder("input command")
		cmdLine.SetFieldBackgroundColor(tcell.ColorDarkSlateGrey)
		cmdLine.SetPlaceholderTextColor(tcell.ColorDimGrey)
		view := tview.NewTextView()
		redisCli := tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(view, 0, 1, false).
			AddItem(cmdLine, 1, 1, true)
		redisCli.SetBorder(true)
		pages.AddPage(title, redisCli, true, false)
		fmt.Fprintf(info, `["%v"][slategrey]%s[white][""] `, title, title)
	}

	info.Highlight("CONSOLE")

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(pages, 0, 1, false).
		AddItem(info, 1, 1, false)
	return layout
}
