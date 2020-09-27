package main

import (
	"fmt"
	"log"
	"net"

	"redis-term/redis"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

func newRedis() *redis.Client {
	conn, err := net.Dial("tcp", "127.0.0.1:6379")
	if err != nil {
		log.Fatalln(err)
	}
	client := redis.NewClient(conn)
	return client
}

func main() {
	client := newRedis()
	defer client.Close()
	r, err := client.Do("config", "get", "databases")
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(r)
	return

	root := tview.NewTreeNode("127.0.0.1").SetColor(tcell.ColorRed)
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)

	// Add node.
	addNode := func(target *tview.TreeNode, name string) {
		node := tview.NewTreeNode(name).SetReference("test").SetSelectable(true)
		node.SetColor(tcell.ColorGreen)
		target.AddChild(node)
	}

	addNode(root, "test1")
	addNode(root, "test2")

	tree.SetSelectedFunc(func(node *tview.TreeNode) {
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
	})

	if err := tview.NewApplication().SetRoot(tree, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
