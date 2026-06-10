package app

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/liwnn/redisterm/datasource/redisapi"
	"github.com/liwnn/redisterm/tlog"
	"github.com/liwnn/redisterm/tui/model"
	"github.com/liwnn/redisterm/tui/view"

	"github.com/rivo/tview"
)

var _ treeController = (*DBTree)(nil)

// Reference referenct
type Reference struct {
	Name  string
	Index int
	Data  *model.DataNode
}

// DBTree tree.
type DBTree struct {
	tree    *view.Tree
	preview *view.Preview

	data *model.Data

	// current table state, captured for inline cell editing
	curKey     string
	curKeyType string
	curRows    [][]string

	// lastChanged dedupes tview's mouse "changed" callback: one left click fires
	// changed twice (once from MouseLeftClick, once from the next Draw's
	// process()), so without this guard every click reloads the key twice.
	lastChanged *tview.TreeNode

	// opened tracks db indices whose keys the user has loaded at least once, so
	// Filter knows which dbs to rescan. This can't be inferred from child count:
	// a db emptied by a no-match search still needs rescanning when the term is
	// shortened, but an unopened db must not be eagerly loaded.
	opened map[int]bool

	// rootClick, when set, is consulted when the root (server) node is selected;
	// returning true means it was handled (e.g. a reconnect was kicked off) and
	// normal expansion is skipped.
	rootClick func() bool

	ShowModalOK func(string)
	ShowModal   func(text string, okFunc func())
}

// NewDBTree new
func NewDBTree(tree *view.Tree, preview *view.Preview) *DBTree {
	dbTree := &DBTree{
		tree:    tree,
		preview: preview,
		opened:  make(map[int]bool),
	}
	tree.SetSelectedFunc(dbTree.OnSelected)
	tree.SetChangedFunc(dbTree.OnChanged)
	preview.SetSaveFunc(dbTree.saveKey)
	preview.SetReloadFunc(dbTree.reloadSelectKey)
	preview.SetRenameFunc(dbTree.renameSelectKey)
	preview.SetDeleteFunc(dbTree.deleteKey)
	preview.SetTableCommitFunc(dbTree.commitCells)
	preview.SetTableReloadFunc(dbTree.reloadCurrentTable)
	return dbTree
}

// commitCells persists all staged table cell edits for the current key.
func (t *DBTree) commitCells(edits []view.CellEdit) error {
	for _, e := range edits {
		if e.Row < 0 || e.Row >= len(t.curRows) {
			continue
		}
		oldRow := t.curRows[e.Row]
		if err := t.data.UpdateCell(t.curKey, t.curKeyType, oldRow, e.Row, e.Col, e.Value); err != nil {
			return err
		}
		if e.Col >= 0 && e.Col < len(oldRow) {
			oldRow[e.Col] = e.Value
		}
	}
	return nil
}

// reloadCurrentTable re-fetches the current key and repaints the table.
func (t *DBTree) reloadCurrentTable() {
	o, keyType := t.data.GetValue(t.curKey)
	t.updatePreviewWithType(o, keyType, true)
}

// SetData change db data.
func (t *DBTree) SetData(name string, data *model.Data) {
	t.tree.GetRoot().ClearChildren()
	t.tree.GetRoot().SetText(name)
	t.data = data
}

func (t *DBTree) changeDB(index int) error {
	return t.data.Select(index)
}

