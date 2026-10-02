package widget

import (
	"image"
	"image/color"
	"math"
	"sync"
	"time"
)

// ScrollView — прокручиваемый контейнер со скроллбарами.
//
// Содержимое может быть больше видимой области. Виджет отсекает
// рисование дочерних элементов по своим границам и управляет
// смещением (scrollY по вертикали, scrollX по горизонтали).
//
// Вертикальная полоса появляется только когда ContentHeight > высоты виджета.
// Горизонтальная — только когда задан ContentWidth и он больше ширины виджета;
// при нулевом ContentWidth горизонтальной прокрутки нет вовсе (см. scrollview_hscroll.go).
type ScrollView struct {
	Base

	Background   color.RGBA
	TrackColor   color.RGBA // фон трека скроллбара
	ThumbColor   color.RGBA // ползунок
	ThumbHoverBG color.RGBA
	ShowBorder   bool
	BorderColor  color.RGBA

	ContentHeight int // полная высота содержимого (задаётся вручную или автоматически)

	// ContentWidth — полная ширина содержимого. Ноль (по умолчанию) означает
	// «горизонтальной прокрутки нет»: содержимое шире области по-прежнему просто
	// обрезается. Значение по умолчанию не может быть «ширина по детям» — тогда
	// у каждого, кто уже держит в ScrollView широкие дети и не просил полосу,
	// она внезапно появилась бы и отняла бы у содержимого высоту.
	ContentWidth int

	mu      sync.Mutex
	scrollY int // текущее смещение прокрутки (>=0)
	scrollX int // смещение вбок (>=0); всегда 0, пока ContentWidth не задан

	// Горизонтальная полоса (подробности — в scrollview_hscroll.go).
	//   hdragging/hdragGrab — идёт перетаскивание ползунка и на сколько пикселей
	//     от его левого края взялись (чтобы содержимое не прыгало под курсор);
	//   hthumbHovered — курсор над ползунком (подсветка);
	//   scrollFracX — субпиксельный остаток горизонтальной пиксельной прокрутки.
	hdragging     bool
	hdragGrab     int
	hthumbHovered bool
	scrollFracX   float64

	// Плавный скролл (пиксельные дельты + инерция).
	//   scrollFrac — субпиксельный остаток пиксельной прокрутки (тачпад
	//     высокой точности отдаёт дробные дельты; без накопления они теряются);
	//   vel — текущая скорость инерции, px/с (знак = направление);
	//   inertiaAnim — «маховик» инерции на часах движка (без горутин);
	//   lastElapsed — прошедшее время предыдущего тика (сек, из прогресса
	//     анимации), для вычисления dt на часах движка.
	scrollFrac  float64
	vel         float64
	inertiaAnim *Animation
	lastElapsed float64

	// Скроллбар
	scrollbarWidth int // ширина полосы (по умолчанию 10)
	dragging       bool
	dragStartY     int
	dragStartScr   int
	thumbHovered   bool

	// Кэш транслирующей обёртки DrawContext (PERF-12, см. Draw): создаётся один
	// раз на конкретный внешний контекст и переиспользуется между кадрами.
	// Трогается ТОЛЬКО из Draw (рендер-горутина), поэтому мьютекс не нужен.
	offCtx     DrawContext
	offCtxBase *svOffsetCtx
	offCtxFor  DrawContext
}

// Параметры инерции ScrollView.
const (
	// inertiaTau — постоянная времени экспоненциального затухания (сек).
	// Импульс dy инжектируется как dy/inertiaTau, поэтому суммарный путь
	// маховика ≈ dy (v0·τ), т.е. пиксельная дельта доезжает целиком, но плавно.
	inertiaTau = 0.30
	// inertiaMinVel — порог остановки маховика (px/с). При τ=0.30 недоезд на
	// остановке < inertiaMinVel·τ ≈ 0.6 px.
	inertiaMinVel = 2.0
	// inertiaDuration — длительность несущей анимации; на порядки больше
	// времени затухания, реально маховик сам останавливается по inertiaMinVel.
	inertiaDuration = 4 * time.Second
)

