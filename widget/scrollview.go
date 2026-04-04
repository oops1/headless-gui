package widget

import (
	"image"
	"image/color"
	"sync"
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

	// Скроллбар
	scrollbarWidth int // ширина полосы (по умолчанию 10)
	dragging       bool
	dragStartY     int
	dragStartScr   int
	thumbHovered   bool
}

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
	defer sv.mu.Unlock()
	sv.setScrollYLocked(y)
}

func (sv *ScrollView) setScrollYLocked(y int) {
	maxY := sv.maxScroll()
	if y < 0 {
		y = 0
	}
	if y > maxY {
		y = maxY
	}
	sv.scrollY = y
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

	viewH := b.Dy()
	trackX := b.Max.X - sv.scrollbarWidth
	ratio := float64(viewH) / float64(sv.ContentHeight)
	thumbH := int(ratio * float64(viewH))
	if thumbH < 20 {
		thumbH = 20
	}
	if thumbH > viewH {
		thumbH = viewH
	}

	maxS := sv.maxScroll()
	var thumbY int
	if maxS > 0 {
		thumbY = int(float64(sv.scrollY) / float64(maxS) * float64(viewH-thumbH))
	}

	return image.Rect(trackX, b.Min.Y+thumbY, b.Max.X, b.Min.Y+thumbY+thumbH)
}

// Draw рисует ScrollView с клиппингом и скроллбаром.
func (sv *ScrollView) Draw(ctx DrawContext) {
	b := sv.bounds
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
		ctx.FillRoundRect(tr.Min.X+1, tr.Min.Y+1, tr.Dx()-2, tr.Dy()-2, 3, tc)
	}

	// Рамка
	if sv.ShowBorder {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), sv.BorderColor)
	}

	sv.drawDisabledOverlay(ctx)
}

// OnMouseButton обрабатывает клик на скроллбаре (drag ползунка).
func (sv *ScrollView) OnMouseButton(e MouseEvent) bool {
	if !sv.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft {
		return false
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
			return true
		}
		// Клик на треке — прыжок к позиции
		b := sv.bounds
		trackX := b.Max.X - sv.scrollbarWidth
		if e.X >= trackX && e.X <= b.Max.X {
			ratio := float64(e.Y-b.Min.Y) / float64(b.Dy())
			sv.setScrollYLocked(int(ratio * float64(sv.ContentHeight)))
			return true
		}
	} else {
		if sv.dragging {
			sv.dragging = false
			return true
		}
	}
	return false
}

// OnMouseMove обрабатывает перемещение мыши (drag скроллбара, hover).
func (sv *ScrollView) OnMouseMove(x, y int) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	if sv.dragging {
		dy := y - sv.dragStartY
		viewH := sv.bounds.Dy()
		tr := sv.thumbRect()
		thumbH := tr.Dy()
		trackUsable := viewH - thumbH
		if trackUsable > 0 {
			scrollDelta := int(float64(dy) / float64(trackUsable) * float64(sv.maxScroll()))
			sv.setScrollYLocked(sv.dragStartScr + scrollDelta)
		}
		return
	}

	// Hover на ползунке
	if sv.needsScrollbar() {
		tr := sv.thumbRect()
		sv.thumbHovered = image.Pt(x, y).In(tr)
	}
}

// ScrollBy прокручивает на delta пикселей (положительное — вниз).
func (sv *ScrollView) ScrollBy(delta int) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.setScrollYLocked(sv.scrollY + delta)
}

// ApplyTheme обновляет цвета ScrollView.
func (sv *ScrollView) ApplyTheme(t *Theme) {
	sv.TrackColor = t.ScrollTrackBG
	sv.ThumbColor = t.ScrollThumbBG
	sv.ThumbHoverBG = t.Accent
	sv.BorderColor = t.Border
}
