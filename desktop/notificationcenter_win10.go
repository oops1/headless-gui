// notificationcenter_win10.go — центр уведомлений в стиле Windows 10 как вариант
// NotificationCenter: презентер, размещение, жизненный цикл подписок, ввод.
//
// Компонент один. Вариант выбирает презентер, который назначает профиль темы
// (theme.Profile.Presenters["notificationcenter"]); имени темы здесь нет. Всё,
// что общее для двух видов, — открытие, закрытие, анимация, клик мимо — делает
// Flyout; здесь только то, чего плоскому центру не нужно: панель на всю высоту
// у правого края, прокрутка, действия, быстрые действия, клавиатура.
package desktop

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/internal/focusreq"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

func init() {
	RegisterPresenter(theme.NotificationCenterPresenter, ncPresenter{})
}

// ncPresenter — презентер центра уведомлений Windows 10. Рисует и меряет
// компонент, который умеет это делать (*NotificationCenter); любому другому
// отвечает пустотой, и тот рисует сам.
type ncPresenter struct{}

type ncRichComponent interface {
	richSize() image.Point
	richDraw(ctx widget.DrawContext)
}

// Measure возвращает размер панели: ширина из метрики, высота — от верха
// рабочей области до панели задач.
func (ncPresenter) Measure(c Component, _ image.Point) image.Point {
	if rc, ok := c.(ncRichComponent); ok {
		return rc.richSize()
	}
	return image.Point{}
}

// Layout не раскладывает ячейки: у центра свои зоны (см. notifyview.go).
func (ncPresenter) Layout(Component, image.Rectangle) []image.Rectangle { return nil }

// Draw рисует содержимое панели.
func (ncPresenter) Draw(ctx widget.DrawContext, c Component) {
	if rc, ok := c.(ncRichComponent); ok {
		rc.richDraw(ctx)
	}
}

// NotificationCenter показывается презентеру как Component. Ячеек у него нет:
// презентер центра обращается к нему напрямую.
func (nc *NotificationCenter) Cells() []Cell { return nil }

// HoverIndex — Component: наведение хранит вид центра, не ячейки.
func (nc *NotificationCenter) HoverIndex() int { return -1 }

// presenter возвращает презентер, назначенный теме для центра, либо nil —
// рисовать плоский список.
func (nc *NotificationCenter) presenter() Presenter {
	return PresenterFor(nc.Theme(), ComponentNotificationCenter)
}

func (nc *NotificationCenter) richSize() image.Point {
	m := ncReadMetrics(nc.Theme())
	w, h := m.width, ncFallbackHeight
	switch {
	case !nc.WorkArea.Empty():
		h = nc.WorkArea.Dy() - m.margin
		if w > nc.WorkArea.Dx() {
			w = nc.WorkArea.Dx()
		}
	case !nc.Screen.Empty():
		switch {
		case nc.Anchor.Empty():
			h = nc.Screen.Dy()
		case nc.Edge == EdgeTop:
			h = nc.Screen.Max.Y - nc.Anchor.Max.Y - m.margin
		default:
			h = nc.Anchor.Min.Y - nc.Screen.Min.Y - m.margin
		}
		if w > nc.Screen.Dx() {
			w = nc.Screen.Dx()
		}
	}
	if h < 1 {
		h = 1
	}
	return image.Pt(w, h)
}

func (nc *NotificationCenter) richDraw(ctx widget.DrawContext) {
	nc.view.draw(ctx, nc.rect(), nc.fs.FocusVisible())
}

