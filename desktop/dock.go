package desktop

import (
	"image"
	"math"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Док — презентер области приложений для темы macOS.
//
// Отличие от полосы кнопок не в палитре: значки крупные и квадратные,
// стоят по центру, подписей нет, а тот, что под курсором, увеличивается и
// раздвигает соседей. Ни один набор цветов такого не даёт, поэтому тема
// приносит эту отрисовку с собой, а компонент остаётся прежним — его
// поведение (активация окна, сворачивание, подписка на список) от вида не
// зависит и проверяется одними и теми же тестами для обеих тем.

// Ключи токенов дока.
const (
	// KeyDockIcon — сторона значка в покое.
	KeyDockIcon theme.Key = "dock.icon"
	// KeyDockMagnify — во сколько раз увеличивается значок под курсором.
	KeyDockMagnify theme.Key = "dock.magnify"
	// KeyDockGap — зазор между значками.
	KeyDockGap theme.Key = "taskbutton.gap"

	// ComponentDock — имя компонента для стилей темы: у дока своя ячейка и
	// свой указатель активного приложения.
	ComponentDock = "dock"
)

// DockPresenter рисует область приложений доком.
type DockPresenter struct{}

// Регистрация под именем, которым его называют профили (Presenters["runningapps"]).
func init() { RegisterPresenter("dock", DockPresenter{}) }

// magnifySpread — на сколько соседей слева и справа расходится увеличение.
// Один сосед: под курсором значок крупный, ближайшие — чуть крупнее, дальше
// обычные. Так дока «дышит», не превращаясь в волну во всю ширину.
const magnifySpread = 1

// Measure возвращает ширину дока: значки в покое плюс запас на увеличение
// одного из них, чтобы при наведении соседи не выпихивались за край.
func (DockPresenter) Measure(c Component, avail image.Point) image.Point {
	n := len(c.Cells())
	if n == 0 {
		return image.Point{}
	}
	icon, mag, gap := dockMetrics(c.Theme())
	w := n*icon + gap*(n-1)
	// Запас: увеличенный значок шире обычного на (mag-1) своей стороны.
	w += int(float64(icon) * (mag - 1))
	if w > avail.X {
		w = avail.X
	}
	return image.Pt(w, avail.Y)
}

// Layout доку не нужен: ячейки считаются при отрисовке — их размер зависит
// от того, где сейчас курсор, и запоминать эту раскладку незачем.
func (DockPresenter) Layout(Component, image.Rectangle) {}

// Draw рисует док: центрированный ряд значков с увеличением под курсором.
func (p DockPresenter) Draw(ctx widget.DrawContext, c Component) {
	b := c.Bounds()
	cells := c.Cells()
	if b.Empty() || len(cells) == 0 {
		return
	}
	tm := c.Theme()
	icon, mag, gap := dockMetrics(tm)
	hover := c.HoverIndex()

	// Размер каждого значка: под курсором — полный, у соседей — промежуточный.
	sizes := make([]int, len(cells))
	total := 0
	for i := range cells {
		sizes[i] = dockIconSize(icon, mag, i, hover)
		total += sizes[i]
	}
	total += gap * (len(cells) - 1)

	// Ряд стоит по центру области — это и есть примета дока.
	x := b.Min.X + (b.Dx()-total)/2
	if x < b.Min.X {
		x = b.Min.X
	}

	for i, cell := range cells {
		size := sizes[i]
		// Значки выровнены по нижнему краю: увеличенный растёт вверх, как в
		// настоящем доке, а не раздвигает ряд по вертикали.
		cellRect := image.Rect(x, b.Max.Y-size, x+size, b.Max.Y)
		st := StateOf(i == hover, false, cell.Active, cell.Muted, false)
		style := styleOf(tm, ComponentDock, "", st)

		PaintStyle(ctx, cellRect, style)
		if cell.Icon != nil {
			ctx.DrawImageScaled(cell.Icon, cellRect.Min.X, cellRect.Min.Y, size, size)
		} else {
			// Значка нет — рисуем плитку цветом текста, чтобы приложение всё
			// же было видно и по нему можно было попасть.
			drawDockTile(ctx, cellRect, style)
		}

		// Точка под значком у запущенного приложения — так док отличает
		// открытое от закреплённого.
		if cell.Active || !cell.Muted {
			drawRunningDot(ctx, cellRect, style, cell.Active)
		}
		x += size + gap
	}
}

// dockIconSize возвращает размер значка i при курсоре над hover.
func dockIconSize(icon int, mag float64, i, hover int) int {
	if hover < 0 || mag <= 1 {
		return icon
	}
	d := i - hover
	if d < 0 {
		d = -d
	}
	if d > magnifySpread {
		return icon
	}
	// Ближе к курсору — крупнее: косинусный спад даёт плавную «волну»
	// вместо ступеньки.
	k := 1 + (mag-1)*math.Cos(float64(d)/float64(magnifySpread+1)*math.Pi/2)
	if d == 0 {
		k = mag
	}
	return int(float64(icon) * k)
}

// dockMetrics читает размеры дока из темы.
func dockMetrics(tm *theme.Manager) (icon int, mag float64, gap int) {
	if tm == nil {
		return 0, 1, 0
	}
	icon = int(tm.GetMetric(KeyDockIcon))
	mag = tm.GetMetric(KeyDockMagnify)
	if mag < 1 {
		mag = 1
	}
	gap = int(tm.GetMetric(KeyDockGap))
	return icon, mag, gap
}

// styleOf — стиль компонента из темы (пустой, если темы нет).
func styleOf(tm *theme.Manager, comp, part string, st theme.State) *theme.Style {
	if tm == nil {
		return &theme.Style{}
	}
	return tm.GetStyle(comp, part, st)
}

// drawDockTile рисует плитку вместо отсутствующего значка.
func drawDockTile(ctx widget.DrawContext, r image.Rectangle, s *theme.Style) {
	if s.Text.A == 0 {
		return
	}
	inset := r.Dy() / 6
	tile := r.Inset(inset)
	if tile.Empty() {
		return
	}
	corner := int(s.Corner)
	if corner > 0 {
		ctx.FillRoundRect(tile.Min.X, tile.Min.Y, tile.Dx(), tile.Dy(), corner, s.Text)
		return
	}
	ctx.FillRect(tile.Min.X, tile.Min.Y, tile.Dx(), tile.Dy(), s.Text)
}

// drawRunningDot рисует точку запущенного приложения под значком.
func drawRunningDot(ctx widget.DrawContext, r image.Rectangle, s *theme.Style, active bool) {
	col := s.Text
	if active && s.Border.A > 0 {
		col = s.Border
	}
	if col.A == 0 {
		return
	}
	d := r.Dy() / 12
	if d < 2 {
		d = 2
	}
	cx := r.Min.X + r.Dx()/2 - d/2
	y := r.Max.Y - d
	ctx.FillRoundRect(cx, y, d, d, d/2, col)
}
