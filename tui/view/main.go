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
	body         *tview.Flex
	leftFlexBox  *tview.Flex
	rightFlexBox *tview.Flex
	modal        *tview.Modal

	// leftWidth, when > 0, pins the tree panel to a fixed column width set by
	// dragging the divider; 0 keeps the default 1:4 proportional split.
	leftWidth   int
	dragDivider bool // a divider drag is in progress

	bottomPanel tview.Primitive
	console     *LogConsole

	opLine      *OpLine
	cmdConsole  *CmdConsole
	connSetting *ConnSetting
	search      *tview.InputField

	opBarHost *tview.Flex     // slot inside the op bar that holds the active preview's op row
	opBarItem tview.Primitive // the op row currently mounted in opBarHost (for removal)

	bottomTabs  *tview.TextView // the CONSOLE / redis-cli tab strip
	bottomPages *tview.Pages    // the panel pages switched by the tab strip
	activeTab   string          // currently selected bottom tab ("CONSOLE" / "redis-cli")
	cliVisible  bool            // whether the redis-cli tab is shown (redis backends only)

	screen tcell.Screen // live tcell screen, captured on first draw, for clipboard
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

	// A focused box otherwise draws a DOUBLE-line border (the "two lines" look).
	// Point the *Focus border runes at the same single-line glyphs so focus only
	// changes the border COLOR (brighter), not the line weight. The color swap is
	// driven per-frame by recolorBorders over panels registered via focusBorder.
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight
}

// borderBox is the subset of a bordered primitive used to color its border by
// focus state. *Box and its embedders (TreeView, Table, TextView, CmdConsole)
// and *InputField all satisfy it. Using HasFocus() (rather than focus/blur
// callbacks) is robust for InputField, whose inner textArea takes focus on a
// mouse click and never forwards Blur back to the Box — leaving a callback-based
// border stuck bright.
type borderBox interface {
	HasFocus() bool
	SetBorderColor(tcell.Color) *tview.Box
}

// focusBorders holds every panel whose border tracks focus; recolorBorders
// repaints them each frame from their live HasFocus() state.
var focusBorders []borderBox

// focusBorder registers a bordered panel so its border brightens while focused
// and dims otherwise — the visual focus cue, instead of tview's doubled border.
func focusBorder(b borderBox) {
	b.SetBorderColor(ThemeBorderDim)
	focusBorders = append(focusBorders, b)
}

// recolorBorders sets each registered panel's border color from whether it
// currently holds focus. Called before every draw.
func recolorBorders() {
	for _, b := range focusBorders {
		if b.HasFocus() {
			b.SetBorderColor(ThemeBorderFocus)
		} else {
			b.SetBorderColor(ThemeBorderDim)
		}
	}
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
	m.search.SetBorder(true)
	focusBorder(m.search) // register the InputField, not its Box: HasFocus() must
	// also see the inner textArea (which holds focus on a mouse click).
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
	m.body = body
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
	m.installDividerDrag()
}

// dividerGrab is how many columns around the tree's right border count as a grab
// zone for starting a drag.
const dividerGrab = 1

// minLeftWidth / minRightWidth keep both panels usable while dragging the divider.
const (
	minLeftWidth  = 14
	minRightWidth = 24
)

// installDividerDrag wires a mouse capture that lets the user drag the border
// between the tree panel and the preview panel to resize the tree. A left-press
// within a column of the tree's right edge starts a drag; subsequent moves pin
// the tree to a fixed width; release ends it.
func (m *MainView) installDividerDrag() {
	m.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if event == nil {
			return event, action
		}
		x, y := event.Position()
		switch action {
		case tview.MouseLeftDown:
			lx, ly, lw, lh := m.leftFlexBox.GetRect()
			edge := lx + lw - 1
			if x >= edge-dividerGrab && x <= edge+dividerGrab && y >= ly && y < ly+lh {
				// Arm the drag but don't resize yet: a plain click on the border
				// shouldn't jump the width. The move handler resizes as the mouse
				// is dragged.
				m.dragDivider = true
				return nil, action // consume: don't let the tree handle it
			}
		case tview.MouseMove:
			if m.dragDivider {
				bx, _, bw, _ := m.body.GetRect()
				m.setLeftWidth(x-bx+1, bw)
				return nil, action
			}
		case tview.MouseLeftUp:
			if m.dragDivider {
				m.dragDivider = false
				return nil, action
			}
		}
		return event, action
	})
}

