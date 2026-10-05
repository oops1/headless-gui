// notificationcenter.go — центр уведомлений: всплывающая панель со списком
// уведомлений оболочки.
//
// Список приходит через интерфейс Notifications (contract.go) — панель не
// хранит уведомления сама, а перерисовывается по подписке, когда список
// меняется (пришло новое, снято одно, снято всё). Открытие/закрытие, клик
// мимо и Esc — забота базового Flyout (flyout.go); здесь только карточки,
// крестики закрытия и кнопка "Очистить все".
package desktop

import (
	"image"
	"image/color"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentNotifications — имя компонента для стилей темы.
const ComponentNotifications = "notifications"

// Ключи метрик темы, которыми управляется раскладка центра уведомлений.
const (
	// KeyNotificationsWidth — ширина содержимого панели.
	KeyNotificationsWidth theme.Key = "notifications.width"
	// KeyNotificationsCardHeight — высота одной карточки уведомления (и
	// кнопки "Очистить все" — она встаёт в ряд той же высоты).
	KeyNotificationsCardHeight theme.Key = "notifications.card.height"
	// KeyNotificationsGap — зазор между карточками и внутренний отступ
	// крестика закрытия внутри карточки: одна и та же единица разметки
	// используется и снаружи карточек, и внутри — как PadX/PadY стиля
	// используются сразу в нескольких местах компонентов пакета.
	KeyNotificationsGap theme.Key = "notifications.gap"
)

// Части компонента "notifications" для GetStyle.
const (
	notifPartEmpty = "empty" // подпись пустого состояния
	notifPartClear = "clear" // кнопка "Очистить все"
)

// severityPart возвращает часть стиля карточки по важности уведомления.
// Имена — из требования: важность управляет ЧАСТЬЮ стиля, а не состоянием
// (в отличие, например, от календаря, где сегодняшний/выбранный день
// различаются именно состоянием одной и той же части).
func severityPart(sev Severity) string {
	switch sev {
	case SeverityWarning:
		return "card.warning"
	case SeverityError:
		return "card.error"
	default:
		return "card.info"
	}
}

// Формат времени на карточке — от культуры (NotificationCenter.Culture, по
// умолчанию LocaleCulture): «15:04» по-русски, «3:04 PM» по-английски.
// Отдельного поля под формат не заводим (в отличие от ClockItem.TimeFormat):
// это деталь одной карточки, а не то, чем управляет тема или оболочка.

// notifCloseSizeDiv — доля высоты карточки под квадрат крестика закрытия.
// Не пиксельный размер (как и делители в tray.go), а пропорция фигуры.
const notifCloseSizeDiv = 3

// NotificationCenter — панель со списком уведомлений.
//
// Вид выбирает тема. Профили, у которых есть презентер компонента
// "notificationcenter" (Windows 10), получают центр с группами по приложению,
// действиями в карточках, быстрыми действиями и прокруткой — на всю высоту от
// верха рабочей области до панели задач. Остальные темы (Windows 11,
// Windows 2000, macOS) рисуют плоский список карточек, как раньше. Компонент
// имени темы не знает: он спрашивает PresenterFor.
type NotificationCenter struct {
	*Flyout

	ns Notifications

	mu           sync.Mutex
	unsub        func() // подписка на уведомления; есть только пока панель открыта
	unsubQuick   func() // подписка на быстрые действия; то же
	quick        QuickActionModel
	pressedClose NotificationID // 0 — ничего не нажато (FakeNotifications выдаёт ID с 1)
	pressedClear bool

	// EmptyText — подпись, когда уведомлений нет. Пусто — стандартная
	// подпись на текущем языке (ключ StrNotifEmpty), которая следует за
	// widget.SetLanguage; непустое значение показывается как есть.
	EmptyText string

	// Culture — региональные правила времени на карточках; nil —
	// LocaleCulture (из строк движка для текущего языка).
	Culture DateCulture

	// OnManage — нажата ссылка «Управление уведомлениями» (центр Windows 10).
	// Панель к этому моменту уже закрыта: параметры открываются поверх
	// рабочего стола, а не под центром.
	OnManage func()
	// OnAction — пользователь нажал кнопку, ссылку или карточку, ввёл ответ,
	// выбрал пункт списка. Источник уведомлений, реализующий
	// NotificationActions, получает то же событие; нужен любой один путь.
	OnAction func(NotificationActionEvent)
	// Clock — источник «сейчас» для подписи времени карточек (сегодня,
	// вчера); nil — системное время.
	Clock Clock
	// WorkArea — область, которую центр Windows 10 занимает по высоте: от её
	// верха до панели задач. Пусто — высота считается от экрана (Screen) и
	// значка, открывшего панель. Оболочка с несколькими мониторами задаёт сюда
	// рабочую область своего монитора.
	WorkArea image.Rectangle

	fs   FocusState
	view *richView
}

// NewNotificationCenter создаёт центр уведомлений, оформляемый темой tm и
// читающий список из ns.
//
// На источник центр подписывается, когда открывается, и отписывается, когда
// закрывается чем угодно — кнопкой, кликом мимо, Esc. Закрытая панель не будит
// ни рендер, ни источник, а открытая после закрытия получает изменения снова.
func NewNotificationCenter(tm *theme.Manager, ns Notifications) *NotificationCenter {
	nc := &NotificationCenter{
		Flyout: NewFlyout(tm, ComponentNotifications),
		ns:     ns,
	}
	nc.view = newRichView(tm, ncModeCenter, nc.viewSource())
	nc.Content = nc.draw
	nc.Size = nc.size
	nc.Place = nc.place
	nc.Plate = nc.plate
	// Центр уведомлений выезжает справа, из-за края экрана, а не снизу.
	nc.Slide = SlideRight
	nc.SlideDistance = -1
	nc.Flyout.Subscribe(nc.onOpenChanged)
	return nc
}

// EmptyLabel — подпись пустого центра: EmptyText, если задан, иначе
// стандартная на текущем языке.
func (nc *NotificationCenter) EmptyLabel() string {
	if nc.EmptyText != "" {
		return nc.EmptyText
	}
	return tr(StrNotifEmpty)
}

// Close закрывает панель. Отписка от источника уходит вместе с закрытием (см.
// onOpenChanged), а не только отсюда: панель закрывают и со стороны — клик
// мимо, Esc, другая панель, — и забытая подписка удерживала бы центр у
// источника вечно.
func (nc *NotificationCenter) Close() {
	nc.Flyout.Close()
}

func (nc *NotificationCenter) list() []Notification {
	if nc.ns == nil {
		return nil
	}
	return nc.ns.List()
}

func (nc *NotificationCenter) dismiss(id NotificationID) {
	if nc.ns != nil {
		nc.ns.Dismiss(id)
	}
}

// clearAll снимает все текущие уведомления по одному — интерфейс
// Notifications не знает операции "очистить всё разом" (contract.go), так
// что панель просто проходит по списку.
func (nc *NotificationCenter) clearAll() {
	for _, n := range nc.list() {
		nc.dismiss(n.ID)
	}
}

// themeStyle читает стиль части компонента "notifications" (переживает
// tm==nil — как и остальные компоненты пакета).
func (nc *NotificationCenter) themeStyle(part string, st theme.State) *theme.Style {
	tm := nc.Theme()
	if tm == nil {
		return &theme.Style{}
	}
	return tm.GetStyle(nc.Component, part, st)
}

// contentRect — прямоугольник содержимого (то же, что draw получает через
// Content), посчитанный заново для обработки кликов.
func (nc *NotificationCenter) contentRect() image.Rectangle {
	r := nc.rect()
	if r.Empty() {
		return image.Rectangle{}
	}
	pad := int(nc.style(theme.StateNormal).PadX)
	return r.Inset(pad)
}

// size — Flyout.Size: пустая панель — одна строка под EmptyText; иначе
// карточки друг под другом плюс, если карточек больше одной, разделитель и
// кнопка "Очистить все" (прятать её при одном уведомлении незачем — нечего
// очищать оптом).
func (nc *NotificationCenter) size() image.Point {
	if p := nc.presenter(); p != nil {
		return p.Measure(nc, image.Point{})
	}
	width := nc.metric(KeyNotificationsWidth)
	cardH := nc.metric(KeyNotificationsCardHeight)
	gap := nc.metric(KeyNotificationsGap)
	pad := int(nc.style(theme.StateNormal).PadX)

	list := nc.list()
	h := cardH
	if n := len(list); n > 0 {
		h = n*cardH + (n-1)*gap
		if n > 1 {
			h += gap + cardH
		}
	}
	return image.Point{X: width + 2*pad, Y: h + 2*pad}
}

// ─── Раскладка ───────────────────────────────────────────────────────────────

type notifCardLayout struct {
	id        NotificationID
	n         Notification
	rect      image.Rectangle
	closeRect image.Rectangle
}

type notifLayout struct {
	cards    []notifCardLayout
	clearAll image.Rectangle
}

// computeLayout раскладывает карточки в content. draw и OnMouseButton
// вызывают её на одних и тех же данных (list передаётся явно), так что
// прямоугольник крестика под курсором клика всегда совпадает с нарисованным.
func (nc *NotificationCenter) computeLayout(content image.Rectangle, list []Notification) notifLayout {
	cardH := nc.metric(KeyNotificationsCardHeight)
	gap := nc.metric(KeyNotificationsGap)

	y := content.Min.Y
	cards := make([]notifCardLayout, 0, len(list))
	for _, n := range list {
		rect := image.Rect(content.Min.X, y, content.Max.X, y+cardH)
		closeSize := cardH / notifCloseSizeDiv
		closeRect := image.Rect(
			rect.Max.X-closeSize-gap, rect.Min.Y+gap,
			rect.Max.X-gap, rect.Min.Y+gap+closeSize,
		)
		cards = append(cards, notifCardLayout{id: n.ID, n: n, rect: rect, closeRect: closeRect})
		y += cardH + gap
	}

	var clearAll image.Rectangle
	if len(list) > 1 {
		clearAll = image.Rect(content.Min.X, y, content.Max.X, y+cardH)
	}
	return notifLayout{cards: cards, clearAll: clearAll}
}

// ─── Отрисовка ───────────────────────────────────────────────────────────────

func (nc *NotificationCenter) draw(ctx widget.DrawContext, r image.Rectangle) {
	if r.Empty() {
		return
	}
	if p := nc.presenter(); p != nil {
		p.Draw(ctx, nc)
		return
	}
	list := nc.list()
	if len(list) == 0 {
		DrawTextCentered(ctx, r, nc.EmptyLabel(), nc.themeStyle(notifPartEmpty, theme.StateNormal))
		return
	}

	layout := nc.computeLayout(r, list)
	for _, card := range layout.cards {
		style := nc.themeStyle(severityPart(card.n.Severity), theme.StateNormal)
		PaintStyle(ctx, card.rect, style)
		nc.drawCard(ctx, card, style)
	}

	if !layout.clearAll.Empty() {
		style := nc.themeStyle(notifPartClear, theme.StateNormal)
		PaintStyle(ctx, layout.clearAll, style)
		DrawTextCentered(ctx, layout.clearAll, tr(StrNotifClearAll), style)
	}
}

// drawCard рисует одну карточку: заголовок и время в верхней половине,
// текст во второй — тот же приём деления пополам, что и в
// ClockItem.Draw (clock.go) для строк времени/даты. Длинные заголовок и
// текст усекаются многоточием (paint.go: DrawTextLeftElided).
func (nc *NotificationCenter) drawCard(ctx widget.DrawContext, card notifCardLayout, style *theme.Style) {
	half := card.rect.Min.Y + card.rect.Dy()/2
	top := image.Rect(card.rect.Min.X, card.rect.Min.Y, card.closeRect.Min.X, half)
	bottom := image.Rect(card.rect.Min.X, half, card.rect.Max.X, card.rect.Max.Y)

	size := fontSizeOf(style)
	pad := int(style.PadX)
	timeStr := card.n.Time.Format(cultureOrDefault(nc.Culture).TimeFormat())
	timeW := MeasureText(ctx, timeStr, style)
	timeX := top.Max.X - timeW - pad
	timeY := top.Min.Y + (top.Dy()-lineHeight(size))/2
	drawText(ctx, timeStr, timeX, timeY, size, style)

	titleR := image.Rect(top.Min.X, top.Min.Y, timeX-pad, top.Max.Y)
	DrawTextLeftElided(ctx, titleR, card.n.Title, style)

	DrawTextLeftElided(ctx, bottom, card.n.Body, style)

	drawCross(ctx, card.closeRect, ink(style))
}

// drawCross рисует крестик закрытия — две диагонали внутри r. Первую
// диагональ (сверху-слева вниз-направо) рисует drawDiagonal из tray.go
// (тот же приём, что и перечёркивание значка Muted); вторую, зеркальную,
// достраивает сам крестик.
func drawCross(ctx widget.DrawContext, r image.Rectangle, col color.RGBA) {
	drawDiagonal(ctx, r, col)
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}
	for i := 0; i <= r.Dx(); i++ {
		x := r.Max.X - i
		y := r.Min.Y + i*r.Dy()/r.Dx()
		ctx.SetPixel(x, y, col)
	}
}

