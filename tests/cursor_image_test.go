package tests

import (
	"image"
	"image/color"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Engine.SetCursorImage: указатель из картинки (до 384×384, NRGBA) поверх
// курсора виджетов; одинаковая картинка узнаётся по хешу; сброс.

func cursorImg(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestCursorImage_SetAndOverride(t *testing.T) {
	testreset.Globals(t)
	eng := overrideEngineWithHandButton(t)

	if _, ok := eng.CursorImageOverride(); ok {
		t.Fatal("картинка задана без вызова")
	}
	src := cursorImg(32, 24, color.NRGBA{R: 255, A: 200})
	eng.SetCursorImage(src, 4, 6)
	ci, ok := eng.CursorImageOverride()
	if !ok || ci.W != 32 || ci.H != 24 || ci.HotX != 4 || ci.HotY != 6 {
		t.Fatalf("CursorImageOverride() = %+v, %v", ci, ok)
	}
	if len(ci.Pix) != 32*24*4 || ci.Pix[0] != 255 || ci.Pix[3] != 200 {
		t.Errorf("пиксели не сохранены: %v...", ci.Pix[:4])
	}
	// Для тех, кто про картинки не знает, override виден как «задан, стрелка», и
	// он сильнее виджета без движения мыши.
	if c, set := eng.CursorOverride(); !set || c != widget.CursorArrow {
		t.Errorf("CursorOverride() = %v,%v", c, set)
	}
	if got := eng.CursorAt(50, 50); got != widget.CursorArrow {
		t.Errorf("над рукой с картинкой CursorAt = %v, ждал Arrow (override сильнее виджета)", got)
	}
}

// Приложение вправе менять свою картинку после вызова: движок держит копию.
func TestCursorImage_CopiesPixels(t *testing.T) {
	testreset.Globals(t)
	eng := engine.New(100, 100, 30)
	src := cursorImg(8, 8, color.NRGBA{G: 100, A: 255})
	eng.SetCursorImage(src, 0, 0)
	src.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 4})
	ci, _ := eng.CursorImageOverride()
	if ci.Pix[0] != 0 || ci.Pix[1] != 100 {
		t.Errorf("движок хранит ссылку на буфер приложения: %v", ci.Pix[:4])
	}
}

// Подкартинка с нулевым Min сдвинута: пиксели читаются с учётом границ.
func TestCursorImage_SubImage(t *testing.T) {
	testreset.Globals(t)
	big := cursorImg(20, 20, color.NRGBA{A: 255})
	big.SetNRGBA(5, 7, color.NRGBA{R: 9, G: 8, B: 7, A: 255})
	sub := big.SubImage(image.Rect(5, 7, 15, 17)).(*image.NRGBA)
	eng := engine.New(100, 100, 30)
	eng.SetCursorImage(sub, 0, 0)
	ci, _ := eng.CursorImageOverride()
	if ci.W != 10 || ci.H != 10 || ci.Pix[0] != 9 || ci.Pix[1] != 8 || ci.Pix[2] != 7 {
		t.Errorf("подкартинка прочитана неверно: %dx%d %v", ci.W, ci.H, ci.Pix[:4])
	}
}

// Хеш: равное содержимое и горячая точка — равный ключ; иное — иной.
func TestCursorImage_KeyByContent(t *testing.T) {
	testreset.Globals(t)
	a := widget.NewCursorImage(cursorImg(16, 16, color.NRGBA{R: 5, A: 255}), 1, 1)
	b := widget.NewCursorImage(cursorImg(16, 16, color.NRGBA{R: 5, A: 255}), 1, 1)
	c := widget.NewCursorImage(cursorImg(16, 16, color.NRGBA{R: 6, A: 255}), 1, 1)
	d := widget.NewCursorImage(cursorImg(16, 16, color.NRGBA{R: 5, A: 255}), 2, 1)
	e := widget.NewCursorImage(cursorImg(8, 32, color.NRGBA{R: 5, A: 255}), 1, 1) // то же число пикселей
	if a.Key != b.Key {
		t.Error("равные картинки получили разные ключи")
	}
	for name, o := range map[string]*widget.CursorImage{"пиксели": c, "горячая точка": d, "размер": e} {
		if o.Key == a.Key {
			t.Errorf("различие в %q не отражено в ключе", name)
		}
	}
}

