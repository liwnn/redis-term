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

	scroll int // index of the first visible line (vertical scroll offset)

	onCmdLineEnter func(string)
	title          string
	address        string
	index          int
}

func NewCmdConsole(title string) *CmdConsole {
	c := &CmdConsole{
		title: title,
		Box:   tview.NewBox(),
	}
	c.Box.SetBorder(true)
	c.Box.SetBackgroundColor(ThemePanelBG)
	c.histPos = 0
	return c
}

func (c *CmdConsole) Title() string { return c.title }

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
		case tcell.KeyCtrlU:
			c.input = c.input[:0]
			c.cursor = 0
		}
	})
}

// MouseHandler focuses the console on a left click so the user can start typing,
// matching the old InputField's click-to-focus behavior.
func (c *CmdConsole) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return c.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if !c.InRect(event.Position()) {
			return false, nil
		}
		if action == tview.MouseLeftClick {
			setFocus(c)
			return true, nil
		}
		return false, nil
	})
}

func (c *CmdConsole) scrollToEnd() {
	// total visual lines = committed lines (last open fragment shares the prompt
	// line) — clamped in Draw, here we just force "follow tail".
	c.scroll = 1 << 30
}

func (c *CmdConsole) Draw(screen tcell.Screen) {
	c.Box.DrawForSubclass(screen, c)
	x, y, width, height := c.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	// Build the full set of visual lines: committed scrollback, with the live
	// prompt+input appended to the final (open) line.
	lines := make([]string, len(c.lines))
	copy(lines, c.lines)
	if len(lines) == 0 {
		lines = append(lines, "")
	}
	promptLine := fmt.Sprintf("%v%v%v%v", PromptColor, c.prompt(), InputColor, tview.Escape(string(c.input)))
	lines[len(lines)-1] += promptLine

	// Vertical scroll: keep the last `height` lines visible (follow tail).
	first := 0
	if len(lines) > height {
		first = len(lines) - height
	}
	if c.scroll < first {
		// (manual scroll-up not wired yet; always follow tail)
	}
	c.scroll = first

	for i := first; i < len(lines); i++ {
		ty := y + (i - first)
		tview.Print(screen, lines[i], x, ty, width, tview.AlignLeft, tcell.ColorWhite)
	}

	// Place the hardware cursor right after the prompt + input[:cursor].
	if c.HasFocus() {
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

// promptLineUpTo returns the prompt plus the input runes before the caret, used
// only to measure the caret column.
func promptLineUpTo(prompt string, input []rune, cursor int) string {
	if cursor > len(input) {
		cursor = len(input)
	}
	return prompt + string(input[:cursor])
}
