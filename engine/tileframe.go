package engine

// tileframe.go — тайловая растеризация списка команд в пуле воркеров
// (ADR-0005, задача 4а.2); отбор тайлов хэш-диффом — hashdiff.go (ADR-0006).
//
// Кадр с путём команд (cmdpath.go) делится на тайлы 64×64 физических
// пикселей — те же, что у диффа. Из выбранных тайлов (все или задевающие
// повреждение) хэш-дифф оставляет изменившиеся и принуждённые. Воркер берёт
// холст-окно (core/raster/window.go): буфер размером с тайл в координатах
// кадра, фон холста в области растра, базовый клип повреждения,
// воспроизведение дерева с отсечкой по тайлу, сравнение с front и копирование
// в него (ADR-0007: один буфер кадра; в окно back холста или чужую память
// потребителя — копирование строк) и перенос признака тайла. Мало тайлов или
// один процессор — те же тайлы тем же воркером на горутине кадра.
//
// Размытая подложка (BlurBehind) читает кадр за пределами тайла. По решению
// владельца — кэш с ореолом на промахе: слой, чьё содержимое под ним не
// менялось (ключ — хэши команд под областью чтения), подменяется блитом
// готового растра; слой-промах рисуется в окнах с запасом по периметру
// (ореол), откуда размытие читает подложку, а растр кэша собирается из
// центров ореолов по ходу кадра. Окно с ореолом в области растра кладёт фон
// и рисует подложку само (под пропущенным соседом ничего не пишется), вне
// её — частичный кадр — берёт прошлый кадр (front при одном буфере, back при
// двух или чужой памяти), который в этом кадре там никто не трогает.
//
// Без пула (один процессор, тайлы выключены) слой размытия читает соседние
// тайлы, и всё выбранное воспроизводится одной горутиной в окно back холста
// над областью растра и областями чтения слоёв; то же — когда ни один
// выбранный тайл не пропущен (строка текста на стыке тайлов обходит свои
// глифы один раз). Иначе мало тайлов — по тайлам тем же воркером на горутине
// кадра (окно со шрифтами холста движка, без личных видов).

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
	// tiledMinTiles — с какого числа рисуемых тайлов кадр идёт по тайлам;
	// меньше — воспроизведение в холст движка (раздача пула, префилл и
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
	vis  image.Rectangle // окно ∩ область растра — что тайл на самом деле нарисовал
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

// tileEnv — окружение холста, от которого зависят растры кэша размытия и
// хэши тайлов: смена любого из полей делает их устаревшими.
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
	// serial — воркер горутины кадра (последовательный путь): окно со
	// шрифтами холста движга без личных видов.
	serial *tileWorker
	batch   workpool.Batch
	cache   map[uint64]*blurEntry
	bytes   int
	frameNo uint64
	env     tileEnv
	// directMu — прямые виджеты рисуются по одному: их Draw не обязан быть
	// готов к вызову из нескольких воркеров сразу (виджет, лежащий на
	// нескольких тайлах, иначе рисовался бы параллельно).
	directMu sync.Mutex

	// Хэш-дифф (hashdiff.go): хэши тайлов на момент, когда их рисовали
	// (prev, действительны по valid), хэши текущего кадра (cur), принуждение
	// и пропуск на кадр — по индексу тайла.
	prev, cur []uint64
	valid     []bool
	forced    []bool
	skipMask  []bool

	// На кадр.
	canvas   *Canvas
	root     *hgdraw.List
	pool     bool // пул доступен: кэш размытия в деле (попадание/промах решены до выбора тайлов)
	// toBack — тайлы копируются в back (чужая память, два буфера), иначе
	// сравниваются с front из окна; win — окно back холста (тайлы под ним —
	// в окно); touch — помечать скопированные в back тайлы тронутыми (свой
	// back); prefill — откуда окно берёт ореол вне области растра.
	toBack  bool
	win     image.Rectangle
	touch   bool
	prefill *image.RGBA
	partial bool
	damage   image.Rectangle // физическое объединённое повреждение
	region   image.Rectangle // область растра: кадр или повреждение ∩ кадр
	regionL  image.Rectangle
	selRect  image.Rectangle // выбранные тайлы одним прямоугольником сетки
	selL     image.Rectangle
	selected []int // выбранные тайлы: все или задевающие повреждение
	tiles    []int // рисуемые: выбранные без пропущенных хэш-диффом
	skipped  []int
	haloH    []int
	layers   []*blurLayer
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

