package window

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
)

// Переходы окна-попапа на хосте (popup_fade.go, задача 3.4).

// fadeWinNative — окно с прозрачностью целиком (как Win32): запоминает
// прозрачность и положения.
type fadeWinNative struct {
	popupFakeNative
	alphas []float64
	pos    []image.Point
	closes int
}

func (f *fadeWinNative) SetPopupAlpha(a float64) { f.alphas = append(f.alphas, a) }
func (f *fadeWinNative) SetPosition(x, y int)    { f.pos = append(f.pos, image.Pt(x, y)) }
func (f *fadeWinNative) Close()                  { f.closes++ }

// fadeCopyNative — окно с альфой точек (как X11 с композитором): запоминает
// выведенные картинки.
type fadeCopyNative struct {
	popupFakeNative
	imgs []*image.RGBA
}

func (f *fadeCopyNative) PopupPixelAlpha() bool { return true }
func (f *fadeCopyNative) blitPopupRegion(img *image.RGBA, _ []image.Rectangle) {
	f.imgs = append(f.imgs, img)
}

// noFadeNative — окно без прозрачности (X11 без композитора).
type noFadeNative struct {
	popupFakeNative
	closes int
}

func (f *noFadeNative) Close() { f.closes++ }

// fadeFixture — хост с одним уже поднятым окном native под оверлей 7.
func fadeFixture(t *testing.T, native NativeWindow, kind popupFadeKind) (*popupHost, *popupFakeNative, *hostedPopup, *image.RGBA) {
	t.Helper()
	carrier := &popupFakeNative{}
	h := newPopupHost(carrier, carrier, &queueEngine{}, 1, nil)
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	hp := &hostedPopup{native: native, rect: image.Rect(40, 30, 44, 32), img: img, w: 4, h: 2,
		alpha: 1, fade: kind}
	// Окно уже поднято и показывает кадр.
	hp.out = popupOut{placed: true, rect: hp.rect, w: 4, h: 2, alpha: 1, img: img}
	hp.setOrigin(hp.rect, 1)
	h.windows[7] = hp
	return h, carrier, hp, img
}

func step(img *image.RGBA, r image.Rectangle, op float64, closing bool) []engine.PopupFrame {
	return []engine.PopupFrame{{ID: 7, Rect: r, Img: img,
		Transition: &engine.PopupTransition{Opacity: op, Closing: closing}}}
}

func runInvoked(c *popupFakeNative) {
	fns := c.invoked
	c.invoked = nil
	for _, fn := range fns {
		fn()
	}
}

// Прозрачность окна целиком: шаг — один вызов, картинка заново не кладётся.
func TestPopupFade_WindowAlpha(t *testing.T) {
	native := &fadeWinNative{}
	h, carrier, hp, img := fadeFixture(t, native, popupFadeWindow)
	h.apply(step(img, hp.rect, 0.5, false))
	runInvoked(carrier)
	h.apply(step(img, hp.rect, 0.75, false))
	runInvoked(carrier)
	if len(native.alphas) != 2 || native.alphas[0] != 0.5 || native.alphas[1] != 0.75 {
		t.Fatalf("прозрачность окна: %v", native.alphas)
	}
	if native.blits != 0 {
		t.Fatalf("шаг перехода перекладывал картинку: %d раз", native.blits)
	}
	// Исчезающее окно держится до конца перехода.
	h.apply(step(img, hp.rect, 0.3, true))
	runInvoked(carrier)
	if native.closes != 0 || h.windows[7] == nil {
		t.Fatal("окно с прозрачностью закрыто посреди исчезновения")
	}
	h.apply(nil)
	runInvoked(carrier)
	if native.closes != 1 {
		t.Fatalf("окно не закрыто, когда ID пропал: %d", native.closes)
	}
}

// Альфа точек: шаг — копия кадра с множителем; копии чередуются, кадр
// оверлея не трогается.
func TestPopupFade_CopyAlpha(t *testing.T) {
	native := &fadeCopyNative{}
	h, carrier, hp, img := fadeFixture(t, native, popupFadeCopy)
	h.apply(step(img, hp.rect, 0.5, false))
	runInvoked(carrier)
	h.apply(step(img, hp.rect, 0.25, false))
	runInvoked(carrier)
	if len(native.imgs) != 2 {
		t.Fatalf("выведено картинок %d, ждали 2", len(native.imgs))
	}
	a, b := native.imgs[0], native.imgs[1]
	if a == img || b == img || a == b {
		t.Fatal("копия на шаг не новая или выведен сам кадр оверлея")
	}
	if got := b.RGBAAt(0, 0); got != (color.RGBA{50, 50, 50, 50}) {
		t.Fatalf("копия при 0.25: %v, ждали 200·0.25", got)
	}
	if img.Pix[0] != 200 {
		t.Fatal("кадр оверлея изменён на месте")
	}
	h.apply([]engine.PopupFrame{{ID: 7, Rect: hp.rect, Img: img}})
	if native.imgs[len(native.imgs)-1] != img {
		t.Fatal("в покое выведен не сам кадр")
	}
}

