// runningapps_vertical.go — полоса кнопок окон в столбце боковой панели.
//
// В ряду кнопки идут слева направо и сжимаются до значков. В столбце они идут
// сверху вниз, каждая — квадрат со значком (подписей в столбце нет: им не
// хватило бы ширины панели), а не влезающие сжимаются по высоте. Устроено так
// же, как у ApplicationArea (applicationarea_vertical.go); без этого столбец
// получал ряд кнопок, втиснутый в квадрат со стороной в толщину панели.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
)

var (
	_ VerticalItem = (*RunningApplications)(nil)
	_ EdgeAware    = (*RunningApplications)(nil)
)

// SetVertical реализует VerticalItem.
func (r *RunningApplications) SetVertical(v bool) {
	if r.vertical == v {
		return
	}
	r.vertical = v
	r.layout()
	r.Invalidate()
}

// SetBarEdge реализует EdgeAware: метка открытого окна встаёт на сторону
// кнопки, обращённую к краю экрана.
func (r *RunningApplications) SetBarEdge(e Edge) {
	if r.edge == e {
		return
	}
	r.edge = e
	r.Invalidate()
}

// markEdge — с какой стороны кнопки рисуется метка открытого окна.
func (r *RunningApplications) markEdge() Edge { return markEdgeOf(r.vertical, r.edge) }

// verticalSide — сторона кнопки в столбце: значок с отступами стиля кнопки, а
// если тема размера значка не задала — вся толщина панели.
func (r *RunningApplications) verticalSide(thickness int) int {
	icon := int(r.metric(KeyTaskButtonIconSize))
	if icon <= 0 {
		return thickness
	}
	side := icon + 2*int(r.style(theme.StateNormal).PadX)
	if thickness > 0 && side > thickness {
		side = thickness
	}
	return side
}

// preferredVertical — желаемый размер столбца кнопок.
func (r *RunningApplications) preferredVertical(n int, avail image.Point) image.Point {
	side := r.verticalSide(avail.X)
	gap := int(r.metric(KeyTaskButtonGap))
	h := n*side + gap*(n-1)
	if avail.Y > 0 && h > avail.Y {
		h = avail.Y
	}
	return image.Pt(side, h)
}

// layoutVertical раскладывает кнопки столбцом; не влезающие по высоте
// сжимаются поровну. Зовётся под r.mu.
func (r *RunningApplications) layoutVertical(b image.Rectangle, windows []WindowInfo) {
	n := len(windows)
	gap := int(r.metric(KeyTaskButtonGap))
	per := r.verticalSide(b.Dx())
	if n*per+gap*(n-1) > b.Dy() {
		per = (b.Dy() - gap*(n-1)) / n
	}
	if per < 1 {
		return
	}
	y := b.Min.Y
	for i := 0; i < n; i++ {
		rect := image.Rect(b.Min.X, y, b.Max.X, y+per).Intersect(b)
		if rect.Empty() {
			break
		}
		r.btns = append(r.btns, winButton{info: windows[i], rect: rect})
		y += per + gap
	}
}
