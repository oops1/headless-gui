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
//
// # Из какой горутины что зовётся
//
// Компоненты живут между двумя горутинами, и это часть контракта, а не
// деталь реализации.
//
//   - Горутина кадра. Отрисовка (Draw, Measure, Layout, презентеры темы) и
//     ввод (OnMouseMove, OnMouseButton, OnKey) идут из неё. Движок обходит
//     дерево последовательно, поэтому между собой они не пересекаются.
//
//   - Горутина потребителя. Замыкание, переданное в Subscribe, зовётся
//     ОТТУДА, где потребитель узнал об изменении: поток сообщений окна,
//     обработчик события системы, таймер. Движок этот вызов никуда не
//     перекладывает — иначе уведомление отставало бы от кадра, а приложение
//     не могло бы рассчитывать, что после Windows() список уже новый.
//
// Отсюда правило для компонента: всё, что уведомление меняет, обязано быть
// под замком, потому что горутина кадра читает то же самое одновременно.
// Образец — ApplicationArea и RunningApplications: замок закрывает
// подсчитанную раскладку и индексы наведения, а наружу (Invalidate, вызовы
// WindowModel, презентер темы) компонент ходит уже ОТПУСТИВ его — иначе
// замок держался бы всю отрисовку, а встречный вызов из потребителя привёл
// бы к взаимной блокировке.
//
// Правило для потребителя: замыкание подписки обязано быть коротким и не
// звать движок обратно. Оно уже выполняется в его горутине — тяжёлая работа
// в нём задержит того, кто прислал уведомление.
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

// ProgressState — вид наложения прогресса на кнопке окна.
type ProgressState int

const (
	// ProgressNone — прогресса нет (нулевое значение: потребитель, который
	// поля не заполнял, наложения не получает).
	ProgressNone ProgressState = iota
	// ProgressNormal — обычный: полоса цвета акцента.
	ProgressNormal
	// ProgressPaused — пауза: полоса жёлтая.
	ProgressPaused
	// ProgressError — ошибка: полоса красная.
	ProgressError
)

// WindowInfo — что панель задач знает об окне.
//
// Первые шесть полей — прежняя модель. Остальные нужны кнопкам Windows 11
// (наложение прогресса, счётчик, «внимание»): тема, которая их не рисует,
// их не читает, а потребитель, который их не заполнял, ничего не теряет.
type WindowInfo struct {
	ID        WindowID
	Title     string
	AppID     AppID
	Icon      image.Image
	Active    bool // окно на переднем плане
	Minimized bool

	// ProgressState и Progress — наложение прогресса: полоса снизу значка.
	// Progress — доля 0..1 (вне диапазона обрезается), видна только при
	// ProgressState != ProgressNone. У кнопки со многими окнами (стопки)
	// показывается самое важное состояние: ошибка, затем пауза, затем обычный.
	ProgressState ProgressState
	Progress      float64
	// Badge — счётчик в кружке на значке (непрочитанные письма); 0 — нет.
	// Больше 99 показывается как «99+». У стопки счётчики окон суммируются.
	Badge int
	// Attention — окно просит внимания: подложка кнопки мигает несколько раз
	// и остаётся подсвеченной, пока окно не станет активным. У активного окна
	// флаг игнорируется.
	Attention bool
}

// WindowModel — список окон и действия над ними.
//
// Subscribe возвращает функцию отписки. Забытая отписка удерживает
// подписчика: панель отписывается, когда её убирают со сцены.
//
// Замыкание зовётся из горутины потребителя — см. раздел «Из какой горутины
// что зовётся» в описании пакета.
type WindowModel interface {
	Windows() []WindowInfo
	Activate(id WindowID)
	Minimize(id WindowID)
	Close(id WindowID)
	Subscribe(func()) func()
}

// WindowPreviews — источник миниатюр окон для предпросмотра на панели задач.
//
// Необязателен: модель, которая его не реализует, просто не получает
// предпросмотра — проверяется приведением типа, как Subscribe у AppCatalog.
//
// Почему не поле в WindowInfo: модель перестраивается на каждое изменение
// состава окон и на каждую смену фокуса, а показывается в один момент ровно
// одна миниатюра. Класть снимок в модель значило бы снимать каждое окно на
// каждое переключение.
//
// Зовётся из горутины кадра, не чаще, чем раз в KeyPreviewRefresh, и только
// пока предпросмотр открыт. Живая миниатюра в каждом кадре — прямая дорога к
// тому, от чего движок ушёл в 3.16.1: неподвижный рабочий стол начинал слать
// кадры непрерывно.
type WindowPreviews interface {
	// Preview возвращает миниатюру окна, вписанную в max (логические
	// пиксели). nil — миниатюры нет: окно ещё не рисовалось.
	//
	// Свёрнутое или закрытое чужим окном отдаёт последний удачный снимок,
	// сделанный пока оно было видно, — Windows ведёт себя так же, миниатюра
	// свёрнутого окна замирает.
	Preview(id WindowID, max image.Point) image.Image
}

