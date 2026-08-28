// invalidate.go — рендер по запросу (on-demand) и damage-трекинг.
//
// Рендер по запросу — режим по умолчанию (с v3.5): кадр рендерится только
// когда UI инвалидирован, причём частично — в пределах damage-области.
// Прежнее поведение «рендер каждый тик» доступно через SetRenderOnDemand(false).
//
// Источники инвалидации:
//
//   - виджеты самоинвалидируются: сеттеры (SetText/SetValue/SetHovered/...)
//     и Base.SetBounds сообщают свой прямоугольник при фактическом изменении
//     (widget.SetUIRectChangeNotifier → InvalidateRect);
//   - события ввода: hover/drag — точечно через виджеты, клики и командные
//     хоткеи — полной инвалидацией, Tab-фокус — областями старого/нового
//     виджета; модалки, SetRoot/SetTheme и прочие API движка — автоматически;
//   - слой данных (BindingScope.Refresh, {Loc}, live-коллекции, смена
//     локали/языка) инвалидирует через widget.SetUIChangeNotifier;
//   - прямые записи в ЭКСПОРТИРОВАННЫЕ ПОЛЯ виджетов (btn.Text = "...")
//     движку не видны — после них нужен widget.Invalidate() либо
//     Engine.Invalidate()/InvalidateRect().
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
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// SetSubtreeCulling включает или выключает пропуск поддеревьев, не
// пересекающихся с изменившейся областью.
//
// По умолчанию включён. Выключатель нужен приложению, чьи виджеты нарушают
// контракт отрисовки (Draw не гарантирован каждый кадр — см. GUIDE, раздел
// «Контракт отрисовки»): одна строка возвращает прежнее поведение, пока
// нарушения разбираются.
func (e *Engine) SetSubtreeCulling(v bool) {
	widget.SetSubtreeCulling(v)
	e.Invalidate()
}

// SubtreeCulling сообщает, включён ли пропуск поддеревьев.
func (e *Engine) SubtreeCulling() bool { return widget.SubtreeCulling() }

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

// maxDamageRects — сколько областей движок держит по отдельности, прежде чем
// схлопнуть их в одно объединение.
//
// Порог нужен, чтобы список не рос без предела: сотня областей сама по себе
// дороже, чем перерисовать их общий прямоугольник. Шестнадцати хватает на
// любой разумный кадр — обычно их две-три.
const maxDamageRects = 16

// InvalidateRect помечает изменившейся прямоугольную область (в ЛОГИЧЕСКИХ
// пикселях холста — система координат виджетов). Diff ближайшего кадра
// ограничится тайлами, пересекающими ЗАЯВЛЕННЫЕ области.
// Внутри damage хранится в физических пикселях (масштабируется здесь).
//
// Области хранятся списком, а не одним объединением. Разница видна там, где
// изменения далеко друг от друга: перетаскивание рамки окна двигает две
// узкие полосы на разных краях экрана, и их объединение — почти весь экран.
// По объединению уходили тайлы всего прямоугольника (десятки килобайт на
// кадр), по списку — только тайлы самих полос.
func (e *Engine) InvalidateRect(r image.Rectangle) {
	if r.Empty() {
		return
	}
	// Масштаб читается lock-free (scaleBits): InvalidateRect вызывается из
	// сеттеров виджетов, в т.ч. когда движок уже держит e.mu.
	if k := e.Scale(); k != 1 {
		r = scaleRectF(r, k)
	}
	e.damageMu.Lock()
	if !e.damageAll {
		e.damage = addDamage(e.damage, r)
	}
	e.damageMu.Unlock()
	e.invGen.Add(1)
}

// addDamage добавляет область к списку, поглощая вложенные.
//
// Вложенные отбрасываются не ради экономии памяти, а потому что тайлы всё
// равно сравнивались бы дважды: первое сравнение синхронизирует буферы, и
// второе вернуло бы «не изменилось» — работа впустую.
func addDamage(list []image.Rectangle, r image.Rectangle) []image.Rectangle {
	// Проверка «уже покрыта» вынесена в отдельный проход СПЕЦИАЛЬНО, хотя
	// одного цикла хватило бы. Компактация идёт «на месте» (out := list[:0] —
	// тот же массив), и ранний возврат из общего цикла отдавал бы наружу уже
	// сдвинутый список. Сегодня это безвредно: сдвиг и ранний возврат разом
	// требуют вложенной пары в списке, а её тут не бывает — обе ветки ниже
	// вложенность как раз и устраняют. Но правильность кода не должна
	// держаться на этом рассуждении: два прохода по списку из шестнадцати
	// прямоугольников ничего не стоят, а рассуждение сломается от первой же
	// правки условий.
	for _, cur := range list {
		if r.In(cur) {
			return list // новая область уже покрыта — список не меняется
		}
	}

	out := list[:0]
	for _, cur := range list {
		if cur.In(r) {
			continue // старая поглощается новой
		}
		out = append(out, cur)
	}
	out = append(out, r)
	if len(out) > maxDamageRects {
		// Порог пройден: дальше дешевле работать одним прямоугольником.
		u := out[0]
		for _, cur := range out[1:] {
			u = u.Union(cur)
		}
		return append(out[:0], u)
	}
	return out
}

// scaleRectF масштабирует логический прямоугольник в физический по краям
// (та же математика, что canvas.sRect, но без доступа к канвасу).
func scaleRectF(r image.Rectangle, k float64) image.Rectangle {
	return image.Rect(
		int(math.Round(float64(r.Min.X)*k)),
		int(math.Round(float64(r.Min.Y)*k)),
		int(math.Round(float64(r.Max.X)*k)),
		int(math.Round(float64(r.Max.Y)*k)),
	)
}

// consumeDamage атомарно забирает накопленные повреждения.
// all==true — нужен полный diff (Invalidate или режим по умолчанию).
func (e *Engine) consumeDamage() (regions []image.Rectangle, all bool) {
	e.damageMu.Lock()
	regions, all = e.damage, e.damageAll
	e.damage = nil
	e.damageAll = false
	e.damageMu.Unlock()
	return regions, all
}

// unionRects — общий прямоугольник списка (пустой для пустого списка).
func unionRects(rects []image.Rectangle) image.Rectangle {
	var u image.Rectangle
	for _, r := range rects {
		u = u.Union(r)
	}
	return u
}

// RenderCount возвращает число фактически отрендеренных кадров с момента
// запуска (для тестов/диагностики экономии on-demand режима).
func (e *Engine) RenderCount() uint64 {
	return e.frameSeq.Load()
}

// animationNeeded — нужен ли кадр несмотря на отсутствие инвалидации:
// мигающая каретка у виджета с фокусом, активная анимация (widget.Animate)
// или «дозревающий» tooltip.
func (e *Engine) animationNeeded(frameInterval time.Duration) bool {
	if f := e.focus.get(); f != nil {
		if a, ok := f.(widget.Animated); ok && a.NeedsAnimation() {
			return true
		}
	}
	if widget.AnimationsActive() {
		return true
	}
	return e.tooltipMayAppear(frameInterval)
}
