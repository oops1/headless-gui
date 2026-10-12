//go:build linux && !android

package window

// wayland_monitor_linux.go — мониторы на Wayland: wl_output (геометрия, режим,
// масштаб, имя) и zxdg_output_v1 (логические положение и размер), если
// композитор объявил zxdg_output_manager_v1.
//
// Окно и раньше привязывало wl_output ради масштаба (wayland_scale_linux.go);
// теперь события этих же объектов дополнительно складываются в wlOutputs.
// Мониторы без окна (window.Monitors) читает короткое отдельное подключение
// (queryMonitors).

import (
	"encoding/binary"
	"image"
	"net"
	"sort"
	"sync"
	"time"
)

const (
	// wl_output события.
	wlOutputEvGeometry    = 0
	wlOutputEvMode        = 1
	wlOutputEvName        = 4
	wlOutputEvDescription = 5

	// wl_output.mode flags: текущий режим.
	wlOutputModeCurrent = 1

	// Версия wl_output, с которой приходят name/description.
	wlOutputVersionName = 4

	// zxdg_output_manager_v1: запрос get_xdg_output(id, output).
	xdgOutMgrGetXdgOutput = 1
	// zxdg_output_v1 события.
	xdgOutEvLogicalPosition = 0
	xdgOutEvLogicalSize     = 1
	xdgOutEvName            = 3
	xdgOutEvDescription     = 4

	// Версия zxdg_output_manager_v1, на которую привязываемся: 3 — и
	// done приходит на wl_output; имена есть с 2-й.
	xdgOutVersion = 3
)

// wlOutputInfo — всё, что известно об одном wl_output.
type wlOutputInfo struct {
	global uint32 // имя глобала registry
	id     uint32 // объект wl_output
	xdgID  uint32 // объект zxdg_output_v1 (0 — нет)

	// wl_output.geometry / mode / scale / name / description.
	x, y         int32
	transform    int32
	haveGeometry bool
	modeW, modeH int32
	haveMode     bool
	scale        int32
	make, model  string
	name, desc   string

	// zxdg_output_v1.
	logX, logY  int32
	logW, logH  int32
	haveLogPos  bool
	haveLogSize bool
	xdgName     string
}

// wlOutputs — мониторы, о которых рассказал композитор. Пишет цикл событий,
// читает любая горутина (Monitors, MonitorOf).
type wlOutputs struct {
	mu   sync.Mutex
	list []*wlOutputInfo
}

func (o *wlOutputs) add(global, id uint32) *wlOutputInfo {
	o.mu.Lock()
	defer o.mu.Unlock()
	info := &wlOutputInfo{global: global, id: id, scale: 1}
	o.list = append(o.list, info)
	return info
}

// isOutput — объект wl_output одного из мониторов.
func (o *wlOutputs) isOutput(obj uint32) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, i := range o.list {
		if i.id == obj {
			return true
		}
	}
	return false
}

// isXdgOutput — объект zxdg_output_v1 одного из мониторов.
func (o *wlOutputs) isXdgOutput(obj uint32) bool {
	if obj == 0 {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, i := range o.list {
		if i.xdgID == obj {
			return true
		}
	}
	return false
}

// bindOutputs привязывает все объявленные wl_output (и их логическое
// описание, если есть zxdg_output_manager_v1). Порядок — по имени глобала:
// map обходится случайно, а список мониторов должен быть устойчив.
func (w *WaylandWindow) bindOutputs() {
	names := make([]uint32, 0, len(w.gOutputs))
	for name := range w.gOutputs {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })

	if w.gXdgOutMgr != 0 && w.xdgOutMgrID == 0 && len(names) > 0 {
		v := min(w.gXdgOutVer, xdgOutVersion)
		w.xdgOutMgrID = w.bind(w.gXdgOutMgr, "zxdg_output_manager_v1", max(v, 1))
	}
	for _, name := range names {
		// Масштаб приходит с версии 2, имя с 4: берём то, что есть, но не
		// выше четвёртой — новее нам незачем.
		v := min(w.gOutputs[name], wlOutputVersionName)
		id := w.bind(name, "wl_output", v)
		w.scale.mu.Lock()
		if w.scale.outputs == nil {
			w.scale.outputs = map[uint32]float64{}
		}
		w.scale.outputs[id] = 0 // масштаб придёт событием scale
		w.scale.mu.Unlock()

		info := w.outs.add(name, id)
		if w.xdgOutMgrID != 0 {
			xid := w.newID()
			w.outs.mu.Lock()
			info.xdgID = xid
			w.outs.mu.Unlock()
			w.send(newWlMsg(w.xdgOutMgrID, xdgOutMgrGetXdgOutput).putUint(xid).putUint(id), -1)
		}
	}
}

