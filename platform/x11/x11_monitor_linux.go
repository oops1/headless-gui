//go:build linux && !android

package x11

// x11_monitor_linux.go — мониторы на X11 (RandR 1.5, RRGetMonitors).
//
// Запросы идут по ОТДЕЛЬНОМУ короткому соединению: на соединении окна ответы
// читает цикл событий, и запрос-ответ из чужой горутины разделил бы с ним поток
// (см. комментарий к posMu о GetPosition). Пока соединение новое и событий на
// нём нет, ответ читается напрямую.
//
// RandR старше 1.5 (нет RRGetMonitors) и сервер без RandR честно сводятся к
// одному монитору — корневому окну.

import (
	"encoding/binary"
	"image"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// Минорные опкоды RandR.
const (
	randrQueryVersion = 0
	randrGetMonitors  = 42
)

// EnumMonitors — мониторы X11 по новому соединению.
func EnumMonitors() []native.Monitor {
	w := &X11Window{}
	if err := w.connect(); err != nil {
		return nil
	}
	defer w.conn.Close()
	return w.x11Monitors()
}

// x11Monitors опрашивает мониторы по соединению w (событий на нём нет).
func (w *X11Window) x11Monitors() []native.Monitor {
	scale := w.DetectScale()
	if scale <= 0 {
		scale = 1
	}
	var out []native.Monitor
	if major, ok := w.x11ExtensionMajor("RANDR"); ok && w.x11RandrAtLeast15(major) {
		if reply, ok := w.x11Call(x11RRGetMonitorsRequest(major, w.rootWin)); ok {
			for i, rm := range parseRRMonitors(reply) {
				out = append(out, native.MonitorWithKey(native.Monitor{
					Name:    w.x11AtomName(rm.name),
					Bounds:  rm.bounds,
					Work:    rm.bounds,
					Scale:   scale,
					Primary: rm.primary,
				}, uint64(i+1))) // ключ — индекс для _NET_WM_FULLSCREEN_MONITORS плюс один
			}
		}
	}
	if len(out) == 0 {
		// Без RandR 1.5: весь экран сервера — один монитор.
		r := image.Rect(0, 0, int(w.screen.WidthInPixels), int(w.screen.HeightInPixels))
		out = []native.Monitor{native.MonitorWithKey(native.Monitor{Bounds: r, Work: r, Scale: scale, Primary: true}, 1)}
	}
	if !hasPrimaryMonitor(out) {
		out[0].Primary = true
	}
	// Рабочая область: _NET_WORKAREA — одна на всю область экрана, а не на
	// монитор, поэтому монитору достаётся её пересечение с ним. Для панели на
	// краю одного из мониторов это точно; для нескольких разных панелей —
	// приближение (EWMH на монитор рабочую область не даёт).
	if wa, ok := w.x11WorkArea(); ok {
		for i := range out {
			if in := out[i].Bounds.Intersect(wa); !in.Empty() {
				out[i].Work = in
			}
		}
	}
	return out
}

func hasPrimaryMonitor(list []native.Monitor) bool {
	for _, m := range list {
		if m.Primary {
			return true
		}
	}
	return false
}

// x11Call отправляет запрос и читает ответ напрямую: тело ответа — 32 байта
// заголовка плюс length*4 байт. ok=false — ошибка сервера или соединения.
func (w *X11Window) x11Call(req []byte) ([]byte, bool) {
	w.x11Send(req)
	hdr := make([]byte, 32)
	if !w.readFull(hdr) || hdr[0] != 1 {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(hdr[4:8])) * 4
	if n == 0 {
		return hdr, true
	}
	if n > 1<<20 { // неправдоподобный ответ — не выделяем
		return nil, false
	}
	extra := make([]byte, n)
	if !w.readFull(extra) {
		return nil, false
	}
	return append(hdr, extra...), true
}

// x11ExtensionMajor — major-опкод расширения (QueryExtension).
func (w *X11Window) x11ExtensionMajor(name string) (byte, bool) {
	reply, ok := w.x11Call(x11QueryExtensionRequest(name))
	if !ok {
		return 0, false
	}
	major, _, present := parseQueryExtensionReply(reply)
	return major, present
}

// x11RandrAtLeast15 — сервер говорит по RandR не старше 1.5 (до неё нет
// RRGetMonitors).
func (w *X11Window) x11RandrAtLeast15(major byte) bool {
	req := make([]byte, 12)
	req[0], req[1] = major, randrQueryVersion
	binary.LittleEndian.PutUint16(req[2:4], 3)
	binary.LittleEndian.PutUint32(req[4:8], 1) // версия клиента 1.5
	binary.LittleEndian.PutUint32(req[8:12], 5)
	reply, ok := w.x11Call(req)
	if !ok || len(reply) < 16 {
		return false
	}
	maj := binary.LittleEndian.Uint32(reply[8:12])
	mnr := binary.LittleEndian.Uint32(reply[12:16])
	return maj > 1 || (maj == 1 && mnr >= 5)
}

// x11RRGetMonitorsRequest — RRGetMonitors (get_active = истина: только
// включённые мониторы).
func x11RRGetMonitorsRequest(major byte, root uint32) []byte {
	req := make([]byte, 12)
	req[0], req[1] = major, randrGetMonitors
	binary.LittleEndian.PutUint16(req[2:4], 3)
	binary.LittleEndian.PutUint32(req[4:8], root)
	req[8] = 1
	return req
}

// rrMonitor — один MONITORINFO из ответа RRGetMonitors.
type rrMonitor struct {
	name    uint32 // атом имени
	primary bool
	bounds  image.Rectangle
}

// parseRRMonitors разбирает ответ RRGetMonitors целиком (заголовок и тело):
// число мониторов @12, дальше записи по 24 байта и по 4 байта на каждый
// вывод. Обрезанный ответ читается до конца, а не паникует.
func parseRRMonitors(reply []byte) []rrMonitor {
	if len(reply) < 32 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(reply[12:16]))
	var out []rrMonitor
	off := 32
	for i := 0; i < n; i++ {
		if off+24 > len(reply) {
			break
		}
		b := reply[off:]
		nOut := int(binary.LittleEndian.Uint16(b[6:8]))
		x := int(int16(binary.LittleEndian.Uint16(b[8:10])))
		y := int(int16(binary.LittleEndian.Uint16(b[10:12])))
		w := int(binary.LittleEndian.Uint16(b[12:14]))
		h := int(binary.LittleEndian.Uint16(b[14:16]))
		out = append(out, rrMonitor{
			name:    binary.LittleEndian.Uint32(b[0:4]),
			primary: b[4] != 0,
			bounds:  image.Rect(x, y, x+w, y+h),
		})
		off += 24 + nOut*4
	}
	return out
}

