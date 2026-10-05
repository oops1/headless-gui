package widget

import (
	"image"
	"image/color"
	"testing"
	"time"
)

// Тонкая полоса прокрутки: поверх содержимого, скрыта в покое, появляется при
// движении мыши и прокрутке, гаснет после паузы; обычный ScrollView не меняется.

// thinVirtualClock подменяет часы активности и возвращает функцию «прошло d».
type thinVirtualClock struct{ now time.Time }

func (c *thinVirtualClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// newThinFixture собирает прокрутку 200×100 с содержимым высотой 400, тонкой
// полосой и виртуальным временем: step(d) двигает часы и анимации вместе.
func newThinFixture(t *testing.T) (*ScrollView, func(d time.Duration)) {
	t.Helper()
	StopAllAnimations()
	t.Cleanup(StopAllAnimations)

	clk := &thinVirtualClock{now: time.Unix(1000, 0)}
	prevClock := thinClock
	thinClock = func() time.Time { return clk.now }
	t.Cleanup(func() { thinClock = prevClock })

	sv := NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 100))
	sv.ContentHeight = 400
	sv.SetScrollbarStyle(ScrollbarThin)
	// Первый шаг только стартует анимации (часы фиксируются на нём).
	step := func(d time.Duration) {
		clk.advance(d)
		StepAnimations(clk.now)
	}
	return sv, step
}

// roundRects возвращает заливки скруглённых прямоугольников — ползунки.
func roundRects(ops []hsOp) []image.Rectangle {
	var out []image.Rectangle
	for _, op := range ops {
		if op.name == "FillRoundRect" {
			out = append(out, op.r)
		}
	}
	return out
}

// Обычный ScrollView: полоса 10 px, отнимает ширину у содержимого, всегда видна.
func TestScrollbarFixed_DefaultUnchanged(t *testing.T) {
	sv := NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 100))
	sv.ContentHeight = 400

	if got := sv.ScrollbarStyle(); got != ScrollbarFixed {
		t.Fatalf("вид по умолчанию %v", got)
	}
	if w := sv.contentWidth(); w != 190 {
		t.Errorf("ширина содержимого %d, ожидалось 190 (полоса 10 px)", w)
	}
	if tr := sv.thumbRect(); tr.Dx() != 10 || tr.Max.X != 200 {
		t.Errorf("ползунок %v", tr)
	}
	rec := &hsRec{}
	sv.Draw(rec)
	var track bool
	for _, op := range rec.ops {
		if op.name == "FillRect" && op.r == image.Rect(190, 0, 200, 100) {
			track = true
		}
	}
	if !track {
		t.Error("трек обычной полосы не нарисован")
	}
}

// Тонкая полоса ничего не отнимает у содержимого.
func TestScrollbarThin_DoesNotTakeContentWidth(t *testing.T) {
	sv, _ := newThinFixture(t)
	if w := sv.contentWidth(); w != 200 {
		t.Errorf("ширина содержимого %d, ожидалось 200 (полоса поверх)", w)
	}
}

// В покое полоса не видна и анимаций нет: простой не готовит кадры.
func TestScrollbarThin_HiddenAtRestNoAnimations(t *testing.T) {
	sv, _ := newThinFixture(t)
	rec := &hsRec{}
	sv.Draw(rec)
	if rr := roundRects(rec.ops); len(rr) != 0 {
		t.Errorf("в покое нарисованы ползунки: %v", rr)
	}
	if AnimationsActive() {
		t.Error("в покое есть активные анимации")
	}
}

// Движение мыши показывает полосу; через паузу она гаснет; в итоге анимаций нет.
func TestScrollbarThin_ShowsOnHoverAndHidesAfterPause(t *testing.T) {
	sv, step := newThinFixture(t)

	sv.OnMouseMove(50, 50)
	if !AnimationsActive() {
		t.Fatal("движение мыши не запустило появление полосы")
	}
	step(0)
	step(300 * time.Millisecond)

	rec := &hsRec{}
	sv.Draw(rec)
	rr := roundRects(rec.ops)
	if len(rr) != 1 {
		t.Fatalf("после наведения ползунков %d, ожидался 1", len(rr))
	}
	idle, _ := sv.thinWidths()
	if rr[0].Dx() != idle || rr[0].Max.X != 200 {
		t.Errorf("ползунок %v: ждали ширину %d у правого края", rr[0], idle)
	}

	// Пауза не вышла — полоса на месте.
	step(500 * time.Millisecond)
	rec = &hsRec{}
	sv.Draw(rec)
	if len(roundRects(rec.ops)) != 1 {
		t.Error("полоса погасла раньше паузы")
	}

	// Пауза вышла, плюс затухание.
	for i := 0; i < 8; i++ {
		step(300 * time.Millisecond)
	}
	rec = &hsRec{}
	sv.Draw(rec)
	if rr := roundRects(rec.ops); len(rr) != 0 {
		t.Errorf("после паузы полоса осталась: %v", rr)
	}
	if AnimationsActive() {
		t.Error("после скрытия остались анимации — движок не заснёт")
	}
}

