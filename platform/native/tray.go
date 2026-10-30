package native

// tray.go — возможность трея и уведомлений (ADR-0013 §3): иконка в
// области уведомлений, её меню и клики, системные уведомления. Живёт в
// platform/tray и регистрируется здесь; ядро окна создаёт трей окну после
// создания окна ОС и снимает при закрытии, а без возможности отвечает
// ErrTrayUnsupported. Здесь же — пиксели значка, общие с бэкендом Win32.

import (
	"errors"
	"image"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// Tray — иконка в трее и уведомления одного окна. Методы зовутся с любой
// горутины; колбэки (клик по иконке, по уведомлению) трей зовёт со своего
// потока — ядро само переносит их на горутину движка.
//
//facadegen:skip
type Tray interface {
	// SetTrayIcon показывает или обновляет иконку с подсказкой tooltip.
	SetTrayIcon(icon image.Image, tooltip string) error
	// RemoveTrayIcon убирает иконку.
	RemoveTrayIcon()
	// SetTrayClickHandler — диспетчер кликов по иконке (button: 0 левая,
	// 1 правая, 2 средняя).
	SetTrayClickHandler(fn func(button int, doubleClick bool))
	// SetTrayMenuNative отдаёт меню трея, если его показывает сама система
	// (Linux, com.canonical.dbusmenu): true — ядро своё меню не открывает и в
	// дерево виджетов не добавляет. Где меню рисует ядро (Windows) — false.
	SetTrayMenuNative(menu *base.PopupMenu) bool
	// SetTrayMenuRunner — где выполнять выбор пункта системного меню (ядро
	// отдаёт свою очередь движка).
	SetTrayMenuRunner(run func(fn func()))
	// CursorScreenPos — курсор в экранных координатах: там ядро открывает
	// своё меню трея.
	CursorScreenPos() (int, int)
	// ShowBalloon показывает системное уведомление (infoFlag: 0 нет значка,
	// 1 информация, 2 предупреждение, 3 ошибка).
	ShowBalloon(title, text string, infoFlag uint32) error
	// SetBalloonClickHandler — колбэк клика по уведомлению.
	SetBalloonClickHandler(fn func())
	// Close снимает иконку и подписки окна; повторный вызов ничего не
	// делает. После Close SetTrayIcon и ShowBalloon отвечают
	// ErrTrayUnsupported, остальное — no-op: вызов, разминувшийся с
	// закрытием окна, иконку не вернёт.
	Close()
}

// TrayFactory — трей окна ОС nw; nil — у этого окна (бэкенда) трея нет.
//
//facadegen:skip
type TrayFactory func(nw NativeWindow) Tray

var traySlot = newCapSlot[TrayFactory]("tray")

// RegisterTray подключает трей (зовёт init пакета platform/tray). restore
// возвращает прежнюю регистрацию — для тестов.
//
//facadegen:skip
func RegisterTray(f TrayFactory) (restore func()) { return traySlot.register(f) }

// TrayService — подключённый трей; ok=false — возможность не подключена
// (window/minimal) или ОС трея не умеет.
//
//facadegen:skip
func TrayService() (f TrayFactory, ok bool) { return traySlot.get() }

// ErrTrayUnsupported возвращается методами трея на платформах без поддержки.
//
//facadegen:skip
var ErrTrayUnsupported = errors.New("window: иконка в трее поддерживается на Windows и Linux")

// IconColorBuffer конвертирует премультиплицированный RGBA в top-down 32bpp
// BGRA-буфер цветного DIB иконки (длина = w*h*4). Порядок R,G,B,A → B,G,R,A.
//
//facadegen:skip
func IconColorBuffer(img *image.RGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			di := (y*w + x) * 4
			out[di+0] = img.Pix[si+2] // B
			out[di+1] = img.Pix[si+1] // G
			out[di+2] = img.Pix[si+0] // R
			out[di+3] = img.Pix[si+3] // A
		}
	}
	return out
}
