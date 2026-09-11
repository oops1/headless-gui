package engine

import (
	"encoding/binary"
	"image"
	"image/color"
)

// Размытая подложка слоя — acrylic и mica Windows 11, материалы macOS.
//
// Полупрозрачная панель сегодня умеет только смешаться с тем, что под ней
// (Panel.UseAlpha). Этого достаточно для плоского цвета, но не для
// современного вида: там сквозь панель видно РАЗМЫТОЕ содержимое —
// обои, окно, что угодно, — и от этого слой читается как стекло, а не как
// плёнка.
//
// Как считается: область под слоем снимается из уже нарисованного кадра,
// уменьшается, размывается и возвращается обратно растянутой. Уменьшение —
// не экономия ради экономии: размытие по уменьшенной картинке даёт тот же
// визуальный радиус в разы дешевле, а мелких деталей, которые от этого
// потерялись бы, в размытии всё равно не остаётся. Полоса панели задач
// 1920×48 после уменьшения вчетверо — это 480×12 пикселей.
//
// Порядок вызова важен: BlurBehind вызывается ДО отрисовки содержимого
// слоя, поверх уже нарисованного фона. Слой рисует поверх результата свою
// полупрозрачную заливку (Tint), и получается стекло.

// backdropDownscale — во сколько раз уменьшается область перед размытием.
// Четыре — компромисс: заметного огрубления ещё нет, а работы в шестнадцать
// раз меньше.
const backdropDownscale = 4

// BlurBehind размывает то, что уже нарисовано в области r, на месте.
//
// r и radius — ЛОГИЧЕСКИЕ координаты и радиус. Пустая область, неположительный
// радиус — ничего не делает. Отсечение канваса уважается: за его пределами
// кадр не меняется.
//
// tint — цвет, которым область подкрашивается поверх размытия
// (alpha-premultiplied); прозрачный — без подкраски. Именно tint отличает
// «матовое стекло» одной темы от другой: сама по себе размытая картинка
// нейтральна.
func (c *Canvas) BlurBehind(r image.Rectangle, radius int, tint color.RGBA) {
	if radius <= 0 {
		if tint.A > 0 {
			c.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), tint)
		}
		return
	}
	pr := c.clampRect(c.sRect(r))
	if pr.Empty() {
		return
	}
	pradius := c.st(radius)
	if pradius <= 0 {
		pradius = 1
	}

	// Захватываем область с запасом: размытие у края берёт соседей, и без
	// запаса вдоль границы слоя пошла бы кайма от зажима координат.
	margin := pradius
	src := pr.Inset(-margin).Intersect(c.back.Bounds())
	if src.Empty() {
		return
	}

	small, sradius := c.downscaleForBlur(src, pradius)
	BlurRGBA(small, sradius, 2)

	// Возвращаем размытое обратно, растягивая; за пределы отсечения не
	// выходим — этим занимается fillRectPx, через который идёт запись.
	c.upscaleInto(small, src, pr)

	if tint.A > 0 {
		c.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), tint)
	}
}

