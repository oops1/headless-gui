// trayglyph.go — значки трея из набора иконок темы (и растеризация под HiDPI).
//
// Значки сети, звука и питания раньше рисовались только фигурами. Теперь
// сначала ищется иконка в наборе темы (tm.GetIcon по ключам из профиля), и
// только если темы с такой иконкой нет — рисуются прежние фигуры. Профиль, не
// объявивший ключей, выглядит как раньше, пиксель в пиксель.
//
// Иконка запрашивается у набора в ФИЗИЧЕСКОМ размере (логический × масштаб
// холста), а не в логическом: векторная иконка, растеризованная под размер
// экрана, остаётся чёткой на 125–200 %, тогда как готовый растр, растянутый
// движком, на дробном масштабе мягкий.
package desktop

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Ключи иконок темы для значков трея (theme.Profile.Icons). Профиль задаёт
// любое подмножество: из объявленных берётся самый конкретный ключ, при его
// отсутствии — более общий, а если нет ни одного — рисуются фигуры.
//
//	сеть:    tray.network.none | tray.network.wifi.N (N=0..4, по Quality) |
//	         tray.network.wifi | tray.network.ethernet | tray.network.cellular |
//	         tray.network.icon (общий)
//	звук:    tray.volume.muted | tray.volume.N (N=0..3, по Level) | tray.volume.icon
//	питание: tray.power.ac.N | tray.power.ac | tray.power.N (N=0..10, по Charge) |
//	         tray.power.icon
//	уведомления: tray.notifications.icon.new (есть новые) | tray.notifications.icon
const (
	KeyTrayNetworkIcon       theme.Key = "tray.network.icon"
	KeyTrayVolumeIcon        theme.Key = "tray.volume.icon"
	KeyTrayPowerIcon         theme.Key = "tray.power.icon"
	KeyTrayNotificationsIcon theme.Key = "tray.notifications.icon"

	// KeyTrayIconTint — признак темы: перекрасить иконку трея цветом текста
	// стиля (по альфа-каналу иконки). Нужен одноцветным наборам, которые
	// отдают белые значки независимо от светлой или тёмной панели.
	KeyTrayIconTint theme.Key = "tray.icon.tint"
)

// Число делений в именах иконок: уровни сигнала, громкости и заряда.
const (
	volumeIconLevels = 3  // tray.volume.0..3
	powerIconLevels  = 10 // tray.power.0..10
)

// networkIconKeys — ключи иконки сети от конкретного к общему.
func networkIconKeys(net NetState) []theme.Key {
	switch net.Kind {
	case NetNone:
		return []theme.Key{"tray.network.none"}
	case NetWiFi:
		return []theme.Key{
			theme.Key(fmt.Sprintf("tray.network.wifi.%d", quantize(net.Quality, networkBars))),
			"tray.network.wifi", KeyTrayNetworkIcon,
		}
	case NetEthernet:
		return []theme.Key{"tray.network.ethernet", KeyTrayNetworkIcon}
	case NetCellular:
		return []theme.Key{
			theme.Key(fmt.Sprintf("tray.network.cellular.%d", quantize(net.Quality, networkBars))),
			"tray.network.cellular", KeyTrayNetworkIcon,
		}
	}
	return []theme.Key{KeyTrayNetworkIcon}
}

// volumeIconKeys — ключи иконки звука.
func volumeIconKeys(v VolState) []theme.Key {
	if v.Muted {
		return []theme.Key{"tray.volume.muted"}
	}
	// 0 — совсем тихо; остальное делится на volumeIconLevels ступеней 1..N.
	n := 0
	if v.Level > 0 {
		n = 1 + int(math.Min(v.Level, 0.999)*volumeIconLevels)
		if n > volumeIconLevels {
			n = volumeIconLevels
		}
	}
	return []theme.Key{theme.Key(fmt.Sprintf("tray.volume.%d", n)), KeyTrayVolumeIcon}
}

