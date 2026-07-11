//go:build linux && !android

package window

import (
	"encoding/binary"
	"testing"
)

// makeConfigureNotify собирает 32-байтовое событие ConfigureNotify с заданными
// x,y и флагом synthetic (send_event — старший бит кода события).
func makeConfigureNotify(x, y int16, synthetic bool) []byte {
	buf := make([]byte, 32)
	buf[0] = 22 // ConfigureNotify
	if synthetic {
		buf[0] |= 0x80
	}
	binary.LittleEndian.PutUint16(buf[16:18], uint16(x))
	binary.LittleEndian.PutUint16(buf[18:20], uint16(y))
	return buf
}

func TestParseConfigureNotifyPos(t *testing.T) {
	// Отрицательные координаты обязаны читаться как INT16 (окно частично за
	// краем экрана — иначе получим ~65k вместо -30).
	buf := makeConfigureNotify(-30, 700, true)
	x, y, synthetic := parseConfigureNotifyPos(buf)
	if x != -30 || y != 700 {
		t.Fatalf("координаты: got (%d,%d), want (-30,700)", x, y)
	}
	if !synthetic {
		t.Fatalf("synthetic не распознан у send_event-события")
	}

	x, y, synthetic = parseConfigureNotifyPos(makeConfigureNotify(100, 200, false))
	if x != 100 || y != 200 || synthetic {
		t.Fatalf("real-событие: got (%d,%d,synthetic=%v), want (100,200,false)", x, y, synthetic)
	}
}

// Под reparenting-WM real-координаты (относительно рамки) не должны затирать
// достоверные synthetic-координаты (root-relative).
func TestGetPositionPrefersSynthetic(t *testing.T) {
	w := &X11Window{}

	// Пока кэш пуст — (0,0).
	if x, y := w.GetPosition(); x != 0 || y != 0 {
		t.Fatalf("пустой кэш: got (%d,%d), want (0,0)", x, y)
	}

	// Real-событие до synthetic — принимается (сценарий «без WM»).
	w.updatePosFromConfigure(50, 60, false)
	if x, y := w.GetPosition(); x != 50 || y != 60 {
		t.Fatalf("real до synthetic: got (%d,%d), want (50,60)", x, y)
	}

	// Synthetic от WM — корневые координаты, перекрывают.
	w.updatePosFromConfigure(800, 400, true)
	if x, y := w.GetPosition(); x != 800 || y != 400 {
		t.Fatalf("synthetic: got (%d,%d), want (800,400)", x, y)
	}

	// Последующее real-событие (parent-relative под рамкой) игнорируется.
	w.updatePosFromConfigure(0, 0, false)
	if x, y := w.GetPosition(); x != 800 || y != 400 {
		t.Fatalf("real после synthetic не должен затирать: got (%d,%d), want (800,400)", x, y)
	}
}

// SetPosition оптимистично обновляет кэш — GetPosition сразу после него отдаёт
// свежее значение (плавный drag не ждёт ConfigureNotify).
func TestSetPositionUpdatesCache(t *testing.T) {
	w := &X11Window{wid: 0} // wid==0 → x11MoveWindow не шлётся, но кэш и не тронется
	w.posMu.Lock()
	w.posX, w.posY, w.posCached = 10, 20, true
	w.posMu.Unlock()

	// wid==0: ранний выход, кэш не меняется.
	w.SetPosition(999, 999)
	if x, y := w.GetPosition(); x != 10 || y != 20 {
		t.Fatalf("wid==0 не должен менять кэш: got (%d,%d), want (10,20)", x, y)
	}
}
