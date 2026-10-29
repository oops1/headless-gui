package input

// frames.go — перевод координат на границе «экран ↔ содержимое прокрутки».
//
// Дети ScrollView рисуются со сдвигом (PERF-12), а их Bounds() остаются в
// координатах СОДЕРЖИМОГО — «кадре» ребёнка. Точка экрана p лежит в кадре
// ребёнка как p+off, где off — сумма widget.ContentOffsetter.ContentOffset() всех
// контейнеров над ним. Всё, что движок знает о мыши, — экранное; всё, что знает
// виджет, — в его кадре. Движок обязан переводить на каждой границе, и все эти
// границы собраны здесь и в events.go:
//
//   - хит-тест строит путь с кадром каждого шага (HitPath);
//   - событие мыши уходит виджету с координатами ЕГО кадра (deliverButton);
//   - обход «всех виджетов» (движение мыши, закрытие оверлеев, поиск оверлея и
//     захватчика) несёт кадр вместе с рекурсией;
//   - виджет, захвативший мышь, получает кадр по цепочке контейнеров над ним
//     (captureOffset): движок помнит не число, а цепочку, потому что прокрутка
//     может сдвинуться посреди перетаскивания;
//   - оверлеи рисуются через widget.OffsetContext, а их границы переводятся на
//     экран (engine.go, popuphost.go);
//   - всё, что виджет отдаёт наружу в своём кадре, — каретка IME, прямоугольник
//     для инвалидации, центр для синтетического клика скринридера, — переводится
//     обратно (frameOffsetOf).
//
// Пока ни один контейнер не сдвигает содержимое, все смещения нулевые и эти
// функции возвращают то же, что возвращали прежние: единственная цена — проверка
// интерфейса на виджетах пути.

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

// HitStep — шаг пути hit-теста: виджет и смещение его кадра. Off — сумма
// ContentOffset контейнеров НАД виджетом: точка экрана (x, y) в системе
// координат его Bounds() — (x+Off.X, y+Off.Y).
type HitStep struct {
	W   widget.Widget
	Off image.Point
}

// ContentShift — на сколько контейнер сдвигает своих детей; нулевой у всех
// остальных виджетов.
func ContentShift(w widget.Widget) image.Point {
	if oc, ok := w.(widget.ContentOffsetter); ok {
		return oc.ContentOffset()
	}
	return image.Point{}
}

// HitPath возвращает путь от корня до самого глубокого виджета под экранной
// точкой (x, y): [root, ..., parent, hit], у каждого шага — кадр виджета.
// Пустой срез (nil) — точка вне дерева.
//
// Точка переводится в кадр детей при спуске через каждый контейнер со сдвигом.
// Родитель проверяется по точке в СВОЁМ кадре, поэтому ребёнок прокрутки
// попадает под щелчок только внутри самой прокрутки — как он и виден.
func HitPath(w widget.Widget, x, y int) []HitStep {
	return appendHitPath(nil, w, x, y, image.Point{}, 0)
}

// appendHitPath дописывает путь [w, ..., hit] в dst; (x, y) — точка в кадре w,
// off — смещение этого кадра. Устройство накопителя — как у прежнего
// appendHitTestPath (PERF-14): один рост среза сверху вниз, nil возвращается
// только до append.
func appendHitPath(dst []HitStep, w widget.Widget, x, y int, off image.Point, depth int) []HitStep {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return nil
	}
	if !image.Pt(x, y).In(w.Bounds()) {
		return nil
	}
	n := len(dst)
	dst = append(dst, HitStep{W: w, Off: off})
	cx, cy, coff := x, y, off
	if sh := ContentShift(w); sh != (image.Point{}) {
		cx, cy, coff = x+sh.X, y+sh.Y, off.Add(sh)
	}
	// Проверяем детей в обратном Z-порядке
	children := w.Children()
	for i := len(children) - 1; i >= 0; i-- {
		if path := appendHitPath(dst, children[i], cx, cy, coff, depth+1); path != nil {
			return path
		}
	}
	return dst[:n+1]
}

// hitTestPath — путь hit-теста одними виджетами, без кадров. Для того, кому
// нужен только состав пути; движок сам ходит по HitPath.
func hitTestPath(w widget.Widget, x, y int) []widget.Widget {
	steps := HitPath(w, x, y)
	if steps == nil {
		return nil
	}
	out := make([]widget.Widget, len(steps))
	for i, st := range steps {
		out[i] = st.W
	}
	return out
}

// pathSet — множество виджетов пути (для DismissOutside).
func pathSet(path []HitStep) map[widget.Widget]struct{} {
	set := make(map[widget.Widget]struct{}, len(path))
	for _, st := range path {
		set[st.W] = struct{}{}
	}
	return set
}

// deliverButton отдаёт нажатие виджету с координатами его кадра off. На время
// обработчика объявляет кадр (widget.SetEventFrame): меню, открытое им, должно
// знать, где в его системе координат край экрана.
func deliverButton(mc widget.MouseClickHandler, ev widget.MouseEvent, off image.Point) bool {
	if off == (image.Point{}) {
		return mc.OnMouseButton(ev)
	}
	prev := widget.SetEventFrame(off)
	ev.X += off.X
	ev.Y += off.Y
	ok := mc.OnMouseButton(ev)
	widget.SetEventFrame(prev)
	return ok
}

