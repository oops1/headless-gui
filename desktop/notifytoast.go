// notifytoast.go — тост: карточка нового уведомления над треем.
//
// Рисуется тем же видом, что и карточка в центре уведомлений (notifyview.go):
// значок, заголовок, текст, время, действия, — на подложке "toast". Появляется,
// когда в источнике возникает новое уведомление, выезжает справа, висит
// заданное время и уходит. Пока над ним мышь, он не уходит.
//
// Тост не регистрируется в FlyoutManager: он не «открыл одну — остальные
// закрылись» (приход письма не должен закрывать меню «Пуск»), клик мимо и Esc
// его не закрывают. Оболочка кладёт его в корень дерева после менеджера панелей
// и вызывает Suppress, чтобы тост молчал, пока открыт центр уведомлений.
package desktop

import (
	"image"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// NotificationToast — всплывающая карточка нового уведомления.
//
// Работает только там, где тема назначила презентер центра уведомлений
// (Windows 10): у плоских тем нет оформления карточки, и тост не показывается.
type NotificationToast struct {
	*Flyout

	ns Notifications

	// Timeout — сколько тост висит; 0 — по метрике темы
	// notificationcenter.toast.timeout (миллисекунды), отрицательное — не
	// уходит сам.
	Timeout time.Duration
	// OnAction — действие пользователя в тосте; то же, что у центра.
	OnAction func(NotificationActionEvent)
	// Clock и Culture — как у NotificationCenter.
	Clock   Clock
	Culture DateCulture
	// WorkArea — область над панелью задач, в правом нижнем углу которой
	// стоит тост. Пусто — от экрана (Screen) и значка (Anchor).
	WorkArea image.Rectangle

	view *richView

	mu         sync.Mutex
	unsub      func()
	seen       map[NotificationID]bool
	current    NotificationID
	shown      *Notification // последняя показанная карточка (для анимации ухода)
	timer      *time.Timer
	hovered    bool
	suppressed int
	unwatch    []func()
	dnd        DoNotDisturb // режим «Не беспокоить»: пока включён, тост молчит
	unsubDND   func()
}

// NewNotificationToast создаёт тост, следящий за источником ns. Уведомления,
// уже лежащие в источнике, тостом не показываются — только новые.
func NewNotificationToast(tm *theme.Manager, ns Notifications) *NotificationToast {
	t := &NotificationToast{
		Flyout: NewFlyout(tm, ComponentNotificationCenter),
		ns:     ns,
		seen:   map[NotificationID]bool{},
	}
	t.view = newRichView(tm, ncModeToast, ncSource{
		notes:      t.notes,
		dismiss:    func(NotificationID) { t.Hide() },
		action:     t.deliver,
		invalid:    widget.InvalidateRect,
		invalidAll: t.Invalidate,
		culture:    func() DateCulture { return t.Culture },
		now:        t.now,
	})
	t.Content = t.draw
	t.Size = t.size
	t.Place = t.place
	t.Plate = func() (string, string) { return ComponentNotificationCenter, ncPartToast }
	t.Slide = SlideRight
	t.SlideDistance = 0
	if ns != nil {
		for _, n := range ns.List() {
			t.seen[n.ID] = true
		}
		t.unsub = ns.Subscribe(t.onChanged)
	}
	return t
}

// SetDoNotDisturb задаёт модель «Не беспокоить» (nil — тост показывается всегда).
// Пока режим включён, новые уведомления тостом не показываются, а в центре
// уведомлений остаются: тост — предложение отвлечься, и режим его отменяет.
// Уже показанный тост режим убирает.
func (t *NotificationToast) SetDoNotDisturb(d DoNotDisturb) {
	t.mu.Lock()
	t.dnd = d
	old := t.unsubDND
	t.unsubDND = nil
	t.mu.Unlock()
	if old != nil {
		old()
	}
	if d == nil {
		return
	}
	u := d.Subscribe(func() {
		if d.Enabled() {
			t.Hide()
		}
	})
	t.mu.Lock()
	t.unsubDND = u
	t.mu.Unlock()
	if d.Enabled() {
		t.Hide()
	}
}

// muted — включён ли режим «Не беспокоить».
func (t *NotificationToast) muted() bool {
	t.mu.Lock()
	d := t.dnd
	t.mu.Unlock()
	return d != nil && d.Enabled()
}

// Close прекращает слежение за источником и убирает тост. После Close тост не
// оживёт: забытая подписка держала бы его у источника вечно.
func (t *NotificationToast) Close() {
	t.mu.Lock()
	u := t.unsub
	t.unsub = nil
	for _, w := range t.unwatch {
		w()
	}
	t.unwatch = nil
	du := t.unsubDND
	t.unsubDND = nil
	t.mu.Unlock()
	if u != nil {
		u()
	}
	if du != nil {
		du()
	}
	t.Hide()
}

// Suppress велит тосту молчать, пока открыта любая из панелей: открытая
// панель закрывает тост, а новые уведомления за это время им не показываются
// (они и так видны в центре).
func (t *NotificationToast) Suppress(panels ...FlyoutPanel) {
	for _, p := range panels {
		fl := p.AsFlyout()
		if fl == nil {
			continue
		}
		open := fl.IsOpen()
		t.mu.Lock()
		if open {
			t.suppressed++
		}
		t.mu.Unlock()
		if open {
			t.Hide()
		}
		unsub := fl.Subscribe(func(o bool) {
			t.mu.Lock()
			if o {
				t.suppressed++
			} else if t.suppressed > 0 {
				t.suppressed--
			}
			t.mu.Unlock()
			if o {
				t.Hide()
			}
		})
		t.mu.Lock()
		t.unwatch = append(t.unwatch, unsub)
		t.mu.Unlock()
	}
}

// Hide убирает тост, не трогая уведомление в источнике.
func (t *NotificationToast) Hide() {
	t.mu.Lock()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	t.hovered = false
	t.mu.Unlock()
	// Закрываем ДО того, как забыть карточку: область закрываемой панели
	// считается по её размеру, а размер — по показанной карточке.
	t.Flyout.Close()
	t.mu.Lock()
	t.current = 0
	t.mu.Unlock()
	t.view.reset()
}

// Current возвращает уведомление, которое показывает тост (0 — ничего).
func (t *NotificationToast) Current() NotificationID {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.current
}

func (t *NotificationToast) now() time.Time {
	if t.Clock != nil {
		return t.Clock.Now()
	}
	return time.Now()
}

// notes — единственное уведомление, которое сейчас показывается.
//
// Закрытый тост, который ещё уезжает, дорисовывает последнюю показанную
// карточку: без неё уход был бы мгновенным.
func (t *NotificationToast) notes() []Notification {
	id := t.Current()
	if id != 0 && t.ns != nil {
		for _, n := range t.ns.List() {
			if n.ID == id {
				t.mu.Lock()
				t.shown = &n
				t.mu.Unlock()
				return []Notification{n}
			}
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if id == 0 && t.shown != nil && t.IsAnimating() {
		return []Notification{*t.shown}
	}
	return nil
}

func (t *NotificationToast) presenter() Presenter {
	return PresenterFor(t.Theme(), ComponentNotificationCenter)
}

// onChanged — в источнике что-то изменилось (горутина потребителя): показать
// самое новое из ещё не показанных; если показываемое исчезло — убрать тост.
func (t *NotificationToast) onChanged() {
	if t.ns == nil {
		return
	}
	list := t.ns.List()
	live := make(map[NotificationID]bool, len(list))
	var fresh *Notification
	t.mu.Lock()
	for i := range list {
		n := list[i]
		live[n.ID] = true
		if !t.seen[n.ID] {
			t.seen[n.ID] = true
			if fresh == nil || n.At().After(fresh.At()) || (n.At().Equal(fresh.At()) && n.ID > fresh.ID) {
				fresh = &list[i]
			}
		}
	}
	for id := range t.seen {
		if !live[id] {
			delete(t.seen, id)
		}
	}
	cur := t.current
	muted := t.suppressed > 0
	t.mu.Unlock()

	if cur != 0 && !live[cur] {
		t.Hide()
		cur = 0
	}
	if fresh == nil || muted || t.muted() || t.presenter() == nil {
		return
	}
	t.show(fresh.ID)
}

// show показывает уведомление id.
func (t *NotificationToast) show(id NotificationID) {
	t.mu.Lock()
	t.current = id
	t.hovered = false
	t.mu.Unlock()
	t.view.reset()
	t.Flyout.Close() // сменить карточку: прежняя уходит, новая встаёт заново
	t.Flyout.Open(t.Anchor)
	t.armTimer()
}

// timeout — сколько тосту висеть.
func (t *NotificationToast) timeout() time.Duration {
	if t.Timeout != 0 {
		return t.Timeout
	}
	if ms := t.metric(KeyNotificationToastTimeout); ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return 5 * time.Second
}

func (t *NotificationToast) armTimer() {
	d := t.timeout()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	if d < 0 || t.hovered || t.current == 0 {
		return
	}
	id := t.current
	t.timer = time.AfterFunc(d, func() {
		if t.Current() == id {
			t.Hide()
		}
	})
}

// deliver — пользователь нажал действие в тосте.
func (t *NotificationToast) deliver(ev NotificationActionEvent, keep bool) {
	if t.OnAction != nil {
		t.OnAction(ev)
	}
	if a, ok := t.ns.(NotificationActions); ok {
		a.InvokeNotificationAction(ev)
	}
	if !keep && t.ns != nil {
		t.ns.Dismiss(ev.Notification)
	}
	if ev.Action == "" || !keep {
		t.Hide()
	}
}

// size: ширина из метрики, высота — по карточке.
func (t *NotificationToast) size() image.Point {
	m := ncReadMetrics(t.Theme())
	n := t.notes()
	if len(n) == 0 {
		return image.Point{}
	}
	f := t.view.fonts()
	t.view.mu.Lock()
	iconArg := hasNoteIcon(n[0])
	if t.view.w11() {
		iconArg = true // тост Windows 11 — карточка со строкой приложения
	}
	c := t.view.layoutCard(m, f, n[0], 0, 0, m.toastW, iconArg)
	t.view.mu.Unlock()
	return image.Pt(m.toastW, c.rect.Dy())
}

// place ставит тост в правый нижний угол над панелью задач.
func (t *NotificationToast) place(anchor, screen image.Rectangle, edge Edge, size image.Point) (image.Rectangle, bool) {
	margin := t.metric(ncKeyToastMargin)
	if wa := t.WorkArea; !wa.Empty() {
		x := wa.Max.X - size.X - margin
		if edge == EdgeTop {
			y := wa.Min.Y + margin
			return image.Rect(x, y, x+size.X, y+size.Y), true
		}
		y := wa.Max.Y - size.Y - margin
		return image.Rect(x, y, x+size.X, y+size.Y), true
	}
	x := screen.Max.X - size.X - margin
	if edge == EdgeTop {
		y := anchor.Max.Y + margin
		return image.Rect(x, y, x+size.X, y+size.Y), true
	}
	y := anchor.Min.Y - size.Y - margin
	return image.Rect(x, y, x+size.X, y+size.Y), true
}

func (t *NotificationToast) draw(ctx widget.DrawContext, r image.Rectangle) {
	if r.Empty() {
		return
	}
	t.view.draw(ctx, t.rect(), false)
}

// ─── Ввод ────────────────────────────────────────────────────────────────────

// DismissAt: клик мимо тоста его не закрывает.
func (t *NotificationToast) DismissAt(x, y int) {}

// DismissOnEscape: Esc принадлежит другим панелям, тост его не перехватывает.
func (t *NotificationToast) DismissOnEscape() bool { return false }

// OnKeyEvent: клавиатура тосту не нужна.
func (t *NotificationToast) OnKeyEvent(widget.KeyEvent) {}

// OnMouseButton разбирает нажатие внутри тоста; снаружи он событий не трогает.
func (t *NotificationToast) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft || !t.IsOpen() {
		return false
	}
	r := t.rect()
	if !image.Pt(e.X, e.Y).In(r) {
		return false
	}
	t.view.onButton(r, e)
	return true
}

// OnMouseMove подсвечивает элемент под курсором и приостанавливает таймер,
// пока мышь над тостом.
func (t *NotificationToast) OnMouseMove(x, y int) {
	if !t.IsOpen() {
		return
	}
	r := t.rect()
	pt := image.Pt(x, y)
	inside := !widget.CursorIsNowhere(x, y) && pt.In(r)
	t.view.onMove(r, pt, !inside)

	t.mu.Lock()
	was := t.hovered
	t.hovered = inside
	t.mu.Unlock()
	switch {
	case inside && !was:
		t.mu.Lock()
		if t.timer != nil {
			t.timer.Stop()
			t.timer = nil
		}
		t.mu.Unlock()
	case !inside && was:
		t.armTimer()
	}
}
