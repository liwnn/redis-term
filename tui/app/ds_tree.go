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
	path      string // full backend path (nested mode); equals the znode path
	folder    bool   // nested mode: has children (rendered with a ▶/▼ twistie)
}

// DSTree is a read-only tree over a datasource.Datasource: server >
// container > entry, with entry content shown as a table in the preview.
// Used for backends (mongo) that don't share redis' write/tree semantics.
type DSTree struct {
	tree    *view.Tree
	preview *view.Preview
	src     datasource.Datasource

	// pathSep, when non-empty, makes entries nest into a folder tree split on
	// the separator (zookeeper: "/"), instead of a flat list (mongo/mysql: "").
	// In nested mode a container is itself a node with data, and every path
	// segment is a real, selectable entry.
	pathSep string

	// entryNoun / containerNoun label the drop-confirmation dialogs per backend
	// (collection/database for mongo, table/database for mysql, znode for zk).
	entryNoun     string
	containerNoun string

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

	t := &DSTree{tree: tree, preview: preview, src: src, entryNoun: "collection", containerNoun: "database"}
	tree.SetSelectedFunc(t.onSelected)
	tree.SetChangedFunc(t.onChanged)
	preview.SetTableCommitFunc(t.commitCells)
	preview.SetTableReloadFunc(t.reloadCurrentTable)
	preview.SetDeleteFunc(t.dropEntry)
	preview.SetReloadFunc(t.reloadCurrentTable)
	preview.SetQueryFunc(t.runQuery)
	preview.SetSaveFunc(t.saveText)
	tree.SetInputCapture(t.onTreeKey)
	return t
}

// saveText persists the edited multi-line value of the current text entry
// (zookeeper znode) via Update. oldValue is unused; the textarea holds the full
// new blob.
func (t *DSTree) saveText(_, newValue string) {
	if t.curEntry == "" {
		return
	}
	e := datasource.Edit{Value: newValue}
	if err := t.src.Update(t.curContainer, t.curEntry, e); err != nil {
		tlog.Log("[DSTree] save %v", err)
		if t.ShowModalOK != nil {
			t.ShowModalOK(fmt.Sprintf("Save failed: %v", err))
		}
		return
	}
	if t.ShowModalOK != nil {
		t.ShowModalOK("Value was updated!")
	}
}