var inertiaDurationSec = inertiaDuration.Seconds()

// NewScrollView создаёт прокручиваемый контейнер.
func NewScrollView() *ScrollView {
	return &ScrollView{
		Background:     color.RGBA{A: 0}, // прозрачный
		TrackColor:     win10.ScrollTrackBG,
		ThumbColor:     win10.ScrollThumbBG,
		ThumbHoverBG:   win10.Accent,
		BorderColor:    win10.Border,
		scrollbarWidth: 10,
	}
}

// ScrollY возвращает текущее смещение прокрутки.
func (sv *ScrollView) ScrollY() int {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.scrollY
}

// SetScrollY задаёт смещение прокрутки с ограничением.
func (sv *ScrollView) SetScrollY(y int) {
	sv.mu.Lock()
	changed := sv.setScrollYLocked(y)
	sv.mu.Unlock()
	if changed {
		sv.Invalidate()
	}
}

// setScrollYLocked зажимает и применяет scrollY; возвращает true,
// если смещение фактически изменилось (для авто-инвалидации).
func (sv *ScrollView) setScrollYLocked(y int) bool {
	maxY := sv.maxScroll()
	if y < 0 {
		y = 0
	}
	if y > maxY {
		y = maxY
	}
	if sv.scrollY == y {
		return false
	}
	sv.scrollY = y
	return true
}

// maxScroll возвращает максимальное значение scrollY.
//
// Видимая высота — без горизонтальной полосы: иначе последние строки
// содержимого так и остались бы под ней, и до них было бы не доехать.
func (sv *ScrollView) maxScroll() int {
	viewH := sv.contentHeight()
	if sv.ContentHeight <= viewH {
		return 0
	}
	return sv.ContentHeight - viewH
}

// needsScrollbar возвращает true, если нужна вертикальная полоса.
func (sv *ScrollView) needsScrollbar() bool {
	vert, _ := sv.bars()
	return vert
}

// contentWidth возвращает ширину контентной области (без скроллбара).
func (sv *ScrollView) contentWidth() int {
	w := sv.bounds.Dx()
	if sv.needsScrollbar() {
		w -= sv.scrollbarWidth
	}
	return w
}

// thumbRect возвращает прямоугольник ползунка скроллбара.
func (sv *ScrollView) thumbRect() image.Rectangle {
	b := sv.bounds
	if !sv.needsScrollbar() {
		return image.Rectangle{}
	}

	// Полоса кончается над горизонтальной (если она есть), а доля видимого
	// считается от видимой высоты: иначе ползунок был бы длиннее, чем надо, и
	// не доходил до низа трека.
	vb := sv.vbarRect()
	trackX := vb.Min.X
	top, workH := sbWorkArea(vb, sv.scrollbarWidth) // в классике — между кнопками ▲▼
	ratio := float64(vb.Dy()) / float64(sv.ContentHeight)
	thumbH := int(ratio * float64(workH))
	if thumbH < 20 {
		thumbH = 20
	}
	if thumbH > workH {
		thumbH = workH
	}

	maxS := sv.maxScroll()
	var thumbY int
	if maxS > 0 {
		thumbY = int(float64(sv.scrollY) / float64(maxS) * float64(workH-thumbH))
	}

	return image.Rect(trackX, top+thumbY, b.Max.X, top+thumbY+thumbH)
}

