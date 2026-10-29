// events.go — диспетчер событий ввода (мышь, клавиатура, фокус).

package input

import (
	"image"
	"math"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Focus manager ───────────────────────────────────────────────────────────

// focusManager хранит текущий виджет с фокусом и управляет передачей фокуса.
//
// SetFocused виджетов зовётся ВНЕ замка (pixgo E19). Виджет вправе внутри
// SetFocused перевести фокус (SetFocus), спросить его (Focused, CaretRect) —
// под замком это ждало само себя навсегда: так зависало поле адреса галереи
// PixGo 1.0.0, по потере фокуса заканчивавшее правку и звавшее SetFocus, и
// цикл событий Wayland (textInputSendCaret → CaretRect → get).
type focusManager struct {
	mu      sync.Mutex
	focused widget.Widget // nil — нет фокуса
	// believer — виджет, которому последним сказали SetFocused(true) и ещё
	// не сказали false; notifying — уведомления уже раздаёт другой вызов set
	// (выше по стеку или в другой горутине): новое значение он доведёт сам.
	believer  widget.Widget
	notifying bool
}

// focusNotifyRounds ограничивает круги уведомлений, если виджеты перекидывают
// фокус друг другу из SetFocused без конца.
const focusNotifyRounds = 16

// set устанавливает фокус на w; снимает фокус с предыдущего (если реализует Focusable).
//
// Под замком меняется только «кто в фокусе». Уведомления идут после Unlock;
// если за время уведомлений фокус сменился ещё раз, применяется последнее
// значение: старый получает false, последний — true, промежуточные — ничего.
func (fm *focusManager) set(w widget.Widget) {
	fm.mu.Lock()
	if fm.focused == w {
		fm.mu.Unlock()
		return
	}
	fm.focused = w
	if fm.notifying {
		fm.mu.Unlock()
		return
	}
	fm.notifying = true
	for round := 0; round < focusNotifyRounds && fm.believer != fm.focused; round++ {
		old := fm.believer
		fm.believer = nil
		fm.mu.Unlock()
		if f, ok := old.(widget.Focusable); ok && old != nil {
			f.SetFocused(false)
		}
		fm.mu.Lock()
		target := fm.focused
		fm.believer = target
		fm.mu.Unlock()
		if f, ok := target.(widget.Focusable); ok && target != nil {
			f.SetFocused(true)
		}
		fm.mu.Lock()
	}
	fm.notifying = false
	fm.mu.Unlock()
}

func (fm *focusManager) get() widget.Widget {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return fm.focused
}

// ─── Mouse Capture ──────────────────────────────────────────────────────────

// SetCapture направляет все события мыши на указанный виджет.
func (d *Dispatcher) SetCapture(w widget.Widget) {
	d.capMu.Lock()
	d.captured = w
	// Цепочку контейнеров со сдвигом над захватчиком найдём при ближайшем
	// событии (captureOffset): здесь нас могли позвать из обработчика виджета,
	// держащего свои замки, а поиск обходит дерево.
	d.capChain, d.capChainOK = nil, false
	d.capMu.Unlock()
}

// ReleaseCapture отменяет захват мыши.
func (d *Dispatcher) ReleaseCapture() {
	d.capMu.Lock()
	d.captured = nil
	d.capChain, d.capChainOK = nil, false
	d.capMu.Unlock()
}

func (d *Dispatcher) getCaptured() widget.Widget {
	d.capMu.Lock()
	w := d.captured
	d.capMu.Unlock()
	return w
}

// ─── SetFocus / SendKeyEvent ─────────────────────────────────────────────────

// SetFocus передаёт фокус ввода виджету w.
// Если w == nil — фокус снимается со всех виджетов.
func (d *Dispatcher) SetFocus(w widget.Widget) {
	d.setFocusInvalidating(w)
}

// setFocusInvalidating переводит фокус и точечно инвалидирует области старого
// и нового виджета (рамка фокуса). Виджеты дополнительно самоинвалидируются
// в SetFocused — двойная инвалидация дешёвая (объединение damage-областей).
func (d *Dispatcher) setFocusInvalidating(w widget.Widget) {
	// Границы виджета — в его кадре, а инвалидировать надо экранное место: у
	// виджета внутри прокрутки это разные прямоугольники (см. frames.go).
	if old := d.focus.get(); old != nil {
		d.host.InvalidateRect(d.screenBoundsOf(old))
	}
	d.focus.set(w)
	if w != nil {
		d.host.InvalidateRect(d.screenBoundsOf(w))
	}
}

// SendKeyEvent доставляет клавиатурное событие виджету с фокусом.
// Tab / Shift+Tab перехватываются для переключения фокуса между виджетами.
//
// Полной инвалидации здесь нет: виджет с фокусом самоинвалидируется при
// изменении своего состояния (текст, каретка, выделение), Tab-навигация
// инвалидирует старый/новый фокус точечно, командные хоткеи — полностью
// (команда может изменить что угодно).
func (d *Dispatcher) SendKeyEvent(ev widget.KeyEvent) {
	defer d.host.GuardInput("SendKeyEvent")()
	defer d.serveFocusRequests()()

	// Tab-навигация: перехватываем Tab до доставки виджету — если только
	// фокусный виджет не забирает его себе (widget.TabAcceptor: редактор кода).
	// Ctrl+Tab остаётся навигацией и у такого виджета.
	// При активном модальном виджете Tab циклит только внутри него.
	if ev.Code == widget.KeyTab && ev.Pressed && !widget.WantsTab(d.focus.get(), ev) {
		var tabRoot widget.Widget
		if m := d.host.TopModal(); m != nil {
			tabRoot = m
		} else {
			tabRoot = d.host.Root()
		}
		if tabRoot != nil {
			reverse := ev.Mod&widget.ModShift != 0
			d.tabCycle(tabRoot, reverse)
		}
		return
	}

	// Escape закрывает верхний модальный виджет (и сообщает об отмене, если
	// диалог задал CancelAction — например, InputDialog возвращает ok=false).
	if ev.Code == widget.KeyEscape && ev.Pressed {
		if m := d.host.TopModal(); m != nil {
			// Тем же путём, что и ✕ (requestCloseModal): иначе диалог,
			// отказавшийся закрываться по крестику, закрывался бы по Escape.
			d.host.RequestCloseModal(m)
			return
		}
	}

	// Escape закрывает всплывающие панели (widget.EscapeDismisser) — тем, у
	// кого может не быть фокуса вовсе, например меню «Пуск» и центр
	// уведомлений. Один Esc закрывает один слой. Если фокус у виджета с
	// собственным открытым оверлеем (выпадающий список, меню), Esc принадлежит
	// ему: сначала закрывается он.
	if ev.Code == widget.KeyEscape && ev.Pressed && d.dismissOnEscape() {
		return
	}

	// Горячие клавиши окна (WPF InputBindings/KeyBinding) — до фокус-диспатча.
	if ev.Pressed {
		var hostRoot widget.Widget
		if m := d.host.TopModal(); m != nil {
			hostRoot = m
		} else {
			hostRoot = d.host.Root()
		}
		if h, ok := hostRoot.(interface {
			HandleInputBinding(widget.KeyCode, widget.KeyMod) bool
		}); ok {
			if h.HandleInputBinding(ev.Code, ev.Mod) {
				// Команда хоткея может изменить произвольную часть UI.
				d.host.Invalidate()
				return
			}
		}
	}

	w := d.focus.get()
	// Пока открыто модальное окно, клавиши получает только виджет внутри него.
	// Фокус за диалогом не снимается — после закрытия ввод вернётся туда же, —
	// но и клавиш, пока диалог открыт, не получает: иначе пароль, набираемый
	// в диалоге защищённого рабочего стола, мог уйти окну программы под ним
	// (ADR-0002, 2.7).
	if m := d.host.TopModal(); m != nil && w != nil && !widgetInTree(m, w) {
		w = nil
	}
	if w == nil {
		// Фокуса нет — клавиша корню (или верхней модалке), если он её ждёт
		// (widget.UnfocusedKeyHandler). Без него — как раньше, никому.
		var host widget.Widget
		if m := d.host.TopModal(); m != nil {
			host = m
		} else {
			host = d.host.Root()
		}
		if uk, ok := host.(widget.UnfocusedKeyHandler); ok {
			uk.OnUnfocusedKey(ev)
		}
		return
	}
	if kh, ok := w.(widget.KeyHandler); ok {
		kh.OnKeyEvent(ev)
	}
}

// dismissOnEscape закрывает верхнюю панель, закрывающуюся по Esc. Возвращает
// true, если что-то закрыто и клавишу дальше отдавать не нужно.
func (d *Dispatcher) dismissOnEscape() bool {
	if w := d.focus.get(); w != nil {
		if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
			return false
		}
	}
	root := d.host.Root()
	if root == nil {
		return false
	}
	return widget.DismissOnEscape(root)
}

