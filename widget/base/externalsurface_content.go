package base

// externalsurface_content.go — что окно программы сообщает о своём
// содержимом (ADR-0002, 2.3): непрозрачную область, вид содержимого и,
// по частоте обновлений, показывает ли оно видео.
//
// Видео — единственное на рабочем столе, что стоит отдельного кодека: тайлами
// фильм стоит сотню мегабит в секунду, видеокодеком — меньше одного. Сессия
// WinLine узнавала его эвристикой у себя; поверхность знает то же самое по
// своим Swap и Update и отдаёт движку, а движок — потребителю в кадре.
//
// Реестр поверхностей, обновлявшихся недавно, общий на процесс: движок
// спрашивает только их, а не обходит дерево, и программа без окон программ
// за это не платит ничем.

import (
	"image"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ContentHint — что показывает окно программы, по словам самой программы
// (wp_content_type_v1 у Wayland).
type ContentHint uint8

const (
	// ContentUnknown — программа не сказала; видео ли это, решает частота
	// обновлений.
	ContentUnknown ContentHint = iota
	// ContentStatic — не видео, как бы часто окно ни обновлялось (игра,
	// анимация интерфейса, которую программа просит не кодировать с потерями).
	ContentStatic
	// ContentVideo — видео, как бы редко окно ни обновлялось.
	ContentVideo
)

// rateWindow — окно, по которому считается частота обновлений.
const rateWindow = time.Second

// rateRing — сколько отметок обновлений помнит поверхность. С запасом над
// любым разумным порогом видео (12–30 в секунду).
const rateRing = 64

// surfaceContent — состояние содержательной части поверхности.
type surfaceContent struct {
	hint atomic.Uint32

	// opaque — непрозрачные прямоугольники от клиента, в координатах буфера;
	// nil — не задано (поверхность непрозрачна целиком при opaque у буфера).
	opaque atomic.Pointer[[]image.Rectangle]
	// opCache — opaque, переведённые в логические координаты кадра, и ключ,
	// при котором они посчитаны. Только горутина кадра.
	opMu    sync.Mutex
	opCache []image.Rectangle
	opKey   opaqueKey

	mu         sync.Mutex
	stamps     [rateRing]int64 // UnixNano обновлений, кольцо
	head, n    int
	registered bool
	videoOn    bool
	lastHalf   int64 // когда частота последний раз была не ниже половины порога
	id         uint32
}

type opaqueKey struct {
	bounds image.Rectangle
	buf    image.Rectangle
	scale  uint64
	src    *[]image.Rectangle
}

var (
	videoRegMu sync.Mutex
	videoReg   = map[*ExternalSurface]struct{}{}
	videoIDSeq atomic.Uint32
)

// SetContentHint сообщает, что показывает окно программы. Подсказка
// программы сильнее частоты обновлений.
func (s *ExternalSurface) SetContentHint(h ContentHint) {
	s.content.hint.Store(uint32(h))
	if h == ContentVideo {
		s.registerVideo()
	}
}

// ContentHint возвращает подсказку программы.
func (s *ExternalSurface) ContentHint() ContentHint {
	return ContentHint(s.content.hint.Load())
}

// SetOpaqueRegion задаёт, какие части буфера непрозрачны (wl_surface.
// set_opaque_region), в координатах буфера. Уточняет OpaqueRegion: под
// поверхностью движок не рисует только то, что лежит в этих прямоугольниках.
// nil снимает уточнение — непрозрачна вся поверхность, если буфер opaque.
// Сам блит не меняется: признак opaque буфера по-прежнему решает, смешивать
// ли пиксели.
func (s *ExternalSurface) SetOpaqueRegion(rects []image.Rectangle) {
	var p *[]image.Rectangle
	if rects != nil {
		cp := append([]image.Rectangle(nil), rects...)
		p = &cp
	}
	s.content.opaque.Store(p)
	// То, что под поверхностью, могло открыться или закрыться.
	s.Invalidate()
}

// SurfaceID — стабильный номер поверхности для потребителя кадров
// (output.VideoRegion.ID, output.Region.VideoID). Выдаётся при первом
// запросе и не меняется.
func (s *ExternalSurface) SurfaceID() uint32 {
	c := &s.content
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.id == 0 {
		c.id = videoIDSeq.Add(1)
	}
	return c.id
}

// noteUpdate — содержимое обновилось (Swap, Update, InvalidateRects).
func (s *ExternalSurface) noteUpdate() {
	c := &s.content
	now := time.Now().UnixNano()
	c.mu.Lock()
	c.stamps[c.head] = now
	c.head = (c.head + 1) % rateRing
	if c.n < rateRing {
		c.n++
	}
	reg := !c.registered
	c.registered = true
	c.mu.Unlock()
	if reg {
		videoRegMu.Lock()
		videoReg[s] = struct{}{}
		videoRegMu.Unlock()
	}
}

func (s *ExternalSurface) registerVideo() {
	c := &s.content
	c.mu.Lock()
	reg := !c.registered
	c.registered = true
	c.mu.Unlock()
	if reg {
		videoRegMu.Lock()
		videoReg[s] = struct{}{}
		videoRegMu.Unlock()
	}
}

// rateLocked — сколько обновлений было за последнюю секунду до now.
func (c *surfaceContent) rateLocked(now int64) (count int, last int64) {
	from := now - int64(rateWindow)
	for i := 0; i < c.n; i++ {
		t := c.stamps[(c.head-1-i+rateRing)%rateRing]
		if i == 0 {
			last = t
		}
		if t < from {
			break
		}
		count++
	}
	return count, last
}

// videoAt решает, видео ли сейчас на поверхности: вход — частота не ниже
// rate, выход — частота ниже rate/2 дольше hold. Поверхность, не
// обновлявшаяся дольше hold+секунды и не помеченная программой как видео,
// уходит из реестра (её вернёт следующее обновление).
func (s *ExternalSurface) videoAt(now time.Time, rate int, hold time.Duration) (on, idle bool) {
	c := &s.content
	switch ContentHint(c.hint.Load()) {
	case ContentVideo:
		return true, false
	case ContentStatic:
		return false, false
	}
	ns := now.UnixNano()
	c.mu.Lock()
	defer c.mu.Unlock()
	count, last := c.rateLocked(ns)
	if rate < 1 {
		rate = 1
	}
	if count*2 >= rate {
		c.lastHalf = ns
	}
	switch {
	case count >= rate:
		c.videoOn = true
	case c.videoOn && ns-c.lastHalf >= int64(hold):
		c.videoOn = false
	}
	idle = !c.videoOn && (c.n == 0 || ns-last > int64(hold+rateWindow))
	if idle {
		c.registered = false
	}
	return c.videoOn, idle
}

// VideoSurface — окно программы, показывающее видео, как его видит движок:
// где оно на экране и что лежит над ним. Координаты логические экранные.
type VideoSurface struct {
	ID    uint32
	Rect  image.Rectangle
	Cover []image.Rectangle
}

// VideoCandidates — сколько поверхностей обновлялись недавно или помечены
// как видео. Ноль — движку незачем искать видео в кадре.
func VideoCandidates() int {
	videoRegMu.Lock()
	defer videoRegMu.Unlock()
	return len(videoReg)
}

// CollectVideoSurfaces дописывает в dst поверхности дерева root, которые
// сейчас показывают видео, с тем, что лежит над ними в дереве (соседи выше
// по Z на каждом уровне, обрезанные поверхностью). Пороги — rate обновлений
// в секунду на вход, hold ниже половины порога на выход. Для движка.
func CollectVideoSurfaces(root Widget, now time.Time, rate int, hold time.Duration, dst []VideoSurface) []VideoSurface {
	rb, ok := root.(baseProvider)
	if !ok {
		return dst
	}
	rootBase := rb.baseRef()
	// Обход под замком реестра, без копии: реестр общий на процесс, и копия
	// на каждом кадре каждого движка была бы аллокацией у программ, которым
	// до чужих окон дела нет. Порядок замков — реестр, затем поверхность;
	// noteUpdate берёт их порознь, так что встречного порядка нет.
	videoRegMu.Lock()
	defer videoRegMu.Unlock()
	for s := range videoReg {
		on, idle := s.videoAt(now, rate, hold)
		if idle {
			delete(videoReg, s) // удаление при обходе map разрешено
		}
		// Чужую поверхность (другое окно, другой движок процесса) отсекаем
		// до screenCover: тот копит покрытия в срез ещё до того, как поймёт,
		// что поверхность не в этом дереве, и каждый кадр движка без видео
		// платил аллокацией за видео соседа, пока оно в реестре.
		if !on || !IsWidgetVisible(s) || !underRoot(&s.Base, rootBase) {
			continue
		}
		rect, cover, inTree := s.screenCover(rootBase)
		if !inTree || rect.Empty() {
			continue
		}
		dst = append(dst, VideoSurface{ID: s.SurfaceID(), Rect: rect, Cover: cover})
	}
	return dst
}

// underRoot — приводит ли подъём от node по родителям к rootBase; тот же
// путь и предел глубины, что в screenCover, но без аллокаций.
func underRoot(node, rootBase *Base) bool {
	parent := node.parent.Load()
	for depth := 0; parent != nil && depth < maxScreenRectDepth; depth++ {
		node, parent = parent, parent.parent.Load()
	}
	return node == rootBase
}

// screenCover поднимается от поверхности к корню: на каждом уровне соседи
// после узла (выше по Z) накрывают его своим поддеревом. false — поверхность
// не в дереве корня rootBase или скрыта по пути.
func (s *ExternalSurface) screenCover(rootBase *Base) (rect image.Rectangle, cover []image.Rectangle, inTree bool) {
	node := &s.Base
	parent := node.parent.Load()
	rect = screenRect(parent, s.Bounds())
	for depth := 0; parent != nil && depth < maxScreenRectDepth; depth++ {
		idx := -1
		for i, c := range parent.children {
			if bp, ok := c.(baseProvider); ok && bp.baseRef() == node {
				idx = i
				if !IsWidgetVisible(c) {
					return rect, nil, false
				}
				break
			}
		}
		if idx < 0 {
			return rect, nil, false
		}
		for _, sib := range parent.children[idx+1:] {
			if !IsWidgetVisible(sib) {
				continue
			}
			r := screenRect(parent, SubtreeRect(sib)).Intersect(rect)
			if !r.Empty() {
				cover = append(cover, r)
			}
		}
		node, parent = parent, parent.parent.Load()
	}
	return rect, cover, node == rootBase
}

// opaqueRegionLocal — непрозрачная область от клиента в логических
// координатах кадра; ok=false — клиент её не задавал.
func (s *ExternalSurface) opaqueRegionLocal() (rects []image.Rectangle, ok bool) {
	src := s.content.opaque.Load()
	if src == nil {
		return nil, false
	}
	m := s.loadMeta()
	key := opaqueKey{bounds: s.Bounds(), buf: m.rect, scale: s.scaleBits.Load(), src: src}
	c := &s.content
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if c.opKey == key {
		return c.opCache, true
	}
	k := math.Float64frombits(key.scale)
	if k <= 0 {
		k = 1
	}
	ox, oy := physEdge(key.bounds.Min.X, k), physEdge(key.bounds.Min.Y, k)
	c.opCache = c.opCache[:0]
	for _, r := range *src {
		r = r.Intersect(m.rect)
		if r.Empty() {
			continue
		}
		r = r.Sub(m.rect.Min).Add(image.Pt(ox, oy))
		// Внутрь: непрозрачным объявляется только то, что накрыто целиком.
		l := image.Rect(logCeil(r.Min.X, k), logCeil(r.Min.Y, k),
			logFloor(r.Max.X, k), logFloor(r.Max.Y, k)).Intersect(key.bounds)
		if !l.Empty() {
			c.opCache = append(c.opCache, l)
		}
	}
	c.opKey = key
	return c.opCache, true
}
