package app

import (
	"fmt"
	"log"
	"time"

	"github.com/liwnn/redisterm/model"
	"github.com/liwnn/redisterm/redisapi"
	"github.com/liwnn/redisterm/tlog"
	"github.com/liwnn/redisterm/view"

	"github.com/rivo/tview"
)

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

	ShowModalOK func(string)
	ShowModal   func(text string, okFunc func())
}

// NewDBTree new
func NewDBTree(tree *view.Tree, preview *view.Preview) *DBTree {
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
	err := t.changeDB(typ.Index)
	if err != nil {
		if err := t.data.Connect(); err != nil {
			tlog.Log("[OnSelected] %v", err)
			return
		}
	}
	tlog.Log("OnSelected: name[%v] index[%v]", typ.Name, typ.Index)
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
				if err := t.data.Connect(); err == nil {
					dbs, _ = t.data.GetDatabases()
				} else {
					tlog.Log("[OnSelected] db %v", err)
					return
				}
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
	typ := t.getReference(node)
	if typ.Name == "db" {
		tlog.Log("OnChanged: db %v", typ.Name)
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
			o := t.data.GetValue(typ.Data.Key())
			tlog.Log("redis value time cost %v", time.Since(begin))
			t.updatePreview(o, true)
		} else {
			t.updatePreview(fmt.Sprintf("%v was removed", typ.Data.Key()), false)
		}
		t.preview.SetDeleteText("Delete")
		t.preview.SetKey(typ.Data.Key())
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
		for _, v := range h {
			rows = append(rows, view.Row{v})
		}
		p.ShowTable(title, rows)
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
		for _, v := range h {
			rows = append(rows, view.Row{v.Key, v.Value})
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
	key := reference.Data.Key()
	tlog.Log("reload %v", key)

	if reference.Name == "key" {
		t.changeDB(reference.Index)
		o := t.data.GetValue(key)
		if o == nil {
			reference.Data.SetRemoved()
			t.tree.SetNodeRemoved()
			t.updatePreview(fmt.Sprintf("%v was removed", key), false)
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
		t.updatePreview(fmt.Sprintf("%v was removed", typ.Data.Key()), false)
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
		t.updatePreview("", false)
	default:
		tlog.Log("delete %v not implement", typ.Name)
	}
}

// Close close
func (t *DBTree) Close() {
	t.data.Close()
}
