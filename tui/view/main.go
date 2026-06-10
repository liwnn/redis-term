package view

import (
	"fmt"
	"io"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// MainView main
type MainView struct {
	*tview.Application
	pages        *tview.Pages
	leftFlexBox  *tview.Flex
	rightFlexBox *tview.Flex
	modal        *tview.Modal

	bottomPanel tview.Primitive
	console     *tview.TextView

	opLine      *OpLine
	cmdConsole  *CmdConsole
	connSetting *ConnSetting
	search      *tview.InputField

	opBar     *tview.Flex     // full-width top bar: dropdown + buttons + preview op row
	opBarHost *tview.Flex     // slot inside opBar that holds the active preview's op row
	opBarItem tview.Primitive // the op row currently mounted in opBarHost (for removal)
}

// NewMainView new
func NewMainView() *MainView {
	applyGlobalStyles()
	m := &MainView{
		Application: tview.NewApplication(),
	}
	m.init()
	return m
}

// applyGlobalStyles replaces tview's washed-out default palette (a muddy
// ~#626262 gray) with a clean, solid dark theme so every panel renders a crisp
// background instead of inheriting the default gray.
func applyGlobalStyles() {
	tview.Styles.PrimitiveBackgroundColor = ThemePanelBG
	tview.Styles.ContrastBackgroundColor = ThemeSelectBG
	tview.Styles.MoreContrastBackgroundColor = ThemeSelectBG
	tview.Styles.PrimaryTextColor = tcell.ColorWhite
	tview.Styles.BorderColor = ThemeBorderDim
	tview.Styles.TitleColor = ThemeTitleFG
	tview.Styles.GraphicsColor = ThemeBorderDim
}

func (m *MainView) init() {
	m.opLine = NewOpLine()
	m.opLine.SetSaveClickFunc(func() {
		m.pages.ShowPage("conn_setting")
		m.connSetting.SetEdit(false)
		m.connSetting.Init(Setting{Kind: "redis"})
	})
	m.search = tview.NewInputField()
	m.search.SetLabel(" 🔍 ").
		SetLabelColor(ThemeQueryLabel).
		SetPlaceholder("search keys… (type to filter)").
		SetFieldStyle(tcell.StyleDefault.Background(ThemeQueryBG).Foreground(ThemeQueryFG)).
		SetPlaceholderStyle(tcell.StyleDefault.Background(ThemeQueryBG).Foreground(ThemeQueryPlaceholder))
	m.search.SetBackgroundColor(ThemeQueryBG)
	m.leftFlexBox = tview.NewFlex().SetDirection(tview.FlexRow)
	m.rightFlexBox = tview.NewFlex().SetDirection(tview.FlexRow)
	m.leftFlexBox.SetBackgroundColor(ThemePanelBG)
	m.rightFlexBox.SetBackgroundColor(ThemePanelBG)
	m.modal = m.createModal()
	// body holds the two side-by-side panels; the connection op bar sits above it
	// spanning the full window width so a long connection name isn't clipped to the
	// narrow left panel.
	body := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(m.leftFlexBox, 0, 1, true).
		AddItem(m.rightFlexBox, 0, 4, false)
	body.SetBackgroundColor(ThemePanelBG)
	mainFlexBox := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.buildOpBar(), 1, 0, false).
		AddItem(body, 0, 1, true)
	mainFlexBox.SetBackgroundColor(ThemePanelBG)
	m.connSetting = NewConnSetting()
	m.connSetting.SetCancelHandler(func() {
		m.pages.HidePage("conn_setting")
	})
	m.pages = tview.NewPages()
	m.pages.AddPage("main", mainFlexBox, true, true)
	m.pages.AddPage("conn_setting", m.connSetting, true, false)
	m.pages.AddPage("modal", m.modal, true, false) // 置顶:测试结果等弹窗要盖在设置表单之上

	m.bottomPanel = m.createBottom()
}

