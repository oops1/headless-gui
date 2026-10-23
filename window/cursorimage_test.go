package window

import (
	"fmt"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Курсор из картинки (Engine.SetCursorImage): кэш с вытеснением, пересчёт
// пикселей, путь от движка до окна ОС и до попапов.

// Настоящий движок удовлетворяет обоим необязательным интерфейсам окна: иначе
// картинка тихо не доходила бы до ОС.
var (
	_ cursorImageSource      = (*engine.Engine)(nil)
	_ cursorOverrideSource   = (*engine.Engine)(nil)
	_ cursorOverrideNotifier = (*engine.Engine)(nil)
)

func ciKey(i int) [32]byte {
	var k [32]byte
	k[0] = byte(i)
	return k
}

func ciNRGBA(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// ─── Кэш ─────────────────────────────────────────────────────────────────────

func TestCursorLRU_EvictsOldestAndFrees(t *testing.T) {
	var freed []int
	c := newCursorLRU[int](3, func(v int) { freed = append(freed, v) }, nil)
	for i := 1; i <= 3; i++ {
		c.put(ciKey(i), i*10)
	}
	c.get(ciKey(1)) // 1 стал свежим, самый давний теперь 2
	c.put(ciKey(4), 40)
	if !reflect.DeepEqual(freed, []int{20}) {
		t.Fatalf("освобождены %v, ждал [20] (самый давний)", freed)
	}
	if _, ok := c.get(ciKey(2)); ok {
		t.Error("вытесненный курсор остался в кэше")
	}
	for _, i := range []int{1, 3, 4} {
		if v, ok := c.get(ciKey(i)); !ok || v != i*10 {
			t.Errorf("ключ %d: %v %v", i, v, ok)
		}
	}
	if c.len() != 3 {
		t.Errorf("в кэше %d записей, ждал 3", c.len())
	}
}

// Показываемый сейчас курсор не вытесняется, даже будучи самым давним.
func TestCursorLRU_KeepsShownCursor(t *testing.T) {
	var freed []int
	shown := 10
	c := newCursorLRU[int](2, func(v int) { freed = append(freed, v) }, func(v int) bool { return v == shown })
	c.put(ciKey(1), 10)
	c.put(ciKey(2), 20)
	c.put(ciKey(3), 30) // самый давний — 10, но он показан: уходит 20
	if !reflect.DeepEqual(freed, []int{20}) {
		t.Fatalf("освобождены %v, ждал [20]", freed)
	}
	if _, ok := c.get(ciKey(1)); !ok {
		t.Error("показанный курсор вытеснен")
	}
	shown = 99 // показ сменился — 10 снова обычная запись
	c.put(ciKey(4), 40)
	c.put(ciKey(5), 50)
	if !reflect.DeepEqual(freed, []int{20, 30, 10}) {
		t.Errorf("после смены показа освобождены %v, ждал [20 30 10]", freed)
	}
}

func TestCursorLRU_ClearFreesAll(t *testing.T) {
	n := 0
	c := newCursorLRU[int](cursorCacheSize, func(int) { n++ }, nil)
	for i := 0; i < cursorCacheSize; i++ {
		c.put(ciKey(i), i)
	}
	c.clear()
	if n != cursorCacheSize || c.len() != 0 {
		t.Errorf("clear: освобождено %d из %d, осталось %d", n, cursorCacheSize, c.len())
	}
}

// ─── Пиксели ─────────────────────────────────────────────────────────────────

func TestCursorPremul(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 255}) // непрозрачный — без изменений
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 0, B: 128, A: 128})  // половина
	img.SetNRGBA(2, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 0})  // прозрачный → нули
	ci := widget.NewCursorImage(img, 0, 0)

	rgba := cursorPremulRGBA(ci)
	want := []byte{200, 100, 50, 255, 128, 0, 64, 128, 0, 0, 0, 0}
	if !reflect.DeepEqual(rgba.Pix, want) {
		t.Errorf("RGBA домноженный: %v, ждал %v", rgba.Pix, want)
	}
	bgra := cursorPremulBGRA(ci)
	wantB := []byte{50, 100, 200, 255, 64, 0, 128, 128, 0, 0, 0, 0}
	if !reflect.DeepEqual(bgra, wantB) {
		t.Errorf("BGRA домноженный: %v, ждал %v", bgra, wantB)
	}
}

