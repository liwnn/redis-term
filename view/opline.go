package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type OpLine struct {
	*tview.Flex
	selectDrop  *tview.DropDown
	saveBtn     *tview.Button
	editBtn     *tview.Button
	saveHandler func()
	editHandler func()
}

func NewOpLine() *OpLine {
	o := &OpLine{}
	o.init()
	return o
}

func (o *OpLine) init() {
	drop := tview.NewDropDown().SetLabel("Select server:")

	saveBtn := tview.NewButton(" + ")
	saveBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	saveBtn.SetSelectedFunc(func() {
		if o.saveHandler != nil {
			o.saveHandler()
		}
	})

	editBtn := tview.NewButton(" e ")
	editBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	editBtn.SetSelectedFunc(func() {
		if o.editHandler != nil {
			o.editHandler()
		}
	})

	flex := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(drop, 0, 1, false).
		AddItem(saveBtn, 3, 1, false).
		AddItem(editBtn, 3, 1, false)

	o.Flex = flex
	o.selectDrop = drop
	o.saveBtn = saveBtn
	o.editBtn = editBtn
}

// AddSelect add select
func (o *OpLine) AddSelect(text string) {
	o.selectDrop.AddOption(text, nil)
}
func (o *OpLine) ClearAllSelect() {
	o.selectDrop.SetOptions(nil, nil)
}

// Select db
func (o *OpLine) Select(index int) {
	o.selectDrop.SetCurrentOption(index)
}

func (o *OpLine) GetOptionCount() int {
	return o.selectDrop.GetOptionCount()
}

func (o *OpLine) GetSelect() int {
	index, _ := o.selectDrop.GetCurrentOption()
	return index
}

func (o *OpLine) SetSelectedFunc(handler func(index int)) {
	o.selectDrop.SetSelectedFunc(func(text string, index int) {
		handler(index)
	})
}

func (o *OpLine) SetSaveClickFunc(handler func()) {
	o.saveHandler = handler
}

func (o *OpLine) SetEditClickFunc(handler func()) {
	o.editHandler = handler
}
