package view

import "github.com/gdamore/tcell/v2"

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
	ThemeBorderDim = tcell.GetColor("#4a5159")
	// ThemeBorderFocus is the border color of the FOCUSED panel — a bright steel
	// blue so the active panel reads clearly without drawing a second line.
	ThemeBorderFocus = tcell.GetColor("#7aa6d6")
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

	// ThemeDropFieldBG is the connection-dropdown field background (the current
	// value / top row when open): a dark fill, dimmer than ThemeInputBG, so it
	// reads as distinct from the option list below it.
	ThemeDropFieldBG = tcell.GetColor("#2a2d30")

	// ThemeSelectBG highlights the active selection (selected table cell,
	// dropdown item, tree node) with a calm blue.
	ThemeSelectBG = tcell.GetColor("#2f5d8a")

	// ThemeRowHighlightBG tints the whole row containing the selected cell. It
	// sits between the panel bg and ThemeSelectBG so the active row reads as a
	// band while the selected cell still stands out on top.
	ThemeRowHighlightBG = tcell.GetColor("#22303d")

	// Toolbar buttons (the +/e icons by the connection selector): a flat,
	// slightly lifted fill that sits cleanly on the dark panel instead of the
	// muddy remapped green. Explicit RGB avoids terminal palette remapping.
	ThemeBtnToolBG      = tcell.GetColor("#454e5a")
	ThemeBtnToolFG      = tcell.GetColor("#dce3ea")
	ThemeBtnToolHoverBG = tcell.GetColor("#5a6675")

	// Destructive buttons (Drop/Delete): a muted red that stays readable on the
	// dark panel, brightening on focus. Explicit RGB avoids palette remapping.
	ThemeBtnDangerBG      = tcell.GetColor("#5c2a2a")
	ThemeBtnDangerFG      = tcell.GetColor("#f5c2c2")
	ThemeBtnDangerHoverBG = tcell.GetColor("#8b3a3a")
)

// Bottom tab strip (CONSOLE / redis-cli): the active tab reads as a solid
// selected chip in the same blue as ThemeSelectBG, inactive tabs as dim text.
// These are tview color-tag strings (not tcell.Color) so they can be written
// directly into the TextView markup, replacing tview's reverse-video highlight.
const (
	tabActiveBG   = "#2f5d8a" // matches ThemeSelectBG
	tabActiveFG   = "white"
	tabInactiveFG = "#7c8597"
)
