// Package widget — GroupBox: рамка с заголовком (аналог WPF GroupBox).
package widget

import (
	"image"
	"image/color"
)

// GroupBox — контейнер с рамкой и текстовым заголовком сверху.
type GroupBox struct {
	Base
	Header       string
	Background   color.RGBA
	BorderColor  color.RGBA
	HeaderColor  color.RGBA
	HeaderHeight int
}

// NewGroupBox создаёт GroupBox с заголовком.
func NewGroupBox(header string) *GroupBox {
	return &GroupBox{
		Header:       header,
		BorderColor:  win10.Border,
		HeaderColor:  win10.TitleText,
		HeaderHeight: 22,
	}
}

func (g *GroupBox) headerH() int {
	if g.HeaderHeight > 0 {
		return g.HeaderHeight
	}
	return 22
}

// ContentBounds возвращает область содержимого (внутри рамки, под заголовком).
func (g *GroupBox) ContentBounds() image.Rectangle {
	b := g.bounds
	return image.Rect(b.Min.X+2, b.Min.Y+g.headerH()+1, b.Max.X-2, b.Max.Y-2)
}

// SetBounds задаёт bounds и размещает содержимое в ContentBounds.
func (g *GroupBox) SetBounds(r image.Rectangle) {
	g.Base.SetBounds(r)
	cb := g.ContentBounds()
	for _, c := range g.children {
		c.SetBounds(cb)
	}
}

// Draw рисует заголовок, рамку и содержимое.
func (g *GroupBox) Draw(ctx DrawContext) {
	b := g.bounds
	if b.Empty() {
		return
	}
	if g.Background.A > 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), g.Background)
	}
	hh := g.headerH()
	ctx.DrawText(g.Header, b.Min.X+6, b.Min.Y+(hh-13)/2, g.HeaderColor)
	ctx.DrawBorder(b.Min.X, b.Min.Y+hh, b.Dx(), b.Dy()-hh, g.BorderColor)
	g.drawChildren(ctx)
}

// ApplyTheme обновляет цвета.
func (g *GroupBox) ApplyTheme(t *Theme) {
	g.BorderColor = t.Border
	g.HeaderColor = t.TitleText
}