// tabCycle переключает фокус на следующий (или предыдущий) Focusable-виджет.
func (d *Dispatcher) tabCycle(root widget.Widget, reverse bool) {
	all := widget.CollectFocusables(root)
	if len(all) == 0 {
		return
	}

	current := d.focus.get()
	idx := -1
	for i, w := range all {
		if w == current {
			idx = i
			break
		}
	}

	var next int
	if idx < 0 {
		// Нет текущего фокуса — ставим на первый/последний
		if reverse {
			next = len(all) - 1
		} else {
			next = 0
		}
	} else if reverse {
		next = (idx - 1 + len(all)) % len(all)
	} else {
		next = (idx + 1) % len(all)
	}

	d.setFocusInvalidating(all[next])
}

// toLogical переводит физические координаты события (пиксели окна/кадра)
// в логические координаты виджетов (HiDPI). При Scale == 1 тождественно.
// Lock-free (scaleBits) — события не конкурируют с мьютексом движка.
//
// Округление вниз, а не к нулю: точка «-1» (вне окна — так окно сообщает об
// уходе указателя) на масштабе 2 при усечении к нулю превращалась в 0 — первую
// точку окна, и наведение с уходом не снималось. Для неотрицательных координат
// результат прежний.
func (d *Dispatcher) toLogical(x, y int) (int, int) {
	k := d.host.Scale()
	if k == 1 {
		return x, y
	}
	return int(math.Floor(float64(x) / k)), int(math.Floor(float64(y) / k))
}

// CursorAt возвращает форму курсора для точки (x, y — физические пиксели):
// курсор самого глубокого виджета под точкой, реализующего
// widget.CursorProvider (иначе — стрелка).
func (d *Dispatcher) CursorAt(x, y int) widget.Cursor {
	// Принудительный курсор сильнее любого виджета и модального окна.
	if c, ok := d.CursorOverride(); ok {
		return c
	}
	x, y = d.toLogical(x, y)
	var disp widget.Widget
	if m := d.host.TopModal(); m != nil {
		disp = m
	} else {
		disp = d.host.Root()
	}
	if disp == nil {
		return widget.CursorArrow
	}
	path := HitPath(disp, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		if ov, ok := path[i].W.(interface{ CursorOverride() (widget.Cursor, bool) }); ok {
			if c, has := ov.CursorOverride(); has {
				return c
			}
		}
		if cp, ok := path[i].W.(widget.CursorProvider); ok {
			// Виджет судит о точке в своём кадре (у ребёнка прокрутки — в
			// координатах содержимого).
			return cp.Cursor(x+path[i].Off.X, y+path[i].Off.Y)
		}
	}
	return widget.CursorArrow
}

// ─── Mouse events ────────────────────────────────────────────────────────────

