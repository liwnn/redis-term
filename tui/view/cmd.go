package view

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	PromptColor = "[#00aa00]"
	InputColor  = "[blue]"
	ResultColor = "[white]"
)

// CmdConsole is a self-contained terminal emulator primitive: a single scrollback
// buffer whose last line is the live "prompt + input" being typed, so input and
// output share one continuous surface exactly like a real CLI. It owns character
// input, the cursor, command history (↑/↓) and scrolling rather than composing a
// TextView + InputField.
type CmdConsole struct {
	*tview.Box

	lines []string // committed scrollback (output + past prompt lines), tview color tags allowed
	input []rune   // the line currently being typed
	cursor int     // caret position within input (0..len(input))

	history []string // submitted commands, for ↑/↓ recall
	histPos int      // index into history while browsing; == len(history) means "current line"

	scroll int  // index of the first visible line when not following the tail
	follow bool // true: stick to the bottom; false: hold a manual scroll offset

	viewHeight int // last drawn inner height, so Page keys can scroll by a screenful

	// Mouse selection over the (color-tagged) scrollback. Because lines carry
	// tview color tags, the selection is tracked in VISIBLE display columns, not
	// rune indices, and both the highlight and the copied text are read back from
	// the already-rendered screen cells — so what you see is exactly what copies,
	// with no tag-parsing or wide-char drift. State captured each Draw lets the
	// mouse handler (which has no screen) read cells on release.
	selecting bool
	hasSel    bool
	anchorLn  int // visual-line index into the last drawn `lines`
	anchorCol int // display column within that line
	cursorLn  int
	cursorCol int

	screen     tcell.Screen // last drawn screen, for read-back on copy
	viewX      int          // inner-rect origin captured in Draw
	viewY      int
	viewWidth  int
	firstVis   int      // first visible line index captured in Draw
	linesCache []string // the visual lines built by the last Draw

	onCopy func(text string)

	onCmdLineEnter func(string)
	title          string
	address        string
	index          int
}

// cellRune is one visible screen cell read back during selection: its primary
// rune, its start column within the line, and its display width (2 for CJK).
type cellRune struct {
	r   rune
	col int
	w   int
}

func NewCmdConsole(title string) *CmdConsole {
	c := &CmdConsole{
		title:  title,
		Box:    tview.NewBox(),
		follow: true,
	}
	c.Box.SetBorder(true)
	c.Box.SetBackgroundColor(ThemePanelBG)
	focusBorder(c.Box)
	c.histPos = 0
	return c
}

func (c *CmdConsole) Title() string { return c.title }

// SetClipboardFunc wires the system-clipboard writer used on selection release
// and double-click.
func (c *CmdConsole) SetClipboardFunc(f func(text string)) { c.onCopy = f }

func (c *CmdConsole) SetEnterHandler(handler func(string)) { c.onCmdLineEnter = handler }

// prompt renders the live prompt string, e.g. "127.0.0.1:6379:0> ".
func (c *CmdConsole) prompt() string {
	return fmt.Sprintf("%v:%v> ", c.address, c.index)
}

func (c *CmdConsole) SetPromt(address string, index int) {
	c.address = address
	c.index = index
}

func (c *CmdConsole) SetIndex(index int) { c.index = index }

// Write appends output to the scrollback. It splits on newlines and merges a
// leading fragment into the last (incomplete) output line so callers can build a
// line across multiple writes. tview color tags in p are preserved for drawing.
func (c *CmdConsole) Write(p []byte) (int, error) {
	c.appendOutput(string(p))
	return len(p), nil
}

// appendOutput pushes text into c.lines, treating '\n' as a line break. A write
// not ending in '\n' leaves the tail as the open last line for the next write.
func (c *CmdConsole) appendOutput(s string) {
	if len(c.lines) == 0 {
		c.lines = append(c.lines, "")
	}
	parts := strings.Split(s, "\n")
	for i, part := range parts {
		if i > 0 {
			c.lines = append(c.lines, "")
		}
		c.lines[len(c.lines)-1] += part
	}
	c.scrollToEnd()
}

// pushHistoryLine commits the echoed "prompt + command" as its own scrollback
// line (always a fresh line, regardless of any open output fragment).
func (c *CmdConsole) pushHistoryLine(line string) {
	// if the last line is a non-empty open fragment, start fresh
	if len(c.lines) > 0 && c.lines[len(c.lines)-1] != "" {
		c.lines = append(c.lines, line)
	} else if len(c.lines) > 0 {
		c.lines[len(c.lines)-1] = line
	} else {
		c.lines = append(c.lines, line)
	}
	c.lines = append(c.lines, "") // open a fresh line for the command's result
}

