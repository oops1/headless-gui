// Package dock — DockPanel: контейнер с докингом по сторонам (WPF DockPanel).
//
// Дочерние виджеты прикрепляются к сторонам через attached property DockPanel.Dock:
//   - Top: элемент занимает всю ширину сверху
//   - Bottom: элемент занимает всю ширину снизу
//   - Left: элемент занимает всю высоту слева
//   - Right: элемент занимает всю высоту справа
//   - Последний элемент (или без Dock) заполняет оставшееся пространство
//
// WPF-совместимый layout: элементы обрабатываются в порядке добавления,
// каждый "откусывает" место от оставшейся области.
package dock

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// DockPanel — контейнер с layout по WPF DockPanel.
type DockPanel struct {
	base.Base

	Background color.RGBA
	UseAlpha   bool
	// BackgroundRole — откуда фон берётся при смене темы (bgrole.go). По
	// умолчанию BackgroundCustom: свой цвет тема не трогает.
	BackgroundRole base.BackgroundRole

	// LastChildFill — растягивать ли последнего ребёнка на остаток места
	// (WPF DockPanel.LastChildFill, по умолчанию true).
	//
	// Выключается там, где ВСЕ дети докованы: строка с полем поиска слева и
	// счётчиком с кнопками справа. С включённым заполнением последний
	// ребёнок забирает весь остаток, и его собственный Dock не действует —
	// приходилось дописывать в конец пустой Border только затем, чтобы
	// растяжение досталось ему.
	//
	// Поле, а не «нулевое значение = true»: у NewDockPanel он ставится в
	// true, а собранный литералом DockPanel{} вёл бы себя иначе. Оба пути
	// живут в чужом коде, и разное поведение у них — источник вопросов, а не
	// экономия.
	LastChildFill bool

	// fillChild/fillOrig — кто сейчас растянут как LastChildFill и его
	// bounds до растяжения. При добавлении следующего ребёнка исходный
	// размер восстанавливается — иначе бывший «последний» резервировал
	// при доке всю панель (зазор после SetBounds-до-AddChild).
	fillChild base.Widget
	fillOrig  image.Rectangle

	// auto — размер, который панель сама выставила ребёнку по содержимому на
	// прошлой раскладке: X — ширина у Left/Right, Y — высота у Top/Bottom.
	// Нынешний размер совпадает с ним — значит, извне его не задавали, и на
	// раскладке он перемеряется: подпись могла смениться (GG-74). Раньше
	// ребёнок мерился по содержимому один раз, пока размер был нулевым.
	auto map[base.Widget]image.Point
}

// NewDockPanel создаёт пустой DockPanel с прозрачным фоном.
func NewDockPanel() *DockPanel {
	return &DockPanel{
		UseAlpha:      true,
		LastChildFill: true,
	}
}

// SetBounds устанавливает bounds и пересчитывает layout.
func (dp *DockPanel) SetBounds(r image.Rectangle) {
	dp.Base.SetBounds(r)
	dp.layout()
}

// AddChild добавляет дочерний виджет и пересчитывает layout.
func (dp *DockPanel) AddChild(w base.Widget) {
	dp.Base.AddChild(w)
	dp.layout()
}

// Relayout пересчитывает раскладку панели.
//
// Нужен, когда содержимое ребёнка сменилось само по себе — например, кодом
// поменяли подпись кнопки: у виджета нет ссылки на родителя, и панель об этом
// не узнаёт. Ребёнок без явного размера перемеряется по новому содержимому.
// Дерево из разметки при смене языка переразмечается само.
func (dp *DockPanel) Relayout() {
	dp.layout()
	dp.Invalidate()
}

// rememberAuto запоминает размер, выставленный ребёнку по содержимому.
func (dp *DockPanel) rememberAuto(child base.Widget, p image.Point) {
	if dp.auto == nil {
		dp.auto = make(map[base.Widget]image.Point)
	}
	dp.auto[child] = p
}

