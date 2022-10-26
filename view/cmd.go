package view

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	PromptColor = "[#00aa00]"
	InputColor  = "[blue]"
	ResultColor = "[white]"
)

type CmdConsole struct {
	*tview.Flex
	view  *tview.TextView
	input *tview.InputField

	onCmdLineEnter func(string)
	title          string

	address string
	index   int
}

func NewCmdConsole(title string) *CmdConsole {
	c := &CmdConsole{
		title: title,
	}
	c.init()
	return c
}

func (c *CmdConsole) init() {
	view := tview.NewTextView()
	view.SetRegions(true).SetDynamicColors(true)

	cmdLine := tview.NewInputField()
	cmdLine.SetPlaceholder("input command")
	cmdLine.SetPlaceholderStyle(tcell.StyleDefault.Foreground(tcell.Color245).Background(tcell.Color238))
	cmdLine.SetFieldStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.Color238))
	cmdLine.SetInputCapture(c.OnEnter)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(view, 0, 1, false).
		AddItem(cmdLine, 1, 1, true)
	flex.SetBorder(true)

	c.view = view
	c.input = cmdLine
	c.Flex = flex
}

func (c *CmdConsole) OnEnter(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEnter:
		text := c.input.GetText()
		c.input.SetText("")

		fmt.Fprintln(c, text)
		if c.onCmdLineEnter != nil {
			fmt.Fprint(c.view, ResultColor)
			c.onCmdLineEnter(text)
		}

		c.printPromt()
		return nil
	}
	return event
}

func (c *CmdConsole) SetEnterHandler(handler func(string)) {
	c.onCmdLineEnter = handler
}

func (c *CmdConsole) Title() string {
	return c.title
}

func (c *CmdConsole) View() *tview.TextView {
	return c.view
}

func (c *CmdConsole) Write(p []byte) (n int, err error) {
	return c.view.Write(p)
}

func (c *CmdConsole) SetPromt(address string, index int) {
	if c.address == address && c.index == index {
		return
	}

	if c.address != "" {
		fmt.Fprintf(c.view, "\n")
	}
	c.address = address
	c.index = index

	c.printPromt()
}

func (c *CmdConsole) SetIndex(index int) {
	c.index = index
}

func (c *CmdConsole) printPromt() {
	fmt.Fprintf(c.view, "%v%v:%v> %v", PromptColor, c.address, c.index, InputColor)
	c.view.ScrollToEnd()
}
