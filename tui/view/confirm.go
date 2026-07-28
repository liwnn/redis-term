package view

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfirmDialog is a modal Ok/Cancel (or single-Ok) prompt styled to match the
// connection-settings dialog: a lifted dark card over a dimmed backdrop, a
// bright focus border, and an accent-blue primary button — replacing tview's
// built-in bright-blue Modal so every dialog in the app reads as one system.
type ConfirmDialog struct {
	tview.Primitive
	form   *tview.Form
	body   *tview.Flex
	card   *tview.Flex
	okBtn  *tview.Button // primary action, repainted in accent color each frame
	text   *tview.TextView
	onDone func(ok bool) // fired with true for Ok, false for Cancel/Esc
}

// NewConfirmDialog builds the dialog once; ShowModal/ShowModalOK reconfigure its
// text and buttons per prompt. It's added as a persistent page like ConnSetting.
func NewConfirmDialog() *ConfirmDialog {
	c := &ConfirmDialog{}
	c.init()
	return c
}

func (c *ConfirmDialog) init() {
	text := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetTextColor(ThemeQueryFG)
	text.SetBackgroundColor(ThemeDialogBG)

	form := tview.NewForm()
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetButtonBackgroundColor(ThemeBtnToolBG)
	form.SetButtonTextColor(ThemeBtnToolFG)
	form.SetButtonActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	form.SetBackgroundColor(ThemeDialogBG)

	// The card stacks the message over the button row, both on the lifted dialog
	// bg. A blank spacer row between them keeps the text off the buttons. The
	// bright focus border makes the card float above the dimmed backdrop, matching
	// the connection-settings dialog.
	body := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(text, 0, 1, false).
		AddItem(form, 3, 0, true)
	body.SetBackgroundColor(ThemeDialogBG)
	body.SetBorder(true).
		SetBorderColor(ThemeBorderFocus).
		SetBackgroundColor(ThemeDialogBG)

	outer, column := centered(56, 9, body)
	c.Primitive = &blockingPrimitive{
		Primitive: outer,
		afterDraw: func(screen tcell.Screen) {
			paintPrimaryButton(screen, c.okBtn)
		},
	}
	c.form = form
	c.body = body
	c.card = column
	c.text = text
}

// configure rebuilds the button row for a prompt and sizes the card to its
// contents. okOnly renders a single "Ok"; otherwise "Ok" + "Cancel".
func (c *ConfirmDialog) configure(msg string, okOnly bool) {
	c.text.SetText(msg)
	c.form.Clear(true)
	c.form.AddButton("  Ok  ", func() { c.finish(true) })
	if !okOnly {
		c.form.AddButton(" Cancel ", func() { c.finish(false) })
	}
	c.okBtn = c.form.GetButton(0)
	// Esc / the form's cancel closes as "not confirmed".
	c.form.SetCancelFunc(func() { c.finish(false) })
	c.fit(msg)
}

// fit sizes the card to hug its contents: the wrapped message height plus the
// button row and borders, so the buttons sit just below the text instead of
// floating in a fixed-height box.
func (c *ConfirmDialog) fit(msg string) {
	const cardWidth = 56
	// text is centered within the card's inner width (card minus its 2 border
	// columns and 1 padding column each side); estimate wrapped line count.
	innerW := cardWidth - 4
	lines := max((tview.TaggedStringWidth(msg)+innerW-1)/innerW, 1)
	const buttonRow, spacer, border = 3, 1, 2
	textH := lines + spacer
	c.body.ResizeItem(c.text, textH, 0)
	c.card.ResizeItem(c.body, textH+buttonRow+border, 0)
}

func (c *ConfirmDialog) finish(ok bool) {
	if c.onDone != nil {
		c.onDone(ok)
	}
}

// Show configures the dialog as an Ok/Cancel confirm and wires the done handler.
func (c *ConfirmDialog) Show(msg string, onDone func(ok bool)) {
	c.onDone = onDone
	c.configure(msg, false)
}

// ShowOK configures the dialog as a single-Ok notice.
func (c *ConfirmDialog) ShowOK(msg string, onDone func()) {
	c.onDone = func(bool) {
		if onDone != nil {
			onDone()
		}
	}
	c.configure(msg, true)
}

// FormPrimitive returns the inner form so the caller can focus it after showing
// the page (Pages focuses the page container, not the form's button).
func (c *ConfirmDialog) FormPrimitive() tview.Primitive {
	return c.form
}
