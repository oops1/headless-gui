package engine

// mica.go — материал Mica (Windows 11): сильно размытые обои под окном или
// панелью.
//
// Отличие от BlurBehind (backdrop.go), которым делается акрил: акрил размывает
// то, что УЖЕ нарисовано в кадре под слоем, — окно, соседнюю панель, что
// угодно, — и потому зависит от предыдущего содержимого кадра (отсюда
// backdropdamage.go: частичная перерисовка расширяется на область размытия).
// Mica размывает ОБОИ, а они не меняются от кадра к кадру. Слой Mica — чистая
// функция (положение на экране, обои): пиксель получается одним и тем же при
// полной и при частичной перерисовке, когда бы и в каком порядке он ни
// рисовался, и расширять повреждённую область не нужно.
//
// Как считается:
//  1. Обои (фон движка — SetBackground — или отдельный источник
//     SetWallpaperSource) один раз на связку «обои + радиус» уменьшаются в
//     micaDownscale раз и размываются. Кэш живёт, пока обои те же; смена обоев,
//     разрешения, масштаба или порядка каналов строит его заново.
//  2. Слой берёт из кэша билинейно растянутые пиксели в своей области и кладёт
//     их заменой (Src) — тем же приёмом, что upscaleInto у BlurBehind.
//  3. Поверх ложится подкраска tint: она задаёт затемнение (Mica светлее,
//     MicaAlt темнее), без шума.
//
// Цена кадра — копирование строк с линейной интерполяцией по вертикали; даже
// панель «Пуск» 642×726 считается за единицы миллисекунд. Размытие обоев
// платится один раз, а не на каждое движение окна — это и делает Mica дешевле
// акрила при перетаскивании, а выключатель на уровне профиля нужен из-за
// канала удалённого рабочего стола: размытая плавная картинка под окном — это
// большая площадь изменившихся пикселей на каждый сдвиг окна.

import (
	"image"
	"image/color"
	"sync"
)

// micaDownscale — во сколько раз уменьшаются обои перед размытием. Размытие
// Mica столь сильное, что деталь в 16 физических пикселей уже ничего не
// значит, а картинка 1920×1080 превращается в 120×68.
const micaDownscale = 16

// micaDefaultRadius — радиус размытия (логические пиксели), когда слой его не
// задал. Большой: Mica почти не читается как «размытая картинка», а как
// мягкое цветовое пятно в тоне обоев.
const micaDefaultRadius = 80

// micaLevel — размытые обои для одного радиуса.
type micaLevel struct {
	small  *image.RGBA // уменьшенные и размытые обои, в порядке каналов буфера
	source *image.RGBA // обои, из которых посчитано (для проверки актуальности)
}

// micaCache — размытые обои по радиусам (физическим, в пикселях источника).
// Радиусов в живом приложении один-два (Mica и MicaAlt), поэтому карта.
type micaCache struct {
	mu     sync.Mutex
	levels map[int]*micaLevel
}

// wallpaper возвращает обои, по которым считается Mica: отдельный источник
// движка, а без него — фон. nil — обоев нет.
func (c *Canvas) wallpaper() *image.RGBA {
	if c.wallScaled != nil {
		return c.wallScaled
	}
	return c.bgImage
}

// wallOwner — холст, у которого берутся обои: сам c или его родитель.
func (c *Canvas) wallOwner() *Canvas {
	if c.wallParent != nil {
		return c.wallParent
	}
	return c
}

// setWallpaperSource масштабирует src до размера холста и сохраняет как
// источник обоев для Mica. Порядок каналов — как у back-буфера (см.
// setBackground).
func (c *Canvas) setWallpaperSource(src image.Image) {
	c.wallImg = src
	c.mica = nil
	if src == nil {
		c.wallScaled = nil
		return
	}
	c.wallScaled = scaleToCanvas(src, c.W, c.H, c.format)
}