// x11AtomName — имя атома (GetAtomName); пусто при ошибке.
func (w *X11Window) x11AtomName(atom uint32) string {
	if atom == 0 {
		return ""
	}
	req := make([]byte, 8)
	req[0] = 17 // GetAtomName
	binary.LittleEndian.PutUint16(req[2:4], 2)
	binary.LittleEndian.PutUint32(req[4:8], atom)
	reply, ok := w.x11Call(req)
	if !ok || len(reply) < 32 {
		return ""
	}
	n := int(binary.LittleEndian.Uint16(reply[8:10]))
	if 32+n > len(reply) {
		return ""
	}
	return string(reply[32 : 32+n])
}

// x11WorkArea — рабочая область первого рабочего стола (_NET_WORKAREA).
func (w *X11Window) x11WorkArea() (image.Rectangle, bool) {
	atom := w.x11InternAtom("_NET_WORKAREA")
	if atom == 0 {
		return image.Rectangle{}, false
	}
	_, format, data := w.x11GetProperty(w.rootWin, atom, false)
	if format != 32 || len(data) < 16 {
		return image.Rectangle{}, false
	}
	x := int(int32(binary.LittleEndian.Uint32(data[0:4])))
	y := int(int32(binary.LittleEndian.Uint32(data[4:8])))
	ww := int(binary.LittleEndian.Uint32(data[8:12]))
	hh := int(binary.LittleEndian.Uint32(data[12:16]))
	r := image.Rect(x, y, x+ww, y+hh)
	return r, !r.Empty()
}
