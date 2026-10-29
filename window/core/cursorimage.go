package core

// cursorimage.go — курсор из картинки (Engine.SetCursorImage): общая часть
// всех бэкендов.
//
// Движок хранит картинку как widget.CursorImage с ключом-хешем содержимого.
// Окно отдаёт её ОС через необязательный cursorImageSetter, и только при смене
// ключа. Системный курсор из картинки — дорогой ресурс (Win32 — пара
// GDI-объектов, X11 — pixmap, picture и курсор на сервере, Wayland —
// shm-буфер), поэтому каждый бэкенд держит готовые курсоры в кэше по ключу,
// не более cursorCacheSize штук: сервер удалённого рабочего стола шлёт то один
// указатель, то другой, и пересоздавать их на каждую смену нельзя.
//
// Файл без платформенного суффикса: кэш и пересчёт пикселей — чистые функции.

import (
	"container/list"
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

// cursorCacheSize — сколько готовых курсоров-картинок держит окно.
const cursorCacheSize = 16

// cursorImageSource — движок знает курсор-картинку (реализует *engine.Engine).
type cursorImageSource interface {
	CursorImageOverride() (*widget.CursorImage, bool)
}

// cursorImageSetter — окно ОС умеет показать курсор из картинки. Зовётся на
// потоке окна, как SetCursor; последующий SetCursor заменяет картинку формой.
type cursorImageSetter interface {
	SetCursorImage(ci *widget.CursorImage)
}

// cursorImageOf возвращает курсор-картинку движка или nil.
func cursorImageOf(eng any) *widget.CursorImage {
	src, ok := eng.(cursorImageSource)
	if !ok {
		return nil
	}
	ci, _ := src.CursorImageOverride()
	return ci
}

// sameCursorImage — одна и та же картинка (по ключу; обе nil — тоже одна).
func sameCursorImage(a, b *widget.CursorImage) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Key == b.Key
}

// cursorLRU — кэш готовых системных курсоров по ключу картинки с вытеснением
// давно не использованных. Не потокобезопасен: владелец сериализует доступ
// (поток окна Win32, замок X11 / Wayland).
type cursorLRU[V any] struct {
	cap  int
	ll   *list.List // впереди — самые свежие; значения *cursorLRUEntry[V]
	m    map[[32]byte]*list.Element
	free func(V)      // освободить системный объект при вытеснении
	keep func(V) bool // true — объект показывается сейчас, вытеснять нельзя
}

type cursorLRUEntry[V any] struct {
	key [32]byte
	val V
}

// newCursorLRU создаёт кэш на cap записей. free и keep могут быть nil.
func newCursorLRU[V any](cap int, free func(V), keep func(V) bool) *cursorLRU[V] {
	if cap < 1 {
		cap = 1
	}
	return &cursorLRU[V]{cap: cap, ll: list.New(), m: map[[32]byte]*list.Element{}, free: free, keep: keep}
}

// get возвращает курсор по ключу и помечает его свежим.
func (c *cursorLRU[V]) get(k [32]byte) (V, bool) {
	if e, ok := c.m[k]; ok {
		c.ll.MoveToFront(e)
		return e.Value.(*cursorLRUEntry[V]).val, true
	}
	var zero V
	return zero, false
}

// put кладёт курсор и вытесняет самые давние сверх ёмкости. Показываемый сейчас
// (keep) не вытесняется: система читает его, пока он выбран; кэш временно
// вмещает на одну запись больше.
func (c *cursorLRU[V]) put(k [32]byte, v V) {
	if e, ok := c.m[k]; ok {
		old := e.Value.(*cursorLRUEntry[V]).val
		e.Value.(*cursorLRUEntry[V]).val = v
		c.ll.MoveToFront(e)
		if c.free != nil {
			c.free(old)
		}
		return
	}
	c.m[k] = c.ll.PushFront(&cursorLRUEntry[V]{key: k, val: v})
	for c.ll.Len() > c.cap {
		victim := (*list.Element)(nil)
		for e := c.ll.Back(); e != nil; e = e.Prev() {
			if c.keep == nil || !c.keep(e.Value.(*cursorLRUEntry[V]).val) {
				victim = e
				break
			}
		}
		if victim == nil {
			return
		}
		c.remove(victim)
	}
}

