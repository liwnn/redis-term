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

// Type type
func (r *Redis) Type(key string) string {
	result, err := r.client.Do("type", key)
	if err != nil {
		return ""
	}
	return result.String()
}

// Get get
func (r *Redis) Get(key string) string {
	result, err := r.client.Do("GET", key)
	if err != nil {
		return ""
	}
	return result.String()
}

// Select select index
func (r *Redis) Select(index int) {
	result, err := r.client.Do("SELECT", strconv.Itoa(index))
	if err != nil {
		log.Fatalln(err)
	}
	if result.String() != "OK" {
		log.Fatalln(result.String())
	}
}

// Reference referenct
type Reference struct {
	Name  string
	Index int
}

// DBTree tree.
type DBTree struct {
	root *tview.TreeNode
	tree *tview.TreeView

	redis *Redis
}

// NewDBTree new
func NewDBTree(rootName string, redis *Redis) *DBTree {
	root := tview.NewTreeNode(rootName).SetColor(tcell.ColorRed)
	root.SetReference(&Reference{
		Name: "db",
	})
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	dbTree := &DBTree{
		root:  root,
		tree:  tree,
		redis: redis,
	}
	tree.SetSelectedFunc(dbTree.OnSelected)
	tree.SetChangedFunc(dbTree.OnChanged)
	return dbTree
}

// AddNode add node
func (t *DBTree) AddNode(target *tview.TreeNode, name string, reference *Reference) {
	node := tview.NewTreeNode(name).SetSelectable(true)
	if reference != nil {
		node.SetReference(reference)
	}
	//node.SetIndent(0)
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
		typ, ok := reference.(*Reference)
		if !ok {
			log.Fatalf("reference \n")
		}
		switch typ.Name {
		case "db":
			dbNum, err := t.redis.GetDatabases()
			if err != nil {
				log.Fatalln(err)
			}

			for index := 0; index < dbNum; index++ {
				t.AddNode(node, "db"+strconv.Itoa(index), &Reference{
					Name:  "index",
					Index: index,
				})
			}
		case "index":
			t.redis.Select(typ.Index)
			for _, v := range t.redis.Keys("*") {
				t.AddNode(node, v, &Reference{
					Name: "key",
				})
			}
		}
	} else {
		node.SetExpanded(!node.IsExpanded())
	}
}

// OnChanged on change
func (t *DBTree) OnChanged(node *tview.TreeNode) {
	reference := node.GetReference()
	if reference == nil {
		return
	}
	previewText.SetText("")
	typ, ok := reference.(*Reference)
	if !ok {
		log.Fatalf("reference \n")
	}
	if typ.Name == "key" {
		val := t.redis.Type(node.GetText())
		switch val {
		case "string":
			v := t.redis.Get(node.GetText())
			previewText.SetText(v)
		}
	}
}

var (
	previewText *tview.TextView
)

func main() {
	client := NewRedis("127.0.0.1:6379")
	defer client.Close()

	pages := tview.NewPages()

	tree := NewDBTree("127.0.0.1", client)

	keyFlexBox := tview.NewFlex()
	keyFlexBox.SetDirection(tview.FlexRow)
	keyFlexBox.SetBorder(true)
	keyFlexBox.SetTitle("KEYS")
	keyFlexBox.AddItem(tree.tree, 0, 1, true)

	previewFlexBox := tview.NewFlex()
	previewFlexBox.SetDirection(tview.FlexRow)
	previewText = tview.NewTextView()
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
