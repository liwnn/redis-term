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

// Keys keys
func (r *Redis) Keys(pattern string) []string {
	result, err := r.client.Do("keys", pattern)
	if err != nil {
		return nil
	}
	d, err := result.List()
	if err != nil {
		return nil
	}
	return d
}

// DBTree tree.
type DBTree struct {
	root *tview.TreeNode
	tree *tview.TreeView
}

// NewDBTree new
func NewDBTree(rootName string) *DBTree {
	root := tview.NewTreeNode(rootName).SetColor(tcell.ColorRed)
	root.SetReference("db")
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	dbTree := &DBTree{
		root: root,
		tree: tree,
	}
	tree.SetSelectedFunc(dbTree.OnSelected)
	return dbTree
}

// AddNode add node
func (t *DBTree) AddNode(target *tview.TreeNode, name string, reference interface{}) {
	node := tview.NewTreeNode(name).SetSelectable(true)
	if reference != nil {
		node.SetReference(reference)
	}
	node.SetColor(tcell.ColorGreen)
	target.AddChild(node)
}

// OnSelected on select
func (t *DBTree) OnSelected(node *tview.TreeNode) {
	reference := node.GetReference()
	if reference == nil {
		return
	}
	childen := node.GetChildren()
	if len(childen) == 0 {
		typ, ok := reference.(string)
		if !ok {
			log.Fatalf("reference \n")
		}
		switch typ {
		case "db":
		}
		nodes, reference := f()
		for _, k := range nodes {
			t.AddNode(node, k, reference)
		}
	} else {
		node.SetExpanded(!node.IsExpanded())
	}
}

func main() {
	client := NewRedis("127.0.0.1:9898")
	defer client.Close()

	keys := func() ([]string, func() []string) {
		return client.Keys("*"), nil
	}

	initDB := func() ([]string, func() ([]string, func() []string)) {
		dbNum, err := client.GetDatabases()
		if err != nil {
			log.Fatalln(err)
		}
		r := make([]string, 0, dbNum)
		for index := 0; index < dbNum; index++ {
			r = append(r, "db"+strconv.Itoa(index))
		}
		return r, keys
	}

	onSelect := func(t *DBTree, node *tview.TreeNode, typ string) {
		switch typ {
		case "db":
			dbNum, err := client.GetDatabases()
			if err != nil {
				log.Fatalln(err)
			}

			for index := 0; index < dbNum; index++ {
				t.AddNode(node, "db"+strconv.Itoa(index), "index")
			}
		case "index":
		}
	}

	pages := tview.NewPages()

	tree := NewDBTree("127.0.0.1")
	tree1 := NewDBTree("127.0.0.1")

	keyFlexBox := tview.NewFlex()
	keyFlexBox.SetDirection(tview.FlexRow)
	keyFlexBox.SetBorder(true)
	keyFlexBox.SetTitle("KEYS")
	keyFlexBox.AddItem(tree.tree, 0, 1, true)
	keyFlexBox.AddItem(tree1.tree, 0, 1, false)

	previewFlexBox := tview.NewFlex()
	previewFlexBox.SetDirection(tview.FlexRow)
	previewText := tview.NewTextView()
	previewText.
		SetDynamicColors(true).
		SetRegions(true).
		SetScrollable(true).
		SetTitle("PREVIEW").
		SetBorder(true).
		SetBorderColor(tcell.ColorSteelBlue)

	previewFlexBox.AddItem(previewText, 0, 10, false)

	mainFlexBox := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(keyFlexBox, 0, 1, true).
		AddItem(previewFlexBox, 0, 4, false)

	pages.AddPage("main", mainFlexBox, true, true)

	app := tview.NewApplication()
	if err := app.SetRoot(pages, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}
