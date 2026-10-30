package pointer

import "github.com/oops1/headless-gui/v4/platform/native"

// pointerleave.go — указатель ушёл с окна.
//
// Движок узнаёт, что мышь над виджетом, по движениям; об уходе с окна ему
// никто не сообщал. Кнопка под последним местом указателя оставалась
// подсвеченной, пока мышь не вернётся: окно калькулятора сжалось по
// «восстановить», указатель остался снаружи — а «развернуть» продолжала
// гореть (задание WinLine, 05.10.2026). Win32 уже уводил мышь за окно на
// WM_NCMOUSELEAVE; теперь так же поступают Wayland (wl_pointer.leave) и X11
// (LeaveNotify): движку приходит движение в точку вне окна, и наведение с
// подсказками снимаются тем же путём, что и при обычном уходе за край.

// Outside — координата точки вне окна: движение туда снимает наведение.
const Outside = -1

// X11PointerLeft — LeaveNotify означает, что указатель покинул окно.
//
// Не означает в двух случаях: detail NotifyInferior (2) — указатель ушёл в
// дочернее окно, оставаясь над нашим; mode NotifyUngrab (2) — снят захват,
// а указатель может быть по-прежнему внутри.
func X11PointerLeft(detail, mode byte) bool {
	const notifyInferior, notifyUngrab = 2, 2
	return detail != notifyInferior && mode != notifyUngrab
}

// ─── Win32: WM_MOUSELEAVE ───────────────────────────────────────────────────
//
// Windows сообщает об уходе указателя только тому, кто об этом попросил:
// TrackMouseEvent(TME_LEAVE) срабатывает один раз и после этого снимается. Без
// запроса выход через клиентскую область не сообщался вовсе — наведение
// «залипало». Теперь окно взводит запрос на первом движении после входа, а
// WM_MOUSELEAVE идёт тем же путём, что LeaveNotify и wl_pointer.leave.

// LeaveTracker помнит, взведён ли у системы запрос на уход указателя.
//
// TME_LEAVE одноразовый: взводить его на каждое движение значило бы звать
// систему сотни раз в секунду, а не взводить после ухода — не узнать о
// следующем. Нулевое значение — запрос не взведён.
type LeaveTracker struct {
	armed bool
}

// NeedArm — стоит ли взводить запрос (вызывать на движении мыши).
func (t *LeaveTracker) NeedArm() bool { return !t.armed }

// ArmedOK запоминает результат TrackMouseEvent: при отказе системы
// (окна уже нет) запрос не считается взведённым, и следующее движение
// попробует снова.
func (t *LeaveTracker) ArmedOK(ok bool) { t.armed = ok }

// left сбрасывает запрос: система сняла его, отправив WM_MOUSELEAVE.
func (t *LeaveTracker) Left() { t.armed = false }

// MouseLeaveReal — действительно ли WM_MOUSELEAVE означает уход указателя
// с окна.
//
// Не означает, когда указатель перешёл с клиентской области на кнопку
// заголовка, которую окно нарисовало само: система считает её нерабочей
// областью (HTMINBUTTON/HTMAXBUTTON/HTCLOSE ради макетов привязки Windows 11),
// шлёт WM_MOUSELEAVE, а подсветку кнопки ведёт тем же указателем уже
// WM_NCMOUSEMOVE. Сообщи мы здесь об уходе — наведение на кнопку снималось бы
// сразу после того, как его поставили (порядок сообщений не гарантирован).
// Заголовок как таковой (HitCaption) и края окна — настоящий уход: там
// виджеты содержимого указателя больше не видят.
//
// inWindow — указатель в пределах окна; under — зона под ним.
func MouseLeaveReal(under native.HitArea, inWindow bool) bool {
	if !inWindow {
		return true
	}
	switch under {
	case native.HitMinButton, native.HitMaxButton, native.HitCloseButton:
		return false
	}
	return true
}
