package desktop

import (
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Плавные переходы цвета и плавное число.

func buttonStyle(tm *theme.Manager) func(theme.State) *theme.Style {
	return func(st theme.State) *theme.Style { return tm.GetStyle(ComponentTaskButton, "", st) }
}

// between сообщает, что v лежит строго между a и b (в любом порядке).
func between(v, a, b uint8) bool {
	if a > b {
		a, b = b, a
	}
	return v > a && v < b
}

func TestMotion_HoverFadesBetweenStyles(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows11)
	get := buttonStyle(tm)
	normal, hover := get(theme.StateNormal), get(theme.StateHover)
	if normal.Fill == hover.Fill {
		t.Skip("в теме у наведения та же заливка — переходить не между чем")
	}

	var m motion
	r := image.Rect(10, 10, 50, 50)

	if got := m.Style(tm, 1, r, theme.StateNormal, get); got.Fill != normal.Fill {
		t.Fatalf("первое появление ячейки должно рисоваться сразу: %v", got.Fill)
	}
	// Состояние сменилось: переход начинается с прежнего цвета, а не со скачка.
	if got := m.Style(tm, 1, r, theme.StateHover, get); got.Fill != normal.Fill {
		t.Errorf("в начале перехода заливка %v, ждали прежнюю %v", got.Fill, normal.Fill)
	}

	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(40 * time.Millisecond))
	mid := m.Style(tm, 1, r, theme.StateHover, get)
	if mid.Fill == normal.Fill || mid.Fill == hover.Fill {
		t.Fatalf("посреди перехода заливка %v — это один из краёв (%v, %v), а не смесь", mid.Fill, normal.Fill, hover.Fill)
	}
	// Смесь по каждому различающемуся каналу — строго между краями.
	for _, ch := range []struct {
		name    string
		v, a, b uint8
		differ  bool
	}{
		{"A", mid.Fill.A, normal.Fill.A, hover.Fill.A, normal.Fill.A != hover.Fill.A},
		{"R", mid.Fill.R, normal.Fill.R, hover.Fill.R, normal.Fill.R != hover.Fill.R},
		{"G", mid.Fill.G, normal.Fill.G, hover.Fill.G, normal.Fill.G != hover.Fill.G},
	} {
		if ch.differ && !between(ch.v, ch.a, ch.b) {
			t.Errorf("канал %s: %d не между %d и %d", ch.name, ch.v, ch.a, ch.b)
		}
	}

	// Курсор ушёл посреди перехода: возврат идёт от того цвета, что на экране.
	back := m.Style(tm, 1, r, theme.StateNormal, get)
	if back.Fill != mid.Fill {
		t.Errorf("оборванный переход прыгнул: было %v, стало %v", mid.Fill, back.Fill)
	}

	// Дальше — до конца.
	t1 := t0.Add(time.Second)
	widget.StepAnimations(t1)
	widget.StepAnimations(t1.Add(time.Second))
	if end := m.Style(tm, 1, r, theme.StateNormal, get); end.Fill != normal.Fill {
		t.Errorf("после перехода заливка %v, ждали %v", end.Fill, normal.Fill)
	}
}

func TestMotion_ZeroDurationIsInstant(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows2000)
	get := buttonStyle(tm)
	hover := get(theme.StateHover)

	var m motion
	r := image.Rect(0, 0, 30, 30)
	m.Style(tm, 1, r, theme.StateNormal, get)
	if got := m.Style(tm, 1, r, theme.StateHover, get); got.Fill != hover.Fill || got.Text != hover.Text {
		t.Errorf("в Windows 2000 наведение должно переключать стиль сразу")
	}
	if widget.AnimationsActive() {
		t.Error("нулевая длительность запустила анимацию")
	}
}

func TestMotion_CellsAreIndependent(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows11)
	get := buttonStyle(tm)
	normal, hover := get(theme.StateNormal), get(theme.StateHover)
	if normal.Fill == hover.Fill {
		t.Skip("заливки совпадают")
	}

	var m motion
	a, b := image.Rect(0, 0, 20, 20), image.Rect(20, 0, 40, 20)
	m.Style(tm, "a", a, theme.StateNormal, get)
	m.Style(tm, "b", b, theme.StateNormal, get)
	m.Style(tm, "a", a, theme.StateHover, get)

	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(time.Second))
	if got := m.Style(tm, "b", b, theme.StateNormal, get); got.Fill != normal.Fill {
		t.Errorf("переход соседней ячейки задел ячейку b: %v", got.Fill)
	}
	if got := m.Style(tm, "a", a, theme.StateHover, get); got.Fill != hover.Fill {
		t.Errorf("ячейка a не дошла до наведения: %v", got.Fill)
	}
}

