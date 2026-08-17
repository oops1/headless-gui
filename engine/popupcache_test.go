package engine

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// popupCacheEngine — движок с открытым меню, активным sink и счётчиком вызовов.
func popupCacheEngine(t *testing.T) (*Engine, *widget.Dropdown, *int, *sync.Mutex) {
	t.Helper()
	e := New(300, 300, 20)
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 300))
	dd := widget.NewDropdown("Alpha", "Beta", "Gamma")
	dd.SetBounds(image.Rect(40, 40, 200, 70))
	root.AddChild(dd)
	e.SetRoot(root)

	var mu sync.Mutex
	calls := 0
	e.SetPopupSink(func([]PopupFrame) {
		mu.Lock()
		calls++
		mu.Unlock()
	})
	dd.SetOpen(true)
	return e, dd, &calls, &mu
}

// Инвалидация вне меню не заставляет пересобирать и переотдавать попап.
func TestPopupCache_OutsideDamageKeepsSink(t *testing.T) {
	e, dd, calls, mu := popupCacheEngine(t)
	defer e.Stop()

	e.renderFrame() // кадр открытия — sink вызван
	mu.Lock()
	openCalls := *calls
	mu.Unlock()
	if openCalls != 1 {
		t.Fatalf("при открытии ожидался 1 вызов sink, got %d", openCalls)
	}

	far := image.Rect(240, 250, 290, 290)
	if far.Overlaps(dd.OverlayBounds()) {
		t.Fatalf("область %v пересекает меню %v", far, dd.OverlayBounds())
	}
	for i := 0; i < 30; i++ {
		e.InvalidateRect(far)
		e.renderFrame()
	}

	mu.Lock()
	got := *calls
	mu.Unlock()
	if got != openCalls {
		t.Fatalf("sink вызван на инвалидации вне меню: %d → %d", openCalls, got)
	}
}

// Кадры без рендера попапа не аллоцируют.
func TestPopupCache_NoAllocPerFrame(t *testing.T) {
	e, dd, _, _ := popupCacheEngine(t)
	defer e.Stop()

	far := image.Rect(240, 250, 290, 290)
	if far.Overlaps(dd.OverlayBounds()) {
		t.Skip("меню перекрывает тестовую область")
	}
	for i := 0; i < 5; i++ { // прогрев буферов
		e.InvalidateRect(far)
		e.renderFrame()
	}
	allocs := testing.AllocsPerRun(50, func() {
		e.InvalidateRect(far)
		e.renderFrame()
	})
	if allocs > 4 {
		t.Fatalf("аллокаций на кадр: %v (ожидалось ≤ 4)", allocs)
	}
}

// Изменение содержимого меню доходит до хоста.
func TestPopupCache_ContentChangeReachesSink(t *testing.T) {
	e, dd, calls, mu := popupCacheEngine(t)
	defer e.Stop()
	e.renderFrame()

	mu.Lock()
	base := *calls
	mu.Unlock()

	// Наводим курсор на второй пункт списка — подсветка меняет пиксели.
	r := dd.OverlayBounds()
	e.SendMouseMove(r.Min.X+5, r.Min.Y+r.Dy()/2)
	e.renderFrame()

	mu.Lock()
	got := *calls
	mu.Unlock()
	if got == base {
		t.Fatal("изменение содержимого меню не дошло до sink")
	}
}

// Закрытие меню чистит кэш оверлеев.
func TestPopupCache_ClearedOnClose(t *testing.T) {
	e, dd, _, _ := popupCacheEngine(t)
	defer e.Stop()
	e.renderFrame()

	e.popupMu.Lock()
	n := len(e.popupCache)
	e.popupMu.Unlock()
	if n != 1 {
		t.Fatalf("в кэше %d записей, ожидалась 1", n)
	}

	dd.SetOpen(false)
	e.renderFrame()

	e.popupMu.Lock()
	n = len(e.popupCache)
	e.popupMu.Unlock()
	if n != 0 {
		t.Fatalf("после закрытия в кэше осталось %d записей", n)
	}
}

// Кэшированный канвас даёт те же пиксели, что и свежий.
func TestPopupCache_PixelIdentity(t *testing.T) {
	e, dd, _, _ := popupCacheEngine(t)
	defer e.Stop()
	e.renderFrame()

	r := dd.OverlayBounds()
	want := e.canvas.renderOverlay(dd, r) // свежий канвас

	e.popupMu.Lock()
	ent := e.popupCache[widgetID(dd)]
	e.popupMu.Unlock()
	if ent == nil || ent.img == nil {
		t.Fatal("попап не попал в кэш")
	}
	// Повторный рендер в переиспользованный канвас.
	got := renderOverlayInto(ent.overlayCanvas(e.canvas, r), dd, r)

	if got.Rect != want.Rect {
		t.Fatalf("размер %v != %v", got.Rect, want.Rect)
	}
	for i := range want.Pix {
		if got.Pix[i] != want.Pix[i] {
			t.Fatalf("байт %d: кэш=%d свежий=%d", i, got.Pix[i], want.Pix[i])
		}
	}
}
