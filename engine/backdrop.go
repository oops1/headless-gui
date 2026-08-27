package engine

import (
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
func (c *Canvas) downscaleForBlur(src image.Rectangle, radius int) (*image.RGBA, int) {
	k := backdropDownscale
	sw, sh := (src.Dx()+k-1)/k, (src.Dy()+k-1)/k
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	small := image.NewRGBA(image.Rect(0, 0, sw, sh))

	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			// Среднее по блоку k×k. Каналы premultiplied, поэтому
			// усредняются все четыре одинаково и без разделения на альфу.
			var sr, sg, sb, sa, n uint32
			for dy := 0; dy < k; dy++ {
				py := src.Min.Y + y*k + dy
				if py >= src.Max.Y {
					break
				}
				off := c.back.PixOffset(src.Min.X+x*k, py)
				for dx := 0; dx < k; dx++ {
					px := src.Min.X + x*k + dx
					if px >= src.Max.X {
						break
					}
					p := c.back.Pix[off : off+4 : off+4]
					sr += uint32(p[0])
					sg += uint32(p[1])
					sb += uint32(p[2])
					sa += uint32(p[3])
					n++
					off += 4
				}
			}
			if n == 0 {
				continue
			}
			o := small.PixOffset(x, y)
			small.Pix[o+0] = uint8(sr / n)
			small.Pix[o+1] = uint8(sg / n)
			small.Pix[o+2] = uint8(sb / n)
			small.Pix[o+3] = uint8(sa / n)
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
// Запись идёт через fillRectPx поблочно: так размытая подложка уважает и
// прямоугольное отсечение, и скруглённое — панель со скруглёнными углами
// получает стекло по своей форме, а не по описанному прямоугольнику.
func (c *Canvas) upscaleInto(small *image.RGBA, src, dst image.Rectangle) {
	k := backdropDownscale
	sw, sh := small.Rect.Dx(), small.Rect.Dy()

	for y := 0; y < sh; y++ {
		top := src.Min.Y + y*k
		bottom := top + k
		if top >= dst.Max.Y {
			break
		}
		if bottom <= dst.Min.Y {
			continue
		}
		for x := 0; x < sw; x++ {
			left := src.Min.X + x*k
			right := left + k
			if left >= dst.Max.X {
				break
			}
			if right <= dst.Min.X {
				continue
			}
			o := small.PixOffset(x, y)
			p := small.Pix[o : o+4 : o+4]
			col := color.RGBA{R: p[0], G: p[1], B: p[2], A: p[3]}
			// Src, а не Over: подложка ЗАМЕНЯЕТ то, что под ней, размытой
			// версией того же самого. Смешивание удвоило бы яркость.
			c.fillRectPx(image.Rect(left, top, right, bottom).Intersect(dst), col, false)
		}
	}
}
