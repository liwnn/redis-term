package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Tree tree
type Tree struct {
	*tview.TreeView
	lastNode *tview.TreeNode
}

// NewTree new
func NewTree(rootName string) *Tree {
	root := tview.NewTreeNode(rootName).SetColor(tcell.ColorYellow)
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	tree.SetBorder(true)
	tree.SetTitle("KEYS")

	t := &Tree{
		TreeView: tree,
	}
	return t
}

// AddNode add node
func (t *Tree) AddNode(name string, reference interface{}) {
	node := tview.NewTreeNode(name).SetSelectable(true)
	if reference != nil {
		node.SetReference(reference)
	}
	node.SetColor(tcell.ColorGreen)
	t.TreeView.GetCurrentNode().AddChild(node)
}

// SetNodeRemoved set node removed
func (t *Tree) SetNodeRemoved() {
	node := t.GetCurrentNode()
	text := node.GetText() + " (Removed)"
	node.SetText(text)
	node.SetColor(tcell.ColorGray)
	node.ClearChildren()
}

// SetSelectedFunc on select
func (t *Tree) SetSelectedFunc(handler func(node *tview.TreeNode)) {
	t.TreeView.SetSelectedFunc(func(node *tview.TreeNode) {
		if len(node.GetChildren()) > 0 {
			if t.GetCurrentNode() != t.lastNode && node.IsExpanded() {
				t.lastNode = node
				return
			}
			node.SetExpanded(!node.IsExpanded())
		}
		handler(node)
		t.lastNode = node
	})
}