func (c *CmdConsole) submit() {
	text := string(c.input)
	c.input = c.input[:0]
	c.cursor = 0

	// echo the prompt + command into scrollback, colored like a CLI
	c.pushHistoryLine(fmt.Sprintf("%v%v%v%v", PromptColor, c.prompt(), InputColor, tview.Escape(text)))

	if strings.TrimSpace(text) != "" {
		c.history = append(c.history, text)
	}
	c.histPos = len(c.history)

	if c.onCmdLineEnter != nil {
		c.appendOutput(ResultColor) // set result color for whatever the handler writes
		c.onCmdLineEnter(text)
	}
	c.scrollToEnd()
}

// recallHistory replaces the current input with a history entry. dir -1 = older
// (↑), +1 = newer (↓).
func (c *CmdConsole) recallHistory(dir int) {
	if len(c.history) == 0 {
		return
	}
	c.histPos += dir
	if c.histPos < 0 {
		c.histPos = 0
	}
	if c.histPos >= len(c.history) {
		c.histPos = len(c.history)
		c.input = c.input[:0] // past the newest: blank line
		c.cursor = 0
		return
	}
	c.input = []rune(c.history[c.histPos])
	c.cursor = len(c.input)
}

func (c *CmdConsole) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return c.WrapInputHandler(func(event *tcell.EventKey, _ func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyEnter:
			c.submit()
		case tcell.KeyRune:
			c.input = append(c.input, 0)
			copy(c.input[c.cursor+1:], c.input[c.cursor:])
			c.input[c.cursor] = event.Rune()
			c.cursor++
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if c.cursor > 0 {
				c.input = append(c.input[:c.cursor-1], c.input[c.cursor:]...)
				c.cursor--
			}
		case tcell.KeyDelete:
			if c.cursor < len(c.input) {
				c.input = append(c.input[:c.cursor], c.input[c.cursor+1:]...)
			}
		case tcell.KeyLeft:
			if c.cursor > 0 {
				c.cursor--
			}
		case tcell.KeyRight:
			if c.cursor < len(c.input) {
				c.cursor++
			}
		case tcell.KeyHome, tcell.KeyCtrlA:
			c.cursor = 0
		case tcell.KeyEnd, tcell.KeyCtrlE:
			c.cursor = len(c.input)
		case tcell.KeyUp:
			c.recallHistory(-1)
		case tcell.KeyDown:
			c.recallHistory(1)
		case tcell.KeyPgUp:
			c.scrollBy(-c.pageStep())
		case tcell.KeyPgDn:
			c.scrollBy(c.pageStep())
		case tcell.KeyCtrlU:
			c.input = c.input[:0]
			c.cursor = 0
		}
	})
}

// MouseHandler focuses the console on a left click and supports drag-to-select
// and double-click-to-select-word over the scrollback, copying to the system
// clipboard on release (matching the CONSOLE log panel).
func (c *CmdConsole) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return c.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		px, py := event.Position()
		switch action {
		case tview.MouseLeftDown:
			if !c.InRect(px, py) {
				return false, nil
			}
			setFocus(c)
			ln, col := c.pointAt(px, py)
			c.anchorLn, c.anchorCol = ln, col
			c.cursorLn, c.cursorCol = ln, col
			c.selecting = true
			c.hasSel = false
			return true, c // capture: route drag moves here even outside the rect
		case tview.MouseMove:
			if !c.selecting {
				return false, nil
			}
			ln, col := c.pointAt(px, py)
			c.cursorLn, c.cursorCol = ln, col
			c.hasSel = true
			return true, c
		case tview.MouseLeftUp:
			if !c.selecting {
				return false, nil
			}
			c.selecting = false
			if text := c.selectedText(); text != "" && c.onCopy != nil {
				c.onCopy(text)
			}
			return true, nil
		case tview.MouseLeftDoubleClick:
			if !c.InRect(px, py) {
				return false, nil
			}
			ln, col := c.pointAt(px, py)
			if start, end, ok := c.wordSpanCols(ln, col); ok {
				c.anchorLn, c.anchorCol = ln, start
				c.cursorLn, c.cursorCol = ln, end
				c.hasSel = true
				if text := c.selectedText(); text != "" && c.onCopy != nil {
					c.onCopy(text)
				}
			}
			return true, nil
		case tview.MouseLeftClick:
			// A plain click (no drag) clears any existing selection and focuses.
			setFocus(c)
			if c.hasSel && c.anchorLn == c.cursorLn && c.anchorCol == c.cursorCol {
				c.hasSel = false
			}
			return true, nil
		case tview.MouseScrollUp:
			c.scrollBy(-3)
			return true, nil
		case tview.MouseScrollDown:
			c.scrollBy(3)
			return true, nil
		}
		return false, nil
	})
}

