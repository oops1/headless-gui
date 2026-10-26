package engine

// pointerleave.go — указатель покинул окно (v3.39).
//
// Окно сообщает об уходе указателя движением в точку вне окна (-1, -1): так
// снимаются наведение и подсказки (SendMouseMove). Виджету, которому нужно знать
// именно об уходе с ОКНА, по одной точке это не отличить от обычного движения
// мимо него — для этого есть widget.PointerLeaveHandler.

import "github.com/oops1/headless-gui/v3/widget"

// SendPointerLeave сообщает виджетам, что указатель покинул окно: каждому
// видимому виджету дерева (и модального окна, и открытых оверлеев), который
// реализует widget.PointerLeaveHandler, приходит OnPointerLeave. Сам по себе
// метод наведение не снимает — для этого окно присылает ещё и движение в точку
// вне окна (SendMouseMove), которое идёт перед вызовом.
func (e *Engine) SendPointerLeave() {
	defer e.guardInput("SendPointerLeave")()
	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
	// Модальное окно может лежать и в дереве корня, и отдельно от него:
	// обработчик зовётся один раз в любом случае.
	seen := map[widget.Widget]bool{}
	if root != nil {
		notifyPointerLeave(root, 0, seen)
	}
	if m := e.topModal(); m != nil {
		notifyPointerLeave(m, 0, seen)
	}
}

func notifyPointerLeave(w widget.Widget, depth int, seen map[widget.Widget]bool) {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) || seen[w] {
		return
	}
	seen[w] = true
	if h, ok := w.(widget.PointerLeaveHandler); ok {
		h.OnPointerLeave()
	}
	for _, c := range w.Children() {
		notifyPointerLeave(c, depth+1, seen)
	}
}