// layout расставляет детей по DockPanel.Dock.
// Последний ребёнок заполняет оставшееся пространство (WPF LastChildFill=true).
func (dp *DockPanel) layout() {
	b := dp.Base.Bounds()
	if b.Empty() {
		return
	}

	// Оставшаяся область (уменьшается по мере размещения детей).
	remaining := b

	// Записи прошлой раскладки: остаются только дети, которых мерили сейчас,
	// — убранные из панели не держатся.
	prevAuto := dp.auto
	dp.auto = nil

	children := dp.Base.Children()
	for i, child := range children {
		isLast := dp.LastChildFill && i == len(children)-1

		// Получаем Dock через интерфейс
		dock := base.DockLeft // default (WPF standard: DockPanel.Dock default = Left)
		type dockGetter interface {
			GetDock() base.DockSide
		}
		if dg, ok := child.(dockGetter); ok {
			dock = dg.GetDock()
		}

		// Margin — внешний отступ ребёнка.
		var m base.Margin
		type marginGetter interface {
			GetMargin() base.Margin
		}
		if mg, ok := child.(marginGetter); ok {
			m = mg.GetMargin()
		}

		// Последний элемент заполняет оставшееся пространство
		if isLast {
			if dp.fillChild != child {
				dp.fillChild = child
				dp.fillOrig = child.Bounds()
			}
			// Растянутый ребёнок не мерится, но запись сохраняет: перестанет
			// быть последним — вернётся к размеру по содержимому, а не к
			// застывшему.
			if p, ok := prevAuto[child]; ok {
				dp.rememberAuto(child, p)
			}
			r := image.Rect(
				remaining.Min.X+m.Left, remaining.Min.Y+m.Top,
				remaining.Max.X-m.Right, remaining.Max.Y-m.Bottom,
			)
			aligned := base.ApplyAlignmentRect(child, r)
			child.SetBounds(aligned)
			break
		}

		cb := child.Bounds()
		if dp.fillChild == child {
			// Ребёнок был растянут как Fill, но перестал быть последним —
			// докуем его по исходному размеру.
			cb = dp.fillOrig
			dp.fillChild = nil
		}
		switch dock {
		case base.DockTop:
			h := cb.Dy()
			if h <= 0 || prevAuto[child].Y == h {
				h = base.DesiredHeight(child)
				dp.rememberAuto(child, image.Pt(0, h))
			}
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Min.Y+m.Top,
				remaining.Max.X-m.Right, remaining.Min.Y+m.Top+h,
			))
			remaining.Min.Y += h + m.Top + m.Bottom

		case base.DockBottom:
			h := cb.Dy()
			if h <= 0 || prevAuto[child].Y == h {
				h = base.DesiredHeight(child)
				dp.rememberAuto(child, image.Pt(0, h))
			}
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Max.Y-m.Bottom-h,
				remaining.Max.X-m.Right, remaining.Max.Y-m.Bottom,
			))
			remaining.Max.Y -= h + m.Top + m.Bottom

		case base.DockLeft:
			w := cb.Dx()
			if w <= 0 || prevAuto[child].X == w {
				w = base.DesiredWidth(child)
				dp.rememberAuto(child, image.Pt(w, 0))
			}
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Min.Y+m.Top,
				remaining.Min.X+m.Left+w, remaining.Max.Y-m.Bottom,
			))
			remaining.Min.X += w + m.Left + m.Right

		case base.DockRight:
			w := cb.Dx()
			if w <= 0 || prevAuto[child].X == w {
				w = base.DesiredWidth(child)
				dp.rememberAuto(child, image.Pt(w, 0))
			}
			child.SetBounds(image.Rect(
				remaining.Max.X-m.Right-w, remaining.Min.Y+m.Top,
				remaining.Max.X-m.Right, remaining.Max.Y-m.Bottom,
			))
			remaining.Max.X -= w + m.Left + m.Right

		default:
			// Fill — заполняет оставшееся
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Min.Y+m.Top,
				remaining.Max.X-m.Right, remaining.Max.Y-m.Bottom,
			))
		}
	}
}

// Draw рисует фон и дочерние виджеты.
func (dp *DockPanel) Draw(ctx base.DrawContext) {
	b := dp.Base.Bounds()
	if b.Empty() {
		return
	}
	if dp.Background.A > 0 {
		if dp.UseAlpha && dp.Background.A < 255 {
			if ac, ok := ctx.(base.DrawContextAlpha); ok {
				ac.FillRectAlpha(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), dp.Background)
			}
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), dp.Background)
		}
	}
	dp.DrawChildren(ctx)
}

// ApplyTheme обновляет фон DockPanel, если панели назначена роль фона.
// Без роли фон не трогается (см. bgrole.go).
func (dp *DockPanel) ApplyTheme(t *base.Theme) {
	if c, ok := base.ThemeBackground(dp.BackgroundRole, t); ok {
		dp.Background = c
		dp.UseAlpha = c.A < 255
	}
}

// BackgroundColor — фон панели докинга (BackgroundColorer).
func (dp *DockPanel) BackgroundColor() (color.RGBA, bool) { return dp.Background, dp.Background.A > 0 }

// SetBackgroundRole задаёт, откуда панель берёт фон, и сразу применяет цвет
// из текущей темы. В разметке — Background="{Theme WindowBG}".
func (dp *DockPanel) SetBackgroundRole(r base.BackgroundRole) {
	dp.BackgroundRole = r
	if c, ok := base.CurrentThemeBackground(r); ok {
		dp.Background = c
		dp.UseAlpha = c.A < 255
	}
	dp.Invalidate()
}

// LaysOutChildren — SetBounds панели сам раскладывает детей (ChildLayouter).
func (dp *DockPanel) LaysOutChildren() bool { return true }

// DescribeAccess — семантика панели для доступности (AccessDescriber).
func (dp *DockPanel) DescribeAccess(info *base.AccessInfo) { info.Role = base.RolePanel }

var (
	_ base.BackgroundColorer = (*DockPanel)(nil)
	_ base.ChildLayouter     = (*DockPanel)(nil)
	_ base.AccessDescriber   = (*DockPanel)(nil)
)