// SendMouseMove уведомляет всё дерево виджетов о перемещении курсора в (x, y).
// Если есть виджет, захвативший мышь — событие идёт только ему.
// Если активен модальный виджет — broadcast только внутри него.
// Иначе — broadcast всему дереву.
//
// Полной инвалидации здесь больше нет: hover-изменения виджеты сообщают сами
// (Base.Invalidate при фактической смене состояния), drag двигает панели через
// SetBounds (авто-инвалидация old∪new). Кадры рендерятся только когда картинка
// действительно меняется.
func (d *Dispatcher) SendMouseMove(x, y int) {
	defer d.host.GuardInput("SendMouseMove")()
	x, y = d.toLogical(x, y)

	// Если на экране висит подсказка — стираем её (движение мыши сбрасывает
	// таймер, следующий кадр рисуется без плашки).
	d.host.InvalidateShownTooltip()

	// Запоминаем позицию курсора и сбрасываем таймер всплывающей подсказки.
	d.host.RecordMouse(x, y)

	// Прежняя позиция курсора — для адресной доставки (см. broadcastMouseMove):
	// виджету, из-под которого курсор ушёл, событие тоже нужно (снять hover).
	ox, oy := d.lastMoveX, d.lastMoveY
	if !d.hasLastMove {
		ox, oy = x, y
		d.hasLastMove = true
	}
	d.lastMoveX, d.lastMoveY = x, y

	// Если мышь захвачена — только захватчику
	if cap := d.getCaptured(); cap != nil {
		if mm, ok := cap.(widget.MouseMoveHandler); ok {
			// В кадре захватчика: перетаскивание внутри прокрутки идёт в
			// координатах содержимого, а курсор — экранный.
			deliverMove(mm, x, y, d.captureOffset(cap))
		}
		d.routePointerOwner(x, y, nil, cap, nil)
		return
	}

	// Модальный виджет: ограничиваем broadcast
	if m := d.host.TopModal(); m != nil {
		broadcastMouseMove(m, ox, oy, x, y)
		d.routePointerOwner(x, y, m, nil, nil)
		return
	}

	root := d.host.Root()
	if root == nil {
		return
	}

	// Открытый оверлей старше обычного Z-порядка дерева — ровно как при
	// нажатии (см. SendMouseButton). Движение под меню, календарём или
	// раскрытым списком принадлежит им, а не тому, что они накрыли: иначе
	// кнопка панели задач под меню «Пуск» исправно подсвечивалась, и сквозь
	// стеклянную панель Windows 11 эта подсветка была видна.
	if ov, ovOff := findOverlayStep(root, x, y); ov != nil {
		// Сначала — всему дереву «курсора над вами нет». Без этого кнопка, с
		// которой курсор ушёл под оверлей, осталась бы подсвеченной навсегда:
		// она бы просто перестала получать события.
		//
		// Кроме самого владельца оверлея и его поддерева: им следом приходит
		// настоящая точка. «Курсора нет» перед ней владелец принимал за
		// движение — меню «Пуск» Windows 10 видело скачок на 2^30 точек от
		// нажатия и начинало перенос плитки, так что щелчок по плитке с
		// малейшим дрожанием руки переставлял её вместо запуска.
		broadcastMouseMoveSkip(root, ox, oy, widget.CursorNowhere, widget.CursorNowhere, ov)
		// Затем — настоящая точка тому, кому она принадлежит, и его детям. Точки
		// переводятся в кадр владельца оверлея: он может лежать в прокрутке.
		broadcastMouseMoveFrame(ov, ox, oy, x, y, ovOff)
		d.routePointerOwner(x, y, root, nil, ov)
		return
	}

	broadcastMouseMove(root, ox, oy, x, y)
	d.routePointerOwner(x, y, root, nil, nil)
}

