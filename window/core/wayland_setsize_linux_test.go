//go:build linux && !android

package core

import (
	"testing"
	"time"
)

// wlSetSizeQuiet — срок, за который при отсутствии отправки ничего не придёт.
func wlSetSizeQuiet() time.Time { return time.Now().Add(50 * time.Millisecond) }

// E10: что окно Wayland делает по SetSize/SetPosition. Остальные методы
// (Minimize, Maximize, Restore, SetResizable) проверены в
// wayland_toplevel_linux_test.go.

// Окно с запретом ресайза фиксируется на НОВОМ размере: min == max == новый
// размер. Без этого композитор держал бы старые пределы, и новый буфер выходил
// бы за них.
func TestWayland_SetSize_FixedWindowMovesLimits(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.SetSize(1024, 768)
	if w, h := c.w.GetSize(); w != 1024 || h != 768 {
		t.Errorf("GetSize = %d×%d, ждал 1024×768", w, h)
	}
	msgs := splitWlMsgs(t, c.read(t))
	minM, okMin := findWlMsg(msgs, 12, xdgToplevelSetMinSize)
	maxM, okMax := findWlMsg(msgs, 12, xdgToplevelSetMaxSize)
	if !okMin || !okMax {
		t.Fatalf("пределы размера не переставлены: %+v", msgs)
	}
	if minM.args[0] != 1024 || minM.args[1] != 768 || maxM.args[0] != 1024 || maxM.args[1] != 768 {
		t.Errorf("min %v и max %v, ждал по 1024×768", minM.args, maxM.args)
	}
}

// Растяжимое окно размер не фиксирует: его определяет пользователь и
// компоновщик, а клиент лишь запоминает размер кадра.
func TestWayland_SetSize_ResizableWindowNotFixed(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.resizable.Store(true)
	c.w.SetSize(1024, 768)
	if w, h := c.w.GetSize(); w != 1024 || h != 768 {
		t.Errorf("GetSize = %d×%d, ждал 1024×768", w, h)
	}
	c.peer.SetReadDeadline(wlSetSizeQuiet())
	buf := make([]byte, 64)
	if n, _ := c.peer.Read(buf); n > 0 {
		t.Errorf("растяжимому окну отправлены пределы размера: %v", buf[:n])
	}
}

// Развёрнутое и полноэкранное окно размер не выбирают.
func TestWayland_SetSize_MaximizedAndFullscreenNotFixed(t *testing.T) {
	for _, name := range []string{"maximized", "fullscreen"} {
		c := newWlTestWindow(t)
		if name == "maximized" {
			c.w.maximized.Store(true)
		} else {
			c.w.fullscreen.Store(true)
		}
		c.w.SetSize(1024, 768)
		c.peer.SetReadDeadline(wlSetSizeQuiet())
		buf := make([]byte, 64)
		if n, _ := c.peer.Read(buf); n > 0 {
			t.Errorf("%s: отправлены пределы размера: %v", name, buf[:n])
		}
	}
}

// До создания toplevel (его нет) SetSize запоминает размер и ничего не шлёт.
func TestWayland_SetSize_BeforeCreate(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.toplevelID = 0
	c.w.SetSize(640, 480)
	if w, h := c.w.GetSize(); w != 640 || h != 480 {
		t.Errorf("GetSize = %d×%d, ждал 640×480", w, h)
	}
}

// Положение окна клиенту неизвестно и задать его нельзя: GetPosition всегда
// (0, 0), SetPosition ничего не отправляет композитору.
func TestWayland_SetPosition_IsNoOp(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.SetPosition(300, 200)
	if x, y := c.w.GetPosition(); x != 0 || y != 0 {
		t.Errorf("GetPosition = (%d,%d), ждал (0,0): положение окна клиенту не известно", x, y)
	}
	c.peer.SetReadDeadline(wlSetSizeQuiet())
	buf := make([]byte, 64)
	if n, _ := c.peer.Read(buf); n > 0 {
		t.Errorf("SetPosition отправил композитору %v: протокол этого не допускает", buf[:n])
	}
}
