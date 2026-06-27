package view

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// maxCellWidth is the absolute ceiling for a column's width when the user
// widens it manually with '+'. It is not the default — see defaultColWidth.
const maxCellWidth = 160

// defaultColWidth is each column's on-screen width before any manual override.
// It is deliberately modest so that, with many columns, the leading columns all
// fit within the pane and render at a stable readable width instead of being
// squeezed to a sliver (which also made tview re-expand the selected column,
// causing the "widen on select" jump). Sparse long-valued columns (e.g. a JSON
// blob present in only a few rows) thus show a useful preview by default.
// Overflow is still truncated by displayText; '+'/'-' adjust per column.
const defaultColWidth = 28

// colWidthStep is how much '+'/'-' adjust the selected column's width.
const colWidthStep = 16

// minCellWidth is the floor for a manually narrowed column.
const minCellWidth = 8

type Row []string

// CellEdit is a single staged table cell change (absolute data coords).
type CellEdit struct {
	Row   int
	Col   int
	Value string
}

// TablePageTitle title
type TablePageTitle struct {
	Name      string
	Expansion int
}

type TablePreview struct {
	*tview.Flex // outer column: inner | status row | editor row
	inner       *tview.Flex
	table       *tview.Table
	editor      *tview.InputField
	statusRow    *tview.Flex // status text + cell info + pagination + action buttons
	actionRow    *tview.Flex // trailing sub-flex holding delete/commit/rollback
	statusView   *tview.TextView
	cellInfoView *tview.TextView // right-aligned "Type/Size" of the selected cell
	delBtn      *tview.Button
	commitBtn   *tview.Button
	rollbackBtn *tview.Button
	prevBtn     *tview.Button
	nextBtn     *tview.Button

	pageDelta int

	title     []TablePageTitle
	rows      []Row
	cellTypes [][]string
	totalPage int
	curPage   int

	editing bool
	editRow int
	editCol int

	hlRow          int // screen row currently row-highlighted (-1 = none)
	userSelChanged func(row, column int)

	pending   map[[2]int]string // staged edits keyed by [absRow, dataCol]
	colWidths map[int]int        // per-data-column max-width overrides (dataCol -> width)

	// rowSelectable enables per-row checkboxes (☐/☑) in the row-number column for
	// backends that support row deletion (mongo/mysql). selected holds the ticked
	// absolute row indices; onDeleteRows commits a deletion of those rows.
	rowSelectable bool
	selected      map[int]bool
	onDeleteRows  func(absRows []int) error
	confirm       func(text string, okFunc func())

	// entryType is the entry's backend type (redis hash/zset/…, mongo collection,
	// mysql table), shown on the left of the status row before the row count. The
	// selected cell's "Type: X  Size: Y" is shown separately in cellInfoView.
	entryType string

	// stretchCol is the data column allowed to grow into leftover horizontal
	// space when the table doesn't fill its pane: the widest-content column (a
	// long value/score/fields column). -1 = none (e.g. the user widened a column
	// manually, so we stop auto-stretching).
	stretchCol int
	// stretchWidth is the stretch column's current truncation width, measured
	// from the real pane width on each draw (0 until first measured). The stretch
	// column IS truncated to this width — not left full — so selecting a long
	// cell can't trigger tview's clamp-to-selection horizontal scroll, which
	// would push other columns past the right edge of the pane.
	stretchWidth int
	// colContentWidth caches each data column's rendered width (max of header and
	// cell content, capped at colWidth), computed once per Update. measureStretch
	// reserves these so the stretch column gets all the genuinely-unused width.
	colContentWidth []int
	onCommit        func(edits []CellEdit) error
	onReload        func()
	focus           func(p tview.Primitive)
}

// colWidth returns the on-screen max-width cap for a data column: a manual
// override (set via '+'/'-') wins, otherwise a fixed default. It must NOT depend
// on the table's drawn size — during Update→Show the pane rect is still 0, which
// would collapse every column to the floor and then visibly "jump wide" once the
// pane gets its real size and tview re-clamps the selected column. A constant cap
// keeps widths stable across redraws; tview still sizes each column to its actual
// content up to this cap, so short columns stay snug.
func (p *TablePreview) colWidth(dataCol int) int {
	if w, ok := p.colWidths[dataCol]; ok {
		return w
	}
	return defaultColWidth
}

// contentWidth returns a data column's cached rendered width (header/content,
// capped at colWidth). Falls back to colWidth before the cache is built.
func (p *TablePreview) contentWidth(dataCol int) int {
	if dataCol >= 0 && dataCol < len(p.colContentWidth) {
		return p.colContentWidth[dataCol]
	}
	return p.colWidth(dataCol)
}

