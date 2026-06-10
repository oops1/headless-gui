// invalidate.go — рендер по запросу (on-demand) и damage-трекинг.
//
// По умолчанию движок перерисовывает кадр на каждый тик (полная обратная
// совместимость). В режиме SetRenderOnDemand(true) кадр рендерится только
// когда UI инвалидирован:
//
//   - события ввода (мышь/клавиатура), фокус, модалки, SetRoot/SetTheme и
//     прочие API движка инвалидируют автоматически;
//   - слой данных (BindingScope.Refresh, {Loc}, live-коллекции, смена
//     локали/языка) инвалидирует через widget.SetUIChangeNotifier;
//   - прямые мутации виджетов из кода приложения (label.SetText из своей
//     горутины) требуют явного Engine.Invalidate()/InvalidateRect().
//
// Анимации, привязанные ко времени (мигающая каретка TextInput/DataGrid,
// дозревающий tooltip), учитываются отдельно: пока виджет с фокусом
// возвращает widget.Animated.NeedsAnimation()==true или tooltip «дозревает»,
// кадры не пропускаются.
//
// InvalidateRect дополнительно ограничивает СРАВНЕНИЕ буферов (diff) тайлами,
// пересекающими повреждённую область — отрисовка дерева остаётся полной.
// Контракт: в on-demand режиме вызывающий обязан сообщать ВСЕ изменившиеся
// области (или использовать Invalidate без прямоугольника).
package engine

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// SetRenderOnDemand включает/выключает рендер по запросу.
// Безопасно вызывать в любой момент; включение сразу инвалидирует кадр.
func (e *Engine) SetRenderOnDemand(v bool) {
	e.onDemand.Store(v)
	e.Invalidate()
}

// RenderOnDemand сообщает, включён ли рендер по запросу.
func (e *Engine) RenderOnDemand() bool {
	return e.onDemand.Load()
}

// Invalidate помечает весь кадр как изменившийся: ближайший тик отрендерит его.
// Дёшево (атомарный инкремент) — можно вызывать часто.
func (e *Engine) Invalidate() {
	e.damageMu.Lock()
	e.damageAll = true
	e.damageMu.Unlock()
	e.invGen.Add(1)
}

// InvalidateRect помечает изменившейся прямоугольную область (в пикселях
// холста). Diff ближайшего кадра ограничится тайлами, пересекающими
// объединение заявленных областей.
func (e *Engine) InvalidateRect(r image.Rectangle) {
	if r.Empty() {
		return
	}
	e.damageMu.Lock()
	if !e.damageAll {
		e.damage = e.damage.Union(r)
	}
	e.damageMu.Unlock()
	e.invGen.Add(1)
}

// consumeDamage атомарно забирает накопленное повреждение.
// all==true — нужен полный diff (Invalidate или режим по умолчанию).
func (e *Engine) consumeDamage() (region image.Rectangle, all bool) {
	e.damageMu.Lock()
	region, all = e.damage, e.damageAll
	e.damage = image.Rectangle{}
	e.damageAll = false
	e.damageMu.Unlock()
	return region, all
}

// RenderCount возвращает число фактически отрендеренных кадров с момента
// запуска (для тестов/диагностики экономии on-demand режима).
func (e *Engine) RenderCount() uint64 {
	return e.frameSeq.Load()
}

// animationNeeded — нужен ли кадр несмотря на отсутствие инвалидации:
// мигающая каретка у виджета с фокусом или «дозревающий» tooltip.
func (e *Engine) animationNeeded(frameInterval time.Duration) bool {
	if f := e.focus.get(); f != nil {
		if a, ok := f.(widget.Animated); ok && a.NeedsAnimation() {
			return true
		}
	}
	return e.tooltipMayAppear(frameInterval)
}
