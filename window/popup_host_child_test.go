package window

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Хост попапов-потомков: там, где попап не самостоятельное окно ОС, а
// поверхность носителя (Wayland). popupHost туда не годится — он ставит окна
// по экранным координатам, которых у клиента Wayland нет вовсе.

// fakeChildHost — бэкенд, записывающий, о чём его просили.
type fakeChildHost struct {
	opened  []string
	moved   []string
	blitted []uintptr
	imgs    []*image.RGBA       // картинки блитов, по порядку
	bands   [][]image.Rectangle // области ввода блитов, по порядку
	closed  []uintptr
	calls   []string // все вызовы по порядку: "open 1", "blit 1", "close 1"
	h       childPopupHandlers
}

func (f *fakeChildHost) OpenChildPopup(id uintptr, x, y, w, h int) error {
	f.opened = append(f.opened, popupCall(id, x, y, w, h))
	f.calls = append(f.calls, fmt.Sprintf("open %d", id))
	return nil
}

func (f *fakeChildHost) MoveChildPopup(id uintptr, x, y, w, h int) {
	f.moved = append(f.moved, popupCall(id, x, y, w, h))
	f.calls = append(f.calls, fmt.Sprintf("move %d", id))
}

func (f *fakeChildHost) BlitChildPopup(id uintptr, img *image.RGBA, bands []image.Rectangle) {
	f.blitted = append(f.blitted, id)
	f.imgs = append(f.imgs, img)
	f.bands = append(f.bands, bands)
	f.calls = append(f.calls, fmt.Sprintf("blit %d", id))
}

func (f *fakeChildHost) CloseChildPopup(id uintptr) {
	f.closed = append(f.closed, id)
	f.calls = append(f.calls, fmt.Sprintf("close %d", id))
}

func (f *fakeChildHost) SetChildPopupHandlers(h childPopupHandlers) { f.h = h }

func popupCall(id uintptr, x, y, w, h int) string {
	return fmt.Sprintf("%d:%d,%d %dx%d", int(id), x, y, w, h)
}

// fakePopupEngine — движок: запоминает пришедший ввод.
type fakePopupEngine struct {
	moves   []image.Point
	buttons []image.Point
	closes  int
}

func (e *fakePopupEngine) SendMouseMove(x, y int) { e.moves = append(e.moves, image.Pt(x, y)) }
func (e *fakePopupEngine) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool) {
	if pressed {
		e.buttons = append(e.buttons, image.Pt(x, y))
	}
}
func (e *fakePopupEngine) CloseAllOverlays() { e.closes++ }

func popupFrame(id uintptr, r image.Rectangle, scale int) engine.PopupFrame {
	img := image.NewRGBA(image.Rect(0, 0, r.Dx()*scale, r.Dy()*scale))
	return engine.PopupFrame{ID: id, Rect: r, Img: img}
}

func TestChildPopups_OpenMoveClose(t *testing.T) {
	be := &fakeChildHost{}
	c := newChildPopups(be, &fakePopupEngine{}, 1, nil)

	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(40, 28, 240, 188), 1)})
	if len(be.opened) != 1 || be.opened[0] != "1:40,28 200x160" {
		t.Fatalf("открытие: %v", be.opened)
	}
	if len(be.blitted) != 1 {
		t.Errorf("кадр не отдан бэкенду: %v", be.blitted)
	}

	// Тот же прямоугольник — только кадр, без перестановки окна.
	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(40, 28, 240, 188), 1)})
	if len(be.moved) != 0 {
		t.Errorf("попап переставлен без нужды: %v", be.moved)
	}
	if len(be.blitted) != 2 {
		t.Errorf("второй кадр не отдан: %v", be.blitted)
	}

	// Раскрылось подменю — попап стал шире.
	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(40, 28, 400, 188), 1)})
	if len(be.moved) != 1 || be.moved[0] != "1:40,28 360x160" {
		t.Errorf("перестановка: %v", be.moved)
	}

	// Меню закрыли — попапа в кадре больше нет.
	c.apply(nil)
	if len(be.closed) != 1 || be.closed[0] != 1 {
		t.Errorf("закрытие: %v", be.closed)
	}
}

