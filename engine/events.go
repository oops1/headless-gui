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
	e.focus.set(w)
}

// SendKeyEvent доставляет клавиатурное событие виджету с фокусом.
// Tab / Shift+Tab перехватываются для переключения фокуса между виджетами.
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

	// Escape закрывает верхний модальный виджет
	if ev.Code == widget.KeyEscape && ev.Pressed {
		if m := e.topModal(); m != nil {
			e.CloseModal(nil) // закрывает верхний
			return
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

	e.focus.set(all[next])
}

// ─── Mouse events ────────────────────────────────────────────────────────────

// SendMouseMove уведомляет всё дерево виджетов о перемещении курсора в (x, y).
// Если есть виджет, захвативший мышь — событие идёт только ему.
// Если активен модальный виджет — broadcast только внутри него.
// Иначе — broadcast всему дереву.
func (e *Engine) SendMouseMove(x, y int) {
	// Если мышь захвачена — только захватчику
	if cap := e.getCaptured(); cap != nil {
		if mm, ok := cap.(widget.MouseMoveHandler); ok {
			mm.OnMouseMove(x, y)
		}
		return
	}

	// Модальный виджет: ограничиваем broadcast
	if m := e.topModal(); m != nil {
		broadcastMouseMove(m, x, y)
		return
	}

	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
	if root == nil {
		return
	}
	broadcastMouseMove(root, x, y)
}

// SendMouseButton уведомляет дерево о нажатии/отпускании кнопки мыши в (x, y).
// Если мышь захвачена — событие идёт только захватчику.
// Иначе: проверяем, хочет ли какой-либо предок захватить мышь (WantsCapture),
// затем передаём событие самому верхнему виджету под курсором.
func (e *Engine) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool) {
	ev := widget.MouseEvent{X: x, Y: y, Button: btn, Pressed: pressed}

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
			if !found {
				// Виджет-поглотитель исчез — проглатываем release
				return
			}
		}
	}

	// Если мышь захвачена — только захватчику
	if cap := e.getCaptured(); cap != nil {
		if mc, ok := cap.(widget.MouseClickHandler); ok {
			mc.OnMouseButton(ev)
		}
		return
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

	// Проверяем, хочет ли кто-то из предков захватить мышь (drag handle)
	if pressed && btn == widget.MouseLeft {
		if capturer := findCapturer(dispatchRoot, x, y, ev); capturer != nil {
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
				dismissOutside(dispatchRoot, pathSet)
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

	// Сначала проверяем: есть ли виджет с активным overlay под курсором.
	if overlayW := findOverlayAt(dispatchRoot, x, y); overlayW != nil {
		if pressed && btn == widget.MouseLeft {
			if _, ok := overlayW.(widget.Focusable); ok {
				e.focus.set(overlayW)
			}
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
		dismissOutside(dispatchRoot, pathSet)
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

// ─── Dismiss ─────────────────────────────────────────────────────────────────

// dismissOutside рекурсивно закрывает все Dismissable-виджеты, которые
// не входят в набор keep (виджеты на пути от корня до клика).
// Это гарантирует закрытие popup/dropdown/menu при клике в другое место.
func dismissOutside(w widget.Widget, keep map[widget.Widget]struct{}) {
	if _, inPath := keep[w]; !inPath {
		if d, ok := w.(widget.Dismissable); ok {
			d.Dismiss()
		}
	}
	for _, child := range w.Children() {
		dismissOutside(child, keep)
	}
}

// ─── Hit testing ─────────────────────────────────────────────────────────────

// hitTest возвращает самый верхний виджет (последний дочерний в Z-порядке),
// чьи bounds содержат точку (x, y). Возвращает nil, если точка вне дерева.
func hitTest(w widget.Widget, x, y int) widget.Widget {
	if !image.Pt(x, y).In(w.Bounds()) {
		return nil
	}
	// Дети рисуются поверх родителя — проверяем в обратном порядке
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if hit := hitTest(children[i], x, y); hit != nil {
			return hit
		}
	}
	return w
}

// hitTestPath возвращает путь от корня до самого глубокого виджета под (x, y).
// Путь: [root, ..., parent, hit]. Пустой срез — точка вне дерева.
// Используется для event bubbling.
func hitTestPath(w widget.Widget, x, y int) []widget.Widget {
	if !image.Pt(x, y).In(w.Bounds()) {
		return nil
	}
	// Проверяем детей в обратном Z-порядке
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if path := hitTestPath(children[i], x, y); path != nil {
			return append([]widget.Widget{w}, path...)
		}
	}
	return []widget.Widget{w}
}

// findCapturer ищет виджет, который хочет захватить мышь, в цепочке предков
// от корня до hit-виджета. Возвращает ближайшего к hit (самого вложенного).
func findCapturer(w widget.Widget, x, y int, ev widget.MouseEvent) widget.Widget {
	pt := image.Pt(x, y)
	if !pt.In(w.Bounds()) {
		return nil
	}
	// Рекурсивно проверяем потомков (в обратном Z-порядке)
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if found := findCapturer(children[i], x, y, ev); found != nil {
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
	pt := image.Pt(x, y)

	// Проверяем детей в обратном Z-порядке (верхние первыми).
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if found := findOverlayAt(children[i], x, y); found != nil {
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

// broadcastMouseMove рекурсивно доставляет событие перемещения мыши
// всему дереву виджетов (не только тем, что под курсором).
// Каждый виджет сам определяет своё hover-состояние через image.Pt(x,y).In(bounds).
func broadcastMouseMove(w widget.Widget, x, y int) {
	if mm, ok := w.(widget.MouseMoveHandler); ok {
		mm.OnMouseMove(x, y)
	}
	for _, child := range w.Children() {
		broadcastMouseMove(child, x, y)
	}
}