// place — Flyout.Place: центр Windows 10 прижат к правому краю экрана и тянется
// от верха рабочей области до панели задач, а не висит над значком.
func (nc *NotificationCenter) place(anchor, screen image.Rectangle, edge Edge, size image.Point) (image.Rectangle, bool) {
	if nc.presenter() == nil {
		return image.Rectangle{}, false
	}
	margin := nc.metric(KeyNotificationCenterMargin)
	if wa := nc.WorkArea; !wa.Empty() {
		y := wa.Min.Y
		if edge == EdgeTop {
			y = wa.Max.Y - size.Y
		}
		return image.Rect(wa.Max.X-size.X, y, wa.Max.X, y+size.Y), true
	}
	x := screen.Max.X - size.X
	if edge == EdgeTop {
		y := anchor.Max.Y + margin
		return image.Rect(x, y, x+size.X, y+size.Y), true
	}
	y := anchor.Min.Y - margin - size.Y
	return image.Rect(x, y, x+size.X, y+size.Y), true
}

// plate — Flyout.Plate: подложка Windows 10 — часть "panel" с акрилом.
func (nc *NotificationCenter) plate() (string, string) {
	if nc.presenter() == nil {
		return "", ""
	}
	return ComponentNotificationCenter, ncPartPanel
}

// ─── Подписки ────────────────────────────────────────────────────────────────

// SetQuickActions задаёт модель быстрых действий (nil — без них). Плитки
// показывает только центр Windows 10; плоский центр их не рисует.
func (nc *NotificationCenter) SetQuickActions(m QuickActionModel) {
	nc.mu.Lock()
	old := nc.unsubQuick
	nc.unsubQuick = nil
	nc.quick = m
	nc.mu.Unlock()
	if old != nil {
		old()
	}
	if nc.IsOpen() {
		nc.attach()
	}
	nc.Invalidate()
}

// QuickActions возвращает модель быстрых действий.
func (nc *NotificationCenter) QuickActions() QuickActionModel {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	return nc.quick
}

func (nc *NotificationCenter) quickList() []QuickAction {
	if q := nc.QuickActions(); q != nil {
		return q.List()
	}
	return nil
}

// SetQuickExpanded раскрывает сетку быстрых действий или сворачивает её в один
// ряд.
func (nc *NotificationCenter) SetQuickExpanded(open bool) {
	nc.view.mu.Lock()
	changed := nc.view.quickOpen != open
	nc.view.quickOpen = open
	nc.view.mu.Unlock()
	if changed {
		nc.Invalidate()
	}
}

// QuickExpanded сообщает, раскрыта ли сетка быстрых действий.
func (nc *NotificationCenter) QuickExpanded() bool {
	nc.view.mu.Lock()
	defer nc.view.mu.Unlock()
	return nc.view.quickOpen
}

// SetGroupCollapsed сворачивает или раскрывает группу уведомлений приложения.
func (nc *NotificationCenter) SetGroupCollapsed(app AppID, collapsed bool) {
	nc.view.mu.Lock()
	if nc.view.collapsed == nil {
		nc.view.collapsed = map[AppID]bool{}
	}
	changed := nc.view.collapsed[app] != collapsed
	nc.view.collapsed[app] = collapsed
	nc.view.mu.Unlock()
	if changed {
		nc.Invalidate()
	}
}

// GroupCollapsed сообщает, свёрнута ли группа приложения.
func (nc *NotificationCenter) GroupCollapsed(app AppID) bool {
	nc.view.mu.Lock()
	defer nc.view.mu.Unlock()
	return nc.view.collapsed[app]
}

// attach подписывается на источники. Зовётся при открытии: подписка живёт
// ровно столько, сколько панель открыта.
func (nc *NotificationCenter) attach() {
	nc.mu.Lock()
	needNotes := nc.ns != nil && nc.unsub == nil
	needQuick := nc.quick != nil && nc.unsubQuick == nil
	q := nc.quick
	nc.mu.Unlock()

	if needNotes {
		u := nc.ns.Subscribe(nc.onNotesChanged)
		nc.mu.Lock()
		if nc.unsub == nil {
			nc.unsub, u = u, nil
		}
		nc.mu.Unlock()
		if u != nil {
			u()
		}
	}
	if needQuick {
		u := q.Subscribe(nc.onQuickChanged)
		snap := q.List()
		nc.mu.Lock()
		if nc.unsubQuick == nil {
			nc.unsubQuick, u = u, nil
			nc.view.setQuickSnapshot(snap)
		}
		nc.mu.Unlock()
		if u != nil {
			u()
		}
	}
}

