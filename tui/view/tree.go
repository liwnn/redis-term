package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Tree tree
type Tree struct {
	*tview.TreeView

	// rootStatus is a single status glyph drawn right-aligned on the root row
	// (connection state). Empty means draw nothing. Set via SetRootStatus.
	rootStatus      rune
	rootStatusColor tcell.Color
}

// treeSelectedStyle is the highlight applied to the focused tree node. tview's
// default inverts to a white background which is too bright on the dark panel;
// this keeps the calm blue used by the table/dropdown selection.
var treeSelectedStyle = tcell.StyleDefault.Background(ThemeSelectBG).Foreground(tcell.ColorWhite)

// NewTree new
func NewTree(rootName string) *Tree {
	root := tview.NewTreeNode(rootName).SetColor(tcell.ColorYellow).SetSelectedTextStyle(treeSelectedStyle)
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	tree.SetBorder(true)
	// tree.SetTitle("KEYS")
	tree.SetBackgroundColor(ThemePanelBG)
	focusBorder(tree.Box)

	t := &Tree{
		TreeView: tree,
	}
	return t
}

// SetRootStatus sets the right-aligned status glyph on the root row. A zero rune
// clears it. The glyph is drawn by Draw, independent of the root node's text.
func (t *Tree) SetRootStatus(glyph rune, color tcell.Color) {
	t.rootStatus = glyph
	t.rootStatusColor = color
}

// HasRootStatus reports whether the root currently shows the given status glyph.
func (t *Tree) HasRootStatus(glyph rune) bool {
	return t.rootStatus == glyph
}

// Draw renders the tree, then overlays the root status glyph at the right edge
// of the root's row so it stays flush-right regardless of the name length.
func (t *Tree) Draw(screen tcell.Screen) {
	t.TreeView.Draw(screen)
	if t.rootStatus == 0 {
		return
	}
	x, y, w, _ := t.GetInnerRect()
	if w < 1 {
		return
	}
	// The root sits on the first inner row of the tree (top-aligned, no scroll
	// offset since it's the first node). tview lets a long root name run to the
	// right edge, so blank the last few cells to carve out clear space and draw
	// the glyph flush-right.
	blank := tcell.StyleDefault.Background(ThemePanelBG)
	screen.SetContent(x+w-3, y, ' ', nil, blank)
	screen.SetContent(x+w-1, y, ' ', nil, blank)
	style := tcell.StyleDefault.Background(ThemePanelBG).Foreground(t.rootStatusColor)
	screen.SetContent(x+w-2, y, t.rootStatus, nil, style)
	// When the name is long enough to reach the carved-out area, it was truncated
	// with no indicator — overwrite the cell just before the cleared gap with an
	// ellipsis so it's clear the name is cut off.
	nameWidth := len([]rune(t.GetRoot().GetText()))
	if nameWidth >= w-3 {
		nameStyle := tcell.StyleDefault.Background(ThemePanelBG).Foreground(tcell.ColorYellow)
		screen.SetContent(x+w-4, y, '…', nil, nameStyle)
	}
}

// AddNode add node
func (t *Tree) AddNode(name string, reference interface{}) {
	node := tview.NewTreeNode(name).SetSelectable(true)
	if reference != nil {
		node.SetReference(reference)
	}
	node.SetColor(tcell.NewRGBColor(136, 199, 150)).SetSelectedTextStyle(treeSelectedStyle)
	t.TreeView.GetCurrentNode().AddChild(node)
}

// AddChildNode adds a child node under the given parent (not the current node).
func (t *Tree) AddChildNode(parent *tview.TreeNode, name string, reference interface{}) *tview.TreeNode {
	node := tview.NewTreeNode(name).SetSelectable(true)
	if reference != nil {
		node.SetReference(reference)
	}
	node.SetColor(tcell.NewRGBColor(136, 199, 150)).SetSelectedTextStyle(treeSelectedStyle)
	parent.AddChild(node)
	return node
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
		// Toggle expand/collapse on Enter. When children aren't loaded yet
		// (len == 0) we skip the toggle and let handler lazily populate them;
		// the node's default-expanded state then reveals them immediately.
		if len(node.GetChildren()) > 0 {
			node.SetExpanded(!node.IsExpanded())
		}
		handler(node)
	})
}
