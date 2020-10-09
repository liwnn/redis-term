package redisterm

import (
	"strconv"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

// Preview preview
type Preview struct {
	flexBox *tview.Flex

	showFlex *tview.Flex
	textView *tview.TextView
	table    *tview.Table

	output *tview.TextView
}

// NewPreview new
func NewPreview() *Preview {
	button := tview.NewButton("hello")

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
	previewFlexBox.AddItem(button, 3, 0, false)
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
		flexBox:  previewFlexBox,
		textView: previewText,
		table:    previewTable,
		output:   outputText,
		showFlex: showFlex,
	}
	return p
}

// SetContent set
func (p *Preview) SetContent(o interface{}) {
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
			p.table.SetCell(i+1, 0, tview.NewTableCell(strconv.Itoa(i+1)))
			p.table.SetCell(i+1, 1, tview.NewTableCell(kv.Key))
			p.table.SetCell(i+1, 2, tview.NewTableCell(kv.Value))
		}
	case []string:
		p.flexBox.Clear()
		p.flexBox.AddItem(p.table, 0, 1, false)
		h := o.([]string)
		p.table.Clear()
		p.table.SetCell(0, 0, tview.NewTableCell("row").SetExpansion(1).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.table.SetCell(0, 1, tview.NewTableCell("value").SetExpansion(20).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.table.Select(1, 1)
		p.table.ScrollToBeginning()

		for i, v := range h {
			p.table.SetCell(i+1, 0, tview.NewTableCell(strconv.Itoa(i+1)))
			p.table.SetCell(i+1, 1, tview.NewTableCell(v))
		}
	}
}