// AppInfo — приложение в каталоге (меню «Пуск», закреплённые значки).
type AppInfo struct {
	ID    AppID
	Title string
	// Icon — значок одной картинкой. Годится, когда у приложения один растр;
	// на панели (24), в меню (20–32) и на плитках (48–64) он одинаково
	// уменьшается или растягивается, то есть где-то мягок.
	Icon image.Image
	// IconAt — необязательный источник значка по размеру: получает сторону
	// квадрата в ФИЗИЧЕСКИХ пикселях (логический размер × масштаб экрана) и
	// отдаёт картинку подходящего размера — растр ближайшего размера или SVG,
	// растеризованный под него. nil или пустой ответ — берётся Icon.
	// Вызывается при отрисовке: функция должна быть быстрой и потокобезопасной.
	IconAt     func(size int) image.Image
	Categories []string
}

// IconFor возвращает значок для квадрата со стороной size (физические пиксели):
// IconAt(size), а при его отсутствии или пустом ответе — Icon.
func (a AppInfo) IconFor(size int) image.Image {
	if a.IconAt != nil && size > 0 {
		if img := a.IconAt(size); img != nil {
			return img
		}
	}
	return a.Icon
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
//
// Замыкание Subscribe зовётся из горутины потребителя — см. раздел «Из какой
// горутины что зовётся» в описании пакета.
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
//
// Первые шесть полей — прежняя модель, её хватает плоскому центру. Остальные
// нужны центру в стиле Windows 10 (группы по приложению, значок, действия):
// плоский их не читает, а потребитель, который их не заполнял, ничего не
// теряет.
type Notification struct {
	ID       NotificationID
	Title    string
	Body     string
	AppID    AppID
	Severity Severity
	Time     time.Time

	// AppName — название приложения в заголовке группы. Пусто — берётся
	// AppID: группа не должна остаться без подписи.
	AppName string
	// Icon — значок приложения. Показывается в заголовке группы (малый) и на
	// карточке (крупный), поэтому лучше отдавать картинку покрупнее — до
	// 48 логических пикселей. IconAt, если задан, главнее: получает сторону в
	// ФИЗИЧЕСКИХ пикселях (как AppInfo.IconAt) и отдаёт растр подходящего
	// размера или SVG, растеризованный под него.
	Icon   image.Image
	IconAt func(size int) image.Image
	// Timestamp — момент прихода; когда задан, главнее Time (поле Time
	// осталось от прежней модели, и двух источников времени не нужно ни
	// тому, ни другому потребителю: читать надо At()).
	Timestamp time.Time
	// Actions — кнопки, ссылки, поле ответа и выпадающий список внутри
	// карточки. Нет действий — карточка без них.
	Actions []NotificationAction
}

// At возвращает время уведомления: Timestamp, а если он не задан, Time.
func (n Notification) At() time.Time {
	if !n.Timestamp.IsZero() {
		return n.Timestamp
	}
	return n.Time
}

// IconFor возвращает значок для квадрата со стороной size (физические пиксели):
// IconAt(size), а при его отсутствии или пустом ответе — Icon.
func (n Notification) IconFor(size int) image.Image {
	if n.IconAt != nil && size > 0 {
		if img := n.IconAt(size); img != nil {
			return img
		}
	}
	return n.Icon
}

// NotificationActionKind — вид действия внутри карточки.
type NotificationActionKind int

const (
	// NotificationActionButton — кнопка. Соседние кнопки встают в один ряд
	// и делят его ширину поровну.
	NotificationActionButton NotificationActionKind = iota
	// NotificationActionLink — ссылка, текст цветом акцента без подложки.
	NotificationActionLink
	// NotificationActionReply — поле ответа и кнопка отправки справа от него.
	NotificationActionReply
	// NotificationActionSelect — выпадающий список с подписью над ним.
	NotificationActionSelect
)

// NotificationAction — одно действие в карточке уведомления.
type NotificationAction struct {
	// ID возвращается в событии; уникален в пределах уведомления.
	ID   string
	Kind NotificationActionKind
	// Title — надпись кнопки или ссылки; у поля ответа — кнопки отправки
	// (пусто — «Ответить» на языке интерфейса); у выпадающего списка —
	// подпись над списком («Напомнить ещё раз через:»).
	Title string
	// Placeholder — подсказка пустого поля ответа.
	Placeholder string
	// Options и Selected — пункты выпадающего списка и выбранный сначала.
	Options  []string
	Selected int
	// Keep оставляет уведомление в центре после нажатия. По умолчанию
	// кнопка, ссылка и отправка ответа его снимают, как в Windows.
	Keep bool
}

// NotificationActionEvent — что пользователь сделал с карточкой.
type NotificationActionEvent struct {
	// Notification — уведомление, на котором сделано действие.
	Notification NotificationID
	// Action — NotificationAction.ID; пусто — нажата сама карточка
	// (активация по умолчанию: открыть приложение).
	Action string
	Kind   NotificationActionKind
	// Value — введённый ответ либо выбранный пункт списка (для кнопки и
	// ссылки пусто); Index — номер выбранного пункта (иначе -1).
	Value string
	Index int
	// Inputs — текущие значения полей карточки (ответ и списки) по
	// NotificationAction.ID: кнопка «Приступим» несёт выбранное «1 неделю».
	Inputs map[string]string
}

// NotificationActions — необязательный интерфейс источника уведомлений:
// получатель действий. Центр вызывает его, если Notifications его
// реализует, и в придачу зовёт колбэк NotificationCenter.OnAction — годится
// любой из двух путей, пользоваться обоими сразу незачем.
type NotificationActions interface {
	InvokeNotificationAction(NotificationActionEvent)
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