// hRule returns a one-row box that draws a thin horizontal line in the dim
// rule color, matching the right preview panel's top border.
func hRule() *tview.Box {
	box := tview.NewBox()
	box.SetBackgroundColor(ThemePanelBG)
	box.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		st := tcell.StyleDefault.Background(ThemePanelBG).Foreground(ThemeRule)
		cy := y + height/2
		for cx := x; cx < x+width; cx++ {
			screen.SetContent(cx, cy, tview.BoxDrawingsLightHorizontal, nil, st)
		}
		return 0, 0, 0, 0
	})
	return box
}

// vRule returns a one-column box that draws a thin vertical line in the dim
// rule color over the given background, used to frame the search box. The
// background matches the field fill so the line sits flush against it with no
// dark gap.
func vRule(bg tcell.Color) *tview.Box {
	box := tview.NewBox()
	box.SetBackgroundColor(bg)
	box.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		st := tcell.StyleDefault.Background(bg).Foreground(ThemeRule)
		cx := x + width/2
		for cy := y; cy < y+height; cy++ {
			screen.SetContent(cx, cy, tview.BoxDrawingsLightVertical, nil, st)
		}
		return 0, 0, 0, 0
	})
	return box
}

func (m *MainView) createModal() *tview.Modal {
	modal := tview.NewModal()
	return modal
}

func (m *MainView) ShowConnSetting(cfg Setting, edit bool) {
	m.pages.ShowPage("conn_setting")
	m.connSetting.SetEdit(edit)
	m.connSetting.Init(cfg)
}

func (m *MainView) HideConnSetting() {
	m.pages.HidePage("conn_setting")
}

// ShowModal show modal
func (m *MainView) ShowModal(text string, okFunc func()) {
	m.modal.ClearButtons()
	m.modal.AddButtons([]string{"Ok", "Cancel"})
	m.modal.SetText(text).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 && okFunc != nil {
				okFunc()
			}
			m.pages.HidePage("modal")
		})
	m.pages.ShowPage("modal")
}

func (m *MainView) ShowModalOK(text string) {
	m.modal.ClearButtons()
	m.modal.AddButtons([]string{"Ok"})
	m.modal.SetText(text).SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		m.pages.HidePage("modal")
	})
	m.pages.ShowPage("modal")
}

// Run run
func (m *MainView) Run() error {
	return m.SetRoot(m.pages, true).EnableMouse(true).Run()
}

// SetGlobalInputCapture installs an application-wide key handler.
func (m *MainView) SetGlobalInputCapture(f func(event *tcell.EventKey) *tcell.EventKey) {
	m.Application.SetInputCapture(f)
}

func (m *MainView) RefreshOpLine(names []string, handler func(index int)) {
	m.opLine.ClearAllSelect()
	for _, name := range names {
		m.opLine.AddSelect(name)
	}
	m.opLine.SetSelectedFunc(handler)
}

func (m *MainView) GetOpLine() *OpLine {
	return m.opLine
}

func (m *MainView) GetConnSetting() *ConnSetting {
	return m.connSetting
}

// opBarDropWidth is the fixed cell width of the connection dropdown in the op
// bar — wide enough for long "[KIND] host:port" labels without clipping, while
// keeping the add/edit/delete buttons right next to it (not pushed to the far
// right of the window).
const opBarDropWidth = 36

