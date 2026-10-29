//go:build linux && !android

package core

import "testing"

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
