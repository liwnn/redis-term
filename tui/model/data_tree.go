package model

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
	keyNum  int
	removed bool

	childMap map[string]*DataNode
}

func (n *DataNode) Name() string {
	return n.name
}

func (n *DataNode) Key() string {
	return n.key
}

func (n *DataNode) KeyNum() int {
	return n.keyNum
}

func (n *DataNode) IsRemoved() bool {
	return n.removed
}

func (n *DataNode) SetRemoved() {
	n.removed = true
}

// HasChild return has child
func (n *DataNode) HasChild() bool {
	return len(n.child) != 0
}

// ClearChildren remove all children
func (n *DataNode) ClearChildren() {
	n.child = n.child[:0]
	n.childMap = make(map[string]*DataNode)
}

// GetChildren return childers.
func (n *DataNode) GetChildren() []*DataNode {
	return n.child
}

// GetChildByKey return child.
func (n *DataNode) GetChildByKey(key string) *DataNode {
	if len(n.childMap) > 0 {
		return n.childMap[key]
	}
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
			if len(n.childMap) > 0 {
				delete(n.childMap, child.key)
			}
			return child
		}
	}
	return nil
}

// AddChild add child
func (n *DataNode) AddChild(name, key string) *DataNode {
	node := &DataNode{
		name:   name,
		key:    key,
		p:      n,
		keyNum: 1,
	}
	n.child = append(n.child, node)
	if len(n.child) > 20 {
		if len(n.childMap) == 0 {
			n.childMap = make(map[string]*DataNode)
			for _, v := range n.child {
				n.childMap[v.key] = v
			}
		} else {
			n.childMap[node.key] = node
		}
	}

	return node
}

// RemoveSelf remove self.
func (n *DataNode) RemoveSelf() {
	if n.p == nil {
		return
	}
	if n.p.RemoveChild(n) == nil {
		return
	}
	n.removed = true
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
		} else {
			node.keyNum++
		}
		lastColon = i
		p = node
	}
	name := key[lastColon+1:]
	prefix := key
	if p.GetChildByKey(prefix) == nil {
		p.AddChild(name, prefix)
	}
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
