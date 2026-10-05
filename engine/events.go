// Package engine — диспетчер событий ввода (мышь, клавиатура, фокус).
package engine

import (
	"image"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Focus manager ───────────────────────────────────────────────────────────

// focusManager хранит текущий виджет с фокусом и управляет передачей фокуса.
type focusManager struct {
	mu      sync.Mutex
	focused widget.Widget // nil — нет фокуса
}

// set устанавливает фокус на w; снимает фокус с предыдущего (если реализует Focusable).
func (fm *focusManager) set(w widget.Widget) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.focused == w {
		return
	}
	// Снимаем фокус со старого
	if fm.focused != nil {
		if f, ok := fm.focused.(widget.Focusable); ok {
			f.SetFocused(false)
		}
	}
	fm.focused = w
	// Даём фокус новому
	if w != nil {
		if f, ok := w.(widget.Focusable); ok {
			f.SetFocused(true)
		}
	}
}

func (fm *focusManager) get() widget.Widget {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return fm.focused
}

// ─── Mouse Capture ──────────────────────────────────────────────────────────

// SetCapture направляет все события мыши на указанный виджет.
func (e *Engine) SetCapture(w widget.Widget) {
	e.capMu.Lock()
	e.captured = w
	// Цепочку контейнеров со сдвигом над захватчиком найдём при ближайшем
	// событии (captureOffset): здесь нас могли позвать из обработчика виджета,
	// держащего свои замки, а поиск обходит дерево.
	e.capChain, e.capChainOK = nil, false
	e.capMu.Unlock()
}

// ReleaseCapture отменяет захват мыши.
func (e *Engine) ReleaseCapture() {
	e.capMu.Lock()
	e.captured = nil
	e.capChain, e.capChainOK = nil, false
	e.capMu.Unlock()
}

func (e *Engine) getCaptured() widget.Widget {
	e.capMu.Lock()
	w := e.captured
	e.capMu.Unlock()
	return w
}

// ─── SetFocus / SendKeyEvent ─────────────────────────────────────────────────

// SetFocus передаёт фокус ввода виджету w.
// Если w == nil — фокус снимается со всех виджетов.
func (e *Engine) SetFocus(w widget.Widget) {
	e.setFocusInvalidating(w)
}

// setFocusInvalidating переводит фокус и точечно инвалидирует области старого
// и нового виджета (рамка фокуса). Виджеты дополнительно самоинвалидируются
// в SetFocused — двойная инвалидация дешёвая (объединение damage-областей).
func (e *Engine) setFocusInvalidating(w widget.Widget) {
	// Границы виджета — в его кадре, а инвалидировать надо экранное место: у
	// виджета внутри прокрутки это разные прямоугольники (см. frames.go).
	if old := e.focus.get(); old != nil {
		e.InvalidateRect(e.screenBoundsOf(old))
	}
	e.focus.set(w)
	if w != nil {
		e.InvalidateRect(e.screenBoundsOf(w))
	}
}

