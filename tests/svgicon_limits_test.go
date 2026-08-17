package tests

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/svg"
)

// SetSVG отвергает слишком глубокую разметку без паники.
func TestSVGIcon_SetSVGDepthLimit(t *testing.T) {
	const n = 100000
	var b strings.Builder
	b.WriteString(`<svg viewBox="0 0 24 24">`)
	b.WriteString(strings.Repeat("<g>", n))
	b.WriteString(`<rect width="4" height="4" fill="#fff"/>`)
	b.WriteString(strings.Repeat("</g>", n))
	b.WriteString(`</svg>`)

	ic := widget.NewSVGIcon()
	if err := ic.SetSVG([]byte(b.String())); err == nil {
		t.Fatal("глубокая иконка принята")
	}
	if ic.Document() != nil {
		t.Fatal("документ не должен быть загружен")
	}
}

// SetSVG отвергает данные больше лимита.
func TestSVGIcon_SetSVGSizeLimit(t *testing.T) {
	data := make([]byte, svg.MaxFileBytes+1)
	for i := range data {
		data[i] = ' '
	}
	copy(data, []byte(`<svg viewBox="0 0 24 24"></svg>`))

	ic := widget.NewSVGIcon()
	if err := ic.SetSVG(data); err == nil {
		t.Fatal("иконка больше лимита принята")
	}
}
