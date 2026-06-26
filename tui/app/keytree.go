package app

// DataNode is a node in a redis key tree, where keys are split on ':' into a
// browsable hierarchy (key "user:1:name" -> "user:" -> "user:1:" -> leaf).
// Interior nodes are key prefixes (not real keys); leaf nodes are real keys.
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

// addChild appends a child to n, drawing the node from the tree's slab so a
// large keyspace doesn't trigger one heap allocation per node. The child map is
// only built once a node has enough children for linear scan to matter.
func (t *DataTree) addChild(n *DataNode, name, key string) *DataNode {
	node := t.newNode()
	node.name = name
	node.key = key
	node.p = n
	node.keyNum = 1
	n.child = append(n.child, node)
	if len(n.child) > 20 {
		if n.childMap == nil {
			n.childMap = make(map[string]*DataNode, len(n.child)*2)
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

// slabBlock is how many DataNodes are allocated at once. A 2M-key tree needs
// ~2.25M nodes; carving them from big blocks turns ~2.25M heap allocations into
// a few hundred, which is the bulk of the build-time win.
const slabBlock = 8192

// DataTree is one redis db's key tree.
type DataTree struct {
	root *DataNode
	slab []DataNode
}

// newNode returns a zeroed *DataNode carved from the current slab block,
// allocating a fresh block when the current one is exhausted. Nodes never move,
// so pointers into the slab stay valid for the tree's lifetime.
func (t *DataTree) newNode() *DataNode {
	if len(t.slab) == cap(t.slab) {
		t.slab = make([]DataNode, 0, slabBlock)
	}
	t.slab = t.slab[:len(t.slab)+1]
	return &t.slab[len(t.slab)-1]
}

// NewDataTree new
func NewDataTree(rootName string) *DataTree {
	t := &DataTree{}
	t.root = t.newNode()
	t.root.name = rootName
	return t
}

// Root returns the tree's root node.
func (t *DataTree) Root() *DataNode {
	return t.root
}

// AddKey splits key on ':' and inserts the prefix path and leaf into the tree.
// The scan is byte-wise, not rune-wise: ':' is ASCII and UTF-8 is self-
// synchronizing, so it can never appear inside a multi-byte rune — skipping the
// per-byte rune decode that "for range key" would do.
func (t *DataTree) AddKey(key string) {
	var lastColon int = -1
	var p = t.root
	for i := 0; i < len(key); i++ {
		if key[i] != ':' {
			continue
		}
		prefix := key[:i+1]
		name := key[lastColon+1 : i]
		node := p.GetChildByKey(prefix)
		if node == nil {
			node = t.addChild(p, name, prefix)
		} else {
			node.keyNum++
		}
		lastColon = i
		p = node
	}
	name := key[lastColon+1:]
	prefix := key
	if p.GetChildByKey(prefix) == nil {
		t.addChild(p, name, prefix)
	}
}

// GetChildren name
func (t *DataTree) GetChildren(p *DataNode) []*DataNode {
	return p.child
}

// leafKeys collects every real key (leaf) under n, in tree order. Interior
// prefix nodes are skipped; only nodes without children are real keys.
func leafKeys(n *DataNode, out *[]string) {
	if !n.HasChild() {
		*out = append(*out, n.key)
		return
	}
	for _, c := range n.child {
		leafKeys(c, out)
	}
}

// LeafKeys returns all real keys under n (n itself if it is a leaf).
func LeafKeys(n *DataNode) []string {
	var out []string
	leafKeys(n, &out)
	return out
}