// computeContentWidths fills colContentWidth: for each data column, the widest
// of its header and its cell values (sampling up to 200 rows), each capped at
// colWidth and padded by cellPad to match the rendered text. The stretch column
// is left at 0 — it is sized from leftover space, not reserved.
func (p *TablePreview) computeContentWidths() {
	dataCols := len(p.title) - 1
	p.colContentWidth = make([]int, dataCols)
	if dataCols <= 0 {
		return
	}
	n := len(p.rows)
	if n > 200 {
		n = 200
	}
	for j := 0; j < dataCols; j++ {
		if j == p.stretchCol {
			continue
		}
		w := len([]rune(p.title[j+1].Name))
		for i := 0; i < n; i++ {
			if j < len(p.rows[i]) {
				if l := len([]rune(p.rows[i][j])); l > w {
					w = l
				}
			}
		}
		w += len(cellPad) // cells carry a left pad; headers (i>=1) do too
		if cap := p.colWidth(j) + len(cellPad); w > cap {
			w = cap
		}
		p.colContentWidth[j] = w
	}
}

// cellMaxWidth is the tview MaxWidth cap for a data column. For the stretch
// column it is the measured remaining pane width (so the column fills the row);
// for all others it's the normal colWidth. Returning the exact remaining width
// keeps tview's per-column clamp equal to the cell's own width, which avoids the
// clamp-to-selection horizontal scroll that would push columns off-screen.
func (p *TablePreview) cellMaxWidth(dataCol int) int {
	if dataCol == p.stretchCol && p.stretchWidth > 0 {
		return p.stretchWidth
	}
	return p.colWidth(dataCol)
}

// measureStretch recomputes the stretch column's truncation width from the
// table's real inner width (paneWidth), reserving the row-number column, every
// non-stretch data column at its colWidth, and one space between columns. When
// the result changes it repaints so the stretch column tracks pane resizes. It
// never lets the stretch column exceed the leftover width, so the table as a
// whole stays within the pane.
func (p *TablePreview) measureStretch(paneWidth int) {
	if p.stretchCol < 0 || paneWidth <= 0 {
		return
	}
	dataCols := len(p.title) - 1
	if dataCols <= 0 {
		return
	}
	// Row-number column width: digits of the largest row number, +1 separator.
	used := len(strconv.Itoa(len(p.rows))) + 1
	for j := 0; j < dataCols; j++ {
		if j == p.stretchCol {
			continue
		}
		// Reserve each non-stretch column its ACTUAL rendered width (content and
		// header, capped at colWidth, pad already included), not the fixed cap —
		// tview sizes columns to content, so reserving the cap would starve the
		// stretch column and truncate it even when there is plenty of room. The
		// +1 is tview's single-space column separator.
		used += p.contentWidth(j) + 1
	}
	avail := paneWidth - used - len(cellPad)
	// Floor at the normal column width, not minCellWidth: when the other columns
	// already overflow the pane there is no leftover space, and clamping to a tiny
	// width would make the stretch column narrower than a plain long column. At
	// defaultColWidth it shows as much as any other column and the table simply
	// scrolls horizontally.
	if avail < defaultColWidth {
		avail = defaultColWidth
	}
	if avail > maxCellWidth {
		avail = maxCellWidth
	}
	if avail == p.stretchWidth {
		return
	}
	p.stretchWidth = avail
	// This runs from the table's draw func, before its cells are rendered this
	// same frame (Table.Draw calls the box draw func first), so updating the
	// stretch column's cells in place takes effect immediately — no extra redraw.
	p.repaintStretchColumn()
}

// repaintStretchColumn re-renders just the stretch column's cells (text + width
// cap) for the visible page, in place, so a pane resize doesn't disturb the
// current selection or scroll position the way a full Show() would.
func (p *TablePreview) repaintStretchColumn() {
	j := p.stretchCol
	if j < 0 {
		return
	}
	begin := p.pageDelta * p.curPage
	end := begin + p.pageDelta - 1
	if end >= len(p.rows) {
		end = len(p.rows) - 1
	}
	if c := p.table.GetCell(0, j+1); c != nil {
		c.SetMaxWidth(p.cellMaxWidth(j))
	}
	for i := begin; i <= end; i++ {
		if j >= len(p.rows[i]) {
			continue
		}
		showIndex := i - begin + 1
		if c := p.table.GetCell(showIndex, j+1); c != nil {
			c.SetText(p.displayText(p.rows[i][j], j)).SetMaxWidth(p.cellMaxWidth(j))
		}
	}
}