// rasterResult — что растр оставил диффу: hashed — хэш-дифф пропустил часть
// выбранных тайлов, сравнивать надо только rendered; skipped — пропущенные
// (при чужих буферах вывода копируются из front). Срезы живут до следующего
// кадра.
type rasterResult struct {
	hashed            bool
	rendered, skipped []int
}

// rasterTree растрит список кадра: по тайлам в пуле или тем же воркером
// одной горутиной (мелкие кадры, один процессор); с хэш-диффом — только
// изменившиеся и принуждённые тайлы (ADR-0006). Без пула слой размытия
// читает соседние тайлы — тогда всё выбранное воспроизводится в окно back
// холста (ADR-0007), как в 4а.1.
func (e *Engine) rasterTree(canvas *Canvas, list *hgdraw.List, exit hgdraw.ClipState, partial bool, damage image.Rectangle, plan *framePlan) rasterResult {
	if list == nil {
		return rasterResult{}
	}
	tr := e.tileRaster()
	tr.frameNo++
	tr.checkEnv(canvas)
	tr.begin(canvas, list, partial, damage)
	tr.findLayers()
	pool := e.tiled.Load() && runtime.GOMAXPROCS(0) > 1
	tr.pool = pool
	if pool {
		// До выбора тайлов: промах слоя, который кадр накроет целиком,
		// принуждает его тайлы к растру (decide) — иначе запись, вымытая по
		// TTL, не восстановилась бы, пока под слоем есть пропущенный тайл.
		tr.lookupLayers()
	}
	hashOn := e.hashDiff.Load() && plan != nil
	if hashOn {
		tr.hashTiles()
		tr.decide(plan)
	} else {
		tr.unskip()
		clear(tr.valid)
	}
	own := canvas.UsingOwnBack()
	single := own && canvas.SingleBuffer()
	tr.toBack, tr.touch, tr.win = !single, own, image.Rectangle{}
	tr.prefill = canvas.BackImage()
	if single {
		tr.prefill = canvas.Front()
		if len(e.post) > 0 {
			// Окно под слои этого кадра: тайлы под ним — в окно. Окно прошлых
			// кадров (до освобождения) тайлы не перехватывает: они уйдут в
			// front мимо него, и оно перестаёт быть равным front.
			tr.win = canvas.BackWindow()
		} else if len(tr.tiles) > 0 {
			canvas.InvalidateBackWindow()
		}
	}
	tiled := pool && len(tr.tiles) >= tiledMinTiles
	whole := false
	if len(tr.tiles) > 0 && !tiled && tr.layersTouchRendered() {
		// Рисуемый тайл задевает слой размытия. В пуле слой — попадание кэша
		// или ореол, и пропуск остаётся (тик часов на панели со стеклом);
		// без пула слой выполняется в холсте движка и читает соседние тайлы:
		// им нужно то же содержимое, что на прямом пути, — рисуется всё
		// выбранное.
		if pool {
			tiled = true
		} else {
			tr.unskip()
			whole = true
		}
	}
	if !tiled && len(tr.tiles) > 0 && len(tr.skipped) == 0 {
		// Ни один выбранный тайл не пропущен: всё выбранное одним
		// воспроизведением в окно back — дешевле, чем по тайлам (строка текста
		// на стыке тайлов обходит свои глифы один раз, а не на каждый тайл).
		whole = true
	}
	e.stats.tilesSkipped.Add(uint64(len(tr.skipped)))
	switch {
	case len(tr.tiles) == 0:
		// Ничего не изменилось: ни фона, ни растра — и диффу нечего делать.
		tr.sweep()
	case whole:
		tr.sweep()
		if own {
			// Окно back над областью растра и областями чтения слоёв: за
			// краем области растра слой читает прошлый кадр из front.
			win := tr.region
			for _, l := range tr.layers {
				win = win.Union(l.src)
			}
			canvas.SetBackWindow(win)
			canvas.Touch(tr.region)
		}
		e.blitBackgroundFrame(canvas, partial, damage)
		e.replayTree(canvas, list, exit, partial, damage)
	default:
		tr.render(tiled)
	}
	// Клип на выходе — как после Draw корня и слоёв на прямом пути:
	// отложенные слои рисуются в холст движка под ним.
	e.replayer.Replay(canvas, tiledExitList, hgdraw.ReplayOpts{Exit: exit})
	res := rasterResult{hashed: len(tr.skipped) > 0, rendered: tr.tiles, skipped: tr.skipped}
	if hashOn {
		tr.finishHash(plan)
	}
	tr.reset()
	return res
}

