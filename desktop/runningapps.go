package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentTaskButton — имя компонента для стилей темы кнопок окон.
const ComponentTaskButton = "taskbutton"

// Ключи токенов, которыми тема управляет кнопками запущенных окон.
const (
	// KeyTaskButtonWidth — желаемая («идеальная») ширина одной кнопки:
	// значок, подпись и отступы помещаются без сжатия.
	KeyTaskButtonWidth theme.Key = "taskbutton.width"
	// KeyTaskButtonMinWidth — минимальная ширина кнопки, при которой
	// подпись ещё показывается (может обрезаться клипом). Меньше этого
	// подпись прячется и остаётся один значок.
	KeyTaskButtonMinWidth theme.Key = "taskbutton.width.min"
	// KeyTaskButtonIconSize — сторона значка окна.
	KeyTaskButtonIconSize theme.Key = "taskbutton.icon.size"
	// KeyTaskButtonGap — зазор между соседними кнопками окон.
	KeyTaskButtonGap theme.Key = "taskbutton.gap"
	// KeyTaskButtonLabelGap — зазор между значком и заголовком окна.
	KeyTaskButtonLabelGap theme.Key = "taskbutton.label.gap"
)

// winButton — раскладка одной кнопки окна, посчитанная layout(). Хранит
// снимок WindowInfo на момент расчёта: между пересчётами (вызванными
// подпиской на WindowModel) кнопка рисуется и кликается по этому снимку.
type winButton struct {
	info      WindowInfo
	rect      image.Rectangle
	showLabel bool
}

// RunningApplications — область панели задач с кнопками запущенных окон.
//
// Деградация при нехватке места (см. Taskbar: середина панели сжимается,
// элемент обязан пережить это предсказуемо) — в три ступени, посчитанные в
// layout():
//
//  1. Всем кнопкам хватает идеальной ширины (KeyTaskButtonWidth) — рисуем
//     значок и подпись без сжатия.
//  2. Идеальной ширины не хватает — кнопки сжимаются пропорционально,
//     вплоть до KeyTaskButtonMinWidth, подпись остаётся (клипуется, если
//     не влезает).
//  3. Не хватает и минимальной ширины — подпись пропадает у всех кнопок,
//     остаются одни значки; кнопки продолжают сжиматься до ширины значка.
//  4. Не хватает места даже под значки в один ряд — лишние кнопки просто
//     не рисуются (в btns не попадают), а не вылезают за границы области
//     или не наезжают друг на друга. Прокрутка — следующий шаг, здесь её
//     нет: пользователю показывается «сколько влезло».
type RunningApplications struct {
	widget.Base

	tm *theme.Manager
	wm WindowModel

	btns     []winButton
	hoverIdx int // -1 — нет наведения
	armedIdx int // -1 — нет взведённой кнопки

	// unsubWM снимает подписку на WindowModel. Забытая отписка удерживает
	// компонент в списке наблюдателей модели — утечка, если область убрали
	// со сцены и не закрыли.
	unsubWM func()
}

// NewRunningApplications создаёт область запущенных приложений,
// оформляемую темами из tm, по списку окон wm.
func NewRunningApplications(tm *theme.Manager, wm WindowModel) *RunningApplications {
	r := &RunningApplications{
		tm:       tm,
		wm:       wm,
		hoverIdx: -1,
		armedIdx: -1,
	}
	if wm != nil {
		r.unsubWM = wm.Subscribe(func() {
			// Список окон реально изменился — индексы наведения/взвода
			// могли начать указывать не на то окно. Безопаснее сбросить их,
			// чем гадать, куда «переехало» окно под курсором.
			r.hoverIdx = -1
			r.armedIdx = -1
			r.layout()
			r.Invalidate()
		})
	}
	r.layout()
	return r
}

// Close снимает подписку на WindowModel.
func (r *RunningApplications) Close() {
	if r.unsubWM != nil {
		r.unsubWM()
		r.unsubWM = nil
	}
}

// SetBounds задаёт границы области и пересчитывает раскладку кнопок.
func (r *RunningApplications) SetBounds(rect image.Rectangle) {
	r.Base.SetBounds(rect)
	r.layout()
}

