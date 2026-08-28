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

// notifTimeFormat — формат времени на карточке. Отдельного поля под него не
// заводим (в отличие от ClockItem.TimeFormat): часы — самостоятельный
// компонент с разными профилями отображения, а тут это деталь одной
// карточки, а не то, чем управляет тема или оболочка.
const notifTimeFormat = "15:04"

// notifCloseSizeDiv — доля высоты карточки под квадрат крестика закрытия.
// Не пиксельный размер (как и делители в tray.go), а пропорция фигуры.
const notifCloseSizeDiv = 3

// NotificationCenter — панель со списком уведомлений.
type NotificationCenter struct {
	*Flyout

	ns Notifications

	mu           sync.Mutex
	unsub        func()
	pressedClose NotificationID // 0 — ничего не нажато (FakeNotifications выдаёт ID с 1)
	pressedClear bool

	// EmptyText — подпись, когда уведомлений нет. Задаётся с осмысленным
	// значением по умолчанию в конструкторе; можно переопределить.
	EmptyText string
}

// NewNotificationCenter создаёт центр уведомлений, оформляемый темой tm и
// читающий список из ns.
func NewNotificationCenter(tm *theme.Manager, ns Notifications) *NotificationCenter {
	nc := &NotificationCenter{
		Flyout:    NewFlyout(tm, ComponentNotifications),
		ns:        ns,
		EmptyText: "Новых уведомлений нет",
	}
	if ns != nil {
		nc.unsub = ns.Subscribe(nc.Invalidate)
	}
	nc.Content = nc.draw
	nc.Size = nc.size
	return nc
}

// Close закрывает панель (как и полагается Flyout) и отписывается от
// Notifications — забытая отписка удерживала бы центр уведомлений в памяти
// у источника вечно, даже после того как панель убрали со сцены.
func (nc *NotificationCenter) Close() {
	nc.Flyout.Close()
	nc.mu.Lock()
	unsub := nc.unsub
	nc.unsub = nil
	nc.mu.Unlock()
	if unsub != nil {
		unsub()
	}
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
	list := nc.list()
	if len(list) == 0 {
		DrawTextCentered(ctx, r, nc.EmptyText, nc.themeStyle(notifPartEmpty, theme.StateNormal))
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
		DrawTextCentered(ctx, layout.clearAll, "Очистить все", style)
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
	timeStr := card.n.Time.Format(notifTimeFormat)
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
