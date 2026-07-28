package view

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Setting struct {
	Name    string
	Kind    string // "redis" (default), "mongo", or "mysql"
	Host    string
	Port    string
	User    string // mysql / mongo username
	Auth    string
	URI     string // mongo connection string (URL mode)
	DB      string // mongo default database (form mode)
	URLMode bool   // mongo: true = URL string, false = host/port form
}

// connKinds lists supported backends in dropdown order.
var connKinds = []string{"redis", "mongo", "mysql", "zookeeper"}

// mongoModes lists the mongo connection-entry modes in dropdown order.
var mongoModes = []string{"form", "url"}

// padOpts returns display copies of dropdown options padded with a leading
// space and right-padded so every option is the SAME display width. Uniform
// width keeps the ▼ caret suffix (added via SetTextOptions) at the field's
// right edge regardless of which (shorter) option is currently selected —
// otherwise the caret hugs the selected text and floats mid-field.
// Selection logic must use the option index, not these padded strings.
func padOpts(items []string) []string {
	maxW := 0
	for _, v := range items {
		if w := tview.TaggedStringWidth(v); w > maxW {
			maxW = w
		}
	}
	out := make([]string, len(items))
	for i, v := range items {
		out[i] = " " + v + strings.Repeat(" ", maxW-tview.TaggedStringWidth(v)+1)
	}
	return out
}

// dropCaret appends a ▼ marker to a form dropdown's closed field so it reads as
// a dropdown rather than a plain text input (both share the same field bg). The
// suffix only affects the closed display, not the option list. No-op if the
// labelled item isn't a *tview.DropDown.
func dropCaret(form *tview.Form, label string) {
	if item := form.GetFormItemByLabel(label); item != nil {
		if d, ok := item.(*tview.DropDown); ok {
			d.SetTextOptions("", "", "", " ▼ ", "")
		}
	}
}

type ConnSetting struct {
	tview.Primitive
	form      *tview.Form
	body      *tview.Flex   // bordered card: form + status, height fit to contents
	card      *tview.Flex   // centered column holding body between flexible spacers
	okBtn     *tview.Button // primary action, repainted in accent color after each Draw
	statusMsg string        // inline Test-result line, painted on the blank row above the buttons
	ok        func(Setting, bool)
	cancel    func()
	test      func(Setting) error
	clipboard func(text string) // system-clipboard writer (OSC52), wired from MainView
	edit      bool
	cur       Setting // current field values, preserved across kind switches
	building  bool    // true while build() rebuilds the form, to ignore dropdown callbacks
	blurred   bool    // true after a blank-area click: suppress the focus highlight until a field is re-focused
}

func NewConnSetting() *ConnSetting {
	p := &ConnSetting{}
	p.init()
	return p
}

// blockingPrimitive wraps a full-screen primitive and consumes any mouse event
// its child doesn't, making a centered dialog behave modally. Returning a nil
// event from a MouseCapture leaves consumed=false, so Pages still forwards the
// click to the page below; only reporting consumed=true here actually stops it.
type blockingPrimitive struct {
	tview.Primitive
	// onOutsideClick is invoked for a left mouse-down the card's children didn't
	// consume — i.e. a click on the dimmed backdrop outside the card. A dropdown
	// opened via keyboard isn't the app's mouse-capturing primitive, so tview's
	// own click-outside-closes path never sees this event (we swallow it here);
	// the hook lets the dialog collapse any still-open dropdown instead.
	onOutsideClick func(event *tcell.EventMouse, setFocus func(p tview.Primitive))
	// afterDraw runs after the wrapped card (and its form) has fully drawn. The
	// form repaints every button with its shared style each frame, clobbering any
	// per-button color; this hook lets the dialog repaint the primary button on
	// top with its own emphasis color once the form has laid it out.
	afterDraw func(screen tcell.Screen)
}

// Draw shades the whole screen (the main UI already painted beneath this page)
// before drawing the card, so the dialog floats above a dimmed backdrop like the
// web UI's translucent rgba(0,0,0,0.5) overlay. The card repaints its own region
// at full brightness afterward; the transparent spacers around it (see Center)
// leave the shaded backdrop visible.
func (b *blockingPrimitive) Draw(screen tcell.Screen) {
	dimBackdrop(screen)
	b.Primitive.Draw(screen)
	if b.afterDraw != nil {
		b.afterDraw(screen)
	}
}

func (b *blockingPrimitive) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	inner := b.Primitive.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if inner != nil {
			if consumed, capture := inner(action, event, setFocus); consumed {
				return consumed, capture
			}
		}
		if action == tview.MouseLeftDown && b.onOutsideClick != nil {
			b.onOutsideClick(event, setFocus)
		}
		return true, nil // child didn't take it: swallow so it can't reach the page below
	}
}

// dimBackdrop darkens every cell currently on screen by scaling its foreground
// and background toward black, approximating the web UI's 50%-black overlay. It
// runs before the dialog card is drawn, so the main UI showing through the
// dialog's transparent spacers reads as a dimmed backdrop. Cells the card later
// repaints are restored to full brightness.
func dimBackdrop(screen tcell.Screen) {
	w, h := screen.Size()
	for y := range h {
		for x := range w {
			str, style, _ := screen.Get(x, y)
			fg, bg, attr := style.Decompose()
			dimmed := tcell.StyleDefault.
				Foreground(dimColor(fg)).
				Background(dimColor(bg)).
				Attributes(attr)
			mainc, combc := decodeCell(str)
			screen.SetContent(x, y, mainc, combc, dimmed)
		}
	}
}

// decodeCell splits a cell's string (as returned by screen.Get) into the
// primary rune plus any combining runes, the shape SetContent expects. An empty
// string means a blank cell, rendered as a space.
func decodeCell(str string) (rune, []rune) {
	runes := []rune(str)
	if len(runes) == 0 {
		return ' ', nil
	}
	return runes[0], runes[1:]
}

// dimBackdropFactor is how much of each color channel survives the shade (the
// rest goes to black). 0.45 ≈ the web overlay's rgba(0,0,0,0.5).
const dimBackdropFactor = 0.45

