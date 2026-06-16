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
	typeText  *tview.TextView
	keyType   string
	delBtn    *tview.Button
	reloadBtn *tview.Button
	renameBtn *tview.Button
	keyInput  *tview.InputField
	grid      *tview.Grid

	queryInput *tview.InputField
	queryFunc  func(query string)

	textPreview  *TextPreview
	tablePreview *TablePreview
}

// NewPreview new
func NewPreview() *Preview {
	typeText := tview.NewTextView()
	typeText.
		SetTextColor(ThemeTypeFG).
		SetBackgroundColor(ThemePanelBG)

	sizeText := tview.NewTextView()
	sizeText.
		SetTextColor(ThemeSizeFG).
		SetBackgroundColor(ThemePanelBG)

	keyInput := tview.NewInputField()
	keyInput.SetLabel("Key:").
		SetLabelWidth(4).
		SetLabelColor(ThemeQueryLabel).
		SetFieldTextColor(ThemeQueryFG).
		SetFieldBackgroundColor(ThemeInputBG)
	delBtn := tview.NewButton("Delete")
	delBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnDangerBG).Foreground(ThemeBtnDangerFG))
	delBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnDangerHoverBG).Foreground(tcell.ColorWhite))
	reloadBtn := tview.NewButton("Reload")
	reloadBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	reloadBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	renameBtn := tview.NewButton("Rename")
	renameBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	renameBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	grid := tview.NewGrid().
		SetRows(-1).
		SetColumns(16, 16, 10, 10, 30, 10, -1).
		SetBorders(false).
		SetGap(0, 2).
		SetMinSize(5, 5)

	queryInput := tview.NewInputField()
	queryInput.SetLabel(" Query ").
		SetLabelColor(ThemeQueryLabel).
		SetPlaceholder(`{"field": value}  or  db.coll.find({})   —  Enter to run, empty = all`)
	// Pin both the field style and the placeholder style: tview defaults the
	// placeholder background to Styles.ContrastBackgroundColor (our selection
	// blue), which would tint the empty field. Set both backgrounds to the dark
	// query fill so the box stays dark whether empty or filled.
	queryInput.SetFieldStyle(tcell.StyleDefault.Background(ThemeQueryBG).Foreground(ThemeQueryFG))
	queryInput.SetPlaceholderStyle(tcell.StyleDefault.Background(ThemeQueryBG).Foreground(ThemeQueryPlaceholder))
	queryInput.SetBackgroundColor(ThemePanelBG)

	showFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	showFlex.
		// SetTitle("PREVIEW").
		SetBorder(true).
		SetBorderColor(ThemeBorderDim)
	showFlex.SetBackgroundColor(ThemePanelBG)

	previewFlexBox := tview.NewFlex()
	previewFlexBox.SetDirection(tview.FlexRow)
	previewFlexBox.SetBackgroundColor(ThemePanelBG)
	// The op row (grid: type/size chips + reload/delete/key/rename) is hosted in
	// the global top bar (see MainView), not here, so the preview is just the
	// value area. MainView pulls it via OpBar().
	previewFlexBox.AddItem(showFlex, 0, 1, false)
	grid.SetBackgroundColor(ThemePanelBG)

	p := &Preview{
		flexBox:    previewFlexBox,
		showFlex:   showFlex,
		sizeText:   sizeText,
		typeText:   typeText,
		delBtn:     delBtn,
		reloadBtn:  reloadBtn,
		renameBtn:  renameBtn,
		keyInput:   keyInput,
		grid:       grid,
		queryInput: queryInput,

		textPreview:  NewTextPreview(),
		tablePreview: NewTablePreview(),
	}
	p.init()
	return p
}

func (p *Preview) FlexBox() *tview.Flex {
	return p.flexBox
}

// OpBar returns the per-preview operation row (type/size chips and
// reload/delete/key/rename buttons). MainView hosts it in the global top bar,
// to the right of the connection dropdown, instead of above the value area.
func (p *Preview) OpBar() *tview.Grid {
	return p.grid
}

func (p *Preview) init() {
	p.queryInput.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			return
		}
		if p.queryFunc != nil {
			p.queryFunc(p.queryInput.GetText())
		}
	})
	p.tablePreview.SetSelectionChangedFunc(func(row, column int) {
		if row <= 0 {
			return
		}
		h := p.tablePreview.rows
		idx := p.tablePreview.curPage*p.tablePreview.pageDelta + row - 1
		if idx < 0 || idx >= len(h) {
			return
		}
		c := h[idx]
		size := len(c[len(c)-1])
		p.SetSizeText(fmt.Sprintf("Size: %d bytes", size))
		if t := p.tablePreview.CellType(row, column); t != "" {
			p.SetTypeText(fmt.Sprintf("Type: %s", t))
		}
	})
}

// SetQueryFunc registers the callback invoked when the user submits a query in
// the query box (Enter). Calling it with a non-nil function also shows the box;
// passing nil hides it. The box sits above the preview content.
func (p *Preview) SetQueryFunc(f func(query string)) {
	p.queryFunc = f
}

// QueryPrimitive returns the focusable query input, for external focus switching.
func (p *Preview) QueryPrimitive() tview.Primitive {
	return p.queryInput
}

// IsQueryShown reports whether the query box is currently visible (only when a
// query callback is registered and the table view is active).
func (p *Preview) IsQueryShown() bool {
	return p.queryFunc != nil && p.IsTableShown()
}

// QueryText returns the current query box text.
func (p *Preview) QueryText() string {
	return p.queryInput.GetText()
}

