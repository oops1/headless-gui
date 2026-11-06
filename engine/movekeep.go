package engine

// movekeep.go — объявленный перенос не перерисовывает окно программы.
//
// Перенос (widget.NotifyMove) применяется к переднему буферу до растра
// (moveapply.go): потребителю уходит команда копии, а дифф оставляет только
// расхождения. Но сам кадр по-прежнему рисовал переехавшее окно целиком — на
// каждом шаге перетаскивания, чтобы убедиться, что получилось то же самое.
// Не отправлялось, а рисовалось и сравнивалось: главный остаток цены
// перетаскивания на сцене (пилот WinLine, ENGINE_ISSUES «A declared move is
// applied to front only»).
//
// Перенос — подсказка, а не обещание: движок не вправе поверить, что на новом
// месте лежит то, что нарисовал бы кадр. Поэтому тайл не рисуется, только
// когда это доказано. Доказать удаётся для окна программы — внешней
// поверхности (widget.ExternalSurface): её пиксели — буфер приложения один в
// один, без округлений масштаба и без шрифтов, так что сдвинутая картинка
// равна нарисованной заново. Тайл остаётся как есть после переноса, если:
//
//  1. сейчас он целиком лежит в назначении переноса и в видимой непрозрачной
//     части поверхности, и поверхность — последнее, что рисует в этом тайле
//     (над окном ничего нет, под ним — не важно);
//  2. то же было верно в кадре, который рисовал источник: тайлы под
//     источником действительны (их пиксели — функция команд) и верхней в них
//     была та же поверхность;
//  3. поверхность с тех пор не менялась (версия содержимого, буфер,
//     непрозрачность, скруглённый клип) и сдвинулась ровно на сдвиг переноса.
//
// Иначе тайл рисуется, как раньше. Обычные виджеты (текст, рамки) сюда не
// попадают: их растр зависит от места на дробном масштабе, и доказывать его
// неизменность — отдельная работа.

import (
	"image"
	"math"

	hgdraw "github.com/oops1/headless-gui/v4/core/draw"
	hgraster "github.com/oops1/headless-gui/v4/core/raster"
	"github.com/oops1/headless-gui/v4/output"
	"github.com/oops1/headless-gui/v4/widget"
)

// SetMoveReuse включает или выключает пропуск растра окон программ после
// объявленного переноса (по умолчанию включён). Выключенный — кадр с
// переносом рисует всё задетое им, как до 4.3. Картинка на обоих путях одна и
// та же.
func (e *Engine) SetMoveReuse(on bool) {
	e.moveReuse.Store(on)
	e.Invalidate()
}

// MoveReuse сообщает, включён ли пропуск растра после переноса.
func (e *Engine) MoveReuse() bool { return e.moveReuse.Load() }

// surfaceStamper — виджет, отдающий снимок своей поверхности
// (widget.ExternalSurface).
type surfaceStamper interface {
	Stamp() widget.SurfaceStamp
}

// surfSeen — поверхность, как её видел кадр: всё в физических пикселях.
type surfSeen struct {
	id     uint32
	rev    uint64
	buf    image.Rectangle
	opaque bool
	solid  []image.Rectangle // непрозрачные части буфера (координаты буфера)
	// origin — где лежит пиксель buf.Min; clip — клип команды в кадре; area —
	// видимая часть буфера (clip ∩ место буфера); round, rad — скруглённый
	// клип команды (round пуст — нет).
	origin image.Point
	clip   image.Rectangle
	area   image.Rectangle
	round  image.Rectangle
	rad    int
	// frame — кадр, видевший поверхность последним; twice — в кадре у
	// поверхности больше одной команды (доказывать нечего).
	frame uint64
	twice bool
}

// beginSurfaces готовит учёт поверхностей кадра.
func (tr *tileRaster) beginSurfaces() {
	tr.now = tr.now[:0]
	clear(tr.nowAt)
}

