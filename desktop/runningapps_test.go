package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// rectIn сообщает, целиком ли r лежит внутри bound (используется для
// проверки, что ни одна кнопка не вышла за границы области при деградации).
func rectIn(r, bound image.Rectangle) bool {
	return r == r.Intersect(bound)
}

func TestRunningApplications_ClickInactiveActivates(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(
		WindowInfo{ID: 1, Title: "Один", Active: true},
		WindowInfo{ID: 2, Title: "Два", Active: false},
	)
	ra := NewRunningApplications(tm, wm)
	ra.SetBounds(image.Rect(0, 0, 500, 32))

	if len(ra.btns) != 2 {
		t.Fatalf("ожидали 2 кнопки, получили %d", len(ra.btns))
	}
	target := ra.btns[1].rect
	px, py := target.Min.X+5, target.Min.Y+5

	if !ra.OnMouseButton(widget.MouseEvent{X: px, Y: py, Button: widget.MouseLeft, Pressed: true}) {
		t.Fatalf("press по кнопке окна должен быть поглощён")
	}
	if !ra.OnMouseButton(widget.MouseEvent{X: px, Y: py, Button: widget.MouseLeft, Pressed: false}) {
		t.Fatalf("release по кнопке окна должен быть поглощён")
	}
	if len(wm.Activated) != 1 || wm.Activated[0] != 2 {
		t.Fatalf("клик по неактивному окну должен вызвать Activate(2), журнал: %v", wm.Activated)
	}
	if len(wm.Minimized) != 0 {
		t.Fatalf("клик по неактивному окну не должен сворачивать: %v", wm.Minimized)
	}
}

func TestRunningApplications_ClickActiveMinimizes(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(
		WindowInfo{ID: 1, Title: "Один", Active: true},
	)
	ra := NewRunningApplications(tm, wm)
	ra.SetBounds(image.Rect(0, 0, 500, 32))

	target := ra.btns[0].rect
	px, py := target.Min.X+5, target.Min.Y+5

	ra.OnMouseButton(widget.MouseEvent{X: px, Y: py, Button: widget.MouseLeft, Pressed: true})
	ra.OnMouseButton(widget.MouseEvent{X: px, Y: py, Button: widget.MouseLeft, Pressed: false})

	if len(wm.Minimized) != 1 || wm.Minimized[0] != 1 {
		t.Fatalf("клик по активному окну должен вызвать Minimize(1), журнал: %v", wm.Minimized)
	}
	if len(wm.Activated) != 0 {
		t.Fatalf("клик по активному окну не должен активировать: %v", wm.Activated)
	}
}

func TestRunningApplications_MiddleClickCloses(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(
		WindowInfo{ID: 7, Title: "Семь", Active: false},
	)
	ra := NewRunningApplications(tm, wm)
	ra.SetBounds(image.Rect(0, 0, 500, 32))

	target := ra.btns[0].rect
	px, py := target.Min.X+5, target.Min.Y+5

	if !ra.OnMouseButton(widget.MouseEvent{X: px, Y: py, Button: widget.MouseMiddle, Pressed: true}) {
		t.Fatalf("средний клик по кнопке окна должен быть поглощён")
	}
	if len(wm.Closed) != 1 || wm.Closed[0] != 7 {
		t.Fatalf("средний клик должен вызвать Close(7), журнал: %v", wm.Closed)
	}
}

func TestRunningApplications_ReleaseAwayDoesNotAct(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(WindowInfo{ID: 1, Title: "Один", Active: false})
	ra := NewRunningApplications(tm, wm)
	ra.SetBounds(image.Rect(0, 0, 500, 32))

	target := ra.btns[0].rect
	px, py := target.Min.X+5, target.Min.Y+5

	ra.OnMouseButton(widget.MouseEvent{X: px, Y: py, Button: widget.MouseLeft, Pressed: true})
	// Отпускание там, где кнопок нет вовсе.
	ra.OnMouseButton(widget.MouseEvent{X: 100000, Y: 100000, Button: widget.MouseLeft, Pressed: false})

	if len(wm.Activated) != 0 {
		t.Fatalf("release в стороне не должен активировать окно: %v", wm.Activated)
	}
}

func TestRunningApplications_WindowListChangeRelayouts(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(WindowInfo{ID: 1, Title: "Один"})
	ra := NewRunningApplications(tm, wm)
	ra.SetBounds(image.Rect(0, 0, 500, 32))

	if len(ra.btns) != 1 {
		t.Fatalf("ожидали 1 кнопку до изменения списка, получили %d", len(ra.btns))
	}

	wm.SetWindows([]WindowInfo{
		{ID: 1, Title: "Один"},
		{ID: 2, Title: "Два"},
		{ID: 3, Title: "Три"},
	})

	if len(ra.btns) != 3 {
		t.Fatalf("подписка на WindowModel не пересчитала кнопки: ожидали 3, получили %d", len(ra.btns))
	}
}

func TestRunningApplications_Close_Unsubscribes(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(WindowInfo{ID: 1, Title: "Один"})
	ra := NewRunningApplications(tm, wm)

	if len(wm.subs) != 1 {
		t.Fatalf("после создания компонент должен быть подписан ровно один раз, подписок: %d", len(wm.subs))
	}
	ra.Close()
	if len(wm.subs) != 0 {
		t.Fatalf("Close должен снять подписку, подписок осталось: %d", len(wm.subs))
	}

	// Повторный Close не должен паниковать.
	ra.Close()
}

// ─── Деградация при нехватке места ──────────────────────────────────────────

