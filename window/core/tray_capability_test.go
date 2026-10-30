package core

import (
	"errors"
	"image"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// trayWinFake — окно ОС с операциями трея: спрятать, вернуть, передний план.
type trayWinFake struct {
	*initFake
	hides, restores, fg int
}

func (f *trayWinFake) HideToTray()      { f.hides++ }
func (f *trayWinFake) RestoreFromTray() { f.restores++ }
func (f *trayWinFake) SetForeground()   { f.fg++ }

// created — окно ОС уже создано (Create в журнале бэкенда).
func (f *trayWinFake) created() bool {
	for _, e := range f.log {
		if e == "create" {
			return true
		}
	}
	return false
}

// fakeTray — трей возможности: журнал вызовов, колбэки для теста.
type fakeTray struct {
	log        []string
	native     bool // меню показывает система
	onClick    func(int, bool)
	onBalloon  func()
	runner     func(func())
	cursorAsks int
	closes     int
}

func (t *fakeTray) SetTrayIcon(image.Image, string) error {
	t.log = append(t.log, "icon")
	return nil
}
func (t *fakeTray) RemoveTrayIcon() { t.log = append(t.log, "remove") }
func (t *fakeTray) SetTrayClickHandler(fn func(int, bool)) {
	t.log = append(t.log, "click")
	t.onClick = fn
}
func (t *fakeTray) SetTrayMenuNative(*base.PopupMenu) bool {
	t.log = append(t.log, "menu")
	return t.native
}
func (t *fakeTray) SetTrayMenuRunner(run func(func())) {
	t.log = append(t.log, "runner")
	t.runner = run
}
func (t *fakeTray) CursorScreenPos() (int, int) { t.cursorAsks++; return 10, 10 }
func (t *fakeTray) ShowBalloon(string, string, uint32) error {
	t.log = append(t.log, "balloon")
	return nil
}
func (t *fakeTray) SetBalloonClickHandler(fn func()) {
	t.log = append(t.log, "balloonClick")
	t.onBalloon = fn
}
func (t *fakeTray) Close() { t.closes++ }

// trayBringUp поднимает окно на trayWinFake с синхронным движком (Post
// выполняется сразу).
func trayBringUp(t *testing.T, setup func(win *Window)) (*Window, *trayWinFake) {
	t.Helper()
	t.Setenv("HEADLESS_GUI_SCALE", "1")
	f := &trayWinFake{initFake: &initFake{}}
	old := newNativeWindow
	newNativeWindow = func() native.NativeWindow { return f }
	t.Cleanup(func() { newNativeWindow = old })
	win := New(syncEngine{engine.New(400, 300, 30)}, "tray")
	if setup != nil {
		setup(win)
	}
	if err := win.bringUp(); err != nil {
		t.Fatal(err)
	}
	return win, f
}

// Без возможности (window/minimal, тестовый бинарь ядра): до Run SetTrayIcon
// запоминает и отвечает nil, после — ErrTrayUnsupported; ShowBalloon — отказ;
// спрятать и вернуть окно работают — это операции окна.
func TestTray_NoCapability(t *testing.T) {
	if _, ok := native.TrayService(); ok {
		t.Skip("трей подключён в этом бинаре")
	}
	win, f := trayBringUp(t, func(win *Window) {
		if err := win.SetTrayIcon(image.NewRGBA(image.Rect(0, 0, 16, 16)), "x"); err != nil {
			t.Errorf("SetTrayIcon до Run без трея: %v", err)
		}
	})
	if err := win.SetTrayIcon(image.NewRGBA(image.Rect(0, 0, 16, 16)), "x"); !errors.Is(err, native.ErrTrayUnsupported) {
		t.Errorf("SetTrayIcon после Run без трея: %v", err)
	}
	if err := win.ShowBalloon("t", "x", widget.SeverityInfo); !errors.Is(err, native.ErrTrayUnsupported) {
		t.Errorf("ShowBalloon без трея: %v", err)
	}
	win.SetTrayMenu(widget.NewPopupMenu())
	win.RemoveTrayIcon()
	win.HideToTray()
	win.RestoreFromTray()
	if f.hides != 1 || f.restores != 1 {
		t.Errorf("спрятать и вернуть без трея: %d, %d", f.hides, f.restores)
	}
	win.tearDown()
}

// С возможностью: трей создаётся окну ОС после Create, отложенное состояние
// уходит в прежнем порядке (клик по уведомлению, диспетчер и очередь меню,
// меню, иконка последней), tearDown снимает трей ровно раз, а новый bringUp
// получает свой трей и отдаёт ему диспетчер заново.
func TestTray_Lifecycle(t *testing.T) {
	var trays []*fakeTray
	var got []native.NativeWindow
	t.Cleanup(native.RegisterTray(func(nw native.NativeWindow) native.Tray {
		got = append(got, nw)
		tr := &fakeTray{}
		trays = append(trays, tr)
		return tr
	}))
	setup := func(win *Window) {
		win.SetOnBalloonClick(func() {})
		win.SetTrayIcon(image.NewRGBA(image.Rect(0, 0, 16, 16)), "x")
		win.SetTrayMenu(widget.NewPopupMenu())
	}
	win, f := trayBringUp(t, setup)
	if len(trays) != 1 || got[0] != native.NativeWindow(f) {
		t.Fatalf("трей окну: %d, окно %v", len(trays), got)
	}
	if !f.created() {
		t.Error("трей создан до окна ОС")
	}
	want := []string{"balloonClick", "click", "runner", "menu", "icon"}
	if !reflect.DeepEqual(trays[0].log, want) {
		t.Errorf("порядок применения %v, ждал %v", trays[0].log, want)
	}
	if err := win.ShowBalloon("t", "x", widget.SeverityWarning); err != nil {
		t.Errorf("ShowBalloon: %v", err)
	}
	win.tearDown()
	if trays[0].closes != 1 {
		t.Fatalf("tearDown снял трей %d раз, ждал 1", trays[0].closes)
	}

	// То же окно запущено снова (повторный Run): трей новый, диспетчер и
	// очередь меню отдаются и ему.
	win.startTray()
	win.applyPendingTray()
	if len(trays) != 2 || trays[1].onClick == nil || trays[1].runner == nil {
		t.Fatalf("повторный запуск окна: треев %d, диспетчер нового трея отдан: %v", len(trays), len(trays) == 2 && trays[1].onClick != nil)
	}
	win.stopTray()

	win2, _ := trayBringUp(t, setup)
	if len(trays) != 3 || trays[2].onClick == nil || trays[2].runner == nil {
		t.Fatalf("второе окно: треев %d", len(trays))
	}
	win2.tearDown()
}

// Клики: двойной левый без колбэка приложения возвращает окно из трея; с
// колбэком — колбэк (на горутине движка); правый открывает наше меню у
// курсора с выводом окна на передний план, но не системное.
func TestTray_DispatchClick(t *testing.T) {
	tr := &fakeTray{}
	t.Cleanup(native.RegisterTray(func(native.NativeWindow) native.Tray { return tr }))
	menu := widget.NewPopupMenu()
	menu.AddItem("Пункт", func() {})
	win, f := trayBringUp(t, func(win *Window) {
		win.SetTrayIcon(image.NewRGBA(image.Rect(0, 0, 16, 16)), "x")
		win.SetTrayMenu(menu)
	})
	tr.onClick(0, true)
	if f.restores != 1 {
		t.Errorf("двойной левый без колбэка: восстановлений %d", f.restores)
	}
	tr.onClick(1, false)
	if tr.cursorAsks != 1 || f.fg != 1 {
		t.Errorf("правый клик: курсор спрошен %d раз, передний план %d", tr.cursorAsks, f.fg)
	}
	var clicks []widget.MouseButton
	win.SetOnTrayClick(func(b widget.MouseButton, dbl bool) { clicks = append(clicks, b) })
	tr.onClick(0, true)
	if len(clicks) != 1 || clicks[0] != widget.MouseLeft || f.restores != 1 {
		t.Errorf("с колбэком: клики %v, восстановлений %d", clicks, f.restores)
	}
	win.tearDown()

	// Меню показывает система: наше не открывается.
	tr2 := &fakeTray{native: true}
	t.Cleanup(native.RegisterTray(func(native.NativeWindow) native.Tray { return tr2 }))
	win2, _ := trayBringUp(t, func(win *Window) { win.SetTrayMenu(widget.NewPopupMenu()) })
	tr2.onClick(1, false)
	if tr2.cursorAsks != 0 {
		t.Error("при системном меню открыто наше")
	}
	win2.tearDown()
}
