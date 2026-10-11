//go:build linux && !android

package window

// wayland_drag_linux.go — окно как ИСТОЧНИК перетаскивания файлов (v3.34):
// wl_data_device.start_drag с wl_data_source, который предлагает
// text/uri-list.
//
// Порядок: создаём источник, объявляем тип и допустимые действия (v≥3),
// отдаём компоновщику вместе с serial'ом нажатия кнопки — тот доказывает, что
// перетаскивание начал пользователь. Дальше ведёт компоновщик: рисует курсор,
// выбирает приёмник, а у нас просит данные событием send (в нём file
// descriptor, в который пишем список). Тело списка собирается лениво, в
// момент send, и пишется в своей горутине: приёмник читает не торопясь, а
// запись в канал блокируется.
//
// Конец перетаскивания:
//   - v≥3: dnd_finished — приёмник забрал данные; итоговое действие пришло
//     событием action. cancelled — отказ или отмена (Esc, сброс не на окно);
//
// Нужен wl_data_device_manager v≥3 (set_actions, dnd_finished): без них
// окончание перетаскивания не узнать — приёмник может запрашивать данные
// ещё на лету, и по первому send итог не определить. На более старом
// компоновщике BeginDragFiles возвращает false.
//
// Чего нет: значка перетаскиваемого (start_drag без icon — компоновщик
// показывает обычный курсор); действие «ссылка» (в протоколе его нет:
// ask — это «спросить пользователя», а не link); перетаскивание с попапа.

import (
	"encoding/binary"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	// wl_data_device
	wlDataDeviceStartDrag = 0

	// wl_data_source
	wlDataSourceSetActions        = 2
	wlDataSourceEvDndDropPerformed = 3
	wlDataSourceEvDndFinished     = 4
	wlDataSourceEvAction          = 5

	// wl_data_device_manager.dnd_action (copy — wlDndActionCopy в native_wayland.go)
	wlDndActionMove = 2
)

// wlDragSource — перетаскивание, начатое окном. Нулевое значение — перетаскивания нет.
type wlDragSource struct {
	mu       sync.Mutex
	id       uint32 // wl_data_source; 0 — перетаскивания нет
	body     string // text/uri-list
	done     func(DragEffect)
	action   DragEffect // действие, о котором сообщил компоновщик
	dropped  bool       // кнопку отпустили над приёмником (dnd_drop_performed)
}

// is сообщает, наш ли это wl_data_source (для разбора событий).
func (d *wlDragSource) is(id uint32) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return id != 0 && d.id == id
}

// wlActionsFor переводит допустимые действия в маску wl_data_device_manager.
// Link в протоколе нет: он отбрасывается.
func wlActionsFor(effect DragEffect) uint32 {
	var m uint32
	if effect&DragEffectCopy != 0 {
		m |= wlDndActionCopy
	}
	if effect&DragEffectMove != 0 {
		m |= wlDndActionMove
	}
	return m
}

// wlEffectFromAction — обратный перевод итогового действия компоновщика.
func wlEffectFromAction(a uint32) DragEffect {
	switch a {
	case wlDndActionCopy:
		return DragEffectCopy
	case wlDndActionMove:
		return DragEffectMove
	}
	return 0
}

// BeginDragFiles начинает перетаскивание файлов (fileDragSource):
// wl_data_device.start_drag. Нужны: wl_data_device, зажатая кнопка мыши и её
// serial; фокус указателя — на самом окне, не на попапе.
func (w *WaylandWindow) BeginDragFiles(paths []string, effect DragEffect, done func(DragEffect)) bool {
	actions := wlActionsFor(effect)
	serial := w.ptrButtonSerial.Load()
	if len(paths) == 0 || actions == 0 || done == nil {
		return false
	}
	if w.dataDevMgrID == 0 || w.dataDeviceID == 0 || w.surfaceID == 0 || w.dataDevVersion < 3 || serial == 0 ||
		w.ptrButtons.Load() == 0 || w.ptrOnPopup {
		return false // нечем доказать, что перетаскивание начал пользователь
	}
	d := &w.drag
	d.mu.Lock()
	if d.id != 0 {
		d.mu.Unlock()
		return false // одно перетаскивание за раз
	}
	id := w.newID()
	d.id, d.body, d.done = id, buildURIList(paths), done
	d.action, d.dropped = 0, false
	d.mu.Unlock()

	w.send(newWlMsg(w.dataDevMgrID, wlDataDevMgrCreateDataSource).putUint(id), -1)
	w.send(newWlMsg(id, wlDataSourceOffer).putString(mimeTextUriList), -1)
	w.send(newWlMsg(id, wlDataSourceSetActions).putUint(actions), -1)
	// start_drag(source, origin, icon=nil, serial)
	w.send(newWlMsg(w.dataDeviceID, wlDataDeviceStartDrag).
		putUint(id).putUint(w.surfaceID).putUint(0).putUint(serial), -1)
	// Нажатие ушло компоновщику, отпускания мы не получим (как после
	// xdg_toplevel.move): снимаем кнопки в движке сами.
	w.releaseHeldButtons()
	return true
}

// handleDragSourceEvent разбирает события нашего wl_data_source.
func (w *WaylandWindow) handleDragSourceEvent(obj uint32, opcode uint16, b []byte) {
	d := &w.drag
	switch opcode {
	case wlDataSourceEvSend:
		mime, _ := wlString(b, 0)
		w.dragHandleSend(mime, w.takeFD())

	case wlDataSourceEvCancelled:
		w.dragFinish(obj, 0)

	case wlDataSourceEvDndDropPerformed:
		d.mu.Lock()
		d.dropped = true
		d.mu.Unlock()

	case wlDataSourceEvAction:
		if len(b) >= 4 {
			act := wlEffectFromAction(binary.LittleEndian.Uint32(b[0:4]))
			d.mu.Lock()
			d.action = act
			d.mu.Unlock()
		}

	case wlDataSourceEvDndFinished:
		d.mu.Lock()
		act := d.action
		d.mu.Unlock()
		w.dragFinish(obj, act)
	}
}

// dragHandleSend отдаёт запросившему список файлов: компоновщик передал fd, в
// который нужно записать данные запрошенного типа. На чужой тип fd просто
// закрывается — приёмник получит пустоту.
func (w *WaylandWindow) dragHandleSend(mime string, fd int) {
	d := &w.drag
	d.mu.Lock()
	body := d.body
	id := d.id
	d.mu.Unlock()
	if fd < 0 {
		return
	}
	f := os.NewFile(uintptr(fd), "wl-drag-send")
	if mime != mimeTextUriList || id == 0 {
		f.Close()
		return
	}
	go func() {
		defer f.Close()
		io.Copy(f, strings.NewReader(body))
	}()
}

// dragFinish заканчивает перетаскивание: источник уничтожается, приложению
// сообщается итог (0 — отмена). Повторный вызов — no-op.
func (w *WaylandWindow) dragFinish(src uint32, effect DragEffect) {
	d := &w.drag
	d.mu.Lock()
	if d.id == 0 || d.id != src {
		d.mu.Unlock()
		return
	}
	done := d.done
	d.id, d.body, d.done = 0, "", nil
	d.mu.Unlock()
	w.send(newWlMsg(src, wlDataSourceDestroy), -1)
	if done != nil {
		done(effect)
	}
}
