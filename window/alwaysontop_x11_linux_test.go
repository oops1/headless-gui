//go:build linux && !android

package window

import (
	"encoding/binary"
	"testing"
)

const tAtomAbove = 0x99

// «Поверх всех» на X11 — ClientMessage _NET_WM_STATE с ADD/REMOVE и атомом
// _NET_WM_STATE_ABOVE, к корневому окну; итог — из свойства менеджера.
func TestX11_SetAlwaysOnTop_ClientMessage(t *testing.T) {
	for _, tc := range []struct {
		on     bool
		action uint32
	}{{true, 1}, {false, 0}} {
		w, server := newFsX11(t)
		w.atomWMStateAbove = tAtomAbove
		if !w.SetAlwaysOnTop(tc.on) {
			t.Fatalf("SetAlwaysOnTop(%v) вернул false", tc.on)
		}
		b := readN(t, server, 44)
		le := binary.LittleEndian
		msg := b[12:]
		if b[0] != 25 || le.Uint32(b[4:8]) != tRoot || msg[0] != 33 {
			t.Fatalf("не SendEvent ClientMessage к корню: %v", b[:13])
		}
		if le.Uint32(msg[8:12]) != tAtomState || le.Uint32(msg[12:16]) != tc.action || le.Uint32(msg[16:20]) != tAtomAbove {
			t.Errorf("on=%v: тип %d, действие %d, свойство %d", tc.on,
				le.Uint32(msg[8:12]), le.Uint32(msg[12:16]), le.Uint32(msg[16:20]))
		}
	}
	// Без атома (менеджер не знает EWMH) — честное «не умею».
	w, _ := newFsX11(t)
	if w.SetAlwaysOnTop(true) {
		t.Error("без _NET_WM_STATE_ABOVE SetAlwaysOnTop вернул true")
	}
}
