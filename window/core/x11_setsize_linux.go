//go:build linux && !android

package core

// x11_setsize_linux.go — программная смена размера окна X11 (SetSize).
//
// Размер окна на X11 решает оконный менеджер: ConfigureWindow — лишь просьба,
// и менеджер вправе исполнить её иначе (минимум, шаг размера, рамка) или не
// исполнить вовсе. Поэтому width/height пишет ТОЛЬКО ConfigureNotify — то, что
// сервер сообщил об окне на самом деле. SetSize запоминает запрос отдельно и
// сам размер не трогает; тогда:
//
//   - ответ менеджера проходит прежний путь resize (x11ShmResize, onResize →
//     холст → раскладка → кадр), ровно один раз на запрошенное изменение;
//   - GetSize всегда отвечает фактическим размером: менеджер отказал или
//     промолчал — размер остался прежним, и это правда;
//   - ConfigureNotify с размером, который окно уже имеет (отказ менеджера,
//     повторное событие), onResize не вызывает.
//
// Раньше SetSize записывал размер ДО ConfigureWindow, и ответ менеджера не
// проходил сравнение «размер изменился»: onResize не вызывался, холст
// оставался старым, а GetSize отвечал заказанным размером при неизменившемся
// окне (отчёт RDP-клиента Tandem).

import (
	"sync"
	"time"
)

// x11SizeReqTTL — сколько запрос размера считается «в пути». Менеджер, не
// ответивший за это время, запрос отклонил: сравнение следующего SetSize идёт
// уже с фактическим размером.
const x11SizeReqTTL = time.Second

// x11SizeReq — запросы размера и замок вокруг width/height окна.
type x11SizeReq struct {
	mu sync.Mutex
	// pending/w/h/at — ConfigureWindow отправлен, ConfigureNotify ещё не
	// пришёл. Нужен, чтобы быстрая пара SetSize(520,370) и SetSize(1400,860)
	// не посчитала второй вызов «размер уже такой» по ещё не изменившемуся
	// фактическому размеру.
	pending bool
	w, h    int
	at      time.Time
	// restoreW/restoreH — размер, заказанный развёрнутому или полноэкранному
	// окну: менеджер распоряжается им сам, заказ ждёт возврата в обычное
	// состояние (x11ApplyRestoreSize).
	restoreW, restoreH int
}

// SetSize просит менеджер окон сменить размер окна (физические пиксели).
//
// Размер окна меняется не здесь, а в ConfigureNotify (см. комментарий файла);
// GetSize до ответа менеджера отвечает прежним размером. Размер совпал с
// фактическим (или с уже запрошенным и ещё не подтверждённым) — запроса нет.
//
// Окно, которого ещё нет на сервере, запоминает размер для Create. Попап
// (override-redirect, менеджера нет и StructureNotify не выбран) меняет размер
// сразу: ответа ждать не от кого.
//
// Развёрнутое и полноэкранное окно размером распоряжается менеджер: заказ
// запоминается и применяется, когда окно вернётся в обычное состояние.
func (w *X11Window) SetSize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	w.sz.mu.Lock()
	if w.wid == 0 {
		w.width, w.height = width, height
		w.sz.mu.Unlock()
		return
	}
	if w.popup {
		w.width, w.height = width, height
		w.sz.mu.Unlock()
		w.x11ConfigureWindow(w.wid, width, height)
		w.x11ShmResize(width, height)
		return
	}
	if w.maximized || w.fs.on.Load() {
		w.sz.restoreW, w.sz.restoreH = width, height
		w.sz.mu.Unlock()
		return
	}
	w.sz.restoreW, w.sz.restoreH = 0, 0
	tw, th := w.width, w.height
	if w.sz.pending && time.Since(w.sz.at) < x11SizeReqTTL {
		tw, th = w.sz.w, w.sz.h
	}
	if tw == width && th == height {
		w.sz.mu.Unlock()
		return
	}
	w.sz.pending, w.sz.w, w.sz.h, w.sz.at = true, width, height, time.Now()
	w.sz.mu.Unlock()
	w.x11ConfigureWindow(w.wid, width, height)
}

// GetSize возвращает фактический размер окна — последний, о котором сообщил
// сервер (ConfigureNotify) или который задан окну при создании.
func (w *X11Window) GetSize() (int, int) {
	w.sz.mu.Lock()
	defer w.sz.mu.Unlock()
	return w.width, w.height
}

// x11NoteConfigureSize принимает размер из ConfigureNotify. true — размер
// отличается от известного, и событие надо провести как ресайз.
func (w *X11Window) x11NoteConfigureSize(newW, newH int) bool {
	w.sz.mu.Lock()
	defer w.sz.mu.Unlock()
	// Любой ответ менеджера снимает «запрос в пути»: подтвердил он запрос,
	// изменил его или отказал (синхронное событие с прежним размером).
	w.sz.pending = false
	if newW == w.width && newH == w.height {
		return false
	}
	w.width, w.height = newW, newH
	return true
}

// x11ApplyRestoreSize отправляет размер, заказанный развёрнутому или
// полноэкранному окну, когда оно вернулось в обычное состояние. Зовётся из
// цикла событий после перечитывания _NET_WM_STATE и из Restore.
func (w *X11Window) x11ApplyRestoreSize() {
	w.sz.mu.Lock()
	rw, rh := w.sz.restoreW, w.sz.restoreH
	if rw <= 0 || rh <= 0 || w.wid == 0 || w.maximized || w.fs.on.Load() {
		w.sz.mu.Unlock()
		return
	}
	w.sz.restoreW, w.sz.restoreH = 0, 0
	same := rw == w.width && rh == w.height
	if !same {
		w.sz.pending, w.sz.w, w.sz.h, w.sz.at = true, rw, rh, time.Now()
	}
	w.sz.mu.Unlock()
	if !same {
		w.x11ConfigureWindow(w.wid, rw, rh)
	}
}
