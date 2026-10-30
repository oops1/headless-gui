package raster

// singlebuf.go — один буфер кадра (ADR-0007, задача 4а.4).
//
// У холста движка один полноразмерный буфер — front, последний отданный
// кадр. Буфер рисования back — окно над front: image.RGBA с Rect в физических
// координатах кадра, выровненный по сетке тайлов, которое кадр задаёт под
// то, что рисует в холст сам (слои поверх растра дерева, прямой путь,
// воспроизведение одной горутиной), и сбрасывает в front по тайлам. Тайлы
// растра воркеры сравнивают с front прямо из своих окон (SyncTileFrom).
// Окно, которым кадры не пользуются backWindowIdle кадров, освобождается.
//
// Выключенный режим (SetSingleBuffer(false)) — окно всегда размером с кадр и
// не освобождается: прежняя модель двух буферов на той же реализации.
// Чужая память потребителя (SetExternalBack, AttachBack) окном не бывает.

import (
	"bytes"
	"image"
	"runtime"

	hgdraw "github.com/oops1/headless-gui/v4/core/draw"
	"github.com/oops1/headless-gui/v4/internal/workpool"
	"github.com/oops1/headless-gui/v4/output"
)

// backWindowIdle — через сколько кадров без окна back оно освобождается.
const backWindowIdle = 60

// SetSingleBuffer включает или выключает окно back над front. Включение
// освобождает собственный буфер рисования до первого окна; выключение
// заводит окно размером с кадр, равное front.
func (c *Canvas) SetSingleBuffer(on bool) {
	if c.single == on {
		return
	}
	c.single = on
	if c.isWindow {
		return
	}
	n := c.tilesX * c.tilesY
	if len(c.touched) != n {
		c.touched, c.changed = make([]bool, n), make([]bool, n)
	}
	if on {
		c.releaseBackWindow()
		return
	}
	c.backClean = false
	c.SetBackWindow(c.Frame())
}

// SingleBuffer сообщает, окно ли back над front.
func (c *Canvas) SingleBuffer() bool { return c.single }

// BackWindow — текущее окно собственного буфера рисования (физические
// координаты кадра); пустое — буфер освобождён или не задан. У холста с
// полным буфером — весь кадр.
func (c *Canvas) BackWindow() image.Rectangle {
	if c.backOwn == nil {
		return image.Rectangle{}
	}
	return c.backOwn.Rect
}

// BufferBytes — байт в собственных буферах кадра холста: front и окно back.
func (c *Canvas) BufferBytes() int {
	n := 0
	if c.front != nil {
		n += len(c.front.Pix)
	}
	if c.backOwn != nil {
		n += len(c.backOwn.Pix)
	}
	return n
}

// alignTiles — r, расширенный до границ тайлов и зажатый кадром.
func (c *Canvas) alignTiles(r image.Rectangle) image.Rectangle {
	r = r.Intersect(c.Frame())
	if r.Empty() {
		return image.Rectangle{}
	}
	ts := output.TileSize
	return image.Rect(r.Min.X/ts*ts, r.Min.Y/ts*ts, (r.Max.X+ts-1)/ts*ts, (r.Max.Y+ts-1)/ts*ts).Intersect(c.Frame())
}

// backShrinkRatio — во сколько раз чистое окно должно быть больше нужного,
// чтобы ужаться до него (иначе окно размером с кадр после полного кадра
// оставалось бы под подсказкой навсегда).
const backShrinkRatio = 4

