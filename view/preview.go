package view

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Preview preview
type Preview struct {
	flexBox *tview.Flex

	showFlex  *tview.Flex
	sizeText  *tview.TextView
	delBtn    *tview.Button
	reloadBtn *tview.Button
	renameBtn *tview.Button
	keyInput  *tview.InputField
	grid      *tview.Grid

	textPreview  *TextPreview
	tablePreview *TablePreview
}

// NewPreview new
func NewPreview() *Preview {
	sizeText := tview.NewTextView()
	keyInput := tview.NewInputField()
	keyInput.SetLabel("Key:").
		SetLabelWidth(4).
		SetLabelColor(tcell.ColorWhite).
		SetFieldBackgroundColor(tcell.ColorDarkSlateGrey)
	delBtn := tview.NewButton("Delete")
	delBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	reloadBtn := tview.NewButton("Reload")
	reloadBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	renameBtn := tview.NewButton("Rename")
	renameBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	grid := tview.NewGrid().
		SetRows(-1).
		SetColumns(20, 10, 10, 30, 10, -1).
		SetBorders(false).
		SetGap(0, 2).
		SetMinSize(5, 5)
	grid.AddItem(sizeText, 0, 0, 1, 1, 0, 0, false)

	showFlex := tview.NewFlex()
	showFlex.
		SetTitle("PREVIEW").
		SetBorder(true).
		SetBorderColor(tcell.ColorSteelBlue)

	previewFlexBox := tview.NewFlex()
	previewFlexBox.SetDirection(tview.FlexRow)
	previewFlexBox.AddItem(grid, 1, 0, false)
	previewFlexBox.AddItem(showFlex, 0, 1, false)

	p := &Preview{
		flexBox:   previewFlexBox,
		showFlex:  showFlex,
		sizeText:  sizeText,
		delBtn:    delBtn,
		reloadBtn: reloadBtn,
		renameBtn: renameBtn,
		keyInput:  keyInput,
		grid:      grid,

		textPreview:  NewTextPreview(),
		tablePreview: NewTablePreview(),
	}
	p.init()
	return p
}

func (p *Preview) FlexBox() *tview.Flex {
	return p.flexBox
}

func (p *Preview) init() {
	p.tablePreview.SetSelectionChangedFunc(func(row, column int) {
		if row <= 0 {
			return
		}
		h := p.tablePreview.rows
		if row-1 >= len(h) {
			return
		}
		c := h[p.tablePreview.curPage*p.tablePreview.pageDelta+row-1]
		size := len(c[len(c)-1])
		p.SetSizeText(fmt.Sprintf("Size: %d bytes", size))
	})
}

// Clear all
func (p *Preview) Clear() {
	p.SetSizeText("")
}

// SetSizeText show text size
func (p *Preview) SetSizeText(text string) {
	p.sizeText.SetText(text)
}

// SetOpBtnVisible show reload delete button
func (p *Preview) SetOpBtnVisible(visible bool) {
	if visible {
		p.grid.AddItem(p.reloadBtn, 0, 1, 1, 1, 0, 0, false)
		p.grid.AddItem(p.delBtn, 0, 2, 1, 1, 0, 0, false)
	} else {
		p.grid.RemoveItem(p.reloadBtn)
		p.grid.RemoveItem(p.delBtn)
	}
}

// SetDeleteFunc 设置删除回调
func (p *Preview) SetDeleteFunc(f func()) {
	p.delBtn.SetSelectedFunc(f)
}

// SetDeleteText set delete button text
func (p *Preview) SetDeleteText(text string) {
	p.delBtn.SetLabel(text)
}

func (p *Preview) SetSaveFunc(f func(oldValue, newValue string)) {
	p.textPreview.SetSaveHandler(f)
}

// SetKey set key input text
func (p *Preview) SetKey(text string) {
	if len(text) > 0 {
		p.grid.AddItem(p.keyInput, 0, 3, 1, 1, 0, 0, false)
		p.grid.AddItem(p.renameBtn, 0, 4, 1, 1, 0, 0, false)
		p.keyInput.SetText(text)
	} else {
		p.grid.RemoveItem(p.keyInput)
		p.grid.RemoveItem(p.renameBtn)
	}
}

// GetKey return key
func (p *Preview) GetKey() string {
	return p.keyInput.GetText()
}

// SetReloadFunc set reload function
func (p *Preview) SetReloadFunc(f func()) {
	p.reloadBtn.SetSelectedFunc(f)
}

// SetRenameFunc set rename function
func (p *Preview) SetRenameFunc(f func()) {
	p.renameBtn.SetSelectedFunc(f)
}

func (p *Preview) ShowTable(title []TablePageTitle, rows []Row) {
	p.showFlex.Clear()
	p.showFlex.AddItem(p.tablePreview, 0, 1, false)
	p.tablePreview.Update(title, rows)
}

func (p *Preview) ShowText(text string, showSave bool) {
	p.showFlex.Clear()
	p.showFlex.AddItem(p.textPreview, 0, 1, false)
	p.textPreview.SetText(text)
	p.textPreview.ShowSaveGrid(showSave)
}