// noteSurface запоминает поверхность команды прямого виджета и возвращает её
// номер; 0 — команда не поверхность или доказывать для неё нечего.
func (tr *tileRaster) noteSurface(c *hgdraw.Cmd) uint32 {
	st, ok := c.Holder.(surfaceStamper)
	if !ok || c.X != 0 || c.Y != 0 || c.Clip == nil {
		return 0 // под сдвигом контекста место поверхности — не её границы
	}
	w, ok := c.Holder.(widget.Widget)
	if !ok {
		return 0
	}
	s := st.Stamp()
	if s.Buf.Empty() || s.ID == 0 {
		return 0
	}
	if i, seen := tr.nowAt[s.ID]; seen {
		tr.now[i].twice = true
		return 0
	}
	canvas := tr.canvas
	b := w.Bounds()
	pb := canvas.PhysRect(b)
	origin := pb.Min
	place := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(min(s.Buf.Dx(), pb.Dx()), min(s.Buf.Dy(), pb.Dy())))}
	clip := canvas.Frame()
	if !c.Clip.NoClip {
		clip = clip.Intersect(canvas.PhysRect(c.Clip.Clip))
	}
	seen := surfSeen{id: s.ID, rev: s.Rev, buf: s.Buf, opaque: s.Opaque, solid: s.Solid, origin: origin}
	if c.Clip.Round != (image.Rectangle{}) && c.Clip.RoundR > 0 {
		// Те же расчёты, что у Canvas.SetRoundClip.
		seen.round = canvas.PhysRect(c.Clip.Round)
		clip = clip.Intersect(seen.round)
		rad := c.Clip.RoundR
		if k := canvas.Scale(); k != 1 {
			rad = max(int(math.Round(float64(rad)*k)), 1)
		}
		seen.rad = min(rad, seen.round.Dx()/2, seen.round.Dy()/2)
		if seen.rad <= 0 {
			seen.round, seen.rad = image.Rectangle{}, 0
		}
	}
	seen.clip = clip
	seen.area = clip.Intersect(place)
	tr.nowAt[s.ID] = len(tr.now)
	tr.now = append(tr.now, seen)
	return s.ID
}

// keepSurfaces переносит поверхности кадра в память между кадрами: передний
// буфер теперь соответствует им. Полный кадр видел все поверхности — записи
// остальных (окна закрыты) убираются.
func (tr *tileRaster) keepSurfaces() {
	for i := range tr.now {
		s := tr.now[i]
		if s.twice {
			delete(tr.surfs, s.id)
			continue
		}
		s.frame = tr.frameNo
		tr.surfs[s.id] = s
	}
	if !tr.partial {
		for id, s := range tr.surfs {
			if s.frame != tr.frameNo {
				delete(tr.surfs, id)
			}
		}
	}
}

// moveFor — перенос кадра для тайла: m — тот, в чьём назначении тайл лежит
// целиком (nil — такого нет); hit — тайл задет назначением хоть одного
// переноса, то есть его пиксели в переднем буфере уже не те, что были.
func moveFor(moves []hgraster.AppliedMove, tile image.Rectangle) (m *hgraster.AppliedMove, hit bool) {
	for i := range moves {
		if !tile.Overlaps(moves[i].Dst) {
			continue
		}
		if hit {
			return nil, true // два назначения в одном тайле — рисуем
		}
		hit = true
		if tile.In(moves[i].Dst) {
			m = &moves[i]
		}
	}
	return m, hit
}