// Draw рисует ScrollView с клиппингом и скроллбарами.
func (sv *ScrollView) Draw(ctx DrawContext) {
	b := sv.bounds
	if b.Empty() {
		return
	}
	// Всё, что кадр берёт из изменяемого состояния, снимаем одним захватом
	// замка: события мыши идут из другой горутины и двигают смещения и флаги
	// подсветки посреди кадра. Раздельные чтения дали бы кадр, где содержимое
	// уже уехало вбок, а ползунок ещё стоит на старом месте.
	sv.mu.Lock()
	scrollX, scrollY := sv.scrollXLocked(), sv.scrollY
	vert, horiz := sv.bars()
	contentW, contentH := sv.contentWidth(), sv.contentHeight()
	vbar := sv.vbarRect()
	thumb := sv.thumbRect()
	hstrip, htrack, hthumb := sv.hbarStrip(), sv.hbarTrack(), sv.hbarThumbLocked()
	vActive := sv.thumbHovered || sv.dragging
	hActive := sv.hthumbHovered || sv.hdragging
	hScrollable := sv.ContentWidth > 0
	sv.mu.Unlock()

	// Фон
	if sv.Background.A > 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sv.Background)
	}

	// Клиппинг для содержимого: без обеих полос (а без горизонтальной —
	// высота остаётся прежней). Сужаем ПЕРЕСЕЧЕНИЕМ с текущей областью —
	// прокрутка может стоять внутри другой, — и ставим его заново перед
	// КАЖДЫМ ребёнком: ребёнок вправе снять своё сужение вместе с нашим
	// (`ClearClip` снимает всё), и тогда соседи рисовали бы мимо.
	outer := ctx.Clip()
	content := image.Rect(b.Min.X, b.Min.Y, b.Min.X+contentW, b.Min.Y+contentH).Intersect(outer)

	// Рисуем дочерние элементы со смещением.
	//
	// PERF-12: смещение даёт транслирующая обёртка DrawContext, а НЕ временная
	// подмена bounds ребёнка. Прежний код делал SetBounds(shifted) → Draw →
	// SetBounds(orig), и это было плохо сразу трижды:
	//   - гонка данных: hit-test/обработка мыши идут в ДРУГОЙ горутине (события
	//     не берут frameMu) и могли прочитать сдвинутые bounds;
	//   - Base.SetBounds шлёт notifyRectChanged → движок инвалидировался на
	//     КАЖДОМ кадре, и on-demand рендер с живым ScrollView никогда не засыпал;
	//   - три лишних вызова на ребёнка за кадр.
	// Рисуемый результат идентичен: ребёнок отдаёт свои (несдвинутые) координаты,
	// обёртка вычитает scrollX/scrollY на входе в канвас.
	childCtx := ctx
	if scrollX != 0 || scrollY != 0 {
		childCtx = sv.offsetContext(ctx, scrollX, scrollY)
	}
	for _, child := range sv.children {
		shifted := child.Bounds().Add(image.Pt(-scrollX, -scrollY))
		// Пропускаем невидимые элементы
		if shifted.Max.Y < b.Min.Y || shifted.Min.Y > b.Max.Y {
			continue
		}
		// По горизонтали отсекаем только когда прокрутка вбок включена: без неё
		// дети правее области рисовались (и целиком терялись на клипе) всегда, и
		// менять это поведение для тех, кто ContentWidth не задавал, незачем.
		if hScrollable && (shifted.Max.X < b.Min.X || shifted.Min.X > b.Max.X) {
			continue
		}
		ctx.SetClip(content)
		child.Draw(childCtx)
	}

	ctx.SetClip(outer)

	// Вертикальная полоса
	if vert {
		ctx.FillRect(vbar.Min.X, vbar.Min.Y, vbar.Dx(), vbar.Dy(), sv.TrackColor)

		tc := sv.ThumbColor
		if vActive {
			tc = sv.ThumbHoverBG
		}
		if st := currentStyle(); st.Classic3D {
			// Классика: кнопки ▲/▼ на концах + выпуклый ползунок.
			drawClassicScrollbar(ctx, vbar, thumb, st, sv.ThumbColor, win10.LabelText)
		} else {
			ctx.FillRoundRect(thumb.Min.X+1, thumb.Min.Y+1, thumb.Dx()-2, thumb.Dy()-2, 3, tc)
		}
	}

	// Горизонтальная полоса — тем же drawHBar, что и в сравнении/слиянии.
	if horiz {
		ctx.FillRect(hstrip.Min.X, hstrip.Min.Y, hstrip.Dx(), hstrip.Dy(), sv.TrackColor)
		tc := sv.ThumbColor
		if hActive {
			tc = sv.ThumbHoverBG
		}
		drawHBar(ctx, htrack, hthumb, sv.TrackColor, tc)
	}

	// Угол, где полосы сходятся. Вертикальная кончается над горизонтальной, а
	// горизонтальная — левее вертикальной, поэтому ни одна его не закрашивает;
	// заливаем один раз здесь. Если бы каждая полоса тянулась до края, угол
	// рисовался бы дважды, и на полупрозрачной теме он был бы темнее остального.
	if vert && horiz {
		ctx.FillRect(vbar.Min.X, hstrip.Min.Y, vbar.Dx(), hstrip.Dy(), sv.TrackColor)
	}

	// Рамка
	if sv.ShowBorder {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sv.BorderColor)
	}

	sv.drawDisabledOverlay(ctx)
}