// SendKeyEvent доставляет клавиатурное событие виджету с фокусом.
// Tab / Shift+Tab перехватываются для переключения фокуса между виджетами.
//
// Полной инвалидации здесь нет: виджет с фокусом самоинвалидируется при
// изменении своего состояния (текст, каретка, выделение), Tab-навигация
// инвалидирует старый/новый фокус точечно, командные хоткеи — полностью
// (команда может изменить что угодно).
func (e *Engine) SendKeyEvent(ev widget.KeyEvent) {
	defer e.serveFocusRequests()()

	// Tab-навигация: перехватываем Tab до доставки виджету — если только
	// фокусный виджет не забирает его себе (widget.TabAcceptor: редактор кода).
	// Ctrl+Tab остаётся навигацией и у такого виджета.
	// При активном модальном виджете Tab циклит только внутри него.
	if ev.Code == widget.KeyTab && ev.Pressed && !widget.WantsTab(e.focus.get(), ev) {
		var tabRoot widget.Widget
		if m := e.topModal(); m != nil {
			tabRoot = m
		} else {
			e.mu.RLock()
			tabRoot = e.root
			e.mu.RUnlock()
		}
		if tabRoot != nil {
			reverse := ev.Mod&widget.ModShift != 0
			e.tabCycle(tabRoot, reverse)
		}
		return
	}

	// Escape закрывает верхний модальный виджет (и сообщает об отмене, если
	// диалог задал CancelAction — например, InputDialog возвращает ok=false).
	if ev.Code == widget.KeyEscape && ev.Pressed {
		if m := e.topModal(); m != nil {
			// Тем же путём, что и ✕ (requestCloseModal): иначе диалог,
			// отказавшийся закрываться по крестику, закрывался бы по Escape.
			e.requestCloseModal(m)
			return
		}
	}

	// Escape закрывает всплывающие панели (widget.EscapeDismisser) — тем, у
	// кого может не быть фокуса вовсе, например меню «Пуск» и центр
	// уведомлений. Один Esc закрывает один слой. Если фокус у виджета с
	// собственным открытым оверлеем (выпадающий список, меню), Esc принадлежит
	// ему: сначала закрывается он.
	if ev.Code == widget.KeyEscape && ev.Pressed && e.dismissOnEscape() {
		return
	}

	// Горячие клавиши окна (WPF InputBindings/KeyBinding) — до фокус-диспатча.
	if ev.Pressed {
		var hostRoot widget.Widget
		if m := e.topModal(); m != nil {
			hostRoot = m
		} else {
			e.mu.RLock()
			hostRoot = e.root
			e.mu.RUnlock()
		}
		if h, ok := hostRoot.(interface {
			HandleInputBinding(widget.KeyCode, widget.KeyMod) bool
		}); ok {
			if h.HandleInputBinding(ev.Code, ev.Mod) {
				// Команда хоткея может изменить произвольную часть UI.
				e.Invalidate()
				return
			}
		}
	}

	w := e.focus.get()
	if w == nil {
		return
	}
	if kh, ok := w.(widget.KeyHandler); ok {
		kh.OnKeyEvent(ev)
	}
}

// dismissOnEscape закрывает верхнюю панель, закрывающуюся по Esc. Возвращает
// true, если что-то закрыто и клавишу дальше отдавать не нужно.
func (e *Engine) dismissOnEscape() bool {
	if w := e.focus.get(); w != nil {
		if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
			return false
		}
	}
	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
	if root == nil {
		return false
	}
	return widget.DismissOnEscape(root)
}

// tabCycle переключает фокус на следующий (или предыдущий) Focusable-виджет.
func (e *Engine) tabCycle(root widget.Widget, reverse bool) {
	all := widget.CollectFocusables(root)
	if len(all) == 0 {
		return
	}

	current := e.focus.get()
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

	e.setFocusInvalidating(all[next])
}

// toLogical переводит физические координаты события (пиксели окна/кадра)
// в логические координаты виджетов (HiDPI). При Scale == 1 тождественно.
// Lock-free (scaleBits) — события не конкурируют с мьютексом движка.
func (e *Engine) toLogical(x, y int) (int, int) {
	k := e.Scale()
	if k == 1 {
		return x, y
	}
	return int(float64(x) / k), int(float64(y) / k)
}