// SendMouseButton уведомляет дерево о нажатии/отпускании кнопки мыши в (x, y).
// Если мышь захвачена — событие идёт только захватчику.
// Иначе: проверяем, хочет ли какой-либо предок захватить мышь (WantsCapture),
// затем передаём событие самому верхнему виджету под курсором.
func (d *Dispatcher) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool) {
	defer d.host.GuardInput("SendMouseButton")()
	defer d.serveFocusRequests()()
	x, y = d.toLogical(x, y)

	// Клик оставляет ПОЛНУЮ инвалидацию сознательно: он может открыть/закрыть
	// overlay (dropdown, меню), сместить фокус, выполнить команду — задеть
	// произвольные области. Клики редки, полный кадр здесь дёшев и надёжен.
	d.host.Invalidate()
	ev := widget.MouseEvent{X: x, Y: y, Button: btn, Pressed: pressed, Mod: d.Modifiers()}
	ev.Clicks = d.countClick(x, y, btn, pressed)

	// Новое нажатие: открываем его номер ДО гашения overlay'ев (DismissOutside
	// ниже) — кнопка, владеющая меню из чужого поддерева, по этому номеру
	// узнаёт, что меню погасил её собственный клик. См. widget.BumpPressSeq.
	if pressed && btn == widget.MouseLeft {
		widget.BumpPressSeq()
	}

	// Если мышь захвачена — только захватчику.
	// ВАЖНО: эта проверка ПЕРЕД pressConsumer, потому что capture-виджет
	// (TextInput, Slider) ожидает release для освобождения захвата.
	// Если pressConsumer проглотит release до capture — захват залипнет
	// и мышь перестанет работать.
	if cap := d.getCaptured(); cap != nil {
		// Сбрасываем pressConsumer — capture-виджет обработает release сам.
		if !pressed && btn == widget.MouseLeft {
			d.pressConsumer = nil
		}
		if mc, ok := cap.(widget.MouseClickHandler); ok {
			deliverButton(mc, ev, d.captureOffset(cap))
		}
		// Движок гарантирует снятие capture при отпускании ЛКМ —
		// даже если виджет не вызвал ReleaseCapture (например, capMgr == nil).
		if !pressed && btn == widget.MouseLeft {
			d.ReleaseCapture()
		}
		return
	}

	// Если предыдущий press был поглощён виджетом, а этот виджет
	// больше не находится под курсором (был закрыт/удалён) — проглатываем
	// release, чтобы он не попал на виджет под закрывшимся окном.
	if !pressed && btn == widget.MouseLeft && d.pressConsumer != nil {
		consumer := d.pressConsumer
		d.pressConsumer = nil

		// Проверяем, есть ли ещё поглотитель в пути под курсором
		var dispRoot widget.Widget
		if m := d.host.TopModal(); m != nil {
			dispRoot = m
		} else {
			dispRoot = d.host.Root()
		}
		if dispRoot != nil {
			path := HitPath(dispRoot, x, y)
			found := false
			for _, st := range path {
				if st.W == consumer {
					found = true
					break
				}
			}
			// Если не найден в обычном дереве — проверяем overlay-виджеты.
			// MenuBar (и другие OverlayDrawer) владеют popup как полем, а не дочерним
			// виджетом, поэтому hitTestPath их не находит в области popup'а.
			if !found {
				if od, ok := consumer.(widget.OverlayDrawer); ok && od.HasOverlay() {
					// По области ОВЕРЛЕЯ, а не владельца: меню открывается
					// рядом с виджетом и почти всегда вылезает за него, а
					// отпускание над меню обязано дойти до меню — именно на
					// отпускании пункт и срабатывает.
					// Прямоугольник оверлея — в кадре его владельца.
					if image.Pt(x, y).Add(d.frameOffsetOf(consumer)).In(OverlayHitRect(consumer)) {
						found = true
					}
				}
			}
			if !found {
				// Виджет-поглотитель исчез — проглатываем release
				return
			}
		}
	}

	// Определяем корень для dispatch'а: модальный виджет или root
	var dispatchRoot widget.Widget
	if m := d.host.TopModal(); m != nil {
		dispatchRoot = m
	} else {
		dispatchRoot = d.host.Root()
	}
	if dispatchRoot == nil {
		return
	}

	// Правый клик (по отпусканию, как в ОС): показываем привязанное контекстное
	// меню (WPF ContextMenu) самого глубокого виджета под курсором.
	if !pressed && btn == widget.MouseRight {
		path := HitPath(dispatchRoot, x, y)
		for i := len(path) - 1; i >= 0; i-- {
			// Виджет судит о точке в своём кадре, и меню, которое он строит,
			// живёт там же: рисуется оно через тот же сдвиг, что и сам виджет.
			fx, fy := x+path[i].Off.X, y+path[i].Off.Y
			// Сперва — меню, собираемое под точкой: список и таблица строят
			// его по строке под курсором, и одного готового меню на виджет им
			// не хватает. Готовое ContextMenu — частный случай, когда меню от
			// точки не зависит, поэтому оно идёт вторым.
			if h, ok := path[i].W.(widget.ContextMenuProvider); ok {
				if pm := h.ContextMenuAt(fx, fy); pm != nil {
					showMenuInFrame(pm, fx, fy, path[i].Off)
					return
				}
			}
			if h, ok := path[i].W.(interface{ GetContextMenu() *widget.PopupMenu }); ok {
				if pm := h.GetContextMenu(); pm != nil {
					// Готовое меню — самостоятельный виджет дерева (обычно
					// ребёнок того, к кому оно приколото), и кадр у него свой.
					moff := d.frameOffsetOf(pm)
					showMenuInFrame(pm, x+moff.X, y+moff.Y, moff)
					return
				}
			}
		}
	}

	// Открытый overlay (popup-меню, раскрытый dropdown) старше и обычного
	// Z-порядка дерева, и заявки на захват мыши: он нарисован поверх всего,
	// значит и клик по нему принадлежит ему.
	//
	// Проверять его НУЖНО до поиска захватчика. Виджет под меню — титлбар
	// окна, вьюха терминала — просит захват на любое нажатие в своих
	// границах и находится первым просто потому, что лежит ниже; ветка
	// захвата гасит меню (DismissOutside по пути к захватчику, меню в него
	// не входит), и до пункта меню нажатие не доходит вовсе. Меню, открытое
	// над окном, из-за этого было полностью мёртвым — а над пустым рабочим
	// столом, где захват никому не нужен, работало.
	if overlayW, overlayOff := findOverlayStep(dispatchRoot, x, y); overlayW != nil {
		if pressed && btn == widget.MouseLeft {
			if _, ok := overlayW.(widget.Focusable); ok {
				d.focus.set(overlayW)
			}
			// Гасим ЧУЖИЕ оверлеи — ровно как на обычном пути доставки.
			// Без этого оверлей, поглотивший клик, оставлял открытыми все
			// остальные: клик внутри календаря не закрывал ни меню «Пуск»,
			// ни соседнюю панель, потому что до DismissOutside дело не
			// доходило вовсе.
			keep := pathSet(HitPath(dispatchRoot, x, y))
			keep[overlayW] = struct{}{}
			DismissOutside(dispatchRoot, keep, x, y)
		}
		if mc, ok := overlayW.(widget.MouseClickHandler); ok {
			// Нажатие — в кадре владельца оверлея: меню поля внутри прокрутки
			// лежит там же, где поле, в координатах содержимого.
			if deliverButton(mc, ev, overlayOff) {
				// Overlay поглотил press — запоминаем для release-проверки.
				if pressed && btn == widget.MouseLeft {
					d.pressConsumer = overlayW
				}
				return
			}
		}
	}

	// Проверяем, хочет ли кто-то из предков захватить мышь (drag handle)
	if pressed && btn == widget.MouseLeft {
		if capturer, capOff := findCapturerStep(dispatchRoot, x, y, ev); capturer != nil {
			// Гарантируем захватчику CaptureManager: injectCaptureManager при
			// SetRoot не достаёт до виджетов, скрытых из Children() (например,
			// содержимое неактивной вкладки TabControl) — без менеджера виджет
			// не смог бы отпустить захват, и весь ввод залипал бы на нём.
			// Менеджер — тот же, что раздаёт движок (Host.CaptureManager):
			// виджеты достают из него и другие возможности движка (Post).
			if ca, ok := capturer.(widget.CaptureAware); ok {
				cm := d.host.CaptureManager()
				if cm == nil {
					cm = d
				}
				ca.SetCaptureManager(cm)
			}
			d.SetCapture(capturer)

			// Устанавливаем фокус на захватчик (TextInput и т.д.)
			if _, ok := capturer.(widget.Focusable); ok {
				d.focus.set(capturer)
			}

			// Закрываем Dismissable-виджеты вне пути к захватчику
			if capPath := HitPath(dispatchRoot, x, y); len(capPath) > 0 {
				DismissOutside(dispatchRoot, pathSet(capPath), x, y)
			}

			// Запоминаем capturer как pressConsumer — если capturer
			// будет закрыт/удалён, release не пролетит на виджет снизу.
			d.pressConsumer = capturer

			if mc, ok := capturer.(widget.MouseClickHandler); ok {
				deliverButton(mc, ev, capOff)
			}
			return
		}
	}

	// Получаем путь от корня до самого глубокого виджета под курсором
	path := HitPath(dispatchRoot, x, y)
	if len(path) == 0 {
		return
	}
	hit := path[len(path)-1].W

	// При нажатии — передаём фокус и закрываем overlay'и вне пути.
	if pressed && btn == widget.MouseLeft {
		if _, ok := hit.(widget.Focusable); ok {
			d.focus.set(hit)
		} else {
			d.focus.set(nil)
		}

		// Закрываем все Dismissable-виджеты, которые НЕ лежат на пути
		// от корня до целевого виджета (dropdown/popup/menu вне клика).
		DismissOutside(dispatchRoot, pathSet(path), x, y)
	}

	// Доставляем событие с bubbling: от самого глубокого виджета к корню.
	// Если виджет поглотил событие (вернул true) — bubbling останавливается.
	// Каждый получает точку в своём кадре: путь через прокрутку меняет кадр.
	for i := len(path) - 1; i >= 0; i-- {
		if mc, ok := path[i].W.(widget.MouseClickHandler); ok {
			if deliverButton(mc, ev, path[i].Off) {
				// Запоминаем поглотивший виджет, чтобы при release проверить,
				// остался ли он под курсором (иначе release проглатывается).
				if pressed && btn == widget.MouseLeft {
					d.pressConsumer = path[i].W
				}
				return
			}
		}
	}
}