// handleOutputEvent разбирает событие wl_output.
func (w *WaylandWindow) handleOutputEvent(obj uint32, opcode uint16, b []byte) {
	w.outs.mu.Lock()
	var info *wlOutputInfo
	for _, i := range w.outs.list {
		if i.id == obj {
			info = i
			break
		}
	}
	if info == nil {
		w.outs.mu.Unlock()
		return
	}
	scale := float64(0)
	switch opcode {
	case wlOutputEvGeometry:
		if len(b) >= 20 {
			info.x = int32(binary.LittleEndian.Uint32(b[0:4]))
			info.y = int32(binary.LittleEndian.Uint32(b[4:8]))
			info.haveGeometry = true
			if mk, off, ok := wlReadString(b, 20); ok {
				if md, off2, ok := wlReadString(b, off); ok {
					info.make, info.model = mk, md
					if off2+4 <= len(b) {
						info.transform = int32(binary.LittleEndian.Uint32(b[off2 : off2+4]))
					}
				}
			}
		}
	case wlOutputEvMode:
		if len(b) >= 12 && binary.LittleEndian.Uint32(b[0:4])&wlOutputModeCurrent != 0 {
			info.modeW = int32(binary.LittleEndian.Uint32(b[4:8]))
			info.modeH = int32(binary.LittleEndian.Uint32(b[8:12]))
			info.haveMode = true
		}
	case wlOutputEvScale:
		if len(b) >= 4 {
			info.scale = max(int32(binary.LittleEndian.Uint32(b[0:4])), 1)
			scale = float64(info.scale)
		}
	case wlOutputEvName:
		if s, _, ok := wlReadString(b, 0); ok {
			info.name = s
		}
	case wlOutputEvDescription:
		if s, _, ok := wlReadString(b, 0); ok {
			info.desc = s
		}
	}
	w.outs.mu.Unlock()
	if scale > 0 {
		w.noteOutputScale(obj, scale) // масштаб поверхности (wayland_scale_linux.go)
	}
}

// handleXdgOutputEvent разбирает событие zxdg_output_v1.
func (w *WaylandWindow) handleXdgOutputEvent(obj uint32, opcode uint16, b []byte) {
	w.outs.mu.Lock()
	defer w.outs.mu.Unlock()
	var info *wlOutputInfo
	for _, i := range w.outs.list {
		if i.xdgID == obj {
			info = i
			break
		}
	}
	if info == nil {
		return
	}
	switch opcode {
	case xdgOutEvLogicalPosition:
		if len(b) >= 8 {
			info.logX = int32(binary.LittleEndian.Uint32(b[0:4]))
			info.logY = int32(binary.LittleEndian.Uint32(b[4:8]))
			info.haveLogPos = true
		}
	case xdgOutEvLogicalSize:
		if len(b) >= 8 {
			info.logW = int32(binary.LittleEndian.Uint32(b[0:4]))
			info.logH = int32(binary.LittleEndian.Uint32(b[4:8]))
			info.haveLogSize = true
		}
	case xdgOutEvName:
		if s, _, ok := wlReadString(b, 0); ok {
			info.xdgName = s
		}
	}
}