// detach снимает подписки: закрытая панель не будит ни источник, ни рендер.
func (nc *NotificationCenter) detach() {
	nc.mu.Lock()
	u1, u2 := nc.unsub, nc.unsubQuick
	nc.unsub, nc.unsubQuick = nil, nil
	nc.mu.Unlock()
	if u1 != nil {
		u1()
	}
	if u2 != nil {
		u2()
	}
}

// onOpenChanged — наблюдатель Flyout: открытие подписывает центр на источники,
// закрытие — отписывает. Это и есть исправление ошибки «после повторного
// открытия список не обновляется»: подписка раньше оформлялась один раз в
// конструкторе, а Close её снимал.
func (nc *NotificationCenter) onOpenChanged(open bool) {
	rich := nc.presenter() != nil
	if open {
		nc.attach()
		if rich {
			nc.view.reset()
			nc.view.prune(nc.list())
			focusreq.Request(nc)
		}
		return
	}
	nc.detach()
	if rich {
		nc.view.reset()
	}
	focusreq.Return(nc)
}

// onNotesChanged — список уведомлений изменился (горутина потребителя).
func (nc *NotificationCenter) onNotesChanged() {
	if nc.presenter() != nil {
		nc.view.prune(nc.list())
	}
	nc.Invalidate()
}

// onQuickChanged — изменились быстрые действия (горутина потребителя). Пока
// набор и порядок плиток прежние, перерисовываются только плитки, у которых
// что-то изменилось: переключение Bluetooth не должно будить размытие всей
// панели.
func (nc *NotificationCenter) onQuickChanged() {
	list := nc.quickList()
	if !nc.IsOpen() {
		return
	}
	rects, ok := nc.view.quickDiff(list)
	if !ok {
		nc.Invalidate()
		return
	}
	for _, r := range rects {
		widget.InvalidateRect(r)
	}
}

// ─── Источник вида ───────────────────────────────────────────────────────────

func (nc *NotificationCenter) viewSource() ncSource {
	return ncSource{
		notes:   nc.list,
		quick:   nc.quickList,
		dismiss: nc.dismiss,
		toggle: func(id QuickActionID) {
			if q := nc.QuickActions(); q != nil {
				q.Toggle(id)
			}
		},
		manage:     nc.manage,
		action:     nc.deliver,
		invalid:    widget.InvalidateRect,
		invalidAll: nc.Invalidate,
		culture:    func() DateCulture { return nc.Culture },
		now:        nc.now,
		emptyMsg:   nc.EmptyLabel,
	}
}

func (nc *NotificationCenter) now() time.Time {
	if nc.Clock != nil {
		return nc.Clock.Now()
	}
	return time.Now()
}

// manage закрывает центр и сообщает оболочке, что нужны настройки уведомлений.
func (nc *NotificationCenter) manage() {
	nc.Close()
	if nc.OnManage != nil {
		nc.OnManage()
	}
}

// deliver отдаёт потребителю действие пользователя и, если действие не
// просит обратного, снимает уведомление. Нажатие самой карточки (Action пусто)
// ещё и закрывает центр: приложение, которое открыли, должно быть видно.
func (nc *NotificationCenter) deliver(ev NotificationActionEvent, keep bool) {
	if nc.OnAction != nil {
		nc.OnAction(ev)
	}
	if a, ok := nc.ns.(NotificationActions); ok {
		a.InvokeNotificationAction(ev)
	}
	if !keep {
		nc.dismiss(ev.Notification)
	}
	if ev.Action == "" {
		nc.Close()
	}
}

// ─── Ввод ────────────────────────────────────────────────────────────────────