// wheelPixelHandler — опциональный интерфейс виджета, принимающего точные
// пиксельные дельты колеса/тачпада (плавный скролл). dy>0 — вниз. Возвращает
// true, если дельта поглощена. Виджеты без него используют тиковый путь.
type wheelPixelHandler interface {
	OnMouseWheelPixels(x, y int, dx, dy float64) bool
}

// wheelPixelModHandler — то же, но с модификаторами, зажатыми в момент
// прокрутки (см. SetModifiers). Нужен виджетам, где Shift+колесо значит
// «вбок»: на мыши без горизонтального колеса это единственный способ увести
// длинную строку влево. Отдельный интерфейс, а не новый параметр
// wheelPixelHandler: менять его сигнатуру значит сломать сборку всем, кто
// его реализовал.
type wheelPixelModHandler interface {
	OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod widget.KeyMod) bool
}

// wheelTickPixels — сколько пикселей точной дельты приходится на один «тик»
// колеса в фолбэке (соответствует шагу тикового колеса в виджетах).
const wheelTickPixels = 40.0

// SendMouseWheelPixels доставляет точную пиксельную дельту прокрутки в точке
// (xPhys, yPhys — физические пиксели окна/кадра). dy>0 — вниз, dx>0 — вправо.
// Событие всплывает от самого глубокого виджета под курсором к корню; первый
// виджет, реализующий wheelPixelHandler и поглотивший дельту, останавливает
// всплытие.
//
// Фолбэк: если точную дельту никто не принял (виджет знает лишь тиковое
// колесо), синтезируем эквивалентные тики — старый тиковый путь остаётся
// рабочим (headless-контракт). Инвалидация — только область получателя.
func (d *Dispatcher) SendMouseWheelPixels(xPhys, yPhys int, dx, dy float64) {
	defer d.host.GuardInput("SendMouseWheelPixels")()
	x, y := d.toLogical(xPhys, yPhys)
	if k := d.host.Scale(); k != 1 && k > 0 {
		dx /= k
		dy /= k
	}

	var dispatchRoot widget.Widget
	if m := d.host.TopModal(); m != nil {
		dispatchRoot = m
	} else {
		dispatchRoot = d.host.Root()
	}
	if dispatchRoot != nil {
		mod := d.Modifiers()
		path := HitPath(dispatchRoot, x, y)
		for i := len(path) - 1; i >= 0; i-- {
			// Точка — в кадре виджета: колесо над ребёнком прокрутки.
			fx, fy := x+path[i].Off.X, y+path[i].Off.Y
			if h, ok := path[i].W.(wheelPixelModHandler); ok {
				if h.OnMouseWheelPixelsMod(fx, fy, dx, dy, mod) {
					d.invalidateWidget(path[i].W, path[i].Off)
					return
				}
				continue
			}
			if h, ok := path[i].W.(wheelPixelHandler); ok {
				if h.OnMouseWheelPixels(fx, fy, dx, dy) {
					d.invalidateWidget(path[i].W, path[i].Off)
					return
				}
			}
		}
	}

	// Фолбэк на тиковый путь: один hit-test, N тиков, одна инвалидация.
	steps, btn, ok := wheelTicksFromPixels(dy)
	if !ok {
		return
	}
	targets := d.wheelTargets(dispatchRoot, x, y)
	if len(targets) == 0 {
		return
	}
	var consumer HitStep
	for i := 0; i < steps; i++ {
		if st := deliverWheelTick(targets, x, y, btn, d.Modifiers()); st.W != nil {
			consumer = st
		}
	}
	if consumer.W != nil {
		d.invalidateWidget(consumer.W, consumer.Off)
	}
}