// ─── Транслирующий DrawContext (PERF-12) ─────────────────────────────────────

// svOffsetCtx — обёртка DrawContext, сдвигающая все координаты рисования на
// (-dx, -dy). Позволяет ScrollView рисовать содержимое прокрученным, не
// трогая bounds дочерних виджетов (см. Draw).
//
// Сдвиг должен затрагивать КАЖДЫЙ метод, принимающий координату x, и обе
// стороны пары SetClip/Clip. Метод, про который забыли, рисует мимо: после
// прокрутки вбок такой элемент (линия, картинка, сглаженный эллипс) остался бы
// на старом месте, пока всё вокруг уехало.
//
// Опциональные интерфейсы контекста: AAShapes пробрасывается отдельным типом
// (svOffsetCtxAA), чтобы обёртка НЕ заявляла его, когда внутренний контекст его
// не поддерживает — иначе виджеты потеряли бы свой откат на не-AA отрисовку.
// DrawContextAlpha удовлетворяется автоматически (FillRectAlpha входит в
// DrawContext). Scale() безопасно заявлять всегда: контекст без HiDPI отдаёт 1,
// что для потребителей (drawLinearGradient) равносильно отсутствию интерфейса.
// Snapshotter НЕ пробрасывается сознательно: он нужен только оверлею
// DockManager, который рисуется движком поверх дерева, а не внутри ScrollView.
type svOffsetCtx struct {
	inner  DrawContext
	dx, dy int
}

// svOffsetCtxAA — вариант обёртки для контекстов со сглаженными примитивами.
type svOffsetCtxAA struct {
	svOffsetCtx
	aa AAShapes
}

var (
	_ DrawContext      = (*svOffsetCtx)(nil)
	_ DrawContextAlpha = (*svOffsetCtx)(nil)
	_ AAShapes         = (*svOffsetCtxAA)(nil)
)

// offsetContext возвращает обёртку над ctx со сдвигом (dx, dy), переиспользуя
// ранее созданную (внешний контекст между кадрами один и тот же). Вызывается
// только из Draw — рендер-горутина единственная.
func (sv *ScrollView) offsetContext(ctx DrawContext, dx, dy int) DrawContext {
	if sv.offCtx == nil || sv.offCtxFor != ctx {
		if aa, ok := ctx.(AAShapes); ok {
			w := &svOffsetCtxAA{svOffsetCtx: svOffsetCtx{inner: ctx}, aa: aa}
			sv.offCtx, sv.offCtxBase = w, &w.svOffsetCtx
		} else {
			w := &svOffsetCtx{inner: ctx}
			sv.offCtx, sv.offCtxBase = w, w
		}
		sv.offCtxFor = ctx
	}
	sv.offCtxBase.dx, sv.offCtxBase.dy = dx, dy
	return sv.offCtx
}

