package engine

// hashdiff.go — хэш-дифф тайлов по командам (ADR-0006, задача 4а.3).
//
// Пиксели тайла — функция от команд, задевающих тайл (в порядке рисования, с
// клипами), фона и окружения холста. Хэш этой функции считается на кадр для
// каждого выбранного тайла и сравнивается с хэшем кадра, когда тайл рисовали
// в последний раз: совпал — тайл не растрится, не сравнивается и не попадает в
// выдачу (его back равен front). Что в функцию не укладывается, тайл
// принуждает к растру: ничейное повреждение, «перерисовать всё», переносы.
// Всё, что ложится поверх растра дерева (оверлеи в холсте, модальные,
// подсказка, переходы оверлеев), в кадре появления ложится на верный back
// (пропущенный тайл равен тому, что нарисовал бы кадр), а на следующем
// кадре лишает свои тайлы действительного хэша: там оно либо рисуется снова
// поверх свежего растра, либо исчезло — и команды под ним не менялись, а
// пиксели да.

import (
	"image"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"github.com/oops1/headless-gui/v3/output"
)

// hashDiffSerialMax — до скольких нарисованных тайлов дифф идёт по их
// индексам одной горутиной; больше — обычный дифф по области в пуле
// (сравнение 64 тайлов — тот же порог параллельности, что у diffTiles).
const hashDiffSerialMax = 64

// SetHashDiff включает или выключает хэш-дифф тайлов пути команд (по
// умолчанию включён). Выключенный — растр и дифф всех выбранных тайлов, как
// в 4а.2. Картинка на обоих путях одна и та же.
func (e *Engine) SetHashDiff(on bool) {
	e.hashDiff.Store(on)
	e.Invalidate()
}

// HashDiff сообщает, включён ли хэш-дифф тайлов.
func (e *Engine) HashDiff() bool { return e.hashDiff.Load() }

// framePlan — что кадр пути команд знает до растра дерева. Слои поверх
// дерева в план не входят: они записаны командами или рисуются в окно back и
// лишают хэша тайлы под своим местом (noteOverlaid, ADR-0007).
type framePlan struct {
	// force — ничейное повреждение кадра (логическое): приложение заявило
	// изменение, о котором команды не знают.
	force []image.Rectangle
	// forceAll — все выбранные тайлы рисуются: Invalidate(), полный кадр,
	// переносы (они меняют front до растра).
	forceAll bool
}

// planFrame собирает план кадра. all — Invalidate() или полный кадр; moved —
// в этом кадре применены переносы.
func (e *Engine) planFrame(all bool, force []image.Rectangle, moved bool) *framePlan {
	p := &e.plan
	*p = framePlan{force: force, forceAll: all || moved}
	return p
}

