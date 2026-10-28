package engine

// overlaytrans.go — переходы оверлеев: появление и исчезновение меню,
// раскрывающихся списков, палитр (задача 3.3 плана 4.0; запрос WinLine —
// «анимация разворачивания меню»).
//
// Включается Engine.SetOverlayTransitions(true); длительность и кривая —
// токен темы theme.AnimMenuOpen (тема — из ApplyThemeProfile или
// SetThemeProfile), сдвиг — метрика theme.KeyMenuOpenOffset. Профили
// движения MotionReduced и MotionOff обнуляют токен — переходов нет;
// MotionRemote квантует шаги.
//
// Как это устроено. Оверлей рисуется прямо в холст поверх дерева, и своих
// пикселей у него нет: акриловая подложка размывает то, что под меню, тень
// полупрозрачна. Поэтому движок снимает с холста ПАРУ снимков области
// оверлея (с запасом под тень): «под» — до DrawOverlay, «с» — после. Кадр
// перехода — смешение одного с другим в доле прогресса, «с» сдвинутым на
// остаток сдвига. Пиксели, которых оверлей не коснулся (там «с» равно «под»),
// при сдвиге не едут — иначе фон вокруг меню поехал бы вместе с ним.
//
// Снимки делаются только в кадре, где область оверлея перерисовывается
// целиком: тогда «под» — свежее дерево. Повреждение, задевшее область хоть
// краем, расширяется на неё целиком (ovtPrepare) — дерево под меню может
// меняться и во время перехода, и снимки тогда обновляются. Шаг перехода сам
// дерево не перерисовывает: тик просит кадр без повреждения, и кадр только
// смешивает снимки в буфер и сравнивает область (renderStepFrame). Так можно,
// только пока над переходом ничего не лежит: он в верхнем слое и не
// пересекается ни с другими оверлеями, ни с подсказкой. Иначе шаг заявляет
// область повреждением, и кадр рисует слои по порядку. Конец перехода
// рисуется всегда обычным путём: последний кадр — прямая отрисовка
// оверлея, с настоящими признаками содержимого тайлов.
//
// Закрытие: виджет гасит HasOverlay сразу, и движок узнаёт об этом, встретив
// владельца при обходе с погашенным оверлеем. Там же, на месте владельца в
// порядке рисования, идёт и смешение исчезающего — он остаётся под теми, кто
// рисуется позже. Ввод закрывающийся оверлей уже не получает. Открытие того
// же владельца посреди закрытия продолжает с текущей доли. Владелец, ушедший
// из дерева, уносит оверлей сразу.
//
// Хостовые попапы (SetPopupSink) переходов не получают: окна ОС без альфы
// (Win32) показали бы проявление из чёрного; это задача 3.4.