// dimColor scales a color's RGB channels toward black by dimBackdropFactor.
// ColorDefault has no RGB to scale, so it's resolved to the panel bg first
// (that's what an unpainted terminal cell shows against this theme).
func dimColor(c tcell.Color) tcell.Color {
	if c == tcell.ColorDefault {
		c = ThemePanelBG
	}
	r, g, b := c.RGB()
	return tcell.NewRGBColor(
		int32(float64(r)*dimBackdropFactor),
		int32(float64(g)*dimBackdropFactor),
		int32(float64(b)*dimBackdropFactor),
	)
}

// centered wraps a fixed-width card in flexible spacers so it floats in the
// screen's center. The spacer cells around the card are transparent (nil) so the
// main UI stays visible behind the dialog; the card itself stands out via its
// lifted fill + bright border (see init). The returned column is the vertical
// Flex holding the card between top/bottom spacers, so callers can resize the
// card item to fit its contents.
func centered(width, height int, card tview.Primitive) (outer, column *tview.Flex) {
	column = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(card, height, 0, true).
		AddItem(nil, 0, 1, false)
	outer = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(column, width, 0, true).
		AddItem(nil, 0, 1, false)
	return outer, column
}

func (s *ConnSetting) init() {
	form := tview.NewForm()
	form.SetButtonsAlign(tview.AlignRight)
	form.SetFieldTextColor(ThemeQueryFG)
	form.SetFieldBackgroundColor(ThemeInputBG)
	form.SetLabelColor(ThemeQueryLabel)
	form.SetButtonBackgroundColor(ThemeBtnToolBG)
	form.SetButtonTextColor(ThemeBtnToolFG)
	form.SetButtonActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	form.SetBackgroundColor(ThemeDialogBG)
	// Any keypress (Tab/arrow nav, typing) means the user is interacting with a
	// field again, so lift the blank-click blur and let the focus highlight return.
	// Ctrl-Y copies the focused input field's full value to the system clipboard —
	// tview's own selection only highlights internally and never reaches the OS
	// clipboard, so a long URI can't otherwise be copied out.
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		s.blurred = false
		if event.Key() == tcell.KeyCtrlY {
			s.copyFocusedField()
			return nil
		}
		return event
	})

	// The form alone fills the bordered card. tview's Form leaves a blank
	// separator row between the last field and the button row; the Test-result
	// status line is painted onto that row in the afterDraw hook (see
	// paintStatusLine), so it sits below the fields and above the buttons without
	// a separate widget. The card uses the lifted ThemeDialogBG + a bright focus
	// border so it floats above the dimmed backdrop instead of blending into the
	// (equally dark) main panels.
	body := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true)
	body.SetBackgroundColor(ThemeDialogBG)
	body.SetBorder(true).
		SetBorderColor(ThemeBorderFocus).
		SetTitle(" Connection Settings ").
		SetTitleColor(ThemeTitleFG)
	s.blockFormBlankClick(form)
	outer, column := centered(64, 20, body)
	s.Primitive = &blockingPrimitive{
		Primitive:      outer, // make the dialog modal to mouse clicks
		onOutsideClick: s.onBackdropClick,
		afterDraw: func(screen tcell.Screen) {
			s.highlightFocusedField(screen)
			s.paintStatusLine(screen)
			s.paintPrimaryButton(screen)
		},
	}
	s.form = form
	s.body = body
	s.card = column
	s.build("redis")
}

// onBackdropClick handles a left-click on the dimmed backdrop outside the card.
// The blockingPrimitive swallows such clicks to stay modal but never moves focus,
// so it must proactively undo the two focus-linked artifacts tview would
// otherwise leave visible: an open dropdown and a lingering text selection.
func (s *ConnSetting) onBackdropClick(event *tcell.EventMouse, setFocus func(p tview.Primitive)) {
	s.closeOpenDropdowns(event, setFocus)
	s.collapseFieldSelections()
}

// closeOpenDropdowns collapses any form dropdown left open (e.g. Kind/Mode
// expanded via keyboard) when the user clicks the dimmed backdrop outside the
// card. Feeding the click to an open dropdown's MouseHandler triggers tview's
// own click-outside-closes path; the swallowing blockingPrimitive otherwise
// starves that path of the event. The click position is off the list, so the
// handler closes rather than selects.
func (s *ConnSetting) closeOpenDropdowns(event *tcell.EventMouse, setFocus func(p tview.Primitive)) {
	for i := 0; i < s.form.GetFormItemCount(); i++ {
		if d, ok := s.form.GetFormItem(i).(*tview.DropDown); ok && d.IsOpen() {
			d.MouseHandler()(tview.MouseLeftDown, event, setFocus)
		}
	}
}

// resetFieldView clears a field's lingering text selection without moving its
// cursor or viewport. tview selects the whole value on focus; on blur we only
// want the white highlight gone, leaving the horizontal scroll exactly where the
// user left it (so a long URI stays showing the part they were editing, not
// snapped back to the "mongodb://" prefix). collapseSelectionInPlace empties the
// selection by hand — unlike SetText(GetText()), whose internal Replace flings
// the caret to the text end and lets the next Draw scroll the viewport there.
func (s *ConnSetting) resetFieldView(f *tview.InputField) {
	collapseSelectionInPlace(f)
}

// collapseFieldSelections resets every input field (see resetFieldView). Used
// when a click on the modal backdrop keeps a field focused, so its selection and
// scroll would otherwise linger.
func (s *ConnSetting) collapseFieldSelections() {
	for i := 0; i < s.form.GetFormItemCount(); i++ {
		if f, ok := s.form.GetFormItem(i).(*tview.InputField); ok {
			s.resetFieldView(f)
		}
	}
}

// collapseFieldSelectionsExcept resets every input field except the one whose
// rect contains (x,y). The clicked field is skipped because tview's own
// MouseLeftDown will position its cursor and clear its selection anyway.
func (s *ConnSetting) collapseFieldSelectionsExcept(x, y int) {
	for i := 0; i < s.form.GetFormItemCount(); i++ {
		f, ok := s.form.GetFormItem(i).(*tview.InputField)
		if !ok || inRect(f, x, y) {
			continue
		}
		s.resetFieldView(f)
	}
}