// scaleToCanvas масштабирует src до w×h и кодирует в порядке каналов format.
func scaleToCanvas(src image.Image, w, h int, format PixelFormat) *image.RGBA {
	holder := &Canvas{W: w, H: h, format: format}
	holder.setBackground(src)
	return holder.bgImage
}

// micaFor возвращает размытые обои для радиуса (физического), строя их при
// необходимости. nil — обоев нет.
func (c *Canvas) micaFor(pradius int) *micaLevel {
	src := c.wallpaper()
	if src == nil {
		return nil
	}
	if c.mica == nil {
		c.mica = &micaCache{levels: map[int]*micaLevel{}}
	}
	mc := c.mica
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if lv := mc.levels[pradius]; lv != nil && lv.source == src {
		return lv
	}
	if len(mc.levels) >= 4 {
		clear(mc.levels) // радиусы сменились: старые уже не понадобятся
	}
	lv := &micaLevel{small: blurWallpaper(src, pradius), source: src}
	mc.levels[pradius] = lv
	return lv
}

// blurWallpaper уменьшает обои в micaDownscale раз усреднением по блокам и
// размывает. pradius — радиус размытия в физических пикселях ИСХОДНОЙ
// картинки.
func blurWallpaper(src *image.RGBA, pradius int) *image.RGBA {
	const k = micaDownscale
	b := src.Bounds()
	sw, sh := (b.Dx()+k-1)/k, (b.Dy()+k-1)/k
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	small := image.NewRGBA(image.Rect(0, 0, sw, sh))
	for y := 0; y < sh; y++ {
		y0 := b.Min.Y + y*k
		rows := min(k, b.Max.Y-y0)
		for x := 0; x < sw; x++ {
			x0 := b.Min.X + x*k
			cols := min(k, b.Max.X-x0)
			var sr, sg, sb, sa uint32
			for dy := 0; dy < rows; dy++ {
				off := src.PixOffset(x0, y0+dy)
				row := src.Pix[off : off+cols*4]
				for i := 0; i+3 < len(row); i += 4 {
					sr += uint32(row[i])
					sg += uint32(row[i+1])
					sb += uint32(row[i+2])
					sa += uint32(row[i+3])
				}
			}
			n := uint32(rows * cols)
			o := small.Pix[small.PixOffset(x, y):]
			o[0], o[1], o[2], o[3] = uint8(sr/n), uint8(sg/n), uint8(sb/n), uint8(sa/n)
		}
	}
	r := pradius / k
	if r < 1 {
		r = 1
	}
	BlurRGBA(small, r, 3)
	return small
}

// MicaBehind кладёт в область r размытые обои и подкрашивает их цветом tint:
// материал Mica. Возвращает false, когда обоев нет (фон движка не задан и
// источник не назначен) — слой тогда рисуют сплошным цветом темы.
//
// r и radius — ЛОГИЧЕСКИЕ; radius <= 0 означает умолчание (micaDefaultRadius).
// Отсечение, в том числе скруглённое, уважается. Результат не зависит от
// того, что нарисовано в кадре под слоем, и от повреждённой области кадра.
func (c *Canvas) MicaBehind(r image.Rectangle, radius int, tint color.RGBA) bool {
	return c.micaAt(r, image.Point{}, radius, tint)
}

// micaAt — MicaBehind для холста, лежащего в пространстве обоев со сдвигом
// shift (логические пиксели): буфер всплывающего оверлея является окошком
// основного холста, и обои берутся у основного (wallParent).
func (c *Canvas) micaAt(r image.Rectangle, shift image.Point, radius int, tint color.RGBA) bool {
	owner := c
	if c.wallParent != nil {
		owner = c.wallParent
	}
	if radius <= 0 {
		radius = micaDefaultRadius
	}
	pr := c.clampRect(c.sRect(r))
	if pr.Empty() {
		return owner.wallpaper() != nil
	}
	lv := owner.micaFor(owner.st(radius))
	if lv == nil {
		return false
	}
	// Положение области в обоях: сдвиг окошка + координата в окошке.
	off := image.Pt(c.sx(shift.X), c.sx(shift.Y))
	src := owner.wallpaper().Bounds()
	c.markImage(pr)
	c.copyMica(lv.small, pr, off, src)

	if tint.A > 0 {
		c.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), tint)
	}
	return true
}