func (o *svOffsetCtx) FillRect(x, y, w, h int, col color.RGBA) {
	o.inner.FillRect(x-o.dx, y-o.dy, w, h, col)
}

func (o *svOffsetCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	o.inner.FillRectAlpha(x-o.dx, y-o.dy, w, h, col)
}

func (o *svOffsetCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	o.inner.FillRoundRect(x-o.dx, y-o.dy, w, h, r, col)
}

func (o *svOffsetCtx) DrawBorder(x, y, w, h int, col color.RGBA) {
	o.inner.DrawBorder(x-o.dx, y-o.dy, w, h, col)
}

func (o *svOffsetCtx) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	o.inner.DrawRoundBorder(x-o.dx, y-o.dy, w, h, r, col)
}

func (o *svOffsetCtx) SetPixel(x, y int, col color.RGBA) {
	o.inner.SetPixel(x-o.dx, y-o.dy, col)
}

func (o *svOffsetCtx) DrawHLine(x, y, length int, col color.RGBA) {
	o.inner.DrawHLine(x-o.dx, y-o.dy, length, col)
}

func (o *svOffsetCtx) DrawVLine(x, y, length int, col color.RGBA) {
	o.inner.DrawVLine(x-o.dx, y-o.dy, length, col)
}

func (o *svOffsetCtx) DrawImage(src image.Image, x, y int) {
	o.inner.DrawImage(src, x-o.dx, y-o.dy)
}

func (o *svOffsetCtx) DrawImageScaled(src image.Image, x, y, w, h int) {
	o.inner.DrawImageScaled(src, x-o.dx, y-o.dy, w, h)
}

func (o *svOffsetCtx) DrawText(text string, x, y int, col color.RGBA) {
	o.inner.DrawText(text, x-o.dx, y-o.dy, col)
}

func (o *svOffsetCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	o.inner.DrawTextSize(text, x-o.dx, y-o.dy, sizePt, col)
}

func (o *svOffsetCtx) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	o.inner.DrawTextFont(text, x-o.dx, y-o.dy, sizePt, fontName, col)
}

func (o *svOffsetCtx) MeasureText(text string, sizePt float64) int {
	return o.inner.MeasureText(text, sizePt)
}

func (o *svOffsetCtx) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return o.inner.MeasureTextFont(text, sizePt, fontName)
}

func (o *svOffsetCtx) MeasureRunePositions(text string, sizePt float64) []int {
	return o.inner.MeasureRunePositions(text, sizePt)
}

func (o *svOffsetCtx) SetClip(r image.Rectangle) {
	o.inner.SetClip(r.Sub(image.Pt(o.dx, o.dy)))
}

func (o *svOffsetCtx) ClearClip() { o.inner.ClearClip() }

// Clip возвращает область отсечения в системе координат ДЕТЕЙ (несдвинутой):
// внутренний клип + (dx, dy) — симметрично SetClip.
func (o *svOffsetCtx) Clip() image.Rectangle {
	return o.inner.Clip().Add(image.Pt(o.dx, o.dy))
}

// Scale проксирует HiDPI-масштаб внутреннего контекста (physicalScaler).
// Контекст без масштаба отдаёт 1 — как если бы интерфейса не было вовсе.
func (o *svOffsetCtx) Scale() float64 {
	if ps, ok := o.inner.(physicalScaler); ok {
		return ps.Scale()
	}
	return 1
}

func (o *svOffsetCtxAA) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	o.aa.FillEllipseAA(cx-o.dx, cy-o.dy, rx, ry, col)
}

func (o *svOffsetCtxAA) StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA) {
	o.aa.StrokeEllipseAA(cx-o.dx, cy-o.dy, rx, ry, thickness, col)
}

func (o *svOffsetCtxAA) FillPolygonAA(pts []image.Point, col color.RGBA) {
	o.aa.FillPolygonAA(o.shift(pts), col)
}

