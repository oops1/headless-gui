// rasterprint.go — печать страниц на растровое устройство (GDI на Windows):
// порядок страниц и копии, поворот под ориентацию листа, раскладка на полосы.
// Без системных вызовов: устройство — интерфейс rasterDevice, на Windows его
// реализует контекст GDI, в тестах — запоминающая подделка. Поэтому всё, кроме
// самих вызовов GDI, проверяется на любой платформе и без принтера.
package printing

import (
	"errors"
	"image"
)

// deviceCaps — размеры физического листа устройства в его единицах (точках
// принтера) и смещение печатной области от угла листа.
type deviceCaps struct {
	PhysW, PhysH int // весь лист, включая непечатаемые края
	OffX, OffY   int // где на листе начинается печатная область (начало координат устройства)
}

// rasterDevice — то, на что печатает printRaster.
type rasterDevice interface {
	Caps() deviceCaps
	StartDoc(name string) error
	StartPage() error
	// Band рисует полосу: bits — srcRows строк по srcW пикселей в порядке BGRX
	// сверху вниз; dst — куда они ложатся, в координатах печатной области.
	Band(dst image.Rectangle, srcW, srcRows int, bits []byte) error
	EndPage() error
	EndDoc() error
	// Abort прерывает документ после ошибки: оборванное задание не должно
	// остаться в очереди принтера недопечатанным.
	Abort()
}

// pageOrder возвращает порядок печати: номера страниц с учётом копий.
// collate — комплектами (1 2 3, 1 2 3), иначе постранично (1 1, 2 2, 3 3).
func pageOrder(n, copies int, collate bool) []int {
	if copies < 1 {
		copies = 1
	}
	out := make([]int, 0, n*copies)
	if collate {
		for c := 0; c < copies; c++ {
			for i := 0; i < n; i++ {
				out = append(out, i)
			}
		}
		return out
	}
	for i := 0; i < n; i++ {
		for c := 0; c < copies; c++ {
			out = append(out, i)
		}
	}
	return out
}

// maxBandBytes — размер буфера одной полосы. Около 8 МБ: достаточно, чтобы
// вызовов на страницу было десятки, а не тысячи, и мало для любого драйвера.
const maxBandBytes = 8 << 20

// printRaster печатает страницы на устройство. Каждая страница — вся картинка
// на весь ЛИСТ (а не на печатную область): лист совмещается с бумагой
// миллиметр в миллиметр, а непечатаемый край принтера просто срезает
// кромку. Если бы страница растягивалась на печатную область, то у принтера с
// полями 4 мм и у принтера с полями 6 мм содержимое оказывалось бы в разных
// местах и в разном масштабе.
func printRaster(dev rasterDevice, name string, pages []image.Image, copies int, collate bool) (err error) {
	if len(pages) == 0 {
		return ErrNoPages
	}
	caps := dev.Caps()
	if caps.PhysW <= 0 || caps.PhysH <= 0 {
		return errors.New("printing: принтер не сообщил размер листа")
	}
	if err := dev.StartDoc(name); err != nil {
		return err
	}
	docOpen := true
	defer func() {
		if err != nil && docOpen {
			dev.Abort()
		}
	}()

	var (
		rotIdx = -1
		rotImg *image.RGBA
	)
	for _, idx := range pageOrder(len(pages), copies, collate) {
		var img image.Image = pages[idx]
		b := img.Bounds()
		if needRotate(b.Dx(), b.Dy(), caps.PhysW, caps.PhysH) {
			// Поворот дорог (проход по всем пикселям), а подряд идущие копии
			// одной страницы — частый случай: запоминаем последнюю.
			if rotIdx != idx {
				rotImg, rotIdx = rotate90(img), idx
			}
			img = rotImg
			b = img.Bounds()
		}
		w, h := b.Dx(), b.Dy()
		fit := fitRect(w, h, caps.PhysW, caps.PhysH)
		// В координаты печатной области: начало устройства смещено от угла
		// листа на OffX, OffY (кромка, где принтер не печатает).
		dstX, dstY := fit.Min.X-caps.OffX, fit.Min.Y-caps.OffY

		if err := dev.StartPage(); err != nil {
			return err
		}
		rows := maxBandBytes / (4 * w)
		if rows < 16 {
			rows = 16
		}
		if rows > 512 {
			rows = 512
		}
		buf := make([]byte, 4*w*min(rows, h))
		for _, s := range bandSpans(h, rows, dstY, fit.Dy()) {
			n := s.SrcY1 - s.SrcY0
			fillBGRX(buf[:4*w*n], img, s.SrcY0, s.SrcY1)
			dst := image.Rect(dstX, s.DstY0, dstX+fit.Dx(), s.DstY1)
			if err := dev.Band(dst, w, n, buf[:4*w*n]); err != nil {
				return err
			}
		}
		if err := dev.EndPage(); err != nil {
			return err
		}
	}
	docOpen = false
	return dev.EndDoc()
}