// ─── Ввод ────────────────────────────────────────────────────────────────────

// OnMouseButton закрывает панель кликом мимо (как Flyout), а внутри
// обрабатывает крестики закрытия и кнопку "Очистить все".
//
// Крестик, как и все закрывающие крестики этого репозитория (см. tray.go,
// trayHandleClick, и комментарий там), срабатывает на ОТПУСКАНИИ, и только
// если курсор всё ещё над тем же крестиком, над которым было нажатие:
// нажать, увести мышь и отпустить — значит передумать.
//
// Своих дочерних виджетов у карточек нет (всё рисует draw вручную), поэтому
// клик по телу панели, не попавший ни в крестик, ни в кнопку, всё равно
// поглощается — иначе он "проваливался" бы сквозь панель.
func (nc *NotificationCenter) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft || !nc.IsOpen() {
		return false
	}
	if nc.presenter() != nil {
		return nc.richMouseButton(e)
	}
	outer := nc.rect()
	pt := image.Pt(e.X, e.Y)
	if !pt.In(outer) {
		if e.Pressed {
			nc.Close()
			return true
		}
		return false
	}

	layout := nc.computeLayout(nc.contentRect(), nc.list())

	if e.Pressed {
		for _, card := range layout.cards {
			if pt.In(card.closeRect) {
				nc.mu.Lock()
				nc.pressedClose = card.id
				nc.mu.Unlock()
				return true
			}
		}
		if !layout.clearAll.Empty() && pt.In(layout.clearAll) {
			nc.mu.Lock()
			nc.pressedClear = true
			nc.mu.Unlock()
			return true
		}
		return true
	}

	nc.mu.Lock()
	wasClose := nc.pressedClose
	wasClear := nc.pressedClear
	nc.pressedClose = 0
	nc.pressedClear = false
	nc.mu.Unlock()

	if wasClose != 0 {
		for _, card := range layout.cards {
			if card.id == wasClose && pt.In(card.closeRect) {
				nc.dismiss(wasClose)
				break
			}
		}
		return true
	}
	if wasClear && !layout.clearAll.Empty() && pt.In(layout.clearAll) {
		nc.clearAll()
		return true
	}
	return true
}