// hashTiles считает хэши выбранных тайлов кадра: обход дерева списков в
// порядке рисования с отсечкой по выбранным тайлам, каждая листовая команда
// подмешивается в тайлы, которые задевает её физический охват. Хэш команды
// посчитан рекордером (Cmd.Sum); на кадре к нему добавляются поколения
// картинок, ключ содержимого слоя размытия и номер кадра у того, что
// версии не имеет (прямые виджеты, блиты буферов, некэшируемые слои).
func (tr *tileRaster) hashTiles() {
	canvas := tr.canvas
	cols, rows := canvas.TileGrid()
	ts := output.TileSize
	cur := tr.cur
	for _, i := range tr.selected {
		cur[i] = hgdraw.HashSeed
	}
	sel, selL := tr.selRect, tr.selL
	nonce := tr.frameNo
	inv := tr.e.imgInv
	fold := func(pr image.Rectangle, v uint64) {
		pr = pr.Intersect(sel)
		if pr.Empty() {
			return
		}
		tx0, ty0 := pr.Min.X/ts, pr.Min.Y/ts
		tx1, ty1 := (pr.Max.X-1)/ts, (pr.Max.Y-1)/ts
		if tx1 >= cols {
			tx1 = cols - 1
		}
		if ty1 >= rows {
			ty1 = rows - 1
		}
		for ty := ty0; ty <= ty1; ty++ {
			row := ty * cols
			for tx := tx0; tx <= tx1; tx++ {
				cur[row+tx] = hgdraw.Mix(cur[row+tx], v)
			}
		}
	}
	mixImage := func(v uint64, img image.Image) uint64 {
		// Поколение — у внутренней картинки обёртки непрозрачности: её адрес
		// знают приложение (InvalidateImageCache) и кэш масштабирования.
		img, _ = hgdraw.IsOpaqueImage(img)
		g, all := inv.Gen(hgdraw.PointerOf(img))
		return hgdraw.Mix(hgdraw.Mix(v, g), all)
	}
	hgdraw.VisitOverlapping(tr.root, selL, func(c *hgdraw.Cmd) bool {
		v := c.Sum()
		switch c.Op {
		case hgdraw.OpDirect, hgdraw.OpBlitPixels, hgdraw.OpBlitBGRA:
			// Прямой виджет и блит буфера без версии: тайл рисуется всегда.
			v = hgdraw.Mix(v, nonce)
		case hgdraw.OpBlurBehind:
			// Слой читает содержимое под собой с запасом радиуса: правка под
			// слоем меняет хэш всех его тайлов, а не только тайлов правки.
			if l := tr.layerOf(c); l != nil && l.cacheable && !l.inGroup {
				v = hgdraw.Mix(v, l.key)
			} else {
				v = hgdraw.Mix(v, nonce)
			}
		case hgdraw.OpOpacity:
			// Хэш группы — живой, а не записанный: список ребёнка внутри
			// группы перезаписывается под удержанным родителем, и Sum группы
			// его новых команд не видит.
			v = c.Hash(hgdraw.HashSeed)
			if hasUnkeyable(c.Ext.Sub, selL) {
				v = hgdraw.Mix(v, nonce)
			}
			eachImageIn(c.Ext.Sub, selL, func(img image.Image) { v = mixImage(v, img) })
		}
		if x := c.Ext; x != nil && x.Img != nil {
			v = mixImage(v, x.Img)
		}
		fold(canvas.PhysRect(c.BBox), v)
		return true
	})
}

// layerOf — слой размытия кадра по его команде.
func (tr *tileRaster) layerOf(c *hgdraw.Cmd) *blurLayer {
	for _, l := range tr.layers {
		if l.cmd == c {
			return l
		}
	}
	return nil
}

// decide делит выбранные тайлы на рисуемые и пропущенные: рисуются тайлы с
// другим хэшем, без действительного прошлого хэша и под принуждением.
func (tr *tileRaster) decide(plan *framePlan) {
	canvas := tr.canvas
	clear(tr.forced)
	clear(tr.skipMask)
	for _, r := range plan.force {
		tr.setTiles(tr.forced, canvas.PhysRect(r), true)
	}
	if tr.pool {
		// Промах слоя, который кадр накроет целиком: все его тайлы рисуются,
		// чтобы собрать растр кэша. Обычно их хэши и так изменились (ключ
		// содержимого — в хэше тайла); иначе это запись, вымытая по TTL за
		// кадры без обращений, — один растр слоя вместо ореола на каждой
		// правке впредь.
		for _, l := range tr.layers {
			if tr.wantsRaster(l) {
				tr.setTiles(tr.forced, l.pr, true)
			}
		}
	}
	tr.tiles, tr.skipped = tr.tiles[:0], tr.skipped[:0]
	for _, i := range tr.selected {
		if plan.forceAll || !tr.valid[i] || tr.forced[i] || tr.cur[i] != tr.prev[i] {
			tr.tiles = append(tr.tiles, i)
			continue
		}
		tr.skipped = append(tr.skipped, i)
		tr.skipMask[i] = true
	}
}

// layersTouchRendered — какой-то из рисуемых тайлов задевает слой размытия
// (его выполнение прочитает соседние тайлы).
func (tr *tileRaster) layersTouchRendered() bool {
	if len(tr.layers) == 0 {
		return false
	}
	for _, i := range tr.tiles {
		tile := tr.canvas.TileRect(i)
		for _, l := range tr.layers {
			if l.pr.Overlaps(tile) {
				return true
			}
		}
	}
	return false
}

// unskip возвращает в растр все выбранные тайлы.
func (tr *tileRaster) unskip() {
	tr.tiles = append(tr.tiles[:0], tr.selected...)
	tr.skipped = tr.skipped[:0]
	clear(tr.skipMask)
}