// CursorAt возвращает форму курсора для точки (x, y — физические пиксели):
// курсор самого глубокого виджета под точкой, реализующего
// widget.CursorProvider (иначе — стрелка).
func (e *Engine) CursorAt(x, y int) widget.Cursor {
	x, y = e.toLogical(x, y)
	var disp widget.Widget
	if m := e.topModal(); m != nil {
		disp = m
	} else {
		e.mu.RLock()
		disp = e.root
		e.mu.RUnlock()
	}
	if disp == nil {
		return widget.CursorArrow
	}
	path := hitPath(disp, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		if ov, ok := path[i].w.(interface{ CursorOverride() (widget.Cursor, bool) }); ok {
			if c, has := ov.CursorOverride(); has {
				return c
			}
		}
		if cp, ok := path[i].w.(widget.CursorProvider); ok {
			// Виджет судит о точке в своём кадре (у ребёнка прокрутки — в
			// координатах содержимого).
			return cp.Cursor(x+path[i].off.X, y+path[i].off.Y)
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
func (e *Engine) SendMouseMove(x, y int) {
	x, y = e.toLogical(x, y)

	// Если на экране висит подсказка — стираем её (движение мыши сбрасывает
	// таймер, следующий кадр рисуется без плашки).
	e.invalidateShownTooltip()

	// Запоминаем позицию курсора и сбрасываем таймер всплывающей подсказки.
	e.recordMouse(x, y)

	// Прежняя позиция курсора — для адресной доставки (см. broadcastMouseMove):
	// виджету, из-под которого курсор ушёл, событие тоже нужно (снять hover).
	ox, oy := e.lastMoveX, e.lastMoveY
	if !e.hasLastMove {
		ox, oy = x, y
		e.hasLastMove = true
	}
	e.lastMoveX, e.lastMoveY = x, y

	// Если мышь захвачена — только захватчику
	if cap := e.getCaptured(); cap != nil {
		if mm, ok := cap.(widget.MouseMoveHandler); ok {
			// В кадре захватчика: перетаскивание внутри прокрутки идёт в
			// координатах содержимого, а курсор — экранный.
			deliverMove(mm, x, y, e.captureOffset(cap))
		}
		return
	}

	// Модальный виджет: ограничиваем broadcast
	if m := e.topModal(); m != nil {
		broadcastMouseMove(m, ox, oy, x, y)
		return
	}

	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
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
		broadcastMouseMove(root, ox, oy, widget.CursorNowhere, widget.CursorNowhere)
		// Затем — настоящая точка тому, кому она принадлежит, и его детям. Точки
		// переводятся в кадр владельца оверлея: он может лежать в прокрутке.
		broadcastMouseMoveFrame(ov, ox, oy, x, y, ovOff)
		return
	}

	broadcastMouseMove(root, ox, oy, x, y)
}

// SendMouseButton уведомляет дерево о нажатии/отпускании кнопки мыши в (x, y).
// Если мышь захвачена — событие идёт только захватчику.
// Иначе: проверяем, хочет ли какой-либо предок захватить мышь (WantsCapture),
// затем передаём событие самому верхнему виджету под курсором.
func (e *Engine) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool) {
	defer e.serveFocusRequests()()
	x, y = e.toLogical(x, y)

	// Клик оставляет ПОЛНУЮ инвалидацию сознательно: он может открыть/закрыть
	// overlay (dropdown, меню), сместить фокус, выполнить команду — задеть
	// произвольные области. Клики редки, полный кадр здесь дёшев и надёжен.
	e.Invalidate()
	ev := widget.MouseEvent{X: x, Y: y, Button: btn, Pressed: pressed, Mod: e.Modifiers()}
	ev.Clicks = e.countClick(x, y, btn, pressed)

	// Новое нажатие: открываем его номер ДО гашения overlay'ев (dismissOutside
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
	if cap := e.getCaptured(); cap != nil {
		// Сбрасываем pressConsumer — capture-виджет обработает release сам.
		if !pressed && btn == widget.MouseLeft {
			e.pressConsumer = nil
		}
		if mc, ok := cap.(widget.MouseClickHandler); ok {
			deliverButton(mc, ev, e.captureOffset(cap))
		}
		// Движок гарантирует снятие capture при отпускании ЛКМ —
		// даже если виджет не вызвал ReleaseCapture (например, capMgr == nil).
		if !pressed && btn == widget.MouseLeft {
			e.ReleaseCapture()
		}
		return
	}

	// Если предыдущий press был поглощён виджетом, а этот виджет
	// больше не находится под курсором (был закрыт/удалён) — проглатываем
	// release, чтобы он не попал на виджет под закрывшимся окном.
	if !pressed && btn == widget.MouseLeft && e.pressConsumer != nil {
		consumer := e.pressConsumer
		e.pressConsumer = nil

		// Проверяем, есть ли ещё поглотитель в пути под курсором
		var dispRoot widget.Widget
		if m := e.topModal(); m != nil {
			dispRoot = m
		} else {
			e.mu.RLock()
			dispRoot = e.root
			e.mu.RUnlock()
		}
		if dispRoot != nil {
			path := hitPath(dispRoot, x, y)
			found := false
			for _, st := range path {
				if st.w == consumer {
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
					if image.Pt(x, y).Add(e.frameOffsetOf(consumer)).In(overlayHitRect(consumer)) {
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
	if m := e.topModal(); m != nil {
		dispatchRoot = m
	} else {
		e.mu.RLock()
		dispatchRoot = e.root
		e.mu.RUnlock()
	}
	if dispatchRoot == nil {
		return
	}

	// Правый клик (по отпусканию, как в ОС): показываем привязанное контекстное
	// меню (WPF ContextMenu) самого глубокого виджета под курсором.
	if !pressed && btn == widget.MouseRight {
		path := hitPath(dispatchRoot, x, y)
		for i := len(path) - 1; i >= 0; i-- {
			// Виджет судит о точке в своём кадре, и меню, которое он строит,
			// живёт там же: рисуется оно через тот же сдвиг, что и сам виджет.
			fx, fy := x+path[i].off.X, y+path[i].off.Y
			// Сперва — меню, собираемое под точкой: список и таблица строят
			// его по строке под курсором, и одного готового меню на виджет им
			// не хватает. Готовое ContextMenu — частный случай, когда меню от
			// точки не зависит, поэтому оно идёт вторым.
			if h, ok := path[i].w.(widget.ContextMenuProvider); ok {
				if pm := h.ContextMenuAt(fx, fy); pm != nil {
					showMenuInFrame(pm, fx, fy, path[i].off)
					return
				}
			}
			if h, ok := path[i].w.(interface{ GetContextMenu() *widget.PopupMenu }); ok {
				if pm := h.GetContextMenu(); pm != nil {
					// Готовое меню — самостоятельный виджет дерева (обычно
					// ребёнок того, к кому оно приколото), и кадр у него свой.
					moff := e.frameOffsetOf(pm)
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
	// захвата гасит меню (dismissOutside по пути к захватчику, меню в него
	// не входит), и до пункта меню нажатие не доходит вовсе. Меню, открытое
	// над окном, из-за этого было полностью мёртвым — а над пустым рабочим
	// столом, где захват никому не нужен, работало.
	if overlayW, overlayOff := findOverlayStep(dispatchRoot, x, y); overlayW != nil {
		if pressed && btn == widget.MouseLeft {
			if _, ok := overlayW.(widget.Focusable); ok {
				e.focus.set(overlayW)
			}
			// Гасим ЧУЖИЕ оверлеи — ровно как на обычном пути доставки.
			// Без этого оверлей, поглотивший клик, оставлял открытыми все
			// остальные: клик внутри календаря не закрывал ни меню «Пуск»,
			// ни соседнюю панель, потому что до dismissOutside дело не
			// доходило вовсе.
			keep := pathSet(hitPath(dispatchRoot, x, y))
			keep[overlayW] = struct{}{}
			dismissOutside(dispatchRoot, keep, x, y)
		}
		if mc, ok := overlayW.(widget.MouseClickHandler); ok {
			// Нажатие — в кадре владельца оверлея: меню поля внутри прокрутки
			// лежит там же, где поле, в координатах содержимого.
			if deliverButton(mc, ev, overlayOff) {
				// Overlay поглотил press — запоминаем для release-проверки.
				if pressed && btn == widget.MouseLeft {
					e.pressConsumer = overlayW
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
			if ca, ok := capturer.(widget.CaptureAware); ok {
				ca.SetCaptureManager(e)
			}
			e.SetCapture(capturer)

			// Устанавливаем фокус на захватчик (TextInput и т.д.)
			if _, ok := capturer.(widget.Focusable); ok {
				e.focus.set(capturer)
			}

			// Закрываем Dismissable-виджеты вне пути к захватчику
			if capPath := hitPath(dispatchRoot, x, y); len(capPath) > 0 {
				dismissOutside(dispatchRoot, pathSet(capPath), x, y)
			}

			// Запоминаем capturer как pressConsumer — если capturer
			// будет закрыт/удалён, release не пролетит на виджет снизу.
			e.pressConsumer = capturer

			if mc, ok := capturer.(widget.MouseClickHandler); ok {
				deliverButton(mc, ev, capOff)
			}
			return
		}
	}

	// Получаем путь от корня до самого глубокого виджета под курсором
	path := hitPath(dispatchRoot, x, y)
	if len(path) == 0 {
		return
	}
	hit := path[len(path)-1].w

	// При нажатии — передаём фокус и закрываем overlay'и вне пути.
	if pressed && btn == widget.MouseLeft {
		if _, ok := hit.(widget.Focusable); ok {
			e.focus.set(hit)
		} else {
			e.focus.set(nil)
		}

		// Закрываем все Dismissable-виджеты, которые НЕ лежат на пути
		// от корня до целевого виджета (dropdown/popup/menu вне клика).
		dismissOutside(dispatchRoot, pathSet(path), x, y)
	}

	// Доставляем событие с bubbling: от самого глубокого виджета к корню.
	// Если виджет поглотил событие (вернул true) — bubbling останавливается.
	// Каждый получает точку в своём кадре: путь через прокрутку меняет кадр.
	for i := len(path) - 1; i >= 0; i-- {
		if mc, ok := path[i].w.(widget.MouseClickHandler); ok {
			if deliverButton(mc, ev, path[i].off) {
				// Запоминаем поглотивший виджет, чтобы при release проверить,
				// остался ли он под курсором (иначе release проглатывается).
				if pressed && btn == widget.MouseLeft {
					e.pressConsumer = path[i].w
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
func (e *Engine) SendMouseWheelPixels(xPhys, yPhys int, dx, dy float64) {
	x, y := e.toLogical(xPhys, yPhys)
	if k := e.Scale(); k != 1 && k > 0 {
		dx /= k
		dy /= k
	}

	var dispatchRoot widget.Widget
	if m := e.topModal(); m != nil {
		dispatchRoot = m
	} else {
		e.mu.RLock()
		dispatchRoot = e.root
		e.mu.RUnlock()
	}
	if dispatchRoot != nil {
		mod := e.Modifiers()
		path := hitPath(dispatchRoot, x, y)
		for i := len(path) - 1; i >= 0; i-- {
			// Точка — в кадре виджета: колесо над ребёнком прокрутки.
			fx, fy := x+path[i].off.X, y+path[i].off.Y
			if h, ok := path[i].w.(wheelPixelModHandler); ok {
				if h.OnMouseWheelPixelsMod(fx, fy, dx, dy, mod) {
					e.invalidateWidget(path[i].w, path[i].off)
					return
				}
				continue
			}
			if h, ok := path[i].w.(wheelPixelHandler); ok {
				if h.OnMouseWheelPixels(fx, fy, dx, dy) {
					e.invalidateWidget(path[i].w, path[i].off)
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
	targets := e.wheelTargets(dispatchRoot, x, y)
	if len(targets) == 0 {
		return
	}
	var consumer hitStep
	for i := 0; i < steps; i++ {
		if st := deliverWheelTick(targets, x, y, btn, e.Modifiers()); st.w != nil {
			consumer = st
		}
	}
	if consumer.w != nil {
		e.invalidateWidget(consumer.w, consumer.off)
	}
}

// wheelTargets — получатели тика колеса по порядку (каждый со своим кадром):
// захватчик мыши либо оверлей под курсором и путь hit-test снизу вверх.
func (e *Engine) wheelTargets(dispatchRoot widget.Widget, x, y int) []hitStep {
	if cap := e.getCaptured(); cap != nil {
		return []hitStep{{w: cap, off: e.captureOffset(cap)}}
	}
	if dispatchRoot == nil {
		return nil
	}
	var out []hitStep
	if ov, ovOff := findOverlayStep(dispatchRoot, x, y); ov != nil {
		out = append(out, hitStep{w: ov, off: ovOff})
	}
	path := hitPath(dispatchRoot, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		out = append(out, path[i])
	}
	return out
}

// deliverWheelTick шлёт один тик (press+release) по списку получателей;
// возвращает получателя, поглотившего нажатие (w == nil — никто).
func deliverWheelTick(targets []hitStep, x, y int, btn widget.MouseButton, mod widget.KeyMod) hitStep {
	var consumer hitStep
	for _, pressed := range [2]bool{true, false} {
		ev := widget.MouseEvent{X: x, Y: y, Button: btn, Pressed: pressed, Mod: mod}
		for _, st := range targets {
			mc, ok := st.w.(widget.MouseClickHandler)
			if !ok {
				continue
			}
			if deliverButton(mc, ev, st.off) {
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
func (e *Engine) invalidateWidget(w widget.Widget, off image.Point) {
	if b := w.Bounds(); !b.Empty() {
		e.InvalidateRect(b.Sub(off))
		return
	}
	e.Invalidate()
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
func (e *Engine) SendFilesDropped(x, y int, paths []string) {
	if len(paths) == 0 {
		return
	}
	x, y = e.toLogical(x, y)

	// Сброс файлов может изменить произвольную часть UI (виджет-приёмник
	// перерисовывается) — полная инвалидация, как у клика.
	e.Invalidate()

	var dispatchRoot widget.Widget
	if m := e.topModal(); m != nil {
		dispatchRoot = m
	} else {
		e.mu.RLock()
		dispatchRoot = e.root
		e.mu.RUnlock()
	}
	if dispatchRoot == nil {
		return
	}

	path := hitPath(dispatchRoot, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		if fd, ok := path[i].w.(widget.FileDropTarget); ok {
			// Точка — в кадре приёмника (приёмник внутри прокрутки).
			if fd.OnFilesDropped(x+path[i].off.X, y+path[i].off.Y, paths) {
				return
			}
		}
	}
}

// ─── Dismiss ─────────────────────────────────────────────────────────────────

// dismissOutside рекурсивно закрывает все Dismissable-виджеты, которые
// не входят в набор keep (виджеты на пути от корня до клика).
// Это гарантирует закрытие popup/dropdown/menu при клике в другое место.
// dismissOutside закрывает виджеты, не лежащие на пути клика в точке (x, y).
//
// Точка нужна не всем: обычному Dismissable довольно самого факта «клик мимо».
// Но виджет, считающий чужую площадь своей — всплывающая панель, у которой
// есть соседка по группе, — без координаты решить не может, и для него есть
// widget.DismissableAt.
//
// (x, y) — экранные; каждому виджету точка отдаётся в его кадре (см. frames.go).
func dismissOutside(w widget.Widget, keep map[widget.Widget]struct{}, x, y int) {
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
	coff := off.Add(contentShift(w))
	for _, child := range children {
		dismissOutsideAt(child, keep, x, y, coff, depth+1)
	}
}

// ─── Hit testing ─────────────────────────────────────────────────────────────

// hitTest возвращает самый верхний виджет (последний дочерний в Z-порядке),
// чьи bounds содержат точку (x, y). Возвращает nil, если точка вне дерева.
// Сам путь с кадрами виджетов строит hitPath (frames.go).
func hitTest(w widget.Widget, x, y int) widget.Widget {
	path := hitPath(w, x, y)
	if len(path) == 0 {
		return nil
	}
	return path[len(path)-1].w
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
	if sh := contentShift(w); sh != (image.Point{}) {
		cx, cy, coff = x+sh.X, y+sh.Y, off.Add(sh)
	}
	// Рекурсивно проверяем потомков (в обратном Z-порядке)
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if found, foff := findCapturerAt(children[i], cx, cy, ev, coff, depth+1); found != nil {
			return found, foff
		}
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
			hit = pt.In(overlayHitRect(w))
		} else if ob, ok := w.(widget.OverlayBoundsProvider); ok {
			hit = pt.In(ob.OverlayBounds())
		}
		if hit {
			return w, off
		}
	}

	return nil, image.Point{}
}

// overlayHitRect — область, в которой щелчок принадлежит оверлею виджета.
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
func overlayHitRect(w widget.Widget) image.Rectangle {
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
	broadcastMouseMoveAt(w, ox, oy, nx, ny, image.Point{}, 0)
}

// broadcastMouseMoveFrame — broadcastMouseMove для поддерева, корень которого
// (оверлей внутри прокрутки) лежит в кадре off.
func broadcastMouseMoveFrame(w widget.Widget, ox, oy, nx, ny int, off image.Point) {
	broadcastMouseMoveAt(w, ox+off.X, oy+off.Y, nx+off.X, ny+off.Y, off, 0)
}

// broadcastMouseMoveAt: обе точки — в кадре w, off — смещение этого кадра.
func broadcastMouseMoveAt(w widget.Widget, ox, oy, nx, ny int, off image.Point, depth int) {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
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
		broadcastMouseMoveAt(child, cox, coy, cnx, cny, coff, depth+1)
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
// d <= 0 возвращает к значению по умолчанию.
func (e *Engine) SetDoubleClickTime(d time.Duration) {
	e.dblClickDelay = d
}

// countClick возвращает номер нажатия в серии для MouseEvent.Clicks.
//
// Серию рвёт что угодно из трёх: другая кнопка, слишком долгая пауза,
// слишком далёкий курсор. Отпускание номера не меняет — оно несёт номер
// своего нажатия, чтобы обработчик release видел ту же серию, что и press.
func (e *Engine) countClick(x, y int, btn widget.MouseButton, pressed bool) int {
	if !pressed {
		if btn != e.clickBtn {
			return 0
		}
		return e.clickN
	}
	delay := e.dblClickDelay
	if delay <= 0 {
		delay = defaultDoubleClick
	}
	now := time.Now()
	sameSpot := abs(x-e.clickX) <= clickSlack && abs(y-e.clickY) <= clickSlack
	if btn == e.clickBtn && e.clickN > 0 && sameSpot && now.Sub(e.clickAt) <= delay {
		e.clickN++
	} else {
		e.clickN = 1
	}
	e.clickBtn, e.clickAt, e.clickX, e.clickY = btn, now, x, y
	return e.clickN
}

// abs — модуль целого (math.Abs просит float и возвращает float).
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
