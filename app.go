package redisterm

import (
	"fmt"
	"log"
	"strconv"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

// Reference referenct
type Reference struct {
	Name  string
	Index int
}

// DBTree tree.
type DBTree struct {
	root *tview.TreeNode
	tree *tview.TreeView

	redis *Redis
}

// NewDBTree new
func NewDBTree(rootName string, redis *Redis) *DBTree {
	root := tview.NewTreeNode(rootName).SetColor(tcell.ColorRed)
	root.SetReference(&Reference{
		Name: "db",
	})
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	dbTree := &DBTree{
		root:  root,
		tree:  tree,
		redis: redis,
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
	//node.SetIndent(0)
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
			dbNum, err := t.redis.GetDatabases()
			if err != nil {
				log.Fatalln(err)
			}

			for index := 0; index < dbNum; index++ {
				t.AddNode(node, "db"+strconv.Itoa(index), &Reference{
					Name:  "index",
					Index: index,
				})
			}
		case "index":
			Log("OnSelected: %v %v", typ.Name, typ.Index)
			t.redis.Select(typ.Index)
			for _, v := range t.redis.Keys("*") {
				t.AddNode(node, v, &Reference{
					Name:  "key",
					Index: typ.Index,
				})
			}
		}
	} else {
		node.SetExpanded(!node.IsExpanded())
	}
}

// OnChanged on change
func (t *DBTree) OnChanged(node *tview.TreeNode) {
	reference := node.GetReference()
	if reference == nil {
		return
	}
	previewText.SetText("")
	typ, ok := reference.(*Reference)
	if !ok {
		log.Fatalf("reference \n")
	}
	if typ.Name == "key" {
		Log("OnChanged: %v - %v", typ.Name, node.GetText())
		t.redis.Select(typ.Index)
		val := t.redis.Type(node.GetText())
		switch val {
		case "string":
			v := t.redis.Get(node.GetText())
			previewText.SetText(v)
		default:
			previewText.SetText(fmt.Sprintf("%v not implement!!!", val))
		}
	}
}

var (
	previewText *tview.TextView
	outputText  *tview.TextView
)

// Run run
func Run() {
	client := NewRedis("127.0.0.1:6379")
	defer client.Close()

	pages := tview.NewPages()

	tree := NewDBTree("127.0.0.1", client)

	keyFlexBox := tview.NewFlex()
	keyFlexBox.SetDirection(tview.FlexRow)
	keyFlexBox.SetBorder(true)
	keyFlexBox.SetTitle("KEYS")
	keyFlexBox.AddItem(tree.tree, 0, 1, true)

	previewFlexBox := tview.NewFlex()
	previewFlexBox.SetDirection(tview.FlexRow)
	previewText = tview.NewTextView()
	previewText.
		SetDynamicColors(true).
		SetRegions(true).
		SetScrollable(true).
		SetTitle("PREVIEW").
		SetBorder(true).
		SetBorderColor(tcell.ColorSteelBlue)

	outputText = tview.NewTextView()
	outputText.SetScrollable(true).SetTitle("CONSOLE").SetBorder(true)

	previewFlexBox.AddItem(previewText, 0, 3, false)
	previewFlexBox.AddItem(outputText, 0, 1, false)

	mainFlexBox := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(keyFlexBox, 0, 1, true).
		AddItem(previewFlexBox, 0, 4, false)

	pages.AddPage("main", mainFlexBox, true, true)

	app := tview.NewApplication()
	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