// Движок считает в логических точках, бэкенд — в физических: на HiDPI место
// попапа и его кадр должны сойтись.
func TestChildPopups_ScaleToPhysical(t *testing.T) {
	be := &fakeChildHost{}
	c := newChildPopups(be, &fakePopupEngine{}, 2, nil)

	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(40, 28, 140, 128), 2)})
	if len(be.opened) != 1 || be.opened[0] != "1:80,56 200x200" {
		t.Fatalf("открытие на масштабе 2: %v", be.opened)
	}
}

// Щелчок по пункту приходит в координатах попапа — движку он нужен в
// координатах холста, там, где движок сам нарисовал оверлей.
func TestChildPopups_InputTranslated(t *testing.T) {
	be := &fakeChildHost{}
	eng := &fakePopupEngine{}
	c := newChildPopups(be, eng, 1, nil)
	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(40, 28, 240, 188), 1)})

	be.h.Move(1, 10, 20)
	be.h.Button(1, 10, 20, 0, true)

	if len(eng.moves) != 1 || eng.moves[0] != image.Pt(50, 48) {
		t.Errorf("движение дошло как %v, ждал 50,48", eng.moves)
	}
	if len(eng.buttons) != 1 || eng.buttons[0] != image.Pt(50, 48) {
		t.Errorf("щелчок дошёл как %v, ждал 50,48", eng.buttons)
	}
}

// Ввод от попапа, которого уже нет, игнорируется: меню закрылось, а событие
// было в пути.
func TestChildPopups_InputAfterClose(t *testing.T) {
	be := &fakeChildHost{}
	eng := &fakePopupEngine{}
	c := newChildPopups(be, eng, 1, nil)
	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(0, 0, 100, 100), 1)})
	c.apply(nil)

	be.h.Move(1, 5, 5)
	be.h.Button(1, 5, 5, 0, true)
	if len(eng.moves) != 0 || len(eng.buttons) != 0 {
		t.Errorf("ввод закрытого попапа дошёл до движка: %v %v", eng.moves, eng.buttons)
	}
}

// Щелчок мимо меню компоновщик обрабатывает сам: окна больше нет, и оверлей
// в дереве виджетов надо убрать, иначе он останется открытым без окна.
func TestChildPopups_DoneClosesOverlays(t *testing.T) {
	be := &fakeChildHost{}
	eng := &fakePopupEngine{}
	c := newChildPopups(be, eng, 1, nil)
	c.apply([]engine.PopupFrame{popupFrame(1, image.Rect(0, 0, 100, 100), 1)})

	be.h.Done(1)
	if eng.closes != 1 {
		t.Errorf("оверлеи движка не закрыты (%d)", eng.closes)
	}
}

// ─── Переходы (engine.PopupTransition) ──────────────────────────────────────

// cascadeImg — кадр каскадного меню 100×20: верхняя полоса во всю ширину,
// под ней подменю справа, слева снизу дыра. Закрашенное — premultiplied-серый
// 200.
func cascadeImg() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 100, 20))
	for y := 0; y < 20; y++ {
		x0 := 0
		if y >= 10 {
			x0 = 60
		}
		for x := x0; x < 100; x++ {
			copy(img.Pix[img.PixOffset(x, y):], []byte{200, 200, 200, 200})
		}
	}
	return img
}

// fadeStep — кадр оверлея id на шаге перехода.
func fadeStep(id uintptr, img *image.RGBA, r image.Rectangle, op float64, closing bool) engine.PopupFrame {
	return engine.PopupFrame{ID: id, Rect: r, Img: img,
		Transition: &engine.PopupTransition{Opacity: op, Closing: closing}}
}