// blockFormBlankClick swallows a left-click that lands on the form's blank areas
// — the padding rows between fields and the gap above the button row — where no
// field or button sits. Two tview paths would otherwise re-focus a field there:
//   - A left-DOWN in the form rect that no item consumes falls through to
//     Form.MouseHandler's "return focus to the last selected element" (form.go),
//     re-focusing the previously edited input and re-selecting its whole value.
//   - Even when we swallow the down, the follow-up left-UP / left-CLICK for the
//     same press still reach the form (they're separate MouseActions) and hit the
//     SAME fallback — so a click on the padding row directly BELOW a field that
//     was focused earlier silently re-selects it. This is the "clicking just
//     above/below a field selects it" bug.
//
// So we intercept every left button/press action, not just the down: on any that
// lands off all fields/buttons we collapse selections and consume it, leaving
// focus and highlight cleared. Installed once on the persistent form; Form.Clear
// doesn't touch the capture.
func (s *ConnSetting) blockFormBlankClick(form *tview.Form) {
	form.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftUp,
			tview.MouseLeftClick, tview.MouseLeftDoubleClick:
		default:
			return action, event // wheel/move/right-button: leave untouched
		}
		x, y := event.Position()
		// A field's on-screen rect spans its whole row, most of which is the empty
		// InputBG bar to the right of the (usually short) value — visually "blank"
		// but still the field's hitbox. tview reacts to a left-down anywhere in that
		// row by re-focusing the field and re-selecting its entire value. Collapse
		// the OTHER fields' lingering selections, but skip the one being clicked:
		// tview's own MouseLeftDown already repositions its cursor and clears its
		// selection. Re-setting the clicked field's text here would first fling its
		// cursor (and horizontal scroll) to the end, then the click yanks it back —
		// a visible slide on the long, scrollable URI field.
		if action == tview.MouseLeftDown {
			s.collapseFieldSelectionsExcept(x, y)
		}
		if s.pointOverFormElement(x, y) {
			s.blurred = false // a real field/button click restores the focus highlight
			return action, event
		}
		s.blurOnBlankClick()
		return tview.MouseConsumed, nil
	})
}

// blurOnBlankClick performs the visual "unfocus" applied when the user clicks a
// blank area of the dialog that we consume without moving tview's logical focus:
// it flags s.blurred so highlightFocusedField hides the focus highlight AND the
// leftover hardware cursor of whatever field was focused (see that method), and
// collapses every field's lingering whole-value selection. Logical focus is left
// untouched on purpose so Esc still cancels the dialog. Called from
// blockFormBlankClick for any click off a real element — including the blank tail
// to the right of a closed dropdown, which pointOverFormElement now excludes.
func (s *ConnSetting) blurOnBlankClick() {
	s.blurred = true
	s.collapseFieldSelections()
}

// pointOverFormElement reports whether screen point (x,y) lies over an
// interactive part of the form — used by blockFormBlankClick to tell a real
// field/button click from one on blank filler.
//
// A vertical Form sizes every item's rect to the FULL row width, but a closed
// DropDown only paints "label + widest option + ▼" at the left; the rest of the
// row is blank yet still inside the rect. Treating that tail as "over an element"
// is exactly what let a click on the space right of the Mode dropdown keep the
// previously focused field (and its blinking cursor) alive. So for dropdowns we
// test the visible field extent, not the whole rect. Because blockFormBlankClick
// runs first on the Form and consumes such a click, the dropdown's own
// MouseHandler never sees it — so no separate hitbox guard on the dropdown is
// needed (or wanted: a second consumer there would strand the cursor again).
func (s *ConnSetting) pointOverFormElement(x, y int) bool {
	for i := 0; i < s.form.GetFormItemCount(); i++ {
		item := s.form.GetFormItem(i)
		if d, ok := item.(*tview.DropDown); ok {
			if s.pointOverDropdownField(d, x, y) {
				return true
			}
			continue
		}
		if inRect(item, x, y) {
			return true
		}
	}
	for i := 0; i < s.form.GetButtonCount(); i++ {
		if inRect(s.form.GetButton(i), x, y) {
			return true
		}
	}
	return false
}

// pointOverDropdownField reports whether (x,y) is over a closed dropdown's
// VISIBLE field — its row, from the row start through "label + widest option +
// ▼" — as opposed to the blank rect tail to its right. An open dropdown captures
// all mouse events itself, so this only gates the closed state.
func (s *ConnSetting) pointOverDropdownField(d *tview.DropDown, x, y int) bool {
	rx, ry, _, _ := d.GetRect()
	if y != ry {
		return false
	}
	return x >= rx && x < rx+s.dropdownFieldWidth(d)
}

// dropdownFieldWidth is the on-screen width of a closed dropdown's painted part:
// the form's aligned label column + the widest option + the " ▼ " caret. It
// mirrors tview's vertical-Form label alignment (widest label + one space).
func (s *ConnSetting) dropdownFieldWidth(d *tview.DropDown) int {
	return s.formLabelWidth() + d.GetFieldWidth() + tview.TaggedStringWidth(" ▼ ")
}

// inRect reports whether screen point (x,y) is inside a primitive's rect.
func inRect(p tview.Primitive, x, y int) bool {
	rx, ry, w, h := p.GetRect()
	return x >= rx && x < rx+w && y >= ry && y < ry+h
}

