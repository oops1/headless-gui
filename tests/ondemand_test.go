package tests

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// newOnDemandEngine — движок 50 fps с простым корнем, tooltips выключены
// (чтобы «дозревание» подсказки не мешало проверке пропуска кадров).
func newOnDemandEngine() (*engine.Engine, *widget.Label) {
	eng := engine.New(320, 200, 50)
	eng.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.SetBounds(image.Rect(0, 0, 320, 200))
	lbl := widget.NewLabel("hello", color.RGBA{R: 220, G: 220, B: 220, A: 255})
	lbl.SetBounds(image.Rect(10, 10, 150, 30))
	root.AddChild(lbl)
	eng.SetRoot(root)
	return eng, lbl
}

// waitCount ждёт, пока RenderCount не станет >= want (до 2 с).
func waitCount(eng *engine.Engine, want uint64) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if eng.RenderCount() >= want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// Рендер по запросу — режим по умолчанию (v3.5): без изменений UI кадры
// пропускаются. Прежнее поведение «каждый тик» доступно через
// SetRenderOnDemand(false).
func TestOnDemand_IsDefaultMode(t *testing.T) {
	eng, _ := newOnDemandEngine()
	if !eng.RenderOnDemand() {
		t.Fatal("рендер по запросу должен быть режимом по умолчанию")
	}
	eng.Start()
	defer eng.Stop()

	if !waitCount(eng, 1) {
		t.Fatal("первый кадр не отрендерился")
	}
	time.Sleep(100 * time.Millisecond)
	base := eng.RenderCount()
	time.Sleep(300 * time.Millisecond)
	if got := eng.RenderCount(); got > base+1 {
		t.Fatalf("по умолчанию без изменений кадры должны пропускаться: %d → %d", base, got)
	}
}

// SetRenderOnDemand(false) возвращает прежнее поведение: рендер каждый тик.
func TestOnDemand_OptOutRendersContinuously(t *testing.T) {
	eng, _ := newOnDemandEngine()
	eng.SetRenderOnDemand(false)
	eng.Start()
	defer eng.Stop()
	if !waitCount(eng, 5) {
		t.Fatalf("continuous mode: RenderCount = %d, want >= 5", eng.RenderCount())
	}
}

// В on-demand режиме простой UI не рендерится: счётчик кадров замирает.
func TestOnDemand_IdleSkipsFrames(t *testing.T) {
	eng, _ := newOnDemandEngine()
	eng.SetRenderOnDemand(true)
	eng.Start()
	defer eng.Stop()

	// Первый кадр обязан отрендериться (Start инвалидирует).
	if !waitCount(eng, 1) {
		t.Fatal("первый кадр не отрендерился")
	}
	// Даём осесть возможным начальным инвалидациям.
	time.Sleep(100 * time.Millisecond)
	base := eng.RenderCount()
	time.Sleep(300 * time.Millisecond) // ~15 тиков при 50 fps
	got := eng.RenderCount()
	if got > base+1 { // +1 — допуск на тик «в полёте»
		t.Fatalf("idle: RenderCount вырос %d → %d (кадры не пропускаются)", base, got)
	}
}

// Invalidate() пробуждает рендер.
func TestOnDemand_InvalidateTriggersFrame(t *testing.T) {
	eng, _ := newOnDemandEngine()
	eng.SetRenderOnDemand(true)
	eng.Start()
	defer eng.Stop()
	waitCount(eng, 1)
	time.Sleep(100 * time.Millisecond)
	base := eng.RenderCount()

	eng.Invalidate()
	if !waitCount(eng, base+1) {
		t.Fatalf("после Invalidate RenderCount не вырос (%d)", eng.RenderCount())
	}
}

// События инвалидируют точечно: движение мыши над пустым местом не рождает
// кадров, наведение на виджет с hover-состоянием — рождает (самоинвалидация),
// клик — инвалидирует полностью.
func TestOnDemand_InputAutoInvalidates(t *testing.T) {
	eng, _ := newOnDemandEngine()
	btn := widget.NewButton("btn")
	btn.SetBounds(image.Rect(200, 100, 300, 130))
	eng.Root().AddChild(btn)
	eng.SetRenderOnDemand(true)
	eng.Start()
	defer eng.Stop()
	waitCount(eng, 1)
	time.Sleep(100 * time.Millisecond)
	base := eng.RenderCount()

	// Движение над пустым местом: hover ни у кого не меняется — кадров нет.
	eng.SendMouseMove(50, 170)
	time.Sleep(150 * time.Millisecond)
	if got := eng.RenderCount(); got > base+1 {
		t.Fatalf("mousemove над пустым местом не должен рождать кадры: %d → %d", base, got)
	}

	// Наведение на кнопку: hover изменился → кадр обязан отрендериться.
	base = eng.RenderCount()
	eng.SendMouseMove(250, 115)
	if !waitCount(eng, base+1) {
		t.Fatalf("после наведения на кнопку RenderCount не вырос (%d)", eng.RenderCount())
	}

	// Клик: полная инвалидация → кадр.
	time.Sleep(100 * time.Millisecond)
	base = eng.RenderCount()
	eng.SendMouseButton(250, 115, widget.MouseLeft, true)
	if !waitCount(eng, base+1) {
		t.Fatalf("после клика RenderCount не вырос (%d)", eng.RenderCount())
	}
}

