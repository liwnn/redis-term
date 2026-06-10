package view

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Setting struct {
	Name    string
	Kind    string // "redis" (default), "mongo", or "mysql"
	Host    string
	Port    string
	User    string // mysql / mongo username
	Auth    string
	URI     string // mongo connection string (URL mode)
	DB      string // mongo default database (form mode)
	URLMode bool   // mongo: true = URL string, false = host/port form
}

// connKinds lists supported backends in dropdown order.
var connKinds = []string{"redis", "mongo", "mysql"}

// mongoModes lists the mongo connection-entry modes in dropdown order.
var mongoModes = []string{"form", "url"}

// padOpts returns display copies of dropdown options padded with left/right
// whitespace so the selected text isn't flush against the highlight edges.
// Selection logic must use the option index, not these padded strings.
func padOpts(items []string) []string {
	out := make([]string, len(items))
	for i, v := range items {
		out[i] = " " + v + " "
	}
	return out
}

type ConnSetting struct {
	tview.Primitive
	form   *tview.Form
	status   *tview.TextView // inline result line under the form (Test feedback)
	ok       func(Setting, bool)
	cancel   func()
	test     func(Setting) error
	edit     bool
	cur      Setting // current field values, preserved across kind switches
	building bool    // true while build() rebuilds the form, to ignore dropdown callbacks
}

func NewConnSetting() *ConnSetting {
	p := &ConnSetting{}
	p.init()
	return p
}

// blockingPrimitive wraps a full-screen primitive and consumes any mouse event
// its child doesn't, making a centered dialog behave modally. Returning a nil
// event from a MouseCapture leaves consumed=false, so Pages still forwards the
// click to the page below; only reporting consumed=true here actually stops it.
type blockingPrimitive struct {
	tview.Primitive
}

func (b *blockingPrimitive) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	inner := b.Primitive.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		if inner != nil {
			if consumed, capture := inner(action, event, setFocus); consumed {
				return consumed, capture
			}
		}
		return true, nil // child didn't take it: swallow so it can't reach the page below
	}
}

// Center returns a new primitive which shows the provided primitive in its
// center, given the provided primitive's size.
func Center(width, height int, p tview.Primitive) *tview.Flex {
	flex := tview.NewFlex()
	flex.AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
	return flex
}

func (s *ConnSetting) init() {
	form := tview.NewForm()
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetFieldTextColor(ThemeQueryFG)
	form.SetFieldBackgroundColor(ThemeInputBG)
	form.SetLabelColor(ThemeQueryLabel)
	form.SetButtonBackgroundColor(ThemeBtnToolBG)
	form.SetButtonTextColor(ThemeBtnToolFG)
	form.SetButtonActivatedStyle(tcell.StyleDefault.Background(ThemeBtnToolHoverBG).Foreground(tcell.ColorWhite))
	form.SetBackgroundColor(ThemePanelBG)

	status := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter)
	status.SetBackgroundColor(ThemePanelBG)

	// form + a one-line status row beneath it, both inside one bordered box so the
	// Test result shows inside the dialog rather than below its border.
	body := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(status, 1, 0, false)
	body.SetBackgroundColor(ThemePanelBG)
	body.SetBorder(true).
		SetBorderColor(ThemeBorder).
		SetTitle(" Connection setting ").
		SetTitleColor(ThemeTitleFG)
	p := Center(64, 20, body)
	s.Primitive = &blockingPrimitive{Primitive: p} // make the dialog modal to mouse clicks
	s.form = form
	s.status = status
	s.build("redis")
}

// build lays out the form for a given backend kind. Redis keeps its original
// Address + Auth fields; mysql adds a Username; mongo swaps to a single URI.
// Switching the Kind dropdown captures the current values and rebuilds so each
// backend shows only the fields it needs.
func (s *ConnSetting) build(kind string) {
	s.building = true
	defer func() { s.building = false }()

	s.cur.Kind = kind
	s.setStatus("") // a kind/mode switch invalidates any prior Test result
	form := s.form
	form.Clear(true) // also clear buttons; they're rebuilt below so Delete tracks edit mode
	form.AddInputField("Name:", s.cur.Name, 44, nil, nil)

	kindIdx := indexOf(connKinds, kind)
	form.AddDropDown("Kind:", padOpts(connKinds), kindIdx, func(_ string, idx int) {
		if s.building || idx < 0 || connKinds[idx] == s.cur.Kind {
			return
		}
		s.capture()
		s.build(connKinds[idx])
	})

	switch kind {
	case "mongo":
		// mongo offers two entry modes: a host/port form or a single URL string.
		modeIdx := indexOf(mongoModes, s.mongoMode())
		form.AddDropDown("Mode:", padOpts(mongoModes), modeIdx, func(_ string, idx int) {
			urlMode := idx >= 0 && mongoModes[idx] == "url"
			if s.building || urlMode == s.cur.URLMode {
				return
			}
			s.capture()
			s.cur.URLMode = urlMode
			s.build(kind)
		})
		if s.cur.URLMode {
			form.AddInputField("URI:", s.cur.URI, 44, nil, nil)
		} else {
			form.AddInputField("Address:", s.address(), 44, nil, nil)
			form.AddInputField("Username:", s.cur.User, 44, nil, nil)
			form.AddPasswordField("Auth:", s.cur.Auth, 44, '*', nil)
			form.AddInputField("DB:", s.cur.DB, 44, nil, nil)
		}
	case "mysql":
		form.AddInputField("Address:", s.address(), 44, nil, nil)
		form.AddInputField("Username:", s.cur.User, 44, nil, nil)
		form.AddPasswordField("Auth:", s.cur.Auth, 44, '*', nil)
	default: // redis
		form.AddInputField("Address:", s.address(), 44, nil, nil)
		form.AddPasswordField("Auth:", s.cur.Auth, 44, '*', nil)
	}

	form.AddButton("  OK  ", s.OnOk).
		AddButton("Test", s.OnTest).
		AddButton("Cancel", s.OnCancel)
}

