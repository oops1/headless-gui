// iconbutton.go — общая основа кнопок панели Windows 11: Task View, виджеты,
// группа значков трея.
//
// Кнопка панели — плашка со скруглением (подсветка при наведении, нажатии и
// «открыта панель»), на которой лежит значок. Поведение у всех одно и то же и
// повторяет «Пуск»: нажатие взводит кнопку, щелчок срабатывает на отпускании над
// ней, Enter и Space — как щелчок, рамка фокуса только от клавиатуры, подсветка
// «открыта» следует за всплывающей панелью (Track / TrackManager). Свои у каждой
// кнопки размер и рисунок значка.
//
// Основа не реализует OnKeyEvent: стрелки переносят фокус к соседу через
// навигатор панели, а тому нужен внешний виджет, а не встроенная основа — поэтому
// каждая кнопка зовёт HandleKey сама.
package desktop

import (
	"image"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// iconButton — состояние и ввод кнопки с плашкой. Не элемент панели сам по себе
// (нет PreferredSize и Draw): встраивается в конкретные кнопки.
type iconButton struct {
	widget.Base
	FocusState

	tm   *theme.Manager
	comp string // имя компонента для стилей темы

	// OnClick вызывается при щелчке (отпускание над кнопкой) и по Enter/Space.
	OnClick func()

	hovered int32
	pressed int32
	active  int32

	// fade — плавный переход цвета плашки (тема: taskbar.item).
	fade motion
}

func (b *iconButton) init(tm *theme.Manager, comp string) {
	b.tm, b.comp = tm, comp
}

// metric читает метрику темы как целое (0 — темы нет или метрика не объявлена).
func (b *iconButton) metric(k theme.Key) int { return int(tmMetric(b.tm, k)) }

// Active сообщает, горит ли кнопка как «панель открыта».
func (b *iconButton) Active() bool { return atomic.LoadInt32(&b.active) == 1 }

// SetActive зажигает или гасит кнопку (состояние StateActive).
func (b *iconButton) SetActive(v bool) {
	want := b2i32(v)
	if atomic.SwapInt32(&b.active, want) != want {
		b.Invalidate()
	}
}

// Track связывает кнопку с источником открытия (*Flyout и любая панель,
// встраивающая его): она горит, пока тот открыт. Возвращает функцию, которая
// разрывает связь и гасит кнопку.
func (b *iconButton) Track(src OpenStateSource) (untrack func()) {
	if src == nil {
		return func() {}
	}
	b.SetActive(src.IsOpen())
	unsub := src.Subscribe(b.SetActive)
	return func() {
		unsub()
		b.SetActive(false)
	}
}

// TrackManager — то же для панели name менеджера всплывающих панелей: кнопка
// горит между событиями FlyoutOpened и FlyoutClosed этой панели.
func (b *iconButton) TrackManager(m *FlyoutManager, name string) (untrack func()) {
	if m == nil {
		return func() {}
	}
	b.SetActive(m.IsOpen(name))
	unsub := m.Subscribe(func(ev FlyoutEvent) {
		if ev.Name == name {
			b.SetActive(ev.Kind == FlyoutOpened)
		}
	})
	return func() {
		unsub()
		b.SetActive(false)
	}
}

// OnMouseMove обновляет наведение.
func (b *iconButton) OnMouseMove(x, y int) {
	trayHandleMove(&b.hovered, b.Bounds(), x, y, b.Invalidate)
}

// OnMouseButton: нажатие взводит кнопку, отпускание над ней — щелчок.
func (b *iconButton) OnMouseButton(e widget.MouseEvent) bool {
	if b.NotePointer(e) {
		b.Invalidate()
	}
	return trayHandleClick(&b.pressed, b.Bounds(), e, b.OnClick, b.Invalidate)
}

// SetFocused реализует widget.Focusable.
func (b *iconButton) SetFocused(v bool) {
	if b.FocusState.Set(v) {
		b.Invalidate()
	}
}

// TabIndex исключает кнопку из обхода, пока у неё нет места на панели.
func (b *iconButton) TabIndex() int {
	if b.Bounds().Empty() {
		return -1
	}
	return 0
}

// FocusRing реализует FocusRinger: рамка обводит всю плашку.
func (b *iconButton) FocusRing() (image.Rectangle, *theme.Style) {
	return b.Bounds(), styleNormal(b.tm, b.comp)
}

// style — стиль плашки в состоянии st (с плавным переходом цвета).
func (b *iconButton) style(r image.Rectangle) *theme.Style {
	st := StateOf(atomic.LoadInt32(&b.hovered) == 1, atomic.LoadInt32(&b.pressed) == 1, b.Active(), false, b.FocusVisible())
	return b.fade.ItemStyle(b.tm, 0, r, st, func(st theme.State) *theme.Style {
		return styleOf(b.tm, b.comp, "", st)
	})
}

// paintPlate рисует плашку и возвращает её стиль (по нему рисуется значок).
func (b *iconButton) paintPlate(ctx widget.DrawContext) (image.Rectangle, *theme.Style) {
	r := b.Bounds()
	if r.Empty() {
		return r, nil
	}
	s := b.style(r)
	PaintStyle(ctx, r, s)
	return r, s
}

// drawIconIn рисует значок в квадрате по центру области r: картинка
// потребителя, иначе иконка темы по ключам, иначе встроенный векторный глиф,
// перекрашенный цветом текста стиля.
func drawIconIn(ctx widget.DrawContext, tm *theme.Manager, r image.Rectangle, size int, s *theme.Style,
	img image.Image, at func(size int) image.Image, keys []theme.Key, glyph StartGlyph, memo *glyphMemo) {

	if size <= 0 || r.Empty() {
		return
	}
	x, y := r.Min.X+(r.Dx()-size)/2, r.Min.Y+(r.Dy()-size)/2
	box := image.Rect(x, y, x+size, y+size)
	switch {
	case img != nil || at != nil:
		drawAppIcon(ctx, img, at, box)
	case drawThemeGlyph(ctx, tm, keys, box, s, memo):
	default:
		drawGlyph(ctx, glyph, box, ink(s))
	}
}