// moveKeeps — доказано ли, что тайл i после переноса m уже содержит то, что
// нарисовал бы кадр (условия — в шапке файла).
func (tr *tileRaster) moveKeeps(i int, tile image.Rectangle, m *hgraster.AppliedMove) bool {
	id := tr.curTop[i]
	if id == 0 {
		return false
	}
	at, ok := tr.nowAt[id]
	if !ok {
		return false
	}
	s := &tr.now[at]
	old, ok := tr.surfs[id]
	if !ok || s.twice {
		return false
	}
	d := m.Dst.Min.Sub(m.Src.Min)
	// 3. Та же поверхность, сдвинутая ровно на сдвиг переноса. Буфер и
	// непрозрачность меняются только вместе с версией; их сверка — на случай
	// снимка, заставшего смену буфера на полпути (версия ещё прежняя).
	if s.origin.Sub(old.origin) != d || s.rev != old.rev || s.buf != old.buf || s.opaque != old.opaque {
		return false
	}
	if s.rad != old.rad || s.round.Empty() != old.round.Empty() ||
		(!s.round.Empty() && s.round != old.round.Add(d)) {
		return false
	}
	// 1. Сейчас: тайл целиком в видимой непрозрачной части поверхности, мимо
	// скруглённых углов и не под слоями после растра.
	if !tile.In(s.area) || !inRoundCross(tile, s.round, s.rad) {
		return false
	}
	if !s.opaque && !solidCovers(s, tile) {
		return false
	}
	// Под слоями после растра (переходы оверлеев) тайл рисуется всегда, как
	// у хэш-диффа: перестраховка — окно back там заполняется из front, и
	// оставленный тайл дал бы тот же кадр.
	if e := tr.e; len(e.post) > 0 && tile.Overlaps(e.postRect) {
		return false
	}
	// 2. Тогда: источник был виден целиком, тайлы под ним действительны, и
	// верхней в них была эта поверхность.
	src := tile.Sub(d)
	if !src.In(old.clip) {
		return false
	}
	canvas := tr.canvas
	cols, _ := canvas.TileGrid()
	ts := output.TileSize
	for ty := src.Min.Y / ts; ty <= (src.Max.Y-1)/ts; ty++ {
		for tx := src.Min.X / ts; tx <= (src.Max.X-1)/ts; tx++ {
			u := ty*cols + tx
			if u < 0 || u >= len(tr.valid) || !tr.valid[u] || tr.prevTop[u] != id {
				return false
			}
		}
	}
	return true
}

// inRoundCross — лежит ли тайл в части скруглённого клипа, не задетой углами:
// в полосе на всю ширину без строк скругления или в полосе на всю высоту без
// его столбцов. Без клипа — да.
func inRoundCross(tile, round image.Rectangle, rad int) bool {
	if round.Empty() {
		return true
	}
	rows := image.Rect(round.Min.X, round.Min.Y+rad, round.Max.X, round.Max.Y-rad)
	cols := image.Rect(round.Min.X+rad, round.Min.Y, round.Max.X-rad, round.Max.Y)
	return tile.In(rows) || tile.In(cols)
}

// solidCovers — лежит ли тайл целиком в одной из непрозрачных частей буфера
// поверхности.
func solidCovers(s *surfSeen, tile image.Rectangle) bool {
	off := s.origin.Sub(s.buf.Min)
	for _, r := range s.solid {
		if tile.In(r.Intersect(s.buf).Add(off)) {
			return true
		}
	}
	return false
}

// dropMovedUnselected снимает действительность хэша с тайлов назначения
// переноса, которые кадр не выбрал: перенос их пиксели уже сменил, а
// повреждения под ними не заявлено (объявление переноса без инвалидации —
// нарушение контракта NotifyMove). Следующий кадр, задевший их, нарисует.
func (tr *tileRaster) dropMovedUnselected(moves []hgraster.AppliedMove) {
	canvas := tr.canvas
	cols, rows := canvas.TileGrid()
	ts := output.TileSize
	for i := range moves {
		r := moves[i].Dst.Intersect(canvas.Frame())
		if r.Empty() {
			continue
		}
		for ty := r.Min.Y / ts; ty <= (r.Max.Y-1)/ts && ty < rows; ty++ {
			for tx := r.Min.X / ts; tx <= (r.Max.X-1)/ts && tx < cols; tx++ {
				if !canvas.TileRect(ty*cols + tx).In(tr.selRect) {
					tr.valid[ty*cols+tx] = false
				}
			}
		}
	}
}
