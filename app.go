package redisterm

import (
	"fmt"
	"log"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	preview *Preview
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
			dataNodes := t.data.GetKeys(typ.Index)
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
			o := t.data.GetValue(typ.Index, typ.Data.key)
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

func (t *DBTree) deleteSelectKey() {
	if t.selected == nil {
		return
	}

	reference := t.selected.GetReference()
	if reference == nil {
		return
	}
	typ, ok := reference.(*Reference)
	if !ok {
		log.Fatalf("reference \n")
	}
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
		var delKey = make([]string, 0, len(typ.Data.GetChildren()))
		for _, v := range typ.Data.GetChildren() {
			t.data.Delete(v)
			delKey = append(delKey, v.key)
		}
		Log("delete %v", delKey)
		t.selected.SetText(typ.Data.name + " (Removed)")
		t.selected.SetColor(tcell.ColorGray)
		t.selected.ClearChildren()
		preview.SetContent("", false)
	default:
		Log("delete %v not implement", typ.Name)
	}
}

func (t *DBTree) reloadSelectKey() {
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
	preview.SetDeleteFunc(tree.deleteSelectKey)
	preview.SetReloadFunc(tree.reloadSelectKey)

	mainFlexBox := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(keyFlexBox, 0, 1, true).
		AddItem(preview.flexBox, 0, 4, false)

	pages := tview.NewPages()
	pages.AddPage("main", mainFlexBox, true, true)

	app := tview.NewApplication()
	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
