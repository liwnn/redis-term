package view

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// dropMinWidth pads each dropdown option to at least this display width so the
// popup list (which tview sizes to the longest option) stays comfortably wide
// instead of hugging short connection names.
const dropMinWidth = 28

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
	drop := tview.NewDropDown().SetLabel("⛁").SetFieldWidth(34) // matches opBarDropWidth (minus label) so long names show in full
	drop.SetLabelWidth(2)
	drop.SetUseStyleTags(true) // 选项用颜色标签区分 redis/mongo/mysql(见 config.GetDbNames)
	drop.SetLabelColor(ThemeQueryLabel)
	drop.SetFieldTextColor(ThemeQueryFG)
	drop.SetFieldBackgroundColor(ThemeQueryBG)
	// 焦点命中时不要用 tview 默认的白底(太刺眼),保持暗底、只把字调亮
	drop.SetFocusedStyle(tcell.StyleDefault.Background(ThemeQueryBG).Foreground(tcell.ColorWhite).Attributes(tcell.AttrBold))
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

// AddSelect add select
func (o *OpLine) AddSelect(text string) {
	if w := tview.TaggedStringWidth(text); w < dropMinWidth {
		text += strings.Repeat(" ", dropMinWidth-w) // 右留白补齐到最小宽度
	}
	text += " " // 右边固定再留一个空格
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
