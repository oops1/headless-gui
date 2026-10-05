package engine

// fullframe.go — полный кадр, когда потребитель мог потерять предыдущие.
//
// Кадры — разности: каждый несёт только тайлы, изменившиеся с прошлого.
// Канал Frames() при переполнении выбрасывал кадр молча, а движок считал его
// доставленным — его front уже совпадал с выброшенным кадром, и дальше шли
// разности относительно картинки, которой у потребителя не было. Окно,
// отставшее на пару кадров во время смены размера, оставалось с раскладкой
// прежнего размера навсегда: перерисовка присылала только изменения поверх
// неё (задание WinLine, калькулятор, 05.10.2026).
//
// Теперь потеря помнится: как только в канале появляется место, следующий
// кадр уходит ПОЛНЫМ — все тайлы холста, без сравнения с front. То же после
// смены размера или масштаба: кадры прежнего размера из канала выбрасываются
// (класть их в буфер нового размера нельзя), а первый кадр нового — полный.
//
// Потребитель, который канал не читает вовсе (у него сток или вообще ничего),
// не платит ничего: места в его канале не бывает, полные кадры не рисуются.

import "github.com/oops1/headless-gui/v3/output"

// markCanvasReplaced — холст пересоздан (размер, масштаб): кадры прежнего
// размера в канале больше ни к чему, следующий кадр — полный.
//
// Вызывается под frameMu: рендер в этот момент не идёт и в канал не пишет.
func (e *Engine) markCanvasReplaced() {
	e.fullNext.Store(true)
	for {
		select {
		case <-e.frames:
			continue
		default:
		}
		return
	}
}

// frameRoom — есть ли место в канале кадров.
func (e *Engine) frameRoom() bool { return len(e.frames) < cap(e.frames) }

// fullFramePending — нужен полный кадр, и его есть куда отдать. Тикер рисует
// кадр по этому признаку даже без изменений в интерфейсе: иначе после потери
// статичный интерфейс так и остался бы у потребителя неверным.
func (e *Engine) fullFramePending() bool {
	return e.fullNext.Load() || (e.lostFrame.Load() && e.frameRoom())
}

// takeFullFrame решает, будет ли этот кадр полным, и снимает признак.
func (e *Engine) takeFullFrame() bool {
	if e.fullNext.Swap(false) {
		e.lostFrame.Store(false)
		return true
	}
	if e.lostFrame.Load() && e.frameRoom() {
		e.lostFrame.Store(false)
		return true
	}
	return false
}

// allTiles извлекает ВСЕ тайлы холста и синхронизирует front — полный кадр.
// Устроено как diffTileRows, только без сравнения: одна аллокация на всё.
func (c *Canvas) allTiles() []output.DirtyTile {
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
	slab := make([]byte, 0, total)
	for i := range tiles {
		t := &tiles[i]
		start := len(slab)
		slab = c.appendTile(slab, t.X, t.Y, t.W, t.H)
		t.Data = slab[start:len(slab):len(slab)]
		c.syncTile(t.X, t.Y, t.W, t.H)
	}
	return tiles
}