func TestCursorMonoPoints(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})                         // чёрный непрозрачный: виден, рисунок
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255}) // белый: виден, фон
	img.SetNRGBA(2, 0, color.NRGBA{A: 100})                         // ниже порога: нет
	src, mask := cursorMonoPoints(widget.NewCursorImage(img, 0, 0), 128)
	if !reflect.DeepEqual(mask, [][2]int16{{0, 0}, {1, 0}}) {
		t.Errorf("маска %v", mask)
	}
	if !reflect.DeepEqual(src, [][2]int16{{0, 0}}) {
		t.Errorf("рисунок %v", src)
	}
}

func TestCursorScaledHotspot(t *testing.T) {
	for _, tc := range []struct{ hot, scale, want int }{
		{16, 1, 16}, {16, 2, 8}, {17, 2, 8}, {5, 0, 5}, {0, 3, 0},
	} {
		if got := cursorScaledHotspot(tc.hot, tc.scale); got != tc.want {
			t.Errorf("hot=%d scale=%d: %d, ждал %d", tc.hot, tc.scale, got, tc.want)
		}
	}
}

// ─── Путь движок → окно ОС ───────────────────────────────────────────────────

// ciEngine — движок с курсором-картинкой, как *engine.Engine.
type ciEngine struct {
	*overrideEngine
	img *widget.CursorImage
}

func (e *ciEngine) CursorImageOverride() (*widget.CursorImage, bool) { return e.img, e.img != nil }

// setImage имитирует Engine.SetCursorImage; clearImage — ClearCursorOverride.
func (e *ciEngine) setImage(ci *widget.CursorImage) {
	e.img = ci
	e.has, e.ovr = true, widget.CursorArrow
	e.hook()
}

func (e *ciEngine) clearImage() {
	e.img = nil
	e.has = false
	e.hook()
}

// ciNative — окно ОС, принимающее и формы, и картинки.
type ciNative struct {
	inputNative
	images []*widget.CursorImage
	order  []string
}

func (f *ciNative) SetCursor(c int) {
	f.inputNative.SetCursor(c)
	f.order = append(f.order, fmt.Sprintf("shape %d", c))
}

func (f *ciNative) SetCursorImage(ci *widget.CursorImage) {
	f.images = append(f.images, ci)
	f.order = append(f.order, "image")
}

func ciSurface(t *testing.T) (*surface, *ciNative, *ciEngine) {
	t.Helper()
	eng := &ciEngine{overrideEngine: &overrideEngine{queueEngine: &queueEngine{}}}
	fn := &ciNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()
	if eng.hook == nil {
		t.Fatal("окно не подписалось на смену принудительного курсора")
	}
	return s, fn, eng
}

func ciPump(eng *ciEngine, fn *ciNative) {
	for {
		n := eng.queueEngine.run()
		inv := fn.invoked
		fn.invoked = nil
		for _, f := range inv {
			f()
		}
		if n == 0 && len(inv) == 0 {
			return
		}
	}
}

func ciImage(c color.NRGBA) *widget.CursorImage {
	return widget.NewCursorImage(ciNRGBA(8, 8, c), 1, 2)
}

