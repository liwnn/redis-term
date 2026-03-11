package view

import "github.com/gdamore/tcell/v2"

// 统一浅色主题，尽量在不同终端保持一致
var (
	ThemeControlBG = tcell.GetColor("darkslategray")
	ThemeControlFG = tcell.ColorWhite

	ThemeBorder = tcell.GetColor("steelblue")

	ThemeTypeBG  = tcell.GetColor("lightcyan")
	ThemeSizeBG  = tcell.GetColor("lightpink")
	ThemeLabelFG = tcell.ColorBlack

	// Semantic button colors using 16-color palette for terminal consistency
	ThemeBtnReloadBG = tcell.ColorBlue
	ThemeBtnReloadFG = tcell.ColorWhite
	ThemeBtnDeleteBG = tcell.ColorRed
	ThemeBtnDeleteFG = tcell.ColorWhite
	ThemeBtnRenameBG = tcell.ColorGreen
	ThemeBtnRenameFG = tcell.ColorBlack
)
