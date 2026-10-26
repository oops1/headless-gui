//go:build linux && !android

package window

// wayland_fullscreen_linux.go — полный экран на Wayland (xdg_toplevel).
//
// Клиент лишь ПРОСИТ: set_fullscreen(output) / unset_fullscreen. Что решил
// компоновщик, видно из следующего configure: в списке состояний есть
// fullscreen — окно на весь экран; нет — компоновщик отказал (или пользователь
// вышел его собственными средствами). Протокол отказа не имеет: отказ — это
// configure без состояния (так отвечает WinLine без WINLINE_FULLSCREEN), и
// различить его можно только по тому, что запрос был и не исполнен.
//
// Поэтому в окне есть флаг «запрос в пути» (fs.pending): configure без
// fullscreen при взведённом флаге — отказ, и приложение получает
// SetOnFullscreenChanged(false), хотя состояние не менялось. Все остальные
// смены (в полный экран, из него — в том числе по воле компоновщика)
// приходят как обычные уведомления об изменении.

import (
	"sync/atomic"
)

// wlFullscreen — служебное состояние запроса полного экрана.
type wlFullscreen struct {
	// pending — запрос set_fullscreen отправлен, ответа (configure) ещё не
	// было.
	pending atomic.Bool
	// onChange — уведомление приложения; вызывается из цикла событий.
	onChange atomic.Pointer[func(bool)]
	// saveW/saveH — размер окна (пиксели буфера) до полного экрана. Окно с
	// запретом ресайза фиксируется на этом размере при выходе: к тому
	// времени w.width уже размер экрана.
	saveW, saveH atomic.Int32
}

// SetFullscreen просит компоновщик развернуть окно на весь экран (output не
// называем: выбор монитора — за компоновщиком) или вернуть обратно. Реализует
// fullscreenBackend.
//
// true — запрос отправлен; результат придёт в configure. Компоновщик вправе
// отказать — тогда SetOnFullscreenChanged получит false.
func (w *WaylandWindow) SetFullscreen(on bool) bool {
	return w.setFullscreen(on, 0)
}

// SetFullscreenOn просит полный экран на мониторе m (fullscreenOnBackend):
// xdg_toplevel.set_fullscreen(wl_output) с выводом этого же соединения,
// который соответствует m. Monitor из window.Monitors пришёл с ДРУГОГО
// подключения (там свои номера объектов), поэтому сопоставление идёт по
// имени глобала registry (ключ монитора), а затем по имени и границам.
//
// Окно, уже стоящее в полном экране, получает новый set_fullscreen с другим
// выводом — протокол разрешает переносить так, без выхода из режима.
// Композитор вправе выбрать другой вывод; где окно оказалось на самом деле,
// видно из MonitorOf. false — вывода нет среди известных (отключён).
func (w *WaylandWindow) SetFullscreenOn(m Monitor) bool {
	out, ok := w.outputObjectFor(m)
	if !ok {
		return false
	}
	return w.setFullscreen(true, out)
}

// outputObjectFor находит объект wl_output этого соединения для монитора m.
func (w *WaylandWindow) outputObjectFor(m Monitor) (uint32, bool) {
	w.outs.mu.Lock()
	var list []Monitor
	var objs []uint32
	for _, i := range w.outs.list {
		if mon, ok := i.monitor(); ok {
			list = append(list, mon)
			objs = append(objs, i.id)
		}
	}
	w.outs.mu.Unlock()
	cur, ok := matchMonitor(list, m)
	if !ok {
		return 0, false
	}
	for i, c := range list {
		if c.id == cur.id {
			return objs[i], true
		}
	}
	return 0, false
}