// richMouseButton — кнопка мыши в центре Windows 10.
func (nc *NotificationCenter) richMouseButton(e widget.MouseEvent) bool {
	outer := nc.rect()
	pt := image.Pt(e.X, e.Y)
	// Отпускание бегунка, который тянули за край панели, приходит оттуда же —
	// извне панели: его надо принять, иначе бегунок так и остался бы схваченным.
	if !e.Pressed && nc.view.dragging() {
		nc.view.onButton(outer, e)
		return true
	}
	if !pt.In(outer) {
		if e.Pressed {
			nc.Close()
			return true
		}
		return false
	}
	if e.Pressed {
		focusreq.Request(nc)
		if nc.fs.NotePointer(e) {
			nc.Invalidate()
		}
	}
	nc.view.onButton(outer, e)
	if e.Pressed && nc.view.dragging() {
		// Бегунок схвачен: мышь принадлежит ему, пока кнопка не отпущена, и
		// курсор волен выходить за панель (движок снимет захват сам на отпускании).
		nc.mu.Lock()
		cm := nc.capture
		nc.mu.Unlock()
		if cm != nil {
			cm.SetCapture(nc)
		}
	}
	return true
}

// SetCaptureManager реализует widget.CaptureAware: менеджер захвата нужен,
// чтобы тянуть бегунок полосы прокрутки за пределы панели.
func (nc *NotificationCenter) SetCaptureManager(cm widget.CaptureManager) {
	nc.mu.Lock()
	nc.capture = cm
	nc.mu.Unlock()
}

// OnMouseMove подсвечивает элемент под курсором.
func (nc *NotificationCenter) OnMouseMove(x, y int) {
	if !nc.IsOpen() || nc.presenter() == nil {
		return
	}
	nc.view.onMove(nc.rect(), image.Pt(x, y), widget.CursorIsNowhere(x, y))
}

// OnMouseWheelPixels прокручивает список уведомлений.
func (nc *NotificationCenter) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	if !nc.IsOpen() || nc.presenter() == nil {
		return false
	}
	return nc.view.onWheel(nc.rect(), image.Pt(x, y), dy)
}

// OnKeyEvent: в центре Windows 10 — клавиатура панели (обход Tab, стрелки,
// Enter, Delete, ввод ответа); в плоском — только Esc, как у Flyout.
func (nc *NotificationCenter) OnKeyEvent(e widget.KeyEvent) {
	if !nc.IsOpen() || nc.presenter() == nil {
		nc.Flyout.OnKeyEvent(e)
		return
	}
	if nc.view.onKey(nc.rect(), e) {
		if e.Pressed && nc.fs.noteKey() {
			nc.Invalidate()
		}
		return
	}
	if e.Pressed && e.Code == widget.KeyEscape {
		nc.DismissOnEscape()
	}
}

// AcceptsTab реализует widget.TabAcceptor: в открытом центре Tab ходит по его
// элементам, а не уводит фокус из панели (выйти можно по Esc).
func (nc *NotificationCenter) AcceptsTab() bool {
	return nc.IsOpen() && nc.presenter() != nil
}

// SetFocused реализует widget.Focusable.
func (nc *NotificationCenter) SetFocused(focused bool) {
	if nc.fs.Set(focused) {
		nc.Invalidate()
	}
}

// IsFocused реализует widget.Focusable.
func (nc *NotificationCenter) IsFocused() bool { return nc.fs.IsFocused() }

// FocusVisible — видна ли рамка фокуса (фокус пришёл с клавиатуры).
func (nc *NotificationCenter) FocusVisible() bool { return nc.fs.FocusVisible() }

// TabIndex исключает центр из обхода Tab, пока он закрыт или рисуется плоским.
func (nc *NotificationCenter) TabIndex() int {
	if !nc.IsOpen() || nc.presenter() == nil {
		return -1
	}
	return 0
}
