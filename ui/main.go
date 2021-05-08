package ui

import (
	"fmt"
	"io"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// MainView main
type MainView struct {
	pages        *tview.Pages
	leftFlexBox  *tview.Flex
	rightFlexBox *tview.Flex
	modal        *tview.Modal
	selectDrop   *tview.DropDown

	bottomPanel tview.Primitive
	console     *tview.TextView

	cmdLineView    *tview.TextView
	onCmdLineEnter func(string)
}

// NewMainView new
func NewMainView() *MainView {
	m := &MainView{}
	m.initLayout()
	return m
}

func (m *MainView) initLayout() {
	m.selectDrop = tview.NewDropDown().SetLabel("Select server:")
	m.leftFlexBox = tview.NewFlex().SetDirection(tview.FlexRow)
	m.rightFlexBox = tview.NewFlex().SetDirection(tview.FlexRow)
	m.modal = m.createModal()
	mainFlexBox := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(m.leftFlexBox, 0, 1, true).
		AddItem(m.rightFlexBox, 0, 4, false)
	m.pages = tview.NewPages()
	m.pages.AddPage("main", mainFlexBox, true, true)
	m.pages.AddPage("modal", m.modal, true, false)

	m.bottomPanel = m.createBottom()
}

func (m *MainView) createModal() *tview.Modal {
	modal := tview.NewModal().
		AddButtons([]string{"Ok", "Cancel"})
	return modal
}

// ShowModal show modal
func (m *MainView) ShowModal(text string, okFunc func()) {
	m.modal.SetText(text).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				okFunc()
			}
			m.pages.HidePage("modal")
		})
	m.pages.ShowPage("modal")
}

// Run run
func (m *MainView) Run() error {
	return tview.NewApplication().SetRoot(m.pages, true).EnableMouse(true).Run()
}

// AddSelect add select
func (m *MainView) AddSelect(text string) {
	m.selectDrop.AddOption(text, nil)
}

// Select db
func (m *MainView) Select(index int) {
	m.selectDrop.SetCurrentOption(index)
}

func (m *MainView) SetSelectedFunc(handler func(index int)) {
	m.selectDrop.SetSelectedFunc(func(text string, index int) {
		handler(index)

	})
}

func (m *MainView) SetTree(tree *tview.TreeView) {
	m.leftFlexBox.Clear()
	m.leftFlexBox.AddItem(m.selectDrop, 1, 0, false)
	m.leftFlexBox.AddItem(tree, 0, 1, true)
}

func (m *MainView) SetPreview(preview *tview.Flex) {
	m.rightFlexBox.Clear()
	m.rightFlexBox.AddItem(preview, 0, 3, false)
	m.rightFlexBox.AddItem(m.bottomPanel, 0, 1, false)
}

func (m *MainView) Show(index int) {
	m.selectDrop.SetCurrentOption(0)
}

func (m *MainView) GetOutput() io.Writer {
	return m.console
}

func (m *MainView) createBottom() tview.Primitive {
	pages := tview.NewPages()

	info := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false).
		SetHighlightedFunc(func(added, removed, remaining []string) {
			pages.SwitchToPage(added[0])
		})

	{
		title := "CONSOLE"
		console := tview.NewTextView()
		console.
			SetScrollable(true).
			SetTitle(title).
			SetBorder(true)
		m.console = console
		pages.GetPageCount()
		pages.AddPage(title, console, true, true)
		fmt.Fprintf(info, `["%v"][slategrey]%s[white][""] `, title, title)
	}

	{
		title := "redis-cli"
		cmdLine := tview.NewInputField()
		view := tview.NewTextView()
		m.cmdLineView = view

		cmdLine.SetPlaceholder("input command")
		cmdLine.SetFieldBackgroundColor(tcell.ColorDarkSlateGrey)
		cmdLine.SetPlaceholderTextColor(tcell.ColorDimGrey)
		cmdLine.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			switch event.Key() {
			case tcell.KeyEnter:
				text := cmdLine.GetText()
				cmdLine.SetText("")

				m.onCmdLineEnter(text)

				view.ScrollToEnd()
				return nil
			}
			return event
		})

		view.SetRegions(true).SetDynamicColors(true)
		redisCli := tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(view, 0, 1, false).
			AddItem(cmdLine, 1, 1, true)
		redisCli.SetBorder(true)
		pages.AddPage(title, redisCli, true, false)
		fmt.Fprintf(info, `["%v"][slategrey]%s[white][""] `, title, title)
	}

	info.Highlight("CONSOLE")

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(pages, 0, 1, false).
		AddItem(info, 1, 1, false)
	return layout
}

func (m *MainView) SetCmdLineEnter(handler func(string)) {
	m.onCmdLineEnter = handler
}

func (m *MainView) GetCmdWriter() io.Writer {
	return m.cmdLineView
}