// pageStep is the number of lines PageUp/PageDown moves: one screenful minus a
// line of overlap so the user keeps context across a page.
func (c *CmdConsole) pageStep() int {
	return max(c.viewHeight-1, 1)
}

func (c *CmdConsole) scrollToEnd() {
	// Re-stick to the tail; Draw recomputes the offset from the live line count.
	c.follow = true
}

// totalLines is the number of visual lines, counting the live prompt+input line
// (which shares the final open scrollback fragment).
func (c *CmdConsole) totalLines() int {
	n := len(c.lines)
	if n == 0 {
		return 1 // the prompt line is always present
	}
	return n
}

// scrollBy moves the manual scroll offset by delta lines (negative = up toward
// older output). Scrolling up leaves follow mode; scrolling back to the bottom
// re-enters it so new output keeps the view pinned to the tail.
func (c *CmdConsole) scrollBy(delta int) {
	maxFirst := max(c.totalLines()-c.viewHeight, 0)
	if c.follow {
		c.scroll = maxFirst // start from the current bottom
	}
	c.scroll += delta
	if c.scroll < 0 {
		c.scroll = 0
	}
	if c.scroll >= maxFirst {
		c.scroll = maxFirst
		c.follow = true
	} else {
		c.follow = false
	}
}

func (c *CmdConsole) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	x, y, width, height := c.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	c.viewHeight = height

	// Build the full set of visual lines: committed scrollback, with the live
	// prompt+input appended to the final (open) line.
	lines := make([]string, len(c.lines))
	copy(lines, c.lines)
	if len(lines) == 0 {
		lines = append(lines, "")
	}
	promptLine := fmt.Sprintf("%v%v%v%v", PromptColor, c.prompt(), InputColor, tview.Escape(string(c.input)))
	lines[len(lines)-1] += promptLine

	// Vertical scroll: follow the tail unless the user scrolled up, in which case
	// hold their offset (clamped to the valid range).
	maxFirst := 0
	if len(lines) > height {
		maxFirst = len(lines) - height
	}
	first := maxFirst
	if !c.follow {
		first = max(min(c.scroll, maxFirst), 0)
	}
	c.scroll = first

	// Capture geometry + screen so the mouse handler can map clicks to lines and
	// read cell text back on copy.
	c.screen = screen
	c.viewX, c.viewY, c.viewWidth = x, y, width
	c.firstVis = first
	c.linesCache = lines

	// Draw at most `height` lines starting at `first`; when scrolled up this caps
	// the render to the visible window instead of spilling past the bottom border.
	last := min(first+height, len(lines))
	for i := first; i < last; i++ {
		ty := y + (i - first)
		tview.Print(screen, lines[i], x, ty, width, tview.AlignLeft, tcell.ColorWhite)
	}

	// Overlay the selection by repainting the highlighted cells: read each cell's
	// rune/style back and re-set it on the selection background, preserving the
	// foreground color. Done after Print so it sits on top.
	if c.hasSel {
		c.paintSelection(screen, last)
	}

	// While selecting, hide the hardware cursor so it doesn't blink mid-drag.
	if c.selecting {
		screen.HideCursor()
		return
	}

	// Place the hardware cursor right after the prompt + input[:cursor]. Only when
	// the prompt line is within the visible window (it scrolls off when paging up).
	if c.HasFocus() && len(lines)-1 < last {
		lastY := y + (len(lines) - 1 - first)
		// visible width of prompt + typed text up to the caret (color tags don't count)
		caretCol := tview.TaggedStringWidth(promptLineUpTo(c.prompt(), c.input, c.cursor))
		// add the width of any open fragment that the prompt was appended to
		openFrag := ""
		if n := len(c.lines); n > 0 {
			openFrag = c.lines[n-1]
		}
		caretCol += tview.TaggedStringWidth(openFrag)
		cx := x + caretCol
		if cx >= x+width {
			cx = x + width - 1
		}
		screen.ShowCursor(cx, lastY)
	}
}

// rowCells reads visual line ln back from the rendered screen as display cells.
// Lines carry color tags (and escaped input), so rather than re-parse them we
// read what was actually drawn — wide runes and all. Trailing blanks are
// dropped. Only valid for on-screen lines (the only ones the user can select).
func (c *CmdConsole) rowCells(ln int) []cellRune {
	if c.screen == nil || ln < c.firstVis || ln >= len(c.linesCache) {
		return nil
	}
	ty := c.viewY + (ln - c.firstVis)
	cells := make([]cellRune, 0, c.viewWidth)
	for col := 0; col < c.viewWidth; {
		str, _, w := c.screen.Get(c.viewX+col, ty)
		if w < 1 {
			w = 1
		}
		r := ' '
		for _, ru := range str {
			r = ru
			break
		}
		cells = append(cells, cellRune{r: r, col: col, w: w})
		col += w
	}
	// Trim trailing spaces so copied rows don't carry edge padding.
	end := len(cells)
	for end > 0 && cells[end-1].r == ' ' {
		end--
	}
	return cells[:end]
}