// address renders the host:port pair as a single Address field value.
func (s *ConnSetting) address() string {
	if s.cur.Host == "" && s.cur.Port == "" {
		return ""
	}
	return s.cur.Host + ":" + s.cur.Port
}

// defaultPort returns the conventional port for a backend, used when the user
// types a bare host into the Address field without ":port".
func defaultPort(kind string) string {
	switch kind {
	case "mongo":
		return "27017"
	case "mysql":
		return "3306"
	default: // redis
		return "6379"
	}
}

// mongoMode returns the current mongo entry mode as a dropdown option string.
func (s *ConnSetting) mongoMode() string {
	if s.cur.URLMode {
		return "url"
	}
	return "form"
}

func (s *ConnSetting) input(label string) *tview.InputField {
	item := s.form.GetFormItemByLabel(label)
	if item == nil {
		return nil
	}
	return item.(*tview.InputField)
}

// text returns the text of a field if present, else "".
func (s *ConnSetting) text(label string) string {
	if in := s.input(label); in != nil {
		return in.GetText()
	}
	return ""
}

// capture reads the currently-shown fields back into s.cur so values survive a
// rebuild when the kind changes.
func (s *ConnSetting) capture() {
	s.cur.Name = s.text("Name:")
	s.cur.User = s.text("Username:")
	s.cur.Auth = s.text("Auth:")
	s.cur.URI = s.text("URI:")
	s.cur.DB = s.text("DB:")
	if addr := s.text("Address:"); addr != "" {
		if h, p, ok := strings.Cut(addr, ":"); ok {
			s.cur.Host, s.cur.Port = h, p
		} else {
			// host without ":port": fall back to the backend's default port so the
			// connection still validates instead of silently failing to save.
			s.cur.Host, s.cur.Port = addr, defaultPort(s.cur.Kind)
		}
	}
}

func indexOf(items []string, v string) int {
	for i, it := range items {
		if it == v {
			return i
		}
	}
	return 0
}

// buildSetting captures the form fields and assembles a Setting, validating the
// required fields per backend. ok=false means a required field is missing.
func (s *ConnSetting) buildSetting() (Setting, bool) {
	s.capture()
	set := Setting{
		Name: s.cur.Name,
		Kind: s.cur.Kind,
		User: s.cur.User,
		Auth: s.cur.Auth,
	}
	switch s.cur.Kind {
	case "mongo":
		if s.cur.URLMode {
			if s.cur.URI == "" {
				return set, false
			}
			set.URLMode = true
			set.URI = s.cur.URI
		} else {
			if s.cur.Host == "" || s.cur.Port == "" {
				return set, false
			}
			set.Host, set.Port = s.cur.Host, s.cur.Port
			set.DB = s.cur.DB
		}
	default: // redis, mysql: host:port (+ user/auth)
		if s.cur.Host == "" || s.cur.Port == "" {
			return set, false
		}
		set.Host, set.Port = s.cur.Host, s.cur.Port
	}
	return set, true
}

func (s *ConnSetting) OnOk() {
	if s.ok != nil {
		set, ok := s.buildSetting()
		if !ok {
			return
		}
		s.ok(set, s.edit)
	}
	s.Clear()
}

// OnTest probes the connection with the current field values without saving it.
// The result is shown on the inline status line; the form stays open so the user
// can fix fields and retry.
func (s *ConnSetting) OnTest() {
	if s.test == nil {
		return
	}
	set, ok := s.buildSetting()
	if !ok {
		s.setStatus("[yellow]Fill in the required fields first")
		return
	}
	s.setStatus("[gray]Connecting...")
	if err := s.test(set); err != nil {
		s.setStatus(fmt.Sprintf("[red]Connect failed: %v", err))
	} else {
		s.setStatus("[green]Connect OK")
	}
}

// setStatus writes the inline result line (guards against nil for safety).
func (s *ConnSetting) setStatus(msg string) {
	if s.status != nil {
		s.status.SetText(msg)
	}
}

func (s *ConnSetting) OnCancel() {
	if s.cancel != nil {
		s.cancel()
	}
	s.Clear()
}

func (s *ConnSetting) Clear() {
	s.edit = false
}

func (s *ConnSetting) SetEdit(edit bool) {
	s.edit = edit
}

func (s *ConnSetting) Init(c Setting) {
	if c.Kind == "" {
		c.Kind = "redis"
	}
	// An existing mongo conn with a URI was saved in URL mode; otherwise form mode.
	if c.Kind == "mongo" {
		c.URLMode = c.URI != ""
	}
	s.cur = c
	s.build(c.Kind)
}

func (s *ConnSetting) SetCancelHandler(f func()) {
	s.cancel = f
	s.form.SetCancelFunc(f)
}

func (s *ConnSetting) SetOKHandler(f func(Setting, bool)) {
	s.ok = f
}

// SetTestHandler wires the Test button to a connectivity probe. The handler
// returns nil on success or the connection error; OnTest renders it inline.
func (s *ConnSetting) SetTestHandler(f func(Setting) error) {
	s.test = f
}