// SetBackWindow задаёт окно собственного буфера рисования под r (физические
// координаты кадра; выравнивается по тайлам). Окно, уже накрывающее r,
// остаётся: его содержимое либо равно front (чистое), либо нарисовано в этом
// кадре и ещё не сброшено; чистое окно много больше r при одном буфере
// ужимается до r (при двух буферах окно — всегда кадр). Иначе
// окно переводится на r (объединяется с ненарисованным содержимым этого
// кадра — буфер перекладывается на месте, когда вмещает) и заполняется из
// front. Что в окне рисуют, вызывающие отмечают сами (Touch, TouchTile):
// префилл равен front, сравнивать его незачем. При чужой памяти
// потребителя — ничего не делает.
func (c *Canvas) SetBackWindow(r image.Rectangle) {
	if c.isWindow || c.back != c.backOwn {
		return
	}
	c.backUsed = true
	if !c.single {
		r = c.Frame() // два буфера: окно — всегда кадр (свежий холст, возврат из чужой памяти)
	}
	r = c.alignTiles(r)
	if r.Empty() {
		return
	}
	cur := c.backOwn.Rect
	if r.In(cur) && (c.backLive || c.backClean && (!c.single || cur.Dx()*cur.Dy() <= backShrinkRatio*r.Dx()*r.Dy())) {
		return
	}
	live := c.backLive && !cur.Empty()
	if live {
		r = c.alignTiles(r.Union(cur).Intersect(c.Frame()))
	}
	w, h := r.Dx(), r.Dy()
	need := w * h * 4
	nb := &image.RGBA{Rect: r, Stride: w * 4}
	if cap(c.backOwn.Pix) >= need {
		nb.Pix = c.backOwn.Pix[:need]
	} else {
		nb.Pix = make([]byte, need)
	}
	if live && &nb.Pix[0] == &c.backOwn.Pix[0] {
		// Живое содержимое перекладывается на месте: новая строка лежит не
		// раньше старой (окно растёт влево и вверх, шаг строки не меньше), и
		// обход с последней строки не затирает ещё не перенесённые.
		old := image.RGBA{Pix: c.backOwn.Pix, Stride: c.backOwn.Stride, Rect: cur}
		n := cur.Dx() * 4
		for y := cur.Max.Y - 1; y >= cur.Min.Y; y-- {
			copy(nb.Pix[nb.PixOffset(cur.Min.X, y):][:n], old.Pix[old.PixOffset(cur.Min.X, y):][:n])
		}
	} else if live {
		old := c.backOwn
		n := cur.Dx() * 4
		for y := cur.Min.Y; y < cur.Max.Y; y++ {
			copy(nb.Pix[nb.PixOffset(cur.Min.X, y):][:n], old.Pix[old.PixOffset(cur.Min.X, y):][:n])
		}
	}
	// Заполнение из front: там лежит прошлый кадр — то, что хранил бы
	// полноразмерный back вне перерисовки; живое содержимое не трогается.
	parts := []image.Rectangle{r}
	if live {
		parts = outsideOf(r, cur)
	}
	for _, p := range parts {
		n := p.Dx() * 4
		for y := p.Min.Y; y < p.Max.Y; y++ {
			copy(nb.Pix[nb.PixOffset(p.Min.X, y):][:n], c.front.Pix[c.front.PixOffset(p.Min.X, y):][:n])
		}
	}
	c.backOwn = nb
	c.back = nb
	c.backClean = !live
}

// outsideOf — части r вне skip: до четырёх полос.
func outsideOf(r, skip image.Rectangle) []image.Rectangle {
	if r.Empty() {
		return nil
	}
	s := r.Intersect(skip)
	if s.Empty() {
		return []image.Rectangle{r}
	}
	var out []image.Rectangle
	for _, q := range [...]image.Rectangle{
		image.Rect(r.Min.X, r.Min.Y, r.Max.X, s.Min.Y),
		image.Rect(r.Min.X, s.Max.Y, r.Max.X, r.Max.Y),
		image.Rect(r.Min.X, s.Min.Y, s.Min.X, s.Max.Y),
		image.Rect(s.Max.X, s.Min.Y, r.Max.X, s.Max.Y),
	} {
		if !q.Empty() {
			out = append(out, q)
		}
	}
	return out
}

// InvalidateBackWindow объявляет окно back не равным front: front изменится
// мимо окна (воркеры пишут тайлы в front, пока окно этого кадра не задано),
// и следующее окно заполнится из front заново.
func (c *Canvas) InvalidateBackWindow() {
	if !c.backLive {
		c.backClean = false
	}
}

// releaseBackWindow освобождает собственный буфер рисования.
func (c *Canvas) releaseBackWindow() {
	if c.backOwn == nil {
		return
	}
	empty := &image.RGBA{Rect: image.Rectangle{}}
	if c.back == c.backOwn {
		c.back = empty
	}
	c.backOwn = empty
	c.backClean, c.backLive = false, false
	c.backIdle = 0
}

// EndFrameBuffers закрывает кадр у буферов: окно, не нужное
// backWindowIdle кадров подряд, освобождается (только в режиме одного
// буфера). Возвращает байты собственных буферов.
func (c *Canvas) EndFrameBuffers() int {
	if c.backUsed {
		c.backIdle = 0
	} else {
		c.backIdle++
		if c.single && c.backIdle > backWindowIdle && c.back == c.backOwn && !c.backOwn.Rect.Empty() {
			c.releaseBackWindow()
		}
	}
	c.backUsed = false
	return c.BufferBytes()
}