// PreferredSize возвращает желаемую ширину для текущего числа окон
// (идеальная ширина кнопки на каждое окно плюс зазоры), но не больше
// avail.X — панель вправе дать меньше, деградация в layout() это переживёт.
func (r *RunningApplications) PreferredSize(avail image.Point) image.Point {
	if r.wm == nil {
		return image.Point{}
	}
	// Презентер темы решает размер сам: доку нужна не сумма ширин кнопок, а
	// ряд значков с запасом на увеличение.
	if p := r.presenter(); p != nil {
		return p.Measure(r, avail)
	}
	n := len(r.wm.Windows())
	if n == 0 {
		return image.Point{}
	}
	ideal := int(r.metric(KeyTaskButtonWidth))
	gap := int(r.metric(KeyTaskButtonGap))
	want := n*ideal + gap*(n-1)
	if want < 0 {
		want = 0
	}
	if avail.X > 0 && want > avail.X {
		want = avail.X
	}
	return image.Pt(want, 0)
}

// layout пересчитывает btns под текущие Bounds() и текущий список окон.
//
// Ширина считается один раз для всех кнопок (тем самым — их поровну), а не
// подгоняется под содержимое каждой: одинаковая ширина кнопок — то, что
// пользователь видит на настоящей панели задач.
func (r *RunningApplications) layout() {
	b := r.Bounds()
	r.btns = r.btns[:0]
	if r.wm == nil || b.Empty() {
		return
	}
	windows := r.wm.Windows()
	n := len(windows)
	if n == 0 {
		return
	}

	gap := int(r.metric(KeyTaskButtonGap))
	ideal := int(r.metric(KeyTaskButtonWidth))
	min := int(r.metric(KeyTaskButtonMinWidth))
	iconOnly := r.iconOnlyWidth()

	avail := b.Dx()
	totalGap := gap * (n - 1)
	perButton := (avail - totalGap) / n

	bw := ideal
	showLabel := true
	count := n

	switch {
	case perButton >= ideal:
		bw, showLabel = ideal, true
	case perButton >= min:
		bw, showLabel = perButton, true
	case perButton >= iconOnly:
		bw, showLabel = perButton, false
	default:
		// Даже по одному значку в ряд не помещаются все окна — считаем,
		// сколько влезет при фиксированной ширине значка, и рисуем только
		// столько кнопок, остальные не попадают в btns вовсе.
		bw, showLabel = iconOnly, false
		if bw > 0 {
			count = (avail + gap) / (bw + gap)
		} else {
			count = 0
		}
		if count > n {
			count = n
		}
		if count < 0 {
			count = 0
		}
	}
	if bw < 1 {
		bw = 1
	}

	x := b.Min.X
	for i := 0; i < count; i++ {
		rect := image.Rect(x, b.Min.Y, x+bw, b.Max.Y).Intersect(b)
		if rect.Empty() {
			break
		}
		r.btns = append(r.btns, winButton{info: windows[i], rect: rect, showLabel: showLabel})
		x += bw + gap
	}
}

// iconOnlyWidth — минимальная ширина кнопки без подписи: значок и отступы
// стиля по обе стороны.
func (r *RunningApplications) iconOnlyWidth() int {
	st := r.style(theme.StateNormal)
	return int(r.metric(KeyTaskButtonIconSize)) + 2*int(st.PadX)
}

// hitIndex возвращает индекс кнопки под точкой (x, y) или -1.
func (r *RunningApplications) hitIndex(x, y int) int {
	pt := image.Pt(x, y)
	for i, wb := range r.btns {
		if pt.In(wb.rect) {
			return i
		}
	}
	return -1
}

// OnMouseMove реализует widget.MouseMoveHandler — обновляет наведённую кнопку.
func (r *RunningApplications) OnMouseMove(x, y int) {
	idx := r.hitIndex(x, y)
	if idx != r.hoverIdx {
		r.hoverIdx = idx
		r.Invalidate()
	}
}

// OnMouseButton реализует widget.MouseClickHandler.
//
// Левая кнопка: press взводит кнопку окна под курсором, release срабатывает,
// только если курсор остался над ней (та же семантика отмены отпусканием в
// сторону, что и у StartButton/системных кнопок окна). Активное окно
// сворачивается, неактивное — активируется (поведение панели задач
// Windows). Средняя кнопка мыши закрывает окно сразу по нажатию.
func (r *RunningApplications) OnMouseButton(e widget.MouseEvent) bool {
	idx := r.hitIndex(e.X, e.Y)

	switch e.Button {
	case widget.MouseLeft:
		if e.Pressed {
			if idx < 0 {
				return false
			}
			r.armedIdx = idx
			r.Invalidate()
			return true
		}
		wasArmed := r.armedIdx
		r.armedIdx = -1
		r.Invalidate()
		if wasArmed < 0 {
			return false
		}
		if wasArmed == idx {
			info := r.btns[wasArmed].info
			if info.Active {
				r.wm.Minimize(info.ID)
			} else {
				r.wm.Activate(info.ID)
			}
		}
		return true
	case widget.MouseMiddle:
		if !e.Pressed || idx < 0 {
			return false
		}
		r.wm.Close(r.btns[idx].info.ID)
		return true
	}
	return false
}

