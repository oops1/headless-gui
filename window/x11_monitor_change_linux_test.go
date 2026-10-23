//go:build linux && !android

package window

import (
	"encoding/binary"
	"os"
	"testing"
)

// Смена мониторов X11: раскладка запросов подписки и разбор событий RandR —
// на проводе, плюс живая проверка против настоящего сервера.

// RRSelectInput: опкод расширения, минорный 4, окно и маска.
func TestX11MonitorChange_RequestLayout(t *testing.T) {
	r := x11RRSelectInputRequest(140, tRoot, randrScreenChangeNotifyMask|randrCrtcChangeNotifyMask|randrOutputChangeNotifyMask)
	if len(r) != 12 || r[0] != 140 || r[1] != 4 || binary.LittleEndian.Uint16(r[2:4]) != 3 {
		t.Fatalf("RRSelectInput: %v", r)
	}
	if binary.LittleEndian.Uint32(r[4:8]) != tRoot || binary.LittleEndian.Uint16(r[8:10]) != 7 {
		t.Errorf("окно/маска: %v", r[4:10])
	}

	p := x11SelectRootPropertyRequest(tRoot)
	if len(p) != 16 || p[0] != 2 || binary.LittleEndian.Uint16(p[2:4]) != 4 {
		t.Fatalf("ChangeWindowAttributes: %v", p)
	}
	if binary.LittleEndian.Uint32(p[8:12]) != 0x800 || binary.LittleEndian.Uint32(p[12:16]) != 0x00400000 {
		t.Errorf("маска значений/событий: %v", p[8:16])
	}
}

func x11MonChgPropEvent(atom uint32) []byte {
	b := make([]byte, 32)
	b[0] = x11EvPropertyNotify
	binary.LittleEndian.PutUint32(b[4:8], tRoot)
	binary.LittleEndian.PutUint32(b[8:12], atom)
	return b
}

// Событие относится к мониторам, если это ScreenChangeNotify, Notify RandR или
// PropertyNotify по _NET_WORKAREA; остальное будить не должно.
func TestX11MonitorChange_EventFilter(t *testing.T) {
	const first = 89 // первое событие RandR на сервере
	const workArea = 777
	ev := func(typ byte) []byte { b := make([]byte, 32); b[0] = typ; return b }

	cases := []struct {
		name string
		buf  []byte
		want bool
	}{
		{"ScreenChangeNotify", ev(first), true},
		{"Notify (CRTC/Output)", ev(first + 1), true},
		{"с признаком SendEvent", ev(first | 0x80), true},
		{"следующее событие расширения", ev(first + 2), false},
		{"MotionNotify", ev(6), false},
		{"PropertyNotify _NET_WORKAREA", x11MonChgPropEvent(workArea), true},
		{"PropertyNotify чужого свойства", x11MonChgPropEvent(5), false},
		{"обрезанный пакет", make([]byte, 8), false},
	}
	for _, c := range cases {
		if got := x11MonitorEvent(c.buf, first, workArea); got != c.want {
			t.Errorf("%s: %v, ждал %v", c.name, got, c.want)
		}
	}
	// Без RandR (firstEvent == 0) его «события» ничего не значат: тип 0 — это
	// ошибка протокола, а не уведомление.
	if x11MonitorEvent(ev(0), 0, workArea) {
		t.Error("ошибка протокола (тип 0) принята за ScreenChangeNotify")
	}
	// Без атома _NET_WORKAREA PropertyNotify не сопоставить.
	if x11MonitorEvent(x11MonChgPropEvent(0), first, 0) {
		t.Error("PropertyNotify принят при неизвестном атоме")
	}
}

// Живой сервер: наблюдатель поднимается и снимается без зависаний. Сменить
// конфигурацию мониторов из теста нечем — проверяется подключение и подписка.
func TestX11MonitorChange_LiveWatcherStartsAndStops(t *testing.T) {
	if os.Getenv("DISPLAY") == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}
	t.Setenv("HEADLESS_GUI_X11", "1")
	stop := x11WatchMonitors(func() {})
	if stop == nil {
		t.Skip("X-сервер недоступен или без RandR")
	}
	stop()
	stop() // повторное снятие не падает
}

func TestX11MonitorChange_NilNotify(t *testing.T) {
	if x11WatchMonitors(nil) != nil {
		t.Error("x11WatchMonitors(nil) должен вернуть nil")
	}
}