// Непрозрачность закладывается в копию кадра: множитель на все четыре канала
// (кадр premultiplied), кадр движка не трогается, область ввода — по кадру в
// покое. В покое бэкенду уходит сам кадр.
func TestChildPopups_FadeBakedIntoCopy(t *testing.T) {
	be := &fakeChildHost{}
	c := newChildPopups(be, &fakePopupEngine{}, 1, nil)
	r := image.Rect(10, 10, 110, 30)
	img := cascadeImg()
	rest := engine.OpaqueBands(img)
	if len(rest) == 0 {
		t.Fatal("у кадра-ступеньки нет полос — тест ничего не проверит")
	}

	c.apply([]engine.PopupFrame{fadeStep(1, img, r, 0.5, false)})
	if len(be.opened) != 1 || len(be.imgs) != 1 {
		t.Fatalf("появляющийся попап: открыт %v, кадров %d", be.opened, len(be.imgs))
	}
	got := be.imgs[0]
	if got == img {
		t.Fatal("на шаге перехода отдан сам кадр движка, а не копия")
	}
	if px := got.Pix[got.PixOffset(70, 15):][:4]; px[0] != 100 || px[1] != 100 || px[2] != 100 || px[3] != 100 {
		t.Errorf("точка копии при 0.5: %v, ждали 200·0.5 во всех каналах", px)
	}
	if img.Pix[0] != 200 {
		t.Error("кадр движка изменён на месте")
	}

	// Совсем прозрачный шаг: клики попап ловит там же, где в покое.
	c.apply([]engine.PopupFrame{fadeStep(1, img, r, 0, false)})
	if len(be.imgs) != 2 || be.imgs[1].Pix[be.imgs[1].PixOffset(70, 15)+3] != 0 {
		t.Fatalf("шаг 0: кадров %d", len(be.imgs))
	}
	if fmt.Sprint(be.bands[1]) != fmt.Sprint(rest) {
		t.Errorf("область ввода на шаге 0: %v, ждали полосы кадра в покое %v", be.bands[1], rest)
	}

	// Шаг, сменивший один сдвиг: картинка та же — ни кадра, ни перестановки
	// (сдвиг не прикладывается: xdg_popup переставляется только пересозданием).
	f := fadeStep(1, img, r, 0, false)
	f.Transition.Offset = image.Pt(0, -4)
	c.apply([]engine.PopupFrame{f})
	if len(be.imgs) != 2 || len(be.moved) != 0 {
		t.Errorf("шаг со сдвигом: кадров %d (ждали 2), перестановок %v", len(be.imgs), be.moved)
	}

	// Покой — сам кадр.
	c.apply([]engine.PopupFrame{{ID: 1, Rect: r, Img: img}})
	if len(be.imgs) != 3 || be.imgs[2] != img {
		t.Fatalf("в покое отдан не сам кадр (кадров %d)", len(be.imgs))
	}
	if len(be.opened) != 1 || len(be.closed) != 0 {
		t.Errorf("переход пересоздавал попап: открыт %v, закрыт %v", be.opened, be.closed)
	}
}

// Исчезающий попап держится, пока его ID в кадрах: гаснет копией, ввод над
// ним уходит движку, popup_done о нём оверлеи не закрывает. Уходит, когда ID
// пропал.
func TestChildPopups_ClosingHeldUntilIDGone(t *testing.T) {
	be := &fakeChildHost{}
	eng := &fakePopupEngine{}
	c := newChildPopups(be, eng, 1, nil)
	r := image.Rect(40, 28, 140, 48)
	img := cascadeImg()
	c.apply([]engine.PopupFrame{{ID: 1, Rect: r, Img: img}})

	c.apply([]engine.PopupFrame{fadeStep(1, img, r, 0.6, true)})
	c.apply([]engine.PopupFrame{fadeStep(1, img, r, 0.3, true)})
	if len(be.closed) != 0 {
		t.Fatalf("исчезающий попап закрыт посреди перехода: %v", be.closed)
	}
	if len(be.imgs) != 3 || be.imgs[2] == img || be.imgs[2].Pix[be.imgs[2].PixOffset(70, 15)+3] != 60 {
		t.Fatalf("шаги исчезновения не отданы копиями: кадров %d", len(be.imgs))
	}

	be.h.Move(1, 10, 2)
	if len(eng.moves) != 1 || eng.moves[0] != image.Pt(50, 30) {
		t.Errorf("ввод над исчезающим попапом: %v, ждали 50,30", eng.moves)
	}
	be.h.Done(1)
	if eng.closes != 0 {
		t.Errorf("popup_done исчезающего попапа закрыл оверлеи движка (%d)", eng.closes)
	}

	c.apply(nil)
	if len(be.closed) != 1 || be.closed[0] != 1 {
		t.Errorf("попап не закрыт, когда ID пропал: %v", be.closed)
	}
}