// build lays out the form for a given backend kind. Redis keeps its original
// Address + Auth fields; mysql adds a Username; mongo swaps to a single URI.
// Switching the Kind dropdown captures the current values and rebuilds so each
// backend shows only the fields it needs.
func (s *ConnSetting) build(kind string) {
	s.building = true
	defer func() { s.building = false }()

	s.cur.Kind = kind
	s.blurred = false // a fresh layout starts with the focused field highlighted
	s.setStatus("")   // a kind/mode switch invalidates any prior Test result
	form := s.form
	form.Clear(true) // also clear buttons; they're rebuilt below so Delete tracks edit mode
	form.AddInputField("Name:", s.cur.Name, 0, nil, nil)

	kindIdx := indexOf(connKinds, kind)
	form.AddDropDown("Kind:", padOpts(connKinds), kindIdx, func(_ string, idx int) {
		if s.building || idx < 0 || connKinds[idx] == s.cur.Kind {
			return
		}
		s.capture()
		s.build(connKinds[idx])
	})
	dropCaret(form, "Kind:")

	switch kind {
	case "mongo":
		// mongo offers two entry modes: a host/port form or a single URL string.
		modeIdx := indexOf(mongoModes, s.mongoMode())
		form.AddDropDown("Mode:", padOpts(mongoModes), modeIdx, func(_ string, idx int) {
			urlMode := idx >= 0 && mongoModes[idx] == "url"
			if s.building || urlMode == s.cur.URLMode {
				return
			}
			s.capture()
			s.cur.URLMode = urlMode
			s.build(kind)
		})
		dropCaret(form, "Mode:")
		if s.cur.URLMode {
			// URI strings run long (mongodb://user:pass@host:port/db?opts). Keep
			// them on a single scrollable line so the value stays copy/paste-clean
			// (a wrapped multi-row field would fold real newlines into a mouse
			// selection); tview's InputField scrolls horizontally to show the part
			// around the cursor.
			form.AddInputField("URI:", s.cur.URI, 0, nil, nil)
		} else {
			form.AddInputField("Address:", s.address(), 0, nil, nil)
			form.AddInputField("Username:", s.cur.User, 0, nil, nil)
			form.AddPasswordField("Auth:", s.cur.Auth, 0, '*', nil)
			form.AddInputField("DB:", s.cur.DB, 0, nil, nil)
		}
	case "mysql":
		form.AddInputField("Address:", s.address(), 0, nil, nil)
		form.AddInputField("Username:", s.cur.User, 0, nil, nil)
		form.AddPasswordField("Auth:", s.cur.Auth, 0, '*', nil)
	case "zookeeper":
		form.AddInputField("Address:", s.address(), 0, nil, nil)
	default: // redis
		form.AddInputField("Address:", s.address(), 0, nil, nil)
		form.AddPasswordField("Auth:", s.cur.Auth, 0, '*', nil)
	}

	// A non-scrollable (thus non-focusable) blank text view sits between the last
	// field and the buttons. It doesn't display anything itself — its row is where
	// paintStatusLine draws the Test result — but adding it as a form item gives
	// the status its own line with a blank padding row on either side, so the
	// message reads clearly separated from the field above and the buttons below
	// instead of hugging the last field's padding row.
	form.AddTextView("", "", 0, 1, false, false)

	// Pad every button label with side spaces so each renders as a raised chip
	// with breathing room, visually distinct from the plain-text field labels
	// above. The primary Save button is repainted in the accent color separately
	// (see paintPrimaryButton); it's looked up by this exact padded label.
	form.AddButton(" Test ", s.OnTest).
		AddButton(" Save ", s.OnOk).
		AddButton(" Cancel ", s.OnCancel)

	// OK is the dialog's primary action. tview's Form repaints every button with
	// the shared buttonStyle on each Draw, so SetStyle here wouldn't survive a
	// frame; instead we keep a reference and repaint it in the accent color via
	// the dialog's afterDraw hook (see init), once the form has positioned it.
	s.okBtn = form.GetButton(form.GetButtonIndex(" Save "))

	s.styleFieldFocus()
	s.fitCard()
}

// fitCard resizes the dialog card to hug its contents so the buttons land at the
// card's bottom edge instead of floating above a large gap. The field count (and
// thus height) changes when the Kind/Mode dropdowns rebuild the form, so this
// recomputes on every build. It pins the form to exactly its content height:
// a proportional form would otherwise pad any leftover space with blank rows
// *under* the buttons (the gap we're removing) or, if undersized, clip the
// button row entirely.
func (s *ConnSetting) fitCard() {
	form := s.form
	// tview's vertical Form draws: a leading blank row, then each item followed
	// by one itemPadding row, then a blank separator, then the button row.
	const topPad, buttonBlock = 1, 2 // leading blank; blank separator + button row
	formHeight := topPad
	for i := 0; i < form.GetFormItemCount(); i++ {
		h := form.GetFormItem(i).GetFieldHeight()
		if h <= 0 {
			h = 1
		}
		formHeight += h + 1 // field + its trailing padding row
	}
	if form.GetButtonCount() > 0 {
		formHeight += buttonBlock
	}
	// The Test-result status shares the form's blank separator row above the
	// buttons (painted in paintStatusLine), so no extra body row is reserved for
	// it; the card is just the form plus its border.
	const border = 2
	s.body.ResizeItem(s.form, formHeight, 0)
	s.card.ResizeItem(s.body, formHeight+border, 0)
}

// styleFieldFocus wires per-field focus styling and mouse behavior that tview
// doesn't do itself:
//   - DropDowns get a focused background matching the input fields
//     (ThemeInputFocusBG) instead of tview's default bright-white fill, so a
//     focused dropdown reads the same as a focused input rather than glaring.
//   - InputFields clear their text selection on blur. tview selects the whole
//     value when a field gains focus; without this the white selection highlight
//     lingers on the field after focus moves away. Re-setting the text collapses
//     the selection (cursor to end) so the blurred field shows plain text.
//   - Every field's label column is made click-inert (see blockLabelClick); the
//     long URI field additionally gets wheel/drag horizontal scrolling, whose
//     capture already folds in the same label guard.
func (s *ConnSetting) styleFieldFocus() {
	for i := 0; i < s.form.GetFormItemCount(); i++ {
		switch f := s.form.GetFormItem(i).(type) {
		case *tview.DropDown:
			// Match the top-left connection dropdown (see opline.go): a lifted field
			// background, a dark focused style (bold white text, no glaring white
			// fill), and an option list whose selected row uses the soft blue accent.
			f.SetFieldBackgroundColor(ThemeDropFieldBG)
			f.SetFocusedStyle(tcell.StyleDefault.
				Background(ThemeDropFieldBG).Foreground(tcell.ColorWhite).Attributes(tcell.AttrBold))
			f.SetListStyles(
				tcell.StyleDefault.Foreground(ThemeQueryFG).Background(ThemeQueryBG),
				tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(ThemeSelectBG),
			)
		case *tview.InputField:
			// NOTE: the blur callback runs while Application holds its lock (SetFocus
			// → Blur), so it must not route through InputHandler/mouse handling —
			// that path re-enters the lock and deadlocks. collapseSelectionInPlace
			// only touches the TextArea's selectionStart field, so it's safe here.
			// It clears the focus-time whole-value selection WITHOUT moving the
			// cursor or scroll — unlike SetText(GetText()), whose internal Replace
			// flings the caret (and a long URI's viewport) to the end on blur.
			field := f
			f.SetBlurFunc(func() { collapseSelectionInPlace(field) })
			if strings.HasPrefix(f.GetLabel(), "URI") {
				enableWheelScroll(f)
				normalizeCursorScroll(f)
			} else {
				blockLabelClick(f)
			}
		}
	}
}

