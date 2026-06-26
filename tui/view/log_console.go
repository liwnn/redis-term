package view

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// maxLogLines caps the scrollback so a long-running session doesn't grow the
// buffer without bound. When the cap is exceeded the oldest lines are purged and
// the stored selection / scroll offsets are shifted to stay anchored.
const maxLogLines = 5000

// LogConsole is a read-only, scrollable text panel for the CONSOLE log that
// supports mouse drag-to-select and copy-on-release. tview's TextView can't
// select text while the app holds the mouse (EnableMouse intercepts the
// terminal's native selection), so this primitive does its own selection:
// a left-drag highlights a span, and releasing copies it to the system
// clipboard via the injected writer (OSC52). It implements io.Writer so it is a
// drop-in target for the logger.
type LogConsole struct {
	*tview.Box

	lines []string // committed scrollback, plain text (no color tags)

	scroll  int  // index of the first visible line when not following the tail
	hscroll int  // horizontal column offset (for no-wrap precise selection)
	follow  bool // stick to the bottom so new log lines stay visible

	viewHeight int // last drawn inner height, for Page scrolling
	viewWidth  int // last drawn inner width

	// Selection span in (line, rune-index) coordinates, normalized lazily. selecting
	// is true between a left-press and release; a zero-length span copies nothing.
	selecting bool
	hasSel    bool
	anchorLn  int
	anchorRn  int
	cursorLn  int
	cursorRn  int

	onCopy func(text string)
}

func NewLogConsole(title string) *LogConsole {
	c := &LogConsole{
		Box:    tview.NewBox(),
		follow: true,
	}
	c.Box.SetBorder(true).SetTitle(title)
	c.Box.SetBackgroundColor(ThemePanelBG)
	focusBorder(c.Box)
	return c
}

// SetClipboardFunc wires the system-clipboard writer used on selection release.
func (c *LogConsole) SetClipboardFunc(f func(text string)) { c.onCopy = f }

// Write appends log output, splitting on newlines. A fragment not ending in
// '\n' is held as the open last line for the next write, mirroring a real
// terminal. Each append re-sticks to the tail.
func (c *LogConsole) Write(p []byte) (int, error) {
	if len(c.lines) == 0 {
		c.lines = append(c.lines, "")
	}
	parts := strings.Split(string(p), "\n")
	for i, part := range parts {
		if i > 0 {
			c.lines = append(c.lines, "")
		}
		c.lines[len(c.lines)-1] += part
	}
	c.purge()
	c.follow = true
	return len(p), nil
}

// purge trims the oldest lines past the cap, shifting selection/scroll so they
// keep pointing at the same logical lines.
func (c *LogConsole) purge() {
	if len(c.lines) <= maxLogLines {
		return
	}
	drop := len(c.lines) - maxLogLines
	c.lines = c.lines[drop:]
	c.scroll = max(c.scroll-drop, 0)
	c.anchorLn -= drop
	c.cursorLn -= drop
	if c.anchorLn < 0 || c.cursorLn < 0 {
		c.hasSel = false // selection scrolled off the top
	}
}

// runeWidth is the display-cell width of a single rune (CJK = 2).
func runeWidth(r rune) int {
	return max(tview.TaggedStringWidth(string(r)), 1)
}

// colToRune maps a display column within a line to the index of the rune that
// covers it (clamped to len). Used to turn a click x into a selection point.
func colToRune(line string, col int) int {
	if col <= 0 {
		return 0
	}
	cell := 0
	i := 0
	for _, r := range []rune(line) {
		w := runeWidth(r)
		if col < cell+w {
			return i
		}
		cell += w
		i++
	}
	return i
}

func (c *LogConsole) totalLines() int {
	if len(c.lines) == 0 {
		return 1
	}
	return len(c.lines)
}

