package engine

import (
	"image"
	"runtime"
	"testing"
)

// Буфер тени строился по всей фигуре, а не по видимой её части. В сравнении
// файлов карточка накрывает блок строк целиком: у файла на тридцать тысяч
// строк это полмиллиона пикселей по высоте, полтора гигабайта на буфер — и
// два прохода размытия по ним на каждый кадр. На экране от такой тени видно
// высоту окна.

// allocBytes — сколько байт куча выделила за время работы fn.
func allocBytes(fn func()) uint64 {
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	fn()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

func TestDrawSoftShadow_TallShapeStaysSmall(t *testing.T) {
	c := newWhiteTestCanvas(400, 300)

	// Фигура высотой полмиллиона пикселей — как карточка блока в сравнении
	// большого файла. Старый буфер занял бы под гигабайт.
	tall := image.Rect(20, -250000, 380, 250000)
	got := allocBytes(func() { c.DrawSoftShadow(tall, 6, 2, shadowColor) })

	const limit = 8 << 20 // 8 МиБ с большим запасом на холст 400×300
	if got > limit {
		t.Errorf("тень высокой фигуры выделила %d байт, предел %d", got, limit)
	}
	// И при этом она нарисована: у видимого края фигуры фон потемнел.
	if darkness(c, 200, 150) == 0 {
		t.Error("тень не нарисована вовсе")
	}
}

// Обрезка буфера не меняет картинку: у фигуры, уходящей за нижний край на
// сотню пикселей, и у такой же, уходящей на тысячи, видимая часть тени
// совпадает пиксель в пиксель.
func TestDrawSoftShadow_ClipDoesNotChangePicture(t *testing.T) {
	draw := func(bottom int) *Canvas {
		c := newWhiteTestCanvas(300, 200)
		c.DrawSoftShadow(image.Rect(40, 40, 260, bottom), 8, 3, shadowColor)
		return c
	}
	near := draw(400)  // на 200 px ниже холста
	far := draw(40000) // на 39 800 px ниже холста
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			if pixAt(near, x, y) != pixAt(far, x, y) {
				t.Fatalf("пиксель (%d,%d) разошёлся: %v против %v",
					x, y, pixAt(near, x, y), pixAt(far, x, y))
			}
		}
	}
}

// Фигура целиком за отсечением тени не даёт: раньше буфер всё равно
// строился и композитился впустую.
func TestDrawSoftShadow_OutsideClipDrawsNothing(t *testing.T) {
	c := newWhiteTestCanvas(200, 150)
	before := append([]uint8(nil), c.back.Pix...)

	c.DrawSoftShadow(image.Rect(20, -5000, 180, -4000), 6, 2, shadowColor)

	for i := range before {
		if before[i] != c.back.Pix[i] {
			t.Fatalf("кадр изменился от тени, которой не видно (байт %d)", i)
		}
	}
}

// Отсечение сужает буфер: тень фигуры, от которой видна узкая полоска,
// стоит примерно столько же, сколько сама полоска.
func TestDrawSoftShadow_HonoursClip(t *testing.T) {
	c := newWhiteTestCanvas(600, 400)
	c.SetClip(image.Rect(0, 0, 600, 40))
	defer c.ClearClip()

	got := allocBytes(func() {
		c.DrawSoftShadow(image.Rect(20, 20, 580, 380), 6, 2, shadowColor)
	})
	const limit = 1 << 20 // 1 МиБ: полоска 600×40 с запасом на размытие
	if got > limit {
		t.Errorf("тень под узким отсечением выделила %d байт, предел %d", got, limit)
	}
	if darkness(c, 300, 30) == 0 {
		t.Error("в видимой полоске тени нет")
	}
}