// downscaleForBlur снимает область src из кадра, уменьшая её в
// backdropDownscale раз усреднением по блокам. Возвращает уменьшенную
// картинку и радиус размытия в её масштабе.
//
// Полный блок суммируется «по полосам в регистре» (SWAR): два пикселя
// читаются одним uint64, маска 0x00FF00FF00FF00FF раскладывает чётные каналы
// (R, B), сдвиг на 8 и та же маска — нечётные (G, A), каждый в свою 16-битную
// полосу. Сумма блока 4×4 по каналу не больше 16·255 = 4080 и в полосе
// помещается без переноса в соседнюю, так что это точно те же суммы, что
// поканальный цикл, — втрое меньше операций. Деление на число пикселей у
// полного блока — на константу k·k, то есть сдвиг; прежде на каждый
// уменьшенный пиксель приходилось четыре деления на переменную. Неполные
// блоки у края — прежним циклом.
func (c *Canvas) downscaleForBlur(src image.Rectangle, radius int) (*image.RGBA, int) {
	const k = backdropDownscale
	sw, sh := (src.Dx()+k-1)/k, (src.Dy()+k-1)/k
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	small := image.NewRGBA(image.Rect(0, 0, sw, sh))

	for y := 0; y < sh; y++ {
		y0 := src.Min.Y + y*k
		rows := min(k, src.Max.Y-y0)
		out := small.Pix[small.PixOffset(0, y):]
		for x := 0; x < sw; x++ {
			x0 := src.Min.X + x*k
			cols := min(k, src.Max.X-x0)
			if rows <= 0 || cols <= 0 {
				continue
			}
			o := out[x*4 : x*4+4 : x*4+4]
			if rows == k && cols == k {
				const lanes = 0x00FF00FF00FF00FF
				var ev, od uint64
				for dy := 0; dy < k; dy++ {
					off := c.back.PixOffset(x0, y0+dy)
					p := c.back.Pix[off : off+16 : off+16]
					v0, v1 := binary.LittleEndian.Uint64(p[0:8]), binary.LittleEndian.Uint64(p[8:16])
					ev += v0&lanes + v1&lanes
					od += (v0>>8)&lanes + (v1>>8)&lanes
				}
				// Полосы: 0 — R (или G) пикселей 0 и 2, 1 — B (A), 2 и 3 —
				// то же для пикселей 1 и 3.
				o[0] = uint8((ev&0xFFFF + ev>>32&0xFFFF) / (k * k))
				o[1] = uint8((od&0xFFFF + od>>32&0xFFFF) / (k * k))
				o[2] = uint8((ev>>16&0xFFFF + ev>>48) / (k * k))
				o[3] = uint8((od>>16&0xFFFF + od>>48) / (k * k))
				continue
			}
			// Среднее по неполному блоку. Каналы premultiplied, поэтому
			// усредняются все четыре одинаково и без разделения на альфу.
			var sr, sg, sb, sa uint32
			for dy := 0; dy < rows; dy++ {
				off := c.back.PixOffset(x0, y0+dy)
				row := c.back.Pix[off : off+cols*4 : off+cols*4]
				for i := 0; i+3 < len(row); i += 4 {
					sr += uint32(row[i])
					sg += uint32(row[i+1])
					sb += uint32(row[i+2])
					sa += uint32(row[i+3])
				}
			}
			n := uint32(rows * cols)
			o[0], o[1], o[2], o[3] = uint8(sr/n), uint8(sg/n), uint8(sb/n), uint8(sa/n)
		}
	}

	sradius := radius / k
	if sradius < 1 {
		sradius = 1
	}
	return small, sradius
}

// upscaleInto возвращает уменьшенную картинку обратно в кадр, растягивая её
// на область src и записывая только внутри dst.
//
// Каждая строка уменьшенной картинки растягивается один раз, в буфер, и
// копируется во все k строк кадра, которые она накрывает, — в пределах
// прямоугольного отсечения и скруглённого: панель со скруглёнными углами
// получает стекло по своей форме, а не по описанному прямоугольнику. Прежде
// каждый блок k×k заливался отдельным вызовом — пять с половиной тысяч на
// полосу панели задач, и каждый со своими проверками и пометкой тайлов.
//
// Запись — Src, а не Over: подложка ЗАМЕНЯЕТ то, что под ней, размытой
// версией того же самого. Смешивание удвоило бы яркость.
func (c *Canvas) upscaleInto(small *image.RGBA, src, dst image.Rectangle) {
	const k = backdropDownscale
	area := c.clampRect(dst.Intersect(src))
	if area.Empty() {
		return
	}
	// Для потребителя кадра размытая подложка — картинка: сплошных тайлов в
	// ней нет, а кодеку изображений она по душе.
	c.markImage(area)

	line := make([]byte, area.Dx()*4)
	lineY := -1
	for y := area.Min.Y; y < area.Max.Y; y++ {
		x0, x1, ok := c.round.spanX(y, area.Min.X, area.Max.X)
		if !ok {
			continue
		}
		if sy := (y - src.Min.Y) / k; sy != lineY {
			lineY = sy
			srow := small.Pix[small.PixOffset(0, sy):]
			for x := area.Min.X; x < area.Max.X; x++ {
				sx := (x - src.Min.X) / k
				d := line[(x-area.Min.X)*4 : (x-area.Min.X)*4+4 : (x-area.Min.X)*4+4]
				p := srow[sx*4 : sx*4+4 : sx*4+4]
				d[0], d[1], d[2], d[3] = p[0], p[1], p[2], p[3]
			}
		}
		off := c.back.PixOffset(x0, y)
		copy(c.back.Pix[off:off+(x1-x0)*4], line[(x0-area.Min.X)*4:(x1-area.Min.X)*4])
	}
}
