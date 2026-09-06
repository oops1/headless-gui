package window

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

// Значок окна — запрос GG-35.
//
// На Win32 значок брался только из ресурса исполняемого файла, на X11
// _NET_WM_ICON не выставлялось вовсе — под Linux у окна значка не было в
// принципе.

// icon рисует картинку размера n с узнаваемой первой точкой.
func icon(n int, first color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	img.Set(0, 0, first)
	return img
}

// Формат _NET_WM_ICON: подряд ширина, высота и w×h точек ARGB, по 32 бита.
func TestNetWMIconData_Format(t *testing.T) {
	small := icon(2, color.RGBA{R: 1, G: 2, B: 3, A: 4})
	big := icon(3, color.RGBA{R: 5, G: 6, B: 7, A: 8})

	data := netWMIconData([]image.Image{small, big})

	want := (2 + 2*2 + 2 + 3*3) * 4
	if len(data) != want {
		t.Fatalf("длина свойства %d байт, ждали %d", len(data), want)
	}

	u32 := func(i int) uint32 { return binary.LittleEndian.Uint32(data[i*4 : i*4+4]) }
	if u32(0) != 2 || u32(1) != 2 {
		t.Errorf("первый значок объявлен как %dx%d, ждали 2x2", u32(0), u32(1))
	}
	// ARGB одним числом: альфа в старшем байте, синий в младшем.
	if got := u32(2); got != 0x04010203 {
		t.Errorf("первая точка %#08x, ждали 0x04010203", got)
	}
	// Второй значок начинается сразу после первого.
	off := 2 + 2*2
	if u32(off) != 3 || u32(off+1) != 3 {
		t.Errorf("второй значок объявлен как %dx%d, ждали 3x3", u32(off), u32(off+1))
	}
	if got := u32(off + 2); got != 0x08050607 {
		t.Errorf("первая точка второго значка %#08x", got)
	}
}

// Пустой список даёт пустое свойство: ChangeProperty идёт в режиме Replace,
// то есть значок снимается.
func TestNetWMIconData_EmptyMeansNoIcon(t *testing.T) {
	if got := netWMIconData(nil); len(got) != 0 {
		t.Errorf("пустой список дал %d байт", len(got))
	}
}

// Размер выбирается БЛИЖАЙШИЙ, а не первый подходящий: иначе в заголовок окна
// попадёт картинка 256×256, ужатая системой до шестнадцати точек.
func TestPickIcon_ChoosesNearestSize(t *testing.T) {
	set := []image.Image{icon(256, color.RGBA{}), icon(16, color.RGBA{}), icon(32, color.RGBA{})}

	if got := pickIcon(set, 16).Bounds().Dx(); got != 16 {
		t.Errorf("для 16 выбран значок %d", got)
	}
	if got := pickIcon(set, 32).Bounds().Dx(); got != 32 {
		t.Errorf("для 32 выбран значок %d", got)
	}
	if got := pickIcon(set, 200).Bounds().Dx(); got != 256 {
		t.Errorf("для 200 выбран значок %d", got)
	}
}

// Картинка копируется: приложение вольно рисовать в своей дальше, у системы
// под руками ничего не меняется.
func TestIconToRGBA_Copies(t *testing.T) {
	src := icon(2, color.RGBA{R: 200, G: 0, B: 0, A: 255})
	dst := iconToRGBA(src)

	src.Set(0, 0, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	if r, _, _, _ := dst.At(0, 0).RGBA(); r>>8 != 200 {
		t.Error("копия значка изменилась вслед за оригиналом")
	}
}

// Картинка со смещённым началом координат приводится к нулевому: X11 и Win32
// ждут буфер, а не прямоугольник где-то в чужой системе координат.
func TestIconToRGBA_NormalizesOrigin(t *testing.T) {
	src := image.NewRGBA(image.Rect(7, 9, 11, 13))
	src.Set(7, 9, color.RGBA{R: 1, G: 2, B: 3, A: 255})

	dst := iconToRGBA(src)
	if got := dst.Bounds(); got != image.Rect(0, 0, 4, 4) {
		t.Fatalf("границы копии %v", got)
	}
	if r, _, _, _ := dst.At(0, 0).RGBA(); r>>8 != 1 {
		t.Error("левый верхний угол копии взят не из левого верхнего угла оригинала")
	}
}
