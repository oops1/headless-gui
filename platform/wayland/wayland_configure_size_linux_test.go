//go:build linux && !android

package wayland

import (
	"image"
	"testing"
)

// Размер из configure для плавающего окна — подсказка: окно держит пределы,
// которые само назначило (floatingSize). Отчёт RDP-клиента на v3.43.2:
// компоновщик WinLine прислал устаревший 1400×860 окну, зафиксированному на
// 520×370, и окно выросло обратно после щелчка по рабочему столу.

// Окно с запретом ресайза: configure с чужим размером не меняет ни размер, ни
// холст (onResize не зовётся); буфер остаётся закреплённого размера.
func TestWayland_ConfigureSize_FixedWindowKeepsPin(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(false)
	c.w.SetSize(520, 370)
	*calls = nil

	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1400, 860, xdgStateActivated))
	if w, h := c.w.GetSize(); w != 520 || h != 370 {
		t.Errorf("GetSize = %dx%d после устаревшего configure, ждал закреплённые 520x370", w, h)
	}
	if len(*calls) != 0 {
		t.Errorf("onResize %v — холст перестроен под чужой размер", *calls)
	}
}

// W9 на уровне протокола. Окну, зафиксированному на 520×370, компоновщик
// присылает устаревший configure 1400×860 и xdg_surface.configure. Окно
// подтверждает serial (ack_configure), пределов не переотправляет, нового
// буфера не заводит и отвечает своим размером: после ack — commit последнего
// кадра 520×370, следующий кадр движка — того же размера.
func TestWayland_ConfigureSize_FixedWindowAnswersWithOwnSize(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(false)
	c.w.SetSize(520, 370)
	c.w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 520, 370)))
	c.readAllWl(t)
	*calls = nil

	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1400, 860, xdgStateActivated))
	c.w.handleEvent(c.w.xdgSurfaceID, xdgSurfaceEvConfigure, wlU32(77))
	msgs := c.readAllWl(t)

	ack, ok := findWlMsg(msgs, c.w.xdgSurfaceID, xdgSurfaceAckConfigure)
	if !ok || len(ack.args) != 1 || ack.args[0] != 77 {
		t.Fatalf("ack_configure(77) не отправлен: %+v", msgs)
	}
	if mins, maxs := wlSzLimits(t, msgs); len(mins) != 0 || len(maxs) != 0 {
		t.Errorf("пределы переотправлены: min %v max %v", mins, maxs)
	}
	for _, m := range msgs {
		if m.obj == c.w.poolID && m.opcode == wlShmPoolCreateBuffer {
			t.Errorf("заведён буфер %v под чужой размер", m.args)
		}
	}
	// Ответ своим размером: после ack — attach уже заведённого буфера (кадр
	// 520×370), damage в его пределах и commit.
	iAck, iAttach, iCommit := -1, -1, -1
	for i, m := range msgs {
		switch {
		case m.obj == c.w.xdgSurfaceID && m.opcode == xdgSurfaceAckConfigure:
			iAck = i
		case iAck >= 0 && iAttach < 0 && m.obj == c.w.surfaceID && m.opcode == wlSurfaceAttach:
			iAttach = i
			if b := m.args[0]; b == 0 || (b != c.w.bufID[0] && b != c.w.bufID[1]) {
				t.Errorf("после ack прикреплён буфер %d, ждал заведённый %v", b, c.w.bufID)
			}
		case iAttach >= 0 && iCommit < 0 && m.obj == c.w.surfaceID && m.opcode == wlSurfaceDamage:
			if x, y, w, h := int32(m.args[0]), int32(m.args[1]), int32(m.args[2]), int32(m.args[3]); x < 0 || y < 0 || x+w > 520 || y+h > 370 {
				t.Errorf("damage %d,%d %dx%d выходит за кадр 520x370", x, y, w, h)
			}
		case iAttach >= 0 && iCommit < 0 && m.obj == c.w.surfaceID && m.opcode == wlSurfaceCommit:
			iCommit = i
		}
	}
	if iAttach < 0 || iCommit < 0 {
		t.Errorf("после ack_configure нет attach+commit своего кадра (ack %d, attach %d, commit %d): компоновщик не получит ответа своим размером", iAck, iAttach, iCommit)
	}
	if c.w.poolW != 520 || c.w.poolH != 370 {
		t.Errorf("буфер %dx%d, ждал закреплённый 520x370", c.w.poolW, c.w.poolH)
	}
	if len(*calls) != 0 {
		t.Errorf("onResize %v — холст перестроен под чужой размер", *calls)
	}

	// Движок рисует следующий кадр по-прежнему 520×370 — буфер тот же.
	c.w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 520, 370)))
	if c.w.poolW != 520 || c.w.poolH != 370 {
		t.Errorf("после кадра буфер %dx%d, ждал 520x370", c.w.poolW, c.w.poolH)
	}
}

// Растяжимое окно принимает размер из configure, но не меньше SetMinSize.
func TestWayland_ConfigureSize_ResizableRespectsMin(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(true)
	c.w.SetMinSize(960, 600)

	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(520, 370, xdgStateActivated))
	if w, h := c.w.GetSize(); w != 960 || h != 600 {
		t.Errorf("GetSize = %dx%d, ждал минимум 960x600", w, h)
	}
	if len(*calls) != 1 || (*calls)[0] != [2]int{960, 600} {
		t.Errorf("onResize %v, ждал один вызов 960x600", *calls)
	}

	// Выше минимума — как назвал компоновщик (верхнего предела нет).
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1600, 1000, xdgStateActivated))
	if w, h := c.w.GetSize(); w != 1600 || h != 1000 {
		t.Errorf("GetSize = %dx%d, ждал 1600x1000", w, h)
	}
}

// Развёрнутое и полноэкранное окно размер не выбирает: configure применяется
// как есть, даже у окна с запретом ресайза.
func TestWayland_ConfigureSize_ConstrainedFollowsCompositor(t *testing.T) {
	for _, st := range []uint32{xdgStateMaximized, xdgStateFullscreen} {
		c, _ := wlSzWindow(t)
		c.w.resizable.Store(false)
		c.w.SetSize(520, 370)
		c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, st, xdgStateActivated))
		if w, h := c.w.GetSize(); w != 1920 || h != 1080 {
			t.Errorf("состояние %d: GetSize = %dx%d, ждал размер компоновщика 1920x1080", st, w, h)
		}
	}
}

// Restore развёрнутого окна с запретом ресайза фиксирует его на размере
// прежней фиксации, а не на развёрнутом; configure после выхода из
// развёрнутого состояния его не перебивает.
func TestWayland_ConfigureSize_RestoreReturnsToPin(t *testing.T) {
	c, _ := wlSzWindow(t)
	c.w.resizable.Store(false)
	c.w.SetSize(520, 370)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateMaximized, xdgStateActivated))
	c.readAllWl(t)

	c.w.Restore()
	mins, maxs := wlSzLimits(t, c.readAllWl(t))
	if len(mins) != 1 || mins[0] != [2]uint32{520, 370} || len(maxs) != 1 || maxs[0] != [2]uint32{520, 370} {
		t.Errorf("Restore: пределы min %v max %v, ждал по 520x370", mins, maxs)
	}
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateActivated))
	if w, h := c.w.GetSize(); w != 520 || h != 370 {
		t.Errorf("после Restore GetSize = %dx%d, ждал 520x370", w, h)
	}
}