// normSel returns the selection ordered start<=end in (line, column) space.
func (c *CmdConsole) normSel() (sl, sc, el, ec int) {
	sl, sc, el, ec = c.anchorLn, c.anchorCol, c.cursorLn, c.cursorCol
	if el < sl || (el == sl && ec < sc) {
		sl, sc, el, ec = el, ec, sl, sc
	}
	return
}

// paintSelection repaints the cells inside the selection on the selection
// background, reading each cell back from the screen so the foreground rune and
// color are preserved. last is the exclusive bound of visible lines.
func (c *CmdConsole) paintSelection(screen tcell.Screen, last int) {
	sl, sc, el, ec := c.normSel()
	selStyle := tcell.StyleDefault.Background(ThemeSelectBG)
	for ln := max(sl, c.firstVis); ln <= el && ln < last; ln++ {
		ty := c.viewY + (ln - c.firstVis)
		startCol, endCol := c.rowSelCols(ln, sl, sc, el, ec)
		for col := startCol; col < endCol && col < c.viewWidth; col++ {
			sx := c.viewX + col
			str, st, _ := screen.Get(sx, ty)
			_, fg, _ := st.Decompose()
			r := ' '
			combc := []rune(str)
			if len(combc) > 0 {
				r, combc = combc[0], combc[1:]
			}
			screen.SetContent(sx, ty, r, combc, selStyle.Foreground(fg))
		}
	}
}

// rowSelCols returns the [start,end) display columns selected on visual line ln,
// given the normalized span. Interior lines (and the newline between rows) select
// to the panel's right edge so a multi-line selection reads as a continuous band.
func (c *CmdConsole) rowSelCols(ln, sl, sc, el, ec int) (int, int) {
	start, end := 0, c.viewWidth
	if ln == sl {
		start = sc
	}
	if ln == el {
		end = ec
	}
	return start, end
}

// pointAt maps a screen position to a (visual-line, display-column) coordinate,
// clamped to the captured viewport.
func (c *CmdConsole) pointAt(px, py int) (int, int) {
	row := max(py-c.viewY, 0)
	ln := max(c.firstVis+row, 0)
	if ln >= len(c.linesCache) {
		ln = max(len(c.linesCache)-1, 0)
	}
	col := max(px-c.viewX, 0)
	return ln, col
}

// selectedText extracts the selection as plain visible text, joining lines with
// newlines and trimming each row's trailing run of spaces (the band paints to the
// edge, but copied text shouldn't carry that padding).
func (c *CmdConsole) selectedText() string {
	if !c.hasSel {
		return ""
	}
	sl, sc, el, ec := c.normSel()
	if sl == el && sc == ec {
		return ""
	}
	var b strings.Builder
	for ln := sl; ln <= el; ln++ {
		cells := c.rowCells(ln)
		lo, hi := 0, len(cells)
		if ln == sl {
			lo = cellIndexAtCol(cells, sc)
		}
		if ln == el {
			hi = cellIndexAtCol(cells, ec)
		}
		var seg strings.Builder
		for i := lo; i < hi && i < len(cells); i++ {
			seg.WriteRune(cells[i].r)
		}
		if ln > sl {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimRight(seg.String(), " "))
	}
	return b.String()
}

// cellIndexAtCol returns the index of the first cell at or after display column
// col (len(cells) if col is past the end).
func cellIndexAtCol(cells []cellRune, col int) int {
	for i, c := range cells {
		if c.col >= col {
			return i
		}
	}
	return len(cells)
}

// wordSpanCols returns the [start,end) display-column span of the word at column
// col on visual line ln, false when col lands on whitespace or past the text.
func (c *CmdConsole) wordSpanCols(ln, col int) (int, int, bool) {
	cells := c.rowCells(ln)
	if len(cells) == 0 {
		return 0, 0, false
	}
	idx := -1
	for i, ce := range cells {
		if col >= ce.col && col < ce.col+ce.w {
			idx = i
			break
		}
	}
	if idx < 0 || !isWordRune(cells[idx].r) {
		return 0, 0, false
	}
	start, end := idx, idx+1
	for start > 0 && isWordRune(cells[start-1].r) {
		start--
	}
	for end < len(cells) && isWordRune(cells[end].r) {
		end++
	}
	return cells[start].col, cells[end-1].col + cells[end-1].w, true
}

// promptLineUpTo returns the prompt plus the input runes before the caret, used
// only to measure the caret column.
func promptLineUpTo(prompt string, input []rune, cursor int) string {
	if cursor > len(input) {
		cursor = len(input)
	}
	return prompt + string(input[:cursor])
}
