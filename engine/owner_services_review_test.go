package engine

// owner_services_review_test.go — случаи из ревью 5.2 (службы владельца): первая
// раскладка до привязки, меню через хозяина, анимации незапущенного и
// остановленного владельца, кэши раскладки при смене владельца, шрифты и
// ревизия метрик, гонка замера с SetScale, переносы offscreen.

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Первая раскладка корня (SetBounds в SetRoot) идёт ДО привязки владельца —
// глобальным измерителем последнего движка, а не движком дерева.
func TestOwnerServicesReview_SetRootFirstLayoutUsesOwner(t *testing.T) {
	testreset.Globals(t)
	a := New(600, 300, 20)
	t.Cleanup(a.Stop)
	a.SetDPI(144)
	b := New(600, 300, 20) // последний — глобальный измеритель
	t.Cleanup(b.Stop)

	root := widget.NewStackPanel(widget.OrientationHorizontal)
	l := widget.NewLabel(svcText, color.RGBA{A: 255})
	root.AddChild(l)
	a.SetRoot(root) // границы пустые — SetRoot раскладывает корень сам

	want, _ := l.DesiredSize() // теперь — движком A
	if got := l.Bounds().Dx(); got != want {
		t.Errorf("ширина лейбла после SetRoot(A) %d, DesiredSize движком A %d (раскладка шла глобальным измерителем B: %d)",
			got, want, widget.MeasureUIText(svcText, widget.DefaultFontSize()))
	}
}

// SetDefaultFont меняет ширины, но ревизию метрик не поднимает: память
// MeasureSet отдаёт ширину прежнего шрифта.
func TestOwnerServicesReview_SetDefaultFontStaleMemo(t *testing.T) {
	testreset.Globals(t)
	e, _, l := svcScene(t, 0, false)
	const pt = 12.0
	before := widget.MeasureTextFor(l, svcText, pt)
	gBefore := widget.MeasureUIText(svcText, pt)
	if err := e.RegisterFontFile("ZZMono", "../assets/fonts/DejaVuSansMono.ttf"); err != nil {
		t.Fatal(err)
	}
	if !e.SetDefaultFont("ZZMono") {
		t.Fatal("SetDefaultFont")
	}
	want := e.canvas.MeasureText(svcText, pt)
	if want == before {
		t.Skip("шрифты одной ширины")
	}
	got := widget.MeasureTextFor(l, svcText, pt)
	g := widget.MeasureUIText(svcText, pt)
	t.Logf("до %d, после: MeasureTextFor %d, холст %d; глобальный до %d после %d", before, got, want, gBefore, g)
	if got != want {
		t.Errorf("MeasureTextFor после SetDefaultFont: %d, холст %d (устаревшая память)", got, want)
	}
}

// Шрифт, измеренный до регистрации (замер подставным шрифтом по умолчанию),
// после RegisterFont остаётся в памяти со старой шириной.
func TestOwnerServicesReview_RegisterFontStaleMemo(t *testing.T) {
	testreset.Globals(t)
	e, _, l := svcScene(t, 0, false)
	const pt = 12.0
	before := widget.MeasureTextFontFor(l, svcText, pt, "ZZMono2")
	if err := e.RegisterFontFile("ZZMono2", "../assets/fonts/DejaVuSansMono.ttf"); err != nil {
		t.Fatal(err)
	}
	want := e.canvas.MeasureTextFont(svcText, pt, "ZZMono2")
	if want == before {
		t.Skip("шрифты одной ширины")
	}
	if got := widget.MeasureTextFontFor(l, svcText, pt, "ZZMono2"); got != want {
		t.Errorf("MeasureTextFontFor после RegisterFont: %d, холст %d (устаревшая память)", got, want)
	}
}

// Анимация виджета дерева НЕ запущенного движка (RenderOnce/offscreen без
// Start) получает ключ этого движка и не шагает никто — раньше её забирал
// первый шагнувший цикл.
func TestOwnerServicesReview_AnimationOfUnstartedOwnerNeverSteps(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	_, _, la := svcScene(t, 0, true) // offscreen, без Start
	b := New(200, 100, 60)
	t.Cleanup(b.Stop)
	b.Start()

	// Конец — по последнему тику (t = 1): OnDone, выставленный после
	// регистрации при работающем цикле, был бы гонкой (контракт Animation).
	var ticks atomic.Int32
	var once sync.Once
	done := make(chan struct{})
	widget.AnimateOwned(la, "zz", 30*time.Millisecond, nil, func(p float64) {
		ticks.Add(1)
		if p >= 1 {
			once.Do(func() { close(done) })
		}
	})
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("анимация виджета движка без цикла не доиграла: тиков %d, AnimationsActive=%v", ticks.Load(), widget.AnimationsActive())
	}
}

// Остановленный владелец: анимация с его ключом остаётся в реестре навсегда,
// OnDone не зовётся, AnimationsActive — true.
func TestOwnerServicesReview_AnimationOfStoppedOwnerLeaks(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	a, _, la := svcScene(t, 0, false)
	b := New(200, 100, 60)
	t.Cleanup(b.Stop)
	b.Start()

	var once sync.Once
	done := make(chan struct{})
	an := widget.AnimateOwned(la, "zz", 30*time.Millisecond, nil, func(p float64) {
		if p >= 1 {
			once.Do(func() { close(done) })
		}
	})
	a.Stop() // окно закрыли, пока шла анимация (A не стартовал — не успел шагнуть)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("анимация остановленного владельца не доиграла: Running=%v, AnimationsActive=%v", an.Running(), widget.AnimationsActive())
	}
}

// Перенос окна в дереве offscreen-движка: доходит до него, не до A.
func TestOwnerServicesReview_MoveOffscreenOwner(t *testing.T) {
	testreset.Globals(t)
	a, _, _ := svcScene(t, 0, false)
	c, rc, _ := svcScene(t, 0, true)
	win := widget.NewWindow("Окно", 300, 200)
	win.SetBounds(image.Rect(60, 60, 360, 260))
	rc.AddChild(win)
	a.takeMoves()
	c.takeMoves()
	win.OnMouseButton(widget.MouseEvent{X: 160, Y: 70, Button: widget.MouseLeft, Pressed: true})
	win.OnMouseMove(190, 100)
	if len(c.takeMoves()) == 0 {
		t.Error("offscreen-движок не получил перенос своего окна")
	}
	if n := len(a.takeMoves()); n != 0 {
		t.Errorf("A получил перенос окна offscreen-движка: %d", n)
	}
	// Окно без владельца: перенос — всем приёмникам (каждый сверяет дерево сам).
	free := widget.NewWindow("Ничьё", 300, 200)
	free.SetBounds(image.Rect(60, 60, 360, 260))
	var seen int
	h := widget.RegisterMoveSink(func(widget.MoveNotice) { seen++ })
	defer widget.UnregisterMoveSink(h)
	free.OnMouseButton(widget.MouseEvent{X: 160, Y: 70, Button: widget.MouseLeft, Pressed: true})
	free.OnMouseMove(190, 100)
	if seen == 0 {
		t.Error("наблюдатель не увидел перенос ничьего окна")
	}
}

// Гонка: замер дерева из горутины приложения против SetScale.
func TestOwnerServicesReview_MeasureRaceWithSetScale(t *testing.T) {
	testreset.Globals(t)
	e, _, l := svcScene(t, 0, true)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = widget.MeasureTextFor(l, svcText+string(rune('a'+i%26)), 12)
		}
	}()
	for i := 0; i < 20; i++ {
		e.SetScale(1 + float64(i%3)*0.5)
	}
	close(stop)
	wg.Wait()
}
