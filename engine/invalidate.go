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
	e.canvas.SetSubtreeCulling(v)
	e.Invalidate()
}

// SubtreeCulling сообщает, включён ли пропуск поддеревьев.
func (e *Engine) SubtreeCulling() bool { return e.canvas.SubtreeCullingEnabled() }

// SetOcclusionCulling включает или выключает вычитание перекрытого: поддерево,
// целиком накрытое непрозрачным соседом, не рисуется (widget/occlusion.go).
//
// По умолчанию включено. Выключатель нужен приложению, чей виджет объявляет
// непрозрачной площадь, которую на деле закрашивает не полностью: под таким
// объявлением остаётся дыра, и одна строка возвращает прежнее поведение, пока
// объявление ищут.
func (e *Engine) SetOcclusionCulling(v bool) {
	e.canvas.SetOcclusionCulling(v)
	e.Invalidate()
}

// OcclusionCulling сообщает, включено ли вычитание перекрытого.
func (e *Engine) OcclusionCulling() bool { return e.canvas.OcclusionEnabled() }

// SetRenderOnDemand включает/выключает рендер по запросу.
// Безопасно вызывать в любой момент; включение сразу инвалидирует кадр.
func (e *Engine) SetRenderOnDemand(v bool) {
	e.lp.SetRenderOnDemand(v)
}

// RenderOnDemand сообщает, включён ли рендер по запросу.
func (e *Engine) RenderOnDemand() bool {
	return e.lp.RenderOnDemand()
}

// Invalidate помечает весь кадр как изменившийся: ближайший тик отрендерит его.
// Дёшево (атомарный инкремент) — можно вызывать часто.
func (e *Engine) Invalidate() {
	e.lp.Invalidate()
}

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
	e.lp.InvalidateRect(r)
}

// damageRect — InvalidateRect без ничейного повреждения: для областей, под
// которыми дерево виджетов не менялось (подсказка, переходы оверлеев) и для
// уведомлений виджетов, уже помеченных устаревшими (widget/retain.go).
func (e *Engine) damageRect(r image.Rectangle) {
	e.lp.DamageRect(r)
}

// noteDamage добавляет область к повреждению кадра, не объявляя UI
// изменившимся (invGen): для кадра, который цикл и так готовит. false —
// область пуста.
func (e *Engine) noteDamage(r image.Rectangle) bool {
	return e.lp.NoteDamage(r)
}

// consumeDamage атомарно забирает накопленные повреждения.
// all==true — нужен полный diff (Invalidate или режим по умолчанию).
// wholeFramePending — следующий кадр будет полным: заявлена полная
// инвалидация или полный кадр (fullframe.go). Без сброса.
func (e *Engine) wholeFramePending() bool {
	return e.lp.WholeFramePending()
}

func (e *Engine) consumeDamage() (regions []image.Rectangle, all bool) {
	return e.lp.ConsumeDamage()
}

// peekDamage — объединение заявленного на сейчас, не забирая его: что
// перерисует ближайший кадр как минимум. all — весь кадр.
func (e *Engine) peekDamage() (union image.Rectangle, all bool) {
	return e.lp.PeekDamage()
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
	if f := e.in.Focused(); f != nil {
		if a, ok := f.(widget.Animated); ok && a.NeedsAnimation() {
			// Кадр ради каретки — частичный, по области виджета: прежде он
			// шёл полным (фон, обход всего дерева, все тайлы), на пути команд
			// (cmdpath.go) перезаписывается и воспроизводится один виджет.
			// Без invGen: кадр готовится сейчас, второй за ним не нужен.
			if h, ok := f.(widget.ListHolder); ok {
				h.MarkDirty()
			}
			e.noteDamage(e.in.ScreenBoundsOf(f))
			return true
		}
	}
	// Зарегистрированная анимация поводом для кадра НЕ считается.
	//
	// Цикл зовёт widget.StepAnimations ДО решения о пропуске, и тик анимации
	// двигает виджет через сеттеры — те заявляют damage сами. Значит к этому
	// месту всё, что анимация изменила, уже в damage, а спрашивать про сам
	// факт её существования — значит готовить кадр, которому нечего делать.
	//
	// Стоило это дорого: при пустом damage кадр идёт не частичным путём, а
	// ПОЛНЫМ — блит фона во весь холст, обход всего дерева без клипа,
	// сравнение всех тайлов. Секундные часы на панели задач держат
	// зациклённую анимацию вечно, и неподвижный рабочий стол съедал этим
	// ядро целиком, готовя шестьдесят полных кадров в секунду.
	//
	// Анимация, чей тик меняет виджет, не заявляя damage, кадра больше не
	// получит — но она сломана и без того: с версии 3.15.0 движок вправе не
	// звать Draw у поддерева вне damage (см. «Контракт отрисовки» в GUIDE).
	return e.tooltipMayAppear(frameInterval)
}

// unionRects — общий прямоугольник списка (пустой для пустого списка).
func unionRects(rects []image.Rectangle) image.Rectangle {
	var u image.Rectangle
	for _, r := range rects {
		u = u.Union(r)
	}
	return u
}
