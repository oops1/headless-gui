package desktop

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Появление и исчезновение всплывающих панелей по анимации темы.

// motionFlyout — панель без содержимого, привязанная к значку внизу экрана.
func motionFlyout(tm *theme.Manager) *Flyout {
	f := NewFlyout(tm, ComponentStartMenu)
	f.Size = func() image.Point { return image.Pt(200, 300) }
	f.Screen = image.Rect(0, 0, 800, 600)
	return f
}

var motionAnchor = image.Rect(0, 560, 48, 600)

// runAnimations прокручивает часы анимаций: старт на t0, затем шаг на d.
func runAnimations(t0 time.Time, d time.Duration) {
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(d))
}

// finishAnimations доводит все анимации до конца: первый шаг запускает часы,
// второй — далеко в будущем.
func finishAnimations() {
	t := time.Now()
	widget.StepAnimations(t)
	widget.StepAnimations(t.Add(time.Hour))
}

func TestFlyout_OpensByAnimationToken(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)
	f := motionFlyout(tm)

	f.Open(motionAnchor)
	if !f.IsOpen() {
		t.Fatal("панель не открыта")
	}
	if f.Presence() != 0 {
		t.Fatalf("присутствие до первого шага = %v, ждали 0: панель не должна появляться разом", f.Presence())
	}
	rest := f.restRect()
	if rest.Empty() {
		t.Fatal("панель без места в покое")
	}

	t0 := time.Now()
	runAnimations(t0, 60*time.Millisecond)
	p := f.Presence()
	if p <= 0 || p >= 1 {
		t.Fatalf("посреди анимации присутствие = %v, ждали между 0 и 1", p)
	}
	// «Пуск» выезжает снизу: пока не доехал, он ниже места в покое.
	cur := f.rect()
	if cur.Min.Y <= rest.Min.Y || cur.Min.X != rest.Min.X {
		t.Errorf("посреди анимации панель %v, ждали сдвиг вниз от %v", cur, rest)
	}

	widget.StepAnimations(t0.Add(time.Second))
	if f.Presence() != 1 || f.rect() != rest {
		t.Errorf("после анимации присутствие %v, панель %v, ждали 1 и %v", f.Presence(), f.rect(), rest)
	}
	if f.IsAnimating() {
		t.Error("анимация закончилась, а IsAnimating истинно")
	}
}

func TestFlyout_ClosingStaysDrawnUntilTheEnd(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)
	f := motionFlyout(tm)
	f.Open(motionAnchor)
	finishAnimations()

	closed := 0
	f.OnClose = func() { closed++ }
	f.Close()
	if f.IsOpen() {
		t.Fatal("после Close панель всё ещё открыта: клики и Esc не должны её видеть")
	}
	if closed != 1 {
		t.Errorf("OnClose вызван %d раз, ждали 1 сразу при закрытии", closed)
	}
	if !f.HasOverlay() {
		t.Error("закрывающаяся панель пропала сразу — анимации закрытия не видно")
	}
	if f.OverlayBounds() != f.restRect() {
		t.Errorf("окно-носитель закрывающейся панели %v, ждали место в покое %v", f.OverlayBounds(), f.restRect())
	}
	if f.Bounds().In(f.restRect()) && f.Bounds().Dx() > 0 && !f.Base.Bounds().Empty() {
		t.Error("закрытая панель не должна ловить мышь")
	}

	t0 := time.Now()
	runAnimations(t0, 60*time.Millisecond)
	if p := f.Presence(); p <= 0 || p >= 1 {
		t.Fatalf("посреди закрытия присутствие = %v", p)
	}
	widget.StepAnimations(t0.Add(time.Second))
	if f.HasOverlay() || f.Presence() != 0 {
		t.Errorf("после закрытия оверлей остался: presence=%v", f.Presence())
	}
}