// Окно без прозрачности: исчезающее закрывается сразу, следующие кадры
// исчезновения его не поднимают, открытие снова — поднимает.
func TestPopupFade_NoFadeClosesAtOnce(t *testing.T) {
	native := &noFadeNative{}
	h, carrier, hp, img := fadeFixture(t, native, popupFadeNone)
	h.apply(step(img, hp.rect, 0.8, true))
	runInvoked(carrier)
	if native.closes != 1 {
		t.Fatalf("окно без прозрачности не закрыто в начале исчезновения: %d", native.closes)
	}
	h.apply(step(img, hp.rect, 0.4, true))
	if len(carrier.invoked) != 0 {
		t.Fatal("кадр исчезновения что-то затеял у закрытого окна")
	}
	h.apply([]engine.PopupFrame{{ID: 7, Rect: hp.rect, Img: img}})
	if len(carrier.invoked) != 1 {
		t.Fatalf("открытие снова не подняло окно: вызовов %d", len(carrier.invoked))
	}
}

// Ввод над исчезающим попапом уходит движку в точке, где окно стоит сейчас
// (со сдвигом): оверлея в дереве уже нет, и событие достаётся тому, что
// видно под ним.
func TestPopupFade_ClosingForwardsInput(t *testing.T) {
	native := &fadeWinNative{}
	h, _, hp, img := fadeFixture(t, native, popupFadeWindow)
	eng := h.eng.(*queueEngine)
	h.setupPopupInput(native, hp)
	h.apply([]engine.PopupFrame{{ID: 7, Rect: hp.rect, Img: img,
		Transition: &engine.PopupTransition{Opacity: 0.5, Offset: image.Pt(0, -4), Closing: true}}})
	native.onMove(1, 1)
	eng.run()
	wantLog(t, eng, "move 41,27")
}

// Задания вывода в любом порядке (X11: горутина на вызов) приводят окно к
// последнему состоянию: опоздавшее старое задание не перекрывает конец.
func TestPopupFade_OutOfOrderJobsEndAtLatest(t *testing.T) {
	native := &fadeWinNative{}
	h, carrier, hp, img := fadeFixture(t, native, popupFadeWindow)
	stepAt := func(op float64, dy int) {
		h.apply([]engine.PopupFrame{{ID: 7, Rect: hp.rect, Img: img,
			Transition: &engine.PopupTransition{Opacity: op, Offset: image.Pt(0, dy)}}})
	}
	stepAt(0.2, -3)
	stepAt(0.5, -2)
	fns := carrier.invoked
	carrier.invoked = nil
	fns[1]() // новое задание успело раньше старого
	fns[0]() // старое опоздало
	if !slices.Equal(native.alphas, []float64{0.5}) || len(native.pos) != 1 || native.pos[0] != hp.rect.Min.Add(image.Pt(0, -2)) {
		t.Fatalf("опоздавшее задание перекрыло новое: alphas %v, pos %v", native.alphas, native.pos)
	}
	h.apply([]engine.PopupFrame{{ID: 7, Rect: hp.rect, Img: img}})
	runInvoked(carrier)
	if got := native.alphas[len(native.alphas)-1]; got != 1 {
		t.Fatalf("последняя прозрачность %v, ждали покой 1 (alphas %v)", got, native.alphas)
	}
	if got := native.pos[len(native.pos)-1]; got != hp.rect.Min {
		t.Fatalf("последнее положение %v, ждали место покоя %v (pos %v)", got, hp.rect.Min, native.pos)
	}
}

// Попап-потомок (Wayland) прозрачности не прикладывает: исчезающий уходит
// сразу.
func TestChildPopups_ClosingGoesAtOnce(t *testing.T) {
	be := &fakeChildHost{}
	c := newChildPopups(be, &fakePopupEngine{}, 1, nil)
	f := popupFrame(1, image.Rect(10, 10, 60, 40), 1)
	f.Transition = &engine.PopupTransition{Opacity: 0.2}
	c.apply([]engine.PopupFrame{f})
	if len(be.opened) != 1 {
		t.Fatalf("появляющийся попап не открыт сразу: %v", be.opened)
	}
	f.Transition = &engine.PopupTransition{Opacity: 0.6, Closing: true}
	c.apply([]engine.PopupFrame{f})
	if len(be.closed) != 1 {
		t.Fatalf("исчезающий попап не закрыт сразу: %v", be.closed)
	}
}

// Окно без прозрачности не проезжает путь перехода: непрозрачное меню
// встаёт на место сразу.
func TestPopupFade_NoFadeStaysPut(t *testing.T) {
	native := &fadeWinNative{}
	h, carrier, hp, img := fadeFixture(t, native, popupFadeNone)
	h.apply([]engine.PopupFrame{{ID: 7, Rect: hp.rect, Img: img,
		Transition: &engine.PopupTransition{Opacity: 0.3, Offset: image.Pt(0, -6)}}})
	runInvoked(carrier)
	for _, p := range native.pos {
		if p != hp.rect.Min {
			t.Fatalf("окно без прозрачности сдвинуто в %v, ждали место покоя %v", p, hp.rect.Min)
		}
	}
}