// Ключ без копирования совпадает с ключом копии, в том числе у подкартинки со
// смещённым Min; недопустимая картинка ключа не имеет.
func TestCursorImage_KeyWithoutCopy(t *testing.T) {
	testreset.Globals(t)
	big := cursorImg(40, 40, color.NRGBA{R: 3, G: 4, B: 5, A: 200})
	big.SetNRGBA(12, 9, color.NRGBA{R: 99, A: 255})
	for _, img := range []*image.NRGBA{big, big.SubImage(image.Rect(10, 8, 30, 28)).(*image.NRGBA)} {
		key, ok := widget.CursorImageKey(img, 3, 4)
		ci := widget.NewCursorImage(img, 3, 4)
		if !ok || key != ci.Key {
			t.Errorf("ключ без копии %x (ok=%v) != ключ копии %x", key[:4], ok, ci.Key[:4])
		}
	}
	if _, ok := widget.CursorImageKey(nil, 0, 0); ok {
		t.Error("nil-картинка получила ключ")
	}
}

// Повторная установка той же картинки не копирует её: указатель удалённого
// рабочего стола шлют на каждое обновление.
func TestCursorImage_RepeatDoesNotCopy(t *testing.T) {
	testreset.Globals(t)
	eng := engine.New(100, 100, 30)
	img := cursorImg(384, 384, color.NRGBA{R: 7, G: 8, B: 9, A: 255})
	eng.SetCursorImage(img, 10, 10)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 20; i++ {
		eng.SetCursorImage(img, 10, 10)
	}
	runtime.ReadMemStats(&after)
	if got := after.TotalAlloc - before.TotalAlloc; got > 200<<10 { // копия одной картинки — 590 КБ
		t.Errorf("20 повторов выделили %d КБ: картинка копируется на каждый вызов", got>>10)
	}
}

// Горячая точка за пределами прижимается; недопустимые картинки — nil.
func TestCursorImage_HotspotClampAndInvalid(t *testing.T) {
	testreset.Globals(t)
	ci := widget.NewCursorImage(cursorImg(10, 10, color.NRGBA{A: 255}), -5, 99)
	if ci.HotX != 0 || ci.HotY != 9 {
		t.Errorf("горячая точка (%d,%d), ждал (0,9)", ci.HotX, ci.HotY)
	}
	if widget.NewCursorImage(nil, 0, 0) != nil {
		t.Error("nil-картинка дала курсор")
	}
	if widget.NewCursorImage(image.NewNRGBA(image.Rect(0, 0, 0, 5)), 0, 0) != nil {
		t.Error("пустая картинка дала курсор")
	}
	if widget.NewCursorImage(image.NewNRGBA(image.Rect(0, 0, widget.MaxCursorImageSide+1, 8)), 0, 0) != nil {
		t.Error("слишком широкая картинка дала курсор")
	}
	if widget.NewCursorImage(image.NewNRGBA(image.Rect(0, 0, 384, 384)), 0, 0) == nil {
		t.Error("384×384 (предел RDP) отвергнута")
	}
}