// powerIconKeys — ключи иконки питания.
func powerIconKeys(p PowerState) []theme.Key {
	n := quantize(p.Charge, powerIconLevels)
	if p.OnAC {
		return []theme.Key{
			theme.Key(fmt.Sprintf("tray.power.ac.%d", n)), "tray.power.ac",
			theme.Key(fmt.Sprintf("tray.power.%d", n)), KeyTrayPowerIcon,
		}
	}
	return []theme.Key{theme.Key(fmt.Sprintf("tray.power.%d", n)), KeyTrayPowerIcon}
}

// quantize переводит долю [0,1] в целое 0..steps.
func quantize(ratio float64, steps int) int {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return int(math.Round(ratio * float64(steps)))
}

// glyphSquare вписывает в r квадрат по центру: иконки квадратные, а прямоугольник
// значка (с отступами стиля) квадратным быть не обязан.
func glyphSquare(r image.Rectangle) image.Rectangle {
	side := r.Dx()
	if r.Dy() < side {
		side = r.Dy()
	}
	if side <= 0 {
		return image.Rectangle{}
	}
	x := r.Min.X + (r.Dx()-side)/2
	y := r.Min.Y + (r.Dy()-side)/2
	return image.Rect(x, y, x+side, y+side)
}

// glyphMemo помнит перекрашенную копию иконки: набор иконок отдаёт одну и ту же
// картинку на один размер, а перекраска стоит прохода по пикселям. Трогается
// только из Draw (горутина кадра), поэтому без замка.
type glyphMemo struct {
	src *image.RGBA
	col color.RGBA
	out *image.RGBA
}

// tinted возвращает копию img, закрашенную цветом col с сохранением альфы.
func (m *glyphMemo) tinted(img image.Image, col color.RGBA) image.Image {
	rgba, ok := img.(*image.RGBA)
	if ok && m.out != nil && m.src == rgba && m.col == col {
		return m.out
	}
	out := tintImage(img, col)
	if ok {
		m.src, m.col, m.out = rgba, col, out
	}
	return out
}

// tintImage строит копию src, у которой цвет всюду col, а прозрачность — как у
// src (альфа-канал иконки как маска).
func tintImage(src image.Image, col color.RGBA) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			_, _, _, a16 := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			a := uint32(a16 >> 8)
			if a == 0 {
				continue
			}
			// Альфа цвета участвует тоже: цвет с A<255 даёт полупрозрачный глиф.
			k := a * uint32(col.A) / 255
			o := out.PixOffset(x, y)
			out.Pix[o+0] = uint8(uint32(col.R) * a / 255)
			out.Pix[o+1] = uint8(uint32(col.G) * a / 255)
			out.Pix[o+2] = uint8(uint32(col.B) * a / 255)
			out.Pix[o+3] = uint8(k)
		}
	}
	return out
}

// themeGlyph возвращает иконку темы для первого найденного ключа в размере
// side×side физических пикселей; nil — темы, разрешателя или иконки нет.
func themeGlyph(tm *theme.Manager, keys []theme.Key, side int) image.Image {
	if tm == nil || side <= 0 {
		return nil
	}
	for _, k := range keys {
		if img := tm.GetIcon(string(k), side); img != nil {
			return img
		}
	}
	return nil
}

// drawThemeGlyph рисует иконку темы в квадрате по центру inner. true — иконка
// нарисована, фигуры рисовать не нужно; false — иконки в теме нет.
func drawThemeGlyph(ctx widget.DrawContext, tm *theme.Manager, keys []theme.Key,
	inner image.Rectangle, s *theme.Style, memo *glyphMemo) bool {

	if tm == nil {
		return false
	}
	box := glyphSquare(inner)
	if box.Empty() {
		return false
	}
	// Физический размер: векторная иконка растеризуется под экран, а не
	// растягивается из логического размера.
	p := widget.PhysicalRect(ctx, box)
	side := p.X
	if p.Y < side {
		side = p.Y
	}
	img := themeGlyph(tm, keys, side)
	if img == nil {
		return false
	}
	if tm.GetFlag(KeyTrayIconTint, false) {
		img = memo.tinted(img, ink(s))
	}
	ctx.DrawImageScaled(img, box.Min.X, box.Min.Y, box.Dx(), box.Dy())
	return true
}
