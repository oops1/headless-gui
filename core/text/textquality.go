package text

import (
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
	q.Contrast = float64(q.ContrastSteps()) / contrastSteps
	if q.LCDOrder != LCDOrderBGR {
		q.LCDOrder = LCDOrderRGB
	}
	return q
}

// ContrastSteps — Contrast в целых 1/64.
func (q TextQuality) ContrastSteps() int {
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

// ContrastLUT возвращает таблицу пересчёта покрытия для цвета текста; nil —
// контраст выключен либо для этого цвета кривая тождественна. Чистая функция
// от (яркость, контраст): одинакова на всех платформах побайтно, считается в
// целых числах.
func ContrastLUT(col color.RGBA, steps int) *[256]uint8 {
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