func manyWindows(n int) []WindowInfo {
	ws := make([]WindowInfo, n)
	for i := range ws {
		ws[i] = WindowInfo{
			ID:    WindowID(i + 1),
			Title: "Окно с достаточно длинным заголовком, чтобы точно не влезть",
		}
	}
	return ws
}

func TestRunningApplications_Degrade_Ideal(t *testing.T) {
	tm := buildTestTheme() // ideal=150, min=70, icon=16+2*4=24, gap=4
	wm := NewFakeWindowModel(manyWindows(3)...)
	ra := NewRunningApplications(tm, wm)
	bounds := image.Rect(0, 0, 800, 32) // 3*150+2*4=458 <= 800: помещаются идеально
	ra.SetBounds(bounds)

	if len(ra.btns) != 3 {
		t.Fatalf("ожидали 3 кнопки, получили %d", len(ra.btns))
	}
	for i, wb := range ra.btns {
		if !wb.showLabel {
			t.Fatalf("кнопка %d: при избытке места подпись должна быть видна", i)
		}
		if wb.rect.Dx() != 150 {
			t.Fatalf("кнопка %d: ширина = %d, ожидали идеальную 150", i, wb.rect.Dx())
		}
		if !rectIn(wb.rect, bounds) {
			t.Fatalf("кнопка %d вышла за границы области: %v не внутри %v", i, wb.rect, bounds)
		}
	}
}

func TestRunningApplications_Degrade_ShrinkWithLabel(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(manyWindows(5)...)
	ra := NewRunningApplications(tm, wm)
	// perButton = (500-4*4)/5 = 96.8 → 96: между min(70) и ideal(150).
	bounds := image.Rect(0, 0, 500, 32)
	ra.SetBounds(bounds)

	if len(ra.btns) != 5 {
		t.Fatalf("ожидали 5 кнопок, получили %d", len(ra.btns))
	}
	for i, wb := range ra.btns {
		if !wb.showLabel {
			t.Fatalf("кнопка %d: на этой ширине подпись ещё должна показываться", i)
		}
		if wb.rect.Dx() >= 150 || wb.rect.Dx() < 70 {
			t.Fatalf("кнопка %d: ширина %d вне ожидаемого диапазона [70,150)", i, wb.rect.Dx())
		}
		if !rectIn(wb.rect, bounds) {
			t.Fatalf("кнопка %d вышла за границы области: %v не внутри %v", i, wb.rect, bounds)
		}
	}
}

func TestRunningApplications_Degrade_IconOnly(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(manyWindows(5)...)
	ra := NewRunningApplications(tm, wm)
	// perButton = (200-16)/5 = 36.8 → 36: меньше min(70), но >= icon-only(24).
	bounds := image.Rect(0, 0, 200, 32)
	ra.SetBounds(bounds)

	if len(ra.btns) != 5 {
		t.Fatalf("ожидали все 5 кнопок (места хватает под значки), получили %d", len(ra.btns))
	}
	for i, wb := range ra.btns {
		if wb.showLabel {
			t.Fatalf("кнопка %d: на этой ширине подпись уже должна прятаться", i)
		}
		if !rectIn(wb.rect, bounds) {
			t.Fatalf("кнопка %d вышла за границы области: %v не внутри %v", i, wb.rect, bounds)
		}
	}
}

func TestRunningApplications_Degrade_OverflowHidesExtraButtons(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(manyWindows(10)...)
	ra := NewRunningApplications(tm, wm)
	// Даже по одному значку (24px + 4px зазор) в ряд все 10 не влезают.
	bounds := image.Rect(0, 0, 100, 32)
	ra.SetBounds(bounds)

	if len(ra.btns) == 0 {
		t.Fatalf("ожидали хотя бы одну кнопку")
	}
	if len(ra.btns) >= 10 {
		t.Fatalf("лишние кнопки должны быть просто не нарисованы, получили все %d", len(ra.btns))
	}
	for i, wb := range ra.btns {
		if !rectIn(wb.rect, bounds) {
			t.Fatalf("кнопка %d вышла за границы области: %v не внутри %v", i, wb.rect, bounds)
		}
	}
}

func TestRunningApplications_Draw_NoOverflowingButtons(t *testing.T) {
	tm := buildTestTheme()
	wm := NewFakeWindowModel(manyWindows(7)...)
	ra := NewRunningApplications(tm, wm)
	// Ширины достаточно для идеальных кнопок с подписью (150*7+4*6=1074) —
	// значит DrawTextLeft внутри Draw реально выставляет и снимает клип,
	// и мы проверяем, что он возвращает его в исходное состояние.
	bounds := image.Rect(10, 10, 1110, 42)
	ra.SetBounds(bounds)
	if len(ra.btns) == 0 || !ra.btns[0].showLabel {
		t.Fatalf("тест предполагает раскладку с подписью, получили btns=%v", ra.btns)
	}

	ctx := newItemRecCtx()
	ra.Draw(ctx) // не должно паниковать и не должно "потерять" клип

	for i, wb := range ra.btns {
		if !rectIn(wb.rect, bounds) {
			t.Fatalf("кнопка %d вышла за границы области: %v не внутри %v", i, wb.rect, bounds)
		}
	}
	if len(ctx.texts) == 0 {
		t.Fatalf("ожидали отрисовку хотя бы одной подписи")
	}
	if ctx.clip != image.Rect(0, 0, 1<<15, 1<<15) {
		t.Fatalf("Draw должен восстановить внешний клип после себя, получили %v", ctx.clip)
	}
}