// Картинка доходит до окна ОС сразу, без движения мыши; повтор той же картинки
// окно не беспокоит; другая — отдаётся.
func TestCursorImage_AppliedOnceUntilChanged(t *testing.T) {
	_, fn, eng := ciSurface(t)
	a := ciImage(color.NRGBA{R: 255, A: 255})
	eng.setImage(a)
	ciPump(eng, fn)
	if len(fn.images) != 1 || fn.images[0] != a {
		t.Fatalf("картинка не дошла до окна: %v", fn.images)
	}
	eng.setImage(ciImage(color.NRGBA{R: 255, A: 255})) // равное содержимое
	ciPump(eng, fn)
	fn.onMove(10, 10) // движение тоже не должно её пересылать
	ciPump(eng, fn)
	if len(fn.images) != 1 {
		t.Errorf("та же картинка отдана повторно: %d раз", len(fn.images))
	}
	b := ciImage(color.NRGBA{G: 255, A: 255})
	eng.setImage(b)
	ciPump(eng, fn)
	if len(fn.images) != 2 || fn.images[1] != b {
		t.Errorf("новая картинка не отдана: %v", fn.images)
	}
	if len(fn.cursors) != 0 {
		t.Errorf("при картинке окну ушла форма курсора: %v", fn.cursors)
	}
}

// Картинку сняли — окну уходит форма (стрелка), даже если до картинки форма
// тоже была стрелкой.
func TestCursorImage_ClearReturnsShape(t *testing.T) {
	_, fn, eng := ciSurface(t)
	fn.onMove(10, 10) // стрелка отдана
	ciPump(eng, fn)
	if len(fn.cursors) != 1 {
		t.Fatalf("формы %v", fn.cursors)
	}
	eng.setImage(ciImage(color.NRGBA{B: 255, A: 255}))
	ciPump(eng, fn)
	eng.clearImage()
	ciPump(eng, fn)
	want := []string{"shape 0", "image", "shape 0"}
	if !reflect.DeepEqual(fn.order, want) {
		t.Errorf("порядок вызовов окна %v, ждал %v", fn.order, want)
	}
}

// Окно без поддержки картинок получает обычный путь: override-стрелка.
func TestCursorImage_NativeWithoutSupport(t *testing.T) {
	eng := &ciEngine{overrideEngine: &overrideEngine{queueEngine: &queueEngine{}}}
	fn := &inputNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()
	eng.setImage(ciImage(color.NRGBA{R: 1, A: 255}))
	pump(eng.queueEngine, fn)
	if len(fn.cursors) != 1 || fn.cursors[0] != int(widget.CursorArrow) {
		t.Errorf("формы %v, ждал [стрелка]", fn.cursors)
	}
}

// ─── Попапы ──────────────────────────────────────────────────────────────────

// ciPopup — окно-попап, принимающее и формы, и картинки.
type ciPopup struct {
	pcPopup
	images []*widget.CursorImage
}

func (f *ciPopup) SetCursorImage(ci *widget.CursorImage) { f.images = append(f.images, ci) }

// ciPopupEngine — движок попапов с картинкой.
type ciPopupEngine struct {
	pcEngine
	img *widget.CursorImage
}

func (e *ciPopupEngine) CursorImageOverride() (*widget.CursorImage, bool) { return e.img, e.img != nil }

func TestPopupHost_CursorImageReachesPopups(t *testing.T) {
	eng := &ciPopupEngine{}
	carrier := &pcPopup{}
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	pop := &ciPopup{}
	h.windows[1] = &hostedPopup{native: pop, rect: image.Rect(0, 0, 10, 10)}

	a := ciImage(color.NRGBA{R: 255, A: 255})
	eng.img, eng.pcEngine.set, eng.pcEngine.cur = a, true, widget.CursorArrow
	h.syncCursorOverride()
	h.syncCursorOverride() // повтор — тишина
	if len(pop.images) != 1 || pop.images[0] != a || len(pop.shapes) != 0 {
		t.Fatalf("попап: картинки %v формы %v", pop.images, pop.shapes)
	}
	// Другая картинка.
	b := ciImage(color.NRGBA{G: 255, A: 255})
	eng.img = b
	h.syncCursorOverride()
	if len(pop.images) != 2 || pop.images[1] != b {
		t.Errorf("новая картинка не дошла: %v", pop.images)
	}
	// Сняли — стрелка.
	eng.img, eng.pcEngine.set = nil, false
	h.syncCursorOverride()
	if len(pop.shapes) != 1 || pop.shapes[0] != int(widget.CursorArrow) {
		t.Errorf("после снятия формы %v, ждал [стрелка]", pop.shapes)
	}
}

