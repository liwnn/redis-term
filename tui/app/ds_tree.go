package app

import (
	"fmt"
	"io"
	"strings"
	"time"

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
	dsFolder // key-tree mode: an interior key prefix (not a real key)
)

// dsRef is attached to each DSTree node.
type dsRef struct {
	level     dsLevel
	container string    // owning container, for entry nodes
	name      string    // container or entry name
	path      string    // full backend path (nested mode); equals the znode path
	folder    bool      // nested mode: has children (rendered with a ▶/▼ twistie)
	node      *DataNode // key-tree mode: backing key-tree node (nil otherwise)
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

	// keySep, when non-empty, makes a container's entries build a redis key tree
	// split on the separator (redis: ":"). Unlike pathSep (zookeeper), interior
	// nodes are key prefixes that are NOT real entries (no content), and the
	// container (db) is not part of any entry's key. keyTrees holds one built tree
	// per container so reload/filter can rebuild from cached scans.
	keySep   string
	keyTrees map[string]*DataTree

	// Streaming key-tree load (redis, keySep mode). When a db is expanded its keys
	// are SCANned on a background goroutine and folded into the tree in batches via
	// queueUpdate, so a multi-million-key db doesn't freeze the UI. queueUpdate
	// runs a closure on the tview UI goroutine (a.main.QueueUpdateDraw); nil for
	// backends that don't stream.
	queueUpdate func(func())
	// loading marks containers whose background SCAN is in flight. While any entry
	// is true the connection is in exclusive use by that scan, so UI actions that
	// would touch the same connection (open a value, switch db, reload, drop) must
	// be refused — see IsLoading.
	loading map[string]bool
	// loadEpoch is bumped on every action that invalidates an in-flight scan
	// (re-expand, reload, container switch). A streaming callback captures the
	// epoch at launch and discards its batch if the epoch has since changed, so a
	// stale goroutine can never write into a rebuilt tree.
	loadEpoch int

	// searching tracks whether a key filter is currently active, and searchAnchor
	// remembers the entry selected when the filter began so clearing the search can
	// restore that selection instead of collapsing back to the bare db list.
	searching    bool
	searchAnchor *dsRef

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
	// Flat backends (mongo collections / mysql tables) support per-row deletion;
	// nested mode (zookeeper) turns this off in SetNested.
	preview.EnableTableRowSelection(true)
	preview.SetTableDeleteRowsFunc(t.deleteRows)
	preview.SetTableConfirmFunc(func(text string, okFunc func()) {
		if t.ShowModal != nil {
			t.ShowModal(text, okFunc)
		}
	})
	preview.SetDeleteFunc(t.dropCurrent)
	preview.SetReloadFunc(t.reloadCurrent)
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
	// Nested backends (zookeeper) show znodes as editable text, not deletable
	// rows, so turn off the per-row checkboxes enabled by NewDSTree.
	t.preview.EnableTableRowSelection(false)
}

// SetKeyTree switches the tree to redis key-tree mode, where a container's flat
// key list is split on sep (":") into a browsable prefix tree. Interior prefix
// nodes are not real keys (no content); only leaves are. Wires the op-bar Rename
// button, which only redis supports.
func (t *DSTree) SetKeyTree(sep string) {
	t.keySep = sep
	t.keyTrees = make(map[string]*DataTree)
	t.loading = make(map[string]bool)
	t.entryNoun = "key"
	t.containerNoun = "db"
	t.preview.SetRenameFunc(t.renameKey)
}

// SetQueueUpdate injects the closure that runs a function on the tview UI
// goroutine (a.main.QueueUpdateDraw). Required for streaming key-tree loads.
func (t *DSTree) SetQueueUpdate(fn func(func())) {
	t.queueUpdate = fn
}

// IsLoading reports whether any container's background SCAN is in flight. While
// true, the redis connection is in exclusive use and UI actions that would issue
// another command on it must be refused.
func (t *DSTree) IsLoading() bool {
	for _, v := range t.loading {
		if v {
			return true
		}
	}
	return false
}

// busyForConn reports whether a connection-touching action must be refused
// because a background SCAN holds the connection, surfacing a one-line hint so
// the refusal isn't silent. Callers should bail when it returns true.
func (t *DSTree) busyForConn() bool {
	if !t.IsLoading() {
		return false
	}
	if t.ShowModalOK != nil {
		t.ShowModalOK("Still loading keys — try again once the tree finishes.")
	}
	return true
}

