package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type OpLine struct {
	*tview.Flex
	selectDrop *tview.DropDown
}

func NewOpLine() *OpLine {
	drop := tview.NewDropDown().SetLabel("Select server:")

	saveBtn := tview.NewButton("+")
	saveBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	saveBtn.SetSelectedFunc(func() {
	})

	flex := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(drop, 0, 1, false).
		AddItem(saveBtn, 3, 1, false)

	return &OpLine{
		Flex:       flex,
		selectDrop: drop,
	}
}

// AddSelect add select
func (o *OpLine) AddSelect(text string) {
	o.selectDrop.AddOption(text, nil)
}

// Select db
func (o *OpLine) Select(index int) {
	o.selectDrop.SetCurrentOption(index)
}

func (o *OpLine) SetSelectedFunc(handler func(index int)) {
	o.selectDrop.SetSelectedFunc(func(text string, index int) {
		handler(index)
	})
}