// wheelTargets — получатели тика колеса по порядку (каждый со своим кадром):
// захватчик мыши либо оверлей под курсором и путь hit-test снизу вверх.
func (d *Dispatcher) wheelTargets(dispatchRoot widget.Widget, x, y int) []HitStep {
	if cap := d.getCaptured(); cap != nil {
		return []HitStep{{W: cap, Off: d.captureOffset(cap)}}
	}
	if dispatchRoot == nil {
		return nil
	}
	var out []HitStep
	if ov, ovOff := findOverlayStep(dispatchRoot, x, y); ov != nil {
		out = append(out, HitStep{W: ov, Off: ovOff})
	}
	path := HitPath(dispatchRoot, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		out = append(out, path[i])
	}
	return out
}

// deliverWheelTick шлёт один тик (press+release) по списку получателей;
// возвращает получателя, поглотившего нажатие (w == nil — никто).
func deliverWheelTick(targets []HitStep, x, y int, btn widget.MouseButton, mod widget.KeyMod) HitStep {
	var consumer HitStep
	for _, pressed := range [2]bool{true, false} {
		ev := widget.MouseEvent{X: x, Y: y, Button: btn, Pressed: pressed, Mod: mod}
		for _, st := range targets {
			mc, ok := st.W.(widget.MouseClickHandler)
			if !ok {
				continue
			}
			if deliverButton(mc, ev, st.Off) {
				if pressed {
					consumer = st
				}
				break
			}
		}
	}
	return consumer
}

// invalidateWidget помечает на экране область виджета, чей кадр сдвинут на off;
// при пустых bounds — весь кадр.
func (d *Dispatcher) invalidateWidget(w widget.Widget, off image.Point) {
	if b := w.Bounds(); !b.Empty() {
		d.host.InvalidateRect(b.Sub(off))
		return
	}
	d.host.Invalidate()
}

// wheelTicksFromPixels переводит пиксельную дельту в число тиков и направление.
func wheelTicksFromPixels(dy float64) (steps int, btn widget.MouseButton, ok bool) {
	if dy == 0 {
		return 0, 0, false
	}
	mag := dy
	btn = widget.MouseWheelDown
	if dy < 0 {
		btn = widget.MouseWheelUp
		mag = -dy
	}
	steps = int(mag/wheelTickPixels + 0.5)
	if steps < 1 {
		steps = 1
	}
	return steps, btn, true
}

// ─── File drop (Drag&Drop файлов из ОС) ─────────────────────────────────────

// SendFilesDropped доставляет событие сброса файлов из ОС в точку (x, y —
// ФИЗИЧЕСКИЕ пиксели окна/кадра, как у SendMouse*). Событие всплывает от
// самого глубокого виджета под точкой к корню; первый виджет, реализующий
// widget.FileDropTarget и вернувший true, поглощает событие и останавливает
// всплытие (bubbling, как у колеса).
//
// paths — абсолютные пути к сброшенным файлам. Координаты, переданные виджету,
// уже логические. Позволяет headless-тестам синтетически «сбрасывать» файлы.
func (d *Dispatcher) SendFilesDropped(x, y int, paths []string) {
	defer d.host.GuardInput("SendFilesDropped")()
	if len(paths) == 0 {
		return
	}
	x, y = d.toLogical(x, y)

	// Сброс файлов может изменить произвольную часть UI (виджет-приёмник
	// перерисовывается) — полная инвалидация, как у клика.
	d.host.Invalidate()

	var dispatchRoot widget.Widget
	if m := d.host.TopModal(); m != nil {
		dispatchRoot = m
	} else {
		dispatchRoot = d.host.Root()
	}
	if dispatchRoot == nil {
		return
	}

	path := HitPath(dispatchRoot, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		if fd, ok := path[i].W.(widget.FileDropTarget); ok {
			// Точка — в кадре приёмника (приёмник внутри прокрутки).
			if fd.OnFilesDropped(x+path[i].Off.X, y+path[i].Off.Y, paths) {
				return
			}
		}
	}
}

// ─── Dismiss ─────────────────────────────────────────────────────────────────

// DismissOutside рекурсивно закрывает все Dismissable-виджеты, которые
// не входят в набор keep (виджеты на пути от корня до клика).
// Это гарантирует закрытие popup/dropdown/menu при клике в другое место.
// DismissOutside закрывает виджеты, не лежащие на пути клика в точке (x, y).
//
// Точка нужна не всем: обычному Dismissable довольно самого факта «клик мимо».
// Но виджет, считающий чужую площадь своей — всплывающая панель, у которой
// есть соседка по группе, — без координаты решить не может, и для него есть
// widget.DismissableAt.
//
// (x, y) — экранные; каждому виджету точка отдаётся в его кадре (см. frames.go).
func DismissOutside(w widget.Widget, keep map[widget.Widget]struct{}, x, y int) {
	dismissOutsideAt(w, keep, x, y, image.Point{}, 0)
}

func dismissOutsideAt(w widget.Widget, keep map[widget.Widget]struct{}, x, y int, off image.Point, depth int) {
	if tooDeep(depth) {
		return
	}
	if _, inPath := keep[w]; !inPath {
		if d, ok := w.(widget.DismissableAt); ok {
			d.DismissAt(x+off.X, y+off.Y)
		} else if d, ok := w.(widget.Dismissable); ok {
			d.Dismiss()
		}
	}
	children := w.Children()
	if len(children) == 0 {
		return
	}
	coff := off.Add(ContentShift(w))
	for _, child := range children {
		dismissOutsideAt(child, keep, x, y, coff, depth+1)
	}
}

// ─── Hit testing ─────────────────────────────────────────────────────────────

// hitTest возвращает самый верхний виджет (последний дочерний в Z-порядке),
// чьи bounds содержат точку (x, y). Возвращает nil, если точка вне дерева.
// Сам путь с кадрами виджетов строит HitPath (frames.go).
func hitTest(w widget.Widget, x, y int) widget.Widget {
	path := HitPath(w, x, y)
	if len(path) == 0 {
		return nil
	}
	return path[len(path)-1].W
}

// findCapturer ищет виджет, который хочет захватить мышь, в цепочке предков
// от корня до hit-виджета. Возвращает ближайшего к hit (самого вложенного).
func findCapturer(w widget.Widget, x, y int, ev widget.MouseEvent) widget.Widget {
	c, _ := findCapturerStep(w, x, y, ev)
	return c
}