// displayText truncates a cell's text to its column width (plus an ellipsis)
// so the cell never holds more than fits. This keeps column widths stable when
// a long-valued cell is selected: tview's clamp-to-selection has nothing extra
// to expand. The full value still lives in p.rows for editing.
// cellPad is a one-space left inset added to every data cell's display text so
// adjacent columns don't visually touch (tview's column separator is a single
// space, which reads as cramped for dense values).
const cellPad = "  "

// widestColumn returns the data-column index whose content is widest on average,
// sampling up to the first 200 rows. That column is the auto-stretch target so a
// long value/score/fields column fills leftover pane width. Returns -1 when there
// are no rows or columns.
func widestColumn(title []TablePageTitle, rows []Row) int {
	cols := len(title) - 1 // first title is the row-number column
	if cols <= 0 || len(rows) == 0 {
		return -1
	}
	sums := make([]int, cols)
	n := len(rows)
	if n > 200 {
		n = 200
	}
	for i := 0; i < n; i++ {
		for j, c := range rows[i] {
			if j < cols {
				sums[j] += len([]rune(c))
			}
		}
	}
	best, bestSum := 0, -1
	for j, s := range sums {
		if s > bestSum {
			best, bestSum = j, s
		}
	}
	return best
}

func (p *TablePreview) displayText(s string, dataCol int) string {
	w := p.colWidth(dataCol)
	if dataCol == p.stretchCol && p.stretchWidth > 0 {
		// Truncate the stretch column to the measured remaining pane width so it
		// fills the row without ever overflowing it.
		w = p.stretchWidth
	}
	r := []rune(s)
	if len(r) <= w {
		return cellPad + s
	}
	if w <= 1 {
		return cellPad + string(r[:w])
	}
	return cellPad + string(r[:w-1]) + "…"
}

func NewTablePreview() *TablePreview {
	p := &TablePreview{
		pageDelta: 1000,
		pending:   make(map[[2]int]string),
		colWidths: make(map[int]int),
		selected:  make(map[int]bool),
		hlRow:     -1,
	}
	p.init()
	return p
}

