package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type TextPreview struct {
	*tview.Flex
	view *tview.TextArea

	oldText  string
	saveBtn  *tview.Button
	copyBtn  *tview.Button
	editBtn  *tview.Button
	saveGrid *tview.Grid

	editable bool // content supports editing at all (Save-capable entry)
	editing  bool // currently in edit mode (textarea writable)

	onSave  func(oldValue, newValue string)
	onCopy  func(text string)
	setFocus func(p tview.Primitive)
}

func NewTextPreview() *TextPreview {
	p := &TextPreview{}
	p.init()
	return p
}

func (p *TextPreview) init() {
	// Read-only by default; the user opts into editing with `i` or the Edit button.
	view := tview.NewTextArea().SetWrap(true)
	view.SetDisabled(true)

	saveBtn := tview.NewButton("Save")
	saveBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	saveBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	saveBtn.SetSelectedFunc(func() {
		if p.onSave != nil {
			newValue := p.view.GetText()
			p.onSave(p.oldText, newValue)
		}
		p.commitEdit()
	})

	editBtn := tview.NewButton("Edit")
	editBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	editBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	editBtn.SetSelectedFunc(func() {
		p.enterEdit()
	})

	copyBtn := tview.NewButton("Copy")
	copyBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	copyBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	copyBtn.SetSelectedFunc(func() {
		if p.onCopy != nil {
			p.onCopy(p.view.GetText())
		}
	})

	// `i` enters edit mode when the value is editable; Esc restores read-only and
	// discards unsaved changes. The capture runs even while the textarea is
	// disabled (WrapInputHandler runs the capture before the disabled guard).
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if !p.editing {
			if p.editable && event.Key() == tcell.KeyRune && event.Rune() == 'i' {
				p.enterEdit()
				return nil
			}
			return event
		}
		if event.Key() == tcell.KeyEscape {
			p.exitEdit()
			return nil
		}
		return event
	})

	// Columns: [stretch | Copy | Edit/Save]; the action button sits right-aligned.
	grid := tview.NewGrid().SetColumns(-1, 8, 8).SetBorders(false).SetGap(0, 2).SetMinSize(5, 5)

	view.SetBackgroundColor(ThemePanelBG)
	grid.SetBackgroundColor(ThemePanelBG)
	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(view, 0, 1, true).
		AddItem(grid, 1, 1, false)
	flex.SetBackgroundColor(ThemePanelBG)

	p.view = view
	p.saveBtn = saveBtn
	p.editBtn = editBtn
	p.copyBtn = copyBtn
	p.saveGrid = grid
	p.Flex = flex
}

func (p *TextPreview) SetText(text string) {
	p.oldText = text
	// if len(text) > 4096 {
	// 	text = text[:4096] + "..."
	// }
	p.view.SetText(text, true)
}

// TextArea returns the focusable editor primitive, for external focus switching.
func (p *TextPreview) TextArea() tview.Primitive {
	return p.view
}

// SetFocusFunc injects the focus setter used when the Edit button (mouse) puts
// the textarea into edit mode, so keyboard focus lands on the editor.
func (p *TextPreview) SetFocusFunc(f func(p tview.Primitive)) {
	p.setFocus = f
}

// IsEditing reports whether the textarea is currently writable.
func (p *TextPreview) IsEditing() bool {
	return p.editing
}

// enterEdit makes the textarea writable and swaps the Edit button for Save.
func (p *TextPreview) enterEdit() {
	if !p.editable || p.editing {
		return
	}
	p.editing = true
	p.view.SetDisabled(false)
	p.refreshButtons()
	if p.setFocus != nil {
		p.setFocus(p.view)
	}
}

// exitEdit leaves edit mode (Esc) without reverting: the typed text stays in the
// box, the user just drops back to read-only. The edit is simply not persisted.
func (p *TextPreview) exitEdit() {
	if !p.editing {
		return
	}
	p.editing = false
	p.view.SetDisabled(true)
	p.refreshButtons()
}

// commitEdit restores read-only mode after a Save, keeping the current text as
// the new baseline.
func (p *TextPreview) commitEdit() {
	if !p.editing {
		return
	}
	p.editing = false
	p.view.SetDisabled(true)
	p.oldText = p.view.GetText()
	p.refreshButtons()
}

// ShowSaveGrid lays out the bottom button row and sets whether the value is
// editable. Copy is always shown. When editable, an Edit button is shown
// (read-only mode) or a Save button (edit mode); each new value starts read-only.
func (p *TextPreview) ShowSaveGrid(editable bool) {
	p.editable = editable
	p.editing = false
	p.view.SetDisabled(true)
	p.refreshButtons()
}

// refreshButtons rebuilds the bottom button row to match the current edit state.
func (p *TextPreview) refreshButtons() {
	p.saveGrid.Clear()
	p.saveGrid.AddItem(p.copyBtn, 0, 1, 1, 1, 0, 0, false)
	if p.editable {
		if p.editing {
			p.saveGrid.AddItem(p.saveBtn, 0, 2, 1, 1, 0, 0, false)
		} else {
			p.saveGrid.AddItem(p.editBtn, 0, 2, 1, 1, 0, 0, false)
		}
	}
}

func (p *TextPreview) SetSaveHandler(f func(string, string)) {
	p.onSave = f
}

// SetClipboardHandler wires the Copy button to a system-clipboard writer.
func (p *TextPreview) SetClipboardHandler(f func(text string)) {
	p.onCopy = f
}