// highlightFocusedField repaints the background of whichever input field holds
// keyboard focus in ThemeInputFocusBG, on top of what the form already drew.
// tview marks focus only with a lone cursor, and Form.Draw resets every field to
// the shared background each frame — so, like paintPrimaryButton, this must run
// in the dialog's afterDraw hook to survive. It recolors each cell in place
// (preserving the rune and foreground the form painted) rather than blanking the
// row, so the field's current text stays visible against the lifted fill.
func (s *ConnSetting) highlightFocusedField(screen tcell.Screen) {
	if s.blurred {
		// A blank-area click cleared the visual focus but kept logical focus (the
		// form was never actually blurred — blockFormBlankClick just swallows the
		// event), so the focused InputField's TextArea still ShowCursor's its caret
		// every Draw. Hide it here so the "unfocused" field shows no lingering
		// hardware cursor either; keep the highlight clear too.
		screen.HideCursor()
		return
	}
	// tview's vertical Form aligns every field to the SAME label column — the
	// widest label plus one space (form.go: maxLabelWidth) — not each field's own
	// label width. Mirror that so the highlight starts where the field text
	// actually begins; using the per-field label width would shift shorter labels
	// (e.g. "Name:") left of the aligned field edge.
	labelW := s.formLabelWidth()
	for i := 0; i < s.form.GetFormItemCount(); i++ {
		item, ok := s.form.GetFormItem(i).(*tview.InputField)
		if !ok || !item.HasFocus() {
			continue
		}
		x, y, w, h := item.GetRect()
		if w <= 0 || h <= 0 {
			continue
		}
		for row := y; row < y+h; row++ {
			for col := x + labelW; col < x+w; col++ {
				str, style, _ := screen.Get(col, row)
				mainc, combc := decodeCell(str)
				screen.SetContent(col, row, mainc, combc, style.Background(ThemeInputFocusBG))
			}
		}
	}
}

// formLabelWidth returns the aligned label column width tview's vertical Form
// uses for all fields: the widest label plus one trailing space. It matches
// form.go's maxLabelWidth computation so overlays keyed off the field's start
// column line up with tview's own layout.
func (s *ConnSetting) formLabelWidth() int {
	maxW := 0
	for i := 0; i < s.form.GetFormItemCount(); i++ {
		if w := tview.TaggedStringWidth(s.form.GetFormItem(i).GetLabel()); w > maxW {
			maxW = w
		}
	}
	return maxW + 1
}

// paintPrimaryButton repaints the OK button in the accent color on top of what
// the form already drew. tview's Form has no per-button style and rewrites every
// button with its shared style each frame, so this is how the primary action
// gets its emphasis. It reuses the rect the form assigned the button and mirrors
// Button.Draw: fill the field, then center the label, picking the hover shade
// when the button holds focus.
func (s *ConnSetting) paintPrimaryButton(screen tcell.Screen) {
	paintPrimaryButton(screen, s.okBtn)
}

// paintPrimaryButton repaints a button in the accent color on top of what tview
// already drew, giving the dialog's primary action its emphasis. Shared by the
// connection form and the confirm dialog; both keep tview's own button for
// layout/focus/keys and only override its fill each frame via an afterDraw hook.
// No-op if the button is nil or laid out off-screen.
func paintPrimaryButton(screen tcell.Screen, btn *tview.Button) {
	if btn == nil {
		return
	}
	x, y, w, h := btn.GetRect()
	if w <= 0 || h <= 0 {
		return // laid out off-screen (e.g. dialog not visible)
	}
	bg := ThemeBtnPrimaryBG
	if btn.HasFocus() {
		bg = ThemeBtnPrimaryHoverBG
	}
	style := tcell.StyleDefault.Background(bg).Foreground(ThemeBtnPrimaryFG)
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}
	}
	tview.Print(screen, btn.GetLabel(), x, y+h/2, w, tview.AlignCenter, ThemeBtnPrimaryFG)
}

// paintStatusLine draws the Test-result message on the blank separator row tview
// leaves between the last field and the button row, so the status reads below
// the fields and above the buttons. The message carries color tags (e.g.
// "[green]Connect OK"); tview.Print parses them. The form repaints that row
// blank every frame, so this runs in afterDraw to survive — like the primary
// button. No-op when there's no message or the button hasn't been laid out.
func (s *ConnSetting) paintStatusLine(screen tcell.Screen) {
	if s.statusMsg == "" || s.okBtn == nil {
		return
	}
	fx, _, fw, _ := s.form.GetRect()
	_, by, _, _ := s.okBtn.GetRect()
	if fw <= 0 || by <= 0 {
		return // laid out off-screen (e.g. dialog not visible)
	}
	// Layout above the button row (at by), from the bottom up: by-1 is the blank
	// padding row under the spacer, by-2 is the spacer's own row, by-3 is the
	// blank padding under the last field. Painting on the spacer row (by-2) leaves
	// one blank row above (field padding) and one below (spacer padding), so the
	// status sits clearly apart from both the fields and the buttons.
	tview.Print(screen, s.statusMsg, fx, by-2, fw, tview.AlignCenter, ThemeQueryFG)
}