// Уведомления окна: установка, смена, снятие — да; та же картинка и лишний
// сброс — нет.
func TestCursorImage_Notifies(t *testing.T) {
	testreset.Globals(t)
	eng := overrideEngineWithHandButton(t)
	var n atomic.Int32
	eng.SetOnCursorOverrideChanged(func() { n.Add(1) })
	step := func(want int32, what string) {
		t.Helper()
		if got := n.Load(); got != want {
			t.Fatalf("%s: уведомлений %d, ждал %d", what, got, want)
		}
	}
	red := color.NRGBA{R: 255, A: 255}
	eng.SetCursorImage(nil, 0, 0) // снимать нечего
	step(0, "nil без картинки")
	eng.SetCursorImage(cursorImg(16, 16, red), 0, 0)
	step(1, "установка")
	eng.SetCursorImage(cursorImg(16, 16, red), 0, 0)
	step(1, "та же картинка")
	eng.SetCursorImage(cursorImg(16, 16, color.NRGBA{G: 255, A: 255}), 0, 0)
	step(2, "другая картинка")
	eng.SetCursorImage(nil, 0, 0)
	step(3, "сброс nil")
	if _, ok := eng.CursorImageOverride(); ok {
		t.Error("картинка осталась после SetCursorImage(nil)")
	}
	if _, set := eng.CursorOverride(); set {
		t.Error("override остался после SetCursorImage(nil)")
	}
	if got := eng.CursorAt(50, 50); got != widget.CursorHand {
		t.Errorf("после сброса CursorAt = %v, ждал Hand", got)
	}
	eng.SetCursorImage(nil, 0, 0)
	step(3, "повторный сброс")

	// ClearCursorOverride тоже снимает картинку.
	eng.SetCursorImage(cursorImg(16, 16, red), 0, 0)
	step(4, "установка после сброса")
	eng.ClearCursorOverride()
	step(5, "Clear после картинки")
	if _, ok := eng.CursorImageOverride(); ok {
		t.Error("картинка осталась после ClearCursorOverride")
	}
}

// Форма и картинка заменяют друг друга: действует последний вызов.
func TestCursorImage_ShapeAndImageReplaceEachOther(t *testing.T) {
	testreset.Globals(t)
	eng := overrideEngineWithHandButton(t)
	var n atomic.Int32
	eng.SetOnCursorOverrideChanged(func() { n.Add(1) })

	eng.SetCursorImage(cursorImg(16, 16, color.NRGBA{A: 255}), 0, 0)
	eng.SetCursorOverride(widget.CursorNone) // форма после картинки
	if _, ok := eng.CursorImageOverride(); ok {
		t.Error("картинка пережила SetCursorOverride")
	}
	if got := eng.CursorAt(1, 1); got != widget.CursorNone {
		t.Errorf("CursorAt = %v, ждал None", got)
	}
	eng.SetCursorOverride(widget.CursorNone) // та же форма — тишина
	if n.Load() != 2 {
		t.Errorf("уведомлений %d, ждал 2 (картинка, форма)", n.Load())
	}
	eng.SetCursorImage(cursorImg(16, 16, color.NRGBA{A: 255}), 0, 0) // картинка после формы
	if _, ok := eng.CursorImageOverride(); !ok {
		t.Error("картинка не заменила форму")
	}
	// SetCursorImage(nil) при действующей форме форму не трогает.
	eng.SetCursorOverride(widget.CursorWait)
	eng.SetCursorImage(nil, 0, 0)
	if c, set := eng.CursorOverride(); !set || c != widget.CursorWait {
		t.Errorf("SetCursorImage(nil) сняла форму: %v,%v", c, set)
	}
}

// Вызовы из разных горутин безопасны (гонки ловит -race).
func TestCursorImage_Concurrent(t *testing.T) {
	testreset.Globals(t)
	eng := overrideEngineWithHandButton(t)
	eng.SetOnCursorOverrideChanged(func() {})
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				switch (g + i) % 3 {
				case 0:
					eng.SetCursorImage(cursorImg(8, 8, color.NRGBA{R: uint8(i), A: 255}), 1, 1)
				case 1:
					eng.CursorImageOverride()
					eng.CursorAt(10, 10)
				default:
					eng.ClearCursorOverride()
				}
			}
		}(g)
	}
	wg.Wait()
}
