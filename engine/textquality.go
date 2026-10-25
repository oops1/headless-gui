package engine

import (
	"image"
	"image/color"
	"math"
	"sync"
)

// LCDOrder — порядок цветных подпикселей монитора слева направо.
type LCDOrder uint8

const (
	LCDOrderRGB LCDOrder = iota // красный слева (почти все мониторы)
	LCDOrderBGR                 // синий слева
)

// DefaultTextContrast — подъём контраста тонких штрихов, который советуем
// включать вместе с хинтингом: тёмный мелкий текст на светлом фоне становится
// заметно плотнее, светлый на тёмном остаётся прежним (см. TextQuality.Contrast).
const DefaultTextContrast = 0.5

// TextQuality — слой качества мелкого текста поверх растеризатора глифов.
// Нулевое значение — прежний вид текста, побайтно: ничего не выравнивается и
// не подправляется. Ширины строк (MeasureText, шаги глифов, кернинг) от режима
// не зависят никогда: меняется только то, как рисуется маска глифа, поэтому
// раскладка приложения и эталонные снимки по ширине остаются прежними.
type TextQuality struct {
	// Hinting включает лёгкий автохинтинг по вертикали (в духе режима light у
	// FreeType и Skia). Для каждого шрифта и кегля по контурам эталонных букв
	// («x», «o», «H», «O», «l», «p»; для кириллицы «х», «о», «Н», «О») находятся
	// зоны — базовая линия, высота строчных и заглавных, верх выносных
	// элементов, низ нижних, — и контур глифа растягивается по вертикали
	// кусочно-линейно так, чтобы эти высоты пришлись на целые ФИЗИЧЕСКИЕ
	// пиксели (то есть с учётом HiDPI-масштаба). Выступы круглых букв
	// («overshoot») меньше полупикселя прижимаются к зоне. По горизонтали контур
	// не меняется. Работает на путях DrawText, DrawTextFont, шейпера и
	// повёрнутого текста; текст внутри SVG — векторная графика, масштабируется
	// произвольно и не хинтится.
	Hinting bool

	// Contrast — подъём контраста покрытия, от 0 (нет) до 1; значения вне
	// отрезка обрезаются. Маска глифа пересчитывается кривой a' = a + k·a·(1−a),
	// где k зависит от яркости цвета текста: тёмный текст получает полный
	// подъём (тонкие штрихи не светлеют), к яркости ≈75 % подъём сходит на нет,
	// а белый текст на тёмном фоне, наоборот, слегка утоньшается (не «расплывается»
	// от светового ореола). Таблица целочисленная и детерминированная, от фона
	// при растеризации не зависит. DefaultTextContrast — разумное значение.
	Contrast float64

	// LCD включает субпиксельное (ClearType-подобное) сглаживание: глиф
	// растеризуется в тройном разрешении по горизонтали и сглаживается
	// фильтром FreeType (веса 0x08 0x4D 0x56 0x4D 0x08), а три канала
	// смешиваются с фоном по отдельности. Только по явной просьбе приложения и
	// только на непрозрачном фоне: на пикселях с прозрачностью (A<255) глиф
	// рисуется обычным серым покрытием. В удалённом сеансе (RDP) бесполезно и
	// хуже сжимается.
	LCD bool
	// LCDOrder — порядок подпикселей монитора (по умолчанию RGB).
	LCDOrder LCDOrder
}

// normalized обрезает Contrast до [0, 1] и приводит значение к кратному 1/64 —
// так число, а значит и кэш таблиц, не зависит от плавающих «хвостов».
func (q TextQuality) normalized() TextQuality {
	switch {
	case !(q.Contrast > 0): // и NaN
		q.Contrast = 0
	case q.Contrast > 1:
		q.Contrast = 1
	}
	q.Contrast = float64(q.contrastSteps()) / contrastSteps
	if q.LCDOrder != LCDOrderBGR {
		q.LCDOrder = LCDOrderRGB
	}
	return q
}

// contrastSteps — Contrast в целых 1/64.
func (q TextQuality) contrastSteps() int {
	return int(math.Round(q.Contrast * contrastSteps))
}

const contrastSteps = 64

// active сообщает, меняет ли режим хоть что-то в виде текста.
func (q TextQuality) active() bool {
	return q.Hinting || q.Contrast > 0 || q.LCD
}

// ── Таблицы контраста ───────────────────────────────────────────────────────

// lumaPivot — яркость цвета текста, при которой подъём контраста равен нулю.
const lumaPivot = 191

// textLuma — яркость цвета текста 0..255 (веса Rec.709, в целых числах).
func textLuma(c color.RGBA) int {
	return (2126*int(c.R) + 7152*int(c.G) + 722*int(c.B) + 5000) / 10000
}

// divRoundInt делит с округлением к ближайшему (половины — от нуля); d > 0.
func divRoundInt(n, d int) int {
	if n >= 0 {
		return (n + d/2) / d
	}
	return -((-n + d/2) / d)
}

