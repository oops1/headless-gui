package engine

// surfaces.go — несколько буферов вывода (ADR-0002, 2.4).
//
// SetSurface даёт движку одну чужую память: он рисует в неё, потребитель
// читает. Но потребитель с двойной буферизацией — tilebus у WinLine, DRM с
// двумя кадрами — читает один буфер, пока движок рисует другой, и возвращает
// его подтверждением. До сих пор сессия WinLine держала такие буферы сама и
// копировала в них кадр из своего мастера, накапливая на буфер то, что
// менялось, пока он был у демона.
//
// Теперь это делает движок. Кадр N рисуется в свободный буфер; перед
// рисованием в него копируется из front всё, что изменилось с тех пор, как
// этот буфер был задним в последний раз (накопленный damage на буфер), и
// буфер становится точной копией прошлого кадра. Дальше — обычный частичный
// кадр и дифф. Буфер, не возвращённый ReleaseSurface, не берётся: при
// внешнем темпе кадр ждёт следующего RequestFrame, при тикерном —
// пропускается (Stats.FramesSkipped), и накопленный damage не теряется.
//
// Один буфер — это прежний SetSurface: ни очереди, ни подтверждений.

import (
	"fmt"
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/output"
)

// maxSurfaces — сколько буферов принимает SetSurfaces. Больше четырёх
// потребителю не нужно: три — уже запас на медленное подтверждение.
const maxSurfaces = 4

// maxPendingRects — сколько прямоугольников копит буфер, пока он у
// потребителя; дальше они сливаются в один объемлющий.
const maxPendingRects = 64

// surfacePending — что скопировать из front в буфер, прежде чем рисовать в
// него: всё (all) или список областей в физических пикселях.
type surfacePending struct {
	all   bool
	rects []image.Rectangle
}

func (p *surfacePending) add(r image.Rectangle) {
	if p.all || r.Empty() {
		return
	}
	if len(p.rects) >= maxPendingRects {
		u := r
		for _, q := range p.rects {
			u = u.Union(q)
		}
		p.rects = append(p.rects[:0], u)
		return
	}
	p.rects = append(p.rects, r)
}

func (p *surfacePending) reset() {
	p.all = false
	p.rects = p.rects[:0]
}

// surfaceSet — буферы SetSurfaces и их состояние.
type surfaceSet struct {
	mu      sync.Mutex
	imgs    []*image.RGBA // обёртки буферов; nil — набора нет
	format  PixelFormat
	w, h    int // физический размер холста, под который набор задан
	owned   []bool
	pending []surfacePending
	last    int  // буфер последнего отданного кадра; -1 — ещё не отдавали
	starved bool // кадр пропущен: ни одного свободного буфера
}

func (s *surfaceSet) active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.imgs) > 0
}

// drop забывает набор: холст пересоздан (SetResolution, SetScale) или
// потребитель задал другой буфер.
func (s *surfaceSet) drop() {
	s.mu.Lock()
	s.imgs, s.owned, s.pending = nil, nil, nil
	s.last, s.starved = -1, false
	s.mu.Unlock()
}

// SetSurfaces задаёт 2–4 чужих буфера вывода одного размера и формата;
// движок чередует их, отдавая номер буфера в Frame.Surface, и не рисует в
// буфер, пока потребитель не вернёт его ReleaseSurface. Один буфер — то же,
// что SetSurface. nil — вернуться к собственному буферу.
//
// Все буферы обязаны вмещать холст при данном stride, как в SetSurface. Смена
// разрешения или масштаба сбрасывает набор: холст пересоздаётся, и буферы
// прежнего размера ему не подходят — задайте их заново.
func (e *Engine) SetSurfaces(bufs [][]byte, stride int, f PixelFormat) error {
	switch {
	case len(bufs) == 0:
		return e.SetSurface(nil, 0, f)
	case len(bufs) == 1:
		return e.SetSurface(bufs[0], stride, f)
	case len(bufs) > maxSurfaces:
		return fmt.Errorf("engine: SetSurfaces: %d буферов, не больше %d", len(bufs), maxSurfaces)
	}
	if f != FormatRGBA && f != FormatBGRX {
		return fmt.Errorf("engine: SetSurfaces: неизвестный формат %d", uint8(f))
	}
	e.frameMu.Lock()
	defer e.frameMu.Unlock()
	w, h := e.canvas.W, e.canvas.H
	for i, pix := range bufs {
		if err := checkSurface(pix, stride, w, h); err != nil {
			return fmt.Errorf("engine: SetSurfaces: буфер %d: %w", i, err)
		}
	}
	s := &e.surfaces
	s.mu.Lock()
	s.imgs = make([]*image.RGBA, len(bufs))
	for i, pix := range bufs {
		s.imgs[i] = &image.RGBA{Pix: pix, Stride: stride, Rect: image.Rect(0, 0, w, h)}
	}
	s.format, s.w, s.h = f, w, h
	s.owned = make([]bool, len(bufs))
	for i := range s.owned {
		s.owned[i] = true
	}
	s.pending = make([]surfacePending, len(bufs))
	s.last, s.starved = -1, false
	s.mu.Unlock()
	// До первого кадра холст рисует в собственный буфер; первый кадр в чужой
	// — полный, как у SetSurface: его байты не совпадают с front.
	e.canvas.useOwnBack()
	e.fullNext.Store(true)
	e.Invalidate()
	return nil
}