func (p *TablePreview) init() {
	// left table
	style := tcell.Style{}
	table := tview.NewTable()
	table.SetBorders(false).
		SetSelectable(true, true).
		SetSeparator(' ').
		SetFixed(1, 1).
		SetSelectedStyle(style.Foreground(tcell.ColorWhite).
			Background(ThemeSelectBG).
			Attributes(tcell.AttrBold)).
		SetEvaluateAllRows(true)
	table.SetBorder(true)
	table.SetBackgroundColor(ThemePanelBG)
	focusBorder(table.Box)
	table.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		ix, iy, iw, ih := table.GetInnerRect()
		p.measureStretch(iw)
		return ix, iy, iw, ih
	})

	// pagination buttons (live in the status row, always visible)
	prevBtn := tview.NewButton("◀")
	prevBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	prevBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	nextBtn := tview.NewButton("▶")
	nextBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	nextBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))

	// inner flex: just the table (full width now that pagination moved to the
	// status row).
	inner := tview.NewFlex().SetDirection(tview.FlexColumn)
	inner.AddItem(table, 0, 1, false)

	// inline editor (hidden until editing)
	editor := tview.NewInputField()
	editor.SetLabel("Edit: ").
		SetLabelColor(ThemeQueryLabel).
		SetFieldBackgroundColor(ThemeQueryBG).
		SetFieldTextColor(ThemeQueryFG)

	// status row: always-visible text (row count / edit / selection hints) plus
	// action buttons that appear by state — Delete on selection, Commit/Rollback
	// on staged edits. Mirrors the web toolbar.
	statusView := tview.NewTextView()
	statusView.SetDynamicColors(true).SetTextColor(tcell.ColorYellow)
	statusView.SetBackgroundColor(ThemePanelBG)

	// cell info ("Type: …  Size: …") for the selected cell, right-aligned just
	// before the pagination buttons.
	cellInfoView := tview.NewTextView()
	cellInfoView.SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	cellInfoView.SetBackgroundColor(ThemePanelBG)

	delBtn := tview.NewButton("Delete selected")
	delBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnDangerBG).Foreground(ThemeBtnDangerFG))
	delBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnDangerHoverBG).Foreground(tcell.ColorWhite))
	commitBtn := tview.NewButton("Commit")
	commitBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	commitBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	rollbackBtn := tview.NewButton("Rollback")
	rollbackBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	rollbackBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))

	// status row (above the table, below the query box), left→right: status text
	// (flexes to push the rest right), pagination ◀ ▶ (always shown), then the
	// state-dependent action buttons (Delete / Commit / Rollback). The action
	// buttons live in a trailing sub-flex so updateStatus can repopulate just them
	// without disturbing the always-present pagination.
	actionRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	actionRow.SetBackgroundColor(ThemePanelBG)

	// Left→right: status text (sized to its content) immediately followed by the
	// action buttons (Delete / Commit / Rollback), then cell info filling the gap
	// right-aligned, then pagination ◀ ▶. statusView and actionRow widths are set
	// per-update by updateStatus.
	statusRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	statusRow.SetBackgroundColor(ThemePanelBG)
	statusRow.AddItem(statusView, 0, 0, false)   // width set by updateStatus
	statusRow.AddItem(actionRow, 0, 0, false)     // width set by updateStatus
	statusRow.AddItem(cellInfoView, 0, 1, false)  // fills middle; text right-aligned
	statusRow.AddItem(nil, 2, 0, false)
	statusRow.AddItem(prevBtn, 3, 0, false)
	statusRow.AddItem(nil, 1, 0, false)
	statusRow.AddItem(nextBtn, 3, 0, false)

	// outer flex: status row (above the table, below the query box) + inner table
	// area + editor row. The status row is always present; the editor row is added
	// on demand at the bottom while editing.
	outer := tview.NewFlex().SetDirection(tview.FlexRow)
	outer.SetBackgroundColor(ThemePanelBG)
	inner.SetBackgroundColor(ThemePanelBG)
	statusView.SetBackgroundColor(ThemePanelBG)
	outer.AddItem(statusRow, 1, 0, false)
	outer.AddItem(inner, 0, 1, false)

	p.Flex = outer
	p.inner = inner
	p.table = table
	p.editor = editor
	p.statusRow = statusRow
	p.actionRow = actionRow
	p.statusView = statusView
	p.cellInfoView = cellInfoView
	p.delBtn = delBtn
	p.commitBtn = commitBtn
	p.rollbackBtn = rollbackBtn
	p.nextBtn = nextBtn
	p.prevBtn = prevBtn

	prevBtn.SetSelectedFunc(p.prevPage)
	nextBtn.SetSelectedFunc(p.nextPage)
	delBtn.SetSelectedFunc(p.deleteSelected)
	commitBtn.SetSelectedFunc(p.commit)
	rollbackBtn.SetSelectedFunc(p.rollback)
	table.SetInputCapture(p.onTableKey)
	editor.SetDoneFunc(p.onEditorDone)
	// Losing focus while editing (e.g. clicking elsewhere) cancels the edit
	// without staging — same as pressing Esc. DoneFunc only fires on Enter/Tab/Esc,
	// so a click away would otherwise leave the editor open.
	editor.SetBlurFunc(func() {
		if p.editing {
			p.cancelEdit()
		}
	})
	table.SetSelectionChangedFunc(p.onSelectionChanged)
}

// onSelectionChanged repaints the row band so the row containing the selected
// cell is tinted, then forwards to any externally-registered handler.
func (p *TablePreview) onSelectionChanged(row, column int) {
	p.highlightRow(row)
	if p.userSelChanged != nil {
		p.userSelChanged(row, column)
	}
}

// rowBG returns the background a data cell should carry given the current
// highlighted row: dirty cells always win (yellow), then the highlighted row
// band, otherwise the panel bg.
func (p *TablePreview) rowBG(absRow, dataCol, showRow int) tcell.Color {
	if _, dirty := p.pending[[2]int{absRow, dataCol}]; dirty {
		return tcell.ColorYellow
	}
	if showRow == p.hlRow {
		return ThemeRowHighlightBG
	}
	return ThemePanelBG
}

// highlightRow moves the row band to showRow, repainting the cells of the old
// and new rows. The selected-cell style (drawn by tview on top) still marks the
// exact cell.
func (p *TablePreview) highlightRow(showRow int) {
	if showRow == p.hlRow {
		return
	}
	prev := p.hlRow
	p.hlRow = showRow
	p.repaintRowBG(prev)
	p.repaintRowBG(showRow)
}

// repaintRowBG restores the background of every data cell in a screen row to
// match the current highlight/dirty state.
func (p *TablePreview) repaintRowBG(showRow int) {
	if showRow <= 0 {
		return
	}
	absRow := p.curPage*p.pageDelta + showRow - 1
	if absRow < 0 || absRow >= len(p.rows) {
		return
	}
	if idx := p.table.GetCell(showRow, 0); idx != nil {
		if showRow == p.hlRow {
			idx.SetBackgroundColor(ThemeRowHighlightBG)
		} else {
			idx.SetBackgroundColor(ThemePanelBG)
		}
	}
	for dataCol := range p.rows[absRow] {
		cell := p.table.GetCell(showRow, dataCol+1)
		if cell == nil {
			continue
		}
		cell.SetBackgroundColor(p.rowBG(absRow, dataCol, showRow))
	}
}