// checkEnv сбрасывает кэш размытия и хэши тайлов, если окружение холста
// сменилось: растры и хэши сняты в формате back, на масштабе и с
// фоном/обоями/шрифтами того кадра.
func (tr *tileRaster) checkEnv(canvas *Canvas) {
	env := tileEnv{canvas: canvas, format: canvas.Format(), scale: canvas.Scale(), fontRev: canvas.FontRev(), bgGen: tr.e.bgGen.Load()}
	if env != tr.env {
		tr.env = env
		clear(tr.cache)
		tr.bytes = 0
		clear(tr.valid)
	}
}

// begin готовит состояние кадра: область растра, таблицы хэшей под сетку,
// выбранные тайлы (при частичном кадре — задевающие повреждение, иначе все).
func (tr *tileRaster) begin(canvas *Canvas, root *hgdraw.List, partial bool, damage image.Rectangle) {
	e := tr.e
	tr.canvas, tr.root, tr.partial, tr.damage = canvas, root, partial, damage
	frame := canvas.Frame()
	tr.region = frame
	if partial {
		tr.region = damage.Intersect(frame)
	}
	tr.regionL = e.logicalRect(tr.region)
	cols, rows := canvas.TileGrid()
	n := cols * rows
	if len(tr.prev) != n {
		tr.prev, tr.cur = make([]uint64, n), make([]uint64, n)
		tr.valid, tr.forced, tr.skipMask = make([]bool, n), make([]bool, n), make([]bool, n)
	}
	tr.selected = tr.selected[:0]
	ts := output.TileSize
	switch {
	case !partial:
		for i := 0; i < n; i++ {
			tr.selected = append(tr.selected, i)
		}
		tr.selRect = frame
	case tr.region.Empty():
		tr.selRect = image.Rectangle{}
	default:
		r := tr.region
		tx0, ty0 := r.Min.X/ts, r.Min.Y/ts
		tx1, ty1 := (r.Max.X-1)/ts, (r.Max.Y-1)/ts
		if tx1 >= cols {
			tx1 = cols - 1
		}
		if ty1 >= rows {
			ty1 = rows - 1
		}
		for ty := ty0; ty <= ty1; ty++ {
			for tx := tx0; tx <= tx1; tx++ {
				tr.selected = append(tr.selected, ty*cols+tx)
			}
		}
		tr.selRect = image.Rect(tx0*ts, ty0*ts, (tx1+1)*ts, (ty1+1)*ts).Intersect(frame)
	}
	tr.selL = e.logicalRect(tr.selRect)
	tr.tiles, tr.skipped = tr.tiles[:0], tr.skipped[:0]
	tr.reset()
}

// reset забывает слои и растры кадра (в том числе после паники в Run, когда
// конец render не выполнился; begin следующего кадра зовёт его тоже).
func (tr *tileRaster) reset() {
	for i := range tr.layers {
		tr.layers[i] = nil
	}
	tr.layers = tr.layers[:0]
	tr.scaled = nil
	tr.hits, tr.misses = 0, 0
}

