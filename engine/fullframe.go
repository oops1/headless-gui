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
		case fr := <-e.frames:
			fr.Release() // кадр прежнего размера не увидит никто
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
// output.GetSlab и записывается в c.slabs — кадр заберёт его через takeSlabs
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

// takeSlabs отдаёт буферы, выданные newSlab с прошлого вызова.
func (c *Canvas) takeSlabs() []*output.Slab {
	c.slabMu.Lock()
	s := c.slabs
	c.slabs = nil
	c.slabMu.Unlock()
	return s
}

// SetFrameRecycling включает переработку буферов данных тайлов: вместо нового
// слэба (в 4K при смене всего окна — 33 МБ на кадр) движок берёт буфер из пула,
// а потребитель возвращает его вызовом Frame.Release. По умолчанию выключено —
// Data кадра принадлежит потребителю навсегда, как раньше.
//
// Включая, потребитель принимает контракт: Data тайлов кадра действительна до
// Frame.Release (после — читать нельзя); не вызывать Release допустимо, буфер
// тогда достанется сборщику мусора. Кадры, которые движок сам выбрасывает из
// переполненного канала, он возвращает в пул сам. Кадры, увиденные
// синхронным стоком (SetFrameSink), в пул не возвращаются никогда: что сток
// сделал с Data, движку неизвестно.
//
// Окно (window) включает переработку само и зовёт Release после наложения
// кадра на свой буфер.
func (e *Engine) SetFrameRecycling(on bool) { e.recycleFrames.Store(on) }

// FrameRecycling сообщает, включена ли переработка буферов кадров.
func (e *Engine) FrameRecycling() bool { return e.recycleFrames.Load() }

// SetFrameTileData задаёт, нести ли тайлам кадра свои байты (Data). По
// умолчанию несут. Потребитель, которому движок рисует прямо в его буферы
// (SetSurface, SetSurfaces), читает пиксели оттуда, и копия каждого
// изменившегося тайла в кадр ему не нужна: без неё тайлы — только места
// изменений, Data — nil.
func (e *Engine) SetFrameTileData(on bool) { e.noTileData.Store(!on) }

// FrameTileData сообщает, несут ли тайлы кадра свои байты.
func (e *Engine) FrameTileData() bool { return !e.noTileData.Load() }

// allTilesNoData — allTiles без байтов тайлов (SetFrameTileData(false)).
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