// onTableKey handles in-table keys: Enter edits a cell (staging it locally),
// Ctrl-S commits all staged edits, Esc rolls them back. Delete (or vim 'd')
// removes the selected rows, matching the tree's drop binding.
func (p *TablePreview) onTableKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlS:
		p.commit()
		return nil
	case tcell.KeyDelete:
		if p.rowSelectable && len(p.selected) > 0 {
			p.deleteSelected()
			return nil
		}
		return event
	case tcell.KeyEsc:
		if len(p.pending) > 0 {
			p.rollback()
			return nil
		}
		if len(p.selected) > 0 {
			p.clearSelection()
			return nil
		}
		return event
	case tcell.KeyEnter:
		if p.editing {
			return nil // an editor is already open; ignore repeat Enters
		}
		r, c := p.table.GetSelection()
		if r <= 0 || c <= 0 {
			return event
		}
		if p.onCommit == nil || p.focus == nil {
			return event
		}
		// edit the full (untruncated) value from p.rows, not the cell's
		// possibly-elided display text.
		absRow := p.curPage*p.pageDelta + r - 1
		dataCol := c - 1
		if absRow < 0 || absRow >= len(p.rows) || dataCol < 0 || dataCol >= len(p.rows[absRow]) {
			return event
		}
		p.startEdit(r, c, p.rows[absRow][dataCol])
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case ' ': // toggle row selection
			if p.rowSelectable {
				p.toggleSelectedRow()
				return nil
			}
		case 'd': // vim alias for Delete: drop selected rows
			if p.rowSelectable && len(p.selected) > 0 {
				p.deleteSelected()
				return nil
			}
		case '+', '=': // widen selected column
			p.resizeSelectedCol(colWidthStep)
			return nil
		case '-', '_': // narrow selected column
			p.resizeSelectedCol(-colWidthStep)
			return nil
		}
	}
	return event
}

// resizeSelectedCol adjusts the selected data column's max width by delta and
// repaints the table.
func (p *TablePreview) resizeSelectedCol(delta int) {
	_, c := p.table.GetSelection()
	dataCol := c - 1
	if dataCol < 0 {
		return
	}
	w := p.colWidth(dataCol) + delta
	if w < minCellWidth {
		w = minCellWidth
	}
	if w > maxCellWidth {
		w = maxCellWidth
	}
	p.colWidths[dataCol] = w
	if dataCol == p.stretchCol {
		// A manual width on the stretch column wins; drop auto-stretch so the
		// fixed cap (and truncation) take effect for it.
		p.stretchCol = -1
	}
	// Recompute reserved widths (this column's cap changed) and force a re-measure
	// so the stretch column re-absorbs any freed/taken space.
	p.computeContentWidths()
	p.stretchWidth = 0
	selRow, selCol := p.table.GetSelection()
	p.Show(p.curPage)
	p.table.Select(selRow, selCol)
}

// toggleSelectedRow flips the ticked state of the row holding the current
// selection (Space key path).
func (p *TablePreview) toggleSelectedRow() {
	r, _ := p.table.GetSelection()
	if r <= 0 {
		return
	}
	p.toggleRow(p.curPage*p.pageDelta + r - 1)
}

// toggleRow flips the ticked state of the given absolute row, repaints its
// row-number cell, and refreshes the status line. Shared by the Space key and
// the mouse-click handler on the checkbox cell.
func (p *TablePreview) toggleRow(absRow int) {
	if absRow < 0 || absRow >= len(p.rows) {
		return
	}
	if p.selected[absRow] {
		delete(p.selected, absRow)
	} else {
		p.selected[absRow] = true
	}
	showRow := absRow - p.curPage*p.pageDelta + 1
	if c := p.table.GetCell(showRow, 0); c != nil {
		c.SetText(p.idxText(absRow))
	}
	p.updateStatus()
}

// clearSelection unticks all rows and repaints the visible row-number cells.
func (p *TablePreview) clearSelection() {
	if len(p.selected) == 0 {
		return
	}
	p.selected = make(map[int]bool)
	begin := p.pageDelta * p.curPage
	end := begin + p.pageDelta - 1
	if end >= len(p.rows) {
		end = len(p.rows) - 1
	}
	for i := begin; i <= end; i++ {
		if c := p.table.GetCell(i-begin+1, 0); c != nil {
			c.SetText(p.idxText(i))
		}
	}
	p.updateStatus()
}