// address renders the host:port pair as a single Address field value.
func (s *ConnSetting) address() string {
	if s.cur.Host == "" && s.cur.Port == "" {
		return ""
	}
	return s.cur.Host + ":" + s.cur.Port
}

// defaultPort returns the conventional port for a backend, used when the user
// types a bare host into the Address field without ":port".
func defaultPort(kind string) string {
	switch kind {
	case "mongo":
		return "27017"
	case "mysql":
		return "3306"
	case "zookeeper":
		return "2181"
	default: // redis
		return "6379"
	}
}

// enableWheelScroll makes a single-line InputField scroll horizontally under the
// mouse, in two cases tview handles poorly:
//
//   - Wheel: tview drives an input field's horizontal offset purely from the
//     cursor position and re-derives it every Draw, so nudging the raw
//     columnOffset (what MouseScrollLeft/Right do) is snapped straight back — the
//     field "doesn't scroll." We translate a wheel tick into a cursor move
//     (Left/Right, plus the vertical wheel since a one-line field has no rows to
//     scroll), which shifts the viewport and sticks.
//   - Drag past the edge: TextArea's mouse handler ignores a MouseMove whose x is
//     outside the field rect, so dragging a selection rightward stops extending
//     (and scrolling) the moment the pointer leaves the visible field. We detect
//     a held-button move beyond either edge and extend the selection to that end
//     of the line with Shift+End / Shift+Home. A per-character Shift+Right won't
//     do: tcell only emits MouseMove when the pointer's cell changes, so once it
//     rests against the edge no further events arrive and a char-at-a-time scroll
//     stalls partway — the selection never reaches the end. Jumping to the line
//     end in one step lands there regardless of whether more moves follow.
//
// Synthetic keys go through the field's own InputHandler so tview updates the
// cursor exactly as it would for a real keypress.
func enableWheelScroll(field *tview.InputField) {
	press := func(key tcell.Key, mod tcell.ModMask) {
		field.InputHandler()(tcell.NewEventKey(key, 0, mod), func(tview.Primitive) {})
	}
	field.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if isLabelClick(field, action, event) {
			return tview.MouseConsumed, nil
		}
		if swallowIdleMove(action, event) {
			return tview.MouseConsumed, nil
		}
		switch action {
		case tview.MouseScrollLeft, tview.MouseScrollUp:
			press(tcell.KeyLeft, tcell.ModNone)
			return action, nil
		case tview.MouseScrollRight, tview.MouseScrollDown:
			press(tcell.KeyRight, tcell.ModNone)
			return action, nil
		case tview.MouseMove:
			// A button-less move was already swallowed above, so a real drag is in
			// progress here. Only act when the pointer has left the visible field
			// horizontally; inside the field tview's own drag-select works fine.
			// Extend to the far end in one jump (not char-by-char) so resting the
			// pointer at the edge still selects through to the end — see the note on
			// MouseMove above.
			x, _ := event.Position()
			left, right := fieldTextBounds(field)
			if x >= right {
				press(tcell.KeyEnd, tcell.ModShift)
				return action, nil
			}
			if x < left {
				press(tcell.KeyHome, tcell.ModShift)
				return action, nil
			}
		}
		return action, event
	})
}

// killCursorScrollOnClick stops a click from horizontally scrolling the field.
// InputField.Draw hard-codes its inner TextArea's minCursorPrefix to fieldWidth-1
// every frame (inputfield.go) so a *keyboard* cursor stays pinned to the right
// edge and drags the viewport with it. On a mouse click that's wrong: tview's
// moveCursor→findCursor applies the same clamp, scrolling the glyph the user
// clicked out from under the pointer (visible only once the field is already
// scrolled, e.g. after dragging to the end).
//
// The fix must live on the *inner TextArea*, not the InputField: a MouseLeftDown
// makes tview route the following MouseLeftUp straight to the TextArea
// (application.go sets it as the mouse-capturing primitive), bypassing any
// capture on the InputField — and a Draw between down and up restores the
// padding, so an InputField-level guard catches the down but not the up. The
// TextArea's own capture sees every positioning event on both paths.
//
// Keyboard navigation has the same flaw on its own path: an arrow key runs
// textArea.InputHandler → moveCursor → findCursor, which applies the identical
// clamp and (with the huge prefix) pins the cursor to the right edge, scrolling
// the text while the caret appears stuck. So we also install an InputCapture on
// the TextArea and shrink the padding before every key, giving standard editor
// scrolling (caret roams the visible window; the view only scrolls at an edge).
//
// We shrink minCursorPrefix/Suffix just before tview positions the cursor (see
// normalizeCursorPadding for why suffix stays 1, not 0), making findCursor a
// no-op for an already-visible target; the next Draw restores tview's value, and
// a positioned cursor (row>=0) skips Draw's own clamp so the offset holds until
// the user types. Reflection reaches both the unexported textArea field and its
// unexported padding fields; a silent no-op if tview's layout ever drifts.
// textAreaOf reaches an InputField's unexported inner *TextArea via reflection.
// tview drives all of an InputField's text/cursor/scroll state through this
// embedded TextArea, but doesn't expose it; several tweaks here (cursor-padding
// normalization, in-place selection collapse) need to poke it directly. Returns
// the *TextArea, its addressable struct Value (for unexported-field writes), and
// ok=false if tview's layout ever drifts so callers no-op instead of panicking.
func textAreaOf(field *tview.InputField) (*tview.TextArea, reflect.Value, bool) {
	taVal := unexportedField(reflect.ValueOf(field).Elem(), "textArea")
	if !taVal.IsValid() || taVal.IsNil() {
		return nil, reflect.Value{}, false
	}
	// taVal is an unexported field, so .Interface() would panic ("value obtained
	// from unexported field"). Re-wrap its address as an exported *TextArea.
	taVal = reflect.NewAt(taVal.Type(), unsafe.Pointer(taVal.UnsafeAddr())).Elem()
	ta, ok := taVal.Interface().(*tview.TextArea)
	if !ok || ta == nil {
		return nil, reflect.Value{}, false
	}
	return ta, reflect.ValueOf(ta).Elem(), true
}

