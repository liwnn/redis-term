package view

import (
	"strings"

	"github.com/liwnn/redisterm/tlog"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Setting struct {
	Name string
	Host string
	Port string
	Auth string
}

type ConnSetting struct {
	tview.Primitive
	form   *tview.Form
	ok     func(Setting)
	cancel func()
}

func NewConnSetting() *ConnSetting {
	p := &ConnSetting{}
	p.init()
	return p
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
	form := tview.NewForm().
		AddInputField("Name:", "", 20, nil, nil).
		AddInputField("Address:", "", 20, nil, nil).
		AddPasswordField("Auth:", "", 20, '*', nil).
		AddButton("  OK  ", func() {
			if s.ok != nil {
				name := s.form.GetFormItem(0).(*tview.InputField).GetText()
				address := s.form.GetFormItem(1).(*tview.InputField).GetText()
				auth := s.form.GetFormItem(2).(*tview.InputField).GetText()
				t := strings.Split(address, ":")
				if len(t) != 2 {
					return
				}
				s.ok(Setting{
					Name: name,
					Host: t[0],
					Port: t[1],
					Auth: auth,
				})
			}
		}).
		AddButton("Cancel", func() {
			if s.cancel != nil {
				s.cancel()
			}
		})
	form.SetButtonsAlign(tview.AlignCenter)
	form.SetFieldTextColor(tcell.ColorBlack)
	form.SetFieldBackgroundColor(tcell.ColorGray)
	form.SetBorder(true).SetTitle("Connection setting")
	p := Center(36, 15, form)
	p.SetMouseCapture(s.onMousecapture)
	s.Primitive = p
	s.form = form
}

func (s *ConnSetting) Init(c Setting) {
	s.form.GetFormItem(0).(*tview.InputField).SetText(c.Name)
	s.form.GetFormItem(1).(*tview.InputField).SetText(c.Host + ":" + c.Port)
	tlog.Log("pass %v", c.Auth)
	s.form.GetFormItem(2).(*tview.InputField).SetText(c.Auth)
}

func (s *ConnSetting) onMousecapture(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
	return action, event
}

func (s *ConnSetting) SetCancelHandler(f func()) {
	s.cancel = f
	s.form.SetCancelFunc(f)
}
func (s *ConnSetting) SetOKHandler(f func(Setting)) {
	s.ok = f
}