// Движение мыши внутри паузы продлевает показ.
func TestScrollbarThin_ActivityExtendsVisibility(t *testing.T) {
	sv, step := newThinFixture(t)
	sv.OnMouseMove(50, 50)
	step(0)
	step(300 * time.Millisecond)
	for i := 0; i < 6; i++ { // 6×500 мс = 3 с > паузы 1,2 с
		step(500 * time.Millisecond)
		sv.OnMouseMove(50+i, 50)
	}
	rec := &hsRec{}
	sv.Draw(rec)
	if len(roundRects(rec.ops)) != 1 {
		t.Error("полоса погасла, хотя мышь двигалась")
	}
}

// Прокрутка колесом показывает полосу без движения мыши.
func TestScrollbarThin_ShowsOnWheel(t *testing.T) {
	sv, step := newThinFixture(t)
	sv.OnMouseButton(MouseEvent{X: 50, Y: 50, Button: MouseWheelDown, Pressed: true})
	step(0)
	step(300 * time.Millisecond)
	rec := &hsRec{}
	sv.Draw(rec)
	if len(roundRects(rec.ops)) != 1 {
		t.Error("колесо не показало полосу")
	}
	if sv.ScrollY() == 0 {
		t.Error("колесо не прокрутило")
	}
}

// Программная прокрутка тоже показывает полосу.
func TestScrollbarThin_ShowsOnProgrammaticScroll(t *testing.T) {
	sv, step := newThinFixture(t)
	sv.ScrollBy(30)
	step(0)
	step(300 * time.Millisecond)
	rec := &hsRec{}
	sv.Draw(rec)
	if len(roundRects(rec.ops)) != 1 {
		t.Error("ScrollBy не показал полосу")
	}
}

// Содержимое, которое помещается, полосу не показывает и анимаций не заводит.
func TestScrollbarThin_NothingToScroll(t *testing.T) {
	sv, step := newThinFixture(t)
	sv.ContentHeight = 50
	sv.OnMouseMove(50, 50)
	step(0)
	step(300 * time.Millisecond)
	if AnimationsActive() {
		t.Error("анимации при отсутствии прокрутки")
	}
	rec := &hsRec{}
	sv.Draw(rec)
	if len(roundRects(rec.ops)) != 0 {
		t.Error("полоса нарисована без прокрутки")
	}
}

// Курсор на самой полосе расширяет её и не даёт погаснуть; уход — гасит.
func TestScrollbarThin_HoldOnBar(t *testing.T) {
	sv, step := newThinFixture(t)
	sv.OnMouseMove(198, 10) // над правой колонкой
	step(0)
	step(400 * time.Millisecond)

	_, hover := sv.thinWidths()
	rec := &hsRec{}
	sv.Draw(rec)
	rr := roundRects(rec.ops)
	if len(rr) != 1 || rr[0].Dx() != hover {
		t.Fatalf("под курсором ожидалась ширина %d, получено %v", hover, rr)
	}
	// Долго стоим на полосе: не гаснет.
	for i := 0; i < 10; i++ {
		step(500 * time.Millisecond)
	}
	rec = &hsRec{}
	sv.Draw(rec)
	if len(roundRects(rec.ops)) != 1 {
		t.Fatal("полоса погасла под курсором")
	}
	// Уходим с полосы (но остаёмся над областью): сужается, затем гаснет.
	sv.OnMouseMove(50, 50)
	for i := 0; i < 12; i++ {
		step(300 * time.Millisecond)
	}
	rec = &hsRec{}
	sv.Draw(rec)
	if len(roundRects(rec.ops)) != 0 {
		t.Error("полоса не погасла после ухода курсора")
	}
}

// Невидимая полоса не перехватывает нажатия у содержимого.
func TestScrollbarThin_HiddenDoesNotCapture(t *testing.T) {
	sv, step := newThinFixture(t)
	press := MouseEvent{X: 198, Y: 5, Button: MouseLeft, Pressed: true}
	if sv.WantsCapture(press) {
		t.Error("скрытая полоса перехватывает мышь")
	}
	sv.OnMouseMove(198, 5)
	step(0)
	step(400 * time.Millisecond)
	if !sv.WantsCapture(press) {
		t.Error("видимая полоса не перехватывает мышь на ползунке")
	}
}