// Touch отмечает тайлы, задетые r (физический), нарисованными в окне back:
// сброс окна сравнит их с front.
func (c *Canvas) Touch(r image.Rectangle) {
	if c.isWindow || c.back != c.backOwn {
		return
	}
	c.touchTiles(r.Intersect(c.backOwn.Rect))
}

// TouchTile — Touch для тайла i сетки (из воркера: индексы разные, общих
// полей не трогает, гонок нет; сброс находит тронутые тайлы сам).
func (c *Canvas) TouchTile(i int) {
	if i >= 0 && i < len(c.touched) {
		c.touched[i] = true
	}
}

func (c *Canvas) touchTiles(r image.Rectangle) {
	tx0, ty0, tx1, ty1, ok := c.tileRange(r)
	if !ok || len(c.touched) == 0 {
		return
	}
	for ty := ty0; ty <= ty1; ty++ {
		row := ty * c.tilesX
		for tx := tx0; tx <= tx1; tx++ {
			c.touched[row+tx] = true
		}
	}
	c.backLive = true
}

// SyncTileFrom сравнивает область r (⊆ одного тайла) буфера src, лежащего в
// координатах кадра (окно воркера), с front; отличается — копирует в front и
// помечает тайл изменившимся. Воркеры зовут его для разных тайлов
// одновременно: строки front не пересекаются, индексы changed разные.
func (c *Canvas) SyncTileFrom(src *image.RGBA, r image.Rectangle) bool {
	r = r.Intersect(src.Rect).Intersect(c.front.Rect)
	if r.Empty() {
		return false
	}
	n := r.Dx() * 4
	same := true
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := src.PixOffset(r.Min.X, y)
		d := c.front.PixOffset(r.Min.X, y)
		if !bytes.Equal(src.Pix[s:s+n], c.front.Pix[d:d+n]) {
			same = false
			break
		}
	}
	if same {
		return false
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := src.PixOffset(r.Min.X, y)
		d := c.front.PixOffset(r.Min.X, y)
		copy(c.front.Pix[d:d+n], src.Pix[s:s+n])
	}
	ts := output.TileSize
	if i := (r.Min.Y/ts)*c.tilesX + r.Min.X/ts; i >= 0 && i < len(c.changed) {
		c.changed[i] = true
	}
	return true
}

// FlushBackWindow сбрасывает окно back в front: тронутые тайлы окна (при
// непустом rects — тайлы окна, задевающие одну из областей, тронуты они
// или нет: шаг перехода смешивается мимо клипа и пометок) сравниваются с
// front, отличающиеся копируются и помечаются изменившимися. Возвращает
// индексы тайлов, которые сброс изменил (срез живёт до следующего вызова).
// После сброса окно равно front в тронутых тайлах; тронутыми они быть
// перестают.
func (c *Canvas) FlushBackWindow(rects []image.Rectangle) []int {
	c.flushed = c.flushed[:0]
	if c.isWindow || c.back != c.backOwn || len(c.touched) == 0 {
		return nil
	}
	win := c.backOwn.Rect
	tx0, ty0, tx1, ty1, ok := c.tileRange(win)
	if !ok {
		c.backLive = false
		clear(c.touched)
		return nil
	}
	rows := ty1 - ty0 + 1
	total := 0
	for ty := ty0; ty <= ty1; ty++ {
		row := ty * c.tilesX
		for tx := tx0; tx <= tx1; tx++ {
			if c.flushCandidate(row+tx, tx, ty, rects) {
				total++
			}
		}
	}
	if total == 0 {
		c.backLive = false
		return nil
	}
	workers := runtime.GOMAXPROCS(0)
	if workers > rows {
		workers = rows
	}
	if c.noPool || workers <= 1 || total < 64 {
		c.flushRows(tx0, ty0, tx1, ty1, rects)
	} else {
		chunk := (rows + workers - 1) / workers
		parts := (rows + chunk - 1) / chunk
		j := &c.flushWork
		j.c, j.tx0, j.tx1, j.ty0, j.ty1, j.chunk, j.rects = c, tx0, tx1, ty0, ty1, chunk, rects
		workpool.Run(j, &j.batch, parts)
		j.rects = nil
	}
	for ty := ty0; ty <= ty1; ty++ {
		row := ty * c.tilesX
		for tx := tx0; tx <= tx1; tx++ {
			i := row + tx
			if c.touched[i] {
				c.touched[i] = false
				if c.flushMark[i] {
					c.flushMark[i] = false
					c.flushed = append(c.flushed, i)
				}
			}
		}
	}
	c.backLive = false
	c.backClean = true
	return c.flushed
}