// deleteSelected confirms, then deletes the ticked rows via onDeleteRows.
func (p *TablePreview) deleteSelected() {
	if p.onDeleteRows == nil || len(p.selected) == 0 {
		return
	}
	absRows := make([]int, 0, len(p.selected))
	for r := range p.selected {
		absRows = append(absRows, r)
	}
	sort.Ints(absRows)
	doDelete := func() {
		if err := p.onDeleteRows(absRows); err != nil {
			p.statusView.SetText(fmt.Sprintf("[red]delete failed: %v", err))
			return
		}
		p.selected = make(map[int]bool)
		p.updateStatus()
		if p.onReload != nil {
			p.onReload()
		}
	}
	if p.confirm != nil {
		p.confirm(fmt.Sprintf("Delete %d selected row(s)?", len(absRows)), doDelete)
		return
	}
	doDelete()
}

func (p *TablePreview) startEdit(r, c int, cur string) {
	p.editing = true
	p.editRow, p.editCol = r, c
	dataCol := c - 1
	label := "Edit: "
	if dataCol >= 0 && dataCol < len(p.title)-1 {
		label = p.title[dataCol+1].Name + ": "
	}
	p.editor.SetLabel(label)
	p.editor.SetText(cur)
	p.Flex.RemoveItem(p.editor) // ensure exactly one editor row, never stacked
	p.Flex.AddItem(p.editor, 1, 0, true)
	p.focus(p.editor)
}

// onEditorDone stages (not commits) the edited value on Enter.
func (p *TablePreview) onEditorDone(key tcell.Key) {
	if !p.editing {
		return
	}
	if key == tcell.KeyEnter {
		absRow := p.curPage*p.pageDelta + p.editRow - 1
		dataCol := p.editCol - 1
		p.stage(absRow, dataCol, p.editor.GetText())
	}
	p.endEdit()
}

func (p *TablePreview) endEdit() {
	p.editing = false
	p.Flex.RemoveItem(p.editor)
	if p.focus != nil {
		p.focus(p.table)
	}
}

// cancelEdit closes the editor without staging the value (the editor lost focus,
// e.g. a click landed elsewhere). It does NOT grab focus back to the table, since
// focus is intentionally moving away.
func (p *TablePreview) cancelEdit() {
	p.editing = false
	p.Flex.RemoveItem(p.editor)
}

// stage records a pending edit and repaints the affected cell as dirty.
func (p *TablePreview) stage(absRow, dataCol int, value string) {
	p.pending[[2]int{absRow, dataCol}] = value
	if absRow >= 0 && absRow < len(p.rows) && dataCol >= 0 && dataCol < len(p.rows[absRow]) {
		p.rows[absRow][dataCol] = value
	}
	p.paintCell(absRow, dataCol)
	p.updateStatus()
}

// paintCell refreshes one on-screen cell to reflect staged/clean state.
func (p *TablePreview) paintCell(absRow, dataCol int) {
	showRow := absRow - p.curPage*p.pageDelta + 1
	if showRow <= 0 {
		return
	}
	if absRow < 0 || absRow >= len(p.rows) || dataCol < 0 || dataCol >= len(p.rows[absRow]) {
		return
	}
	cell := tview.NewTableCell(p.displayText(p.rows[absRow][dataCol], dataCol)).SetMaxWidth(p.colWidth(dataCol))
	if _, dirty := p.pending[[2]int{absRow, dataCol}]; dirty {
		cell.SetTextColor(tcell.ColorBlack)
	}
	cell.SetBackgroundColor(p.rowBG(absRow, dataCol, showRow))
	p.table.SetCell(showRow, dataCol+1, cell)
}