// findCapturerStep — findCapturer, который заодно отдаёт кадр найденного виджета:
// захватчик получает нажатие в своих координатах.
func findCapturerStep(w widget.Widget, x, y int, ev widget.MouseEvent) (widget.Widget, image.Point) {
	return findCapturerAt(w, x, y, ev, image.Point{}, 0)
}

// findCapturerAt: (x, y) — точка в кадре w, off — смещение этого кадра. Вопрос
// WantsCapture задаётся с событием в кадре самого спрашиваемого: он сверяет его
// со своими Bounds.
func findCapturerAt(w widget.Widget, x, y int, ev widget.MouseEvent, off image.Point, depth int) (widget.Widget, image.Point) {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return nil, image.Point{}
	}
	pt := image.Pt(x, y)
	if !pt.In(w.Bounds()) {
		return nil, image.Point{}
	}
	cx, cy, coff := x, y, off
	if sh := ContentShift(w); sh != (image.Point{}) {
		cx, cy, coff = x+sh.X, y+sh.Y, off.Add(sh)
	}
	// Потомки в обратном Z-порядке — но только по пути hit-теста: первый же
	// ребёнок, накрывающий точку, и есть путь (как в appendHitPath), и соседи
	// под ним щелчка не получают. Раньше поиск шёл дальше, к соседям ниже, и
	// захват доставался виджету под непрозрачной панелью: щелчок по панели
	// задач поверх окна программы уходил окну (ADR-0002, 2.2).
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		if !widget.IsWidgetVisible(child) || !image.Pt(cx, cy).In(child.Bounds()) {
			continue
		}
		if found, foff := findCapturerAt(child, cx, cy, ev, coff, depth+1); found != nil {
			return found, foff
		}
		break
	}
	// Проверяем сам виджет
	if cr, ok := w.(widget.CaptureRequester); ok {
		ev.X, ev.Y = x, y
		if cr.WantsCapture(ev) {
			return w, off
		}
	}
	return nil, image.Point{}
}

// findOverlayAt ищет виджет с активным overlay (popup/dropdown/menu),
// чьи расширенные bounds (включая overlay) содержат точку (x, y).
// Overlay имеет приоритет над обычным Z-порядком дерева виджетов.
// Возвращает nil, если ни один overlay не содержит точку.
func findOverlayAt(w widget.Widget, x, y int) widget.Widget {
	o, _ := findOverlayStep(w, x, y)
	return o
}

// findOverlayStep — findOverlayAt, который заодно отдаёт кадр владельца оверлея:
// оверлей лежит в координатах своего виджета (у поля внутри прокрутки — в
// координатах содержимого), и событие ему нужно отдавать в них же.
func findOverlayStep(w widget.Widget, x, y int) (widget.Widget, image.Point) {
	return findOverlayAtDepth(w, x, y, image.Point{}, true, 0)
}

// findOverlayAtDepth: (x, y) — точка в кадре w, off — смещение этого кадра,
// inView — точка видна сквозь все контейнеры со сдвигом над w.
//
// Вне видимой области прокрутки владелец оверлея не получает точку «своей
// площадью»: сам он оттуда обрезан клипом, и его границы в координатах
// содержимого могли бы перекрывать совсем другие кнопки под прокруткой. Сам
// оверлей (меню) по-прежнему ловит точку — он рисуется поверх всего.
func findOverlayAtDepth(w widget.Widget, x, y int, off image.Point, inView bool, depth int) (widget.Widget, image.Point) {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return nil, image.Point{}
	}
	pt := image.Pt(x, y)

	// Проверяем детей в обратном Z-порядке (верхние первыми).
	children := w.Children()
	cx, cy, coff, cview := x, y, off, inView
	if len(children) > 0 {
		if oc, ok := w.(widget.ContentOffsetter); ok {
			sh := oc.ContentOffset()
			cview = inView && pt.In(w.Bounds())
			cx, cy, coff = x+sh.X, y+sh.Y, off.Add(sh)
		}
	}
	for i := len(children) - 1; i >= 0; i-- {
		if found, foff := findOverlayAtDepth(children[i], cx, cy, coff, cview, depth+1); found != nil {
			return found, foff
		}
	}

	// Проверяем сам виджет: есть ли активный overlay и попадает ли точка в него.
	if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
		hit := false
		if inView {
			hit = pt.In(OverlayHitRect(w))
		} else if ob, ok := w.(widget.OverlayBoundsProvider); ok {
			hit = pt.In(ob.OverlayBounds())
		}
		if hit {
			return w, off
		}
	}

	return nil, image.Point{}
}

// OverlayHitRect — область, в которой щелчок принадлежит оверлею виджета.
//
// Границы САМОГО виджета для этого не годятся: меню открывается рядом с ним и
// почти всегда вылезает наружу — контекстное меню поля ввода вниз, меню
// переполнения панели инструментов под панель. Щелчок по такому меню не
// попадал в границы владельца, оверлей не находился, и движок гасил меню
// вместо того, чтобы отдать ему нажатие: пункт нельзя было выбрать мышью.
//
// Объединение, а не одна лишь область оверлея: виджет может рисовать оверлей
// частично поверх себя, и терять свою часть незачем.
//
// Область — в кадре виджета (см. frames.go).
func OverlayHitRect(w widget.Widget) image.Rectangle {
	r := w.Bounds()
	if ob, ok := w.(widget.OverlayBoundsProvider); ok {
		r = r.Union(ob.OverlayBounds())
	}
	return r
}

