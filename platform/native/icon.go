package native

// icon.go — значок окна: контракт бэкенда, выбор размера, _NET_WM_ICON.

import (
	"encoding/binary"
	"errors"
	"image"
	"image/draw"
)

// ErrIconUnsupported возвращается, когда бэкенд не умеет менять значок окна.
//
// Отдельная ошибка, а не молчание: приложение, у которого значка не видно,
// должно узнать причину здесь, а не искать её в настройках рабочего стола.
var ErrIconUnsupported = errors.New("window: смена значка окна не поддержана этим бэкендом")

// IconSetter — опциональная возможность бэкенда сменить значок окна.
//
// Опциональная, а не часть NativeWindow: не всякий бэкенд это умеет (macOS не
// умеет; Wayland — только если компоновщик объявил xdg-toplevel-icon-v1), и
// требовать метод от всех значило бы заставить их писать заглушки.
//
//facadegen:skip
type IconSetter interface {
	SetIcon(icons []image.Image) error
}

// IconToRGBA приводит картинку к *image.RGBA (копия, если тип уже тот).
//
// Копия нужна и в этом случае: дальше буфер уходит в системные вызовы, и
// картинка, которую приложение продолжает рисовать, менялась бы у системы под
// руками.
//
//facadegen:skip
func IconToRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// NetWMIconData собирает содержимое _NET_WM_ICON.
//
// Пустой результат для пустого списка — свойство заменяется пустым, и значок
// снимается: ChangeProperty идёт в режиме Replace.
//
//facadegen:skip
func NetWMIconData(icons []image.Image) []byte {
	total := 0
	for _, ic := range icons {
		b := ic.Bounds()
		total += 2 + b.Dx()*b.Dy()
	}
	out := make([]byte, 0, total*4)
	var num [4]byte
	put := func(v uint32) {
		binary.LittleEndian.PutUint32(num[:], v)
		out = append(out, num[:]...)
	}
	for _, ic := range icons {
		rgba := IconToRGBA(ic)
		b := rgba.Bounds()
		put(uint32(b.Dx()))
		put(uint32(b.Dy()))
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				i := rgba.PixOffset(x, y)
				// ARGB одним числом — так требует EWMH. Порядок байт в
				// свойстве задаёт протокол (числа, а не байты картинки),
				// поэтому собираем значение арифметикой, а не копированием.
				put(uint32(rgba.Pix[i+3])<<24 |
					uint32(rgba.Pix[i])<<16 |
					uint32(rgba.Pix[i+1])<<8 |
					uint32(rgba.Pix[i+2]))
			}
		}
	}
	return out
}

// PickIcon выбирает картинку, ближайшую по размеру к want.
//
// Ближайшую, а не первую подходящую: приложение перечисляет размеры в том
// порядке, в каком ему удобно, и угадывать по порядку — значит поставить в
// заголовок окна картинку 256×256, ужатую системой до шестнадцати точек.
//
//facadegen:skip
func PickIcon(icons []image.Image, want int) image.Image {
	var best image.Image
	bestDiff := 1 << 30
	for _, ic := range icons {
		b := ic.Bounds()
		side := b.Dx()
		if b.Dy() > side {
			side = b.Dy()
		}
		d := side - want
		if d < 0 {
			d = -d
		}
		if d < bestDiff {
			best, bestDiff = ic, d
		}
	}
	return best
}