// Ползунок тонкой полосы тянется мышью.
func TestScrollbarThin_DragThumb(t *testing.T) {
	sv, step := newThinFixture(t)
	sv.OnMouseMove(198, 5)
	step(0)
	step(400 * time.Millisecond)

	if !sv.OnMouseButton(MouseEvent{X: 198, Y: 5, Button: MouseLeft, Pressed: true}) {
		t.Fatal("нажатие на ползунок не принято")
	}
	sv.OnMouseMove(198, 40)
	if sv.ScrollY() <= 0 {
		t.Errorf("перетаскивание не прокрутило: %d", sv.ScrollY())
	}
	sv.OnMouseButton(MouseEvent{X: 198, Y: 40, Button: MouseLeft, Pressed: false})
}

// Тема включает тонкую полосу, меняет ширину и возвращает обратно; явный выбор
// экземпляра главнее темы.
func TestScrollbarThin_ThemeDrivesStyleAndWidth(t *testing.T) {
	thin := &Theme{}
	thin.Style.ScrollbarThin = true
	thin.Style.ScrollbarThinWidth = 3
	thin.Style.ScrollbarThinHoverWidth = 9

	sv := NewScrollView()
	sv.ApplyTheme(thin)
	if sv.ScrollbarStyle() != ScrollbarThin {
		t.Fatal("тема не включила тонкую полосу")
	}
	if i, h := sv.thinWidths(); i != 3 || h != 9 {
		t.Errorf("ширины из темы %d/%d, ожидалось 3/9", i, h)
	}
	sv.ApplyTheme(&Theme{}) // обычная тема — назад
	if sv.ScrollbarStyle() != ScrollbarFixed {
		t.Error("смена темы не вернула обычную полосу")
	}

	wide := &Theme{}
	wide.Style.ScrollbarWidth = 16
	sv.ApplyTheme(wide)
	if sv.scrollbarWidth != 16 {
		t.Errorf("ширина из темы %d, ожидалось 16", sv.scrollbarWidth)
	}
	sv.ApplyTheme(&Theme{})
	if sv.scrollbarWidth != 10 {
		t.Errorf("после смены темы ширина %d, ожидалась прежняя 10", sv.scrollbarWidth)
	}

	// Явный выбор главнее темы.
	sv2 := NewScrollView()
	sv2.SetScrollbarStyle(ScrollbarFixed)
	sv2.SetScrollbarWidth(12)
	sv2.ApplyTheme(thin)
	if sv2.ScrollbarStyle() != ScrollbarFixed || sv2.scrollbarWidth != 12 {
		t.Errorf("тема перебила явные настройки: вид %v, ширина %d", sv2.ScrollbarStyle(), sv2.scrollbarWidth)
	}
}

// Цвета тонкой полосы по умолчанию — цвета обычной (темы); явные главнее.
func TestScrollbarThin_ColorsDefaultToThemeColors(t *testing.T) {
	sv := NewScrollView()
	thumb, hover, track := sv.thinColors()
	if thumb != sv.ThumbColor || hover != sv.ThumbHoverBG || track != sv.TrackColor {
		t.Errorf("цвета по умолчанию не следуют теме: %v %v %v", thumb, hover, track)
	}
	custom := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	sv.SetThinColors(custom, color.RGBA{}, color.RGBA{})
	thumb, hover, _ = sv.thinColors()
	if thumb != custom || hover != sv.ThumbHoverBG {
		t.Errorf("явные цвета: %v %v", thumb, hover)
	}
}

// Перерисовка при показе и затухании ограничена колонкой полосы, а не всей
// прокруткой.
func TestScrollbarThin_InvalidatesOnlyBarStrip(t *testing.T) {
	sv, step := newThinFixture(t)

	var rects []image.Rectangle
	SetUIRectChangeNotifier(func(r image.Rectangle) { rects = append(rects, r) })
	defer SetUIRectChangeNotifier(nil)
	full := 0
	SetUIChangeNotifier(func() { full++ })
	defer SetUIChangeNotifier(nil)

	sv.OnMouseMove(50, 50)
	step(0)
	step(100 * time.Millisecond)
	step(400 * time.Millisecond)

	if len(rects) == 0 {
		t.Fatal("появление полосы не заявило перерисовки")
	}
	_, hover := sv.thinWidths()
	for _, r := range rects {
		if r.Dx() > hover {
			t.Errorf("заявлена область шире колонки полосы: %v", r)
		}
	}
	if full != 0 {
		t.Errorf("полная инвалидация %d раз — анимация полосы будит весь кадр", full)
	}
}
