package main

import (
	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

var (
	app *tview.Application
)

func main() {
	root := tview.NewTreeNode("test").SetColor(tcell.ColorRed)
	tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
	if err := tview.NewApplication().SetRoot(tree, true).Run(); err != nil {
		panic(err)
	}

}
