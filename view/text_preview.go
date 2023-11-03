package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type TextPreview struct {
	*tview.Flex
	view *tview.TextArea

	oldText  string
	saveBtn  *tview.Button
	saveGrid *tview.Grid

	onSave func(oldValue, newValue string)
}

func NewTextPreview() *TextPreview {
	p := &TextPreview{}
	p.init()
	return p
}

func (p *TextPreview) init() {
	view := tview.NewTextArea().SetWrap(true)

	saveBtn := tview.NewButton("Save")
	saveBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	saveBtn.SetSelectedFunc(func() {
		if p.onSave != nil {
			newValue := p.view.GetText()
			p.onSave(p.oldText, newValue)
		}
	})

	grid := tview.NewGrid().SetColumns(-1, 8).SetBorders(false).SetGap(0, 2).SetMinSize(5, 5)
	grid.AddItem(saveBtn, 0, 1, 1, 1, 0, 0, false)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(view, 0, 1, true).
		AddItem(grid, 1, 1, false)

	p.view = view
	p.saveBtn = saveBtn
	p.saveGrid = grid
	p.Flex = flex
}

func (p *TextPreview) SetText(text string) {
	p.oldText = text
	// if len(text) > 4096 {
	// 	text = text[:4096] + "..."
	// }
	p.view.SetText(text, true)
}

func (p *TextPreview) ShowSaveGrid(visible bool) {
	p.saveGrid.Clear()
	if visible {
		p.saveGrid.AddItem(p.saveBtn, 0, 1, 1, 1, 0, 0, false)
	}
}

func (p *TextPreview) SetSaveHandler(f func(string, string)) {
	p.onSave = f
}
