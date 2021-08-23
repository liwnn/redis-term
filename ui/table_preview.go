package ui

import (
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Row []string

// TablePageTitle title
type TablePageTitle struct {
	Name      string
	Expansion int
}

type TablePreview struct {
	*tview.Flex
	table   *tview.Table
	prevBtn *tview.Button
	nextBtn *tview.Button
	numView *tview.TextView

	pageDelta int

	title     []TablePageTitle
	rows      []Row
	totalPage int
	curPage   int
}

func NewTablePreview() *TablePreview {
	p := &TablePreview{
		pageDelta: 1000,
	}
	p.init()
	return p
}

func (p *TablePreview) init() {
	// left table
	style := tcell.Style{}
	table := tview.NewTable()
	table.SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetFixed(1, 1).
		SetSelectedStyle(style.Foreground(tcell.ColorWhite).
			Background(tcell.ColorDarkSlateGrey).
			Attributes(tcell.AttrBold)).
		SetEvaluateAllRows(true)
	table.SetBorder(true)

	// button
	prevBtn := tview.NewButton("◀")
	prevBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	nextBtn := tview.NewButton("▶")
	nextBtn.SetBackgroundColor(tcell.ColorDarkSlateGrey)
	opGrid := tview.NewGrid().SetRows(1).SetColumns(5, 5, -1).
		SetBorders(false).SetGap(0, 2)
	opGrid.AddItem(prevBtn, 0, 0, 1, 1, 0, 0, false)
	opGrid.AddItem(nextBtn, 0, 1, 1, 1, 0, 0, false)

	// right box
	numView := tview.NewTextView()
	ctrlBox := tview.NewFlex().SetDirection(tview.FlexRow)
	ctrlBox.AddItem(nil, 0, 1, false)
	ctrlBox.AddItem(numView, 1, 0, false)
	ctrlBox.AddItem(nil, 1, 1, false)
	ctrlBox.AddItem(opGrid, 1, 0, false)
	ctrlBox.AddItem(nil, 1, 1, false)

	// flex
	flex := tview.NewFlex().SetDirection(tview.FlexColumn)
	flex.AddItem(table, 0, 1, false)
	flex.AddItem(nil, 1, 0, false)
	flex.AddItem(ctrlBox, 13, 0, false)

	p.Flex = flex
	p.table = table
	p.nextBtn = nextBtn
	p.prevBtn = prevBtn
	p.numView = numView

	prevBtn.SetSelectedFunc(p.prevPage)
	nextBtn.SetSelectedFunc(p.nextPage)
}

func (p *TablePreview) nextPage() {
	if p.curPage+1 >= p.totalPage {
		return
	}
	p.Show(p.curPage + 1)
}

func (p *TablePreview) prevPage() {
	if p.curPage == 0 {
		return
	}
	p.Show(p.curPage - 1)
}

func (p *TablePreview) Update(title []TablePageTitle, rows []Row) {
	p.title = title
	p.rows = rows

	pageCount := len(rows) / p.pageDelta
	if len(rows)%p.pageDelta > 0 {
		pageCount++
	}
	p.totalPage = pageCount
	p.Show(0)

	p.numView.SetText("Count:" + strconv.Itoa(len(rows)))
}

func (p *TablePreview) Show(pageNum int) {
	p.curPage = pageNum
	p.table.Clear()
	for i, v := range p.title {
		p.table.SetCell(0, i, tview.NewTableCell(v.Name).SetExpansion(v.Expansion).SetSelectable(false).SetTextColor(tcell.ColorYellow))
	}
	p.table.Select(1, 1)
	p.table.ScrollToBeginning()

	var begin = p.pageDelta * pageNum
	var end = begin + p.pageDelta - 1
	if end >= len(p.rows) {
		end = len(p.rows) - 1
	}
	for i := begin; i <= end; i++ {
		row := p.rows[i]
		showIndex := i - begin + 1
		p.table.SetCell(showIndex, 0, tview.NewTableCell(strconv.Itoa(i+1)))
		for j, c := range row {
			if len(c) > 1024 {
				c = c[:1024]
			}
			p.table.SetCell(showIndex, j+1, tview.NewTableCell(c))
		}
	}
}

func (p *TablePreview) SetSelectionChangedFunc(handler func(row, column int)) {
	p.table.SetSelectionChangedFunc(handler)
}