// setFullscreen — SetFullscreen и SetFullscreenOn: output — объект wl_output
// (0 — выбор за компоновщиком).
func (w *WaylandWindow) setFullscreen(on bool, output uint32) bool {
	if w.toplevelID == 0 {
		return false
	}
	if on {
		if w.fullscreen.Load() && output != 0 {
			// Уже в полном экране: перенос на другой вывод. Сохранённый
			// размер и фиксация не трогаются, ответ придёт обычным configure.
			wlLog("set_fullscreen(output %d): перенос", output)
			w.send(newWlMsg(w.toplevelID, xdgToplevelSetFullscreen).putUint(output), -1)
			return true
		}
		if w.fullscreen.Load() || w.fs.pending.Load() {
			if output != 0 && w.fs.pending.Load() {
				// Запрос ещё в пути, а монитор другой — уточняем его.
				w.send(newWlMsg(w.toplevelID, xdgToplevelSetFullscreen).putUint(output), -1)
			}
			return true // уже запрошено или выполнено
		}
		w.mu.Lock()
		w.fs.saveW.Store(int32(w.width))
		w.fs.saveH.Store(int32(w.height))
		w.mu.Unlock()
		// Окно с зафиксированным размером (min == max) на весь экран не
		// растянуть: фиксация снимается так же, как перед разворачиванием.
		w.applySizeLimits(true)
		w.fs.pending.Store(true)
		wlLog("set_fullscreen")
		w.send(newWlMsg(w.toplevelID, xdgToplevelSetFullscreen).putUint(output), -1)
		return true
	}
	if !w.fullscreen.Load() && !w.fs.pending.Load() {
		return true // и так не в полном экране
	}
	w.fs.pending.Store(false)
	wlLog("unset_fullscreen")
	w.send(newWlMsg(w.toplevelID, xdgToplevelUnsetFullscreen), -1)
	w.restoreFixedSize()
	return true
}

// IsFullscreen — последний configure содержал состояние fullscreen.
// Реализует fullscreenBackend.
func (w *WaylandWindow) IsFullscreen() bool { return w.fullscreen.Load() }

// SetOnFullscreenChanged регистрирует уведомление о смене состояния.
// Реализует fullscreenBackend. Вызывается из цикла событий.
func (w *WaylandWindow) SetOnFullscreenChanged(fn func(on bool)) {
	if fn == nil {
		w.fs.onChange.Store(nil)
		return
	}
	w.fs.onChange.Store(&fn)
}

// restoreFixedSize возвращает окну с запретом ресайза фиксацию на размере,
// который был до полного экрана (см. applySizeLimits).
func (w *WaylandWindow) restoreFixedSize() {
	if w.resizable.Load() {
		return
	}
	sw, sh := int(w.fs.saveW.Load()), int(w.fs.saveH.Load())
	if sw <= 0 || sh <= 0 {
		return
	}
	w.pinSize(sw, sh)
}

// noteToplevelStates применяет состояния из xdg_toplevel.configure.
//
//   - «Развёрнуто» на время полного экрана не меняется: IsMaximized
//     возвращает прежнее значение (компоновщик в этот момент может вообще не
//     называть состояние maximized).
//   - Смена полного экрана → уведомление приложения.
//   - Configure без fullscreen при запросе в пути → отказ: уведомление false.
func (w *WaylandWindow) noteToplevelStates(maximized, fullscreen bool) {
	if !fullscreen {
		w.maximized.Store(maximized)
	}
	was := w.fullscreen.Swap(fullscreen)
	pending := w.fs.pending.Swap(false)

	switch {
	case fullscreen != was:
		if !fullscreen {
			w.restoreFixedSize() // компоновщик вернул окно сам (Esc, клавиша)
		}
		w.notifyFullscreen(fullscreen)
	case !fullscreen && pending:
		// Отказ: configure ответил размером и состояниями окна как есть.
		wlLog("set_fullscreen отклонён компоновщиком")
		w.restoreFixedSize()
		w.notifyFullscreen(false)
	}
}

func (w *WaylandWindow) notifyFullscreen(on bool) {
	if fn := w.fs.onChange.Load(); fn != nil {
		(*fn)(on)
	}
}