// contrastKey — ключ таблицы: яркость текста и контраст в 1/64.
type contrastKey struct{ luma, steps uint8 }

var contrastLUTs sync.Map // contrastKey → *[256]uint8

// contrastLUT возвращает таблицу пересчёта покрытия для цвета текста; nil —
// контраст выключен либо для этого цвета кривая тождественна. Чистая функция
// от (яркость, контраст): одинакова на всех платформах побайтно, считается в
// целых числах.
func contrastLUT(col color.RGBA, steps int) *[256]uint8 {
	if steps <= 0 {
		return nil
	}
	if steps > contrastSteps {
		steps = contrastSteps
	}
	l := textLuma(col)
	if l == lumaPivot {
		return nil
	}
	key := contrastKey{uint8(l), uint8(steps)}
	if v, ok := contrastLUTs.Load(key); ok {
		return v.(*[256]uint8)
	}
	// k в долях 1/4096: steps/64 · (lumaPivot − L)/lumaPivot (отрицательно у
	// светлого текста).
	k := divRoundInt(steps*(lumaPivot-l)*4096, contrastSteps*lumaPivot)
	var t [256]uint8
	for a := 0; a < 256; a++ {
		v := a + divRoundInt(k*a*(255-a), 4096*255)
		if v < 0 {
			v = 0
		} else if v > 255 {
			v = 255
		}
		t[a] = uint8(v)
	}
	actual, _ := contrastLUTs.LoadOrStore(key, &t)
	return actual.(*[256]uint8)
}

// drawLCDMask смешивает цвет с фоном по каналам (субпиксельное сглаживание).
// Канал смешивается по своему покрытию только там, где фон непрозрачен
// (A=255): подпиксели имеют смысл лишь на известном фоне. На прозрачных и
// полупрозрачных пикселях глиф рисуется обычным серым покрытием — средним по
// каналам. Формулы смешивания те же, что у pixsimd.BlendMaskRow.
func (c *Canvas) drawLCDMask(m *lcdMask, gx, gy int, col color.RGBA, lut *[256]uint8) {
	col = c.enc(col)
	c.markKind(image.Rect(gx, gy, gx+m.w, gy+m.h))
	r := c.clampRect(image.Rect(gx, gy, gx+m.w, gy+m.h))
	if r.Empty() {
		return
	}
	const m16 = 0xFFFF
	sc := [3]uint32{uint32(col.R) * 0x101, uint32(col.G) * 0x101, uint32(col.B) * 0x101}
	sa := uint32(col.A) * 0x101
	// Логический канал байта j буфера: у BGRX байты R и B переставлены (enc).
	var logical [3]int
	for j := range logical {
		logical[j] = j
		if c.format == FormatBGRX {
			logical[j] = 2 - j
		}
	}
	dst := c.back.Pix
	for yy := r.Min.Y; yy < r.Max.Y; yy++ {
		lx, rx, ok := c.round.spanX(yy, r.Min.X, r.Max.X)
		if !ok {
			continue
		}
		src := m.pix[(yy-gy)*m.w*3:]
		dOff := c.back.PixOffset(lx, yy)
		for xx := lx; xx < rx; xx, dOff = xx+1, dOff+4 {
			cov := src[(xx-gx)*3 : (xx-gx)*3+3 : (xx-gx)*3+3]
			if cov[0]|cov[1]|cov[2] == 0 {
				continue
			}
			c0, c1, c2 := uint32(cov[0]), uint32(cov[1]), uint32(cov[2])
			if lut != nil {
				c0, c1, c2 = uint32(lut[c0]), uint32(lut[c1]), uint32(lut[c2])
			}
			p := dst[dOff : dOff+4 : dOff+4]
			if p[3] == 255 {
				cs := [3]uint32{c0, c1, c2}
				var avg uint32
				for j := 0; j < 3; j++ {
					ma := cs[logical[j]]
					ma |= ma << 8
					inv := m16 - sa*ma/m16
					p[j] = uint8((uint32(p[j])*0x101*inv/m16 + sc[j]*ma/m16) >> 8)
					avg += cs[j]
				}
				// Альфа пикселя — как у серого покрытия среднего значения.
				ma := (avg + 1) / 3
				ma |= ma << 8
				inv := m16 - sa*ma/m16
				p[3] = uint8((uint32(p[3])*0x101*inv/m16 + sa*ma/m16) >> 8)
				continue
			}
			ma := (c0 + c1 + c2 + 1) / 3
			ma |= ma << 8
			a := sa * ma / m16
			inv := m16 - a
			for j := 0; j < 3; j++ {
				p[j] = uint8((uint32(p[j])*0x101*inv/m16 + sc[j]*ma/m16) >> 8)
			}
			p[3] = uint8((uint32(p[3])*0x101*inv/m16 + sa*ma/m16) >> 8)
		}
	}
}