// Картинка, заданная до открытия попапа, ставится окну сразу после создания
// (createPopup читает запомненное хостом).
func TestPopupHost_CursorImageRememberedForNewPopup(t *testing.T) {
	eng := &ciPopupEngine{}
	carrier := &pcPopup{}
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	h.windows[9] = &hostedPopup{} // окно ещё поднимается
	eng.img, eng.pcEngine.set = ciImage(color.NRGBA{R: 7, A: 255}), true
	h.syncCursorOverride()
	h.mu.Lock()
	got := h.ovrImg
	h.mu.Unlock()
	if got != eng.img {
		t.Error("хост не запомнил картинку для окна, которое поднимается")
	}
}

// ciChildHost — бэкенд попапов-потомков с курсором-картинкой.
type ciChildHost struct {
	pcChildHost
	images []*widget.CursorImage
}

func (f *ciChildHost) SetPopupCursorImage(ci *widget.CursorImage) { f.images = append(f.images, ci) }

func TestChildPopups_CursorImage(t *testing.T) {
	be := &ciChildHost{}
	eng := &ciPopupEngine{}
	c := newChildPopups(be, eng, 1, nil)

	a := ciImage(color.NRGBA{R: 255, A: 255})
	eng.img, eng.pcEngine.set = a, true
	c.syncCursorOverride()
	c.syncCursorOverride()
	if len(be.images) != 1 || be.images[0] != a || len(be.calls) != 0 {
		t.Fatalf("картинки %v, формы %v", be.images, be.calls)
	}
	eng.img, eng.pcEngine.set = nil, false
	c.syncCursorOverride()
	if len(be.calls) != 1 || be.calls[0] != "clear" {
		t.Errorf("после снятия вызовы %v, ждал [clear]", be.calls)
	}

	// Попап открылся под действующей картинкой.
	be.images, be.calls = nil, nil
	eng.img, eng.pcEngine.set = a, true
	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(0, 0, 50, 40), 1)})
	if len(be.images) != 1 {
		t.Errorf("при открытии попапа картинка не отдана: %v", be.images)
	}
}

// Настоящий движок: SetCursorImage с любой горутины доходит до окна ОС через
// очередь движка; та же картинка окно не беспокоит; сброс возвращает форму виджета.
func TestCursorImage_RealEngine(t *testing.T) {
	eng := engine.New(200, 100, 30)
	btn := widget.NewButton("ok")
	btn.SetBounds(image.Rect(0, 0, 200, 100))
	btn.SetCursor(widget.CursorHand)
	eng.SetRoot(btn)
	eng.RenderOnce()

	fn := &ciNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()
	step := func() {
		eng.RenderOnce()
		inv := fn.invoked
		fn.invoked = nil
		for _, f := range inv {
			f()
		}
	}

	fn.onMove(10, 10)
	step()
	img := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	eng.SetCursorImage(img, 2, 3)
	step()
	if len(fn.images) != 1 || fn.images[0].HotX != 2 || fn.images[0].HotY != 3 || fn.images[0].W != 24 {
		t.Fatalf("картинка не дошла до окна: %+v", fn.images)
	}
	eng.SetCursorImage(img, 2, 3) // та же
	fn.onMove(20, 20)
	step()
	if len(fn.images) != 1 {
		t.Errorf("та же картинка отдана повторно: %d", len(fn.images))
	}
	eng.SetCursorImage(nil, 0, 0)
	step()
	last := fn.cursors[len(fn.cursors)-1]
	if last != int(widget.CursorHand) {
		t.Errorf("после сброса форма %d, ждал Hand (%d)", last, widget.CursorHand)
	}
}
