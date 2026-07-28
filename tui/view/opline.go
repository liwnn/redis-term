package view

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// dropFieldWidth is the closed dropdown field's display width (excluding the
// label). dropCaretSuffix is appended to the closed value as a ▼ hint.
const (
	dropFieldWidth  = 34
	dropCaretSuffix = " ▼ " // leading space + caret + one trailing space
)

type OpLine struct {
	*tview.Flex
	selectDrop  *tview.DropDown
	saveBtn     *tview.Button
	editBtn     *tview.Button
	delBtn      *tview.Button
	saveHandler func()
	editHandler func()
	delHandler  func()
}

func NewOpLine() *OpLine {
	o := &OpLine{}
	o.init()
	return o
}

func (o *OpLine) init() {
	drop := tview.NewDropDown().SetLabel("⛁").SetFieldWidth(dropFieldWidth) // matches opBarDropWidth (minus label) so long names show in full
	drop.SetLabelWidth(2)
	drop.SetUseStyleTags(true) // 选项用颜色标签区分 redis/mongo/mysql(见 config.GetDbNames)
	// 闭合态字段右侧加一个 ▼ 提示符,让它一眼区别于普通文本输入框(两者背景色几乎一致)。
	// currentSuffix 只作用于闭合显示,不污染下拉列表;AddSelect 已把选项右补齐到
	// dropFieldWidth-len(▼),故箭头正好落在字段右缘、右侧仅留一个空格。
	drop.SetTextOptions("", "", "", dropCaretSuffix, "")
	drop.SetLabelColor(ThemeQueryLabel)
	drop.SetFieldTextColor(ThemeQueryFG)
	// The field (the closed value / top row when open) gets a lifted background so
	// it reads as distinct from the option list below it, which shares the dark
	// ThemeQueryBG — otherwise the current value blends into the popup.
	drop.SetFieldBackgroundColor(ThemeDropFieldBG)
	// 焦点命中时不要用 tview 默认的白底(太刺眼),保持暗底、只把字调亮
	drop.SetFocusedStyle(tcell.StyleDefault.Background(ThemeDropFieldBG).Foreground(tcell.ColorWhite).Attributes(tcell.AttrBold))
	// 下拉列表:与暗色面板一致,选中项用柔和蓝色高亮
	normal := tcell.StyleDefault.Foreground(ThemeQueryFG).Background(ThemeQueryBG)
	selected := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(ThemeSelectBG)
	drop.SetListStyles(normal, selected)

	saveBtn := tview.NewButton("+")
	saveBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	saveBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	saveBtn.SetSelectedFunc(func() {
		if o.saveHandler != nil {
			o.saveHandler()
		}
	})

	editBtn := tview.NewButton("✎")
	editBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	editBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	editBtn.SetSelectedFunc(func() {
		if o.editHandler != nil {
			o.editHandler()
		}
	})

	delBtn := tview.NewButton("✕")
	delBtn.SetStyle(tcell.StyleDefault.Background(ThemeBtnToolBG).Foreground(ThemeBtnToolFG))
	delBtn.SetActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	delBtn.SetSelectedFunc(func() {
		if o.delHandler != nil {
			o.delHandler()
		}
	})

	flex := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(drop, 0, 1, false).
		AddItem(saveBtn, 3, 1, false).
		AddItem(editBtn, 3, 1, false).
		AddItem(delBtn, 3, 1, false)

	o.Flex = flex
	o.selectDrop = drop
	o.saveBtn = saveBtn
	o.editBtn = editBtn
	o.delBtn = delBtn
}

// AddSelect adds a connection option. The visible label is right-padded so that
// label + caret suffix fills the field exactly — that keeps the ▼ flush at the
// field's right edge (with one trailing space) instead of floating mid-field
// with an empty colored box after it.
func (o *OpLine) AddSelect(text string) {
	pad := dropFieldWidth - tview.TaggedStringWidth(dropCaretSuffix)
	if w := tview.TaggedStringWidth(text); w < pad {
		text += strings.Repeat(" ", pad-w)
	}
	o.selectDrop.AddOption(text, nil)
}
func (o *OpLine) ClearAllSelect() {
	o.selectDrop.SetOptions(nil, nil)
}

// Select db
func (o *OpLine) Select(index int) {
	o.selectDrop.SetCurrentOption(index)
}

// SelectDropPrimitive returns the connection dropdown for external focus
// switching (Ctrl-P jumps here from anywhere).
func (o *OpLine) SelectDropPrimitive() tview.Primitive {
	return o.selectDrop
}

// OpenSelectDrop focuses the connection dropdown and pops its option list open
// in one step, so Ctrl-P lands the user directly in the list instead of on a
// closed field they'd then have to press Enter to open. setFocus is the app's
// focus setter (tview's DropDown opens its list via that callback). An Enter
// event routed through the dropdown's own InputHandler opens the list exactly
// the way a manual Enter would.
func (o *OpLine) OpenSelectDrop(setFocus func(p tview.Primitive)) {
	setFocus(o.selectDrop)
	if h := o.selectDrop.InputHandler(); h != nil {
		h(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), setFocus)
	}
}

// CloseSelectDrop collapses the dropdown's option list if it's open, without
// changing the selection. It routes an Esc through the dropdown's own
// InputHandler (which invokes tview's internal closeList); the caller restores
// focus to wherever it wants afterward.
func (o *OpLine) CloseSelectDrop() {
	if !o.selectDrop.IsOpen() {
		return
	}
	if h := o.selectDrop.InputHandler(); h != nil {
		h(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone), func(tview.Primitive) {})
	}
}

func (o *OpLine) GetOptionCount() int {
	return o.selectDrop.GetOptionCount()
}

func (o *OpLine) GetSelect() int {
	index, _ := o.selectDrop.GetCurrentOption()
	return index
}

func (o *OpLine) SetSelectedFunc(handler func(index int)) {
	o.selectDrop.SetSelectedFunc(func(text string, index int) {
		handler(index)
	})
}

func (o *OpLine) SetSaveClickFunc(handler func()) {
	o.saveHandler = handler
}

func (o *OpLine) SetEditClickFunc(handler func()) {
	o.editHandler = handler
}

func (o *OpLine) SetDeleteClickFunc(handler func()) {
	o.delHandler = handler
}