// SetNested switches the tree to nested mode, where entries are split on sep
// into a folder tree (zookeeper: "/"). nounEntry/nounContainer label the
// drop-confirmation dialogs.
func (t *DSTree) SetNested(sep, nounEntry, nounContainer string) {
	t.pathSep = sep
	t.entryNoun = nounEntry
	t.containerNoun = nounContainer
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

// dropEntry asks for confirmation, then drops the selected entry. In nested
// mode the entry is a znode addressed by its full path.
func (t *DSTree) dropEntry() {
	node := t.tree.GetCurrentNode()
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil || ref.level != dsEntry {
		return
	}
	target := fmt.Sprintf("%v.%v", ref.container, ref.name)
	if t.pathSep != "" {
		target = ref.path
	}
	notice := fmt.Sprintf("Drop %s %v ?", t.entryNoun, target)
	t.ShowModal(notice, func() {
		entry := ref.name
		if t.pathSep != "" {
			entry = ref.path
		}
		if err := t.src.DropEntry(ref.container, entry); err != nil {
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

// dropContainer asks for confirmation, then drops the selected top-level node.
func (t *DSTree) dropContainer() {
	node := t.tree.GetCurrentNode()
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil || ref.level != dsContainer {
		return
	}
	notice := fmt.Sprintf("Drop %s %v ? This removes ALL %ss.", t.containerNoun, ref.name, t.entryNoun)
	if t.pathSep != "" {
		notice = fmt.Sprintf("Drop %s %v ?", t.containerNoun, ref.path)
	}
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

// removeNode detaches a node from the tree, searching the full subtree so a
// nested entry at any depth is removed from its actual parent.
func (t *DSTree) removeNode(node *tview.TreeNode) {
	var walk func(parent *tview.TreeNode) bool
	walk = func(parent *tview.TreeNode) bool {
		for _, child := range parent.GetChildren() {
			if child == node {
				parent.RemoveChild(node)
				return true
			}
			if walk(child) {
				return true
			}
		}
		return false
	}
	walk(t.tree.GetRoot())
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
		// Children already built; in nested mode keep the folder twistie in sync
		// with the expand state that view.Tree.SetSelectedFunc just toggled.
		if t.pathSep != "" && ref.folder {
			t.syncTwistie(node, ref)
		}
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
			label := name
			if t.pathSep != "" {
				// In nested mode a container is itself a znode; mark it a folder so
				// the user can expand it to reveal descendants (loaded lazily).
				label = "▶ " + name
			}
			t.tree.AddNode(label, &dsRef{level: dsContainer, name: name, path: t.pathSep + name, folder: t.pathSep != ""})
		}
	case dsContainer:
		entries, err := t.src.Entries(ref.name)
		if err != nil {
			tlog.Log("[DSTree] entries %v", err)
			return
		}
		if t.pathSep != "" {
			t.addNestedChildren(node, ref.path, entries)
			// The container just expanded to reveal its children; flip its twistie.
			t.syncTwistie(node, ref)
			return
		}
		for _, name := range entries {
			t.tree.AddNode(name, &dsRef{level: dsEntry, container: ref.name, name: name})
		}
	}
}

// addNestedChildren builds the full nested subtree under parentPath from the
// flat descendant-path list. Each immediate child becomes a node; folder
// children (those with descendants) get a ▶ twistie and are populated
// recursively, collapsed by default. Every node is a real znode, so all are
// selectable and show their own data. Config-store trees are small enough to
// materialize eagerly in one pass, mirroring the redis key tree.
func (t *DSTree) addNestedChildren(parent *tview.TreeNode, parentPath string, allPaths []string) {
	prefix := parentPath + t.pathSep
	order := make([]string, 0)
	deeper := make(map[string]bool)
	for _, p := range allPaths {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		if rest == "" {
			continue
		}
		seg, more, hasMore := strings.Cut(rest, t.pathSep)
		if _, ok := deeper[seg]; !ok {
			order = append(order, seg)
			deeper[seg] = false
		}
		if hasMore && more != "" {
			deeper[seg] = true
		}
	}
	container := topSegment(parentPath, t.pathSep)
	for _, seg := range order {
		childPath := prefix + seg
		folder := deeper[seg]
		ref := &dsRef{level: dsEntry, container: container, name: childPath, path: childPath, folder: folder}
		label := seg
		if folder {
			label = "▶ " + seg
		}
		child := t.tree.AddChildNode(parent, label, ref)
		if folder {
			t.addNestedChildren(child, childPath, allPaths)
			child.SetExpanded(false)
		}
	}
}

// syncTwistie updates a nested folder node's ▶/▼ prefix to match its current
// expand state (the bare label is the node's own path segment).
func (t *DSTree) syncTwistie(node *tview.TreeNode, ref *dsRef) {
	bare := ref.name
	if ref.level == dsContainer {
		bare = ref.name
	} else {
		bare = lastSegment(ref.path, t.pathSep)
	}
	if node.IsExpanded() {
		node.SetText("▼ " + bare)
	} else {
		node.SetText("▶ " + bare)
	}
}

// lastSegment returns the final path segment of a nested path like
// "/app/config" -> "config".
func lastSegment(path, sep string) string {
	if i := strings.LastIndex(path, sep); i >= 0 {
		return path[i+len(sep):]
	}
	return path
}

// topSegment returns the first path segment (the owning container) of a nested
// path like "/app/config" -> "app".
func topSegment(path, sep string) string {
	trimmed := strings.TrimPrefix(path, sep)
	if i := strings.Index(trimmed, sep); i >= 0 {
		return trimmed[:i]
	}
	return trimmed
}

func (t *DSTree) onChanged(node *tview.TreeNode) {
	ref, _ := node.GetReference().(*dsRef)
	// In nested mode only leaf znodes (no children) show their value; folder
	// nodes — containers and parent entries alike — just expand/collapse. In
	// flat mode every entry is a leaf and shows content.
	hasContent := ref != nil && ref.level == dsEntry && !(t.pathSep != "" && ref.folder)
	if !hasContent {
		t.preview.SetOpBtnVisible(false)
		t.preview.Clear()
		t.preview.ShowText("", false)
		return
	}
	// In nested mode the backend keys content off the full path; in flat mode
	// off the (container, entry) pair.
	container, entry := ref.container, ref.name
	if t.pathSep != "" {
		container, entry = ref.container, ref.path
	}
	t.preview.SetOpBtnVisible(true)
	c, err := t.src.Content(container, entry, datasource.Page{})
	if err != nil {
		t.preview.Clear()
		t.preview.ShowText(fmt.Sprintf("%v", err), false)
		return
	}
	t.curContainer = container
	t.curEntry = entry
	t.curQuery = ""
	t.cur = c
	t.showContent(c)
}

func (t *DSTree) showContent(c datasource.Content) {
	p := t.preview
	p.Clear()
	p.SetKeyType(c.Type)
	// Mirror the full backend statement (SELECT ... / db.coll.find(...)) into the
	// query box so the user can see and edit what ran.
	p.SetQueryText(c.Query)
	if c.Kind == datasource.KindText {
		// Nested-mode text (zookeeper znode data) is editable with a Save button;
		// other backends' text is read-only.
		p.ShowText(c.Text, t.pathSep != "")
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
		if t.pathSep != "" {
			// Nested mode: keep any path whose own segment matches, plus the
			// ancestor folders needed to reach it, so the tree stays navigable.
			kept := keepWithAncestors(entries, ref.path, t.pathSep, needle)
			t.addNestedChildren(cNode, ref.path, kept)
			continue
		}
		for _, name := range entries {
			if needle != "" && !strings.Contains(strings.ToLower(name), needle) {
				continue
			}
			t.tree.AddChildNode(cNode, name, &dsRef{level: dsEntry, container: ref.name, name: name})
		}
	}
}

// keepWithAncestors filters a flat descendant-path list to those paths whose
// final segment contains needle, plus every ancestor path up to (but excluding)
// rootPath, so addNestedChildren can still build a connected subtree. An empty
// needle returns the list unchanged.
func keepWithAncestors(paths []string, rootPath, sep, needle string) []string {
	if needle == "" {
		return paths
	}
	keep := make(map[string]bool)
	for _, p := range paths {
		if !strings.Contains(strings.ToLower(lastSegment(p, sep)), needle) {
			continue
		}
		// Walk ancestors from the matched path up to rootPath, marking each.
		for cur := p; cur != rootPath && strings.HasPrefix(cur, rootPath+sep); {
			keep[cur] = true
			i := strings.LastIndex(cur, sep)
			if i <= 0 {
				break
			}
			cur = cur[:i]
		}
	}
	out := make([]string, 0, len(keep))
	for _, p := range paths {
		if keep[p] {
			out = append(out, p)
		}
	}
	return out
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