// flushRows — сброс тронутых тайлов рядов [ty0..ty1]: сравнение окна с
// front и копирование в своих рядах (полосы пула не пересекаются).
func (c *Canvas) flushRows(tx0, ty0, tx1, ty1 int, rects []image.Rectangle) {
	ts := output.TileSize
	win := c.backOwn.Rect
	for ty := ty0; ty <= ty1; ty++ {
		row := ty * c.tilesX
		for tx := tx0; tx <= tx1; tx++ {
			i := row + tx
			if !c.flushCandidate(i, tx, ty, rects) {
				continue
			}
			tile := image.Rect(tx*ts, ty*ts, min(tx*ts+ts, c.W), min(ty*ts+ts, c.H)).Intersect(win)
			if tile.Empty() {
				continue
			}
			if c.syncFromBack(tile) {
				c.changed[i] = true
				c.flushMark[i] = true
			}
		}
	}
}

// flushCandidate — сравнивать ли тайл (i; tx, ty) при сбросе: по областям,
// если они заданы, иначе по пометке.
func (c *Canvas) flushCandidate(i, tx, ty int, rects []image.Rectangle) bool {
	if len(rects) == 0 {
		return c.touched[i]
	}
	ts := output.TileSize
	return overlapsAny(rects, image.Rect(tx*ts, ty*ts, min(tx*ts+ts, c.W), min(ty*ts+ts, c.H)))
}

// GrowBackWindowFor расширяет окно back холста движка на область чтения r
// (физическая): команда, читающая кадр за своим участком (размытая подложка
// отложенного оверлея), видит вне окна то, что хранил бы полный back —
// прошлый кадр из front. У окна воркера, двух буферов и чужой памяти —
// ничего не делает.
func (c *Canvas) GrowBackWindowFor(r image.Rectangle) {
	if c.isWindow || !c.single || c.back != c.backOwn || c.backOwn.Rect.Empty() {
		return
	}
	if r.Intersect(c.Frame()).In(c.backOwn.Rect) {
		return
	}
	c.SetBackWindow(r)
}

// syncFromBack сравнивает область r окна back с front и копирует при
// различии.
func (c *Canvas) syncFromBack(r image.Rectangle) bool {
	n := r.Dx() * 4
	same := true
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := c.backOwn.PixOffset(r.Min.X, y)
		d := c.front.PixOffset(r.Min.X, y)
		if !bytes.Equal(c.backOwn.Pix[s:s+n], c.front.Pix[d:d+n]) {
			same = false
			break
		}
	}
	if same {
		return false
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := c.backOwn.PixOffset(r.Min.X, y)
		d := c.front.PixOffset(r.Min.X, y)
		copy(c.front.Pix[d:d+n], c.backOwn.Pix[s:s+n])
	}
	return true
}

// flushJob — сброс окна back по полосам рядов (workpool).
type flushJob struct {
	c                  *Canvas
	tx0, tx1, ty0, ty1 int
	chunk              int
	rects              []image.Rectangle
	batch              workpool.Batch
}

func (j *flushJob) RunPart(i int) {
	r0 := j.ty0 + i*j.chunk
	r1 := min(r0+j.chunk-1, j.ty1)
	if r0 > r1 {
		return
	}
	j.c.flushRows(j.tx0, r0, j.tx1, r1, j.rects)
}

// ChangedTiles — тайлы, изменившиеся в этом кадре (воркеры и сброс окна), с
// данными из front одним слэбом; отметки снимаются.
func (c *Canvas) ChangedTiles() []output.DirtyTile {
	ts := output.TileSize
	var tiles []output.DirtyTile
	total := 0
	for i, ch := range c.changed {
		if !ch {
			continue
		}
		c.changed[i] = false
		px, py := (i%c.tilesX)*ts, (i/c.tilesX)*ts
		pw, ph := min(ts, c.W-px), min(ts, c.H-py)
		if pw <= 0 || ph <= 0 {
			continue
		}
		tiles = append(tiles, output.DirtyTile{X: px, Y: py, W: pw, H: ph})
		total += pw * ph * 4
	}
	return c.extractFront(tiles, total)
}