// render — кадр по тайлам: в пуле (parallel) или тем же воркером подряд на
// горутине кадра.
func (tr *tileRaster) render(parallel bool) {
	canvas := tr.canvas
	tr.allocRasters()
	tr.prescale(tr.root, tr.regionL)
	tr.planHalos()

	n := len(tr.tiles)
	if !parallel {
		// Горутина кадра: своё окно со шрифтами холста движка — ни замка
		// вида, ни второго кэша глифов на пару тайлов.
		if tr.serial == nil {
			tr.serial = tr.newWorker(hgraster.NewWindowCanvasShared(canvas))
		}
		for i := 0; i < n; i++ {
			tr.runTile(tr.serial, i)
		}
		tr.finishLayers()
		tr.e.stats.blurHits.Add(uint64(tr.hits))
		tr.e.stats.blurMisses.Add(uint64(tr.misses))
		return
	}
	for tr.made < cap(tr.workers) && tr.made < n {
		tr.workers <- tr.newWorker(hgraster.NewWindowCanvas(canvas))
		tr.made++
	}
	workpool.Run(tr, &tr.batch, n)
	tr.e.stats.tiledFrames.Add(1)
	tr.finishLayers()
	tr.e.stats.blurHits.Add(uint64(tr.hits))
	tr.e.stats.blurMisses.Add(uint64(tr.misses))
}

// newWorker — воркер над окном c.
func (tr *tileRaster) newWorker(c *hgraster.Canvas) *tileWorker {
	w := &tileWorker{c: c, tr: tr}
	w.c.SetScaledBudget((16 << 20) / cap(tr.workers))
	w.c.SetScaledOverride(tr.scaledFor)
	w.opts = hgdraw.ReplayOpts{Direct: tr.direct, Offset: offsetContextFor, Override: w.override, Tap: w.tap}
	return w
}

// RunPart — один тайл (workpool.Task).
func (tr *tileRaster) RunPart(i int) {
	w := <-tr.workers
	defer func() { tr.workers <- w }()
	tr.runTile(w, i)
}

// runTile рисует тайл i воркером w.
func (tr *tileRaster) runTile(w *tileWorker, i int) {
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
	w.c.SetCenter(tile)
	if h > 0 && tr.partial {
		// Ореол вне области растра: прошлый кадр (front при одном буфере,
		// back при чужой памяти или двух буферах — в этом кадре там никто не
		// пишет) — как подложка частичного кадра на прямом пути.
		for _, part := range outsideOf(window, tr.region) {
			w.c.PrefillFrom(tr.prefill, part)
		}
	}
	// Фон — своей частью окна, а не из back: под пропущенным соседом back
	// хранит готовый кадр, а подложку ореола окно рисует от фона само.
	vis := window.Intersect(tr.region)
	w.c.BlitBackgroundIn(vis)
	if tr.partial {
		// Базовый клип — всё повреждение кадра, а не его часть в окне: от
		// клипа зависит область слоя размытия и сетка его уменьшения, а они
		// обязаны совпасть с прямым путём; запись и так зажата окном.
		w.c.SetBaseClip(tr.damage)
	}
	w.tile, w.vis, w.halo = tile, vis, h > 0
	w.dmg[0] = tr.e.logicalRect(vis)
	w.opts.Damage = w.dmg[:]
	w.rep.Replay(w.c, tr.root, w.opts)
	part := vis.Intersect(tile)
	if tr.toBack || tile.Overlaps(tr.win) {
		// Чужая память, два буфера или тайл под слоями после растра: строки
		// в back, сравнит сброс окна или дифф.
		w.c.CopyTo(canvas.BackImage(), part)
		if tr.touch {
			canvas.TouchTile(idx)
		}
	} else {
		// Один буфер: сравнение с front и копирование — здесь, в воркере.
		canvas.SyncTileFrom(w.c.BackImage(), part)
	}
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
// то, что тайл действительно нарисовал (центр ∩ область растра ∩ слой), и
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

// findLayers находит слои размытия в области растра, отмечает их области
// подложки у холста и считает ключи содержимого; попадание или промах решает
// lookupLayers — только на тайловом пути (последовательный путь кэш не
// трогает, ключи нужны хэшам тайлов).
func (tr *tileRaster) findLayers() {
	canvas, root, regionL := tr.canvas, tr.root, tr.regionL
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
			tr.contentKey(l, owner)
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

// contentKey считает идентичность слоя (холдер списка и хэш команды) и ключ
// содержимого под ним: команды до слоя в области чтения с поколениями их
// картинок; слой под прямым виджетом, блитом буфера или другим слоем — не
// кэшируется (его результат зависит от того, чего ключ не видит).
func (tr *tileRaster) contentKey(l *blurLayer, owner hgdraw.ListHolder) {
	l.id = hgdraw.Mix(l.cmd.Hash(hgdraw.HashSeed), uint64(hgdraw.PointerOf(owner)))
	srcL := tr.e.logicalRect(l.src)
	key := hgdraw.HashSeed
	cacheable := true
	var imgs []image.Image
	inv := tr.e.imgInv
	mixImage := func(img image.Image) {
		img, _ = hgdraw.IsOpaqueImage(img) // поколение — у внутренней картинки
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
			// Нижний слой размытия: его результат под нами зависит от того,
			// что лежит за пределами нашей области чтения, — это описывает его
			// собственный ключ (слои идут в порядке рисования, нижний уже
			// посчитан); некэшируемый нижний делает некэшируемым и верхний.
			if c.R > 0 {
				ll := tr.layerOf(c)
				if ll == nil || ll.inGroup || !ll.cacheable {
					cacheable = false
					return false
				}
				key = hgdraw.Mix(key, ll.key)
			}
		case hgdraw.OpOpacity:
			if hasUnkeyable(c.Ext.Sub, srcL) {
				cacheable = false
				return false
			}
			eachImageIn(c.Ext.Sub, srcL, mixImage)
		}
		if c.Op == hgdraw.OpOpacity {
			key = c.Hash(key) // живой: списки детей внутри группы перезаписываются
		} else {
			key = hgdraw.Mix(key, c.Sum())
		}
		if e := c.Ext; e != nil && e.Img != nil {
			mixImage(e.Img)
		}
		return true
	})
	l.cacheable, l.key, l.imgs = cacheable, key, imgs
}