// setLeftWidth pins the tree panel to width columns, clamped so neither panel
// collapses. total is the full body width used for clamping the right side.
func (m *MainView) setLeftWidth(width, total int) {
	if total <= 0 {
		return
	}
	if maxW := total - minRightWidth; width > maxW {
		width = maxW
	}
	if width < minLeftWidth {
		width = minLeftWidth
	}
	m.leftWidth = width
	m.body.ResizeItem(m.leftFlexBox, width, 0)
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
	// Capture the live tcell screen so Clipboard() can post to the system
	// clipboard (OSC52). The screen isn't available until the app starts drawing.
	m.SetAfterDrawFunc(func(screen tcell.Screen) {
		m.screen = screen
	})
	// Repaint focus-tracking panel borders from live focus state before each draw
	// (callbacks alone miss InputField blur — see focusBorder).
	m.SetBeforeDrawFunc(func(tcell.Screen) bool {
		recolorBorders()
		return false
	})
	return m.SetRoot(m.pages, true).EnableMouse(true).Run()
}

// Clipboard posts text to the system clipboard via the tcell screen (OSC52).
// No-op until the screen is available (first draw) or if text is empty.
func (m *MainView) Clipboard(text string) {
	if m.screen == nil || text == "" {
		return
	}
	m.screen.SetClipboard([]byte(text))
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

// opBarLeftWidth is the fixed width of the op bar's left segment (dropdown +
// add/edit/delete buttons). It must be wide enough to hold all of them, plus a
// gap, so the preview op row in the right segment never overlaps the buttons.
const opBarLeftWidth = opBarDropWidth + 2 + 3 + 1 + 3 + 1 + 3 + 3 // drop + gaps + 3 buttons + trailing gap

// opBar is the connection op bar. Its left segment width tracks the tree panel
// (window/5, matching the body's 1:4 split) so the preview op row in the right
// segment starts at the preview's left edge — but never shrinks below the width
// the dropdown+buttons need, so on a narrow window they can't be overlapped.
type opBar struct {
	*tview.Flex
	left *tview.Flex

	// treeWidth reports the current tree-panel width so the left segment can track
	// it. It returns 0 when the panel uses the default proportional split, in
	// which case the op bar falls back to width/5.
	treeWidth func() int
}

func (b *opBar) Draw(screen tcell.Screen) {
	_, _, width, _ := b.GetRect()
	// Align the left segment with the body's tree panel: use the dragged width
	// when set, else the default 1:4 proportion. Never shrink below the width the
	// dropdown+buttons need.
	target := width / 5
	if b.treeWidth != nil {
		if w := b.treeWidth(); w > 0 {
			target = w
		}
	}
	leftW := max(target, opBarLeftWidth)
	b.Flex.ResizeItem(b.left, leftW, 0)
	b.Flex.Draw(screen)
}

// buildOpBar builds the full-width connection op bar as two segments: a left
// segment (dropdown + add/edit/delete buttons) whose width follows the tree panel
// (clamped to a minimum), and a flexible right segment hosting the active
// preview's op row (type/size chips, reload/delete/key/rename).
func (m *MainView) buildOpBar() tview.Primitive {
	left := tview.NewFlex().SetDirection(tview.FlexColumn)
	left.SetBackgroundColor(ThemePanelBG)
	left.AddItem(m.opLine.selectDrop, opBarDropWidth, 0, false)
	left.AddItem(nil, 2, 0, false)
	left.AddItem(m.opLine.saveBtn, 3, 0, false)
	left.AddItem(nil, 1, 0, false)
	left.AddItem(m.opLine.editBtn, 3, 0, false)
	left.AddItem(nil, 1, 0, false)
	left.AddItem(m.opLine.delBtn, 3, 0, false)
	left.AddItem(nil, 0, 1, false) // absorb the trailing gap within the segment

	host := tview.NewFlex().SetDirection(tview.FlexColumn)
	host.SetBackgroundColor(ThemePanelBG)

	flex := tview.NewFlex().SetDirection(tview.FlexColumn)
	flex.SetBackgroundColor(ThemePanelBG)
	flex.AddItem(left, opBarLeftWidth, 0, false) // width is re-set each Draw
	flex.AddItem(host, 0, 1, false)              // flexible: the preview op row
	m.opBarHost = host
	return &opBar{Flex: flex, left: left, treeWidth: func() int { return m.leftWidth }}
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
	// The search box uses the standard panel border (single line, brightens on
	// focus) just like the tree/preview, instead of hand-drawn rules. A bordered
	// input is 3 rows tall (top border + field + bottom border).
	m.leftFlexBox.AddItem(m.search, 3, 0, false)
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

// BottomPanelPrimitive returns the focusable widget of the bottom panel's active
// tab — the redis-cli command console when that tab is selected, otherwise the
// CONSOLE log. Used so Tab can cycle focus into the bottom panel.
func (m *MainView) BottomPanelPrimitive() tview.Primitive {
	if m.activeTab == "redis-cli" && m.cliVisible {
		return m.cmdConsole
	}
	return m.console
}

func (m *MainView) createBottom() tview.Primitive {
	pages := tview.NewPages()

	pages.SetBackgroundColor(ThemePanelBG)
	info := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false).
		SetHighlightedFunc(func(added, removed, remaining []string) {
			// A click highlights the region; record it as the active tab, switch the
			// panel, then immediately clear tview's reverse-video highlight — the
			// active tab is colored as a themed chip by renderBottomTabs instead.
			if len(added) == 0 {
				return
			}
			m.activeTab = added[0]
			pages.SwitchToPage(added[0])
			m.bottomTabs.Highlight()
			m.renderBottomTabs(m.cliVisible)
			// Selecting the redis-cli tab drops focus straight into the command
			// console so the user can type a command without an extra Tab/click;
			// the console owns char input, so focusing it == entering edit mode.
			if added[0] == "redis-cli" && m.cmdConsole != nil {
				m.SetFocus(m.cmdConsole)
			}
		})
	info.SetBackgroundColor(ThemePanelBG)

	{
		title := "CONSOLE"
		// A custom read-only console so the user can drag-select and copy log text.
		// It follows the tail itself on each Write; redraws are driven by the app
		// event loop / QueueUpdateDraw callers.
		console := NewLogConsole(title)
		console.SetClipboardFunc(m.Clipboard)
		m.console = console
		pages.AddPage(title, console, true, true)
	}

	{
		cmd := NewCmdConsole("redis-cli")
		cmd.SetClipboardFunc(m.Clipboard)
		m.cmdConsole = cmd
		pages.AddPage(cmd.Title(), cmd, true, false)
	}

	m.bottomTabs = info
	m.bottomPages = pages
	m.renderBottomTabs(true) // default to redis (cli tab shown) until a connection is shown

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(pages, 0, 1, false).
		AddItem(info, 1, 1, false)
	layout.SetBackgroundColor(ThemePanelBG)
	return layout
}

// renderBottomTabs rebuilds the bottom tab strip. The redis-cli tab is only
// shown for redis connections (it issues raw redis commands); for other backends
// it's hidden and the panel falls back to CONSOLE. Call on every connection show.
//
// The active tab is drawn as a solid themed chip (blue fill, matching the rest
// of the selection theme) rather than tview's default reverse-video highlight;
// regions are still emitted so a click is detected, but the highlight is cleared
// in SetHighlightedFunc.
func (m *MainView) renderBottomTabs(isRedis bool) {
	if m.bottomTabs == nil {
		return
	}
	m.cliVisible = isRedis
	if m.activeTab == "" {
		m.activeTab = "CONSOLE"
	}
	// If the cli tab is hidden while it was active, fall back to CONSOLE.
	if !isRedis && m.activeTab == "redis-cli" {
		m.activeTab = "CONSOLE"
		m.bottomPages.SwitchToPage("CONSOLE")
	}

	m.bottomTabs.Clear()
	fmt.Fprint(m.bottomTabs, m.tabChip("CONSOLE"))
	if isRedis {
		fmt.Fprint(m.bottomTabs, " "+m.tabChip("redis-cli"))
	}
}

// tabChip renders one tab as a tview color-tag string: the active tab gets a
// solid blue fill with white text (a pill, padded by a space on each side); an
// inactive tab is dim text on the panel background.
func (m *MainView) tabChip(id string) string {
	if id == m.activeTab {
		return fmt.Sprintf(`["%s"][%s:%s] %s [-:-][""]`, id, tabActiveFG, tabActiveBG, id)
	}
	return fmt.Sprintf(`["%s"][%s:-] %s [-:-][""]`, id, tabInactiveFG, id)
}

// SetCliVisible shows or hides the redis-cli tab based on the active backend.
func (m *MainView) SetCliVisible(isRedis bool) {
	m.renderBottomTabs(isRedis)
}