// OnSelected on select
func (t *DBTree) OnSelected(node *tview.TreeNode) {
	typ := t.getReference(node)
	// Clicking the server (root) node may trigger a reconnect when the previous
	// connect failed; if handled, skip normal expansion.
	if typ.Name == "db" && t.rootClick != nil && t.rootClick() {
		return
	}
	// Not connected yet (or connect failed): bail quietly. Connecting is the
	// background goroutine's job — never dial inline, or a click on an
	// unreachable connection freezes the UI thread for the dial timeout.
	if err := t.changeDB(typ.Index); err != nil {
		tlog.Log("[OnSelected] %v", err)
		return
	}
	if typ.Data != nil && typ.Data.HasChild() {
		node := t.tree.GetCurrentNode()
		if node.IsExpanded() {
			node.SetText(fmt.Sprintf("▼ %v (%v)", typ.Data.Name(), typ.Data.KeyNum()))
		} else {
			node.SetText(fmt.Sprintf("▶ %v (%v)", typ.Data.Name(), typ.Data.KeyNum()))
		}
	}
	childen := node.GetChildren()
	if len(childen) == 0 {
		var dataNodes []*model.DataNode
		switch typ.Name {
		case "db":
			dbs, err := t.data.GetDatabases()
			if err != nil {
				tlog.Log("[OnSelected] db %v", err)
				return
			}
			for i, dataNode := range dbs {
				t.tree.AddNode(dataNode.Name(), &Reference{
					Name:  "index",
					Index: i,
					Data:  dataNode,
				})
			}
			t.addNode(node, dataNodes)
		case "index":
			var err error
			dataNodes, err = t.data.ScanAllKeys()
			if err != nil {
				tlog.Log("[OnSelected] index %v", err)
				return
			}
			t.opened[typ.Index] = true
			t.addNode(node, dataNodes)
		case "dir":
			dataNodes = t.data.GetChildren(typ.Data)
			t.addNode(node, dataNodes)
		}
	}
}

func (t *DBTree) addNode(node *tview.TreeNode, dataNodes []*model.DataNode) {
	typ := t.getReference(node)
	for _, dataNode := range dataNodes {
		r := &Reference{
			Index: typ.Index,
			Data:  dataNode,
		}
		t.addReference(dataNode, r)
	}
}

// OnChanged on change
func (t *DBTree) OnChanged(node *tview.TreeNode) {
	if node == t.lastChanged {
		return // tview fires changed twice per mouse click; ignore the repeat
	}
	t.lastChanged = node

	typ := t.getReference(node)
	if typ.Name == "db" {
		t.preview.SetOpBtnVisible(false)
	} else {
		if typ.Name == "index" {
			tlog.Log("OnChanged: %v - %v", typ.Name, typ.Index)
		} else {
			tlog.Log("OnChanged: %v - %v", typ.Name, typ.Data.Key())
		}
		t.preview.SetOpBtnVisible(true)
	}

	if typ.Name == "key" {
		if !typ.Data.IsRemoved() {
			t.changeDB(typ.Index)
			begin := time.Now()
			t.curKey = typ.Data.Key()
			o, keyType := t.data.GetValue(typ.Data.Key())
			tlog.Log("redis value time cost %v", time.Since(begin))
			t.updatePreviewWithType(o, keyType, true)
		} else {
			t.updatePreviewWithType(fmt.Sprintf("%v was removed", typ.Data.Key()), "", false)
		}
		t.preview.SetDeleteText("Delete")
		t.preview.SetKey(typ.Data.Key())
	} else {
		if typ.Name == "index" {
			t.preview.SetDeleteText("Flush")
		} else {
			t.preview.SetDeleteText("Delete")
		}
		t.updatePreviewWithType("", "", false)
		t.preview.SetKey("")
	}
}