// lookupLayers решает по слоям попадание или промах; попадание продлевает
// запись (TTL считает кадры без слоя, а не без растра).
func (tr *tileRaster) lookupLayers() {
	for _, l := range tr.layers {
		if l.inGroup || !l.cacheable {
			tr.misses++
			continue
		}
		if ent := tr.cache[l.id]; ent != nil && ent.key == l.key {
			ent.last = tr.frameNo
			l.hit = ent
			tr.hits++
			continue
		}
		tr.misses++
	}
}

// wantsRaster — промах слоя, растр которого кадр может собрать: слой в
// повреждении целиком (decide принуждает его тайлы к растру).
func (tr *tileRaster) wantsRaster(l *blurLayer) bool {
	if l.inGroup || !l.cacheable || l.hit != nil {
		return false
	}
	return !tr.partial || l.pr.In(tr.damage)
}

// allocRasters заводит растры промахов, которые кадр накроет целиком: без
// пропущенных хэш-диффом тайлов под слоем (иначе растр не собрать — и не
// стоит его выделять).
func (tr *tileRaster) allocRasters() {
	for _, l := range tr.layers {
		if !tr.wantsRaster(l) || tr.anySkipped(l.pr) {
			continue
		}
		if ent := tr.cache[l.id]; ent != nil && ent.raster.Rect == l.pr {
			// Устаревшая запись того же размера: её растр переписывается на
			// месте (запись снимается — ключ прежний, содержимое уже нет).
			delete(tr.cache, l.id)
			tr.bytes -= len(ent.raster.Pix)
			l.raster = ent.raster
			continue
		}
		if tr.bytes+l.pr.Dx()*l.pr.Dy()*4 > blurCacheBudget {
			continue // не влезает в бюджет — только ореол
		}
		l.raster = image.NewRGBA(l.pr)
	}
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

// planHalos назначает рисуемым тайлам запас ореола по слоям-промахам.
func (tr *tileRaster) planHalos() {
	tr.haloH = tr.haloH[:0]
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
	}
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
