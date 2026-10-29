//go:build linux && !android

package core

// wayland_monitor_change_linux.go — смена конфигурации мониторов на Wayland
// (описание — в monitor_change.go).
//
// Свою конфигурацию композитор сообщает на том же соединении, что и всё
// остальное, — отдельного подключения не нужно. Что считается переменой:
//
//	wl_registry.global (wl_output)        появился новый монитор — привязываем
//	                                      его на ходу и ждём его события;
//	wl_registry.global_remove (wl_output) монитор исчез — отпускаем объекты;
//	wl_output.geometry/mode/scale/done    монитор перенастроен;
//	zxdg_output_v1.logical_position/size  логические положение и размер.
//
// Событие только будит окно (notify); состав и значения читает уже общий слой
// (monitor_change.go): он берёт Monitors() и сравнивает со своим прежним
// списком, поэтому повторы здесь безвредны.

import (
	"encoding/binary"
	"sync/atomic"
)

const (
	wlRegistryEvGlobalRemove = 1 // wl_registry.global_remove(name)
	wlOutputRelease          = 0 // wl_output.release (с версии 3)
	xdgOutputDestroy         = 0 // zxdg_output_v1.destroy
	wlOutputVersionRelease   = 3
)

// wlMonitorWatch — подписка окна на смену мониторов.
type wlMonitorWatch struct {
	notify atomic.Pointer[func()]
}

// WatchMonitors подписывается на смену мониторов (monitorChangeSource).
func (w *WaylandWindow) WatchMonitors(notify func()) (stop func()) {
	if notify == nil {
		return nil
	}
	w.monWatch.notify.Store(&notify)
	return func() { w.monWatch.notify.Store(nil) }
}

// noteMonitorsChanged будит подписчика.
func (w *WaylandWindow) noteMonitorsChanged() {
	if fn := w.monWatch.notify.Load(); fn != nil {
		(*fn)()
	}
}

// outputGlobalAdded — композитор объявил новый wl_output уже после запуска окна.
func (w *WaylandWindow) outputGlobalAdded(name uint32) {
	if !w.outs.live.Load() {
		return // стартовая привязка ещё впереди (bindOutputs подхватит всё сразу)
	}
	w.outs.mu.Lock()
	for _, i := range w.outs.list {
		if i.global == name {
			w.outs.mu.Unlock()
			return
		}
	}
	w.outs.mu.Unlock()
	w.bindOneOutput(name)
	w.noteMonitorsChanged()
}

// outputGlobalRemoved — композитор убрал глобал (в том числе wl_output).
func (w *WaylandWindow) outputGlobalRemoved(name uint32) {
	w.outs.mu.Lock()
	idx := -1
	for k, i := range w.outs.list {
		if i.global == name {
			idx = k
			break
		}
	}
	if idx < 0 {
		w.outs.mu.Unlock()
		return
	}
	gone := w.outs.list[idx]
	w.outs.list = append(w.outs.list[:idx:idx], w.outs.list[idx+1:]...)
	w.outs.mu.Unlock()

	delete(w.gOutputs, name)
	w.scale.mu.Lock()
	delete(w.scale.outputs, gone.id)
	if w.scale.entered == gone.id {
		w.scale.entered = 0
	}
	w.scale.mu.Unlock()
	if gone.xdgID != 0 {
		w.send(newWlMsg(gone.xdgID, xdgOutputDestroy), -1)
	}
	// release есть только с версии 3: на более старом объекте запрос —
	// ошибка протокола, и композитор разорвал бы соединение.
	if gone.ver >= wlOutputVersionRelease {
		w.send(newWlMsg(gone.id, wlOutputRelease), -1)
	}
	w.noteMonitorsChanged()
}

// registryGlobalRemove разбирает wl_registry.global_remove.
func (w *WaylandWindow) registryGlobalRemove(b []byte) {
	if len(b) < 4 {
		return
	}
	w.outputGlobalRemoved(binary.LittleEndian.Uint32(b[0:4]))
}
