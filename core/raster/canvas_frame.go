package raster

// canvas_frame.go — то, что холст отдаёт в кадр: все тайлы полного кадра и их
// буферы (fullframe.go), переносы областей в переднем буфере (moveapply.go),
// разметка тайлов видео (video.go). Решает, когда это нужно, движок.

import (
	"image"

	"github.com/oops1/headless-gui/v3/output"
)

// AllTiles извлекает ВСЕ тайлы холста и синхронизирует front — полный кадр.
// Устроено как diffTileRows, только без сравнения: одна аллокация на всё.
func (c *Canvas) AllTiles() []output.DirtyTile {
	if c.noTileData {
		return c.allTilesNoData()
	}
	ts := output.TileSize
	tiles := make([]output.DirtyTile, 0, c.tilesX*c.tilesY)
	total := 0
	for ty := 0; ty < c.tilesY; ty++ {
		for tx := 0; tx < c.tilesX; tx++ {
			px, py := tx*ts, ty*ts
			pw, ph := min(ts, c.W-px), min(ts, c.H-py)
			if pw <= 0 || ph <= 0 {
				continue
			}
			tiles = append(tiles, output.DirtyTile{X: px, Y: py, W: pw, H: ph})
			total += pw * ph * 4
		}
	}
	slab := c.newSlab(total)
	for i := range tiles {
		t := &tiles[i]
		start := len(slab)
		slab = c.appendTile(slab, t.X, t.Y, t.W, t.H)
		t.Data = slab[start:len(slab):len(slab)]
		c.syncTile(t.X, t.Y, t.W, t.H)
	}
	return tiles
}

// newSlab даёт буфер длины 0 и ёмкости total под данные тайлов кадра. При
// включённой переработке (Engine.SetFrameRecycling) он берётся из пула
// output.GetSlab и записывается в c.slabs — кадр заберёт его через TakeSlabs
// и вернёт в пул по Frame.Release; иначе это обычная новая аллокация, как
// было всегда. Память из пула не обнулена: appendTile переписывает каждый
// байт, который попадает в Data.
func (c *Canvas) newSlab(total int) []byte {
	if !c.recycle {
		return make([]byte, 0, total)
	}
	s := output.GetSlab(total)
	c.slabMu.Lock()
	c.slabs = append(c.slabs, s)
	c.slabMu.Unlock()
	return s.Bytes()
}

// TakeSlabs отдаёт буферы, выданные newSlab с прошлого вызова.
func (c *Canvas) TakeSlabs() []*output.Slab {
	c.slabMu.Lock()
	s := c.slabs
	c.slabs = nil
	c.slabMu.Unlock()
	return s
}

// allTilesNoData — AllTiles без байтов тайлов (SetFrameTileData(false)).
func (c *Canvas) allTilesNoData() []output.DirtyTile {
	ts := output.TileSize
	tiles := make([]output.DirtyTile, 0, c.tilesX*c.tilesY)
	for ty := 0; ty < c.tilesY; ty++ {
		for tx := 0; tx < c.tilesX; tx++ {
			px, py := tx*ts, ty*ts
			pw, ph := min(ts, c.W-px), min(ts, c.H-py)
			if pw <= 0 || ph <= 0 {
				continue
			}
			tiles = append(tiles, output.DirtyTile{X: px, Y: py, W: pw, H: ph})
			c.syncTile(px, py, pw, ph)
		}
	}
	return tiles
}

