package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type CmdConsole struct {
	*tview.Flex
	view  *tview.TextView
	input *tview.InputField

	onCmdLineEnter func(string)
	title          string
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
	cmdLine.SetFieldBackgroundColor(tcell.ColorDarkSlateGrey)
	cmdLine.SetPlaceholderTextColor(tcell.ColorDimGrey)
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

		if c.onCmdLineEnter != nil {
			c.onCmdLineEnter(text)
		}

		c.view.ScrollToEnd()
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
