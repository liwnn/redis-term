package redisterm

import (
	"strconv"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

// Preview preview
type Preview struct {
	flexBox      *tview.Flex
	previewText  *tview.TextView
	previewTable *tview.Table
	outputText   *tview.TextView
}

// NewPreview new
func NewPreview() *Preview {
	previewFlexBox := tview.NewFlex()
	previewFlexBox.SetDirection(tview.FlexRow)
	previewText := tview.NewTextView()
	previewText.
		SetDynamicColors(true).
		SetRegions(true).
		SetScrollable(true).
		SetTitle("PREVIEW").
		SetBorder(true).
		SetBorderColor(tcell.ColorSteelBlue)

	previewTable := tview.NewTable()
	previewTable.SetBorders(false).
		SetSelectable(true, false).
		SetSeparator(' ').
		SetFixed(1, 1).
		SetSelectedStyle(tcell.ColorWhite, tcell.ColorBlue, tcell.AttrBold).
		SetEvaluateAllRows(true)
	previewTable.
		SetTitle("PREVIEW").
		SetBorder(true).
		SetBorderColor(tcell.ColorSteelBlue)

	outputText := tview.NewTextView()
	SetLogger(outputText)
	outputText.SetScrollable(true).SetTitle("CONSOLE").SetBorder(true)

	p := &Preview{
		flexBox:      previewFlexBox,
		previewText:  previewText,
		previewTable: previewTable,
		outputText:   outputText,
	}
	return p
}

// SetContent set
func (p *Preview) SetContent(o interface{}) {
	switch o.(type) {
	case string:
		p.flexBox.Clear()
		p.flexBox.AddItem(p.previewText, 0, 3, false)
		p.flexBox.AddItem(p.outputText, 0, 1, false)
		p.previewText.SetText(o.(string))
	case []KVText:
		p.flexBox.Clear()
		p.flexBox.AddItem(p.previewTable, 0, 3, false)
		p.flexBox.AddItem(p.outputText, 0, 1, false)
		h := o.([]KVText)
		p.previewTable.Clear()
		p.previewTable.SetCell(0, 0, tview.NewTableCell("row").SetExpansion(1).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.previewTable.SetCell(0, 1, tview.NewTableCell("key").SetExpansion(3).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.previewTable.SetCell(0, 2, tview.NewTableCell("value").SetExpansion(24).SetSelectable(false).SetTextColor(tcell.ColorYellow))
		p.previewTable.Select(1, 1)
		p.previewTable.ScrollToBeginning()

		for i, kv := range h {
			p.previewTable.SetCell(i+1, 0, tview.NewTableCell(strconv.Itoa(i+1)))
			p.previewTable.SetCell(i+1, 1, tview.NewTableCell(kv.Key))
			p.previewTable.SetCell(i+1, 2, tview.NewTableCell(kv.Value))
		}
	}
}
