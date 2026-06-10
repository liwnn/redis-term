package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/liwnn/redisterm/datasource"
	"github.com/liwnn/redisterm/tlog"
	"github.com/liwnn/redisterm/tui/view"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// dsLevel marks what a tree node represents in a DSTree.
type dsLevel int

const (
	dsServer dsLevel = iota
	dsContainer
	dsEntry
)

// dsRef is attached to each DSTree node.
type dsRef struct {
	level     dsLevel
	container string // owning container, for entry nodes
	name      string // container or entry name
}

// DSTree is a read-only tree over a datasource.Datasource: server >
// container > entry, with entry content shown as a table in the preview.
// Used for backends (mongo) that don't share redis' write/tree semantics.
type DSTree struct {
	tree    *view.Tree
	preview *view.Preview
	src     datasource.Datasource

	// current entry + content, captured for inline cell editing
	curContainer string
	curEntry     string
	curQuery     string // active mongo filter for the current entry
	cur          datasource.Content

	// rootClick, when set, is consulted when the server (root) node is selected;
	// returning true means it was handled (e.g. a reconnect) and normal expansion
	// is skipped.
	rootClick func() bool

	ShowModal   func(text string, okFunc func())
	ShowModalOK func(string)
}

var _ treeController = (*DSTree)(nil)

// NewDSTree new
func NewDSTree(name string, src datasource.Datasource) *DSTree {
	tree := view.NewTree(name)
	tree.GetRoot().SetReference(&dsRef{level: dsServer, name: name})
	preview := view.NewPreview()
	preview.SetOpBtnVisible(false)
	preview.SetDeleteText("Drop")

	t := &DSTree{tree: tree, preview: preview, src: src}
	tree.SetSelectedFunc(t.onSelected)
	tree.SetChangedFunc(t.onChanged)
	preview.SetTableCommitFunc(t.commitCells)
	preview.SetTableReloadFunc(t.reloadCurrentTable)
	preview.SetDeleteFunc(t.dropEntry)
	preview.SetReloadFunc(t.reloadCurrentTable)
	preview.SetQueryFunc(t.runQuery)
	tree.SetInputCapture(t.onTreeKey)
	return t
}

// runQuery re-fetches the current entry using the user-typed filter and
// repaints the table. An empty query matches all documents.
func (t *DSTree) runQuery(query string) {
	if t.curEntry == "" {
		return
	}
	t.curQuery = query
	c, err := t.src.Content(t.curContainer, t.curEntry, datasource.Page{Query: query})
	if err != nil {
		t.preview.ShowText(fmt.Sprintf("%v", err), false)
		return
	}
	t.cur = c
	t.showContent(c)
}

// onTreeKey lets the user drop the selected collection or database with
// 'd' or Delete.
func (t *DSTree) onTreeKey(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyDelete || (event.Key() == tcell.KeyRune && event.Rune() == 'd') {
		node := t.tree.GetCurrentNode()
		ref, _ := node.GetReference().(*dsRef)
		if ref == nil {
			return event
		}
		switch ref.level {
		case dsEntry:
			t.dropEntry()
			return nil
		case dsContainer:
			t.dropContainer()
			return nil
		}
	}
	return event
}

// dropEntry asks for confirmation, then drops the selected collection.
func (t *DSTree) dropEntry() {
	node := t.tree.GetCurrentNode()
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil || ref.level != dsEntry {
		return
	}
	notice := fmt.Sprintf("Drop collection %v.%v ?", ref.container, ref.name)
	t.ShowModal(notice, func() {
		if err := t.src.DropEntry(ref.container, ref.name); err != nil {
			tlog.Log("[DSTree] drop %v", err)
			if t.ShowModalOK != nil {
				t.ShowModalOK(fmt.Sprintf("Drop failed: %v", err))
			}
			return
		}
		t.removeNode(node)
		t.preview.Clear()
		t.preview.ShowText("", false)
		t.preview.SetOpBtnVisible(false)
		t.preview.SetKey("")
	})
}

// dropContainer asks for confirmation, then drops the selected database.
func (t *DSTree) dropContainer() {
	node := t.tree.GetCurrentNode()
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil || ref.level != dsContainer {
		return
	}
	notice := fmt.Sprintf("Drop database %v ? This removes ALL collections.", ref.name)
	t.ShowModal(notice, func() {
		if err := t.src.DropContainer(ref.name); err != nil {
			tlog.Log("[DSTree] drop db %v", err)
			if t.ShowModalOK != nil {
				t.ShowModalOK(fmt.Sprintf("Drop failed: %v", err))
			}
			return
		}
		t.tree.GetRoot().RemoveChild(node)
		t.preview.Clear()
		t.preview.ShowText("", false)
		t.preview.SetOpBtnVisible(false)
		t.preview.SetKey("")
	})
}

// removeNode detaches a node from its parent in the tree.
func (t *DSTree) removeNode(node *tview.TreeNode) {
	root := t.tree.GetRoot()
	for _, parent := range root.GetChildren() { // container level
		for _, child := range parent.GetChildren() {
			if child == node {
				parent.RemoveChild(node)
				return
			}
		}
	}
}

// commitCells persists all staged document field edits for the current entry.
func (t *DSTree) commitCells(edits []view.CellEdit) error {
	for _, ed := range edits {
		if ed.Row < 0 || ed.Row >= len(t.cur.Rows) {
			continue
		}
		oldRow := t.cur.Rows[ed.Row]
		var oldRowTypes []string
		oldType := ""
		if t.cur.CellTypes != nil && ed.Row < len(t.cur.CellTypes) {
			oldRowTypes = t.cur.CellTypes[ed.Row]
			if ed.Col < len(oldRowTypes) {
				oldType = oldRowTypes[ed.Col]
			}
		}
		e := datasource.Edit{
			Columns:     t.cur.Columns,
			Row:         ed.Row,
			Column:      ed.Col,
			OldRow:      oldRow,
			OldRowTypes: oldRowTypes,
			Value:       ed.Value,
			OldType:     oldType,
		}
		if err := t.src.Update(t.curContainer, t.curEntry, e); err != nil {
			return err
		}
	}
	return nil
}