// Исчезающий оверлей, которого здесь не показывали, попапа не поднимает.
func TestChildPopups_ClosingUnknownIgnored(t *testing.T) {
	be := &fakeChildHost{}
	c := newChildPopups(be, &fakePopupEngine{}, 1, nil)
	c.apply([]engine.PopupFrame{fadeStep(1, cascadeImg(), image.Rect(0, 0, 100, 20), 0.5, true)})
	if len(be.calls) != 0 {
		t.Errorf("исчезающий незнакомый оверлей что-то затеял: %v", be.calls)
	}
}

// Новый попап, пока старый исчезает: исчезающий уходит сразу и до открытия
// нового. Иначе новый стал бы потомком исчезающего (тот держит захват
// ввода), и родитель, уйдя раньше потомка, разорвал бы соединение
// протокольной ошибкой. Досматривать исчезновение убранного попапа нечем;
// открытый снова, он поднимается заново.
func TestChildPopups_NewPopupDropsClosingFirst(t *testing.T) {
	be := &fakeChildHost{}
	c := newChildPopups(be, &fakePopupEngine{}, 1, nil)
	r1, r2 := image.Rect(0, 0, 100, 20), image.Rect(120, 0, 220, 20)
	img1, img2 := cascadeImg(), cascadeImg()
	c.apply([]engine.PopupFrame{{ID: 1, Rect: r1, Img: img1}})
	c.apply([]engine.PopupFrame{fadeStep(1, img1, r1, 0.8, true)})
	be.calls = nil

	c.apply([]engine.PopupFrame{fadeStep(1, img1, r1, 0.6, true), fadeStep(2, img2, r2, 0.2, false)})
	if fmt.Sprint(be.calls) != "[close 1 open 2 blit 2]" {
		t.Fatalf("порядок вызовов %v, ждали [close 1 open 2 blit 2]", be.calls)
	}

	// Следующие шаги исчезновения убранного попапа не трогают, конец — тоже.
	be.calls = nil
	c.apply([]engine.PopupFrame{fadeStep(1, img1, r1, 0.4, true), fadeStep(2, img2, r2, 0.5, false)})
	c.apply([]engine.PopupFrame{fadeStep(2, img2, r2, 0.8, false)})
	if fmt.Sprint(be.calls) != "[blit 2 blit 2]" {
		t.Fatalf("после ухода исчезающего: %v, ждали [blit 2 blit 2]", be.calls)
	}

	// Убранный раньше времени оверлей открыли снова — попап поднимается.
	be.calls = nil
	c.apply([]engine.PopupFrame{fadeStep(2, img2, r2, 0.8, false), {ID: 1, Rect: r1, Img: img1}})
	if fmt.Sprint(be.calls) != "[open 1 blit 1]" {
		t.Errorf("открытие снова: %v, ждали [open 1 blit 1]", be.calls)
	}
}

// Исчезающий оверлей снова открыт до конца перехода (мышь вернулась на пункт
// меню): тот же попап, без пересоздания, и popup_done о нём снова закрывает
// оверлеи.
func TestChildPopups_ClosingReopenedKeepsPopup(t *testing.T) {
	be := &fakeChildHost{}
	eng := &fakePopupEngine{}
	c := newChildPopups(be, eng, 1, nil)
	r := image.Rect(0, 0, 100, 20)
	img := cascadeImg()
	c.apply([]engine.PopupFrame{{ID: 1, Rect: r, Img: img}})
	c.apply([]engine.PopupFrame{fadeStep(1, img, r, 0.5, true)})
	c.apply([]engine.PopupFrame{fadeStep(1, img, r, 0.7, false)})
	if len(be.opened) != 1 || len(be.closed) != 0 {
		t.Fatalf("попап пересоздан: открыт %v, закрыт %v", be.opened, be.closed)
	}
	be.h.Done(1)
	if eng.closes != 1 {
		t.Errorf("popup_done открытого снова попапа не закрыл оверлеи (%d)", eng.closes)
	}
}
