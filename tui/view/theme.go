package view

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// 统一浅色主题，尽量在不同终端保持一致
var (
	// ThemePanelBG is the global panel background. tview otherwise leaves boxes
	// at ColorDefault, so the terminal's muddy mid-gray (~#626262) shows
	// through; painting every panel this near-black keeps the UI crisp.
	ThemePanelBG = tcell.GetColor("#191a1b")

	ThemeControlBG = tcell.GetColor("darkslategray")
	ThemeControlFG = tcell.ColorWhite

	ThemeBorder = tcell.GetColor("steelblue")
	// ThemeBorderDim is the border color of an UNFOCUSED panel: dim enough to
	// recede quietly against the dark panel bg. The focused panel switches to
	// ThemeBorderFocus (brighter) instead of tview's default doubled-line look.
	ThemeBorderDim = tcell.GetColor("#3d424a")
	// ThemeBorderFocus is the border color of the FOCUSED panel — a bright steel
	// blue so the active panel reads clearly without drawing a second line.
	ThemeBorderFocus = tcell.GetColor("#5a80ab")
	ThemeTitleFG     = tcell.GetColor("#c8ccd0")

	// Type/Size are rendered as muted "chips": a single slightly-lifted dark fill
	// shared by both, distinguished only by a calm colored foreground. This keeps
	// the preview toolbar consistent with the dark theme instead of the loud
	// lightcyan/lightpink blocks it used before.
	ThemeChipBG = tcell.GetColor("#252627")
	ThemeTypeFG = tcell.GetColor("#6fb3c9")
	ThemeSizeFG = tcell.GetColor("#c98fb0")

	// Query input: a slightly lifted dark fill (just above the panel bg) so the
	// field reads as a distinct box, with a dim placeholder when empty.
	ThemeQueryBG          = ThemePanelBG
	ThemeQueryFG          = tcell.ColorWhite
	ThemeQueryPlaceholder = tcell.GetColor("#6b6b6b")
	ThemeQueryLabel       = tcell.GetColor("lightsteelblue")

	// ThemeInputBG is the fill for single-line input fields (the Key box): a
	// lifted dark fill, slightly brighter than ThemeDropFieldBG, so the input
	// box reads clearly against the panel bg.
	ThemeInputBG = tcell.GetColor("#2e3238")

	// ThemeInputFocusBG is the fill for the input field that currently holds
	// keyboard focus — a lifted, cooler shade of ThemeInputBG so the active field
	// stands out from the other (dimmer) fields at a glance, beyond just the cursor.
	ThemeInputFocusBG = tcell.GetColor("#3c4657")

	// ThemeDropFieldBG is the connection-dropdown field background (the current
	// value / top row when open): a dark fill, dimmer than ThemeInputBG, so it
	// reads as distinct from the option list below it.
	ThemeDropFieldBG = tcell.GetColor("#2a2d30")

	// ThemeDialogBG is the fill of a floating dialog card — lifted above
	// ThemePanelBG so the dialog stands out from the (equally dark) main panels
	// still visible behind it.
	ThemeDialogBG = tcell.GetColor("#23262b")

	// ThemeSelectBG highlights the active selection (selected table cell,
	// dropdown item, tree node) with a calm blue.
	ThemeSelectBG = tcell.GetColor("#2f5d8a")

	// ThemeRowHighlightBG tints the whole row containing the selected cell. It
	// sits between the panel bg and ThemeSelectBG so the active row reads as a
	// band while the selected cell still stands out on top.
	ThemeRowHighlightBG = tcell.GetColor("#22303d")

	// Bottom-panel tab header: a barely-lifted background one step above the panel
	// so the header row reads as distinct from the content below it by tone alone —
	// replacing the hand-drawn rule that used to separate them.
	ThemeBottomHeaderBG = tcell.GetColor("#282a2f")

	// Toolbar buttons (the +/e icons by the connection selector, and the dialog's
	// secondary Test/Cancel actions): a lifted blue-gray fill with clear contrast
	// against both the dark panel and the plain-text field labels, so a button
	// reads as a raised, clickable chip rather than a coloured label. Explicit RGB
	// avoids terminal palette remapping.
	ThemeBtnToolBG      = tcell.GetColor("#556377")
	ThemeBtnToolFG      = tcell.GetColor("#eef2f6")
	ThemeBtnToolHoverBG = tcell.GetColor("#6c7d94")

	// Primary button (the dialog's OK / confirm action): a saturated blue that
	// stands out from the flat gray toolbar buttons so the default action reads
	// as the emphasized one. Explicit RGB avoids terminal palette remapping.
	ThemeBtnPrimaryBG      = tcell.GetColor("#2f6fd6")
	ThemeBtnPrimaryFG      = tcell.ColorWhite
	ThemeBtnPrimaryHoverBG = tcell.GetColor("#4a86ea")

	// Destructive buttons (Drop/Delete): a muted red that stays readable on the
	// dark panel, brightening on focus. Explicit RGB avoids palette remapping.
	ThemeBtnDangerBG      = tcell.GetColor("#5c2a2a")
	ThemeBtnDangerFG      = tcell.GetColor("#f5c2c2")
	ThemeBtnDangerHoverBG = tcell.GetColor("#8b3a3a")
)

// Bottom tab strip (CONSOLE / cli): the active tab reads as a solid
// selected chip in the same blue as ThemeSelectBG, inactive tabs as dim text.
// These are tview color-tag strings (not tcell.Color) so they can be written
// directly into the TextView markup, replacing tview's reverse-video highlight.
const (
	tabActiveBG   = "#26466a" // a deep muted blue: clearly marks the active tab without pulling focus
	tabActiveFG   = "white"
	tabInactiveBG = "#454b54" // a clearly lifted gray so an inactive tab reads as a solid clickable pill
	tabInactiveFG = "#c3cad6"
)

// typeBadgeColors maps lowercase Redis / datasource type names to a background
// color tag string used to render a coloured badge (e.g. "[white:#3465a4:b] HASH [-:-:-]").
var typeBadgeColors = map[string]string{
	"string":     "#5c6bc0", // dark blue-cyan (more blue)
	"hash":       "#3465a4", // blue
	"zset":       "#7d387f", // deeper light orchid/purple (slightly more purple)
	"set":        "#723030", // warm maroon/red-brown
	"list":       "#6e5223", // muted amber/yellow-brown
	"stream":     "#1e6e2e", // greener dark green (increased green brightness slightly)
	"collection": "#5d4037", // brown (mongo)
	"table":      "#455a64", // blue-gray (mysql)
	"znode":      "#616161", // gray (zookeeper)
}

// TypeBadge returns a tview color-tagged string that renders as an uppercase
// type name on a coloured background chip (e.g. "[white:#3465a4:b] HASH [-:-:-]").
// If the type is empty it returns "".
func TypeBadge(typeName string) string {
	if typeName == "" {
		return ""
	}
	bg, ok := typeBadgeColors[typeName]
	if !ok {
		bg = "#546e7a" // default steel
	}
	return "[white:" + bg + ":b] " + strings.ToUpper(typeName) + " [-:-:-]"
}

// TypeBadgeColor returns the tcell.Color for the given type name's badge
// background color, for use with SetBackgroundColor on a tview widget.
func TypeBadgeColor(typeName string) tcell.Color {
	bg, ok := typeBadgeColors[typeName]
	if !ok {
		bg = "#546e7a"
	}
	return tcell.GetColor(bg)
}
