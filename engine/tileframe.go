package engine

// tileframe.go — тайловая растеризация списка команд в пуле воркеров
// (ADR-0005, задача 4а.2).
//
// Кадр с путём команд (cmdpath.go) делится на тайлы 64×64 физических
// пикселей — те же, что у диффа. Воркер берёт холст-окно (core/raster/
// window.go): буфер размером с тайл в координатах кадра, префилл из back,
// базовый клип повреждения, воспроизведение дерева с отсечкой по тайлу,
// копирование строк обратно в back и перенос признака тайла. Дальше кадр
// идёт как прежде: оверлеи, дифф back/front, регионы.
//
// Размытая подложка (BlurBehind) читает кадр за пределами тайла. По решению
// владельца — кэш с ореолом на промахе: слой, чьё содержимое под ним не
// менялось (ключ — хэши команд под областью чтения), подменяется блитом
// готового растра; слой-промах рисуется в окнах с запасом по периметру
// (ореол), откуда размытие читает подложку, а растр кэша собирается из
// центров ореолов по ходу кадра. Снимок back под ореолами снимает горутина
// кадра до раздачи: воркеры соседних тайлов пишут в back параллельно.

import (
	"image"
	"image/color"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	hgraster "github.com/oops1/headless-gui/v3/core/raster"
	"github.com/oops1/headless-gui/v3/internal/workpool"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

const (
	// tiledMinTiles — с какого числа тайлов повреждения кадр идёт по тайлам;
	// меньше — прямое воспроизведение в холст движка (раздача пула, префилл и
	// копирование на паре тайлов дороже самой работы).
	tiledMinTiles = 5
	// tiledBigImage — физическая площадь приёмника картинки, с которой она
	// масштабируется на горутине кадра, а не в каждом воркере.
	tiledBigImage = 4 * output.TileSize * output.TileSize
	// blurCacheBudget — байт растров кэша размытой подложки на движок; слой,
	// не влезающий в бюджет, не кэшируется (идёт ореолом).
	blurCacheBudget = 16 << 20
	// blurCacheTTL — через сколько кадров без использования запись удаляется.
	blurCacheTTL = 60
)

// SetTiledRaster включает или выключает тайловый растр пути команд. По
// умолчанию включён (у движка NewOffscreen — выключен: одиночные кадры
// печати). Выключенный — воспроизведение списка в холст движка одной
// горутиной, как в 4а.1. Картинка на обоих путях одна и та же.
func (e *Engine) SetTiledRaster(on bool) {
	e.tiled.Store(on)
	e.Invalidate()
}

// TiledRaster сообщает, включён ли тайловый растр.
func (e *Engine) TiledRaster() bool { return e.tiled.Load() }

// directPanic — паника прямого виджета (DirectDrawer) в воркере: виновник
// известен из команды, стек снят на воркере (учёт рисующихся по goroutine
// id в горутине кадра его не нашёл бы).
type directPanic struct {
	w     widget.Widget
	value any
	stack []byte
}

// tileWorker — воркер тайлового растра: холст-окно и то, что нужно на тайл
// без аллокаций.
type tileWorker struct {
	c    *hgraster.Canvas
	rep  hgdraw.Replayer
	opts hgdraw.ReplayOpts
	dmg  [1]image.Rectangle
	// Текущий тайл: для Tap и Override.
	tr   *tileRaster
	tile image.Rectangle
	vis  image.Rectangle // окно ∩ повреждение — что тайл на самом деле нарисовал
	halo bool
}

// blurLayer — слой размытой подложки текущего кадра.
type blurLayer struct {
	cmd       *hgdraw.Cmd
	rectL     image.Rectangle // логическая область слоя
	pr, src   image.Rectangle // физические: слой с клипом, область чтения
	pradius   int
	tint      color.RGBA
	inGroup   bool // внутри группы прозрачности — только ореол
	cacheable bool
	id, key   uint64
	hit       *blurEntry  // попадание: блит этого растра
	raster    *image.RGBA // промах: растр собирается из центров ореолов
	covered   atomic.Int64
	imgs      []image.Image
}

// blurEntry — запись кэша размытия.
type blurEntry struct {
	key    uint64
	raster *image.RGBA
	imgs   []image.Image // удерживают адреса картинок ключа
	last   uint64
}

type scaledKey struct {
	ptr    uintptr
	pw, ph int
}

// tileEnv — окружение холста, от которого зависят растры кэша размытия:
// смена любого из полей делает их устаревшими.
type tileEnv struct {
	canvas  *Canvas
	format  PixelFormat
	scale   float64
	fontRev uint64
	bgGen   uint64
}

// tileRaster — состояние тайлового растра движка между кадрами и на кадр.
type tileRaster struct {
	e       *Engine
	workers chan *tileWorker
	made    int
	batch   workpool.Batch
	cache   map[uint64]*blurEntry
	bytes   int
	frameNo uint64
	env     tileEnv
	snapBuf []byte
	// directMu — прямые виджеты рисуются по одному: их Draw не обязан быть
	// готов к вызову из нескольких воркеров сразу (виджет, лежащий на
	// нескольких тайлах, иначе рисовался бы параллельно).
	directMu sync.Mutex

	// На кадр.
	canvas   *Canvas
	root     *hgdraw.List
	tiles    []int
	haloH    []int
	partial  bool
	damage   image.Rectangle // физическое объединённое повреждение
	layers   []*blurLayer
	snapshot *image.RGBA
	scaled   map[scaledKey]*image.RGBA
	hits     int
	misses   int
}

// tiledExitList — пустой список для приведения клипа холста к клипу выхода.
var tiledExitList = &hgdraw.List{}

func (e *Engine) tileRaster() *tileRaster {
	if e.tr == nil {
		n := runtime.GOMAXPROCS(0) + 1
		e.tr = &tileRaster{e: e, workers: make(chan *tileWorker, n), cache: map[uint64]*blurEntry{}}
	}
	return e.tr
}

// rasterTree растрит список кадра: по тайлам в пуле или, для мелких кадров и
// одного процессора, прямо в холст движка (как 4а.1).
func (e *Engine) rasterTree(canvas *Canvas, list *hgdraw.List, exit hgdraw.ClipState, partial bool, damage image.Rectangle) {
	if list == nil || !e.tiled.Load() || runtime.GOMAXPROCS(0) == 1 {
		e.replayTree(canvas, list, exit, partial, damage)
		return
	}
	tr := e.tileRaster()
	tr.frameNo++
	tr.checkEnv(canvas)
	tr.selectTiles(canvas, partial, damage)
	if len(tr.tiles) < tiledMinTiles {
		tr.sweep()
		e.replayTree(canvas, list, exit, partial, damage)
		return
	}
	tr.render(canvas, list, partial, damage)
	// Клип на выходе — как после Draw корня на прямом пути: оверлеи и
	// модальные слои рисуются в холст движка под ним.
	e.replayer.Replay(canvas, tiledExitList, hgdraw.ReplayOpts{Exit: exit})
}

// checkEnv сбрасывает кэш размытия, если окружение холста сменилось: растры
// сняты в формате back, на масштабе и с фоном/обоями/шрифтами того кадра.
func (tr *tileRaster) checkEnv(canvas *Canvas) {
	env := tileEnv{canvas: canvas, format: canvas.Format(), scale: canvas.Scale(), fontRev: canvas.FontRev(), bgGen: tr.e.bgGen.Load()}
	if env != tr.env {
		tr.env = env
		clear(tr.cache)
		tr.bytes = 0
	}
}

// selectTiles выбирает тайлы кадра: при частичном кадре — задевающие
// повреждение, иначе все.
func (tr *tileRaster) selectTiles(canvas *Canvas, partial bool, damage image.Rectangle) {
	tr.tiles = tr.tiles[:0]
	cols, rows := canvas.TileGrid()
	n := cols * rows
	if !partial {
		for i := 0; i < n; i++ {
			tr.tiles = append(tr.tiles, i)
		}
		return
	}
	for i := 0; i < n; i++ {
		if canvas.TileRect(i).Overlaps(damage) {
			tr.tiles = append(tr.tiles, i)
		}
	}
}

// reset забывает состояние прошлого кадра (в том числе после паники в
// Run, когда конец render не выполнился).
func (tr *tileRaster) reset() {
	for i := range tr.layers {
		tr.layers[i] = nil
	}
	tr.layers = tr.layers[:0]
	tr.snapshot, tr.scaled = nil, nil
	tr.hits, tr.misses = 0, 0
}

// render — кадр по тайлам.
func (tr *tileRaster) render(canvas *Canvas, root *hgdraw.List, partial bool, damage image.Rectangle) {
	tr.reset()
	tr.canvas, tr.root, tr.partial, tr.damage = canvas, root, partial, damage
	frame := canvas.Frame()
	region := frame
	if partial {
		region = damage.Intersect(frame)
	}
	regionL := tr.e.logicalRect(region)

	tr.findLayers(root, regionL)
	tr.prescale(root, regionL)
	tr.planHalos(frame)

	n := len(tr.tiles)
	for tr.made < cap(tr.workers) && tr.made < n {
		w := &tileWorker{c: hgraster.NewWindowCanvas(canvas), tr: tr}
		w.c.SetScaledBudget((16 << 20) / cap(tr.workers))
		w.c.SetScaledOverride(tr.scaledFor)
		w.opts = hgdraw.ReplayOpts{Direct: tr.direct, Offset: offsetContextFor, Override: w.override, Tap: w.tap}
		tr.workers <- w
		tr.made++
	}
	workpool.Run(tr, &tr.batch, n)
	tr.finishLayers()
	tr.e.stats.tiledFrames.Add(1)
	tr.e.stats.blurHits.Add(uint64(tr.hits))
	tr.e.stats.blurMisses.Add(uint64(tr.misses))
	tr.reset()
}

// RunPart — один тайл (workpool.Task).
func (tr *tileRaster) RunPart(i int) {
	w := <-tr.workers
	defer func() { tr.workers <- w }()
	canvas := tr.canvas
	w.c.SyncFrame(canvas)
	idx := tr.tiles[i]
	tile := canvas.TileRect(idx)
	h := tr.haloH[i]
	window := tile
	if h > 0 {
		window = tile.Inset(-h).Intersect(canvas.Frame())
	}
	w.c.Retarget(window)
	if h > 0 {
		w.c.PrefillFrom(tr.snapshot, window)
	} else {
		w.c.PrefillFrom(canvas.BackImage(), tile)
	}
	w.c.SetCenter(tile, canvas, idx)
	vis := window
	if tr.partial {
		// Базовый клип — всё повреждение кадра, а не его часть в окне: от
		// клипа зависит область слоя размытия и сетка его уменьшения, а они
		// обязаны совпасть с прямым путём; запись и так зажата окном.
		vis = window.Intersect(tr.damage)
		w.c.SetBaseClip(tr.damage)
	}
	w.tile, w.vis, w.halo = tile, vis, h > 0
	w.dmg[0] = tr.e.logicalRect(vis)
	w.opts.Damage = w.dmg[:]
	w.rep.Replay(w.c, tr.root, w.opts)
	w.c.CopyTo(canvas.BackImage(), vis.Intersect(tile))
	w.c.StoreCenterMark(canvas, idx)
}

// direct рисует прямой виджет в воркере: по одному (directMu), паника
// заворачивается в directPanic с виновником и стеком воркера (политика
// изоляции), при распространении — как есть.
func (tr *tileRaster) direct(h hgdraw.ListHolder, ctx hgdraw.DrawContext) {
	tr.directMu.Lock()
	defer tr.directMu.Unlock()
	w, ok := h.(widget.Widget)
	if !ok {
		h.Draw(ctx)
		return
	}
	if tr.e.FailurePolicy() == FailurePropagate {
		widget.DrawTraced(w, ctx)
		return
	}
	defer func() {
		if r := recover(); r != nil {
			widget.ResetDrawTrace()
			panic(&directPanic{w: w, value: r, stack: debug.Stack()})
		}
	}()
	widget.DrawTraced(w, ctx)
}

// override подменяет слой с попаданием кэша блитом готового растра.
func (w *tileWorker) override(c *hgdraw.Cmd, ctx hgdraw.DrawContext) bool {
	if c.Op != hgdraw.OpBlurBehind {
		return false
	}
	for _, l := range w.tr.layers {
		if l.cmd == c && l.hit != nil {
			w.c.BlitBackdrop(l.hit.raster, l.rectL, l.tint)
			return true
		}
	}
	return false
}

// tap снимает результат слоя-промаха из центра ореола в растр кэша: только
// то, что тайл действительно нарисовал (центр ∩ повреждение ∩ слой), и
// считает накрытую площадь — растр записывается в кэш, когда накрыт весь.
func (w *tileWorker) tap(c *hgdraw.Cmd) {
	if !w.halo || c.Op != hgdraw.OpBlurBehind {
		return
	}
	for _, l := range w.tr.layers {
		if l.cmd == c && l.raster != nil {
			part := w.tile.Intersect(w.vis).Intersect(l.pr)
			if part.Empty() {
				continue
			}
			w.c.CopyTo(l.raster, part)
			l.covered.Add(int64(part.Dx() * part.Dy()))
		}
	}
}

// scaledFor — готовый растр крупной картинки с горутины кадра.
func (tr *tileRaster) scaledFor(src image.Image, pw, ph int) *image.RGBA {
	if tr.scaled == nil {
		return nil
	}
	return tr.scaled[scaledKey{hgdraw.PointerOf(src), pw, ph}]
}

// prescale масштабирует крупные картинки кадра холстом движка один раз.
func (tr *tileRaster) prescale(root *hgdraw.List, regionL image.Rectangle) {
	canvas := tr.canvas
	hgdraw.VisitOverlapping(root, regionL, func(c *hgdraw.Cmd) bool {
		var src image.Image
		var r image.Rectangle
		switch c.Op {
		case hgdraw.OpDrawImageScaled:
			src, r = c.Ext.Img, c.Rect()
		case hgdraw.OpDrawImage:
			if canvas.Scale() == 1 {
				return true
			}
			src, r = c.Ext.Img, c.Rect()
		default:
			return true
		}
		// Холст масштабирует картинку без обёртки непрозрачности — ключ по ней.
		src, _ = hgdraw.IsOpaqueImage(src)
		pr := canvas.PhysRect(r)
		pw, ph := pr.Dx(), pr.Dy()
		if pw*ph < tiledBigImage || hgdraw.PointerOf(src) == 0 {
			return true
		}
		if tr.scaled == nil {
			tr.scaled = map[scaledKey]*image.RGBA{}
		}
		k := scaledKey{hgdraw.PointerOf(src), pw, ph}
		if _, ok := tr.scaled[k]; !ok {
			tr.scaled[k] = canvas.ScaledFor(src, pw, ph)
		}
		return true
	})
}

// haloFor — запас ореола для физического радиуса: два прохода box-blur
// радиусом pradius/4 на уменьшенной в 4 раза подложке плюс неполный блок с
// каждой стороны; кратно 4 — одинаковые размеры окон.
func haloFor(pradius int) int {
	sr := pradius / 4
	if sr < 1 {
		sr = 1
	}
	h := 8*sr + 12
	return (h + 3) &^ 3
}

// findLayers находит слои размытия в области кадра, считает их ключи и
// решает попадание/промах.
func (tr *tileRaster) findLayers(root *hgdraw.List, regionL image.Rectangle) {
	canvas := tr.canvas
	frame := canvas.Frame()
	add := func(c *hgdraw.Cmd, owner hgdraw.ListHolder, inGroup bool) {
		rectL := c.Rect()
		pr := canvas.PhysRect(rectL)
		if cl := c.Clip; cl != nil && !cl.NoClip {
			pr = pr.Intersect(canvas.PhysRect(cl.Clip))
		}
		pr = pr.Intersect(frame)
		if pr.Empty() || tr.partial && !pr.Overlaps(tr.damage) {
			return // как на прямом пути: слой вне повреждения не выполняется
		}
		pradius := canvas.ScaleThickness(int(c.R))
		if pradius <= 0 {
			pradius = 1
		}
		l := &blurLayer{cmd: c, rectL: rectL, pr: pr, pradius: pradius, tint: c.Col, inGroup: inGroup,
			src: pr.Inset(-pradius).Intersect(frame)}
		canvas.NoteBackdrop(rectL)
		if !inGroup {
			tr.keyFor(l, owner)
		} else {
			tr.misses++
		}
		tr.layers = append(tr.layers, l)
	}
	hgdraw.VisitOverlappingOwner(root, regionL, func(c *hgdraw.Cmd, owner hgdraw.ListHolder) bool {
		switch c.Op {
		case hgdraw.OpBlurBehind:
			if c.R > 0 {
				add(c, owner, false)
			}
		case hgdraw.OpOpacity:
			eachBlurIn(c.Ext.Sub, regionL, func(b *hgdraw.Cmd) { add(b, owner, true) })
		}
		return true
	})
}

// eachBlurIn зовёт fn для каждого слоя размытия внутри списка (группы) и
// его детей, задевающих rect.
func eachBlurIn(l *hgdraw.List, rect image.Rectangle, fn func(*hgdraw.Cmd)) {
	hgdraw.VisitOverlapping(l, rect, func(c *hgdraw.Cmd) bool {
		switch c.Op {
		case hgdraw.OpBlurBehind:
			if c.R > 0 {
				fn(c)
			}
		case hgdraw.OpOpacity:
			eachBlurIn(c.Ext.Sub, rect, fn)
		}
		return true
	})
}

// keyFor считает идентичность слоя (холдер списка и хэш команды) и ключ
// содержимого под ним; решает, попадание это или промах.
func (tr *tileRaster) keyFor(l *blurLayer, owner hgdraw.ListHolder) {
	l.id = hgdraw.Mix(l.cmd.Hash(hgdraw.HashSeed), uint64(hgdraw.PointerOf(owner)))
	srcL := tr.e.logicalRect(l.src)
	key := hgdraw.HashSeed
	cacheable := true
	var imgs []image.Image
	inv := tr.e.imgInv
	mixImage := func(img image.Image) {
		g, all := inv.Gen(hgdraw.PointerOf(img))
		key = hgdraw.Mix(hgdraw.Mix(key, g), all)
		imgs = append(imgs, img)
	}
	hgdraw.VisitOverlapping(tr.root, srcL, func(c *hgdraw.Cmd) bool {
		if c == l.cmd {
			return false
		}
		switch c.Op {
		case hgdraw.OpDirect, hgdraw.OpBlitPixels, hgdraw.OpBlitBGRA:
			// Содержимое без версии: буфер переписывается на месте.
			cacheable = false
			return false
		case hgdraw.OpBlurBehind:
			// Нижний слой размытия: его результат под нами зависит и от того,
			// что лежит за пределами нашей области чтения.
			if c.R > 0 {
				cacheable = false
				return false
			}
		case hgdraw.OpOpacity:
			if hasUnkeyable(c.Ext.Sub, srcL) {
				cacheable = false
				return false
			}
			eachImageIn(c.Ext.Sub, srcL, mixImage)
		}
		key = c.Hash(key)
		if e := c.Ext; e != nil && e.Img != nil {
			mixImage(e.Img)
		}
		return true
	})
	l.cacheable, l.key, l.imgs = cacheable, key, imgs
	if !cacheable {
		tr.misses++
		return
	}
	ent := tr.cache[l.id]
	if ent != nil && ent.key == key {
		ent.last = tr.frameNo
		l.hit = ent
		tr.hits++
		return
	}
	tr.misses++
	if tr.partial && !l.pr.In(tr.damage) {
		return // часть слоя не рисуется — растр не собрать
	}
	if ent != nil && ent.raster.Rect == l.pr {
		// Устаревшая запись того же размера: её растр переписывается на
		// месте (запись снимается — ключ прежний, содержимое уже нет).
		delete(tr.cache, l.id)
		tr.bytes -= len(ent.raster.Pix)
		l.raster = ent.raster
		return
	}
	if tr.bytes+l.pr.Dx()*l.pr.Dy()*4 > blurCacheBudget {
		return // не влезает в бюджет — только ореол
	}
	l.raster = image.NewRGBA(l.pr)
}

// hasUnkeyable — в списке группы есть команда, которую ключ учесть не умеет.
func hasUnkeyable(l *hgdraw.List, rect image.Rectangle) bool {
	found := false
	hgdraw.VisitOverlapping(l, rect, func(c *hgdraw.Cmd) bool {
		switch c.Op {
		case hgdraw.OpDirect, hgdraw.OpBlitPixels, hgdraw.OpBlitBGRA:
			found = true
			return false
		case hgdraw.OpBlurBehind:
			if c.R > 0 {
				found = true
				return false
			}
		case hgdraw.OpOpacity:
			if hasUnkeyable(c.Ext.Sub, rect) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// eachImageIn зовёт fn для каждой картинки в списке группы (и вложенных).
func eachImageIn(l *hgdraw.List, rect image.Rectangle, fn func(image.Image)) {
	hgdraw.VisitOverlapping(l, rect, func(c *hgdraw.Cmd) bool {
		if c.Op == hgdraw.OpOpacity {
			eachImageIn(c.Ext.Sub, rect, fn)
		} else if e := c.Ext; e != nil && e.Img != nil {
			fn(e.Img)
		}
		return true
	})
}

// planHalos назначает тайлам запас ореола по слоям-промахам и снимает
// копию back под ореолами.
func (tr *tileRaster) planHalos(frame image.Rectangle) {
	tr.haloH = tr.haloH[:0]
	var union image.Rectangle
	for _, idx := range tr.tiles {
		tile := tr.canvas.TileRect(idx)
		// Сверху вниз: верхний слой читает нижний, и нижнему нужен запас в
		// пределах досягаемости верхнего (tile.Inset(−h) с уже набранным h).
		h := 0
		for j := len(tr.layers) - 1; j >= 0; j-- {
			l := tr.layers[j]
			if l.hit != nil {
				continue
			}
			if l.pr.Overlaps(tile.Inset(-h)) {
				h += haloFor(l.pradius)
			}
		}
		tr.haloH = append(tr.haloH, h)
		if h > 0 {
			union = union.Union(tile.Inset(-h).Intersect(frame))
		}
	}
	tr.snapshot = nil
	if union.Empty() {
		return
	}
	need := union.Dx() * union.Dy() * 4
	if cap(tr.snapBuf) < need {
		tr.snapBuf = make([]byte, need)
	}
	snap := &image.RGBA{Pix: tr.snapBuf[:need], Stride: union.Dx() * 4, Rect: union}
	back := tr.canvas.BackImage()
	n := union.Dx() * 4
	for y := union.Min.Y; y < union.Max.Y; y++ {
		s := back.PixOffset(union.Min.X, y)
		d := snap.PixOffset(union.Min.X, y)
		copy(snap.Pix[d:d+n], back.Pix[s:s+n])
	}
	tr.snapshot = snap
}

// finishLayers записывает растры промахов, накрытые целиком, и чистит кэш.
func (tr *tileRaster) finishLayers() {
	for _, l := range tr.layers {
		if l.raster == nil || !l.cacheable {
			continue
		}
		if l.covered.Load() != int64(l.pr.Dx()*l.pr.Dy()) {
			continue // часть слоя не рисовалась (повреждение уже слоя)
		}
		if old := tr.cache[l.id]; old != nil {
			tr.bytes -= len(old.raster.Pix)
		}
		tr.cache[l.id] = &blurEntry{key: l.key, raster: l.raster, imgs: l.imgs, last: tr.frameNo}
		tr.bytes += len(l.raster.Pix)
	}
	tr.sweep()
}

// sweep удаляет записи, не использованные blurCacheTTL кадров.
func (tr *tileRaster) sweep() {
	for id, ent := range tr.cache {
		if tr.frameNo-ent.last > blurCacheTTL {
			tr.bytes -= len(ent.raster.Pix)
			delete(tr.cache, id)
		}
	}
}