// collapseSelectionInPlace clears a field's whole-value selection (which tview
// creates every time the field gains focus) without moving the cursor or the
// horizontal scroll. tview's own SetText(GetText()) collapses the selection too,
// but by round-tripping through Replace, which resets the caret to the text end
// and lets the next Draw's findCursor scroll a long value's viewport to the tail
// — the visible "cursor/text jumps to the end on blur." A TextArea marks "no
// selection" by selectionStart == cursor, so we copy the live cursor struct into
// selectionStart directly; nothing else about the field's state changes.
func collapseSelectionInPlace(field *tview.InputField) {
	_, taElem, ok := textAreaOf(field)
	if !ok {
		return
	}
	cur := taElem.FieldByName("cursor")
	sel := taElem.FieldByName("selectionStart")
	if !cur.IsValid() || !sel.IsValid() {
		return
	}
	// Both are unexported struct fields; rewrap sel as addressable+settable and
	// assign the cursor's current value so start==cursor (an empty selection).
	reflect.NewAt(sel.Type(), unsafe.Pointer(sel.UnsafeAddr())).Elem().
		Set(reflect.NewAt(cur.Type(), unsafe.Pointer(cur.UnsafeAddr())).Elem())
}

func normalizeCursorScroll(field *tview.InputField) {
	ta, taElem, ok := textAreaOf(field)
	if !ok {
		return
	}

	prevMouse := ta.GetMouseCapture()
	ta.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftUp, tview.MouseMove:
			normalizeCursorPadding(taElem)
		}
		if prevMouse != nil {
			return prevMouse(action, event)
		}
		return action, event
	})

	prevKey := ta.GetInputCapture()
	ta.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		normalizeCursorPadding(taElem)
		if prevKey != nil {
			return prevKey(event)
		}
		return event
	})
}

// normalizeCursorPadding shrinks the TextArea value's unexported
// minCursorPrefix/minCursorSuffix fields so findCursor keeps an already-visible
// caret put (see killCursorScrollOnClick), while still keeping the caret on
// screen at the line ends.
//
// prefix→0 lets the caret sit flush against the left edge (screen column x,
// still visible). suffix→1 (NOT 0) is the fix for the end-of-line case: the
// caret is drawn only when column-columnOffset < fieldWidth (textarea.go), and
// findCursor's right-scroll lands the caret at columnOffset = actualColumn -
// fieldWidth + minCursorSuffix. With suffix 0 the caret at the very end maps to
// screen column x+fieldWidth — exactly one cell past the visible field, so it
// vanishes. suffix 1 pulls it back to x+fieldWidth-1, the last visible cell.
func normalizeCursorPadding(taElem reflect.Value) {
	for name, val := range map[string]int64{"minCursorPrefix": 0, "minCursorSuffix": 1} {
		if f := unexportedField(taElem, name); f.IsValid() && f.CanInt() {
			reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().SetInt(val)
		}
	}
}

// unexportedField returns the named field of a struct value, addressable for
// reading even though it's unexported. Returns the zero Value if absent.
func unexportedField(v reflect.Value, name string) reflect.Value {
	if v.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	return v.FieldByName(name)
}

// isLabelClick reports whether a mouse event is a left-button press landing on
// the field's label column (left of the editable text area). tview's InputField
// treats its whole row — label included — as the hitbox, so a click on "URI:"
// would otherwise focus the field and jump the cursor to the start. Callers
// swallow such events so a label click has no effect.
func isLabelClick(field *tview.InputField, action tview.MouseAction, event *tcell.EventMouse) bool {
	switch action {
	case tview.MouseLeftDown, tview.MouseLeftUp, tview.MouseLeftClick:
		left, _ := fieldTextBounds(field)
		x, _ := event.Position()
		return x < left
	}
	return false
}

// blockLabelClick installs a mouse capture that makes clicks on the field's
// label inert (see isLabelClick). It's the whole-behavior for plain fields;
// enableWheelScroll folds the same check into its richer capture for the URI
// field, so don't apply both to one field (the second SetMouseCapture wins).
func blockLabelClick(field *tview.InputField) {
	field.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if isLabelClick(field, action, event) {
			return tview.MouseConsumed, nil
		}
		if swallowIdleMove(action, event) {
			return tview.MouseConsumed, nil
		}
		return action, event
	})
}

// swallowIdleMove reports whether a mouse-move with no button held should be
// dropped before it reaches tview's InputField/TextArea. tview tracks drag
// selection with a private `dragging` flag that it clears only on a MouseLeftUp
// landing inside the field rect. A drag whose button is released off the field
// (easy on the long, horizontally-scrolling URI field) never delivers that up,
// so `dragging` stays set — and every later button-less move over the field is
// treated as continued drag-selection, extending the highlight without any
// click. Swallowing idle moves here starves that stale path; a real drag (button
// held) still passes through.
func swallowIdleMove(action tview.MouseAction, event *tcell.EventMouse) bool {
	return action == tview.MouseMove && event.Buttons()&tcell.ButtonPrimary == 0
}

// fieldTextBounds returns the screen x range [left, right) of an InputField's
// editable text area — its inner rect start plus the label width, up to the field
// width. Used to tell when a drag has crossed the visible field's edge.
func fieldTextBounds(field *tview.InputField) (left, right int) {
	rx, _, w, _ := field.GetInnerRect()
	labelW := tview.TaggedStringWidth(field.GetLabel())
	left = rx + labelW
	right = rx + w
	if fw := field.GetFieldWidth(); fw > 0 && left+fw < right {
		right = left + fw
	}
	return left, right
}

// mongoMode returns the current mongo entry mode as a dropdown option string.
func (s *ConnSetting) mongoMode() string {
	if s.cur.URLMode {
		return "url"
	}
	return "form"
}

// text returns the text of a field if present, else "". Handles both the
// single-line InputField and a TextArea, should any field use one.
func (s *ConnSetting) text(label string) string {
	switch item := s.form.GetFormItemByLabel(label).(type) {
	case *tview.InputField:
		return item.GetText()
	case *tview.TextArea:
		return item.GetText()
	}
	return ""
}