func (o *svOffsetCtxAA) StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA) {
	o.aa.StrokePolylineAA(o.shift(pts), thickness, closed, col)
}

func (o *svOffsetCtxAA) DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA) {
	o.aa.DrawLineAA(x1-o.dx, y1-o.dy, x2-o.dx, y2-o.dy, thickness, col)
}

// shift возвращает копию точек, сдвинутых на (-dx, -dy).
func (o *svOffsetCtxAA) shift(pts []image.Point) []image.Point {
	if len(pts) == 0 || (o.dx == 0 && o.dy == 0) {
		return pts
	}
	out := make([]image.Point, len(pts))
	for i, p := range pts {
		out[i] = image.Pt(p.X-o.dx, p.Y-o.dy)
	}
	return out
}

// OnMouseButton обрабатывает клик на скроллбаре (drag ползунка) и колесо мыши.
func (sv *ScrollView) OnMouseButton(e MouseEvent) bool {
	if !sv.IsEnabled() {
		return false
	}
	// Колесо мыши: прокрутка содержимого (движок шлёт press+release).
	if e.Button == MouseWheelUp || e.Button == MouseWheelDown {
		if !e.Pressed {
			return true
		}
		const wheelStep = 40
		// Shift+колесо — вбок, но только если есть куда: в ScrollView без
		// горизонтальной полосы Shift по-прежнему ничего не меняет, иначе
		// привычное колесо с зажатым Shift внезапно перестало бы листать.
		if e.Mod&ModShift != 0 && sv.hscrollAvailable() {
			if e.Button == MouseWheelUp {
				sv.ScrollXBy(-wheelStep)
			} else {
				sv.ScrollXBy(wheelStep)
			}
			return true
		}
		if !sv.needsScrollbar() {
			return false // нечего прокручивать — пусть событие всплывёт выше
		}
		if e.Button == MouseWheelUp {
			sv.ScrollBy(-wheelStep)
		} else {
			sv.ScrollBy(wheelStep)
		}
		return true
	}
	if e.Button != MouseLeft {
		return false
	}

	// Любой клик ЛКМ прерывает инерцию (новый ввод перебивает «бросок»).
	if e.Pressed {
		sv.stopInertia()
	}

	sv.mu.Lock()
	defer sv.mu.Unlock()

	if e.Pressed {
		// Проверяем клик на ползунке
		tr := sv.thumbRect()
		if image.Pt(e.X, e.Y).In(tr) {
			sv.dragging = true
			sv.dragStartY = e.Y
			sv.dragStartScr = sv.scrollY
			sv.Invalidate() // ползунок подсвечивается при drag
			return true
		}
		// Горизонтальная полоса: ползунок или прыжок по треку.
		if sv.hbarPressLocked(e) {
			return true
		}
		// Клик на скроллбаре: кнопки ▲/▼ (классика) или прыжок по треку.
		// Трек — это vbarRect: он короче виджета на высоту горизонтальной полосы,
		// а угол между полосами не принадлежит ни одной из них.
		b := sv.bounds
		vb := sv.vbarRect()
		trackX := b.Max.X - sv.scrollbarWidth
		inCorner := sv.needsHScrollbar() && e.Y >= vb.Max.Y
		if e.X >= trackX && e.X <= b.Max.X && !inCorner && sv.needsScrollbar() {
			if currentStyle().Classic3D {
				btn := classicSBBtnH(sv.scrollbarWidth)
				const arrowStep = 40
				if e.Y < vb.Min.Y+btn {
					if sv.setScrollYLocked(sv.scrollY - arrowStep) {
						sv.Invalidate()
					}
					return true
				}
				if e.Y >= vb.Max.Y-btn {
					if sv.setScrollYLocked(sv.scrollY + arrowStep) {
						sv.Invalidate()
					}
					return true
				}
			}
			top, workH := sbWorkArea(vb, sv.scrollbarWidth)
			ratio := float64(e.Y-top) / float64(workH)
			if sv.setScrollYLocked(int(ratio * float64(sv.ContentHeight))) {
				sv.Invalidate()
			}
			return true
		}
	} else {
		if sv.dragging {
			sv.dragging = false
			sv.Invalidate() // подсветка ползунка гаснет
			return true
		}
		if sv.hdragging {
			sv.hdragging = false
			sv.Invalidate()
			return true
		}
	}
	return false
}