func TestFlyout_ReopenDuringCloseContinuesFromWhereItWas(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)
	f := motionFlyout(tm)
	f.Open(motionAnchor)
	finishAnimations()

	f.Close()
	t0 := time.Now()
	runAnimations(t0, 60*time.Millisecond)
	mid := f.Presence()
	if mid <= 0 || mid >= 1 {
		t.Fatalf("присутствие посреди закрытия = %v", mid)
	}

	f.Open(motionAnchor)
	if got := f.Presence(); got != mid {
		t.Errorf("переоткрытие сбросило присутствие %v -> %v: панель прыгнула", mid, got)
	}
	t1 := t0.Add(80 * time.Millisecond)
	widget.StepAnimations(t1)
	widget.StepAnimations(t1.Add(20 * time.Millisecond))
	if f.Presence() < mid {
		t.Errorf("после переоткрытия присутствие упало: %v < %v", f.Presence(), mid)
	}
	widget.StepAnimations(t1.Add(time.Second))
	if f.Presence() != 1 {
		t.Errorf("переоткрытая панель не доехала: %v", f.Presence())
	}
}

func TestFlyout_ZeroDurationIsInstant(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows2000)
	f := motionFlyout(tm)

	f.Open(motionAnchor)
	if f.Presence() != 1 || f.rect() != f.restRect() {
		t.Errorf("в Windows 2000 панель открывается не разом: presence=%v", f.Presence())
	}
	if widget.AnimationsActive() {
		t.Error("нулевая длительность запустила анимацию")
	}
	f.Close()
	if f.HasOverlay() || f.Presence() != 0 {
		t.Error("в Windows 2000 панель закрывается не разом")
	}
}

func TestFlyout_SlideDirections(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)

	cases := []struct {
		name  string
		slide SlideFrom
		edge  Edge
		want  image.Point // знак сдвига
	}{
		{"auto снизу", SlideAuto, EdgeBottom, image.Pt(0, 1)},
		{"auto сверху", SlideAuto, EdgeTop, image.Pt(0, -1)},
		{"справа", SlideRight, EdgeBottom, image.Pt(1, 0)},
		{"слева", SlideLeft, EdgeBottom, image.Pt(-1, 0)},
		{"без сдвига", SlideNone, EdgeBottom, image.Pt(0, 0)},
	}
	for _, c := range cases {
		f := motionFlyout(tm)
		f.Slide, f.Edge = c.slide, c.edge
		f.Open(motionAnchor)
		runAnimations(time.Now(), 60*time.Millisecond)
		off := f.rect().Min.Sub(f.restRect().Min)
		got := image.Pt(sign(off.X), sign(off.Y))
		if got != c.want {
			t.Errorf("%s: сдвиг %v, ждали знаки %v", c.name, off, c.want)
		}
		f.Settle()
		widget.StopAllAnimations()
	}
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func TestFlyout_SlideToScreenEdge(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)
	f := motionFlyout(tm)
	f.Slide, f.SlideDistance = SlideRight, -1
	f.Open(motionAnchor)
	// В самом начале панель целиком за краем экрана.
	if f.rect().Min.X < f.Screen.Max.X {
		t.Errorf("в начале анимации панель %v ещё на экране %v", f.rect(), f.Screen)
	}
	region := f.dirtyRect()
	if region.Max.X != f.Screen.Max.X {
		t.Errorf("область движения %v не доходит до края экрана", region)
	}
}

func TestFlyout_ThemeDistanceMetricOverrides(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)
	f := motionFlyout(tm)
	f.SlideDistance = 100
	f.Open(motionAnchor)
	// p=0: сдвиг равен заданному расстоянию.
	if off := f.rect().Min.Y - f.restRect().Min.Y; off != 100 {
		t.Errorf("начальный сдвиг %d, ждали 100", off)
	}
}

// ─── Перерисовывается только область панели ────────────────────────────────