// normSel returns the selection span ordered start<=end as (line,rune) pairs.
func (c *LogConsole) normSel() (sl, sr, el, er int) {
	sl, sr, el, er = c.anchorLn, c.anchorRn, c.cursorLn, c.cursorRn
	if el < sl || (el == sl && er < sr) {
		sl, sr, el, er = el, er, sl, sr
	}
	return
}

// selectedText extracts the current selection as plain text, joining spanned
// lines with newlines.
func (c *LogConsole) selectedText() string {
	if !c.hasSel {
		return ""
	}
	sl, sr, el, er := c.normSel()
	if sl == el && sr == er {
		return ""
	}
	clampLine := func(ln int) int { return max(0, min(ln, len(c.lines)-1)) }
	sl, el = clampLine(sl), clampLine(el)
	if sl == el {
		rs := []rune(c.lines[sl])
		return string(rs[min(sr, len(rs)):min(er, len(rs))])
	}
	var b strings.Builder
	first := []rune(c.lines[sl])
	b.WriteString(string(first[min(sr, len(first)):]))
	for ln := sl + 1; ln < el; ln++ {
		b.WriteByte('\n')
		b.WriteString(c.lines[ln])
	}
	b.WriteByte('\n')
	last := []rune(c.lines[el])
	b.WriteString(string(last[:min(er, len(last))]))
	return b.String()
}

// isSelected reports whether the rune at (ln, rn) falls inside the selection.
func (c *LogConsole) isSelected(ln, rn int) bool {
	if !c.hasSel {
		return false
	}
	sl, sr, el, er := c.normSel()
	if ln < sl || ln > el {
		return false
	}
	if sl == el {
		return rn >= sr && rn < er
	}
	if ln == sl {
		return rn >= sr
	}
	if ln == el {
		return rn < er
	}
	return true // a full interior line
}

// selSpansNewline reports whether the selection includes the line break after
// ln, so Draw can paint the trailing area to show the newline is selected.
func (c *LogConsole) selSpansNewline(ln int) bool {
	if !c.hasSel {
		return false
	}
	sl, _, el, _ := c.normSel()
	return ln >= sl && ln < el
}