func (c *cursorLRU[V]) remove(e *list.Element) {
	ent := e.Value.(*cursorLRUEntry[V])
	c.ll.Remove(e)
	delete(c.m, ent.key)
	if c.free != nil {
		c.free(ent.val)
	}
}

// len — сколько курсоров в кэше.
func (c *cursorLRU[V]) len() int { return c.ll.Len() }

// clear освобождает всё.
func (c *cursorLRU[V]) clear() {
	for c.ll.Len() > 0 {
		c.remove(c.ll.Back())
	}
}

// cursorPremulRGBA переводит картинку курсора (NRGBA, прямая альфа) в
// image.RGBA с домноженной альфой — формат, который ждут GDI и compositing
// X11/Wayland.
func cursorPremulRGBA(ci *widget.CursorImage) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, ci.W, ci.H))
	for i := 0; i+3 < len(ci.Pix); i += 4 {
		a := uint32(ci.Pix[i+3])
		switch a {
		case 0xFF:
			out.Pix[i], out.Pix[i+1], out.Pix[i+2] = ci.Pix[i], ci.Pix[i+1], ci.Pix[i+2]
		case 0:
			// полностью прозрачный пиксель — нули
		default:
			out.Pix[i] = byte((uint32(ci.Pix[i])*a + 127) / 255)
			out.Pix[i+1] = byte((uint32(ci.Pix[i+1])*a + 127) / 255)
			out.Pix[i+2] = byte((uint32(ci.Pix[i+2])*a + 127) / 255)
		}
		out.Pix[i+3] = byte(a)
	}
	return out
}

// cursorPremulBGRA — те же пиксели в порядке байтов B,G,R,A с домноженной
// альфой: Wayland wl_shm ARGB8888, X11 RENDER ARGB32 и DIB Windows на
// little-endian хосте.
func cursorPremulBGRA(ci *widget.CursorImage) []byte {
	out := make([]byte, len(ci.Pix))
	for i := 0; i+3 < len(ci.Pix); i += 4 {
		a := uint32(ci.Pix[i+3])
		r, g, b := uint32(ci.Pix[i]), uint32(ci.Pix[i+1]), uint32(ci.Pix[i+2])
		if a != 0xFF {
			r, g, b = (r*a+127)/255, (g*a+127)/255, (b*a+127)/255
		}
		out[i], out[i+1], out[i+2], out[i+3] = byte(b), byte(g), byte(r), byte(a)
	}
	return out
}

// cursorMonoPoints строит из картинки две 1-битные карты для курсора без
// альфы (X11 без RENDER): mask — пиксель виден (альфа не меньше порога),
// src — виден и тёмный (яркость меньше половины): тёмное рисуется «fore»
// (чёрным), светлое — «back» (белым). Координаты — пиксели картинки.
func cursorMonoPoints(ci *widget.CursorImage, alphaThreshold uint8) (src, mask [][2]int16) {
	for y := 0; y < ci.H; y++ {
		for x := 0; x < ci.W; x++ {
			i := (y*ci.W + x) * 4
			if ci.Pix[i+3] < alphaThreshold {
				continue
			}
			mask = append(mask, [2]int16{int16(x), int16(y)})
			// Яркость по целым весам Rec.601: (299 R + 587 G + 114 B) / 1000.
			lum := (299*int(ci.Pix[i]) + 587*int(ci.Pix[i+1]) + 114*int(ci.Pix[i+2])) / 1000
			if lum < 128 {
				src = append(src, [2]int16{int16(x), int16(y)})
			}
		}
	}
	return src, mask
}

// cursorScaledHotspot переводит горячую точку из пикселей картинки в
// логические координаты поверхности при буфере масштаба scale (Wayland:
// wl_pointer.set_cursor принимает hotspot в координатах поверхности после
// buffer_scale). Округление вниз: точка остаётся внутри логического размера.
func cursorScaledHotspot(hot, scale int) int {
	if scale < 1 {
		scale = 1
	}
	return hot / scale
}