// copyMica записывает в area (физическая область back) пиксели размытых обоев
// со сдвигом off: билинейно по уменьшенной картинке. Запись — заменой, а не
// смешиванием; скруглённое отсечение сужает строки.
func (c *Canvas) copyMica(small *image.RGBA, area image.Rectangle, off image.Point, wall image.Rectangle) {
	const k = micaDownscale
	sw, sh := small.Bounds().Dx(), small.Bounds().Dy()
	w := area.Dx()
	if w <= 0 || sw <= 0 || sh <= 0 {
		return
	}

	// По горизонтали пара соседних пикселей уменьшенной картинки и вес
	// второго (0..256) для каждого столбца области — один раз на вызов.
	type tap struct{ a, b, w int }
	taps := make([]tap, w)
	for i := range taps {
		// Центр пикселя обоев в долях уменьшенной сетки; фиксированная точка 8 бит.
		wx := area.Min.X + i + off.X - wall.Min.X
		fx := ((2*wx + 1) * 256) / (2 * k) // (wx+0.5)/k в 1/256
		fx -= 128                          // центр ячейки уменьшенной картинки
		if fx < 0 {
			fx = 0
		}
		a := fx >> 8
		bi := a + 1
		if a >= sw-1 {
			a, bi, fx = sw-1, sw-1, 0
		}
		taps[i] = tap{a: a, b: bi, w: fx & 255}
	}

	// Строка уменьшенной картинки, растянутая по горизонтали на область.
	hline := func(sy int) []byte {
		line := make([]byte, w*4)
		row := small.Pix[small.PixOffset(0, sy):]
		for i, t := range taps {
			pa := row[t.a*4 : t.a*4+4 : t.a*4+4]
			pb := row[t.b*4 : t.b*4+4 : t.b*4+4]
			d := line[i*4 : i*4+4 : i*4+4]
			if t.w == 0 {
				d[0], d[1], d[2], d[3] = pa[0], pa[1], pa[2], pa[3]
				continue
			}
			wb, wa := t.w, 256-t.w
			d[0] = uint8((int(pa[0])*wa + int(pb[0])*wb) >> 8)
			d[1] = uint8((int(pa[1])*wa + int(pb[1])*wb) >> 8)
			d[2] = uint8((int(pa[2])*wa + int(pb[2])*wb) >> 8)
			d[3] = uint8((int(pa[3])*wa + int(pb[3])*wb) >> 8)
		}
		return line
	}

	cache := map[int][]byte{}
	get := func(sy int) []byte {
		if l, ok := cache[sy]; ok {
			return l
		}
		if len(cache) > 4 {
			clear(cache)
		}
		l := hline(sy)
		cache[sy] = l
		return l
	}

	out := make([]byte, w*4)
	for y := area.Min.Y; y < area.Max.Y; y++ {
		x0, x1, ok := c.round.spanX(y, area.Min.X, area.Max.X)
		if !ok {
			continue
		}
		wy := y + off.Y - wall.Min.Y
		fy := ((2*wy+1)*256)/(2*k) - 128
		if fy < 0 {
			fy = 0
		}
		ya := fy >> 8
		yb := ya + 1
		if ya >= sh-1 {
			ya, yb, fy = sh-1, sh-1, 0
		}
		wb := fy & 255
		la := get(ya)
		if wb == 0 {
			copy(out, la)
		} else {
			lb := get(yb)
			wa := 256 - wb
			for i := 0; i < len(out); i++ {
				out[i] = uint8((int(la[i])*wa + int(lb[i])*wb) >> 8)
			}
		}
		dst := c.back.PixOffset(x0, y)
		copy(c.back.Pix[dst:dst+(x1-x0)*4], out[(x0-area.Min.X)*4:(x1-area.Min.X)*4])
	}
}
