package engine

// owner.go — движок как владелец дерева (ADR-0008, задача 5.1).
//
// Корень (SetRoot) и модальные окна в холсте (ShowModal) привязываются к
// движку через widget.BindOwner, а сам движок регистрируется в реестре
// уведомлений как приёмник-владелец (widget.RegisterUINotifierFor, см. New):
// уведомления своих деревьев доходят до него, деревьев других движков — нет;
// наблюдатели реестра и внешний слот получают всё. CloseModal и Stop
// привязку снимают.

import (
	"github.com/oops1/headless-gui/v4/widget"
	"github.com/oops1/headless-gui/v4/widget/svg"
)

// treeOwner — ключ владельца: сравнимое значение, по которому приёмник
// реестра узнаёт себя. Отдельный тип, а не *Engine: маркер интерфейса не
// должен расширять открытый API движка.
type treeOwner struct{ e *Engine }

var _ widget.Owner = treeOwner{}

// TreeOwner — маркер widget.Owner.
func (treeOwner) TreeOwner() {}

// Службы владельца (5.2, widget/owner_services.go): замер текста шрифтами и
// масштабом этого движка, цикл анимаций и растеризатор <text> для SVG.
var (
	_ widget.OwnerMeasurer       = treeOwner{}
	_ widget.OwnerAnimationLoop  = treeOwner{}
	_ widget.OwnerTextRasterizer = treeOwner{}
	_ widget.OwnerPoster         = treeOwner{}
)

// OwnerPost — Post движка: поток интерфейса дерева (widget.PostFor).
func (o treeOwner) OwnerPost(fn func()) { o.e.Post(fn) }

// OwnerMeasurers — измерители движка.
func (o treeOwner) OwnerMeasurers() *widget.MeasureSet { return o.e.measureSet }

// OwnerLoopKey — ключ цикла анимаций движка (BindAnimationLoop(e)); nil, пока
// цикл не запущен или уже остановлен: такую анимацию никто бы не шагал, а без
// ключа её заберёт работающий цикл, как прежде (движок, которому кадры
// заказывают RenderOnce, анимации не шагает).
func (o treeOwner) OwnerLoopKey() any {
	if !o.e.lp.Started() || o.e.lp.Stopped() {
		return nil
	}
	return o.e
}

// OwnerTextRasterizer — шрифты движка для <text> в SVG.
func (o treeOwner) OwnerTextRasterizer() svg.TextRasterizer { return svgTextBridge{o.e} }

// bindRoot привязывает новый корень и отвязывает прежний: дерево, отданное
// другому движку, не должно носить старую привязку.
func (e *Engine) bindRoot(prev, next widget.Widget) {
	if prev != nil && prev != next {
		if widget.OwnerOf(prev) == widget.Owner(treeOwner{e}) {
			widget.BindOwner(prev, nil)
		}
	}
	if next != nil {
		widget.BindOwner(next, treeOwner{e})
	}
}

// unbindAll снимает привязку у корня и модальных окон (Stop).
func (e *Engine) unbindAll() {
	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
	if root != nil {
		widget.BindOwner(root, nil)
	}
	e.modMu.Lock()
	modals := append([]widget.ModalWidget(nil), e.modals...)
	e.modMu.Unlock()
	for _, m := range modals {
		widget.BindOwner(m, nil)
	}
}