// Draw рисует все кнопки окон текущей раскладки: подложку по стилю темы,
// значок (если у окна есть WindowInfo.Icon) и, если showLabel, заголовок.
// ─── Отдача отрисовки презентеру темы ───────────────────────────────────────
//
// Тема вправе принести свою отрисовку компонента: macOS рисует область
// приложений доком, а не полосой кнопок. Компонент об этом не знает — он
// спрашивает, есть ли для него презентер, и если есть, отдаёт ему
// отрисовку и раскладку. Поведение (активация, сворачивание, подписка на
// список окон) остаётся здесь и одинаково для всех тем.

// Theme реализует Component: презентеру нужен доступ к теме.
func (r *RunningApplications) Theme() *theme.Manager { return r.tm }

// Cells реализует Component: содержимое области для чужой отрисовки.
func (r *RunningApplications) Cells() []Cell {
	cells := make([]Cell, 0, len(r.btns))
	for _, wb := range r.btns {
		cells = append(cells, Cell{
			Title:  wb.info.Title,
			Icon:   wb.info.Icon,
			Active: wb.info.Active,
			Muted:  wb.info.Minimized,
		})
	}
	return cells
}

// HoverIndex реализует Component.
func (r *RunningApplications) HoverIndex() int { return r.hoverIdx }

// presenter возвращает презентер, назначенный темой этому компоненту.
func (r *RunningApplications) presenter() Presenter {
	return PresenterFor(r.tm, PresenterKeyRunningApps)
}

// PresenterKeyRunningApps — имя, под которым профиль темы назначает
// презентер области запущенных приложений.
const PresenterKeyRunningApps = "runningapps"

func (r *RunningApplications) Draw(ctx widget.DrawContext) {
	b := r.Bounds()
	if b.Empty() {
		return
	}
	if p := r.presenter(); p != nil {
		p.Draw(ctx, r)
		return
	}
	iconSize := int(r.metric(KeyTaskButtonIconSize))
	labelGap := int(r.metric(KeyTaskButtonLabelGap))

	prevClip := ctx.Clip()
	for i, wb := range r.btns {
		// Активное окно — состоянием StateActive; свёрнутое рисуется
		// приглушённо состоянием StateDisabled (не как «недоступная»
		// кнопка в смысле ввода — клик по ней по-прежнему разворачивает
		// окно, приглушение чисто визуальное: так их видно от обычных
		// свёрнутых на настоящей панели задач).
		st := StateOf(i == r.hoverIdx, i == r.armedIdx, wb.info.Active, wb.info.Minimized, false)
		style := r.style(st)
		PaintStyle(ctx, wb.rect, style)

		padX := int(style.PadX)
		iconX := wb.rect.Min.X + padX
		iconY := wb.rect.Min.Y + (wb.rect.Dy()-iconSize)/2
		if wb.info.Icon != nil && iconSize > 0 {
			ctx.DrawImageScaled(wb.info.Icon, iconX, iconY, iconSize, iconSize)
		}

		if wb.showLabel {
			// Заголовок клипуется по кнопке: он не должен наезжать на
			// соседнюю кнопку, даже если строка длиннее отведённого места.
			ctx.SetClip(wb.rect.Intersect(prevClip))
			textLeft := iconX + iconSize + labelGap
			labelRect := image.Rect(textLeft-int(style.PadX), wb.rect.Min.Y, wb.rect.Max.X, wb.rect.Max.Y)
			DrawTextLeft(ctx, labelRect, wb.info.Title, style)
			ctx.SetClip(prevClip)
		}
	}

	r.DrawChildren(ctx)
}

// metric читает метрику темы (0, если темы нет).
func (r *RunningApplications) metric(k theme.Key) float64 {
	if r.tm == nil {
		return 0
	}
	return r.tm.GetMetric(k)
}

// style возвращает стиль кнопки окна из активной темы для состояния st.
func (r *RunningApplications) style(st theme.State) *theme.Style {
	if r.tm == nil {
		return &theme.Style{}
	}
	return r.tm.GetStyle(ComponentTaskButton, "", st)
}
