// Package widget — Expander: раскрывающаяся панель (аналог WPF Expander).
//
// Заголовок с треугольником-индикатором; по клику разворачивает/сворачивает
// содержимое. В свёрнутом состоянии содержимое не рисуется и не участвует в
// hit-тесте.
package widget

import (
	"image"
	"image/color"
)

// Expander — раскрывающаяся панель.
type Expander struct {
	Base
	Header       string
	IsExpanded   bool
	HeaderHeight int
	HeaderBG     color.RGBA
	BorderColor  color.RGBA
	TextColor    color.RGBA

	// OnExpandedChanged вызывается при смене состояния.
	OnExpandedChanged func(expanded bool)
}

// NewExpander создаёт Expander с заголовком (свёрнут по умолчанию).
func NewExpander(header string) *Expander {
	return &Expander{
		Header:       header,
		HeaderHeight: 30,
		HeaderBG:     win10.TabBG,
		BorderColor:  win10.Border,
		TextColor:    win10.TitleText,
	}
}

func (e *Expander) headerH() int {
	if e.HeaderHeight > 0 {
		return e.HeaderHeight
	}
	return 30
}

// ContentBounds — область содержимого (пустая, когда свёрнут).
func (e *Expander) ContentBounds() image.Rectangle {
	if !e.IsExpanded {
		return image.Rectangle{}
	}
	b := e.bounds
	return image.Rect(b.Min.X+1, b.Min.Y+e.headerH(), b.Max.X-1, b.Max.Y-1)
}

// Children возвращает содержимое только в развёрнутом состоянии
// (чтобы движок не доставлял события скрытому контенту).
func (e *Expander) Children() []Widget {
	if e.IsExpanded {
		return e.children
	}
	return nil
}

// SetBounds задаёт bounds и размещает содержимое.
func (e *Expander) SetBounds(r image.Rectangle) {
	e.Base.SetBounds(r)
	cb := e.ContentBounds()
	if cb.Empty() {
		return
	}
	for _, c := range e.children {
		c.SetBounds(cb)
	}
}

// SetExpanded разворачивает/сворачивает панель.
func (e *Expander) SetExpanded(v bool) {
	if e.IsExpanded == v {
		return
	}
	e.IsExpanded = v
	e.SetBounds(e.bounds)
	if e.OnExpandedChanged != nil {
		e.OnExpandedChanged(v)
	}
}

// Draw рисует заголовок (с треугольником) и, если развёрнут, содержимое.
func (e *Expander) Draw(ctx DrawContext) {
	b := e.bounds
	if b.Empty() {
		return
	}
	hh := e.headerH()
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), hh, e.HeaderBG)
	arrow := "▶" // ▶
	if e.IsExpanded {
		arrow = "▼" // ▼
	}
	ctx.DrawText(arrow, b.Min.X+8, b.Min.Y+(hh-13)/2, e.TextColor)
	ctx.DrawText(e.Header, b.Min.X+28, b.Min.Y+(hh-13)/2, e.TextColor)

	if e.IsExpanded {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), e.BorderColor)
		// Содержимое отсекается по внутренней области, чтобы не выходить за рамку.
		save := ctx.Clip()
		ctx.SetClip(e.ContentBounds().Intersect(save))
		e.drawChildren(ctx)
		ctx.SetClip(save)
	} else {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), hh, e.BorderColor)
	}
}

// OnMouseButton переключает состояние по клику на заголовок.
func (e *Expander) OnMouseButton(ev MouseEvent) bool {
	if ev.Button != MouseLeft || !ev.Pressed {
		return false
	}
	b := e.bounds
	hh := e.headerH()
	if ev.X >= b.Min.X && ev.X < b.Max.X && ev.Y >= b.Min.Y && ev.Y < b.Min.Y+hh {
		e.SetExpanded(!e.IsExpanded)
		return true
	}
	return false
}

// ApplyTheme обновляет цвета. Содержимое темизируется явно: в свёрнутом
// состоянии Children() возвращает nil и общий обход темы детей не видит.
func (e *Expander) ApplyTheme(t *Theme) {
	e.HeaderBG = t.TabBG
	e.BorderColor = t.Border
	e.TextColor = t.LabelText
	if !e.IsExpanded {
		for _, c := range e.children {
			ApplyThemeTree(c, t)
		}
	}
}
