//go:build linux && !android

package x11

// x11_monitor_change_linux.go — смена конфигурации мониторов на X11 (описание
// — в window/core/monitor_change.go).
//
// Наблюдатель живёт на ОТДЕЛЬНОМ соединении с сервером (как и сам опрос
// мониторов, x11_monitor_linux.go): события RandR приходят в поток соединения,
// а поток соединения окна читает цикл событий, которому чужие подписки ни к
// чему. Соединение наблюдателя ничего, кроме этих событий, не получает, так что
// разбирать на нём нечего, кроме 32-байтовых пакетов.
//
// Подписка:
//
//	RRSelectInput(root, ScreenChange|CrtcChange|OutputChange)  // RandR 1.2+
//	ChangeWindowAttributes(root, PropertyChange)               // _NET_WORKAREA
//
// ScreenChangeNotify приходит при смене разрешения, поворота и раскладки,
// CrtcChangeNotify/OutputChangeNotify — при подключении и отключении выводов и
// перенастройке контроллеров; PropertyNotify по _NET_WORKAREA — когда панель
// задач заняла или освободила край экрана (Monitor.Work).

import (
	"encoding/binary"
	"sync/atomic"
)

const (
	randrSelectInput = 4 // RRSelectInput (минорный опкод RandR)

	// Маски RRSelectInput.
	randrScreenChangeNotifyMask = 1
	randrCrtcChangeNotifyMask   = 2
	randrOutputChangeNotifyMask = 4

	// Маска событий окна: PropertyChange.
	x11MaskPropertyChange = 0x00400000
)

// x11RRSelectInputRequest — RRSelectInput(window, enable): запрос без ответа.
func x11RRSelectInputRequest(major byte, window uint32, mask uint16) []byte {
	req := make([]byte, 12)
	req[0], req[1] = major, randrSelectInput
	binary.LittleEndian.PutUint16(req[2:4], 3)
	binary.LittleEndian.PutUint32(req[4:8], window)
	binary.LittleEndian.PutUint16(req[8:10], mask)
	return req
}

// x11SelectRootPropertyRequest — ChangeWindowAttributes(root, event-mask =
// PropertyChange): запрос без ответа. Маска событий у каждого клиента своя, так
// что оконный менеджер и остальные программы подписку не замечают.
func x11SelectRootPropertyRequest(root uint32) []byte {
	req := make([]byte, 16)
	req[0] = 2 // ChangeWindowAttributes
	binary.LittleEndian.PutUint16(req[2:4], 4)
	binary.LittleEndian.PutUint32(req[4:8], root)
	binary.LittleEndian.PutUint32(req[8:12], 0x00000800) // CWEventMask
	binary.LittleEndian.PutUint32(req[12:16], x11MaskPropertyChange)
	return req
}

// x11MonitorEvent — событие относится к мониторам: ScreenChangeNotify
// (firstEvent), Notify подтипов CrtcChange/OutputChange (firstEvent+1) или
// PropertyNotify по _NET_WORKAREA. Чистая функция: раскладку событий проверяет
// тест.
func x11MonitorEvent(buf []byte, randrFirstEvent byte, workAreaAtom uint32) bool {
	if len(buf) < 12 {
		return false
	}
	t := buf[0] & 0x7F
	if randrFirstEvent != 0 && (t == randrFirstEvent || t == randrFirstEvent+1) {
		return true
	}
	if t == x11EvPropertyNotify && workAreaAtom != 0 {
		return binary.LittleEndian.Uint32(buf[8:12]) == workAreaAtom
	}
	return false
}

// WatchMonitors подписывается на смену мониторов (monitorChangeSource).
func (w *X11Window) WatchMonitors(notify func()) (stop func()) {
	return x11WatchMonitors(notify)
}

// x11WatchMonitors поднимает наблюдатель на новом соединении. nil — сервера
// нет, соединиться не удалось.
func x11WatchMonitors(notify func()) (stop func()) {
	if notify == nil {
		return nil
	}
	c := &X11Window{}
	if err := c.connect(); err != nil {
		return nil
	}

	var randrFirst byte
	if major, first, ok := c.x11ExtensionInfo("RANDR"); ok {
		randrFirst = first
		// Маска CrtcChange/OutputChange есть с RandR 1.2; на более старом
		// сервере остаётся ScreenChange (его хватает для смены разрешения).
		mask := uint16(randrScreenChangeNotifyMask)
		if c.x11RandrAtLeast(major, 1, 2) {
			mask |= randrCrtcChangeNotifyMask | randrOutputChangeNotifyMask
		}
		c.x11Send(x11RRSelectInputRequest(major, c.rootWin, mask))
	}
	workArea := c.x11InternAtom("_NET_WORKAREA")
	if workArea != 0 {
		c.x11Send(x11SelectRootPropertyRequest(c.rootWin))
	}
	if randrFirst == 0 && workArea == 0 {
		c.conn.Close()
		return nil
	}

	var stopped atomic.Bool
	go func() {
		buf := make([]byte, 32)
		for !stopped.Load() {
			if !c.readFull(buf) {
				return
			}
			if x11MonitorEvent(buf, randrFirst, workArea) {
				notify()
			}
		}
	}()
	return func() {
		stopped.Store(true)
		c.conn.Close() // читающая горутина получает ошибку чтения и выходит
	}
}

// x11ExtensionInfo — major-опкод и первый код события расширения.
func (w *X11Window) x11ExtensionInfo(name string) (major, firstEvent byte, ok bool) {
	reply, got := w.x11Call(x11QueryExtensionRequest(name))
	if !got {
		return 0, 0, false
	}
	major, firstEvent, present := parseQueryExtensionReply(reply)
	return major, firstEvent, present
}

// x11RandrAtLeast — сервер говорит по RandR не старше majorWant.minorWant.
// Заодно это обязательное согласование версии: до него RandR не отвечает на
// запросы новее 1.0.
func (w *X11Window) x11RandrAtLeast(major byte, majorWant, minorWant uint32) bool {
	req := make([]byte, 12)
	req[0], req[1] = major, randrQueryVersion
	binary.LittleEndian.PutUint16(req[2:4], 3)
	binary.LittleEndian.PutUint32(req[4:8], 1) // версия клиента 1.5, сервер ответит своей
	binary.LittleEndian.PutUint32(req[8:12], 5)
	reply, ok := w.x11Call(req)
	if !ok || len(reply) < 16 {
		return false
	}
	maj := binary.LittleEndian.Uint32(reply[8:12])
	mnr := binary.LittleEndian.Uint32(reply[12:16])
	return maj > majorWant || (maj == majorWant && mnr >= minorWant)
}
