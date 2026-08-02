package widget

import (
	"image"
	"image/color"
	"math"
	"sync"
	"time"
)

// ScrollView — прокручиваемый контейнер с вертикальным скроллбаром.
//
// Содержимое может быть больше видимой области. Виджет отсекает
// рисование дочерних элементов по своим границам и управляет
// вертикальным смещением (scrollY).
//
// Скроллбар появляется только когда ContentHeight > высоты виджета.
type ScrollView struct {
	Base

	Background   color.RGBA
	TrackColor   color.RGBA // фон трека скроллбара
	ThumbColor   color.RGBA // ползунок
	ThumbHoverBG color.RGBA
	ShowBorder   bool
	BorderColor  color.RGBA

	ContentHeight int // полная высота содержимого (задаётся вручную или автоматически)

	mu      sync.Mutex
	scrollY int // текущее смещение прокрутки (>=0)

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
func (sv *ScrollView) maxScroll() int {
	viewH := sv.bounds.Dy()
	if sv.ContentHeight <= viewH {
		return 0
	}
	return sv.ContentHeight - viewH
}

// needsScrollbar возвращает true, если содержимое больше видимой области.
func (sv *ScrollView) needsScrollbar() bool {
	return sv.ContentHeight > sv.bounds.Dy()
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

	trackX := b.Max.X - sv.scrollbarWidth
	top, workH := sbWorkArea(b, sv.scrollbarWidth) // в классике — между кнопками ▲▼
	ratio := float64(b.Dy()) / float64(sv.ContentHeight)
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

// Draw рисует ScrollView с клиппингом и скроллбаром.
func (sv *ScrollView) Draw(ctx DrawContext) {
	b := sv.bounds
	if b.Empty() {
		return
	}
	sv.mu.Lock()
	scrollY := sv.scrollY
	sv.mu.Unlock()

	// Фон
	if sv.Background.A > 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sv.Background)
	}

	// Клиппинг для содержимого
	contentW := sv.contentWidth()
	ctx.SetClip(image.Rect(b.Min.X, b.Min.Y, b.Min.X+contentW, b.Max.Y))

	// Рисуем дочерние элементы со смещением
	// Каждый дочерний виджет должен быть позиционирован относительно ScrollView.
	// Мы сдвигаем их bounds на -scrollY перед отрисовкой и возвращаем обратно.
	for _, child := range sv.children {
		origBounds := child.Bounds()
		shifted := origBounds.Add(image.Pt(0, -scrollY))
		// Пропускаем невидимые элементы
		if shifted.Max.Y < b.Min.Y || shifted.Min.Y > b.Max.Y {
			continue
		}
		child.SetBounds(shifted)
		child.Draw(ctx)
		child.SetBounds(origBounds) // восстанавливаем
	}

	ctx.ClearClip()

	// Скроллбар
	if sv.needsScrollbar() {
		trackX := b.Max.X - sv.scrollbarWidth
		ctx.FillRect(trackX, b.Min.Y, sv.scrollbarWidth, b.Dy(), sv.TrackColor)

		sv.mu.Lock()
		tr := sv.thumbRect()
		sv.mu.Unlock()

		tc := sv.ThumbColor
		if sv.thumbHovered || sv.dragging {
			tc = sv.ThumbHoverBG
		}
		if st := currentStyle(); st.Classic3D {
			// Классика: кнопки ▲/▼ на концах + выпуклый ползунок.
			track := image.Rect(trackX, b.Min.Y, b.Max.X, b.Max.Y)
			drawClassicScrollbar(ctx, track, tr, st, sv.ThumbColor, win10.LabelText)
		} else {
			ctx.FillRoundRect(tr.Min.X+1, tr.Min.Y+1, tr.Dx()-2, tr.Dy()-2, 3, tc)
		}
	}

	// Рамка
	if sv.ShowBorder {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sv.BorderColor)
	}

	sv.drawDisabledOverlay(ctx)
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
		if !sv.needsScrollbar() {
			return false // нечего прокручивать — пусть событие всплывёт выше
		}
		const wheelStep = 40
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
		// Клик на скроллбаре: кнопки ▲/▼ (классика) или прыжок по треку.
		b := sv.bounds
		trackX := b.Max.X - sv.scrollbarWidth
		if e.X >= trackX && e.X <= b.Max.X && sv.needsScrollbar() {
			if currentStyle().Classic3D {
				btn := classicSBBtnH(sv.scrollbarWidth)
				const arrowStep = 40
				if e.Y < b.Min.Y+btn {
					if sv.setScrollYLocked(sv.scrollY - arrowStep) {
						sv.Invalidate()
					}
					return true
				}
				if e.Y >= b.Max.Y-btn {
					if sv.setScrollYLocked(sv.scrollY + arrowStep) {
						sv.Invalidate()
					}
					return true
				}
			}
			top, workH := sbWorkArea(b, sv.scrollbarWidth)
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
	}
	return false
}

// WantsCapture захватывает мышь при нажатии ЛКМ на ползунке скроллбара:
// во время перетаскивания курсор свободно выходит за границы виджета, и без
// захвата drag обрывался бы на краю (а с адресной доставкой мыши события
// вне bounds иначе вовсе не приходят).
func (sv *ScrollView) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if !sv.needsScrollbar() {
		return false
	}
	return image.Pt(e.X, e.Y).In(sv.thumbRect())
}

// OnMouseMove обрабатывает перемещение мыши (drag скроллбара, hover).
func (sv *ScrollView) OnMouseMove(x, y int) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if sv.dragging {
		dy := y - sv.dragStartY
		_, workH := sbWorkArea(sv.bounds, sv.scrollbarWidth)
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

// OnMouseWheelPixels — плавная прокрутка точной пиксельной дельтой (колесо
// высокой точности / тачпад). dy>0 — вниз. Возвращает true, если событие
// поглощено; false — если прокручивать нечего или мы упёрлись в край в
// сторону жеста (тогда дельта всплывёт к родителю).
//
// В обычных темах дельта запускает «маховик» инерции (импульс скорости,
// затухание на часах движка). В Classic3D — мгновенно, без инерции.
func (sv *ScrollView) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	if !sv.IsEnabled() {
		return false
	}
	sv.mu.Lock()
	if sv.ContentHeight <= sv.bounds.Dy() {
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