func TestFlyout_AnimationInvalidatesOnlyItsArea(t *testing.T) {
	defer widget.StopAllAnimations()
	const w, h = 900, 600

	tm := managerFor(t, theme.ProfileWindows10)

	root := widget.NewPanel(theme.RGB(20, 40, 80))
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	// Далеко от панели: ни тень, ни путь выезда сюда не доходят.
	far := &countingItem{Item: newWallPanel(image.Rect(0, 0, w, 48))}
	// Под панелью: перерисовываться должна.
	near := &countingItem{Item: newWallPanel(image.Rect(0, 200, w, 520))}
	root.AddChild(far)
	root.AddChild(near)

	f := NewFlyout(tm, ComponentStartMenu)
	f.Size = func() image.Point { return image.Pt(300, 400) }
	f.Screen = image.Rect(0, 0, w, h)
	// Собственные границы панели в дереве — значок на панели задач, от которого
	// она всплывает; всё содержимое живёт в оверлее.
	f.SetBounds(image.Rect(0, h-40, 48, h))
	root.AddChild(f)

	eng := engine.New(w, h, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()
	eng.RenderOnce()
	farBefore, nearBefore := far.draws, near.draws

	f.Open(image.Rect(0, h-40, 48, h))
	eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimations(t0)
	eng.RenderOnce()
	widget.StepAnimations(t0.Add(70 * time.Millisecond))
	eng.RenderOnce()
	widget.StepAnimations(t0.Add(time.Second))
	eng.RenderOnce()

	if far.draws != farBefore {
		t.Errorf("далёкая область перерисована %d раз: анимация панели будит кадр целиком",
			far.draws-farBefore)
	}
	if near.draws == nearBefore {
		t.Error("область под панелью не перерисована — панель на экране не появилась")
	}

	// И закрытие тоже.
	farBefore = far.draws
	f.Close()
	t1 := t0.Add(2 * time.Second)
	widget.StepAnimations(t1)
	eng.RenderOnce()
	widget.StepAnimations(t1.Add(time.Second))
	eng.RenderOnce()
	if far.draws != farBefore {
		t.Errorf("закрытие панели перерисовало далёкую область %d раз", far.draws-farBefore)
	}
}

// pixelAt берёт пиксель кадра.
func pixelAt(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }

func TestFlyout_FadesIn(t *testing.T) {
	defer widget.StopAllAnimations()
	const w, h = 600, 400

	tm := managerFor(t, theme.ProfileWindows10)
	root := widget.NewPanel(theme.RGB(200, 30, 30))
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	f := NewFlyout(tm, ComponentStartMenu)
	f.Size = func() image.Point { return image.Pt(300, 200) }
	f.Screen = image.Rect(0, 0, w, h)
	root.AddChild(f)

	eng := engine.New(w, h, 60)
	eng.SetRoot(root)
	bg := pixelAt(eng.RenderOnce(), 150, 100)

	f.Open(image.Rect(0, h-40, 48, h))
	t0 := time.Now()
	widget.StepAnimations(t0)
	start := pixelAt(eng.RenderOnce(), f.restRect().Min.X+150, f.restRect().Min.Y+120)
	if start != bg {
		t.Errorf("в начале анимации панель уже видна: %v, фон %v", start, bg)
	}

	widget.StepAnimations(t0.Add(time.Second))
	end := pixelAt(eng.RenderOnce(), f.restRect().Min.X+150, f.restRect().Min.Y+120)
	if end == bg {
		t.Fatal("после анимации панель не видна — тест не различает состояния")
	}

	// Посередине — смесь: не фон и не готовая панель.
	g := motionFlyout(tm)
	_ = g
	f.Close()
	t1 := t0.Add(2 * time.Second)
	widget.StepAnimations(t1)
	widget.StepAnimations(t1.Add(50 * time.Millisecond))
	mid := pixelAt(eng.RenderOnce(), f.restRect().Min.X+150, f.restRect().Min.Y+120)
	if mid == bg || mid == end {
		t.Errorf("посреди исчезновения пиксель %v — это фон %v или готовая панель %v, а не смесь", mid, bg, end)
	}
}
