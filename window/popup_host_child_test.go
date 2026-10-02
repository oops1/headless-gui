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
	closed  []uintptr
	h       childPopupHandlers
}

func (f *fakeChildHost) OpenChildPopup(id uintptr, x, y, w, h int) error {
	f.opened = append(f.opened, popupCall(id, x, y, w, h))
	return nil
}

func (f *fakeChildHost) MoveChildPopup(id uintptr, x, y, w, h int) {
	f.moved = append(f.moved, popupCall(id, x, y, w, h))
}

func (f *fakeChildHost) BlitChildPopup(id uintptr, img *image.RGBA, bands []image.Rectangle) {
	f.blitted = append(f.blitted, id)
}

func (f *fakeChildHost) CloseChildPopup(id uintptr)                 { f.closed = append(f.closed, id) }
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
