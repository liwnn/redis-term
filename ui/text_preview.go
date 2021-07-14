package ui

import "github.com/rivo/tview"

type TextPreview struct {
	*tview.TextView
}

func NewTextPreview() *TextPreview {
	p := &TextPreview{}
	p.init()
	return p
}

func (p *TextPreview) init() {
	previewText := tview.NewTextView()
	previewText.
		SetDynamicColors(true).
		SetRegions(true)

	p.TextView = previewText
}

func (p *TextPreview) SetText(text string) {
	if len(text) > 4096 {
		text = text[:4096] + "..."
	}
	p.TextView.SetText(text)
}
