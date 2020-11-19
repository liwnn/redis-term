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
	tree *tview.TreeView
	data *Data

	selected *tview.TreeNode
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
	t.data.Select(typ.Index)
	childen := node.GetChildren()
	if len(childen) == 0 {
		switch typ.Name {
		case "db":
			Log("OnSelected: %v", typ.Name)
			for i, dataNode := range t.data.GetDatabases() {
				t.AddNode(node, dataNode.name, &Reference{
					Name:  "index",
					Index: i,
					Data:  dataNode,
				})
			}
		case "index":
			Log("OnSelected: %v %v", typ.Name, typ.Index)
			dataNodes := t.data.GetKeys()
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
			Log("OnSelected: %v %v", typ.Name, typ.Index)
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
		node.SetExpanded(!node.IsExpanded())
	}
	if typ.Data != nil && typ.Data.CanExpand() {
		if node.IsExpanded() {
			node.SetText("▼ " + typ.Data.name)
		} else {
			node.SetText("▶ " + typ.Data.name)
		}
	}
}

// OnChanged on change
func (t *DBTree) OnChanged(node *tview.TreeNode) {
	t.selected = node
	reference := node.GetReference()
	if reference == nil {
		return
	}
	typ, ok := reference.(*Reference)
	if !ok {
		log.Fatalf("reference \n")
	}
	if typ.Name == "key" {
		if !typ.Data.removed {
			Log("OnChanged: %v - %v", typ.Name, typ.Data.key)
			t.data.Select(typ.Index)
			o := t.data.GetValue(typ.Data.key)
			preview.SetContent(o, true)
		} else {
			preview.SetContent(fmt.Sprintf("%v was removed", typ.Data.key), false)
		}
		preview.SetDeleteText("Delete")
	} else {
		if typ.Name == "index" {
			preview.SetDeleteText("Flush")
		} else {
			preview.SetDeleteText("Delete")
		}
		preview.SetContent("", false)
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

func (t *DBTree) deleteSelectKey(typ *Reference) {
	switch typ.Name {
	case "key":
		Log("delete %v", typ.Data.key)
		t.data.Delete(typ.Data)
		t.selected.SetText(typ.Data.key + " (Removed)")
		t.selected.SetColor(tcell.ColorGray)
		preview.SetContent(fmt.Sprintf("%v was removed", typ.Data.key), false)
	case "index":
		t.data.FlushDB(typ.Data)
		t.selected.ClearChildren()
		t.selected.SetText(typ.Data.name)
	case "dir":
		t.data.Delete(typ.Data)
		t.selected.SetText(typ.Data.name + " (Removed)")
		t.selected.SetColor(tcell.ColorGray)
		t.selected.ClearChildren()
		preview.SetContent("", false)
	default:
		Log("delete %v not implement", typ.Name)
	}
}

func (t *DBTree) reloadSelectKey() {
	reference := t.getReference(t.selected)
	if reference == nil {
		return
	}
	Log("reload %v", reference.Data.key)

	if reference.Name == "key" {
		t.data.Select(reference.Index)
		o := t.data.GetValue(reference.Data.key)
		if o == nil {
			reference.Data.removed = true
			t.selected.SetText(reference.Data.name + " (Removed)")
			preview.SetContent(fmt.Sprintf("%v was removed", reference.Data.key), false)
			preview.SetDeleteText("Delete")
		} else {
			preview.SetContent(o, true)
		}
		return
	}

	t.selected.ClearChildren()
	t.data.Reload(reference.Data)

	node := t.selected
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
		t.selected.SetText(reference.Data.name + " (Removed)")
		t.selected.SetColor(tcell.ColorGray)
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
func Run(host string, port int) {
	client := NewRedis(fmt.Sprintf("%v:%v", host, port))
	defer client.Close()
	data := NewData(client)

	tree := NewDBTree(host, data)

	keyFlexBox := tview.NewFlex()
	keyFlexBox.SetDirection(tview.FlexRow)
	keyFlexBox.SetBorder(true)
	keyFlexBox.SetTitle("KEYS")
	keyFlexBox.AddItem(tree.tree, 0, 1, true)

	preview = NewPreview()
	SetLogger(preview.output)
	preview.SetDeleteFunc(func() {
		typ := tree.getReference(tree.selected)
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

	mainFlexBox := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(keyFlexBox, 0, 1, true).
		AddItem(preview.flexBox, 0, 4, false)
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
