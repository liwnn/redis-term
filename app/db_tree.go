package app

import (
	"fmt"
	"log"
	"redisterm/redisapi"
	"redisterm/tlog"
	"redisterm/ui"
	"time"

	"github.com/rivo/tview"
)

// Reference referenct
type Reference struct {
	Name  string
	Index int
	Data  *DataNode
}

// DBTree tree.
type DBTree struct {
	tree    *ui.Tree
	preview *ui.Preview

	data *Data

	ShowModalOK func(string)
	ShowModal   func(text string, okFunc func())
}

// NewDBTree new
func NewDBTree(tree *ui.Tree, preview *ui.Preview) *DBTree {
	dbTree := &DBTree{
		tree:    tree,
		preview: preview,
	}
	tree.SetSelectedFunc(dbTree.OnSelected)
	tree.SetChangedFunc(dbTree.OnChanged)
	preview.SetSaveFunc(dbTree.saveKey)
	preview.SetReloadFunc(dbTree.reloadSelectKey)
	preview.SetRenameFunc(dbTree.renameSelectKey)
	preview.SetDeleteFunc(dbTree.deleteKey)
	return dbTree
}

// SetData change db data.
func (t *DBTree) SetData(name string, data *Data) {
	t.tree.GetRoot().ClearChildren()
	t.tree.GetRoot().SetText(name)
	t.data = data
}

// OnSelected on select
func (t *DBTree) OnSelected(node *tview.TreeNode) {
	typ := t.getReference(node)
	err := t.data.Select(typ.Index)
	if err != nil {
		if err := t.data.Connect(); err != nil {
			tlog.Log("[OnSelected] %v", err)
			return
		}
	}
	tlog.Log("OnSelected: name[%v] index[%v]", typ.Name, typ.Index)
	if typ.Data != nil && typ.Data.HasChild() {
		t.tree.SetNodeText(typ.Data.name)
	}
	childen := node.GetChildren()
	if len(childen) == 0 {
		var dataNodes []*DataNode
		switch typ.Name {
		case "db":
			dbs, err := t.data.GetDatabases()
			if err != nil {
				if err := t.data.Connect(); err == nil {
					dbs, _ = t.data.GetDatabases()
				} else {
					tlog.Log("[OnSelected] db %v", err)
					return
				}
			}
			for i, dataNode := range dbs {
				t.tree.AddNode(dataNode.name, &Reference{
					Name:  "index",
					Index: i,
					Data:  dataNode,
				})
			}
			t.addNode(node, dataNodes)
		case "index":
			dataNodes, err = t.data.ScanAllKeys()
			if err != nil {
				if err := t.data.Connect(); err == nil {
					dataNodes, _ = t.data.ScanAllKeys()
				} else {
					tlog.Log("[OnSelected] index %v", err)
					return
				}
			}
			t.addNode(node, dataNodes)
		case "dir":
			dataNodes = t.data.GetChildren(typ.Data)
			t.addNode(node, dataNodes)
		}
	}
}

func (t *DBTree) addNode(node *tview.TreeNode, dataNodes []*DataNode) {
	typ := t.getReference(node)
	for _, dataNode := range dataNodes {
		r := &Reference{
			Index: typ.Index,
			Data:  dataNode,
		}
		if dataNode.HasChild() {
			r.Name = "dir"
			t.tree.AddNode("▶ "+dataNode.name, r)
		} else {
			r.Name = "key"
			t.tree.AddNode(dataNode.name, r)
		}
	}
}

// OnChanged on change
func (t *DBTree) OnChanged(node *tview.TreeNode) {
	typ := t.getReference(node)
	if typ.Name == "db" {
		tlog.Log("OnChanged: db %v", typ.Name)
		t.preview.SetOpBtnVisible(false)
	} else {
		if typ.Name == "index" {
			tlog.Log("OnChanged: %v - %v", typ.Name, typ.Index)
		} else {
			tlog.Log("OnChanged: %v - %v", typ.Name, typ.Data.key)
		}
		t.preview.SetOpBtnVisible(true)
	}

	if typ.Name == "key" {
		if !typ.Data.removed {
			t.data.Select(typ.Index)
			begin := time.Now()
			o := t.data.GetValue(typ.Data.key)
			tlog.Log("redis value time cost %v", time.Since(begin))
			t.updatePreview(o, true)
		} else {
			t.updatePreview(fmt.Sprintf("%v was removed", typ.Data.key), false)
		}
		t.preview.SetDeleteText("Delete")
		t.preview.SetKey(typ.Data.key)
	} else {
		if typ.Name == "index" {
			t.preview.SetDeleteText("Flush")
		} else {

			t.preview.SetDeleteText("Delete")
		}
		t.updatePreview("", false)
		t.preview.SetKey("")
	}
}

