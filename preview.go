package redisterm

import (
	"fmt"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type page struct {
	data interface{}
}

// Preview preview
type Preview struct {
	flexBox *tview.Flex

	showFlex  *tview.Flex
	textView  *tview.TextView
	table     *tview.Table
	sizeText  *tview.TextView
	delBtn    *tview.Button
	reloadBtn *tview.Button
	renameBtn *tview.Button
	keyInput  *tview.InputField
	grid      *tview.Grid

	output *tview.TextView

	pages     []page
	pageDelta int
	curPage   int
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
	prevBtn := tview.NewButton("◀")
	prevBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	nextBtn := tview.NewButton("▶")
	nextBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	grid := tview.NewGrid().
		SetRows(-1).
		SetColumns(20, 10, 10, 30, 10, 5, 5, -1).
		SetBorders(false).
		SetGap(0, 2).
		SetMinSize(5, 5)
	grid.AddItem(sizeText, 0, 0, 1, 1, 0, 0, false)
	grid.AddItem(reloadBtn, 0, 1, 1, 1, 0, 0, false)
	grid.AddItem(delBtn, 0, 2, 1, 1, 0, 0, false)
	grid.AddItem(prevBtn, 0, 5, 1, 1, 0, 0, false) // 0行1列,占用1行1列(2则向后占一列)
	grid.AddItem(nextBtn, 0, 6, 1, 1, 0, 0, false)

	showFlex := tview.NewFlex()
	showFlex.
		SetTitle("PREVIEW").
		SetBorder(true).
		SetBorderColor(tcell.ColorSteelBlue)

	outputText := tview.NewTextView()
	outputText.
		SetScrollable(true).
		SetTitle("CONSOLE").
		SetBorder(true)

	previewFlexBox := tview.NewFlex()
	previewFlexBox.SetDirection(tview.FlexRow)
	previewFlexBox.AddItem(grid, 1, 0, false)
	previewFlexBox.AddItem(showFlex, 0, 3, false)
	previewFlexBox.AddItem(outputText, 0, 1, false)

	previewText := tview.NewTextView()
	previewText.
		SetDynamicColors(true).
		SetRegions(true)

	previewTable := tview.NewTable()
	style := tcell.Style{}
	previewTable.SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetFixed(1, 1).
		SetSelectedStyle(style.Foreground(tcell.ColorWhite).Background(tcell.ColorBlue).Attributes(tcell.AttrBold)).
		SetEvaluateAllRows(true)
	p := &Preview{
		flexBox:   previewFlexBox,
		textView:  previewText,
		table:     previewTable,
		output:    outputText,
		showFlex:  showFlex,
		pageDelta: 1000,
		sizeText:  sizeText,
		delBtn:    delBtn,
		reloadBtn: reloadBtn,
		renameBtn: renameBtn,
		keyInput:  keyInput,
		grid:      grid,
	}
	prevBtn.SetSelectedFunc(p.prevPage)
	nextBtn.SetSelectedFunc(p.nextPage)
	p.init()
	return p
}

func (p *Preview) init() {
	p.table.SetSelectionChangedFunc(func(row, column int) {
		Log("Preview Update List sel row[%v] colujn[%v]", row, column)
		if row <= 0 {
			return
		}
		page := p.pages[p.curPage]
		var size int
		switch page.data.(type) {
		case []KVText:
			h := page.data.([]KVText)
			if row-1 >= len(h) {
				return
			}
			size = len(h[row-1].Value)
		case []string:
			h := page.data.([]string)
			if row-1 >= len(h) {
				return
			}
			size = len(h[row-1])
		}
		p.setSizeText(fmt.Sprintf("Size: %d bytes", size))
	})
}

// SetContent set
func (p *Preview) SetContent(o interface{}, valid bool) {
	p.setSizeText("")
	p.pages = p.pages[:0]
	switch o.(type) {
	case string:
		p.pages = append(p.pages, page{
			data: o,
		})
		text := o.(string)
		if valid {
			p.setSizeText(fmt.Sprintf("Size: %d bytes", len(text)))
		}
	case []KVText:
		h := o.([]KVText)
		pageCount := len(h) / p.pageDelta
		if len(h)%p.pageDelta > 0 {
			pageCount++
		}
		for i := 0; i < pageCount-1; i++ {
			p.pages = append(p.pages, page{
				data: h[i*p.pageDelta : (i+1)*p.pageDelta],
			})
		}
		p.pages = append(p.pages, page{
			data: h[(pageCount-1)*p.pageDelta:],
		})
	case []string:
		h := o.([]string)
		pageCount := len(h) / p.pageDelta
		if len(h)%p.pageDelta > 0 {
			pageCount++
		}
		for i := 0; i < (pageCount - 1); i++ {
			p.pages = append(p.pages, page{
				data: h[i*p.pageDelta : (i+1)*p.pageDelta],
			})
		}
		p.pages = append(p.pages, page{
			data: h[(pageCount-1)*p.pageDelta:],
		})
	}
	p.Update(0)
}

func (p *Preview) setSizeText(text string) {
	p.sizeText.SetText(text)
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

// Getkey return key
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
	o := page.data
	switch o.(type) {
	case string:
		p.showFlex.Clear()
		p.showFlex.AddItem(p.textView, 0, 1, false)
		p.textView.SetText(o.(string))
	case []KVText:
		p.showFlex.Clear()
		p.showFlex.AddItem(p.table, 0, 1, false)
		h := o.([]KVText)
		p.table.Clear()
		p.table.SetCell(0, 0, tview.NewTableCell("row").SetExpansion(1).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.table.SetCell(0, 1, tview.NewTableCell("key").SetExpansion(3).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.table.SetCell(0, 2, tview.NewTableCell("value").SetExpansion(24).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.table.Select(1, 1)
		p.table.ScrollToBeginning()

		for i, kv := range h {
			index := p.curPage*p.pageDelta + i + 1
			p.table.SetCell(i+1, 0, tview.NewTableCell(strconv.Itoa(index)))
			p.table.SetCell(i+1, 1, tview.NewTableCell(kv.Key))
			p.table.SetCell(i+1, 2, tview.NewTableCell(kv.Value))
		}
	case []string:
		p.showFlex.Clear()
		p.showFlex.AddItem(p.table, 0, 1, false)
		h := o.([]string)
		p.table.Clear()
		p.table.SetCell(0, 0, tview.NewTableCell("row").SetExpansion(1).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.table.SetCell(0, 1, tview.NewTableCell("value").SetExpansion(20).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.table.Select(1, 1)
		p.table.ScrollToBeginning()

		for i, v := range h {
			index := p.curPage*p.pageDelta + i + 1
			p.table.SetCell(i+1, 0, tview.NewTableCell(strconv.Itoa(index)))
			p.table.SetCell(i+1, 1, tview.NewTableCell(v))
		}
	}
}