func (t *DBTree) updatePreviewWithType(o interface{}, keyType string, valid bool) {
	p := t.preview
	p.Clear()
	p.SetKeyType(keyType)
	t.curKeyType = keyType
	t.curRows = nil
	switch h := o.(type) {
	case []byte:
		b := o.([]byte)
		var text string
		if model.IsText(b) {
			text = string(b)
		} else {
			text = model.EncodeToHexString(b)
		}
		p.ShowText(text, valid)
		if valid {
			p.SetSizeText(fmt.Sprintf("Size: %d bytes", len(b)))
		}

	case []string:
		title := []view.TablePageTitle{
			{
				Name:      "row",
				Expansion: 1,
			},
			{
				Name:      "value",
				Expansion: 20,
			},
		}

		rows := make([]view.Row, 0, len(h))
		cur := make([][]string, 0, len(h))
		for _, v := range h {
			rows = append(rows, view.Row{v})
			cur = append(cur, []string{v})
		}
		t.curRows = cur
		p.ShowTable(title, rows, nil)
	case []redisapi.KVText:
		title := []view.TablePageTitle{
			{
				Name:      "row",
				Expansion: 1,
			},
			{
				Name:      "key",
				Expansion: 3,
			},
			{
				Name:      "value",
				Expansion: 24,
			},
		}
		rows := make([]view.Row, 0, len(h))
		cur := make([][]string, 0, len(h))
		for _, v := range h {
			rows = append(rows, view.Row{v.Key, v.Value})
			cur = append(cur, []string{v.Key, v.Value})
		}
		t.curRows = cur
		p.ShowTable(title, rows, nil)
	case []redisapi.ZSetText:
		title := []view.TablePageTitle{
			{
				Name:      "row",
				Expansion: 1,
			},
			{
				Name:      "member",
				Expansion: 3,
			},
			{
				Name:      "score",
				Expansion: 24,
			},
		}
		rows := make([]view.Row, 0, len(h))
		cur := make([][]string, 0, len(h))
		for _, v := range h {
			rows = append(rows, view.Row{v.Key, v.Value})
			cur = append(cur, []string{v.Key, v.Value})
		}
		t.curRows = cur
		p.ShowTable(title, rows, nil)
	case []redisapi.StreamEntry:
		// Columns: row, id, then the union of all field names in first-seen order,
		// so heterogeneous entries still line up by field (matches the web view).
		fieldCols := make([]string, 0)
		colIdx := make(map[string]int)
		for _, e := range h {
			for _, kv := range e.Fields {
				if _, ok := colIdx[kv.Key]; !ok {
					colIdx[kv.Key] = len(fieldCols)
					fieldCols = append(fieldCols, kv.Key)
				}
			}
		}
		title := []view.TablePageTitle{
			{Name: "row", Expansion: 1},
			{Name: "id", Expansion: 4},
		}
		for _, name := range fieldCols {
			title = append(title, view.TablePageTitle{Name: name, Expansion: 6})
		}
		rows := make([]view.Row, 0, len(h))
		cur := make([][]string, 0, len(h))
		for _, e := range h {
			row := make([]string, 1+len(fieldCols))
			row[0] = e.ID
			for _, kv := range e.Fields {
				row[colIdx[kv.Key]+1] = kv.Value
			}
			rows = append(rows, view.Row(row))
			cur = append(cur, row)
		}
		t.curRows = cur
		p.ShowTable(title, rows, nil)
	}
}

func (t *DBTree) reloadSelectKey() {
	node := t.getCurrentNode()

	reference := t.getReference(node)
	if reference == nil {
		return
	}
	key := reference.Data.Key()
	tlog.Log("reload %v", key)

	if reference.Name == "key" {
		t.changeDB(reference.Index)
		o, kt := t.data.GetValue(key)
		if o == nil {
			reference.Data.SetRemoved()
			t.tree.SetNodeRemoved()
			t.updatePreviewWithType(fmt.Sprintf("%v was removed", key), "", false)
			t.preview.SetDeleteText("Delete")
		} else {
			t.updatePreviewWithType(o, kt, true)
		}
		return
	}

	node.ClearChildren()
	if err := t.data.Reload(reference.Data); err != nil {
		tlog.Log("[App] err %v", err)
		node.SetExpanded(false)
		node.SetText(reference.Data.Name())
	}

	childen := reference.Data.GetChildren()
	for _, dataNode := range childen {
		r := &Reference{
			Index: reference.Index,
			Data:  dataNode,
		}
		t.addReference(dataNode, r)
	}

	if reference.Data.IsRemoved() {
		t.tree.SetNodeRemoved()
	}
}

