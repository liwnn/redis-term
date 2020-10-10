package redisterm

import (
	"strconv"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type page struct {
	data interface{}
}

// Preview preview
type Preview struct {
	flexBox *tview.Flex

	showFlex *tview.Flex
	textView *tview.TextView
	table    *tview.Table

	output *tview.TextView

	pages     []page
	pageDelta int
	curPage   int
}

// NewPreview new
func NewPreview() *Preview {
	prevBtn := tview.NewButton("<-")
	nextBtn := tview.NewButton("->")
	grid := tview.NewGrid().
		SetRows(-1).
		SetColumns(-5, 5, 5, -1).
		SetBorders(false).
		SetGap(0, 2).
		SetMinSize(5, 5)
	grid.AddItem(prevBtn, 0, 1, 1, 1, 0, 0, false) // 0行1列,占用1行1列(2则向后占一列)
	grid.AddItem(nextBtn, 0, 2, 1, 1, 0, 0, false)

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
	previewTable.SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetFixed(1, 1).
		SetSelectedStyle(tcell.ColorWhite, tcell.ColorBlue, tcell.AttrBold).
		SetEvaluateAllRows(true)

	p := &Preview{
		flexBox:   previewFlexBox,
		textView:  previewText,
		table:     previewTable,
		output:    outputText,
		showFlex:  showFlex,
		pageDelta: 1000,
	}
	prevBtn.SetSelectedFunc(p.prevPage)
	nextBtn.SetSelectedFunc(p.nextPage)
	return p
}

// SetContent set
func (p *Preview) SetContent(o interface{}) {
	p.pages = p.pages[:0]
	switch o.(type) {
	case string:
		p.pages = append(p.pages, page{
			data: o,
		})
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
