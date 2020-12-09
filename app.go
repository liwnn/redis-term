package redisterm

import (
	"fmt"
	"log"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	pages   *tview.Pages
	preview *Preview
	modal   *tview.Modal
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
}

// NewDBTree new
func NewDBTree(rootName string, data *Data) *DBTree {
	root := tview.NewTreeNode(rootName).SetColor(tcell.ColorYellow)
	root.SetReference(&Reference{
		Name: "db",
	})
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	dbTree := &DBTree{
		tree: tree,
		data: data,
	}
	tree.SetSelectedFunc(dbTree.OnSelected)
	tree.SetChangedFunc(dbTree.OnChanged)
	return dbTree
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
		preview.SetOpBtnVisible(false)
	} else {
		if typ.Name == "index" {
			Log("OnChanged: %v - %v", typ.Name, typ.Index)
		} else {
			Log("OnChanged: %v - %v", typ.Name, typ.Data.key)
		}
		preview.SetOpBtnVisible(true)
	}

	if typ.Name == "key" {
		if !typ.Data.removed {
			t.data.Select(typ.Index)
			o := t.data.GetValue(typ.Data.key)
			preview.SetContent(o, true)
		} else {
			preview.SetContent(fmt.Sprintf("%v was removed", typ.Data.key), false)
		}
		preview.SetDeleteText("Delete")
		preview.SetKey(typ.Data.key)
	} else {
		if typ.Name == "index" {
			preview.SetDeleteText("Flush")
		} else {
			preview.SetDeleteText("Delete")
		}
		preview.SetContent("", false)
		preview.SetKey("")
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
		preview.SetContent(fmt.Sprintf("%v was removed", typ.Data.key), false)
	case "index":
		t.data.FlushDB(typ.Data)
		t.getCurrentNode().ClearChildren()
		t.getCurrentNode().SetText(typ.Data.name)
	case "dir":
		t.data.Delete(typ.Data)
		t.getCurrentNode().SetText(typ.Data.name + " (Removed)")
		t.getCurrentNode().SetColor(tcell.ColorGray)
		t.getCurrentNode().ClearChildren()
		preview.SetContent("", false)
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

	notice := fmt.Sprintf("Rename %v->%v", reference.Data.key, preview.GetKey())
	ShowModal(notice, func() {
		if reference.Data.key == preview.GetKey() {
			return
		}

		Log("rename %v %v", reference.Data.key, preview.GetKey())
		t.data.Rename(reference.Data, preview.GetKey())
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
			preview.SetContent(fmt.Sprintf("%v was removed", reference.Data.key), false)
			preview.SetDeleteText("Delete")
		} else {
			preview.SetContent(o, true)
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

// Run run
func Run(host string, port int, auth string) {
	client := NewRedis(fmt.Sprintf("%v:%v", host, port), auth)
	defer client.Close()
	data := NewData(client)

	tree := NewDBTree(host, data)

	keyFlexBox := tview.NewFlex()
	keyFlexBox.SetDirection(tview.FlexRow)
	keyFlexBox.SetBorder(true)
	keyFlexBox.SetTitle("KEYS")
	keyFlexBox.AddItem(tree.tree, 0, 1, true)

	preview = NewPreview()
	preview.SetDeleteFunc(func() {
		typ := tree.getReference(tree.getCurrentNode())
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
			tree.deleteSelectKey(typ)
		})
	})
	preview.SetReloadFunc(tree.reloadSelectKey)
	preview.SetRenameFunc(tree.renameSelectKey)
	bottomPanel := createBottom()
	rightFlexBox := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(preview.flexBox, 0, 3, false).
		AddItem(bottomPanel, 0, 1, false)

	mainFlexBox := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(keyFlexBox, 0, 1, true).
		AddItem(rightFlexBox, 0, 4, false)
	modal = tview.NewModal().
		AddButtons([]string{"Ok", "Cancel"})

	pages = tview.NewPages()
	pages.AddPage("main", mainFlexBox, true, true)
	pages.AddPage("modal", modal, true, false)

	app := tview.NewApplication()
	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func createBottom() tview.Primitive {
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
