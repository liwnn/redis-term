package main

import (
	"log"
	"net"
	"strconv"

	"redis-term/redis"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

// Redis client
type Redis struct {
	client *redis.Client
}

// NewRedis new
func NewRedis(address string) *Redis {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		log.Fatalln(err)
	}
	client := redis.NewClient(conn)
	return &Redis{
		client: client,
	}
}

// Close close conn.
func (r *Redis) Close() {
	r.client.Close()
}

// GetDatabases return database count.
func (r *Redis) GetDatabases() (int, error) {
	result, err := r.client.Do("config", "get", "databases")
	if err != nil {
		return 0, err
	}
	d, err := result.List()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(d[1])
}

// DBTree tree.
type DBTree struct {
	root *tview.TreeNode
	tree *tview.TreeView
}

// NewDBTree new
func NewDBTree() *DBTree {
	root := tview.NewTreeNode("127.0.0.1").SetColor(tcell.ColorRed)
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	dbTree := &DBTree{
		root: root,
		tree: tree,
	}
	tree.SetSelectedFunc(dbTree.OnSelected)
	return dbTree
}

// AddNode add node
func (t *DBTree) AddNode(target *tview.TreeNode, name string) {
	node := tview.NewTreeNode(name).SetReference("test").SetSelectable(true)
	node.SetColor(tcell.ColorGreen)
	target.AddChild(node)
}

// OnSelected on select
func (t *DBTree) OnSelected(node *tview.TreeNode) {
	node.SetExpanded(!node.IsExpanded())

	//reference := node.GetReference()
	//if reference == nil {
	//    return
	//}
	//childen := node.GetChildren()
	//if len(childen) == 0 {
	//    addNode(root, "test2")
	//} else {
	//    node.SetExpanded(!node.IsExpanded())
	//}
}

// InitDB init db index.
func (t *DBTree) InitDB(databases int) {
	for index := 0; index < databases; index++ {
		t.AddNode(t.root, "db"+strconv.Itoa(index))
	}
}

func main() {
	client := NewRedis("127.0.0.1:9898")
	defer client.Close()
	dbNum, err := client.GetDatabases()
	if err != nil {
		log.Fatalln(err)
	}

	tree := NewDBTree()
	tree.InitDB(dbNum)
	if err := tview.NewApplication().SetRoot(tree.tree, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
