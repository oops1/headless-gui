package engine

// cmdpath.go — путь команд кадра (ADR-0004, задача 4а.1).
//
// Дерево корня пишется в рекордер команд (core/draw.Recorder): Draw виджета
// отдаёт не пиксели, а команды, и виджет, чей список команд ещё верен
// (widget/retain.go), не рисуется вовсе — в список родителя идёт ссылка.
// Затем список кадра воспроизводится в холст по повреждению (draw.Replay),
// дальше кадр идёт как прежде: дифф тайлов, оверлеи, модальные слои.
//
// Ничейное повреждение. Область, заявленная не виджетом о себе
// (Engine.InvalidateRect, widget.InvalidateRect, фокус движка), ничьих меток
// не ставит: виджет, чей охват её задевает, перезаписывается при визите.
// Уведомления виджетов, уже помеченных устаревшими (Base.Invalidate,
// SetBounds, состав детей), идут отдельным входом без этого.

import (
	"image"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"github.com/oops1/headless-gui/v3/widget"
)

// SetCommandPath включает или выключает путь команд. По умолчанию включён.
// Выключенный — прежний прямой путь: Draw каждого виджета прямо в холст на
// каждом кадре (с пропуском поддеревьев вне повреждения). Картинка на обоих
// путях одна и та же; выключатель — для сравнения и на случай виджета,
// рисующего по тому, чего рекордер не знает. Переключение перерисовывает
// кадр целиком.
func (e *Engine) SetCommandPath(on bool) {
	e.cmdPath.Store(on)
	e.Invalidate()
}

// CommandPath сообщает, включён ли путь команд.
func (e *Engine) CommandPath() bool { return e.cmdPath.Load() }

// takeForce забирает накопленное ничейное повреждение (логическое). Зовётся
// ПОСЛЕ consumeDamage: область, заявленная между двумя вызовами, перезапишет
// виджет уже сейчас, а её damage дойдёт следующим кадром — наоборот список
// остался бы устаревшим под уже забранным damage.
func (e *Engine) takeForce() []image.Rectangle {
	e.forceFrame = e.lp.TakeForce(e.forceFrame)
	return e.forceFrame
}

// recordFrame пишет в рекордер дерево корня, а за ним слои поверх дерева
// (toplayers.go: оверлеи, модальные, подсказка) и возвращает список кадра и
// клип на выходе из записи. Удержание списков включено вместе с пропуском
// поддеревьев (SetSubtreeCulling): выключенный пропуск означает «Draw каждый
// кадр». Паника в Draw отменяет запись (Recorder.Abort: открытые виджеты
// снова устарели, их списки не публикуются) и уходит границе кадра.
func (e *Engine) recordFrame(canvas *Canvas, root widget.Widget, modals []widget.ModalWidget, hosted, ovtOn bool, force []image.Rectangle) (list *hgdraw.List, exit hgdraw.ClipState) {
	// Кадр ради анимации виджета с фокусом (мигающая каретка): его команды
	// зависят от времени, а не от меток — перезаписать.
	if f := e.in.Focused(); f != nil {
		if a, ok := f.(widget.Animated); ok && a.NeedsAnimation() {
			if h, ok := f.(widget.ListHolder); ok {
				h.MarkDirty()
			}
		}
	}
	rec := e.rec
	rec.BeginFrame(canvas, canvas.SubtreeCullingEnabled(), force)
	done := false
	defer func() {
		if !done {
			rec.Abort()
		}
	}()
	widget.DrawTraced(root, rec)
	e.recordTop(rec, canvas, root, modals, hosted, ovtOn)
	done = true
	list, exit = rec.EndFrame()
	e.stats.recorded.Add(rec.Recorded)
	e.stats.retained.Add(rec.Retained)
	return list, exit
}

// replayTree воспроизводит список кадра в холст: при частичном кадре — только
// то, что задевает повреждение (в логических координатах, как границы
// виджетов); прямые виджеты (ExternalSurface) под прокруткой рисуются через
// OffsetContext, как на прямом пути.
func (e *Engine) replayTree(canvas *Canvas, list *hgdraw.List, exit hgdraw.ClipState, partial bool, damage image.Rectangle) {
	var dmg []image.Rectangle
	if partial {
		e.replayDmg[0] = damage
		dmg = e.logicalDamage(e.replayDmg[:])
	}
	e.replayer.Replay(canvas, list, hgdraw.ReplayOpts{Damage: dmg, Offset: offsetContextFor, Direct: drawDirectFor, Exit: exit})
}

func offsetContextFor(ctx hgdraw.DrawContext, off image.Point) hgdraw.DrawContext {
	return widget.OffsetContext(ctx, off.X, off.Y)
}

// drawDirectFor рисует прямой виджет при воспроизведении через DrawTraced:
// под холстом это прежний путь — учёт рисующихся для политики сбоев и
// заглушка сломанного виджета.
func drawDirectFor(h hgdraw.ListHolder, ctx hgdraw.DrawContext) {
	if w, ok := h.(widget.Widget); ok {
		widget.DrawTraced(w, ctx)
		return
	}
	h.Draw(ctx)
}