// setTiles ставит v тайлам, задевающим физический прямоугольник r.
func (tr *tileRaster) setTiles(mask []bool, r image.Rectangle, v bool) {
	canvas := tr.canvas
	cols, rows := canvas.TileGrid()
	ts := output.TileSize
	r = r.Intersect(canvas.Frame())
	if r.Empty() {
		return
	}
	tx0, ty0 := r.Min.X/ts, r.Min.Y/ts
	tx1, ty1 := (r.Max.X-1)/ts, (r.Max.Y-1)/ts
	if tx1 >= cols {
		tx1 = cols - 1
	}
	if ty1 >= rows {
		ty1 = rows - 1
	}
	for ty := ty0; ty <= ty1; ty++ {
		row := ty * cols
		for tx := tx0; tx <= tx1; tx++ {
			mask[row+tx] = v
		}
	}
}

// anySkipped — среди тайлов, задевающих физический r, есть пропущенные.
func (tr *tileRaster) anySkipped(r image.Rectangle) bool {
	canvas := tr.canvas
	cols, rows := canvas.TileGrid()
	ts := output.TileSize
	r = r.Intersect(canvas.Frame())
	if r.Empty() {
		return false
	}
	tx0, ty0 := r.Min.X/ts, r.Min.Y/ts
	tx1, ty1 := (r.Max.X-1)/ts, (r.Max.Y-1)/ts
	if tx1 >= cols {
		tx1 = cols - 1
	}
	if ty1 >= rows {
		ty1 = rows - 1
	}
	for ty := ty0; ty <= ty1; ty++ {
		row := ty * cols
		for tx := tx0; tx <= tx1; tx++ {
			if tr.skipMask[row+tx] {
				return true
			}
		}
	}
	return false
}

// noteOverlaid снимает действительность хэша у тайлов, поверх которых кадр
// рисовал после растра дерева (все или области — место отложенных слоёв,
// шагов переходов): их пиксели — не функция команд, следующий кадр их
// перерисует. Все тайлы области, а не только изменившиеся при сбросе окна
// back: слой, нарисовавший то же, что кадр назад, сброс не отличит от слоя,
// не тронувшего тайл, — а под ним в front уже не дерево.
func (tr *tileRaster) noteOverlaid(all bool, rects []image.Rectangle) {
	if tr.canvas == nil || len(tr.valid) == 0 {
		return
	}
	if all {
		clear(tr.valid)
		return
	}
	for _, r := range rects {
		tr.setTiles(tr.valid, r, false)
	}
}

// finishHash запоминает хэши нарисованных тайлов. В частичном кадре
// действительным становится только тайл, лежащий в повреждении целиком:
// тайл на кромке нарисован под клипом, а виджет, рисующий за своими
// границами (тень, ореол, текст шире bounds), за клипом остался прежним —
// полный кадр его дорисует, пропуск бы закрепил обрезанную картинку.
func (tr *tileRaster) finishHash(plan *framePlan) {
	canvas := tr.canvas
	for _, i := range tr.tiles {
		tr.prev[i] = tr.cur[i]
		tr.valid[i] = !tr.partial || canvas.TileRect(i).In(tr.damage)
	}
	if tr.partial {
		// Слой, выполненный в частичном кадре с областью чтения шире
		// повреждения, читал за его краем прошлый back — как на прямом пути,
		// но не так, как в полном кадре. Его тайлы перерисуются следующим
		// кадром, чтобы пропуск не закрепил эту разницу; попадание кэша
		// блитит растр и от кадра не зависит.
		for _, l := range tr.layers {
			if l.hit == nil && !l.src.In(tr.damage) {
				tr.setTiles(tr.valid, l.pr, false)
			}
		}
	}
}

// blitBackgroundFrame кладёт фон на весь back или на повреждение, сбросив
// признаки тайлов — ДО фона: фон помечает тайлы, которые накрыл, и сброс
// после него стёр бы эту пометку (тайл с одними обоями уезжал бы к
// потребителю с ответом «неизвестно что»).
func (e *Engine) blitBackgroundFrame(canvas *Canvas, partial bool, damage image.Rectangle) {
	if partial {
		canvas.ResetTileMarks(damage)
		canvas.BlitBackgroundIn(damage)
		return
	}
	canvas.ResetTileMarks(image.Rect(0, 0, canvas.W, canvas.H))
	canvas.BlitBackground()
}

