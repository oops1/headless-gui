package overlay

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/core/raster"
	"github.com/oops1/headless-gui/v4/core/text"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// stubHost — движок для тестов оверлеев без движка: журнал повреждений.
type stubHost struct {
	damages []image.Rectangle
	bumps   int
}

var _ Host = (*stubHost)(nil)

func (h *stubHost) TopModal() widget.ModalWidget { return nil }
func (h *stubHost) DamageRect(r image.Rectangle) { h.damages = append(h.damages, r) }
func (h *stubHost) BumpGen()                     { h.bumps++ }
func (h *stubHost) ThemeManager() *theme.Manager { return nil }
func (h *stubHost) TransitionsOn() bool          { return false }
func (h *stubHost) SlideDir(widget.Widget, image.Rectangle, int) image.Point {
	return image.Point{}
}

// Закрытие модального окна получают и слот хоста (SetOnModalClosed), и
// подписчики AddOnModalClosed; снятый подписчик больше не получает (GG-82).
func TestModalClosed_SlotAndSubscribers(t *testing.T) {
	testreset.Globals(t)
	m := New(&stubHost{})
	dlg := widget.NewDialog("Диалог", 100, 80)
	var slot, sub int
	m.SetOnModalClosed(func(mw widget.ModalWidget) {
		if mw == widget.ModalWidget(dlg) {
			slot++
		}
	})
	id := m.AddOnModalClosed(func(widget.ModalWidget) { sub++ })
	m.NotifyModalClosed(dlg)
	if slot != 1 || sub != 1 {
		t.Fatalf("слот %d, подписчик %d — ждали оба по разу", slot, sub)
	}
	m.RemoveOnModalClosed(id)
	m.NotifyModalClosed(dlg)
	if slot != 2 || sub != 1 {
		t.Fatalf("после снятия подписчика: слот %d, подписчик %d", slot, sub)
	}
}

// Показанная плашка подсказки запоминается; движение мыши стирает её
// повреждением области у хоста (дерево под ней не менялось), и область
// забывается.
func TestTooltip_ShownAreaErasedThroughHost(t *testing.T) {
	testreset.Globals(t)
	h := &stubHost{}
	m := New(h)
	c := raster.NewCanvas(200, 100, text.NewFontCache("assets"))
	r := image.Rect(20, 30, 120, 52)
	m.DrawTooltipBox(c, "подсказка", r)
	if got := m.TooltipShownAt(); got != r {
		t.Fatalf("показанная область %v, ждали %v", got, r)
	}
	m.InvalidateShownTooltip()
	if len(h.damages) != 1 || h.damages[0] != r {
		t.Fatalf("повреждения у хоста %v, ждали одно %v", h.damages, r)
	}
	if !m.TooltipShownAt().Empty() {
		t.Error("область показанной подсказки не забыта")
	}
	m.InvalidateShownTooltip()
	if len(h.damages) != 1 {
		t.Error("стирание без показанной подсказки заявило повреждение")
	}
}