// capture reads the currently-shown fields back into s.cur so values survive a
// rebuild when the kind changes. It only writes back fields the CURRENT layout
// actually contains: a mode/kind switch drops some fields, and reading an absent
// field returns "" (GetFormItemByLabel misses), which would otherwise clobber a
// value the user entered in the other mode — e.g. switching url→form→url must
// keep the URI. captureInto assigns only when the field is present.
func (s *ConnSetting) capture() {
	s.captureInto("Name:", &s.cur.Name)
	s.captureInto("Username:", &s.cur.User)
	s.captureInto("Auth:", &s.cur.Auth)
	s.captureInto("URI:", &s.cur.URI)
	s.captureInto("DB:", &s.cur.DB)
	if item := s.form.GetFormItemByLabel("Address:"); item != nil {
		if addr := s.text("Address:"); addr != "" {
			if h, p, ok := strings.Cut(addr, ":"); ok {
				s.cur.Host, s.cur.Port = h, p
			} else {
				// host without ":port": fall back to the backend's default port so the
				// connection still validates instead of silently failing to save.
				s.cur.Host, s.cur.Port = addr, defaultPort(s.cur.Kind)
			}
		}
	}
}

// captureInto writes the labelled field's text into dst only if that field
// exists in the current layout, so a rebuild that drops the field leaves the
// stored value untouched instead of overwriting it with "".
func (s *ConnSetting) captureInto(label string, dst *string) {
	if s.form.GetFormItemByLabel(label) != nil {
		*dst = s.text(label)
	}
}

func indexOf(items []string, v string) int {
	for i, it := range items {
		if it == v {
			return i
		}
	}
	return 0
}

// buildSetting captures the form fields and assembles a Setting, validating the
// required fields per backend. ok=false means a required field is missing.
func (s *ConnSetting) buildSetting() (Setting, bool) {
	s.capture()
	set := Setting{
		Name: s.cur.Name,
		Kind: s.cur.Kind,
		User: s.cur.User,
		Auth: s.cur.Auth,
	}
	switch s.cur.Kind {
	case "mongo":
		if s.cur.URLMode {
			if s.cur.URI == "" {
				return set, false
			}
			set.URLMode = true
			set.URI = s.cur.URI
		} else {
			if s.cur.Host == "" || s.cur.Port == "" {
				return set, false
			}
			set.Host, set.Port = s.cur.Host, s.cur.Port
			set.DB = s.cur.DB
		}
	default: // redis, mysql: host:port (+ user/auth)
		if s.cur.Host == "" || s.cur.Port == "" {
			return set, false
		}
		set.Host, set.Port = s.cur.Host, s.cur.Port
	}
	return set, true
}

func (s *ConnSetting) OnOk() {
	if s.ok != nil {
		set, ok := s.buildSetting()
		if !ok {
			return
		}
		s.ok(set, s.edit)
	}
	s.Clear()
}

// OnTest probes the connection with the current field values without saving it.
// The result is shown on the inline status line; the form stays open so the user
// can fix fields and retry.
func (s *ConnSetting) OnTest() {
	if s.test == nil {
		return
	}
	set, ok := s.buildSetting()
	if !ok {
		s.setStatus("[yellow]Fill in the required fields first")
		return
	}
	s.setStatus("[gray]Connecting...")
	if err := s.test(set); err != nil {
		s.setStatus(fmt.Sprintf("[red]Connect failed: %v", err))
	} else {
		s.setStatus("[green]Connect OK")
	}
}

// setStatus stores the inline Test-result message; paintStatusLine draws it on
// the blank row above the buttons each frame.
func (s *ConnSetting) setStatus(msg string) {
	s.statusMsg = msg
}

// copyFocusedField copies the focused input field's full value to the system
// clipboard (Ctrl-Y). It reads the field's stored text, not the on-screen
// selection, so the whole value is copied even when a long URI is scrolled
// horizontally. A brief status line confirms the copy.
func (s *ConnSetting) copyFocusedField() {
	if s.clipboard == nil {
		return
	}
	for i := 0; i < s.form.GetFormItemCount(); i++ {
		f, ok := s.form.GetFormItem(i).(*tview.InputField)
		if !ok || !f.HasFocus() {
			continue
		}
		if txt := f.GetText(); txt != "" {
			s.clipboard(txt)
			s.setStatus("[green]Copied to clipboard")
		}
		return
	}
}

// SetClipboardHandler wires the system-clipboard writer (OSC52) used by Ctrl-Y
// to copy the focused input field's value.
func (s *ConnSetting) SetClipboardHandler(f func(text string)) {
	s.clipboard = f
}

func (s *ConnSetting) OnCancel() {
	if s.cancel != nil {
		s.cancel()
	}
	s.Clear()
}

func (s *ConnSetting) Clear() {
	s.edit = false
}

func (s *ConnSetting) SetEdit(edit bool) {
	s.edit = edit
}

func (s *ConnSetting) Init(c Setting) {
	if c.Kind == "" {
		c.Kind = "redis"
	}
	// An existing mongo conn with a URI was saved in URL mode; otherwise form mode.
	if c.Kind == "mongo" {
		c.URLMode = c.URI != ""
	}
	s.cur = c
	s.build(c.Kind)
}

func (s *ConnSetting) SetCancelHandler(f func()) {
	s.cancel = f
	s.form.SetCancelFunc(f)
}

// FormPrimitive returns the inner form so the caller can hand it keyboard focus
// after Init rebuilds the fields. build() calls form.Clear(true) + re-adds every
// item, discarding whatever element Pages focused when the page was shown; the
// form is then left with no focused item, so its SetCancelFunc (Esc → close)
// receives nothing until a click focuses a field. Focusing the form here fixes
// that.
func (s *ConnSetting) FormPrimitive() tview.Primitive {
	return s.form
}

func (s *ConnSetting) SetOKHandler(f func(Setting, bool)) {
	s.ok = f
}

// SetTestHandler wires the Test button to a connectivity probe. The handler
// returns nil on success or the connection error; OnTest renders it inline.
func (s *ConnSetting) SetTestHandler(f func(Setting) error) {
	s.test = f
}