// updateStatus refreshes the always-visible status row, mirroring the web
// toolbar: staged edits take precedence (Commit/Rollback buttons), then a row
// selection (Delete button), otherwise the idle row count. The status text is
// sized to its content so the action buttons sit immediately after it.
func (p *TablePreview) updateStatus() {
	p.actionRow.Clear()
	statusText := ""
	actionWidth := 0
	if n := len(p.pending); n > 0 {
		statusText = fmt.Sprintf("[yellow]%d pending edit(s)  [white]Ctrl-S[yellow]=commit  [white]Esc[yellow]=rollback", n)
		p.actionRow.AddItem(nil, 1, 0, false)
		p.actionRow.AddItem(p.commitBtn, 10, 0, false)
		p.actionRow.AddItem(nil, 1, 0, false)
		p.actionRow.AddItem(p.rollbackBtn, 12, 0, false)
		actionWidth = 24
	} else if n := len(p.selected); n > 0 {
		statusText = fmt.Sprintf("[yellow]%d row(s) selected  [white]Del[yellow]=delete", n)
		label := fmt.Sprintf("Delete selected (%d)", n)
		p.delBtn.SetLabel(label)
		p.actionRow.AddItem(nil, 1, 0, false)
		p.actionRow.AddItem(p.delBtn, len(label)+2, 0, false)
		actionWidth = len(label) + 3
	} else {
		statusText = fmt.Sprintf("[gray]%d row%s", len(p.rows), plural(len(p.rows)))
		if p.entryType != "" {
			statusText = fmt.Sprintf("[gray]Type: %s   %d row%s", p.entryType, len(p.rows), plural(len(p.rows)))
		}
	}
	p.statusView.SetText(statusText)
	// Resize statusView to its visible text (tags stripped) + a small pad, and
	// the action slot to its buttons, so the buttons sit right after the text.
	p.statusRow.ResizeItem(p.statusView, plainWidth(statusText)+2, 0)
	p.statusRow.ResizeItem(p.actionRow, actionWidth, 0)
}

// plainWidth returns the rune width of s with tview color tags ([...]) removed.
func plainWidth(s string) int {
	n, inTag := 0, false
	for _, r := range s {
		switch {
		case r == '[':
			inTag = true
		case r == ']':
			inTag = false
		case !inTag:
			n++
		}
	}
	return n
}

// SetEntryType sets the entry's backend type shown on the left of the idle
// status line (redis hash/zset, mongo collection, mysql table).
func (p *TablePreview) SetEntryType(t string) {
	p.entryType = t
	p.updateStatus()
}

// plural returns "s" unless n == 1.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// commit flushes all staged edits via onCommit, then reloads on success.
func (p *TablePreview) commit() {
	if len(p.pending) == 0 || p.onCommit == nil {
		return
	}
	edits := make([]CellEdit, 0, len(p.pending))
	for k, v := range p.pending {
		edits = append(edits, CellEdit{Row: k[0], Col: k[1], Value: v})
	}
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].Row != edits[j].Row {
			return edits[i].Row < edits[j].Row
		}
		return edits[i].Col < edits[j].Col
	})
	if err := p.onCommit(edits); err != nil {
		p.statusView.SetText(fmt.Sprintf("[red]commit failed: %v  [white]Esc[red]=rollback", err))
		return
	}
	p.pending = make(map[[2]int]string)
	p.updateStatus()
	if p.onReload != nil {
		p.onReload()
	}
}

// rollback discards staged edits and restores the original values.
func (p *TablePreview) rollback() {
	p.pending = make(map[[2]int]string)
	p.updateStatus()
	if p.onReload != nil {
		p.onReload()
	}
}

// HasPending reports whether there are uncommitted staged edits.
func (p *TablePreview) HasPending() bool {
	return len(p.pending) > 0
}

// SetCommitFunc registers the callback invoked when staged edits are committed.
// Each CellEdit carries absolute data row/col and the new value.
func (p *TablePreview) SetCommitFunc(f func(edits []CellEdit) error) {
	p.onCommit = f
}

// SetReloadFunc registers the callback used to repaint after commit/rollback.
func (p *TablePreview) SetReloadFunc(f func()) {
	p.onReload = f
}

// SetFocusFunc injects the focus setter (typically Application.SetFocus).
func (p *TablePreview) SetFocusFunc(f func(p tview.Primitive)) {
	p.focus = f
}

// EnableRowSelection toggles per-row checkboxes in the row-number column.
func (p *TablePreview) EnableRowSelection(on bool) {
	p.rowSelectable = on
}

// SetDeleteRowsFunc registers the callback invoked (after confirmation) when the
// user deletes the ticked rows with Delete / 'd'. It receives sorted absolute rows.
func (p *TablePreview) SetDeleteRowsFunc(f func(absRows []int) error) {
	p.onDeleteRows = f
}

// SetConfirmFunc injects the confirm-dialog helper used before a row deletion.
func (p *TablePreview) SetConfirmFunc(f func(text string, okFunc func())) {
	p.confirm = f
}

// SetCellInfo sets the right-aligned selected-cell label (e.g. "Type: int
// Size: 4 bytes") shown in the status row.
func (p *TablePreview) SetCellInfo(text string) {
	p.cellInfoView.SetText(text)
}