// WantsCapture захватывает мышь при нажатии ЛКМ на ползунке скроллбара:
// во время перетаскивания курсор свободно выходит за границы виджета, и без
// захвата drag обрывался бы на краю (а с адресной доставкой мыши события
// вне bounds иначе вовсе не приходят).
//
// Горизонтальную полосу захватываем целиком, а не только ползунок: она лежит
// у нижнего края, где наверняка тянутся дети (их bounds не сдвинуты прокруткой),
// и без захвата щелчок мимо ползунка доставался бы глубокому ребёнку под полосой,
// а не ScrollView.
func (sv *ScrollView) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if !sv.needsScrollbar() && !sv.needsHScrollbar() {
		return false
	}
	pt := image.Pt(e.X, e.Y)
	if pt.In(sv.hbarStrip()) {
		return true
	}
	return sv.needsScrollbar() && pt.In(sv.thumbRect())
}

// OnMouseMove обрабатывает перемещение мыши (drag скроллбара, hover).
func (sv *ScrollView) OnMouseMove(x, y int) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if sv.hdragging {
		if sv.hbarDragToLocked(x) {
			sv.Invalidate()
		}
		return
	}

	if sv.dragging {
		dy := y - sv.dragStartY
		_, workH := sbWorkArea(sv.vbarRect(), sv.scrollbarWidth)
		tr := sv.thumbRect()
		thumbH := tr.Dy()
		trackUsable := workH - thumbH
		if trackUsable > 0 {
			scrollDelta := int(float64(dy) / float64(trackUsable) * float64(sv.maxScroll()))
			if sv.setScrollYLocked(sv.dragStartScr + scrollDelta) {
				sv.Invalidate()
			}
		}
		return
	}

	// Hover на ползунке
	if sv.needsScrollbar() {
		tr := sv.thumbRect()
		hov := image.Pt(x, y).In(tr)
		if hov != sv.thumbHovered {
			sv.thumbHovered = hov
			sv.Invalidate()
		}
	}

	// Hover на горизонтальном ползунке. Сбрасывается и тогда, когда полосы уже
	// нет (ContentWidth уменьшили): иначе подсветка осталась бы залипшей.
	if hov := sv.hbarThumbHitLocked(x, y); hov != sv.hthumbHovered {
		sv.hthumbHovered = hov
		sv.Invalidate()
	}
}

// ScrollBy прокручивает на delta пикселей (положительное — вниз).
func (sv *ScrollView) ScrollBy(delta int) {
	sv.mu.Lock()
	changed := sv.setScrollYLocked(sv.scrollY + delta)
	sv.mu.Unlock()
	if changed {
		sv.Invalidate()
	}
}

// wheelPixelsY — вертикальная часть плавной прокрутки точной пиксельной
// дельтой (колесо высокой точности / тачпад). dy>0 — вниз. Возвращает true,
// если дельта поглощена; false — если прокручивать нечего или мы упёрлись в
// край в сторону жеста (тогда дельта всплывёт к родителю).
//
// В обычных темах дельта запускает «маховик» инерции (импульс скорости,
// затухание на часах движка). В Classic3D — мгновенно, без инерции.
func (sv *ScrollView) wheelPixelsY(dy float64) bool {
	if !sv.IsEnabled() {
		return false
	}
	sv.mu.Lock()
	if !sv.needsScrollbar() {
		sv.mu.Unlock()
		return false // нечего прокручивать — пусть всплывёт выше
	}
	// Упёрлись в край в сторону жеста — отдаём событие родителю.
	if (dy < 0 && sv.scrollY <= 0) || (dy > 0 && sv.scrollY >= sv.maxScroll()) {
		sv.mu.Unlock()
		return false
	}

	if currentStyle().Classic3D {
		// Классика: без инерции, дельта применяется сразу (с субпиксельным
		// накоплением, чтобы дробные дельты тачпада не терялись).
		sv.scrollFrac += dy
		whole := math.Trunc(sv.scrollFrac)
		sv.scrollFrac -= whole
		changed := sv.setScrollYLocked(sv.scrollY + int(whole))
		sv.mu.Unlock()
		if changed {
			sv.Invalidate()
		}
		return true
	}

	// Маховик: инжектируем импульс скорости. Стационарный путь ≈ dy.
	sv.vel += dy / inertiaTau
	sv.mu.Unlock()
	sv.ensureInertia()
	return true
}

