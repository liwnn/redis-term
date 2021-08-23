package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type TextPreview struct {
	*tview.Flex
	view    *tview.TextView
	input   *tview.InputField
	saveBtn *tview.Button

	onSave func(oldValue, newValue string)
}

func NewTextPreview() *TextPreview {
	p := &TextPreview{}
	p.init()
	return p
}

func (p *TextPreview) init() {
	view := tview.NewTextView()
	view.SetDynamicColors(true).SetRegions(true)

	input := tview.NewInputField()
	input.SetPlaceholder("value")
	input.SetFieldBackgroundColor(tcell.ColorDarkSlateGrey)
	input.SetPlaceholderTextColor(tcell.ColorDimGrey)

	saveBtn := tview.NewButton("Save")
	saveBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	saveBtn.SetSelectedFunc(func() {
		if p.onSave != nil {
			oldValue := p.view.GetText(true)
			newValue := p.input.GetText()
			p.onSave(oldValue, newValue)
		}
	})

	grid := tview.NewGrid().SetColumns(-1, 8).SetBorders(false).SetGap(0, 2).SetMinSize(5, 5)
	grid.AddItem(input, 0, 0, 1, 1, 0, 0, false)
	grid.AddItem(saveBtn, 0, 1, 1, 1, 0, 0, false)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(view, 0, 1, true).
		AddItem(grid, 1, 1, false)

	p.input = input
	p.view = view
	p.saveBtn = saveBtn
	p.Flex = flex
}

func (p *TextPreview) SetText(text string) {
	if len(text) > 4096 {
		text = text[:4096] + "..."
	}
	p.view.SetText(text)
	p.input.SetText(text)
}

func (p *TextPreview) SetSaveHandler(f func(string, string)) {
	p.onSave = f
}