// buildOpBar builds the full-width connection op bar: a fixed-width dropdown (wide
// enough that long names aren't clipped), the add/edit/delete buttons, then a
// flexible host slot that carries the active preview's op row (type/size chips,
// reload/delete/key/rename) — moved here from above the value area.
func (m *MainView) buildOpBar() *tview.Flex {
	opBar := tview.NewFlex().SetDirection(tview.FlexColumn)
	opBar.SetBackgroundColor(ThemePanelBG)
	opBar.AddItem(m.opLine.selectDrop, opBarDropWidth, 0, false)
	opBar.AddItem(nil, 2, 0, false)
	opBar.AddItem(m.opLine.saveBtn, 3, 0, false)
	opBar.AddItem(nil, 1, 0, false)
	opBar.AddItem(m.opLine.editBtn, 3, 0, false)
	opBar.AddItem(nil, 1, 0, false)
	opBar.AddItem(m.opLine.delBtn, 3, 0, false)
	opBar.AddItem(nil, 4, 0, false) // gap before the preview op row (Type chip etc.)
	host := tview.NewFlex().SetDirection(tview.FlexColumn)
	host.SetBackgroundColor(ThemePanelBG)
	opBar.AddItem(host, 0, 1, false) // flexible: holds the preview op row
	m.opBar = opBar
	m.opBarHost = host
	return opBar
}

// SetPreviewOpBar mounts a preview's op row into the top bar's host slot,
// replacing whatever was there. Passing nil clears it.
func (m *MainView) SetPreviewOpBar(row tview.Primitive) {
	if m.opBarItem != nil {
		m.opBarHost.RemoveItem(m.opBarItem)
	}
	m.opBarItem = row
	if row != nil {
		m.opBarHost.AddItem(row, 0, 1, false)
	}
}

func (m *MainView) SetTree(tree tview.Primitive) {
	m.leftFlexBox.Clear()
	// A thin horizontal rule above the search box, aligned and color-matched
	// with the right preview panel's top border (its row 1 here).
	m.leftFlexBox.AddItem(hRule(), 1, 0, false)
	// Frame the search box with a thin vertical rule on each side. The rules
	// share the field's fill color so the whole row reads as one boxed input.
	searchRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	searchRow.SetBackgroundColor(ThemeQueryBG)
	searchRow.AddItem(vRule(ThemeQueryBG), 1, 0, false)
	searchRow.AddItem(m.search, 0, 1, false)
	searchRow.AddItem(vRule(ThemeQueryBG), 1, 0, false)
	m.leftFlexBox.AddItem(searchRow, 1, 0, false)
	m.leftFlexBox.AddItem(tree, 0, 1, true)
}

// GetSearch returns the key-search input field.
func (m *MainView) GetSearch() *tview.InputField {
	return m.search
}

func (m *MainView) SetPreview(preview *tview.Flex) {
	m.rightFlexBox.Clear()
	m.rightFlexBox.AddItem(preview, 0, 3, false)
	m.rightFlexBox.AddItem(m.bottomPanel, 0, 1, false)
}

func (m *MainView) GetOutput() io.Writer {
	return m.console
}

func (m *MainView) GetCmd() *CmdConsole {
	return m.cmdConsole
}

func (m *MainView) createBottom() tview.Primitive {
	pages := tview.NewPages()

	pages.SetBackgroundColor(ThemePanelBG)
	info := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false).
		SetHighlightedFunc(func(added, removed, remaining []string) {
			pages.SwitchToPage(added[0])
		})
	info.SetBackgroundColor(ThemePanelBG)

	{
		title := "CONSOLE"
		console := tview.NewTextView()
		console.
			SetScrollable(true).
			SetTitle(title).
			SetBorder(true)
		console.SetBackgroundColor(ThemePanelBG)
		m.console = console
		pages.GetPageCount()
		pages.AddPage(title, console, true, true)
		fmt.Fprintf(info, `["%v"][#9aa6c2]%s[white][""] `, title, title)
	}

	{
		cmd := NewCmdConsole("redis-cli")
		m.cmdConsole = cmd

		pages.AddPage(cmd.Title(), cmd, true, false)
		fmt.Fprintf(info, `["%v"][#9aa6c2]%s[white][""] `, cmd.Title(), cmd.Title())
	}

	info.Highlight("CONSOLE")

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(pages, 0, 1, false).
		AddItem(info, 1, 1, false)
	layout.SetBackgroundColor(ThemePanelBG)
	return layout
}