// ensureInertia запускает несущую анимацию инерции, если она ещё не идёт.
// Скорость (sv.vel) уже задана вызывающим. Часы — движка (tick через
// StepAnimations); ни одной горутины.
//
// Инерция только вертикальная, и это сделано сознательно: маховик держит одну
// скорость, один остаток и один признак «упёрлись в край». Вторую ось пришлось бы
// вести парой (вектор скорости, два края, остановка по обоим), а выигрыш мал —
// вбок листают редко и обычно короткими рывками. Горизонтальная пиксельная
// дельта применяется сразу (см. wheelPixelsX в scrollview_hscroll.go).
func (sv *ScrollView) ensureInertia() {
	sv.mu.Lock()
	if sv.inertiaAnim != nil && sv.inertiaAnim.Running() {
		sv.mu.Unlock()
		return
	}
	sv.lastElapsed = 0
	sv.mu.Unlock()
	a := AnimateOwned(sv, "scroll-inertia", inertiaDuration, nil, sv.inertiaTick)
	sv.mu.Lock()
	sv.inertiaAnim = a
	sv.mu.Unlock()
}

// inertiaTick — шаг маховика: интегрируем скорость в позицию и экспоненциально
// затухаем. dt берём из прогресса t (часы движка), а не из time.Now.
func (sv *ScrollView) inertiaTick(t float64) {
	sv.mu.Lock()
	elapsed := t * inertiaDurationSec
	dt := elapsed - sv.lastElapsed
	sv.lastElapsed = elapsed
	if dt <= 0 {
		sv.mu.Unlock()
		return
	}
	sv.scrollFrac += sv.vel * dt
	whole := math.Trunc(sv.scrollFrac)
	sv.scrollFrac -= whole
	changed := sv.setScrollYLocked(sv.scrollY + int(whole))
	sv.vel *= math.Exp(-dt / inertiaTau)
	atEdge := sv.scrollY <= 0 || sv.scrollY >= sv.maxScroll()
	stop := math.Abs(sv.vel) < inertiaMinVel || atEdge
	if stop {
		sv.vel = 0
		sv.scrollFrac = 0
	}
	a := sv.inertiaAnim
	sv.mu.Unlock()
	if changed {
		sv.Invalidate()
	}
	if stop && a != nil {
		a.Stop()
	}
}

// stopInertia гасит инерцию (новый ввод/клик перебивает «бросок»).
func (sv *ScrollView) stopInertia() {
	sv.mu.Lock()
	a := sv.inertiaAnim
	sv.inertiaAnim = nil
	sv.vel = 0
	sv.scrollFrac = 0
	sv.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// ApplyTheme обновляет цвета ScrollView.
func (sv *ScrollView) ApplyTheme(t *Theme) {
	sv.TrackColor = t.ScrollTrackBG
	sv.ThumbColor = t.ScrollThumbBG
	sv.ThumbHoverBG = t.Accent
	sv.BorderColor = t.Border
	// Непрозрачный фон следует за темой (единая семантика SetTheme).
	if sv.Background.A > 0 {
		sv.Background = t.PanelBG
	}
}