func (t *DBTree) addReference(dataNode *model.DataNode, r *Reference) {
	if dataNode.HasChild() {
		r.Name = "dir"
		t.tree.AddNode(fmt.Sprintf("▶ %v (%v)", dataNode.Name(), dataNode.KeyNum()), r)
	} else {
		r.Name = "key"
		t.tree.AddNode(dataNode.Name(), r)
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

	key := reference.Data.Key()
	notice := fmt.Sprintf("Rename %v->%v", key, t.preview.GetKey())
	t.ShowModal(notice, func() {
		if key == t.preview.GetKey() {
			return
		}

		tlog.Log("rename %v %v", key, t.preview.GetKey())
		t.data.Rename(reference.Data, t.preview.GetKey())
		t.getCurrentNode().SetText(reference.Data.Name())
	})
}

func (t *DBTree) deleteKey() {
	typ := t.getReference(t.getCurrentNode())
	if typ == nil {
		return
	}
	var notice string
	switch typ.Name {
	case "key":
		notice = "Delete " + typ.Data.Key() + " ?"
	case "index":
		notice = fmt.Sprintf("FlushDB index:%v?", typ.Index)
	case "dir":
		notice = "Delete " + typ.Data.Key() + "* ?"
	}
	t.ShowModal(notice, func() {
		go t.deleteSelectKey(typ)
	})
}

func (t *DBTree) saveKey(oldValue, newValue string) {
	if oldValue == newValue {
		t.ShowModalOK("Nothing to save")
		return
	}
	typ := t.getReference(t.getCurrentNode())
	if typ == nil {
		return
	}
	switch typ.Name {
	case "key":
		if err := t.data.SetValue(typ.Data, newValue); err == nil {
			t.preview.ShowText(newValue, true)
			t.ShowModalOK("Value was updated!")
		} else {
			tlog.Log("saveKey %v", err)
		}
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
		tlog.Log("delete %v", typ.Data.Key())
		if err := t.data.Delete(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.tree.SetNodeRemoved()
		t.updatePreviewWithType(fmt.Sprintf("%v was removed", typ.Data.Key()), "", false)
	case "index":
		if err := t.data.FlushDB(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.getCurrentNode().ClearChildren()
		t.getCurrentNode().SetText(typ.Data.Name())
	case "dir":
		tlog.Log("delete %v", typ.Data.Key())
		if err := t.data.Delete(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.tree.SetNodeRemoved()
		t.updatePreviewWithType("", "", false)
	default:
		tlog.Log("delete %v not implement", typ.Name)
	}
}

// Expand loads the db level (db0..dbN) and expands the root node.
// Connect opens the redis connection. It may block on the network, so Show calls
// it from a background goroutine before Expand.
func (t *DBTree) Connect() error {
	return t.data.Connect()
}

// SetConnState sets the right-aligned connection-status glyph on the root row.
func (t *DBTree) SetConnState(s connState) {
	applyConnState(t.tree, s)
}

// Failed reports whether the root currently shows the failed status.
func (t *DBTree) Failed() bool {
	return t.tree.HasRootStatus('✖')
}

// SetRootClickFunc registers a handler consulted when the root node is selected.
func (t *DBTree) SetRootClickFunc(f func() bool) {
	t.rootClick = f
}

func (t *DBTree) Expand() {
	root := t.tree.GetRoot()
	t.tree.SetCurrentNode(root)
	t.OnSelected(root)
	root.SetExpanded(true)
}

// Filter rescans every expanded db with a substring match on the key name,
// rebuilding that db's key tree. An empty term restores all keys. Collapsed
// dbs are left untouched and re-filter lazily when next expanded.
//
// The user's expand/collapse state is preserved across the rebuild: before
// clearing we snapshot which dir subtrees were open (and the selected node) by
// key, then re-materialize and re-expand exactly those after the rescan.
func (t *DBTree) Filter(term string) {
	pattern := globContains(term)
	for _, idxNode := range t.tree.GetRoot().GetChildren() {
		ref := t.getReference(idxNode)
		if ref == nil || ref.Name != "index" {
			continue
		}
		if !idxNode.IsExpanded() || !t.opened[ref.Index] {
			continue // only refresh dbs the user has actually opened
		}
		if err := t.changeDB(ref.Index); err != nil {
			if err := t.data.Connect(); err != nil {
				continue
			}
			t.changeDB(ref.Index)
		}

		expanded := make(map[string]bool)
		collectExpandedKeys(idxNode, expanded)
		selKey := t.selectedKey()

		dataNodes, err := t.data.ScanKeysMatch(pattern)
		if err != nil {
			continue
		}
		idxNode.ClearChildren()
		for _, dataNode := range dataNodes {
			r := &Reference{Index: ref.Index, Data: dataNode}
			child := t.addChildReference(idxNode, dataNode, r)
			t.restoreExpansion(child, dataNode, ref.Index, expanded)
		}
		if selKey != "" {
			if n := findNodeByKey(idxNode, selKey); n != nil {
				t.tree.SetCurrentNode(n)
			}
		}
	}
}

// collectExpandedKeys records, by key, every dir node under parent that the
// user had expanded (and thus had loaded children), so Filter can restore them.
func collectExpandedKeys(parent *tview.TreeNode, out map[string]bool) {
	for _, child := range parent.GetChildren() {
		ref, _ := child.GetReference().(*Reference)
		if ref == nil || ref.Data == nil {
			continue
		}
		if child.IsExpanded() && len(child.GetChildren()) > 0 {
			out[ref.Data.Key()] = true
			collectExpandedKeys(child, out)
		}
	}
}

// restoreExpansion re-materializes and re-expands node's subtree when its key
// was previously open, recursing so nested expanded dirs are restored too.
func (t *DBTree) restoreExpansion(node *tview.TreeNode, dataNode *model.DataNode, index int, expanded map[string]bool) {
	children := t.data.GetChildren(dataNode)
	if !expanded[dataNode.Key()] || len(children) == 0 {
		return
	}
	for _, dn := range children {
		r := &Reference{Index: index, Data: dn}
		child := t.addChildReference(node, dn, r)
		t.restoreExpansion(child, dn, index, expanded)
	}
	node.SetExpanded(true)
	node.SetText(fmt.Sprintf("▼ %v (%v)", dataNode.Name(), dataNode.KeyNum()))
}

// selectedKey returns the key of the currently selected node, or "".
func (t *DBTree) selectedKey() string {
	cur := t.tree.GetCurrentNode()
	if cur == nil {
		return ""
	}
	if ref := t.getReference(cur); ref != nil && ref.Data != nil {
		return ref.Data.Key()
	}
	return ""
}

// findNodeByKey searches parent's subtree for a node whose reference key matches.
func findNodeByKey(parent *tview.TreeNode, key string) *tview.TreeNode {
	for _, child := range parent.GetChildren() {
		ref, _ := child.GetReference().(*Reference)
		if ref != nil && ref.Data != nil && ref.Data.Key() == key {
			return child
		}
		if n := findNodeByKey(child, key); n != nil {
			return n
		}
	}
	return nil
}

// addChildReference adds a data node under a specific parent (filter rebuilds
// under an index node rather than the tree's current node) and returns it.
func (t *DBTree) addChildReference(parent *tview.TreeNode, dataNode *model.DataNode, r *Reference) *tview.TreeNode {
	if dataNode.HasChild() {
		r.Name = "dir"
		return t.tree.AddChildNode(parent, fmt.Sprintf("▶ %v (%v)", dataNode.Name(), dataNode.KeyNum()), r)
	}
	r.Name = "key"
	return t.tree.AddChildNode(parent, dataNode.Name(), r)
}

// globContains turns a plain search term into a redis glob that matches the
// term as a substring, escaping glob metacharacters so they're literal. An
// empty term matches everything.
func globContains(term string) string {
	if term == "" {
		return "*"
	}
	var b strings.Builder
	b.WriteByte('*')
	for _, r := range term {
		switch r {
		case '*', '?', '[', ']', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('*')
	return b.String()
}

// TreeView returns the underlying tview tree.
func (t *DBTree) TreeView() *view.Tree {
	return t.tree
}

// PreviewFlex returns the preview pane.
func (t *DBTree) PreviewFlex() *tview.Flex {
	return t.preview.FlexBox()
}

// Preview returns the preview pane controller.
func (t *DBTree) Preview() *view.Preview {
	return t.preview
}

// Index returns the currently selected db index.
func (t *DBTree) Index() int {
	return t.data.Index()
}

// Cmd runs a raw redis command.
func (t *DBTree) Cmd(w io.Writer, cmd string, params ...string) error {
	return t.data.Cmd(w, cmd, params...)
}

// Close close
func (t *DBTree) Close() {
	t.data.Close()
}