// runQuery re-fetches the current entry using the user-typed filter and
// repaints the table. An empty query matches all documents.
func (t *DSTree) runQuery(query string) {
	if t.curEntry == "" || t.busyForConn() {
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
// 'd'/Delete, or reload the selected node's children with 'r'/F5.
func (t *DSTree) onTreeKey(event *tcell.EventKey) *tcell.EventKey {
	isDrop := event.Key() == tcell.KeyDelete || (event.Key() == tcell.KeyRune && event.Rune() == 'd')
	isReload := event.Key() == tcell.KeyF5 || (event.Key() == tcell.KeyRune && event.Rune() == 'r')
	if (isDrop || isReload) && t.busyForConn() {
		return nil // a background SCAN owns the connection; swallow the action
	}
	if isDrop {
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
		case dsFolder:
			t.dropKeyFolder()
			return nil
		}
	}
	if isReload {
		if t.reloadNode() {
			return nil
		}
	}
	return event
}

// reloadNode re-fetches and rebuilds the children of the selected non-leaf node
// (server, container, or nested folder), preserving its expand state. Returns
// true if it handled the node. Leaf entries are left to the preview's Reload
// button (they have no children to rebuild).
func (t *DSTree) reloadNode() bool {
	node := t.tree.GetCurrentNode()
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil {
		return false
	}
	switch ref.level {
	case dsServer:
		// Rebuild the whole tree: re-list containers from scratch.
		node.ClearChildren()
		t.onSelected(node)
		node.SetExpanded(true)
		return true
	case dsContainer:
		entries, err := t.src.Entries(ref.name)
		if err != nil {
			tlog.Log("[DSTree] reload entries %v", err)
			if t.ShowModalOK != nil {
				t.ShowModalOK(fmt.Sprintf("Reload failed: %v", err))
			}
			return true
		}
		node.ClearChildren()
		if t.keySep != "" {
			t.buildKeyTree(node, ref.name, entries)
		} else if t.pathSep != "" {
			t.addNestedChildren(node, ref.path, entries)
		} else {
			for _, name := range entries {
				t.tree.AddChildNode(node, name, &dsRef{level: dsEntry, container: ref.name, name: name})
			}
		}
		return true
	case dsFolder:
		// Reload a key prefix: redis SCAN can't cheaply list just one prefix, so
		// rescan the whole db to get fresh data — but rebuild ONLY this folder's
		// subtree in place, leaving the rest of the tree (and the selection) put
		// rather than collapsing back to the container root.
		entries, err := t.src.Entries(ref.container)
		if err != nil {
			tlog.Log("[DSTree] reload entries %v", err)
			if t.ShowModalOK != nil {
				t.ShowModalOK(fmt.Sprintf("Reload failed: %v", err))
			}
			return true
		}
		// Rebuild the cached key tree from the fresh scan, then locate the DataNode
		// for this exact prefix in it.
		dt := NewDataTree(ref.container)
		for _, k := range entries {
			dt.AddKey(k)
		}
		t.keyTrees[ref.container] = dt
		dn := findPrefixNode(dt, ref.name)
		if dn == nil {
			// Every key under this prefix was deleted since it was last shown: the
			// folder no longer exists. Rebuild the whole container so the now-stale
			// folder node disappears with the rest brought up to date.
			if cNode := t.containerNode(ref.container); cNode != nil {
				cNode.ClearChildren()
				for _, c := range dt.GetChildren(dt.Root()) {
					t.addKeyNode(cNode, ref.container, c)
				}
			}
			return true
		}
		ref.node = dn // re-point the ref at the rebuilt DataNode
		expanded := node.IsExpanded()
		node.ClearChildren()
		for _, c := range dn.GetChildren() {
			t.addKeyNode(node, ref.container, c)
		}
		node.SetExpanded(expanded)
		t.syncKeyFolder(node, ref)
		return true
	case dsEntry:
		// Only nested folder znodes have a rebuildable subtree; leaves don't.
		if t.pathSep == "" || !ref.folder {
			return false
		}
		entries, err := t.src.Entries(ref.container)
		if err != nil {
			tlog.Log("[DSTree] reload entries %v", err)
			if t.ShowModalOK != nil {
				t.ShowModalOK(fmt.Sprintf("Reload failed: %v", err))
			}
			return true
		}
		expanded := node.IsExpanded()
		node.ClearChildren()
		t.addNestedChildren(node, ref.path, entries)
		node.SetExpanded(expanded)
		return true
	}
	return false
}

// dropCurrent drops whatever node is selected: an entry or (in nested mode) a
// container. Wired to the op-bar Drop button, which is shown for both.
func (t *DSTree) dropCurrent() {
	if t.busyForConn() {
		return
	}
	ref, _ := t.tree.GetCurrentNode().GetReference().(*dsRef)
	if ref == nil {
		return
	}
	if ref.level == dsContainer {
		t.dropContainer()
		return
	}
	if ref.level == dsFolder {
		t.dropKeyFolder()
		return
	}
	t.dropEntry()
}

// reloadCurrent reloads whatever node is selected. For a content-bearing leaf it
// repaints the value/table; for a folder znode or container it rebuilds the
// subtree (same as the 'r'/F5 key). Wired to the op-bar Reload button.
func (t *DSTree) reloadCurrent() {
	if t.busyForConn() {
		return
	}
	ref, _ := t.tree.GetCurrentNode().GetReference().(*dsRef)
	if ref == nil {
		return
	}
	// A folder znode or container rebuilds its children; a plain leaf repaints.
	if ref.level == dsContainer || ref.level == dsFolder || (t.pathSep != "" && ref.level == dsEntry && ref.folder) {
		t.reloadNode()
		// Folder/container znodes also carry their own value: refresh the pane too.
		t.onChanged(t.tree.GetCurrentNode())
		return
	}
	t.reloadCurrentTable()
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
		if t.keySep != "" && ref.node != nil {
			ref.node.RemoveSelf()
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
	if t.keySep != "" {
		notice = fmt.Sprintf("FlushDB %v ?", ref.name)
	}
	t.ShowModal(notice, func() {
		if err := t.src.DropContainer(ref.name); err != nil {
			tlog.Log("[DSTree] drop db %v", err)
			if t.ShowModalOK != nil {
				t.ShowModalOK(fmt.Sprintf("Drop failed: %v", err))
			}
			return
		}
		if t.keySep != "" {
			// FlushDB empties the db but the db node itself remains (db0..dbN are
			// fixed); just drop its children and forget the cached key tree.
			node.ClearChildren()
			delete(t.keyTrees, ref.name)
		} else {
			t.tree.GetRoot().RemoveChild(node)
		}
		t.preview.Clear()
		t.preview.ShowText("", false)
		t.preview.SetOpBtnVisible(false)
		t.preview.SetKey("")
	})
}

// dropKeyFolder asks for confirmation, then deletes every real key under the
// selected prefix folder (redis has no native "drop prefix", so each leaf key is
// dropped individually).
func (t *DSTree) dropKeyFolder() {
	node := t.tree.GetCurrentNode()
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil || ref.level != dsFolder || ref.node == nil {
		return
	}
	notice := fmt.Sprintf("Delete %v* ?", ref.name)
	t.ShowModal(notice, func() {
		keys := LeafKeys(ref.node)
		for _, k := range keys {
			if err := t.src.DropEntry(ref.container, k); err != nil {
				tlog.Log("[DSTree] drop %v", err)
				if t.ShowModalOK != nil {
					t.ShowModalOK(fmt.Sprintf("Drop failed: %v", err))
				}
				return
			}
		}
		ref.node.RemoveSelf()
		t.removeNode(node)
		t.preview.Clear()
		t.preview.ShowText("", false)
		t.preview.SetOpBtnVisible(false)
		t.preview.SetKey("")
	})
}

// containerNode returns the top-level container node with the given name.
// restoreAnchor re-selects the entry that was current when a key search began,
// after the search is cleared and the tree rebuilt to the bare db list. It
// expands the entry's db (from cache, no re-SCAN), walks down the prefix folders
// that lead to it (building each lazily), then selects the entry's node.
func (t *DSTree) restoreAnchor() {
	ref := t.searchAnchor
	t.searchAnchor = nil
	if ref == nil || ref.node == nil || ref.container == "" {
		return
	}
	// Ancestor chain root→…→anchor, by DataNode identity.
	var chain []*DataNode
	for n := ref.node; n != nil && n.Key() != ""; n = n.p {
		chain = append([]*DataNode{n}, chain...)
	}
	cNode := t.containerNode(ref.container)
	if cNode == nil {
		return
	}
	t.tree.SetCurrentNode(cNode)
	t.onSelected(cNode) // expand the db from cache
	cNode.SetExpanded(true)

	parent := cNode
	for i, dn := range chain {
		var found *tview.TreeNode
		for _, child := range parent.GetChildren() {
			cref, _ := child.GetReference().(*dsRef)
			if cref != nil && cref.node == dn {
				found = child
				break
			}
		}
		if found == nil {
			return // path diverged (key changed since search began); stop gracefully
		}
		if i == len(chain)-1 {
			t.tree.SetCurrentNode(found) // the anchor entry/folder itself
			return
		}
		// Interior prefix folder: expand it (lazily builds children) and descend.
		found.SetExpanded(true)
		t.onSelected(found)
		parent = found
	}
}

func (t *DSTree) containerNode(name string) *tview.TreeNode {
	for _, child := range t.tree.GetRoot().GetChildren() {
		ref, _ := child.GetReference().(*dsRef)
		if ref != nil && ref.level == dsContainer && ref.name == name {
			return child
		}
	}
	return nil
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
	if t.IsLoading() {
		return fmt.Errorf("still loading keys; try again once the tree finishes")
	}
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

// deleteRows removes the selected document/table rows by their original cell
// values, then reloads the table. absRows index into the current content.
func (t *DSTree) deleteRows(absRows []int) error {
	if t.IsLoading() {
		return fmt.Errorf("still loading keys; try again once the tree finishes")
	}
	refs := make([]datasource.RowRef, 0, len(absRows))
	for _, r := range absRows {
		if r < 0 || r >= len(t.cur.Rows) {
			continue
		}
		var types []string
		if t.cur.CellTypes != nil && r < len(t.cur.CellTypes) {
			types = t.cur.CellTypes[r]
		}
		refs = append(refs, datasource.RowRef{Row: t.cur.Rows[r], Types: types})
	}
	if len(refs) == 0 {
		return nil
	}
	return t.src.DeleteRows(t.curContainer, t.curEntry, t.cur.Columns, refs)
}

// reloadCurrentTable re-fetches the current entry and repaints the table,
// preserving the active query filter.
func (t *DSTree) reloadCurrentTable() {
	if t.busyForConn() {
		return
	}
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
	// A redis db whose keys are still streaming in has placeholder/partial
	// children but is not finished: don't treat it as "already built" (that would
	// short-circuit re-expand) and don't kick off a second SCAN (one is running).
	if t.keySep != "" && ref.level == dsContainer && t.loading[ref.name] {
		return
	}
	if len(node.GetChildren()) > 0 {
		// Children already built; keep the folder twistie in sync with the expand
		// state that view.Tree.SetSelectedFunc just toggled.
		if t.pathSep != "" && ref.folder {
			t.syncTwistie(node, ref)
		}
		if t.keySep != "" && ref.level == dsFolder {
			t.syncKeyFolder(node, ref)
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
		if t.keySep != "" {
			// redis: stream the keyspace in on a background goroutine so a
			// multi-million-key db doesn't freeze the UI on expand.
			t.streamKeyTree(node, ref.name)
			return
		}
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
	case dsFolder:
		// Key-tree interior prefix node: build its immediate children on first
		// expand (lazy), then keep the twistie in sync.
		if ref.node != nil {
			for _, c := range ref.node.GetChildren() {
				t.addKeyNode(node, ref.container, c)
			}
		}
		t.syncKeyFolder(node, ref)
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

// findPrefixNode locates the DataNode for an interior prefix key (e.g. "user:1:")
// in a freshly built DataTree by walking the colon-delimited path from the root,
// matching the cumulative prefix at each level the same way AddKey builds it.
// Returns nil if the prefix no longer exists (all its keys were deleted).
func findPrefixNode(dt *DataTree, prefixKey string) *DataNode {
	p := dt.Root()
	for i := 0; i < len(prefixKey); i++ {
		if prefixKey[i] != ':' {
			continue
		}
		p = p.GetChildByKey(prefixKey[:i+1])
		if p == nil {
			return nil
		}
	}
	return p
}

// buildKeyTree splits a container's flat key list on keySep into a prefix tree
// and materializes it under the container node. The built tree is cached per
// container so reload/filter can rebuild from it.
func (t *DSTree) buildKeyTree(parent *tview.TreeNode, container string, keys []string) {
	dt := NewDataTree(container)
	for _, k := range keys {
		dt.AddKey(k)
	}
	t.keyTrees[container] = dt
	for _, dn := range dt.GetChildren(dt.Root()) {
		t.addKeyNode(parent, container, dn)
	}
}

// entriesStreamer is the optional streaming-SCAN capability a source may expose
// (redis does). Taken via type assertion so the Datasource interface stays
// unchanged and non-streaming backends are unaffected.
type entriesStreamer interface {
	EntriesStream(container string, fn func(batch []string) bool) error
}

// streamKeyTree loads a redis db's keyspace incrementally: a background
// goroutine SCANs (network I/O only) and hands each accumulated chunk to the UI
// goroutine, which folds it into the key tree and renders newly-revealed
// top-level nodes. The first chunk lands within milliseconds, so expanding a
// multi-million-key db no longer freezes the UI.
//
// All DataTree and tview mutations happen on the UI goroutine (inside the
// queueUpdate closure); the background goroutine never touches them, so there is
// no data race on the tree even though SCAN runs concurrently. While the scan is
// in flight loading[container] is set, which gates connection-touching UI
// actions (see IsLoading) — the client is a single unsynchronized connection.
func (t *DSTree) streamKeyTree(node *tview.TreeNode, container string) {
	// Only one background SCAN at a time: the connection is single and
	// unsynchronized, so expanding a second db while one is loading would
	// interleave SCANs. Refuse with a hint; the user can retry once it finishes.
	if t.IsLoading() {
		if t.ShowModalOK != nil {
			t.ShowModalOK("Still loading another db — try again once it finishes.")
		}
		return
	}
	// Re-render from the cached tree if this db was already fully scanned (e.g. its
	// tview nodes were discarded when a search rebuilt the root). No re-SCAN.
	if dt := t.keyTrees[container]; dt != nil && !t.loading[container] {
		for _, dn := range dt.GetChildren(dt.Root()) {
			t.addKeyNode(node, container, dn)
		}
		return
	}
	streamer, ok := t.src.(entriesStreamer)
	if !ok || t.queueUpdate == nil {
		// No streaming capability (or no UI-goroutine hook): fall back to the
		// synchronous build so behavior is still correct, just blocking.
		entries, err := t.src.Entries(container)
		if err != nil {
			tlog.Log("[DSTree] entries %v", err)
			return
		}
		t.buildKeyTree(node, container, entries)
		return
	}

	t.loadEpoch++
	epoch := t.loadEpoch
	t.loading[container] = true
	dt := NewDataTree(container)
	t.keyTrees[container] = dt
	renderedTop := 0

	// Show "(loading…)" only if the scan is still running after a short delay, so a
	// small db that loads instantly doesn't flash the hint on and off.
	time.AfterFunc(150*time.Millisecond, func() {
		t.queueUpdate(func() {
			if epoch == t.loadEpoch && t.loading[container] {
				node.SetText(container + " (loading…)")
			}
		})
	})

	// apply runs on the UI goroutine: fold one chunk (a slice of SCAN batches) into
	// the tree and render. Batches are kept as a [][]string rather than flattened,
	// so accumulating them costs no per-key copy.
	apply := func(chunk [][]string, done bool) {
		if epoch != t.loadEpoch {
			return // a newer scan superseded this one; drop the stale chunk
		}
		for _, batch := range chunk {
			for _, k := range batch {
				dt.AddKey(k)
			}
		}
		top := dt.GetChildren(dt.Root())
		for i := renderedTop; i < len(top); i++ {
			t.addKeyNode(node, container, top[i])
		}
		renderedTop = len(top)
		// Refresh counts on already-rendered folders as their keyNum grows.
		for _, child := range node.GetChildren() {
			cref, _ := child.GetReference().(*dsRef)
			if cref == nil || cref.level != dsFolder || cref.node == nil {
				continue
			}
			// A lazy folder defaults to expanded=true but has no children built yet,
			// so tview draws no twistie; only show ▼ once it actually holds children.
			glyph := "▶"
			if len(child.GetChildren()) > 0 && child.IsExpanded() {
				glyph = "▼"
			}
			child.SetText(fmt.Sprintf("%v %v (%v)", glyph, cref.node.Name(), cref.node.KeyNum()))
		}
		if done {
			node.SetText(container)
			t.loading[container] = false
		}
	}

	// Decouple SCAN from render: the scanner hands chunks to a buffered channel
	// and never waits, while a consumer goroutine drains it and renders serially
	// via queueUpdate. queueUpdate (tview's QueueUpdateDraw) blocks until the UI
	// goroutine has run the closure and redrawn — if the scanner did that inline,
	// each flush would stall SCAN for a full build+draw, inflating scan wall-clock.
	// Here only the consumer blocks on render; the scanner runs at network speed.
	go func() {
		const flushEvery = 100000
		chunks := make(chan [][]string, 32) // ~40 chunks for a 2M-key db; never fills

		go func() {
			for chunk := range chunks {
				c := chunk
				t.queueUpdate(func() { apply(c, false) })
			}
			t.queueUpdate(func() { apply(nil, true) }) // clear loading after the last chunk
		}()

		var pending [][]string
		var pendingKeys int
		err := streamer.EntriesStream(container, func(batch []string) bool {
			pending = append(pending, batch) // keep the batch by reference; no per-key copy
			pendingKeys += len(batch)
			// Throttle hand-offs to keep redraws bounded; the trailing flush below
			// covers whatever is left after the scan ends.
			if pendingKeys >= flushEvery {
				chunks <- pending
				pending = nil
				pendingKeys = 0
			}
			return true
		})
		if err != nil {
			tlog.Log("[DSTree] stream entries %v", err)
		}
		if len(pending) > 0 {
			chunks <- pending
		}
		close(chunks) // consumer drains remaining chunks, then finalizes
	}()
}

// addKeyNode renders one key-tree node under parent: an interior prefix becomes
// a collapsible folder labeled "▶ name (count)"; a leaf becomes a real key entry.
// Folder children are loaded lazily (built by onSelected when the folder is first
// expanded), so a huge keyspace doesn't materialize every node up front.
func (t *DSTree) addKeyNode(parent *tview.TreeNode, container string, dn *DataNode) {
	if dn.HasChild() {
		label := fmt.Sprintf("▶ %v (%v)", dn.Name(), dn.KeyNum())
		ref := &dsRef{level: dsFolder, container: container, name: dn.Key(), node: dn}
		t.tree.AddChildNode(parent, label, ref)
		return
	}
	ref := &dsRef{level: dsEntry, container: container, name: dn.Key(), node: dn}
	t.tree.AddChildNode(parent, dn.Name(), ref)
}

// addKeyNodeTree renders dn and, for a prefix folder, all its descendants
// eagerly with the folder expanded. Used for search results, where the matched
// subset is small and must be shown in full (lazy folders would render a ▼ with
// no visible children).
func (t *DSTree) addKeyNodeTree(parent *tview.TreeNode, container string, dn *DataNode) {
	if !dn.HasChild() {
		ref := &dsRef{level: dsEntry, container: container, name: dn.Key(), node: dn}
		t.tree.AddChildNode(parent, dn.Name(), ref)
		return
	}
	label := fmt.Sprintf("▼ %v (%v)", dn.Name(), dn.KeyNum())
	ref := &dsRef{level: dsFolder, container: container, name: dn.Key(), node: dn}
	folder := t.tree.AddChildNode(parent, label, ref)
	for _, c := range dn.GetChildren() {
		t.addKeyNodeTree(folder, container, c)
	}
	folder.SetExpanded(true)
}

// syncKeyFolder updates a key-tree folder's ▶/▼ prefix to match its expand state.
func (t *DSTree) syncKeyFolder(node *tview.TreeNode, ref *dsRef) {
	name := ref.name
	if ref.node != nil {
		name = ref.node.Name()
	}
	if node.IsExpanded() {
		node.SetText(fmt.Sprintf("▼ %v (%v)", name, folderKeyNum(ref)))
	} else {
		node.SetText(fmt.Sprintf("▶ %v (%v)", name, folderKeyNum(ref)))
	}
}

func folderKeyNum(ref *dsRef) int {
	if ref.node != nil {
		return ref.node.KeyNum()
	}
	return 0
}

// renameKey renames the selected key to the value typed in the op-bar key input.
func (t *DSTree) renameKey() {
	if t.busyForConn() {
		return
	}
	node := t.tree.GetCurrentNode()
	ref, _ := node.GetReference().(*dsRef)
	if ref == nil || ref.level != dsEntry {
		return
	}
	newKey := t.preview.GetKey()
	if newKey == "" || newKey == ref.name {
		return
	}
	if err := t.src.Rename(ref.container, ref.name, newKey); err != nil {
		tlog.Log("[DSTree] rename %v", err)
		if t.ShowModalOK != nil {
			t.ShowModalOK(fmt.Sprintf("Rename failed: %v", err))
		}
		return
	}
	ref.name = newKey
	node.SetText(lastSegment(newKey, t.keySep))
	t.curEntry = newKey
	if t.ShowModalOK != nil {
		t.ShowModalOK("Key was renamed!")
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

// onChangedKeyTree drives the preview for redis key-tree selections: a db
// (container) shows the op bar with a Flush button; an interior prefix folder
// shows the op bar with a Delete button (delete all keys under it); both have no
// single value. A real key loads its content and enables the key input + Rename.
func (t *DSTree) onChangedKeyTree(ref *dsRef) {
	if ref == nil || ref.level == dsServer {
		t.preview.Clear()
		t.preview.ShowText("", false)
		t.preview.SetOpBtnVisible(false)
		t.preview.SetKey("")
		return
	}
	if ref.level != dsEntry {
		// db or prefix folder: droppable, but no single value.
		t.preview.Clear()
		t.preview.ShowText("", false)
		t.preview.SetOpBtnVisible(true)
		if ref.level == dsContainer {
			t.preview.SetDeleteText("Flush")
		} else {
			t.preview.SetDeleteText("Delete")
		}
		t.preview.SetKey("")
		return
	}
	t.preview.SetOpBtnVisible(true)
	t.preview.SetDeleteText("Delete")
	if t.IsLoading() {
		// A background SCAN owns the (single, unsynchronized) connection; reading a
		// value now would interleave commands on it. Skip until the load finishes.
		t.preview.Clear()
		t.preview.ShowText("loading keys…", false)
		t.preview.SetKey(ref.name)
		return
	}
	c, err := t.src.Content(ref.container, ref.name, datasource.Page{})
	if err != nil {
		t.preview.Clear()
		t.preview.ShowText(fmt.Sprintf("%v", err), false)
		t.preview.SetKey(ref.name)
		return
	}
	t.curContainer = ref.container
	t.curEntry = ref.name
	t.curQuery = ""
	t.cur = c
	t.showContent(c)
	t.preview.SetKey(ref.name)
}

func (t *DSTree) onChanged(node *tview.TreeNode) {
	ref, _ := node.GetReference().(*dsRef)
	if t.keySep != "" {
		t.onChangedKeyTree(ref)
		return
	}
	// Every entry shows content. In nested mode (zookeeper) both folder znodes and
	// top-level containers are real nodes with their own data, so they show their
	// value and op bar (Reload/Drop) too — selecting loads content; Enter still
	// expands/collapses. In flat mode only entries (leaves) show content.
	hasContent := ref != nil && (ref.level == dsEntry || (t.pathSep != "" && ref.level == dsContainer))
	if !hasContent {
		// A flat-mode container (mongo database / mysql) has no single value, but it
		// can still be dropped, so show the op bar (Reload/Drop) over an empty pane.
		t.preview.Clear()
		t.preview.ShowText("", false)
		t.preview.SetOpBtnVisible(ref != nil && ref.level == dsContainer)
		return
	}
	// In nested mode the backend keys content off the full path; in flat mode
	// off the (container, entry) pair.
	container, entry := ref.container, ref.name
	if t.pathSep != "" {
		// A container's own path is "/name"; an entry already carries its full path.
		if ref.level == dsContainer {
			container, entry = ref.name, ref.path
		} else {
			container, entry = ref.container, ref.path
		}
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
		p.SetSizeText(fmt.Sprintf("%d bytes", len(c.Text)))
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

// Pollable reports whether the backend supports background health polling.
func (t *DSTree) Pollable() bool {
	_, ok := t.src.(datasource.Pinger)
	return ok
}

// Ping checks the connection is still alive, if the backend supports it. A nil
// return means healthy (or the backend can't be polled).
func (t *DSTree) Ping() error {
	if p, ok := t.src.(datasource.Pinger); ok {
		return p.Ping()
	}
	return nil
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

// Filter rebuilds the container list, keeping only containers that match the
// term (case-insensitive) or hold a matching entry; non-matching containers are
// hidden entirely. Matching containers are expanded so their hits are visible.
// An empty term restores the full tree in its default collapsed state.
// filterKeyTrees filters the redis key view against keys already loaded into the
// per-db DataTree caches, instead of re-SCANning every db (which on a
// multi-million-key db would re-incur the full scan and collide with any
// in-flight streaming load). Only dbs the user has expanded (hence cached) are
// searched; unexpanded dbs are skipped. A db still streaming is matched against
// the keys loaded so far.
func (t *DSTree) filterKeyTrees(root *tview.TreeNode, needle string) {
	// Cap how many matches are materialized: results are rendered eagerly and fully
	// expanded, so a broad term matching hundreds of thousands of keys would build
	// that many tview nodes and make every redraw O(matches) — the UI would lock
	// up. Beyond the cap we render the first maxMatches and label the db with the
	// true total so the user knows to narrow the search.
	const maxMatches = 1000
	root.ClearChildren()
	for name, dt := range t.keyTrees {
		if dt == nil {
			continue
		}
		var matched []string
		total := 0
		for _, k := range LeafKeys(dt.Root()) {
			if !strings.Contains(strings.ToLower(k), needle) {
				continue
			}
			total++
			if len(matched) < maxMatches {
				matched = append(matched, k)
			}
		}
		nameMatch := strings.Contains(strings.ToLower(name), needle)
		if total == 0 && !nameMatch {
			continue
		}
		label := name
		if total > len(matched) {
			label = fmt.Sprintf("%v (showing %v of %v — narrow the search)", name, len(matched), total)
		}
		cNode := t.tree.AddChildNode(root, label, &dsRef{level: dsContainer, name: name})
		// Render a throwaway prefix tree of just the matches; do NOT route through
		// buildKeyTree, which would overwrite the cached full tree with this subset.
		sub := NewDataTree(name)
		for _, k := range matched {
			sub.AddKey(k)
		}
		for _, dn := range sub.GetChildren(sub.Root()) {
			t.addKeyNodeTree(cNode, name, dn)
		}
		cNode.SetExpanded(true)
	}
}

func (t *DSTree) Filter(term string) {
	needle := strings.ToLower(term)
	root := t.tree.GetRoot()
	// Remember the selection when a key search begins, so clearing it can restore
	// that entry rather than collapsing to the bare db list.
	if t.keySep != "" && needle != "" && !t.searching {
		t.searching = true
		t.searchAnchor, _ = t.tree.GetCurrentNode().GetReference().(*dsRef)
	}
	if needle == "" {
		// Restore the default view: re-list all containers, collapsed.
		root.ClearChildren()
		t.onSelected(root)
		if t.keySep != "" && t.searching {
			t.searching = false
			t.restoreAnchor()
		}
		return
	}
	if t.keySep != "" {
		t.filterKeyTrees(root, needle)
		return
	}
	containers, err := t.src.Containers()
	if err != nil {
		return
	}
	root.ClearChildren()
	for _, name := range containers {
		entries, err := t.src.Entries(name)
		if err != nil {
			continue
		}
		nameMatch := strings.Contains(strings.ToLower(name), needle)
		path := t.pathSep + name
		if t.pathSep != "" {
			// Nested mode: keep any path whose own segment matches, plus the
			// ancestor folders needed to reach it. Hide the container if nothing
			// (its own name included) matches.
			kept := keepWithAncestors(entries, path, t.pathSep, needle)
			if len(kept) == 0 && !nameMatch {
				continue
			}
			cNode := t.tree.AddChildNode(root, "▼ "+name, &dsRef{level: dsContainer, name: name, path: path, folder: true})
			t.addNestedChildren(cNode, path, kept)
			t.expandFolders(cNode) // reveal matches nested below intermediate folders
			cNode.SetExpanded(true)
			continue
		}
		// Flat mode: keep matching entry names; hide the container if it has none
		// (and its own name doesn't match).
		var matched []string
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e), needle) {
				matched = append(matched, e)
			}
		}
		if len(matched) == 0 && !nameMatch {
			continue
		}
		cNode := t.tree.AddChildNode(root, name, &dsRef{level: dsContainer, name: name, path: path})
		for _, e := range matched {
			t.tree.AddChildNode(cNode, e, &dsRef{level: dsEntry, container: name, name: e})
		}
		cNode.SetExpanded(true)
	}
}

// expandFolders expands every nested folder node under parent (and updates its
// twistie) so search hits buried below collapsed intermediate folders are shown.
func (t *DSTree) expandFolders(parent *tview.TreeNode) {
	for _, child := range parent.GetChildren() {
		ref, _ := child.GetReference().(*dsRef)
		if ref == nil {
			continue
		}
		if t.keySep != "" && ref.level == dsFolder {
			child.SetExpanded(true)
			t.syncKeyFolder(child, ref)
			t.expandFolders(child)
			continue
		}
		if ref.folder {
			child.SetExpanded(true)
			t.syncTwistie(child, ref)
			t.expandFolders(child)
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