// AllTilesFront — все тайлы кадра с данными из front (полный кадр при одном
// буфере: front уже содержит кадр целиком); отметки изменившихся снимаются.
func (c *Canvas) AllTilesFront() []output.DirtyTile {
	clear(c.changed)
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
	return c.extractFront(tiles, total)
}

// extractFront — данные тайлов из front одним слэбом (без синхронизации:
// front уже содержит кадр).
func (c *Canvas) extractFront(tiles []output.DirtyTile, total int) []output.DirtyTile {
	if len(tiles) == 0 || c.noTileData {
		return tiles
	}
	slab := c.newSlab(total)
	for i := range tiles {
		t := &tiles[i]
		start := len(slab)
		rowBytes := t.W * 4
		for row := 0; row < t.H; row++ {
			src := c.front.PixOffset(t.X, t.Y+row)
			slab = append(slab, c.front.Pix[src:src+rowBytes]...)
		}
		t.Data = slab[start:len(slab):len(slab)]
	}
	return tiles
}

// RenderOffscreen — draw в отдельную картинку области r (логическая) на
// прозрачном фоне (hgdraw.OffscreenRenderer): своё окно переводится на
// область, очищается в прозрачный и отдаётся draw; кадр не трогается.
// Картинка — всегда RGBA с альфой: формат кадра (FormatBGRX у чужой памяти)
// окну не передаётся, иначе каналы были бы переставлены, а прозрачность
// потеряна. Область, не помещающаяся в кадр целиком, и холст-окно — nil.
func (c *Canvas) RenderOffscreen(r image.Rectangle, draw func(ctx hgdraw.DrawContext)) *image.RGBA {
	full := c.PhysRect(r)
	pr := full.Intersect(c.Frame())
	if pr.Empty() || pr != full || c.isWindow || draw == nil {
		return nil
	}
	w := c.offWin
	if w == nil {
		w = NewWindowCanvas(c)
		c.offWin = w
	}
	w.SyncFrame(c)
	if w.format != FormatRGBA {
		w.setOwnFormat(FormatRGBA)
		w.scaledCache, w.scaledPixels = nil, 0
	}
	w.Retarget(pr)
	w.SetCenter(pr)
	n := pr.Dx() * 4
	for y := pr.Min.Y; y < pr.Max.Y; y++ {
		s := w.back.PixOffset(pr.Min.X, y)
		clear(w.back.Pix[s : s+n])
	}
	draw(w)
	dst := image.NewRGBA(image.Rect(0, 0, pr.Dx(), pr.Dy()))
	for y := 0; y < pr.Dy(); y++ {
		s := w.back.PixOffset(pr.Min.X, pr.Min.Y+y)
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+n], w.back.Pix[s:s+n])
	}
	return dst
}

// RenderSnapshot отрисовывает область r (логическая) текущего кадра в окно
// холста воспроизведением draw и возвращает копию (физические пиксели от
// (0,0), premultiplied): Snapshot рекордера команд (ADR-0007) — панель под
// призраком DockManager снимается с записанных команд, а не с буфера.
func (c *Canvas) RenderSnapshot(r image.Rectangle, draw func(ctx hgdraw.DrawContext)) *image.RGBA {
	pr := c.PhysRect(r).Intersect(c.Frame())
	if pr.Empty() || c.isWindow {
		return nil
	}
	w := c.snapWin
	if w == nil {
		w = NewWindowCanvas(c)
		c.snapWin = w
	}
	w.SyncFrame(c)
	w.Retarget(pr)
	w.SetCenter(pr)
	w.BlitBackgroundIn(pr)
	draw(w)
	dst := image.NewRGBA(image.Rect(0, 0, pr.Dx(), pr.Dy()))
	n := pr.Dx() * 4
	for y := 0; y < pr.Dy(); y++ {
		s := w.back.PixOffset(pr.Min.X, pr.Min.Y+y)
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+n], w.back.Pix[s:s+n])
	}
	return dst
}

// ChangedCount — сколько тайлов помечено изменившимися (для тестов).
func (c *Canvas) ChangedCount() int {
	n := 0
	for _, ch := range c.changed {
		if ch {
			n++
		}
	}
	return n
}
