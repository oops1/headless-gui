// Package engine — диспетчер событий ввода (мышь, клавиатура, фокус).
package engine

import (
	"image"
	"sync"

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
	e.capMu.Unlock()
}

// ReleaseCapture отменяет захват мыши.
func (e *Engine) ReleaseCapture() {
	e.capMu.Lock()
	e.captured = nil
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
	if old := e.focus.get(); old != nil {
		e.InvalidateRect(old.Bounds())
	}
	e.focus.set(w)
	if w != nil {
		e.InvalidateRect(w.Bounds())
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
	// Tab-навигация: перехватываем Tab до доставки виджету.
	// При активном модальном виджете Tab циклит только внутри него.
	if ev.Code == widget.KeyTab && ev.Pressed {
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
			if c, ok := m.(interface{ OnCancel() }); ok {
				c.OnCancel()
			}
			e.CloseModal(nil) // закрывает верхний
			return
		}
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
	path := hitTestPath(disp, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		if ov, ok := path[i].(interface{ CursorOverride() (widget.Cursor, bool) }); ok {
			if c, has := ov.CursorOverride(); has {
				return c
			}
		}
		if cp, ok := path[i].(widget.CursorProvider); ok {
			return cp.Cursor(x, y)
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
			mm.OnMouseMove(x, y)
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
	if ov := findOverlayAt(root, x, y); ov != nil {
		// Сначала — всему дереву «курсора над вами нет». Без этого кнопка, с
		// которой курсор ушёл под оверлей, осталась бы подсвеченной навсегда:
		// она бы просто перестала получать события.
		broadcastMouseMove(root, ox, oy, widget.CursorNowhere, widget.CursorNowhere)
		// Затем — настоящая точка тому, кому она принадлежит, и его детям.
		broadcastMouseMove(ov, ox, oy, x, y)
		return
	}

	broadcastMouseMove(root, ox, oy, x, y)
}

// SendMouseButton уведомляет дерево о нажатии/отпускании кнопки мыши в (x, y).
// Если мышь захвачена — событие идёт только захватчику.
// Иначе: проверяем, хочет ли какой-либо предок захватить мышь (WantsCapture),
// затем передаём событие самому верхнему виджету под курсором.
func (e *Engine) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool) {
	x, y = e.toLogical(x, y)

	// Клик оставляет ПОЛНУЮ инвалидацию сознательно: он может открыть/закрыть
	// overlay (dropdown, меню), сместить фокус, выполнить команду — задеть
	// произвольные области. Клики редки, полный кадр здесь дёшев и надёжен.
	e.Invalidate()
	ev := widget.MouseEvent{X: x, Y: y, Button: btn, Pressed: pressed}

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
			mc.OnMouseButton(ev)
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
			path := hitTestPath(dispRoot, x, y)
			found := false
			for _, w := range path {
				if w == consumer {
					found = true
					break
				}
			}
			// Если не найден в обычном дереве — проверяем overlay-виджеты.
			// MenuBar (и другие OverlayDrawer) владеют popup как полем, а не дочерним
			// виджетом, поэтому hitTestPath их не находит в области popup'а.
			if !found {
				if od, ok := consumer.(widget.OverlayDrawer); ok && od.HasOverlay() {
					pt := image.Pt(x, y)
					if pt.In(consumer.Bounds()) {
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
		path := hitTestPath(dispatchRoot, x, y)
		for i := len(path) - 1; i >= 0; i-- {
			if h, ok := path[i].(interface{ GetContextMenu() *widget.PopupMenu }); ok {
				if pm := h.GetContextMenu(); pm != nil {
					pm.Show(x, y)
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
	if overlayW := findOverlayAt(dispatchRoot, x, y); overlayW != nil {
		if pressed && btn == widget.MouseLeft {
			if _, ok := overlayW.(widget.Focusable); ok {
				e.focus.set(overlayW)
			}
			// Гасим ЧУЖИЕ оверлеи — ровно как на обычном пути доставки.
			// Без этого оверлей, поглотивший клик, оставлял открытыми все
			// остальные: клик внутри календаря не закрывал ни меню «Пуск»,
			// ни соседнюю панель, потому что до dismissOutside дело не
			// доходило вовсе.
			keep := map[widget.Widget]struct{}{overlayW: {}}
			for _, w := range hitTestPath(dispatchRoot, x, y) {
				keep[w] = struct{}{}
			}
			dismissOutside(dispatchRoot, keep, x, y)
		}
		if mc, ok := overlayW.(widget.MouseClickHandler); ok {
			if mc.OnMouseButton(ev) {
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
		if capturer := findCapturer(dispatchRoot, x, y, ev); capturer != nil {
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
			capPath := hitTestPath(dispatchRoot, x, y)
			if len(capPath) > 0 {
				pathSet := make(map[widget.Widget]struct{}, len(capPath))
				for _, w := range capPath {
					pathSet[w] = struct{}{}
				}
				dismissOutside(dispatchRoot, pathSet, x, y)
			}

			// Запоминаем capturer как pressConsumer — если capturer
			// будет закрыт/удалён, release не пролетит на виджет снизу.
			e.pressConsumer = capturer

			if mc, ok := capturer.(widget.MouseClickHandler); ok {
				mc.OnMouseButton(ev)
			}
			return
		}
	}

	// Получаем путь от корня до самого глубокого виджета под курсором
	path := hitTestPath(dispatchRoot, x, y)
	if len(path) == 0 {
		return
	}
	hit := path[len(path)-1]

	// При нажатии — передаём фокус и закрываем overlay'и вне пути.
	if pressed && btn == widget.MouseLeft {
		if _, ok := hit.(widget.Focusable); ok {
			e.focus.set(hit)
		} else {
			e.focus.set(nil)
		}

		// Закрываем все Dismissable-виджеты, которые НЕ лежат на пути
		// от корня до целевого виджета (dropdown/popup/menu вне клика).
		pathSet := make(map[widget.Widget]struct{}, len(path))
		for _, w := range path {
			pathSet[w] = struct{}{}
		}
		dismissOutside(dispatchRoot, pathSet, x, y)
	}

	// Доставляем событие с bubbling: от самого глубокого виджета к корню.
	// Если виджет поглотил событие (вернул true) — bubbling останавливается.
	for i := len(path) - 1; i >= 0; i-- {
		if mc, ok := path[i].(widget.MouseClickHandler); ok {
			if mc.OnMouseButton(ev) {
				// Запоминаем поглотивший виджет, чтобы при release проверить,
				// остался ли он под курсором (иначе release проглатывается).
				if pressed && btn == widget.MouseLeft {
					e.pressConsumer = path[i]
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
		path := hitTestPath(dispatchRoot, x, y)
		for i := len(path) - 1; i >= 0; i-- {
			if h, ok := path[i].(wheelPixelHandler); ok {
				if h.OnMouseWheelPixels(x, y, dx, dy) {
					e.invalidateWidget(path[i])
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
	var consumer widget.Widget
	for i := 0; i < steps; i++ {
		if w := deliverWheelTick(targets, x, y, btn); w != nil {
			consumer = w
		}
	}
	if consumer != nil {
		e.invalidateWidget(consumer)
	}
}

// wheelTargets — получатели тика колеса по порядку: захватчик мыши либо
// оверлей под курсором и путь hit-test снизу вверх.
func (e *Engine) wheelTargets(dispatchRoot widget.Widget, x, y int) []widget.Widget {
	if cap := e.getCaptured(); cap != nil {
		return []widget.Widget{cap}
	}
	if dispatchRoot == nil {
		return nil
	}
	var out []widget.Widget
	if ov := findOverlayAt(dispatchRoot, x, y); ov != nil {
		out = append(out, ov)
	}
	path := hitTestPath(dispatchRoot, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		out = append(out, path[i])
	}
	return out
}

// deliverWheelTick шлёт один тик (press+release) по списку получателей;
// возвращает виджет, поглотивший нажатие.
func deliverWheelTick(targets []widget.Widget, x, y int, btn widget.MouseButton) widget.Widget {
	var consumer widget.Widget
	for _, pressed := range [2]bool{true, false} {
		ev := widget.MouseEvent{X: x, Y: y, Button: btn, Pressed: pressed}
		for _, w := range targets {
			mc, ok := w.(widget.MouseClickHandler)
			if !ok {
				continue
			}
			if mc.OnMouseButton(ev) {
				if pressed {
					consumer = w
				}
				break
			}
		}
	}
	return consumer
}

// invalidateWidget помечает область виджета; при пустых bounds — весь кадр.
func (e *Engine) invalidateWidget(w widget.Widget) {
	if b := w.Bounds(); !b.Empty() {
		e.InvalidateRect(b)
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

	path := hitTestPath(dispatchRoot, x, y)
	for i := len(path) - 1; i >= 0; i-- {
		if fd, ok := path[i].(widget.FileDropTarget); ok {
			if fd.OnFilesDropped(x, y, paths) {
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
func dismissOutside(w widget.Widget, keep map[widget.Widget]struct{}, x, y int) {
	dismissOutsideAt(w, keep, x, y, 0)
}

func dismissOutsideAt(w widget.Widget, keep map[widget.Widget]struct{}, x, y, depth int) {
	if tooDeep(depth) {
		return
	}
	if _, inPath := keep[w]; !inPath {
		if d, ok := w.(widget.DismissableAt); ok {
			d.DismissAt(x, y)
		} else if d, ok := w.(widget.Dismissable); ok {
			d.Dismiss()
		}
	}
	for _, child := range w.Children() {
		dismissOutsideAt(child, keep, x, y, depth+1)
	}
}

// ─── Hit testing ─────────────────────────────────────────────────────────────

// hitTest возвращает самый верхний виджет (последний дочерний в Z-порядке),
// чьи bounds содержат точку (x, y). Возвращает nil, если точка вне дерева.
func hitTest(w widget.Widget, x, y int) widget.Widget {
	return hitTestAt(w, x, y, 0)
}

func hitTestAt(w widget.Widget, x, y, depth int) widget.Widget {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return nil
	}
	if !image.Pt(x, y).In(w.Bounds()) {
		return nil
	}
	// Дети рисуются поверх родителя — проверяем в обратном порядке
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if hit := hitTestAt(children[i], x, y, depth+1); hit != nil {
			return hit
		}
	}
	return w
}

// hitTestPath возвращает путь от корня до самого глубокого виджета под (x, y).
// Путь: [root, ..., parent, hit]. Пустой срез — точка вне дерева.
// Используется для event bubbling.
func hitTestPath(w widget.Widget, x, y int) []widget.Widget {
	return appendHitTestPath(nil, w, x, y, 0)
}

// appendHitTestPath дописывает путь [w, ..., hit] в dst и возвращает результат;
// nil — точка вне w.
//
// PERF-14: прежняя реализация на каждом уровне рекурсии делала
// append([]Widget{w}, path...) — новый срез и полное копирование хвоста, т.е.
// O(depth²) аллокаций и копий на каждое движение мыши. Здесь путь растёт в один
// накопитель сверху вниз: одна амортизированная аллокация на весь путь.
//
// Аллиасинг безопасен: nil возвращается ТОЛЬКО до append (первые две проверки),
// поэтому «протухшая» ссылка на dst у родителя после реаллокации в потомке
// никогда не используется.
func appendHitTestPath(dst []widget.Widget, w widget.Widget, x, y, depth int) []widget.Widget {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return nil
	}
	if !image.Pt(x, y).In(w.Bounds()) {
		return nil
	}
	n := len(dst)
	dst = append(dst, w)
	// Проверяем детей в обратном Z-порядке
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if path := appendHitTestPath(dst, children[i], x, y, depth+1); path != nil {
			return path
		}
	}
	return dst[:n+1]
}

// findCapturer ищет виджет, который хочет захватить мышь, в цепочке предков
// от корня до hit-виджета. Возвращает ближайшего к hit (самого вложенного).
func findCapturer(w widget.Widget, x, y int, ev widget.MouseEvent) widget.Widget {
	return findCapturerAt(w, x, y, ev, 0)
}

func findCapturerAt(w widget.Widget, x, y int, ev widget.MouseEvent, depth int) widget.Widget {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return nil
	}
	pt := image.Pt(x, y)
	if !pt.In(w.Bounds()) {
		return nil
	}
	// Рекурсивно проверяем потомков (в обратном Z-порядке)
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if found := findCapturerAt(children[i], x, y, ev, depth+1); found != nil {
			return found
		}
	}
	// Проверяем сам виджет
	if cr, ok := w.(widget.CaptureRequester); ok {
		if cr.WantsCapture(ev) {
			return w
		}
	}
	return nil
}

// findOverlayAt ищет виджет с активным overlay (popup/dropdown/menu),
// чьи расширенные bounds (включая overlay) содержат точку (x, y).
// Overlay имеет приоритет над обычным Z-порядком дерева виджетов.
// Возвращает nil, если ни один overlay не содержит точку.
func findOverlayAt(w widget.Widget, x, y int) widget.Widget {
	return findOverlayAtDepth(w, x, y, 0)
}

func findOverlayAtDepth(w widget.Widget, x, y, depth int) widget.Widget {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return nil
	}
	pt := image.Pt(x, y)

	// Проверяем детей в обратном Z-порядке (верхние первыми).
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if found := findOverlayAtDepth(children[i], x, y, depth+1); found != nil {
			return found
		}
	}

	// Проверяем сам виджет: есть ли активный overlay и попадает ли точка в него.
	if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
		if pt.In(w.Bounds()) {
			return w
		}
	}

	return nil
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
func broadcastMouseMove(w widget.Widget, ox, oy, nx, ny int) {
	broadcastMouseMoveAt(w, ox, oy, nx, ny, 0)
}

func broadcastMouseMoveAt(w widget.Widget, ox, oy, nx, ny, depth int) {
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
			mm.OnMouseMove(nx, ny)
		}
	}
	for _, child := range w.Children() {
		broadcastMouseMoveAt(child, ox, oy, nx, ny, depth+1)
	}
}