// ReleaseSurface возвращает буфер i (нумерация Frame.Surface, с единицы)
// движку: потребитель прочитал его. Если кадр ждал свободного буфера, он
// будет подготовлен: при внешнем темпе — по этому вызову, как RequestFrame.
func (e *Engine) ReleaseSurface(i int) {
	s := &e.surfaces
	s.mu.Lock()
	idx := i - 1
	if idx < 0 || idx >= len(s.owned) {
		s.mu.Unlock()
		return
	}
	s.owned[idx] = true
	wake := s.starved
	s.starved = false
	s.mu.Unlock()
	if !wake {
		return
	}
	if e.pacingIsExternal() {
		e.RequestFrame()
		return
	}
	// Тикерный темп: damage ждёт с прошлого раза, но поколение не менялось
	// — цикл решил бы, что рисовать нечего.
	e.invGen.Add(1)
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// Surfaces сообщает, сколько буферов задано SetSurfaces (0 — набора нет).
func (e *Engine) Surfaces() int {
	e.surfaces.mu.Lock()
	defer e.surfaces.mu.Unlock()
	return len(e.surfaces.imgs)
}

// acquireSurface в начале кадра выбирает свободный буфер, делает его задним
// и приводит к прошлому кадру. Возвращает номер буфера (с единицы) или 0,
// когда набора нет; ok=false — все буферы у потребителя. skipCopy — кадр
// будет полным, и копировать в буфер прошлый кадр незачем; redraw — что кадр
// перерисует наверняка, это тоже не копируется.
func (e *Engine) acquireSurface(canvas *Canvas, skipCopy bool, redraw image.Rectangle) (surface int, ok bool) {
	s := &e.surfaces
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.imgs) == 0 {
		return 0, true
	}
	if s.w != canvas.W || s.h != canvas.H {
		// Холст сменил размер, а набор не сбросили: буферы ему не подходят.
		return 0, true
	}
	n := len(s.imgs)
	idx := -1
	for k := 1; k <= n; k++ {
		i := (s.last + k) % n
		if s.owned[i] {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.starved = true
		return 0, false
	}
	back := s.imgs[idx]
	canvas.back, canvas.format = back, s.format
	if !skipCopy {
		copyFrontInto(canvas.front, back, &s.pending[idx], redraw)
	}
	s.pending[idx].reset()
	return idx + 1, true
}

// surfaceFrameDone — кадр нарисован в буфер surface и отдан (или не отдан:
// пустой кадр потребитель не увидит, и буфер остаётся у движка). Остальным
// буферам записывается, что в них устарело.
func (e *Engine) surfaceFrameDone(surface int, fr *output.Frame, full bool) {
	if surface == 0 {
		return
	}
	s := &e.surfaces
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := surface - 1
	if idx >= len(s.imgs) {
		return
	}
	if len(fr.Tiles) == 0 && len(fr.Moves) == 0 && !full {
		return
	}
	fr.Surface = surface
	s.owned[idx] = false
	s.last = idx
	for j := range s.pending {
		if j == idx {
			continue
		}
		p := &s.pending[j]
		if full {
			p.all = true
			p.rects = p.rects[:0]
			continue
		}
		for _, t := range fr.Tiles {
			p.add(image.Rect(t.X, t.Y, t.X+t.W, t.Y+t.H))
		}
		for _, m := range fr.Moves {
			p.add(m.Rect)
		}
	}
}

// copyFrontInto копирует из front в buf области pending (или всё), кроме
// skip: туда кадр положит своё.
//
// На перетаскивании окна это было всё окно на кадр: буфер догонял прошлый
// кадр там, где окно стояло, и тот же кадр сразу рисовал там окно заново.
func copyFrontInto(front, buf *image.RGBA, p *surfacePending, skip image.Rectangle) {
	if front == nil || buf == nil {
		return
	}
	bounds := front.Rect
	rects := p.rects
	if p.all {
		rects = []image.Rectangle{bounds}
	}
	for _, whole := range rects {
		for _, r := range outsideOf(whole.Intersect(bounds), skip) {
			copyRectWithin(front, buf, r)
		}
	}
}

// outsideOf — части r вне skip: до четырёх полос.
func outsideOf(r, skip image.Rectangle) []image.Rectangle {
	if r.Empty() {
		return nil
	}
	c := r.Intersect(skip)
	if c.Empty() {
		return []image.Rectangle{r}
	}
	var out []image.Rectangle
	for _, q := range [...]image.Rectangle{
		image.Rect(r.Min.X, r.Min.Y, r.Max.X, c.Min.Y),
		image.Rect(r.Min.X, c.Max.Y, r.Max.X, r.Max.Y),
		image.Rect(r.Min.X, c.Min.Y, c.Min.X, c.Max.Y),
		image.Rect(c.Max.X, c.Min.Y, r.Max.X, c.Max.Y),
	} {
		if !q.Empty() {
			out = append(out, q)
		}
	}
	return out
}

// copyRectWithin копирует область r из front в buf.
func copyRectWithin(front, buf *image.RGBA, r image.Rectangle) {
	n := r.Dx() * 4
	for y := r.Min.Y; y < r.Max.Y; y++ {
		so := front.PixOffset(r.Min.X, y)
		do := buf.PixOffset(r.Min.X, y)
		copy(buf.Pix[do:do+n], front.Pix[so:so+n])
	}
}

// checkSurface — проверка буфера под холст w×h, общая для SetSurface и
// SetSurfaces.
func checkSurface(pix []byte, stride, w, h int) error {
	rowBytes := w * 4
	if stride < rowBytes {
		return fmt.Errorf("stride %d меньше ширины строки %d байт (физический размер холста %dx%d)",
			stride, rowBytes, w, h)
	}
	need := rowBytes
	if h > 1 {
		need = stride*(h-1) + rowBytes
	}
	if h <= 0 {
		need = 0
	}
	if len(pix) < need {
		return fmt.Errorf("буфер длиной %d байт мал для холста %dx%d при stride=%d (нужно ≥ %d)",
			len(pix), w, h, stride, need)
	}
	return nil
}