// SetQueryText sets the query box text (without firing the query callback).
func (p *Preview) SetQueryText(text string) {
	p.queryInput.SetText(text)
}

// Clear all
func (p *Preview) Clear() {
	p.keyType = ""
	p.SetSizeText("")
	p.SetTypeText("")
}

// SetSizeText show text size
func (p *Preview) SetSizeText(text string) {
	if len(text) == 0 {
		p.grid.RemoveItem(p.sizeText)
		p.sizeText.SetBackgroundColor(ThemePanelBG)
	} else {
		p.grid.AddItem(p.sizeText, 0, 1, 1, 1, 0, 0, false)
		p.sizeText.SetBackgroundColor(ThemeChipBG)
	}
	p.sizeText.SetText(text)
}

// SetKeyType set current redis key type text prefix
func (p *Preview) SetKeyType(t string) {
	p.keyType = t
	p.SetTypeText("")
	if len(t) > 0 {
		p.SetTypeText(fmt.Sprintf("Type: %s", t))
	}
}

// SetTypeText set type label text
func (p *Preview) SetTypeText(text string) {
	if len(text) == 0 {
		p.grid.RemoveItem(p.typeText)
		p.typeText.SetBackgroundColor(ThemePanelBG)
	} else {
		p.grid.AddItem(p.typeText, 0, 0, 1, 1, 0, 0, false)
		p.typeText.SetBackgroundColor(ThemeChipBG)
		text = " " + text // 左侧留一个空格,chip 文字不贴边
	}
	p.typeText.SetText(text)
}

// SetOpBtnVisible show reload delete button
func (p *Preview) SetOpBtnVisible(visible bool) {
	if visible {
		p.grid.AddItem(p.reloadBtn, 0, 2, 1, 1, 0, 0, false)
		p.grid.AddItem(p.delBtn, 0, 3, 1, 1, 0, 0, false)
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

// SetClipboardFunc wires the system-clipboard writer used by the text preview's
// Copy button (copies the current text value to the system clipboard).
func (p *Preview) SetClipboardFunc(f func(text string)) {
	p.textPreview.SetClipboardHandler(f)
}

// SetKey set key input text
func (p *Preview) SetKey(text string) {
	if len(text) > 0 {
		p.grid.AddItem(p.keyInput, 0, 4, 1, 1, 0, 0, false)
		p.grid.AddItem(p.renameBtn, 0, 5, 1, 1, 0, 0, false)
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

func (p *Preview) ShowTable(title []TablePageTitle, rows []Row, cellTypes [][]string) {
	p.showFlex.Clear()
	if p.queryFunc != nil {
		p.showFlex.AddItem(p.queryInput, 1, 0, false)
	}
	p.showFlex.AddItem(p.tablePreview, 0, 1, false)
	p.tablePreview.Update(title, rows, cellTypes)
}

// SetTableCommitFunc registers the callback invoked when staged cell edits
// are committed (Ctrl-S). It receives all pending edits as a batch.
func (p *Preview) SetTableCommitFunc(f func(edits []CellEdit) error) {
	p.tablePreview.SetCommitFunc(f)
}

// SetTableReloadFunc registers the callback used to repaint the table after a
// commit or rollback (typically re-fetches and re-renders the current entry).
func (p *Preview) SetTableReloadFunc(f func()) {
	p.tablePreview.SetReloadFunc(f)
}

// TablePrimitive returns the focusable table primitive of the table preview.
func (p *Preview) TablePrimitive() tview.Primitive {
	return p.tablePreview.TablePrimitive()
}

// TableAtLeftEdge reports whether the table selection is on its first data
// column, used to decide when Left should leave the table for the tree rather
// than move the selection one column left.
func (p *Preview) TableAtLeftEdge() bool {
	_, col := p.tablePreview.table.GetSelection()
	return col <= 1
}

// IsTableShown reports whether the table preview is the active right-pane view.
func (p *Preview) IsTableShown() bool {
	for i := 0; i < p.showFlex.GetItemCount(); i++ {
		if p.showFlex.GetItem(i) == p.tablePreview {
			return true
		}
	}
	return false
}

// IsTextShown reports whether the text preview is the active right-pane view.
func (p *Preview) IsTextShown() bool {
	for i := 0; i < p.showFlex.GetItemCount(); i++ {
		if p.showFlex.GetItem(i) == p.textPreview {
			return true
		}
	}
	return false
}

// TextPrimitive returns the focusable editor primitive of the text preview.
func (p *Preview) TextPrimitive() tview.Primitive {
	return p.textPreview.TextArea()
}

// SetTableFocusFunc injects the focus setter used during inline editing.
func (p *Preview) SetTableFocusFunc(f func(prim tview.Primitive)) {
	p.tablePreview.SetFocusFunc(f)
}

// SetTextFocusFunc injects the focus setter used when the text preview enters
// edit mode via the Edit button (so focus lands on the editor).
func (p *Preview) SetTextFocusFunc(f func(prim tview.Primitive)) {
	p.textPreview.SetFocusFunc(f)
}

// IsTextEditing reports whether the text preview is currently in edit mode
// (textarea writable). Used to decide whether Left should leave for the tree.
func (p *Preview) IsTextEditing() bool {
	return p.textPreview.IsEditing()
}

func (p *Preview) ShowText(text string, showSave bool) {
	p.showFlex.Clear()
	p.showFlex.AddItem(p.textPreview, 0, 1, false)
	p.textPreview.SetText(text)
	p.textPreview.ShowSaveGrid(showSave)
}