// ApplyMovesToFront выполняет переносы внутри переднего буфера.
//
// Координаты объявлений — ЛОГИЧЕСКИЕ (виджеты живут в них), буфер —
// физический, поэтому каждая область масштабируется. Перенос, выходящий за
// холст, обрезается по нему: копировать из-за края нечего.
//
// Возвращает применённые переносы в логических координатах — в кадр уходят
// только они. Отброшенное объявление в кадре было бы ложью: потребитель
// скопировал бы то, чего движок у себя не копировал.
func (c *Canvas) ApplyMovesToFront(moves []output.MoveRegion) []output.MoveRegion {
	if len(moves) == 0 || c.front == nil {
		return nil
	}
	out := make([]output.MoveRegion, 0, len(moves))
	bounds := image.Rect(0, 0, c.W, c.H)
	// taken — области, уже занятые применёнными переносами (и источники, и
	// назначения) в физических координатах. См. проверку пересечения ниже.
	taken := make([]image.Rectangle, 0, 2*len(moves))

	for _, m := range moves {
		dst := c.PhysRect(m.Rect).Intersect(bounds)
		if dst.Empty() {
			continue
		}
		// Источник сдвигается ровно на ту же величину, что и назначение:
		// перенос не меняет размера, иначе это уже масштабирование.
		delta := dst.Min.Sub(c.PhysRect(m.Rect).Min)
		src := c.PhysRect(image.Rect(m.From.X, m.From.Y,
			m.From.X+m.Rect.Dx(), m.From.Y+m.Rect.Dy())).Add(delta)
		src = src.Intersect(bounds)
		if src.Empty() || src.Dx() != dst.Dx() || src.Dy() != dst.Dy() {
			// Обрезка развела источник и назначение — переносить нечего:
			// пусть область уедет обычными тайлами.
			continue
		}
		if src.Min == dst.Min {
			continue
		}
		// Переносы, задевающие уже перенесённое, дальше не идут.
		//
		// Движок применяет их к своему буферу подряд, поэтому второй читает
		// пиксели, которые первый уже переложил. Потребитель обязан повторить
		// ровно тот же порядок — а он этого не обязан уметь: команда копии
		// поверхности в RDP выполняется когда угодно относительно соседних, и
		// локальный потребитель вправе применить их разом. Разошедшийся
		// порядок даёт копию с неверным источником — прямоугольник чужого
		// содержимого с резким швом.
		//
		// Поэтому в кадр уходят только непересекающиеся переносы: их можно
		// применять в любом порядке и получить один и тот же результат.
		// Остальное уезжает обычными тайлами — дороже, но верно.
		if overlapsAny(taken, src) || overlapsAny(taken, dst) {
			continue
		}
		blitWithin(c.front, src, dst.Min)
		taken = append(taken, src, dst)
		out = append(out, output.MoveRegion{From: m.From, Rect: m.Rect})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// overlapsAny сообщает, задевает ли r хоть одну из областей.
func overlapsAny(rects []image.Rectangle, r image.Rectangle) bool {
	for _, o := range rects {
		if r.Overlaps(o) {
			return true
		}
	}
	return false
}

// blitWithin копирует прямоугольник внутри одного изображения.
//
// Строки идут в порядке, зависящем от направления сдвига: при перекрытии
// областей копирование сверху вниз затёрло бы ещё не прочитанные строки —
// та же осторожность, что у memmove.
func blitWithin(img *image.RGBA, src image.Rectangle, dstMin image.Point) {
	w := src.Dx() * 4
	if w <= 0 {
		return
	}
	rows := src.Dy()
	down := dstMin.Y > src.Min.Y

	for i := 0; i < rows; i++ {
		y := i
		if down {
			y = rows - 1 - i // сверху вниз: идём с конца, чтобы не затереть
		}
		so := img.PixOffset(src.Min.X, src.Min.Y+y)
		do := img.PixOffset(dstMin.X, dstMin.Y+y)
		copy(img.Pix[do:do+w], img.Pix[so:so+w])
	}
}

// MarkVideo помечает видео тайлы, целиком лежащие в r (физический) и не
// задетые покрытием. Тайлы на краю окна или под чем-то остаются с тем, что
// нарисовано.
func (c *Canvas) MarkVideo(r image.Rectangle, cover []image.Rectangle, id uint32) {
	tx0, ty0, tx1, ty1, ok := c.tileRange(r)
	if !ok {
		return
	}
	ts := output.TileSize
	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
			tile := image.Rect(tx*ts, ty*ts, min(tx*ts+ts, c.W), min(ty*ts+ts, c.H))
			if !tile.In(r) || overlapsAny(cover, tile) {
				continue
			}
			m := &c.marks[ty*c.tilesX+tx]
			m.touched, m.kind, m.video = true, output.RegionVideo, id
		}
	}
}