import (
	"encoding/binary"
	"image"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// SetOverlayTransitions включает переходы появления и исчезновения
// оверлеев (меню, списков, палитр). По умолчанию выключены: кадр после
// открытия меню показывает его целиком, как прежде. Включённые без темы с
// ненулевым theme.AnimMenuOpen ничего не меняют. Уже открытые оверлеи
// появления заново не проигрывают.
func (e *Engine) SetOverlayTransitions(on bool) {
	if e.ovt.on.Swap(on) == on {
		return
	}
	if on {
		e.ovt.settleReq.Store(true)
		return
	}
	// Идущие переходы снимаются в ближайшем кадре (под frameMu), а кадр
	// перерисовывается целиком: на месте закрывающегося меню не должно
	// остаться его полупрозрачного следа.
	e.ovt.dropReq.Store(true)
	e.Invalidate()
}

// OverlayTransitions сообщает, включены ли переходы оверлеев.
func (e *Engine) OverlayTransitions() bool { return e.ovt.on.Load() }

// ovtState — фаза оверлея.
type ovtState uint8

const (
	ovtStatic  ovtState = iota // открыт, в покое (рисуется DrawOverlay)
	ovtPending                 // появился, но его место не перерисовывалось — начнём в следующем кадре
	ovtOpening                 // появляется
	ovtClosing                 // исчезает (владелец уже погасил его)
)

// overlayTrans — один оверлей под присмотром движка. Поля без атомиков
// читаются и пишутся только под frameMu.
type overlayTrans struct {
	key   uintptr       // widgetID владельца
	layer widget.Widget // слой: корень или модальное окно
	state ovtState
	seen  uint64

	rest  image.Rectangle // место в покое с запасом под тень: физические, по холсту
	slide image.Point     // сдвиг при доле 0, физические

	under, with []byte   // снимки rest построчно, rest.Dx()*4 на строку
	mask        []bool   // пиксели, которых оверлей коснулся
	runs        []ovtRun // mask прогонами: строка → отрезки [x0, x1)
	rowRuns     []int32  // начало прогонов строки в runs; len = высота+1
	have        bool     // снимки есть
	step        bool     // в этом кадре — шаг без перерисовки дерева
	doneSeen    bool     // переход дошёл до цели к началу кадра
	finished    bool     // исчезновение закончено: убрать
	job         compositeJob

	// Пишет тик анимации (горутина цикла), читает кадр.
	v      atomic.Uint64 // доля после кривой, float64: 0 — не виден, 1 — покой
	ticked atomic.Bool
	done   atomic.Bool // переход дошёл до цели
	direct atomic.Bool // шаг можно сделать без перерисовки дерева

	mu    sync.Mutex
	restL image.Rectangle // rest в логических — для InvalidateRect из тика
	cover image.Rectangle // логические, без запаса: то, что закрывает видео
	anim  *widget.Animation
}

func (tr *overlayTrans) val() float64     { return math.Float64frombits(tr.v.Load()) }
func (tr *overlayTrans) setVal(v float64) { tr.v.Store(math.Float64bits(v)) }

func (tr *overlayTrans) logicalRest() image.Rectangle {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.restL
}

// overlayTransitions — состояние переходов движка.
type overlayTransitions struct {
	on        atomic.Bool
	dropReq   atomic.Bool
	settleReq atomic.Bool

	// Дальше — под frameMu.
	list      []*overlayTrans
	canvas    *Canvas
	format    PixelFormat
	gen       uint64
	settle    bool            // кадр начинает присмотр: открытые уже оверлеи — в покой
	redrawAll bool            // кадр рисует весь холст
	redraw    image.Rectangle // иначе — эту область (физические)
	top       widget.Widget   // верхний слой кадра
	tt        image.Rectangle // подсказка (физические)
	others    []ovtOther      // оверлеи кадра вне присмотра (физические, с запасом)
	byKey     map[uintptr]*overlayTrans
}

// ovtOther — оверлей, нарисованный в обход присмотра (анимирует себя сам,
// без границ): переходу, который он накрывает, шагать без перерисовки нельзя.
type ovtOther struct {
	layer widget.Widget
	rect  image.Rectangle
}

// ovtSpec — длительность, кривая и сдвиг перехода по теме движка, с учётом
// профиля движения вызывающей горутины (горутины кадра).
func (e *Engine) ovtSpec(canvas *Canvas) (time.Duration, widget.Easing, int) {
	m := e.themeMgr.Load()
	if m == nil {
		return 0, nil, 0
	}
	a := m.GetAnimation(theme.AnimMenuOpen)
	if a.Duration <= 0 {
		return 0, nil, 0
	}
	off := canvas.PhysX(int(math.Round(m.GetMetric(theme.KeyMenuOpenOffset))))
	return a.Duration, widget.EasingByName(a.Curve), off
}

// ovtCloseSpec — длительность и кривая исчезновения: theme.AnimMenuClose, а
// если профиль его не объявил — theme.AnimMenuOpen.
func (e *Engine) ovtCloseSpec() (time.Duration, widget.Easing) {
	m := e.themeMgr.Load()
	if m == nil {
		return 0, nil
	}
	a := m.GetAnimationFirst(theme.AnimMenuClose, theme.AnimMenuOpen)
	if a.Duration <= 0 {
		return 0, nil
	}
	return a.Duration, widget.EasingByName(a.Curve)
}

// ovtLayers — слои кадра: корень и открытые модальные окна, снизу вверх.
func ovtLayers(root widget.Widget, modals []widget.ModalWidget) []widget.Widget {
	layers := make([]widget.Widget, 0, 1+len(modals))
	if root != nil {
		layers = append(layers, root)
	}
	for _, m := range modals {
		if m.IsModal() {
			layers = append(layers, m)
		}
	}
	return layers
}

// ovtPrepare — до решения о частичной перерисовке (под frameMu).
//
// Область перехода, которую повреждение задевает хоть краем (или лежащая
// внутри объединения повреждений, которое кадр перерисует), становится
// отдельной областью повреждения: снимки снимаются только с полностью
// перерисованной области, и её дифф обязателен — смешение меняет пиксели.
// Повреждение заодно расширяется на размытые подложки (expandForBackdrops),
// пока оба расширения не перестанут что-то добавлять. Возвращает
// повреждение и области шагов без перерисовки дерева.
func (e *Engine) ovtPrepare(canvas *Canvas, rects []image.Rectangle, all bool, layers []widget.Widget, on bool) ([]image.Rectangle, []image.Rectangle) {
	t := &e.ovt
	t.settle = t.settleReq.Swap(false)
	if len(t.list) == 0 {
		t.canvas, t.format = canvas, canvas.Format()
		t.dropReq.Store(false)
		return rects, nil
	}
	if t.dropReq.Swap(false) || !on || t.canvas != canvas || t.format != canvas.Format() {
		// Переходы выключены, холст сменился (разрешение, масштаб) или
		// сменился формат буфера: снимки недействительны.
		for _, tr := range t.list {
			e.ovtStop(tr)
			if !all {
				rects = append(rects, tr.rest)
			}
		}
		t.list = t.list[:0]
		t.canvas, t.format = canvas, canvas.Format()
		t.settle = true
		return rects, nil
	}
	// Слой исчез (модальное окно закрыто, корень заменён) — его оверлеи
	// уходят сразу.
	kept := t.list[:0]
	for _, tr := range t.list {
		if !containsLayer(layers, tr.layer) {
			e.ovtStop(tr)
			if !all {
				rects = append(rects, tr.rest)
			}
			continue
		}
		kept = append(kept, tr)
	}
	clear(t.list[len(kept):])
	t.list = kept

	e.ttMu.Lock()
	t.tt = canvas.PhysRect(e.ttShownAt)
	e.ttMu.Unlock()

	for _, tr := range t.list {
		tr.doneSeen = tr.done.Load()
	}

	if !all {
		// Добавляется только то, что ещё не накрыто одной из областей:
		// кандидатов конечное число (места переходов и подложки), и цикл
		// сходится.
		for grew := true; grew; {
			grew = false
			u := unionRects(rects)
			for _, tr := range t.list {
				if tr.rest.Empty() || coveredByOne(rects, tr.rest) {
					continue
				}
				if tr.rest.Overlaps(u) || tr.rest.Overlaps(t.tt) {
					rects = append(rects, tr.rest)
					u = u.Union(tr.rest)
					grew = true
				}
			}
			// Полная ёмкость: расширение дописывает в свою копию, а не в rects.
			for _, r := range canvas.ExpandForBackdrops(rects[:len(rects):len(rects)]) {
				if !r.Empty() && !coveredByOne(rects, r) {
					rects = append(rects, r)
					grew = true
				}
			}
		}
	}
	var steps []image.Rectangle
	u := unionRects(rects)
	for _, tr := range t.list {
		tr.step = false
		if !tr.ticked.Swap(false) {
			continue
		}
		if all || tr.rest.Overlaps(u) {
			continue // область перерисовывается — смешение в обходе
		}
		tr.step = true
		steps = append(steps, tr.rest)
	}
	return rects, steps
}

// coveredByOne — лежит ли r целиком в одной из областей.
func coveredByOne(rects []image.Rectangle, r image.Rectangle) bool {
	for _, q := range rects {
		if r.In(q) {
			return true
		}
	}
	return false
}

func containsLayer(layers []widget.Widget, w widget.Widget) bool {
	for _, l := range layers {
		if l == w {
			return true
		}
	}
	return false
}

// ovtBeginFrame запоминает, что кадр перерисовывает.
func (e *Engine) ovtBeginFrame(all bool, redraw image.Rectangle, layers []widget.Widget) {
	t := &e.ovt
	t.gen++
	t.redrawAll, t.redraw = all, redraw
	t.top = nil
	if len(layers) > 0 {
		t.top = layers[len(layers)-1]
	}
	t.others = t.others[:0]
}

// ovtRedrawn — перерисована ли область кадром целиком.
func (e *Engine) ovtRedrawn(r image.Rectangle) bool {
	if r.Empty() {
		return false
	}
	return e.ovt.redrawAll || r.In(e.ovt.redraw)
}

// ovtStepOnly — кадр без перерисовки: только шаги переходов.
func (e *Engine) ovtStepOnly(canvas *Canvas) {
	for _, tr := range e.ovt.list {
		if tr.step && tr.have {
			tr.composite(canvas, tr.val())
		}
	}
}

// drawOverlayLayer рисует оверлеи слоя layer. Без переходов в кадре —
// прежний drawOverlays; с ними — тот же обход, но оверлеи под присмотром
// идут через ovtVisit, а погашенные — через ovtVisitClosed.
func (e *Engine) drawOverlayLayer(layer widget.Widget, canvas *Canvas, hosted, on bool) {
	if !on {
		if drawOverlays(layer, canvas, hosted) {
			e.overlayDrawn = true
		}
		return
	}
	t := &e.ovt
	if t.byKey == nil {
		t.byKey = make(map[uintptr]*overlayTrans)
	}
	clear(t.byKey)
	for _, tr := range t.list {
		if tr.layer == layer {
			t.byKey[tr.key] = tr
		}
	}
	e.drawOverlaysT(layer, canvas, hosted, layer, image.Point{}, 0)
	e.ovtLayerEnd(layer)
}

// drawOverlaysT — drawOverlaysAt с присмотром за переходами.
func (e *Engine) drawOverlaysT(w widget.Widget, canvas *Canvas, hosted bool, layer widget.Widget, off image.Point, depth int) {
	if tooDeep(depth) {
		return
	}
	if od, ok := w.(widget.OverlayDrawer); ok {
		switch {
		case od.HasOverlay():
			if !hosted || !isHostedOverlay(w) {
				e.overlayDrawn = true
				e.ovtVisit(w, od, canvas, layer, off)
			}
		case len(e.ovt.byKey) > 0:
			if tr := e.ovt.byKey[widgetID(w)]; tr != nil {
				e.overlayDrawn = true
				e.ovtVisitClosed(tr, canvas)
			}
		}
	}
	children := w.Children()
	if len(children) == 0 {
		return
	}
	coff := off.Add(contentShift(w))
	for _, child := range children {
		e.drawOverlaysT(child, canvas, hosted, layer, coff, depth+1)
	}
}

// ovtDrawOther рисует оверлей мимо присмотра и запоминает его место.
func (e *Engine) ovtDrawOther(w widget.Widget, od widget.OverlayDrawer, canvas *Canvas, layer widget.Widget, off image.Point) {
	widget.DrawOverlayTraced(w, od, widget.OffsetContext(canvas, off.X, off.Y))
	r := overlayHitRect(w).Sub(off).Inset(-widget.DefaultOverlayMargin())
	e.ovt.others = append(e.ovt.others, ovtOther{layer: layer, rect: canvas.PhysRect(r)})
}

// ovtVisit — открытый оверлей в обходе слоя.
func (e *Engine) ovtVisit(w widget.Widget, od widget.OverlayDrawer, canvas *Canvas, layer widget.Widget, off image.Point) {
	draw := func() { widget.DrawOverlayTraced(w, od, widget.OffsetContext(canvas, off.X, off.Y)) }
	ob, ok := w.(widget.OverlayBoundsProvider)
	key := widgetID(w)
	if !ok || key == 0 {
		e.ovtDrawOther(w, od, canvas, layer, off)
		return
	}
	if sa, ok := w.(widget.SelfAnimatedOverlay); ok && sa.AnimatesOverlay() {
		e.ovtDrawOther(w, od, canvas, layer, off)
		return
	}
	bounds := ob.OverlayBounds()
	if bounds.Empty() {
		e.ovtDrawOther(w, od, canvas, layer, off)
		return
	}
	screen := bounds.Sub(off)
	margin := widget.DefaultOverlayMargin()
	if om, ok := w.(widget.OverlayMarginer); ok {
		margin = om.OverlayMargin()
	}
	restL := screen.Inset(-margin)
	rest := canvas.PhysRect(restL).Intersect(image.Rect(0, 0, canvas.W, canvas.H))
	if rest.Empty() {
		draw()
		return
	}
	redrawn := e.ovtRedrawn(rest)

	tr := e.ovt.byKey[key]
	if tr == nil {
		tr = &overlayTrans{key: key, layer: layer, state: ovtPending}
		if e.ovt.settle {
			// Присмотр начался при открытом оверлее (переходы только что
			// включены, холст сменился): он уже на экране.
			tr.state = ovtStatic
			tr.setVal(1)
		}
		e.ovt.list = append(e.ovt.list, tr)
		e.ovt.byKey[key] = tr
	}
	tr.seen = e.ovt.gen
	if tr.rest != rest {
		if tr.state == ovtOpening || tr.state == ovtClosing {
			// Смешение лежало на прежнем месте — оно перерисуется.
			e.damageRect(tr.logicalRest())
		}
		tr.setGeom(rest, restL, overlayHitRect(w).Sub(off))
	}
	dur, curve, slide := e.ovtSpec(canvas)
	capture := func() {
		tr.under = snapRect(canvas.Back(), rest, tr.under)
		draw()
		tr.with = snapRect(canvas.Back(), rest, tr.with)
		tr.mask = buildMask(tr.under, tr.with, tr.mask)
		// Непрозрачные части едут целиком, даже пиксель, совпавший с фоном.
		if orp, ok := w.(widget.OverlayRectsProvider); ok {
			markOpaque(tr.mask, rest, orp.OverlayRects(), off, canvas)
		} else {
			markOpaque(tr.mask, rest, []image.Rectangle{bounds}, off, canvas)
		}
		tr.runs, tr.rowRuns = buildRuns(tr.mask, rest.Dx(), rest.Dy(), tr.runs, tr.rowRuns)
		tr.have = true
	}
	settle := func() {
		e.ovtStop(tr)
		tr.state = ovtStatic
		tr.setVal(1)
		tr.done.Store(false)
		tr.doneSeen = false
	}

	switch tr.state {
	case ovtPending:
		if dur <= 0 {
			// Без перехода — как прежде; снимки пригодятся, если закрытие
			// пойдёт уже с переходом (профиль сменили).
			if redrawn {
				capture()
			} else {
				draw()
			}
			settle()
			return
		}
		if !redrawn {
			// Место не перерисовывалось: снимок «под» взять неоткуда. Не
			// рисуем — переход начнётся со следующего кадра, а показ целиком
			// с последующим проявлением с нуля выглядел бы миганием.
			e.damageRect(restL)
			return
		}
		tr.slide = ovtSlideDir(w, bounds, slide)
		capture()
		tr.state = ovtOpening
		tr.setVal(0)
		e.ovtAnimate(tr, 0, 1, dur, curve)
		tr.composite(canvas, 0)

	case ovtClosing:
		// Владелец открыл оверлей снова — переход разворачивается с текущей
		// доли. В этом кадре исчезающий не смешивался: владелец открыт, и
		// буфер на его месте — дерево и то, что под ним.
		if !redrawn {
			e.ovtStop(tr)
			tr.state, tr.have = ovtPending, false
			e.damageRect(restL)
			return
		}
		from := tr.val()
		capture()
		if dur <= 0 {
			settle()
			return
		}
		tr.slide = ovtSlideDir(w, bounds, slide)
		tr.state = ovtOpening
		e.ovtAnimate(tr, from, 1, dur, curve)
		tr.composite(canvas, from)

	case ovtOpening:
		if !tr.have && !redrawn {
			// Сдвинулся, не заявив места: показываем как есть, без перехода.
			draw()
			settle()
			e.damageRect(restL)
			return
		}
		if redrawn {
			capture()
			if tr.doneSeen {
				settle() // конец: в буфере прямая отрисовка
				return
			}
			tr.composite(canvas, tr.val())
			return
		}
		if tr.step {
			tr.composite(canvas, tr.val())
		}

	case ovtStatic:
		if redrawn {
			capture()
		} else if !tr.have {
			draw() // сдвинулся, не заявив места: как прежде
			e.damageRect(restL)
		}
	}
}

// ovtVisitClosed — владелец, погасивший оверлей, в обходе слоя: здесь, на
// его месте в порядке рисования, оверлей исчезает.
func (e *Engine) ovtVisitClosed(tr *overlayTrans, canvas *Canvas) {
	tr.seen = e.ovt.gen
	redrawn := e.ovtRedrawn(tr.rest)
	if tr.state != ovtClosing {
		dur, curve := e.ovtCloseSpec()
		from := tr.val()
		if tr.state == ovtPending || !tr.have || dur <= 0 || from <= 0 {
			e.ovtStop(tr)
			tr.finished = true
			return
		}
		if redrawn {
			tr.under = snapRect(canvas.Back(), tr.rest, tr.under)
		}
		tr.state = ovtClosing
		e.ovtAnimate(tr, from, 0, dur, curve)
		tr.composite(canvas, from)
		return
	}
	if tr.doneSeen {
		if redrawn {
			tr.finished = true // в буфере — дерево без оверлея
		}
		return
	}
	if redrawn {
		tr.under = snapRect(canvas.Back(), tr.rest, tr.under)
		tr.composite(canvas, tr.val())
		return
	}
	if tr.step {
		tr.composite(canvas, tr.val())
	}
}

// ovtLayerEnd — оверлеи слоя, чей владелец не встретился в обходе (ушёл из
// дерева): уходят сразу.
func (e *Engine) ovtLayerEnd(layer widget.Widget) {
	for _, tr := range e.ovt.list {
		if tr.layer != layer || tr.seen == e.ovt.gen {
			continue
		}
		e.ovtStop(tr)
		tr.finished = true
		if tr.state == ovtOpening || tr.state == ovtClosing {
			e.damageRect(tr.logicalRest())
		}
	}
}

// ovtEndFrame — после всех слоёв: убирает законченные переходы и решает,
// какие шаги пойдут без перерисовки дерева.
func (e *Engine) ovtEndFrame() {
	t := &e.ovt
	kept := t.list[:0]
	for _, tr := range t.list {
		if tr.finished {
			continue
		}
		kept = append(kept, tr)
	}
	clear(t.list[len(kept):])
	t.list = kept

	for _, tr := range t.list {
		direct := tr.layer == t.top && !tr.rest.Overlaps(t.tt)
		for _, o := range t.list {
			if !direct {
				break
			}
			if o != tr && o.rest.Overlaps(tr.rest) {
				direct = false
			}
		}
		for _, o := range t.others {
			if !direct {
				break
			}
			if o.rect.Overlaps(tr.rest) {
				direct = false
			}
		}
		tr.direct.Store(direct)
	}
}

// ovtAnimate ведёт долю от from к to за часть dur, пропорциональную пути.
//
// Кривая применяется в тике к линейному прогрессу: конец перехода — это
// прогресс 1, а не значение кривой (у кривой с выбросом оно бывает равно 1 и
// посреди пути). Последний тик заявляет область повреждением: конец
// рисуется обычным путём.
func (e *Engine) ovtAnimate(tr *overlayTrans, from, to float64, dur time.Duration, curve widget.Easing) {
	e.ovtStop(tr)
	tr.done.Store(false)
	tr.doneSeen = false
	d := time.Duration(float64(dur) * math.Abs(to-from))
	restL := tr.logicalRest()
	finish := func() {
		tr.setVal(to)
		tr.done.Store(true) // до заявки: кадр, забравший её, увидит и конец
		e.damageRect(restL)
	}
	if d <= 0 {
		finish()
		return
	}
	a := widget.AnimateOwned(tr, "ovt", d, nil, func(p float64) {
		if p >= 1 {
			finish()
			return
		}
		k := p
		if curve != nil {
			k = curve(p)
		}
		tr.setVal(from + (to-from)*k)
		tr.ticked.Store(true)
		e.ovtWake(tr)
	})
	tr.mu.Lock()
	tr.anim = a
	tr.mu.Unlock()
}

// ovtStop снимает анимацию перехода.
func (e *Engine) ovtStop(tr *overlayTrans) {
	tr.mu.Lock()
	a := tr.anim
	tr.anim = nil
	tr.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// ovtWake просит кадр под шаг: без повреждения, если шаг можно сделать
// смешением снимков, иначе — повреждением области (кадр нарисует слои).
func (e *Engine) ovtWake(tr *overlayTrans) {
	if tr.direct.Load() {
		e.invGen.Add(1)
		return
	}
	e.damageRect(tr.logicalRest())
}

// setGeom запоминает место оверлея; снимки прежнего места недействительны.
func (tr *overlayTrans) setGeom(rest, restL, cover image.Rectangle) {
	tr.rest = rest
	tr.have = false
	tr.mu.Lock()
	tr.restL, tr.cover = restL, cover
	tr.mu.Unlock()
}

// ovtCovers добавляет к dst места закрывающихся оверлеев: они ещё видны, и
// видео под ними не должно ложиться поверх (video.go).
func (e *Engine) ovtCovers(dst []image.Rectangle) []image.Rectangle {
	for _, tr := range e.ovt.list {
		if tr.state != ovtClosing || !tr.have {
			continue
		}
		tr.mu.Lock()
		c := tr.cover
		tr.mu.Unlock()
		if !c.Empty() {
			dst = append(dst, c)
		}
	}
	return dst
}

// ovtSlideDir — сдвиг в начале перехода: оверлей выше точки привязки
// (OverlayAnchorer) или верха владельца выезжает снизу вверх, остальные —
// сверху вниз. overlay — в кадре владельца.
func ovtSlideDir(w widget.Widget, overlay image.Rectangle, d int) image.Point {
	if d <= 0 {
		return image.Point{}
	}
	top := w.Bounds().Min.Y
	if a, ok := w.(widget.OverlayAnchorer); ok {
		top = a.OverlayAnchor().Y
	}
	if overlay.Min.Y < top {
		return image.Pt(0, d)
	}
	return image.Pt(0, -d)
}

// snapRect копирует область r буфера в buf (r.Dx()*4 на строку).
func snapRect(back *image.RGBA, r image.Rectangle, buf []byte) []byte {
	row := r.Dx() * 4
	n := row * r.Dy()
	if cap(buf) < n {
		buf = make([]byte, n)
	}
	buf = buf[:n]
	for y := 0; y < r.Dy(); y++ {
		so := back.PixOffset(r.Min.X, r.Min.Y+y)
		copy(buf[y*row:(y+1)*row], back.Pix[so:so+row])
	}
	return buf
}

// buildMask отмечает пиксели, где «с» отличается от «под».
func buildMask(under, with []byte, mask []bool) []bool {
	n := len(under) / 4
	if cap(mask) < n {
		mask = make([]bool, n)
	}
	mask = mask[:n]
	for i := 0; i < n; i++ {
		j := i * 4
		mask[i] = binary.LittleEndian.Uint32(under[j:j+4]) != binary.LittleEndian.Uint32(with[j:j+4])
	}
	return mask
}

// markOpaque отмечает в маске непрозрачные части оверлея (в кадре владельца)
// целиком: пиксель меню, случайно совпавший с фоном, тоже часть меню и едет
// с ним.
func markOpaque(mask []bool, rest image.Rectangle, rects []image.Rectangle, off image.Point, c *Canvas) {
	w := rest.Dx()
	for _, r := range rects {
		p := c.PhysRect(r.Sub(off)).Intersect(rest)
		for y := p.Min.Y; y < p.Max.Y; y++ {
			row := mask[(y-rest.Min.Y)*w:]
			for x := p.Min.X; x < p.Max.X; x++ {
				row[x-rest.Min.X] = true
			}
		}
	}
}

// ovtRun — отрезок строки [x0, x1), где оверлей коснулся пикселей.
type ovtRun struct{ x0, x1 int32 }

// buildRuns сводит маску в прогоны по строкам: смешение идёт только по ним,
// остальное — копия строки «под».
func buildRuns(mask []bool, w, h int, runs []ovtRun, rowRuns []int32) ([]ovtRun, []int32) {
	runs = runs[:0]
	if cap(rowRuns) < h+1 {
		rowRuns = make([]int32, h+1)
	}
	rowRuns = rowRuns[:h+1]
	for y := 0; y < h; y++ {
		rowRuns[y] = int32(len(runs))
		m := mask[y*w : (y+1)*w]
		for x := 0; x < w; {
			if !m[x] {
				x++
				continue
			}
			x0 := x
			for x < w && m[x] {
				x++
			}
			runs = append(runs, ovtRun{int32(x0), int32(x)})
		}
	}
	rowRuns[h] = int32(len(runs))
	return runs, rowRuns
}

// lerpPixel смешивает два пикселя (любой порядок каналов) с долей a/256:
// два канала за одно умножение. В каждой половине слова 16 бит, и сумма
// u·(256−a) + w·a ≤ 255·256 = 65280 в них помещается.
func lerpPixel(u, w, a uint32) uint32 {
	na := 256 - a
	rb := ((u&0x00ff00ff)*na + (w&0x00ff00ff)*a) >> 8 & 0x00ff00ff
	ag := ((u>>8&0x00ff00ff)*na + (w>>8&0x00ff00ff)*a) & 0xff00ff00
	return rb | ag
}

// compositeJob — смешение перехода по полосам строк (workpool.go): строки
// буфера не пересекаются, и полосы идут параллельно.
type compositeJob struct {
	tr     *overlayTrans
	back   *image.RGBA
	a256   uint32
	sx, sy int
	chunk  int
	batch  poolBatch
}

func (j *compositeJob) RunPart(i int) {
	h := j.tr.rest.Dy()
	j.tr.compositeRows(j.back, i*j.chunk, min((i+1)*j.chunk, h), j.a256, j.sx, j.sy)
}

// compositeParallelPx — с какой площади смешение идёт полосами: меньшую
// горутины пула не окупают.
const compositeParallelPx = 64 * 1024

// composite кладёт в холст кадр перехода с долей v: «под», поверх него «с»
// со сдвигом slide*(1-v) и непрозрачностью v. Доля бывает больше 1 (кривая с
// выбросом): непрозрачность ограничивается, сдвиг перелетает место покоя.
func (tr *overlayTrans) composite(c *Canvas, v float64) {
	r := tr.rest
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 || len(tr.under) < w*h*4 || len(tr.with) < w*h*4 || len(tr.rowRuns) != h+1 {
		return
	}
	a256 := uint32(math.Round(math.Max(0, math.Min(1, v)) * 256))
	sx := int(math.Round(float64(tr.slide.X) * (1 - v)))
	sy := int(math.Round(float64(tr.slide.Y) * (1 - v)))
	parts := 1
	if w*h >= compositeParallelPx {
		parts = min(runtime.GOMAXPROCS(0), h/16)
	}
	if parts <= 1 {
		tr.compositeRows(c.Back(), 0, h, a256, sx, sy)
	} else {
		j := &tr.job
		j.tr, j.back, j.a256, j.sx, j.sy = tr, c.Back(), a256, sx, sy
		j.chunk = (h + parts - 1) / parts
		poolRun(j, &j.batch, (h+j.chunk-1)/j.chunk)
		j.back = nil
	}
	// Смесь — картинка: ни сплошной заливкой, ни текстом её не назвать.
	// Конец перехода рисуется обычным путём, с настоящими признаками.
	c.MarkImage(r)
}

// compositeRows — composite для строк [y0, y1) области.
func (tr *overlayTrans) compositeRows(back *image.RGBA, y0, y1 int, a256 uint32, sx, sy int) {
	r := tr.rest
	w, h := r.Dx(), r.Dy()
	row := w * 4
	for y := y0; y < y1; y++ {
		do := back.PixOffset(r.Min.X, r.Min.Y+y)
		dst := back.Pix[do : do+row]
		urow := tr.under[y*row : (y+1)*row]
		copy(dst, urow)
		qy := y - sy
		if a256 == 0 || qy < 0 || qy >= h {
			continue
		}
		wrow := tr.with[qy*row : (qy+1)*row]
		for _, run := range tr.runs[tr.rowRuns[qy]:tr.rowRuns[qy+1]] {
			// Отрезок источника [x0, x1) ложится в [x0+sx, x1+sx).
			d0, d1 := max(int(run.x0)+sx, 0), min(int(run.x1)+sx, w)
			if d0 >= d1 {
				continue
			}
			if a256 == 256 {
				copy(dst[d0*4:d1*4], wrow[(d0-sx)*4:(d1-sx)*4])
				continue
			}
			for x := d0; x < d1; x++ {
				i, j := x*4, (x-sx)*4
				binary.LittleEndian.PutUint32(dst[i:i+4], lerpPixel(
					binary.LittleEndian.Uint32(urow[i:i+4]), binary.LittleEndian.Uint32(wrow[j:j+4]), a256))
			}
		}
	}
}