func (c *LogConsole) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return c.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		x, y := event.Position()
		switch action {
		case tview.MouseLeftDown:
			if !c.InRect(x, y) {
				return false, nil
			}
			setFocus(c)
			ln, rn := c.pointAt(x, y)
			c.anchorLn, c.anchorRn = ln, rn
			c.cursorLn, c.cursorRn = ln, rn
			c.selecting = true
			c.hasSel = false
			return true, c // capture: route drag moves here even outside the rect
		case tview.MouseMove:
			if !c.selecting {
				return false, nil
			}
			ln, rn := c.pointAt(x, y)
			c.cursorLn, c.cursorRn = ln, rn
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
			if !c.InRect(x, y) {
				return false, nil
			}
			ln, rn := c.pointAt(x, y)
			if start, end, ok := c.wordSpan(ln, rn); ok {
				c.anchorLn, c.anchorRn = ln, start
				c.cursorLn, c.cursorRn = ln, end
				c.hasSel = true
				if text := c.selectedText(); text != "" && c.onCopy != nil {
					c.onCopy(text)
				}
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

// pointAt maps a screen position to a (line, rune) coordinate, clamped to the
// content. Positions below the last line clamp to the end; above clamp to top.
func (c *LogConsole) pointAt(x, y int) (int, int) {
	ix, iy, _, _ := c.GetInnerRect()
	top := c.topLine()
	row := max(y-iy, 0)
	ln := max(top+row, 0)
	if ln >= len(c.lines) {
		ln = max(len(c.lines)-1, 0)
	}
	line := ""
	if ln < len(c.lines) {
		line = c.lines[ln]
	}
	col := x - ix + c.hscroll
	return ln, colToRune(line, col)
}

// isWordRune reports whether r counts as part of a word for double-click
// selection: letters, digits, and the punctuation common in keys/paths/numbers
// (so "user:1001", "/api/v1", "3.14" select as one token).
func isWordRune(r rune) bool {
	if r == '_' || r == '-' || r == ':' || r == '.' || r == '/' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
		return true
	}
	return r > 127 // treat CJK and other non-ASCII as word characters
}

// wordSpan returns the [start,end) rune range of the word at (ln, rn). ok is
// false when the click lands on whitespace or past the line's end (nothing to
// select).
func (c *LogConsole) wordSpan(ln, rn int) (int, int, bool) {
	if ln < 0 || ln >= len(c.lines) {
		return 0, 0, false
	}
	rs := []rune(c.lines[ln])
	if rn >= len(rs) || !isWordRune(rs[rn]) {
		return 0, 0, false
	}
	start, end := rn, rn+1
	for start > 0 && isWordRune(rs[start-1]) {
		start--
	}
	for end < len(rs) && isWordRune(rs[end]) {
		end++
	}
	return start, end, true
}

// topLine computes the first visible line index from follow/scroll state.
func (c *LogConsole) topLine() int {
	maxTop := max(c.totalLines()-c.viewHeight, 0)
	if c.follow {
		return maxTop
	}
	return max(0, min(c.scroll, maxTop))
}

func (c *LogConsole) pageStep() int { return max(c.viewHeight-1, 1) }

// scrollBy moves the vertical offset; reaching the bottom re-enters follow mode.
func (c *LogConsole) scrollBy(delta int) {
	maxTop := max(c.totalLines()-c.viewHeight, 0)
	if c.follow {
		c.scroll = maxTop
	}
	c.scroll += delta
	if c.scroll < 0 {
		c.scroll = 0
	}
	if c.scroll >= maxTop {
		c.scroll = maxTop
		c.follow = true
	} else {
		c.follow = false
	}
}

func (c *LogConsole) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return c.WrapInputHandler(func(event *tcell.EventKey, _ func(p tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyUp:
			c.scrollBy(-1)
		case tcell.KeyDown:
			c.scrollBy(1)
		case tcell.KeyPgUp:
			c.scrollBy(-c.pageStep())
		case tcell.KeyPgDn:
			c.scrollBy(c.pageStep())
		case tcell.KeyHome:
			c.scroll = 0
			c.follow = false
		case tcell.KeyEnd:
			c.follow = true
		case tcell.KeyLeft:
			if c.hscroll > 0 {
				c.hscroll -= 4
				if c.hscroll < 0 {
					c.hscroll = 0
				}
			}
		case tcell.KeyRight:
			c.hscroll += 4
		}
	})
}

func (c *LogConsole) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	x, y, width, height := c.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	c.viewWidth = width
	c.viewHeight = height

	base := tcell.StyleDefault.Background(ThemePanelBG).Foreground(tcell.ColorWhite)
	sel := tcell.StyleDefault.Background(ThemeSelectBG).Foreground(tcell.ColorWhite)

	top := c.topLine()
	c.scroll = top
	for row := range height {
		ln := top + row
		if ln >= len(c.lines) {
			break
		}
		cell := 0 // display column within the line
		rn := 0
		for _, r := range []rune(c.lines[ln]) {
			w := runeWidth(r)
			sx := cell - c.hscroll
			if sx+w > 0 && sx < width {
				st := base
				if c.isSelected(ln, rn) {
					st = sel
				}
				if sx >= 0 {
					screen.SetContent(x+sx, y+row, r, nil, st)
				}
			}
			cell += w
			rn++
			if sx >= width {
				break
			}
		}
		// Paint the trailing area when the newline after this line is selected, so
		// a multi-line selection reads as a continuous band.
		if c.selSpansNewline(ln) {
			start := max(cell-c.hscroll, 0)
			for sx := start; sx < width; sx++ {
				screen.SetContent(x+sx, y+row, ' ', nil, sel)
			}
		}
	}
}

var _ interface {
	Write([]byte) (int, error)
} = (*LogConsole)(nil)