func (t *DBTree) updatePreview(o interface{}, valid bool) {
	p := t.preview
	p.Clear()
	switch h := o.(type) {
	case string:
		text := o.(string)
		p.ShowText(text, valid)
		if valid {
			p.SetSizeText(fmt.Sprintf("Size: %d bytes", len(text)))
		}
	case []string:
		title := []ui.TablePageTitle{
			{
				Name:      "row",
				Expansion: 1,
			},
			{
				Name:      "value",
				Expansion: 20,
			},
		}

		rows := make([]ui.Row, 0, len(h))
		for _, v := range h {
			rows = append(rows, ui.Row{v})
		}
		p.ShowTable(title, rows)
	case []redisapi.KVText:
		title := []ui.TablePageTitle{
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
		rows := make([]ui.Row, 0, len(h))
		for _, v := range h {
			rows = append(rows, ui.Row{v.Key, v.Value})
		}
		p.ShowTable(title, rows)
	}
}

func (t *DBTree) reloadSelectKey() {
	node := t.getCurrentNode()

	reference := t.getReference(node)
	if reference == nil {
		return
	}
	tlog.Log("reload %v", reference.Data.key)

	if reference.Name == "key" {
		t.data.Select(reference.Index)
		o := t.data.GetValue(reference.Data.key)
		if o == nil {
			reference.Data.removed = true
			t.tree.SetNodeRemoved()
			t.updatePreview(fmt.Sprintf("%v was removed", reference.Data.key), false)
			t.preview.SetDeleteText("Delete")
		} else {
			t.updatePreview(o, true)
		}
		return
	}

	node.ClearChildren()
	if err := t.data.Reload(reference.Data); err != nil {
		tlog.Log("[App] err %v", err)
		node.SetExpanded(false)
		node.SetText(reference.Data.name)
	}

	childen := reference.Data.GetChildren()
	for _, dataNode := range childen {
		r := &Reference{
			Index: reference.Index,
			Data:  dataNode,
		}
		if dataNode.HasChild() {
			r.Name = "dir"
			t.tree.AddNode("▶ "+dataNode.name, r)
		} else {
			r.Name = "key"
			t.tree.AddNode(dataNode.name, r)
		}
	}

	if reference.Data.removed {
		t.tree.SetNodeRemoved()
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

	notice := fmt.Sprintf("Rename %v->%v", reference.Data.key, t.preview.GetKey())
	t.ShowModal(notice, func() {
		if reference.Data.key == t.preview.GetKey() {
			return
		}

		tlog.Log("rename %v %v", reference.Data.key, t.preview.GetKey())
		t.data.Rename(reference.Data, t.preview.GetKey())
		t.getCurrentNode().SetText(reference.Data.name)
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
		notice = "Delete " + typ.Data.key + " ?"
	case "index":
		notice = fmt.Sprintf("FlushDB index:%v?", typ.Index)
	case "dir":
		notice = "Delete " + typ.Data.key + "* ?"
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
		tlog.Log("delete %v", typ.Data.key)
		if err := t.data.Delete(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.tree.SetNodeRemoved()
		t.updatePreview(fmt.Sprintf("%v was removed", typ.Data.key), false)
	case "index":
		if err := t.data.FlushDB(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.getCurrentNode().ClearChildren()
		t.getCurrentNode().SetText(typ.Data.name)
	case "dir":
		tlog.Log("delete %v", typ.Data.key)
		if err := t.data.Delete(typ.Data); err != nil {
			tlog.Log("DBTree deleteSelectKey %v", err)
			return
		}
		t.tree.SetNodeRemoved()
		t.updatePreview("", false)
	default:
		tlog.Log("delete %v not implement", typ.Name)
	}
}

// Close close
func (t *DBTree) Close() {
	t.data.Close()
}