func TestMotion_StepsInvalidateOnlyTheCell(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows11)
	get := buttonStyle(tm)

	var (
		mu    sync.Mutex
		rects []image.Rectangle
		full  int
	)
	widget.SetUIRectChangeNotifier(func(r image.Rectangle) { mu.Lock(); rects = append(rects, r); mu.Unlock() })
	widget.SetUIChangeNotifier(func() { mu.Lock(); full++; mu.Unlock() })
	defer widget.SetUIRectChangeNotifier(nil)
	defer widget.SetUIChangeNotifier(nil)

	var m motion
	r := image.Rect(100, 20, 140, 60)
	m.Style(tm, 1, r, theme.StateNormal, get)
	m.Style(tm, 1, r, theme.StateHover, get)
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(50 * time.Millisecond))

	mu.Lock()
	defer mu.Unlock()
	if len(rects) == 0 {
		t.Fatal("шаги перехода ничего не заявили — кнопка на экране не изменится")
	}
	for _, got := range rects {
		if got != r {
			t.Errorf("шаг перехода заявил %v, ждали только область ячейки %v", got, r)
		}
	}
}

// ─── Кнопка на экране ───────────────────────────────────────────────────────

func TestMotion_StartButtonFadesOnScreen(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows11)
	tm.SetIconResolver(widget.BuiltinIcons())

	const w, h = 120, 60
	root := widget.NewPanel(color.RGBA{R: 40, G: 40, B: 40, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	btn := NewStartButton(tm)
	btn.SetBounds(image.Rect(10, 10, 50, 50))
	root.AddChild(btn)

	eng := engine.New(w, h, 60)
	eng.SetRoot(root)
	probe := image.Pt(12, 12) // угол кнопки: значок сюда не залезает
	rest := eng.RenderOnce().RGBAAt(probe.X, probe.Y)

	btn.OnMouseMove(30, 30)
	start := eng.RenderOnce().RGBAAt(probe.X, probe.Y)
	if start != rest {
		t.Errorf("сразу после наведения цвет уже %v, ждали прежний %v", start, rest)
	}
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(time.Second))
	end := eng.RenderOnce().RGBAAt(probe.X, probe.Y)
	if end == rest {
		t.Skip("наведение в теме не меняет угол кнопки")
	}

	btn.OnMouseMove(5, 5) // курсор ушёл
	// Смену состояния кнопка замечает при отрисовке — как и в цикле движка,
	// где шаг анимаций идёт после кадра, заведшего переход.
	eng.RenderOnce()
	t1 := t0.Add(2 * time.Second)
	widget.StepAnimations(t1)
	widget.StepAnimations(t1.Add(40 * time.Millisecond))
	mid := eng.RenderOnce().RGBAAt(probe.X, probe.Y)
	if mid == end || mid == rest {
		t.Errorf("посреди ухода пиксель %v — это край (%v, %v), а не смесь", mid, end, rest)
	}
}

// ─── Tween ──────────────────────────────────────────────────────────────────

func TestTween_MovesByThemeAnimation(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows11)

	changes := 0
	tw := NewTween(tm, AnimMenuOpen, 48, func() { changes++ })
	if tw.Value() != 48 || tw.Animating() {
		t.Fatalf("начальное состояние: %v, идёт=%v", tw.Value(), tw.Animating())
	}
	tw.To(256)
	if tw.Target() != 256 || !tw.Animating() {
		t.Fatalf("цель %v, идёт=%v", tw.Target(), tw.Animating())
	}
	if tw.Value() != 48 {
		t.Errorf("значение прыгнуло сразу: %v", tw.Value())
	}

	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(60 * time.Millisecond))
	if v := tw.Value(); v <= 48 || v >= 256 {
		t.Errorf("посреди перехода %v, ждали между 48 и 256", v)
	}
	if changes == 0 {
		t.Error("onChange не звали — область не перерисуется")
	}

	// Цель сменилась посреди пути: идём назад от текущего значения.
	cur := tw.Value()
	tw.To(48)
	t1 := t0.Add(time.Second)
	widget.StepAnimations(t1)
	widget.StepAnimations(t1.Add(10 * time.Millisecond))
	if v := tw.Value(); v > cur {
		t.Errorf("обратный ход начался с прыжка: %v > %v", v, cur)
	}
	widget.StepAnimations(t1.Add(time.Second))
	if tw.Value() != 48 || tw.Animating() {
		t.Errorf("вернулось не в 48: %v, идёт=%v", tw.Value(), tw.Animating())
	}
}

func TestTween_ZeroDurationIsInstant(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows2000)
	changes := 0
	tw := NewTween(tm, AnimMenuOpen, 48, func() { changes++ })
	tw.To(256)
	if tw.Value() != 256 || tw.Animating() || changes != 1 {
		t.Errorf("значение %v, идёт=%v, onChange %d раз — ждали мгновенный переход", tw.Value(), tw.Animating(), changes)
	}
	if widget.AnimationsActive() {
		t.Error("нулевая длительность запустила анимацию")
	}
}

func TestTween_SetStopsAnimation(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows11)
	tw := NewTween(tm, AnimMenuOpen, 0, nil)
	tw.To(100)
	tw.Set(7)
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(time.Second))
	if tw.Value() != 7 || tw.Animating() {
		t.Errorf("Set не остановил переход: %v", tw.Value())
	}
}
