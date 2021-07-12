package ui

import (
	"fmt"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Page .
type Page interface{}

// PageText show text
type PageText struct {
	text string
}

// NewTextPage new
func NewTextPage(text string) Page {
	return &PageText{
		text: text,
	}
}

// TablePageTitle title
type TablePageTitle struct {
	Name      string
	Expansion int
}

// TablePage show table
type TablePage struct {
	title  []TablePageTitle
	rows   [][]string
	offset int
}

// NewTablePage new
func NewTablePage(title []TablePageTitle, rows [][]string, offset int) Page {
	return &TablePage{
		title:  title,
		rows:   rows,
		offset: offset,
	}
}

// Preview preview
type Preview struct {
	flexBox *tview.Flex

	showFlex  *tview.Flex
	table     *tview.Table
	sizeText  *tview.TextView
	delBtn    *tview.Button
	reloadBtn *tview.Button
	renameBtn *tview.Button
	keyInput  *tview.InputField
	grid      *tview.Grid
	prevBtn   *tview.Button
	nextBtn   *tview.Button
	numView   *tview.TextView

	textPreview *TextPreview

	pages   []Page
	curPage int
}

// NewPreview new
func NewPreview() *Preview {
	sizeText := tview.NewTextView()
	numView := tview.NewTextView()
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
	prevBtn := tview.NewButton("◀")
	prevBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	nextBtn := tview.NewButton("▶")
	nextBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	grid := tview.NewGrid().
		SetRows(-1).
		SetColumns(20, 10, 10, 30, 10, 15, 5, 5, -1).
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

	previewTable := tview.NewTable()
	style := tcell.Style{}
	previewTable.SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetFixed(1, 1).
		SetSelectedStyle(style.Foreground(tcell.ColorWhite).
			Background(tcell.ColorDarkSlateGrey).
			Attributes(tcell.AttrBold)).
		SetEvaluateAllRows(true)
	p := &Preview{
		flexBox:   previewFlexBox,
		table:     previewTable,
		showFlex:  showFlex,
		sizeText:  sizeText,
		delBtn:    delBtn,
		reloadBtn: reloadBtn,
		renameBtn: renameBtn,
		keyInput:  keyInput,
		grid:      grid,
		nextBtn:   nextBtn,
		prevBtn:   prevBtn,
		numView:   numView,

		textPreview: NewTextPreview(),
	}
	prevBtn.SetSelectedFunc(p.prevPage)
	nextBtn.SetSelectedFunc(p.nextPage)
	p.init()
	return p
}

func (p *Preview) FlexBox() *tview.Flex {
	return p.flexBox
}

func (p *Preview) init() {
	p.table.SetSelectionChangedFunc(func(row, column int) {
		if row <= 0 {
			return
		}
		page := p.pages[p.curPage]
		var size int
		switch lt := page.(type) {
		case (*TablePage):
			h := lt.rows
			if row-1 >= len(h) {
				return
			}
			c := h[row-1]
			size = len(c[len(c)-1])
		default:
		}
		p.SetSizeText(fmt.Sprintf("Size: %d bytes", size))
	})
}

// AddPage add page
func (p *Preview) AddPage(page Page) {
	p.pages = append(p.pages, page)
}

// Clear all
func (p *Preview) Clear() {
	p.pages = p.pages[:0]
	p.SetSizeText("")
}

// SetContent set
func (p *Preview) SetContent(count int) {
	p.Update(0)
	p.updatePageBtn()
	p.updateNumView(count)
}

func (p *Preview) updatePageBtn() {
	if len(p.pages) > 1 {
		p.grid.AddItem(p.nextBtn, 0, 7, 1, 1, 0, 0, false)
		p.grid.AddItem(p.prevBtn, 0, 6, 1, 1, 0, 0, false) // 0行1列,占用1行1列(2则向后占一列)
	} else {
		p.grid.RemoveItem(p.nextBtn)
		p.grid.RemoveItem(p.prevBtn)
	}
}

func (p *Preview) updateNumView(count int) {
	if count > 0 {
		p.numView.SetText("Count:" + strconv.Itoa(count))
		p.grid.AddItem(p.numView, 0, 5, 1, 1, 0, 0, false)
	} else {
		p.grid.RemoveItem(p.numView)
	}
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

func (p *Preview) nextPage() {
	if p.curPage+1 >= len(p.pages) {
		return
	}
	p.Update(p.curPage + 1)
}

func (p *Preview) prevPage() {
	if p.curPage == 0 {
		return
	}
	p.Update(p.curPage - 1)
}

// Update set
func (p *Preview) Update(pageNum int) {
	p.curPage = pageNum
	page := p.pages[p.curPage]
	switch pt := page.(type) {
	case *PageText:
		p.showFlex.Clear()
		p.showFlex.AddItem(p.textPreview, 0, 1, false)
		p.textPreview.SetText(pt.text)
	case *TablePage:
		p.showFlex.Clear()
		p.showFlex.AddItem(p.table, 0, 1, false)
		p.table.Clear()
		for i, v := range pt.title {
			p.table.SetCell(0, i, tview.NewTableCell(v.Name).SetExpansion(v.Expansion).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		}
		p.table.Select(1, 1)
		p.table.ScrollToBeginning()

		for i, row := range pt.rows {
			index := pt.offset + i
			p.table.SetCell(i+1, 0, tview.NewTableCell(strconv.Itoa(index)))
			for j, c := range row {
				if len(c) > 1024 {
					c = c[:1024]
				}
				p.table.SetCell(i+1, j+1, tview.NewTableCell(c))
			}
		}
	}
}