// deliverMove — то же для движения мыши.
func deliverMove(mm widget.MouseMoveHandler, x, y int, off image.Point) {
	if off == (image.Point{}) {
		mm.OnMouseMove(x, y)
		return
	}
	prev := widget.SetEventFrame(off)
	mm.OnMouseMove(x+off.X, y+off.Y)
	widget.SetEventFrame(prev)
}

// deliverMoveFramed — движение мыши, уже переведённое в кадр off (обход
// broadcastMouseMove носит точки в кадре текущего виджета).
func deliverMoveFramed(mm widget.MouseMoveHandler, x, y int, off image.Point) {
	if off == (image.Point{}) {
		mm.OnMouseMove(x, y)
		return
	}
	prev := widget.SetEventFrame(off)
	mm.OnMouseMove(x, y)
	widget.SetEventFrame(prev)
}

// showMenuInFrame открывает меню в точке (x, y) его кадра off. Кадр объявлен на
// время Show: меню прижимается к краям холста, а холст — экранный.
func showMenuInFrame(pm *widget.PopupMenu, x, y int, off image.Point) {
	if off == (image.Point{}) {
		pm.Show(x, y)
		return
	}
	prev := widget.SetEventFrame(off)
	pm.Show(x, y)
	widget.SetEventFrame(prev)
}

// ─── Кадр виджета вне пути hit-теста ─────────────────────────────────────────

// frameOffsetIn ищет target в поддереве root и возвращает смещение его кадра:
// сумму ContentOffset контейнеров над ним. off — смещение кадра самого root.
func frameOffsetIn(root, target widget.Widget, off image.Point, depth int) (image.Point, bool) {
	if tooDeep(depth) {
		return image.Point{}, false
	}
	if root == target {
		return off, true
	}
	coff := off.Add(ContentShift(root))
	for _, c := range root.Children() {
		if p, ok := frameOffsetIn(c, target, coff, depth+1); ok {
			return p, true
		}
	}
	return image.Point{}, false
}

// treeRoots — корни, в которых может лежать виджет: дерево и открытые модалки.
func (d *Dispatcher) treeRoots() []widget.Widget {
	root := d.host.Root()
	roots := make([]widget.Widget, 0, 2)
	if root != nil {
		roots = append(roots, root)
	}
	for _, m := range d.host.Modals() {
		roots = append(roots, m)
	}
	return roots
}

// frameOffsetOf возвращает смещение кадра виджета w. Нулевое, если w в дереве
// не нашёлся (виджет вне дерева — ушедший в своё окно попап, содержимое
// неактивной вкладки): тогда его координаты считаются экранными, как и до
// появления кадров.
//
// Обходит дерево, поэтому годится для редких событий — смена фокуса, каретка
// IME, синтетический клик скринридера, — а не для каждого движения мыши.
func (d *Dispatcher) frameOffsetOf(w widget.Widget) image.Point {
	if w == nil {
		return image.Point{}
	}
	for _, r := range d.treeRoots() {
		if off, ok := frameOffsetIn(r, w, image.Point{}, 0); ok {
			return off
		}
	}
	return image.Point{}
}

// screenBoundsOf — границы виджета в экранных координатах (для инвалидации).
func (d *Dispatcher) screenBoundsOf(w widget.Widget) image.Rectangle {
	return w.Bounds().Sub(d.frameOffsetOf(w))
}

// ─── Захват мыши ─────────────────────────────────────────────────────────────

// offsetChainIn ищет target в поддереве root и возвращает контейнеры со сдвигом
// над ним (сверху вниз, без самого target).
func offsetChainIn(root, target widget.Widget, chain []widget.ContentOffsetter, depth int) ([]widget.ContentOffsetter, bool) {
	if tooDeep(depth) {
		return nil, false
	}
	if root == target {
		return chain, true
	}
	next := chain
	if oc, ok := root.(widget.ContentOffsetter); ok {
		// Срез с ограниченной ёмкостью: append копирует, и соседние ветки
		// обхода не затирают цепочку друг друга.
		next = append(chain[:len(chain):len(chain)], oc)
	}
	for _, c := range root.Children() {
		if got, ok := offsetChainIn(c, target, next, depth+1); ok {
			return got, true
		}
	}
	return nil, false
}

// captureOffset возвращает смещение кадра захватившего мышь виджета cap.
//
// Цепочку контейнеров над ним движок находит один раз (обход дерева) и держит до
// конца захвата, а само смещение берёт у них при каждом событии: прокрутка
// может сдвинуться посреди перетаскивания — колесом, инерцией, самим
// перетаскиванием ползунка, — и запомненное число стало бы ложью.
//
// Захват выставляют и виджеты сами (через CaptureManager.SetCapture) прямо в
// своём обработчике, где обходить дерево нельзя — они могут держать свои замки;
// поэтому SetCapture цепочку только сбрасывает, а ищется она здесь, при
// следующем событии.
func (d *Dispatcher) captureOffset(cap widget.Widget) image.Point {
	d.capMu.Lock()
	if d.captured != cap {
		d.capMu.Unlock()
		return image.Point{}
	}
	ok, chain := d.capChainOK, d.capChain
	d.capMu.Unlock()
	if !ok {
		for _, r := range d.treeRoots() {
			if c, found := offsetChainIn(r, cap, nil, 0); found {
				chain = c
				break
			}
		}
		d.capMu.Lock()
		if d.captured == cap {
			d.capChain, d.capChainOK = chain, true
		}
		d.capMu.Unlock()
	}
	var off image.Point
	for _, oc := range chain {
		off = off.Add(oc.ContentOffset())
	}
	return off
}
