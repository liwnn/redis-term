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
	t.addNode(t.root, key, key)
}

// 增加节点
func (t *DataTree) addNode(p *DataNode, name, key string) {
	var n *DataNode
	index := strings.Index(name, ":")
	if index == -1 {
		n = t.getNodeByName(p, name)
		if n == nil {
			n = &DataNode{
				name: key,
				key:  key,
			}
			p.child = append(p.child, n)
		}
	} else {
		n = t.getNodeByName(p, name[:index])
		if n == nil {
			index1 := strings.Index(key, name)
			n = &DataNode{
				name: name[:index],
				key:  key[:index1+index+1],
			}
			p.child = append(p.child, n)
		}
		t.addNode(n, name[index+1:], key)
	}
}

func (t *DataTree) getNodeByName(p *DataNode, name string) *DataNode {
	for _, v := range p.child {
		if v.name == name && v.CanExpand() {
			return v
		}
	}
	return nil
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