// idxText renders the row-number cell text, prefixed with a checkbox glyph when
// row selection is enabled.
func (p *TablePreview) idxText(absRow int) string {
	n := strconv.Itoa(absRow + 1)
	if !p.rowSelectable {
		return n
	}
	if p.selected[absRow] {
		return "☑ " + n
	}
	return "☐ " + n
}

// CellType returns the backend type of the cell at table coords (row, col).
func (p *TablePreview) CellType(row, column int) string {
	if row <= 0 || column <= 0 || p.cellTypes == nil {
		return ""
	}
	dataRow := p.curPage*p.pageDelta + row - 1
	dataCol := column - 1
	if dataRow < 0 || dataRow >= len(p.cellTypes) {
		return ""
	}
	if dataCol < 0 || dataCol >= len(p.cellTypes[dataRow]) {
		return ""
	}
	return p.cellTypes[dataRow][dataCol]
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

func (p *TablePreview) Update(title []TablePageTitle, rows []Row, cellTypes [][]string) {
	p.title = title
	p.rows = rows
	p.cellTypes = cellTypes
	p.pending = make(map[[2]int]string)
	p.selected = make(map[int]bool) // drop row ticks from the previous key
	p.colWidths = make(map[int]int) // drop manual overrides from the previous key
	p.stretchCol = widestColumn(title, rows)
	p.stretchWidth = 0 // re-measured on the next draw against the real pane width
	p.computeContentWidths()
	p.cellInfoView.SetText("") // clear stale type/size from the previous entry

	pageCount := len(rows) / p.pageDelta
	if len(rows)%p.pageDelta > 0 {
		pageCount++
	}
	p.totalPage = pageCount
	p.Show(0)

	p.updateStatus()
}

func (p *TablePreview) Show(pageNum int) {
	p.curPage = pageNum
	p.table.Clear()
	for i, v := range p.title {
		// no Expansion: columns size to their content (capped by colWidth),
		// rather than stretching proportionally to fill the table width.
		// Data-column headers (i>=1) get the same left pad as their cells so the
		// header text lines up with the values below it.
		name := v.Name
		if i >= 1 {
			name = cellPad + name
		}
		hcell := tview.NewTableCell(name).SetSelectable(false).SetTextColor(tcell.ColorYellow)
		if i >= 1 {
			hcell.SetMaxWidth(p.cellMaxWidth(i - 1))
		}
		p.table.SetCell(0, i, hcell)
	}
	// With no data rows, make the table non-selectable and don't point the
	// selection at a phantom row: a selectable table whose only row is the
	// non-selectable header spins tview's draw-time selection clamp and freezes
	// the UI. Re-enable selection once there are rows to land on.
	if len(p.rows) == 0 {
		p.table.SetSelectable(false, false)
		p.hlRow = -1
	} else {
		p.table.SetSelectable(true, true)
		p.hlRow = 1
		p.table.Select(1, 1)
	}
	p.table.ScrollToBeginning()

	var begin = p.pageDelta * pageNum
	var end = begin + p.pageDelta - 1
	if end >= len(p.rows) {
		end = len(p.rows) - 1
	}
	for i := begin; i <= end; i++ {
		row := p.rows[i]
		showIndex := i - begin + 1
		idxCell := tview.NewTableCell(p.idxText(i)).SetSelectable(false)
		if showIndex == p.hlRow {
			idxCell.SetBackgroundColor(ThemeRowHighlightBG)
		} else {
			idxCell.SetBackgroundColor(ThemePanelBG)
		}
		if p.rowSelectable {
			// Clicking the checkbox cell toggles that row's ticked state. Returning
			// true suppresses the normal cell-select so the click only toggles.
			absRow := i
			idxCell.SetClickedFunc(func() bool {
				p.toggleRow(absRow)
				return true
			})
		}
		p.table.SetCell(showIndex, 0, idxCell)
		for j, c := range row {
			cell := tview.NewTableCell(p.displayText(c, j)).SetMaxWidth(p.cellMaxWidth(j))
			if _, dirty := p.pending[[2]int{i, j}]; dirty {
				cell.SetTextColor(tcell.ColorBlack)
			}
			cell.SetBackgroundColor(p.rowBG(i, j, showIndex))
			p.table.SetCell(showIndex, j+1, cell)
		}
	}
}

func (p *TablePreview) SetSelectionChangedFunc(handler func(row, column int)) {
	p.userSelChanged = handler
}

// TablePrimitive returns the focusable table, for external focus switching.
func (p *TablePreview) TablePrimitive() tview.Primitive {
	return p.table
}
