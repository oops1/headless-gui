// applicationarea_vertical.go — область приложений в столбце боковой панели.
//
// В ряду ячейки идут слева направо и сжимаются до значков. В столбце они идут
// сверху вниз, каждая — квадрат со значком (подписей в столбце нет: им не
// хватило бы ширины панели), а не влезающие сжимаются по высоте.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
)

var (
	_ VerticalItem = (*ApplicationArea)(nil)
	_ EdgeAware    = (*ApplicationArea)(nil)
)

// SetBarEdge реализует EdgeAware: метка открытого окна встаёт на сторону
// ячейки, обращённую к краю экрана.
func (a *ApplicationArea) SetBarEdge(e Edge) {
	if a.edge == e {
		return
	}
	a.edge = e
	a.Invalidate()
}

// SetVertical реализует VerticalItem.
func (a *ApplicationArea) SetVertical(v bool) {
	if a.vertical == v {
		return
	}
	a.vertical = v
	a.layout()
	a.Invalidate()
}

// verticalSide — сторона ячейки в столбце: значок с отступами стиля кнопки,
// а если тема размера значка не задала — вся толщина панели.
func (a *ApplicationArea) verticalSide(thickness int) int {
	icon := int(a.metric(KeyTaskButtonIconSize))
	if icon <= 0 {
		return thickness
	}
	st := styleOf(a.tm, ComponentTaskButton, "", theme.StateNormal)
	side := icon + 2*int(st.PadX)
	if thickness > 0 && side > thickness {
		side = thickness
	}
	return side
}

// preferredVertical — желаемый размер столбца ячеек.
func (a *ApplicationArea) preferredVertical(n int, avail image.Point) image.Point {
	side := a.verticalSide(avail.X)
	gap := int(a.metric(KeyTaskButtonGap))
	h := n*side + gap*(n-1)
	if avail.Y > 0 && h > avail.Y {
		h = avail.Y
	}
	return image.Pt(side, h)
}

// layoutVertical раскладывает ячейки столбцом; не влезающие по высоте
// сжимаются поровну.
func (a *ApplicationArea) layoutVertical(b image.Rectangle, n int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	gap := int(a.metric(KeyTaskButtonGap))
	per := a.verticalSide(b.Dx())
	if n*per+gap*(n-1) > b.Dy() {
		per = (b.Dy() - gap*(n-1)) / n
	}
	if per < 1 {
		return
	}
	y := b.Min.Y
	for i := 0; i < n; i++ {
		r := image.Rect(b.Min.X, y, b.Max.X, y+per).Intersect(b)
		if r.Empty() {
			break
		}
		a.rects = append(a.rects, r)
		y += per + gap
	}
}