// Виджет с фокусом и мигающей кареткой (Animated) продолжает рендериться —
// но ТОЛЬКО на смену фазы мигания, а не на каждом тике (PERF-11).
//
// Прежний контракт (NeedsAnimation()==true, пока есть фокус) заставлял движок
// рендерить ПОЛНЫЙ кадр всего дерева на целевом FPS ради каретки с полупериодом
// 530 мс. Теперь TextInput инвалидирует свой прямоугольник только когда фазу
// пора сменить, поэтому кадров примерно 2 в секунду вместо fps.
func TestOnDemand_FocusedTextInputKeepsRendering(t *testing.T) {
	eng, _ := newOnDemandEngine() // 50 fps
	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(10, 60, 200, 90))
	eng.Root().AddChild(ti)
	eng.SetRenderOnDemand(true)
	eng.Start()
	defer eng.Stop()

	eng.SetFocus(ti)
	waitCount(eng, 1)
	time.Sleep(100 * time.Millisecond) // дождаться кадра по фокусу
	base := eng.RenderCount()

	// 1.2 с — минимум две смены фазы мигания (полупериод 530 мс).
	time.Sleep(1200 * time.Millisecond)
	got := eng.RenderCount()
	if got < base+2 {
		t.Fatalf("с фокусом на TextInput каретка должна мигать (кадры): %d → %d", base, got)
	}
	// И при этом кадров должно быть на порядок меньше, чем тиков (60 при 50 fps).
	if got > base+12 {
		t.Fatalf("каретка не должна тянуть кадры на каждом тике: %d → %d за 1.2 с", base, got)
	}

	// Сняли фокус — кадры снова замирают.
	eng.SetFocus(nil)
	time.Sleep(100 * time.Millisecond)
	base = eng.RenderCount()
	time.Sleep(250 * time.Millisecond)
	if got := eng.RenderCount(); got > base+1 {
		t.Fatalf("без фокуса кадры должны замереть: %d → %d", base, got)
	}
}

// Обновление биндинга (слой данных) инвалидирует через SetUIChangeNotifier.
func TestOnDemand_BindingRefreshInvalidates(t *testing.T) {
	type vmT struct {
		datagrid.PropertyNotifier
		Title string
	}
	m := &vmT{Title: "v1"}
	const xaml = `<Canvas xmlns="clr" Width="320" Height="200">
		<TextBlock Name="t" Text="{Binding Title}"/>
	</Canvas>`
	root, _, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), m)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	eng := engine.New(320, 200, 50) // регистрирует свой Invalidate в notifier
	eng.SetTooltipsEnabled(false)
	eng.SetRoot(root)
	eng.SetRenderOnDemand(true)
	eng.Start()
	defer eng.Stop()
	waitCount(eng, 1)
	time.Sleep(100 * time.Millisecond)
	base := eng.RenderCount()

	m.Title = "v2"
	scope.Refresh()
	if !waitCount(eng, base+1) {
		t.Fatalf("после scope.Refresh RenderCount не вырос (%d)", eng.RenderCount())
	}
}

// InvalidateRect ограничивает diff заявленной областью: тайлы кадра лежат
// только в тайл-диапазоне повреждения.
func TestOnDemand_InvalidateRectLimitsDiff(t *testing.T) {
	eng, lbl := newOnDemandEngine()
	eng.SetRenderOnDemand(true)
	frames := eng.Frames()
	eng.Start()
	defer eng.Stop()
	waitCount(eng, 1)

	// Сливаем стартовые кадры.
	drain := time.After(150 * time.Millisecond)
drainLoop:
	for {
		select {
		case <-frames:
		case <-drain:
			break drainLoop
		}
	}

	// Меняем текст метки и заявляем её прямоугольник.
	lbl.SetText("CHANGED!")
	region := lbl.Bounds() // (10,10)-(150,30) → тайлы (0..2, 0)
	eng.InvalidateRect(region)

	select {
	case f := <-frames:
		if len(f.Tiles) == 0 {
			t.Fatal("пустой кадр")
		}
		for _, tile := range f.Tiles {
			tr := image.Rect(tile.X, tile.Y, tile.X+tile.W, tile.Y+tile.H)
			// Тайл обязан пересекать заявленную область (с выравниванием 64).
			if !tr.Overlaps(region) {
				t.Fatalf("тайл %v вне damage-области %v", tr, region)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("кадр после InvalidateRect не пришёл")
	}
	_ = fmt.Sprintf
}

// SetRoot не блокируется рендером и безопасен под нагрузкой (узкий e.mu).
func TestEngine_ConcurrentSetRootAndEvents(t *testing.T) {
	eng, _ := newOnDemandEngine()
	eng.Start()
	defer eng.Stop()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			p := widget.NewPanel(color.RGBA{R: uint8(i), G: 60, B: 90, A: 255})
			p.SetBounds(image.Rect(0, 0, 320, 200))
			eng.SetRoot(p)
			eng.SendMouseMove(i%320, i%200)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SetRoot/события зависли (вероятно, блокировка рендером)")
	}
}