// broadcastMouseMove рекурсивно доставляет событие перемещения мыши дереву
// виджетов АДРЕСНО: OnMouseMove получают только виджеты, которых движение
// касается — прежняя (ox,oy) или новая (nx,ny) точка в их bounds (вторая
// нужна, чтобы виджет, из которого курсор ушёл, снял свой hover). Исключения,
// получающие событие всегда:
//
//   - пустые bounds — оверлейные виджеты (PopupMenu до Show) и контейнеры без
//     геометрии судят о попадании сами;
//   - активный overlay (открытый dropdown/меню) — его видимая область шире
//     bounds виджета-хозяина.
//
// Drag-механики адресность не задевает: всё, что тянет мышью (скроллбары,
// сплиттеры, заголовок окна, выделение текста), берёт мышь через
// SetCapture, а захваченная мышь доставляется напрямую (см. SendMouseMove).
//
// Обход по-прежнему идёт по всему дереву (дети в абсолютных координатах и
// могут выходить за родителя — отсечься по родителю нельзя), но дорогая часть
// — интерфейсный ассерт + вызов OnMouseMove на каждом из сотен виджетов при
// каждом движении — выполняется теперь только у затронутых.
//
// Точки — экранные. Дети контейнера со сдвигом (прокрутки) получают их в своём
// кадре, а точку вне видимой области контейнера — как «курсора нет»
// (CursorNowhere): под прокруткой, в координатах содержимого, лежат кнопки,
// которых с экрана не видно, и подсвечивать их нельзя; а ушедшая с кнопки мышь
// обязана снять с неё подсветку.
func broadcastMouseMove(w widget.Widget, ox, oy, nx, ny int) {
	broadcastMouseMoveAt(w, ox, oy, nx, ny, image.Point{}, 0, nil)
}

// broadcastMouseMoveSkip — broadcastMouseMove мимо поддерева skip.
func broadcastMouseMoveSkip(w widget.Widget, ox, oy, nx, ny int, skip widget.Widget) {
	broadcastMouseMoveAt(w, ox, oy, nx, ny, image.Point{}, 0, skip)
}

// broadcastMouseMoveFrame — broadcastMouseMove для поддерева, корень которого
// (оверлей внутри прокрутки) лежит в кадре off.
func broadcastMouseMoveFrame(w widget.Widget, ox, oy, nx, ny int, off image.Point) {
	broadcastMouseMoveAt(w, ox+off.X, oy+off.Y, nx+off.X, ny+off.Y, off, 0, nil)
}

// broadcastMouseMoveAt: обе точки — в кадре w, off — смещение этого кадра.
// skip (если не nil) — поддерево, которое обходится стороной.
func broadcastMouseMoveAt(w widget.Widget, ox, oy, nx, ny int, off image.Point, depth int, skip widget.Widget) {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return
	}
	if skip != nil && w == skip {
		return
	}
	b := w.Bounds()
	interested := b.Empty() ||
		(nx >= b.Min.X && nx < b.Max.X && ny >= b.Min.Y && ny < b.Max.Y) ||
		(ox >= b.Min.X && ox < b.Max.X && oy >= b.Min.Y && oy < b.Max.Y)
	if !interested {
		if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
			interested = true
		}
	}
	if interested {
		if mm, ok := w.(widget.MouseMoveHandler); ok {
			deliverMoveFramed(mm, nx, ny, off)
		}
	}
	children := w.Children()
	if len(children) == 0 {
		return // лист: кадр детей считать незачем (в больших деревьях листьев большинство)
	}
	cox, coy, cnx, cny, coff := ox, oy, nx, ny, off
	if oc, ok := w.(widget.ContentOffsetter); ok {
		sh := oc.ContentOffset()
		cox, coy = intoContent(ox, oy, b, sh)
		cnx, cny = intoContent(nx, ny, b, sh)
		coff = off.Add(sh)
	}
	for _, child := range children {
		broadcastMouseMoveAt(child, cox, coy, cnx, cny, coff, depth+1, skip)
	}
}

// intoContent переводит точку контейнера со сдвигом sh в кадр его детей; точка
// вне видимой области view становится «курсора нет».
func intoContent(x, y int, view image.Rectangle, sh image.Point) (int, int) {
	if !image.Pt(x, y).In(view) {
		return widget.CursorNowhere, widget.CursorNowhere
	}
	return x + sh.X, y + sh.Y
}

// ─── Серия нажатий (двойной и тройной щелчок) ───────────────────────────────

// defaultDoubleClick — интервал серии, когда системного значения нет.
// 500 мс — то же, что ставит Windows по умолчанию.
const defaultDoubleClick = 500 * time.Millisecond

// clickSlack — насколько курсор может сдвинуться между нажатиями серии
// (логические точки). Рука дрожит, и требовать точку в точку нельзя.
const clickSlack = 4

// SetDoubleClickTime задаёт интервал, внутри которого второе нажатие считается
// двойным щелчком, а третье — тройным.
//
// Значение системное: человек выставляет его в параметрах мыши, и угадывать
// за него не нужно. Окно сообщает его при запуске, если бэкенд умеет узнать
// (Win32 — GetDoubleClickTime); иначе остаётся значение по умолчанию.
// delay <= 0 возвращает к значению по умолчанию.
func (d *Dispatcher) SetDoubleClickTime(delay time.Duration) {
	d.dblClickDelay = delay
}

// countClick возвращает номер нажатия в серии для MouseEvent.Clicks.
//
// Серию рвёт что угодно из трёх: другая кнопка, слишком долгая пауза,
// слишком далёкий курсор. Отпускание номера не меняет — оно несёт номер
// своего нажатия, чтобы обработчик release видел ту же серию, что и press.
func (d *Dispatcher) countClick(x, y int, btn widget.MouseButton, pressed bool) int {
	if !pressed {
		if btn != d.clickBtn {
			return 0
		}
		return d.clickN
	}
	delay := d.dblClickDelay
	if delay <= 0 {
		delay = defaultDoubleClick
	}
	now := time.Now()
	sameSpot := abs(x-d.clickX) <= clickSlack && abs(y-d.clickY) <= clickSlack
	if btn == d.clickBtn && d.clickN > 0 && sameSpot && now.Sub(d.clickAt) <= delay {
		d.clickN++
	} else {
		d.clickN = 1
	}
	d.clickBtn, d.clickAt, d.clickX, d.clickY = btn, now, x, y
	return d.clickN
}

// abs — модуль целого (math.Abs просит float и возвращает float).
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
