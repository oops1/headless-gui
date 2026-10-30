package tray

// event.go — разбор уведомления иконки трея Win32 (uCallbackMessage несёт код
// события в младшем слове lParam). Переносимый: проверяется на любой машине.

// Коды мышиных сообщений Win32, продублированы здесь для платформенно-
// независимого маппинга событий трея (uCallbackMessage несёт код события в
// младшем слове lParam). См. trayEventToClick, winTray.handleCallback.
const (
	trayEvtLButtonUp        = 0x0202 // WM_LBUTTONUP
	trayEvtLButtonDblClk    = 0x0203 // WM_LBUTTONDBLCLK
	trayEvtRButtonUp        = 0x0205 // WM_RBUTTONUP
	trayEvtMButtonUp        = 0x0208 // WM_MBUTTONUP
	trayEvtBalloonUserClick = 0x0405 // NIN_BALLOONUSERCLICK (WM_USER+5)
)

// trayEventToClick разбирает событие иконки трея (младшее слово lParam
// callback-сообщения) в кнопку/двойной клик/клик-по-balloon. ok=false —
// событие нас не интересует (move, down и т.п.).
func trayEventToClick(ev uint32) (button int, doubleClick, balloon, ok bool) {
	switch ev & 0xFFFF {
	case trayEvtLButtonUp:
		return 0, false, false, true
	case trayEvtLButtonDblClk:
		return 0, true, false, true
	case trayEvtRButtonUp:
		return 1, false, false, true
	case trayEvtMButtonUp:
		return 2, false, false, true
	case trayEvtBalloonUserClick:
		return 0, false, true, true
	}
	return 0, false, false, false
}