// wlReadString читает строку протокола (длина с NUL, байты, выравнивание до 4)
// с проверкой границ: чужое событие не должно ронять цикл паникой на
// обрезанном теле. Возвращает строку и смещение за ней.
func wlReadString(b []byte, off int) (string, int, bool) {
	if off < 0 || off+4 > len(b) {
		return "", 0, false
	}
	n := int(binary.LittleEndian.Uint32(b[off : off+4]))
	end := off + 4 + ((n + 3) &^ 3)
	if n < 0 || off+4+n > len(b) || end > len(b) {
		return "", 0, false
	}
	s := ""
	if n > 0 {
		s = string(b[off+4 : off+4+n-1])
	}
	return s, end, true
}

// monitor описывает wl_output как Monitor. ok=false — размер неизвестен.
//
// Границы — логические (в тех единицах, в которых компоновщик раскладывает окна):
// из xdg_output, а без него — положение из geometry и размер режима, делённый
// на масштаб (с учётом поворота). Масштаб — целый масштаб wl_output.
func (i *wlOutputInfo) monitor() (Monitor, bool) {
	scale := max(i.scale, 1)
	var b image.Rectangle
	switch {
	case i.haveLogSize:
		x, y := i.logX, i.logY
		if !i.haveLogPos && i.haveGeometry {
			x, y = i.x, i.y
		}
		b = image.Rect(int(x), int(y), int(x+i.logW), int(y+i.logH))
	case i.haveMode:
		w, h := i.modeW, i.modeH
		if i.transform&1 != 0 { // 90° и 270° (в том числе отражённые): ширина с высотой меняются
			w, h = h, w
		}
		b = image.Rect(int(i.x), int(i.y), int(i.x+w/scale), int(i.y+h/scale))
	}
	if b.Empty() {
		return Monitor{}, false
	}
	name := i.xdgName
	if name == "" {
		name = i.name
	}
	if name == "" {
		name = i.desc
	}
	if name == "" {
		name = trimJoin(i.make, i.model)
	}
	return Monitor{Name: name, Bounds: b, Work: b, Scale: float64(scale)}, true
}

func trimJoin(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " " + b
}

// monitors возвращает известные мониторы. Главным считается первый (в порядке
// глобалов): у wl_output понятия «основной» нет.
func (w *WaylandWindow) monitors() []Monitor {
	w.outs.mu.Lock()
	defer w.outs.mu.Unlock()
	var out []Monitor
	for _, i := range w.outs.list {
		if m, ok := i.monitor(); ok {
			m.Primary = len(out) == 0
			out = append(out, m)
		}
	}
	return out
}

// monitorOfWindow — монитор, на который компоновщик вывел окно
// (wl_surface.enter). Реализует monitorSource.
func (w *WaylandWindow) monitorOfWindow() (Monitor, bool) {
	w.scale.mu.Lock()
	entered := w.scale.entered
	w.scale.mu.Unlock()

	w.outs.mu.Lock()
	defer w.outs.mu.Unlock()
	for idx, i := range w.outs.list {
		if i.id != entered {
			continue
		}
		m, ok := i.monitor()
		if ok {
			m.Primary = idx == 0
		}
		return m, ok
	}
	// Окно ещё нигде не показано, а монитор один — это он.
	if len(w.outs.list) == 1 {
		m, ok := w.outs.list[0].monitor()
		m.Primary = true
		return m, ok
	}
	return Monitor{}, false
}

// wlQueryTimeout — сколько ждать композитор при опросе мониторов (переменная —
// чтобы тест не ждал секунды).
var wlQueryTimeout = 2 * time.Second

// queryMonitors опрашивает мониторы по отдельному подключению (окна нет) и
// закрывает его. Реализует window.Monitors на Wayland.
func (w *WaylandWindow) queryMonitors() []Monitor {
	defer func() {
		w.repeat.cancel()
		w.conn.Close()
	}()
	if uc, ok := net.Conn(w.conn).(*net.UnixConn); ok {
		uc.SetDeadline(time.Now().Add(wlQueryTimeout))
	}
	w.registryID = w.newID()
	if err := w.send(newWlMsg(wlDisplayID, wlDisplayGetRegistry).putUint(w.registryID), -1); err != nil {
		return nil
	}
	if err := w.roundtrip(); err != nil { // глобалы
		return nil
	}
	w.bindOutputs()
	if err := w.roundtrip(); err != nil { // события выводов
		return nil
	}
	return w.monitors()
}
