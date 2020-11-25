package redisterm

import (
	"fmt"
	"strings"
)

// DataNode node
type DataNode struct {
	name    string
	key     string
	child   []*DataNode
	p       *DataNode
	removed bool
}

// CanExpand 是否可展开
func (n *DataNode) CanExpand() bool {
	return len(n.child) != 0
}

// ClearChildren remove all children
func (n *DataNode) ClearChildren() {
	n.child = n.child[:0]
}

// GetChildren return childers.
func (n *DataNode) GetChildren() []*DataNode {
	return n.child
}

// GetChildByKey return child.
func (n *DataNode) GetChildByKey(key string) *DataNode {
	for _, v := range n.child {
		if v.key == key {
			return v
		}
	}
	return nil
}

// RemoveChild remove child
func (n *DataNode) RemoveChild(child *DataNode) *DataNode {
	for i, v := range n.child {
		if v == child {
			n.child = append(n.child[:i], n.child[i+1:]...)
			return child
		}
	}
	return nil
}

// AddChild add child
func (n *DataNode) AddChild(name, key string) *DataNode {
	node := &DataNode{
		name: name,
		key:  key,
		p:    n,
	}
	n.child = append(n.child, node)
	return node
}

// RemoveSelf remove self.
func (n *DataNode) RemoveSelf() {
	n.p.RemoveChild(n)
	if len(n.p.child) == 0 {
		n.p.RemoveSelf()
	}
}

// DataTree 数据
type DataTree struct {
	root *DataNode
}

// NewDataTree new
func NewDataTree(rootName string) *DataTree {
	t := &DataTree{
		root: &DataNode{
			name: rootName,
		},
	}
	return t
}

// AddKey 增加key
func (t *DataTree) AddKey(key string) {
	var lastColon int = -1
	var p = t.root
	for i, c := range key {
		if c != ':' {
			continue
		}
		prefix := key[:i+1]
		name := key[lastColon+1 : i]
		node := p.GetChildByKey(prefix)
		if node == nil {
			node = p.AddChild(name, prefix)
		}
		lastColon = i
		p = node
	}
	name := key[lastColon+1:]
	prefix := key
	//if node := p.GetChildByKey(prefix); node == nil {
	p.AddChild(name, prefix)
	//}
}

// GetChildren name
func (t *DataTree) GetChildren(p *DataNode) []*DataNode {
	return p.child
}

// Dump 输出
func (t *DataTree) Dump(n *DataNode, level int) {
	space := strings.Repeat("  ", level)
	pre := ""
	if n.child != nil {
		pre = "+"
	}
	fmt.Println(space, pre, n.name)
	if n.child == nil {
		return
	}
	for _, v := range n.child {
		t.Dump(v, level+1)
	}
}
