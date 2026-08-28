// Package desktop — компоненты рабочего стола: панель задач и всё, что на
// ней живёт, меню «Пуск», всплывающие панели уведомлений и настроек.
//
// Пакет отвечает за поведение и раскладку, но не за внешний вид: ни одного
// цвета и ни одного размера в отрисовке компонентов нет — и то и другое
// приходит из темы (пакет theme). Одна и та же панель задач под профилем
// Windows 11 выглядит полосой кнопок, под macOS — доком; меняется тема, не
// компонент.
//
// Компоненты не ходят в систему сами. Список окон, каталог приложений,
// состояние сети и звука приходят через интерфейсы этого файла, которые
// реализует потребитель — оболочка удалённого рабочего стола, оконный
// менеджер, что угодно. Движок поставляет тестовые реализации (fakes.go),
// чтобы панель можно было показать и покрыть тестами, не имея ни одной
// настоящей системы под рукой.
package desktop

import (
	"image"
	"time"
)

// WindowID — идентификатор окна в модели потребителя. Движок его не
// толкует: это может быть HWND, номер в списке или что угодно ещё.
type WindowID uint64

// AppID — идентификатор приложения в каталоге.
type AppID string

// NotificationID — идентификатор уведомления.
type NotificationID uint64

// WindowInfo — что панель задач знает об окне.
type WindowInfo struct {
	ID        WindowID
	Title     string
	AppID     AppID
	Icon      image.Image
	Active    bool // окно на переднем плане
	Minimized bool
}

// WindowModel — список окон и действия над ними.
//
// Subscribe возвращает функцию отписки. Забытая отписка удерживает
// подписчика: панель отписывается, когда её убирают со сцены.
type WindowModel interface {
	Windows() []WindowInfo
	Activate(id WindowID)
	Minimize(id WindowID)
	Close(id WindowID)
	Subscribe(func()) func()
}

// AppInfo — приложение в каталоге (меню «Пуск», закреплённые значки).
type AppInfo struct {
	ID         AppID
	Title      string
	Icon       image.Image
	Categories []string
}

// AppCatalog — каталог приложений и закрепление.
type AppCatalog interface {
	Apps() []AppInfo
	Pinned() []AppID
	Pin(AppID)
	Unpin(AppID)
	Launch(AppID) error
}

// NetKind — вид подключения.
type NetKind int

const (
	NetNone NetKind = iota
	NetEthernet
	NetWiFi
	NetCellular
)

// NetState — состояние сети: вид связи, качество сигнала (0..1) и имя
// подключения.
type NetState struct {
	Kind    NetKind
	Quality float64
	Name    string
}

// VolState — состояние звука: уровень (0..1) и приглушение.
type VolState struct {
	Level float64
	Muted bool
}

// PowerState — состояние питания: заряд (0..1), питание от сети,
// признак «батареи нет вовсе» (настольная машина).
type PowerState struct {
	Charge    float64
	OnAC      bool
	NoBattery bool
}

// SystemStatus — показатели, которые панель отображает в трее.
type SystemStatus interface {
	Network() NetState
	Volume() VolState
	Power() PowerState
	Subscribe(func()) func()
}

// Severity — важность уведомления.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

// Notification — одно уведомление.
type Notification struct {
	ID       NotificationID
	Title    string
	Body     string
	AppID    AppID
	Severity Severity
	Time     time.Time
}

// Notifications — центр уведомлений.
type Notifications interface {
	List() []Notification
	Dismiss(NotificationID)
	Subscribe(func()) func()
}

// Clock — источник времени. Отдельный интерфейс нужен ровно затем, чтобы в
// тестах часы показывали заданное время, а не текущее: иначе golden-тест
// панели пришлось бы переснимать каждую минуту.
type Clock interface {
	Now() time.Time
}

// SystemClock — часы, идущие по системному времени.
type SystemClock struct{}

// Now возвращает текущее время.
func (SystemClock) Now() time.Time { return time.Now() }