// reloadCurrentTable re-fetches the current entry and repaints the table,
// preserving the active query filter.
func (t *DSTree) reloadCurrentTable() {
	c, err := t.src.Content(t.curContainer, t.curEntry, datasource.Page{Query: t.curQuery})
	if err == nil {
		t.cur = c
		t.showContent(c)
	}
}

func (t *DSTree) onSelected(node *tview.TreeNode) {
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil {
		return
	}
	if ref.level == dsServer && t.rootClick != nil && t.rootClick() {
		return // root click triggered a reconnect; skip normal expansion
	}
	if len(node.GetChildren()) > 0 {
		return
	}
	switch ref.level {
	case dsServer:
		containers, err := t.src.Containers()
		if err != nil {
			tlog.Log("[DSTree] containers %v", err)
			return
		}
		for _, name := range containers {
			t.tree.AddNode(name, &dsRef{level: dsContainer, name: name})
		}
	case dsContainer:
		entries, err := t.src.Entries(ref.name)
		if err != nil {
			tlog.Log("[DSTree] entries %v", err)
			return
		}
		for _, name := range entries {
			t.tree.AddNode(name, &dsRef{level: dsEntry, container: ref.name, name: name})
		}
	}
}

func (t *DSTree) onChanged(node *tview.TreeNode) {
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil || ref.level != dsEntry {
		t.preview.SetOpBtnVisible(false)
		t.preview.Clear()
		t.preview.ShowText("", false)
		return
	}
	t.preview.SetOpBtnVisible(true)
	c, err := t.src.Content(ref.container, ref.name, datasource.Page{})
	if err != nil {
		t.preview.Clear()
		t.preview.ShowText(fmt.Sprintf("%v", err), false)
		return
	}
	t.curContainer = ref.container
	t.curEntry = ref.name
	t.curQuery = ""
	t.preview.SetQueryText("")
	t.cur = c
	t.showContent(c)
}

func (t *DSTree) showContent(c datasource.Content) {
	p := t.preview
	p.Clear()
	p.SetKeyType(c.Type)
	if c.Kind == datasource.KindText {
		p.ShowText(c.Text, false)
		return
	}
	title := make([]view.TablePageTitle, 0, len(c.Columns)+1)
	title = append(title, view.TablePageTitle{Name: "row", Expansion: 1})
	for _, col := range c.Columns {
		title = append(title, view.TablePageTitle{Name: col, Expansion: 6})
	}
	rows := make([]view.Row, 0, len(c.Rows))
	for _, r := range c.Rows {
		rows = append(rows, view.Row(r))
	}
	p.ShowTable(title, rows, c.CellTypes)
}

// Expand loads the container level (mongo databases) and expands the root node.
// Connect opens the datasource. It may block on the network, so Show calls it
// from a background goroutine before Expand.
func (t *DSTree) Connect() error {
	return t.src.Open()
}

// SetConnState sets the right-aligned connection-status glyph on the root row.
func (t *DSTree) SetConnState(s connState) {
	applyConnState(t.tree, s)
}

// Failed reports whether the root currently shows the failed status.
func (t *DSTree) Failed() bool {
	return t.tree.HasRootStatus('✖')
}

// SetRootClickFunc registers a handler consulted when the root node is selected.
func (t *DSTree) SetRootClickFunc(f func() bool) {
	t.rootClick = f
}

func (t *DSTree) Expand() {
	root := t.tree.GetRoot()
	t.tree.SetCurrentNode(root)
	t.onSelected(root)
	root.SetExpanded(true)
}

// Filter re-lists entries of every expanded container, keeping only those whose
// name contains term (case-insensitive). An empty term restores all entries.
// Collapsed containers are left untouched and re-filter when next expanded.
func (t *DSTree) Filter(term string) {
	needle := strings.ToLower(term)
	for _, cNode := range t.tree.GetRoot().GetChildren() {
		ref, _ := cNode.GetReference().(*dsRef)
		if ref == nil || ref.level != dsContainer {
			continue
		}
		if !cNode.IsExpanded() || len(cNode.GetChildren()) == 0 {
			continue
		}
		entries, err := t.src.Entries(ref.name)
		if err != nil {
			continue
		}
		cNode.ClearChildren()
		for _, name := range entries {
			if needle != "" && !strings.Contains(strings.ToLower(name), needle) {
				continue
			}
			t.tree.AddChildNode(cNode, name, &dsRef{level: dsEntry, container: ref.name, name: name})
		}
	}
}

// TreeView returns the underlying tview tree.
func (t *DSTree) TreeView() *view.Tree {
	return t.tree
}

// PreviewFlex returns the preview pane.
func (t *DSTree) PreviewFlex() *tview.Flex {
	return t.preview.FlexBox()
}

// Preview returns the preview pane controller.
func (t *DSTree) Preview() *view.Preview {
	return t.preview
}

// Index is meaningless for a datasource tree; always 0.
func (t *DSTree) Index() int {
	return 0
}

// Cmd runs a backend-native command via the datasource.
func (t *DSTree) Cmd(w io.Writer, cmd string, params ...string) error {
	raw := cmd
	for _, p := range params {
		raw += " " + p
	}
	reply, err := t.src.Command("", raw)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, reply)
	return err
}

// Close close
func (t *DSTree) Close() {
	t.src.Close()
}
