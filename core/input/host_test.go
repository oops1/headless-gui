package input

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v4/widget"
)

// stubHost — движок для тестов ввода без движка: корень, модальные окна и
// журнал инвалидаций; Post выполняет сразу (цикла нет).
type stubHost struct {
	mu     sync.Mutex
	root   widget.Widget
	modals []widget.ModalWidget
	scale  float64
	rects  []image.Rectangle
	full   int
}

var _ Host = (*stubHost)(nil)

func (h *stubHost) Root() widget.Widget {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.root
}

func (h *stubHost) TopModal() widget.ModalWidget {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.modals) == 0 {
		return nil
	}
	return h.modals[len(h.modals)-1]
}

func (h *stubHost) Modals() []widget.ModalWidget {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]widget.ModalWidget(nil), h.modals...)
}

func (h *stubHost) RequestCloseModal(m widget.ModalWidget) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, x := range h.modals {
		if x == m {
			h.modals = append(h.modals[:i], h.modals[i+1:]...)
			return
		}
	}
}

func (h *stubHost) Scale() float64 {
	if h.scale == 0 {
		return 1
	}
	return h.scale
}

func (h *stubHost) Invalidate() {
	h.mu.Lock()
	h.full++
	h.mu.Unlock()
}

func (h *stubHost) InvalidateRect(r image.Rectangle) {
	h.mu.Lock()
	h.rects = append(h.rects, r)
	h.mu.Unlock()
}

func (h *stubHost) Post(fn func())                        { fn() }
func (h *stubHost) InLoop() bool                          { return true }
func (h *stubHost) GuardInput(string) func()              { return func() {} }
func (h *stubHost) RecordMouse(int, int)                  {}
func (h *stubHost) InvalidateShownTooltip()               {}
func (h *stubHost) CaptureManager() widget.CaptureManager { return nil }
