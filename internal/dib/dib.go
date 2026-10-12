// Package dib — сборка и разбор растров в формате DIB (CF_DIB, CF_DIBV5) и BMP.
//
// Нужен буферу обмена: на Windows изображение ходит блоками DIB, а на X11 и
// Wayland часть программ отдаёт его как image/bmp — тот же DIB с файловым
// заголовком впереди. Чистые функции без системных вызовов, поэтому формат
// проверяет общий прогон тестов на любой платформе.
//
// Блоки чужие: каждая длина и каждое смещение сверяются с размером данных, а
// размеры картинки ограничены — заголовок, обещающий гигапиксели, не должен
// заставить выделить память.
package dib

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
)

// Сжатие (biCompression).
const (
	biRGB            = 0
	biBitfields      = 3
	biAlphaBitfields = 6
)

// Размеры заголовков.
const (
	coreHeaderSize = 12  // BITMAPCOREHEADER
	infoHeaderSize = 40  // BITMAPINFOHEADER
	v4HeaderSize   = 108 // BITMAPV4HEADER
	v5HeaderSize   = 124 // BITMAPV5HEADER
	fileHeaderSize = 14  // BITMAPFILEHEADER
)

// MaxPixels — предел на число точек разбираемой картинки (100 Мпкс).
const MaxPixels = 100 << 20

// Ошибки разбора.
var (
	ErrFormat      = errors.New("dib: неверный формат")
	ErrUnsupported = errors.New("dib: неподдерживаемый вариант DIB")
	ErrTooLarge    = errors.New("dib: картинка слишком велика")
)

// lcsSRGB — 'sRGB' в bV5CSType; lcsGmImages — намерение «изображения».
const (
	lcsSRGB     = 0x73524742
	lcsGmImages = 4
)

// EncodeV5 собирает CF_DIBV5: BITMAPV5HEADER и 32-битные точки BGRA.
//
// Альфа НЕ премультиплицирована, а маски каналов заданы явно (BI_BITFIELDS) —
// иначе читатель принял бы четвёртый байт за неиспользуемый. Строки идут снизу
// вверх (положительная высота): сверху вниз понимают не все программы.
func EncodeV5(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, v5HeaderSize+w*h*4)
	le := binary.LittleEndian
	le.PutUint32(out[0:], v5HeaderSize)
	le.PutUint32(out[4:], uint32(int32(w)))
	le.PutUint32(out[8:], uint32(int32(h)))
	le.PutUint16(out[12:], 1)  // biPlanes
	le.PutUint16(out[14:], 32) // biBitCount
	le.PutUint32(out[16:], biBitfields)
	le.PutUint32(out[20:], uint32(w*h*4)) // biSizeImage
	le.PutUint32(out[24:], 2835)          // 72 dpi в точках на метр
	le.PutUint32(out[28:], 2835)
	le.PutUint32(out[40:], 0x00FF0000) // bV5RedMask
	le.PutUint32(out[44:], 0x0000FF00) // bV5GreenMask
	le.PutUint32(out[48:], 0x000000FF) // bV5BlueMask
	le.PutUint32(out[52:], 0xFF000000) // bV5AlphaMask
	le.PutUint32(out[56:], lcsSRGB)    // bV5CSType
	le.PutUint32(out[108:], lcsGmImages)
	px := out[v5HeaderSize:]
	for y := 0; y < h; y++ {
		row := px[(h-1-y)*w*4:]
		for x := 0; x < w; x++ {
			r, g, bl, a := nrgba(img, b.Min.X+x, b.Min.Y+y)
			row[x*4], row[x*4+1], row[x*4+2], row[x*4+3] = bl, g, r, a
		}
	}
	return out
}

// EncodeDIB собирает CF_DIB: BITMAPINFOHEADER и 24-битные точки BGR снизу вверх.
//
// Прозрачности в таком блоке нет, поэтому полупрозрачные точки смешиваются с
// белым фоном: программа, взявшая только CF_DIB (старая или без альфы),
// получит картинку как на белом листе, а не чёрные дыры на месте прозрачного.
func EncodeDIB(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := (w*3 + 3) &^ 3
	out := make([]byte, infoHeaderSize+stride*h)
	le := binary.LittleEndian
	le.PutUint32(out[0:], infoHeaderSize)
	le.PutUint32(out[4:], uint32(int32(w)))
	le.PutUint32(out[8:], uint32(int32(h)))
	le.PutUint16(out[12:], 1)
	le.PutUint16(out[14:], 24)
	le.PutUint32(out[16:], biRGB)
	le.PutUint32(out[20:], uint32(stride*h))
	le.PutUint32(out[24:], 2835)
	le.PutUint32(out[28:], 2835)
	px := out[infoHeaderSize:]
	for y := 0; y < h; y++ {
		row := px[(h-1-y)*stride:]
		for x := 0; x < w; x++ {
			r, g, bl, a := nrgba(img, b.Min.X+x, b.Min.Y+y)
			if a != 255 {
				// Смешение с белым: c*a/255 + 255*(255-a)/255.
				r = over(r, a)
				g = over(g, a)
				bl = over(bl, a)
			}
			row[x*3], row[x*3+1], row[x*3+2] = bl, g, r
		}
	}
	return out
}

// over смешивает канал c с белым фоном при непрозрачности a.
func over(c, a uint8) uint8 {
	return uint8((uint32(c)*uint32(a) + 255*uint32(255-a) + 127) / 255)
}

// nrgba даёт точку без премультипликации (цвет, а не доля цвета).
func nrgba(img image.Image, x, y int) (r, g, b, a uint8) {
	if n, ok := img.(*image.NRGBA); ok {
		i := n.PixOffset(x, y)
		return n.Pix[i], n.Pix[i+1], n.Pix[i+2], n.Pix[i+3]
	}
	c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	return c.R, c.G, c.B, c.A
}

// EncodeBMP собирает файл BMP (image/bmp): BITMAPFILEHEADER и V5-блок.
func EncodeBMP(img image.Image) []byte {
	d := EncodeV5(img)
	out := make([]byte, fileHeaderSize+len(d))
	out[0], out[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(out[2:], uint32(len(out)))
	binary.LittleEndian.PutUint32(out[10:], fileHeaderSize+v5HeaderSize)
	copy(out[fileHeaderSize:], d)
	return out
}

// DecodeBMP разбирает файл BMP: файловый заголовок и DIB за ним. Смещение
// точек берётся из заголовка файла.
func DecodeBMP(data []byte) (image.Image, error) {
	if len(data) < fileHeaderSize || data[0] != 'B' || data[1] != 'M' {
		return nil, ErrFormat
	}
	off := int(binary.LittleEndian.Uint32(data[10:]))
	body := data[fileHeaderSize:]
	if off < fileHeaderSize || off > len(data) {
		return nil, ErrFormat
	}
	return decode(body, off-fileHeaderSize)
}

// Decode разбирает блок DIB (CF_DIB, CF_DIBV5): заголовок любого из видов
// (CORE, INFO, V4, V5), маски и палитра за ним, затем точки.
//
// Понимает 1, 4, 8 (с палитрой), 16, 24 и 32 бита; BI_RGB, BI_BITFIELDS и
// BI_ALPHABITFIELDS; строки снизу вверх и сверху вниз. Сжатие (RLE, JPEG, PNG
// внутри DIB) не поддержано — ErrUnsupported.
//
// Прозрачность берётся только из явной маски альфы (V4/V5 с BI_BITFIELDS или
// BI_ALPHABITFIELDS): четвёртый байт 32-битного BI_RGB пишут нулём даже
// программы, рисующие непрозрачное, и принять его за альфу значило бы получить
// невидимую картинку. Результат — *image.RGBA (непрозрачная) или *image.NRGBA.
func Decode(data []byte) (image.Image, error) { return decode(data, -1) }

func decode(data []byte, pixOff int) (image.Image, error) {
	le := binary.LittleEndian
	if len(data) < 4 {
		return nil, ErrFormat
	}
	hdr := int(le.Uint32(data[0:]))
	var w, h, bpp, comp, clrUsed int
	topDown := false
	switch {
	case hdr == coreHeaderSize:
		if len(data) < coreHeaderSize {
			return nil, ErrFormat
		}
		w = int(le.Uint16(data[4:]))
		h = int(le.Uint16(data[6:]))
		bpp = int(le.Uint16(data[10:]))
	case hdr >= infoHeaderSize && hdr <= 4096:
		if len(data) < hdr {
			return nil, ErrFormat
		}
		w = int(int32(le.Uint32(data[4:])))
		h = int(int32(le.Uint32(data[8:])))
		bpp = int(le.Uint16(data[14:]))
		comp = int(le.Uint32(data[16:]))
		clrUsed = int(le.Uint32(data[32:]))
		if h < 0 {
			h, topDown = -h, true
		}
	default:
		return nil, ErrFormat
	}
	if w <= 0 || h <= 0 || w > 1<<16 || h > 1<<16 {
		return nil, ErrFormat
	}
	if w*h > MaxPixels {
		return nil, ErrTooLarge
	}
	switch bpp {
	case 1, 4, 8, 16, 24, 32:
	default:
		return nil, ErrUnsupported
	}

	// Маски каналов: в самом заголовке (V4/V5) или за ним (INFO).
	var rm, gm, bm, am uint32
	pos := hdr // где кончаются заголовок и маски
	hasMasks := false
	switch comp {
	case biRGB:
	case biBitfields, biAlphaBitfields:
		if bpp != 16 && bpp != 32 {
			return nil, ErrUnsupported
		}
		hasMasks = true
		if hdr >= v4HeaderSize {
			rm, gm, bm, am = le.Uint32(data[40:]), le.Uint32(data[44:]), le.Uint32(data[48:]), le.Uint32(data[52:])
		} else if hdr >= 52 {
			// INFO с расширением V2/V3: маски внутри заголовка.
			rm, gm, bm = le.Uint32(data[40:]), le.Uint32(data[44:]), le.Uint32(data[48:])
			if hdr >= 56 {
				am = le.Uint32(data[52:])
			}
		} else {
			n := 12
			if comp == biAlphaBitfields {
				n = 16
			}
			if len(data) < pos+n {
				return nil, ErrFormat
			}
			rm, gm, bm = le.Uint32(data[pos:]), le.Uint32(data[pos+4:]), le.Uint32(data[pos+8:])
			if n == 16 {
				am = le.Uint32(data[pos+12:])
			}
			pos += n
		}
	default:
		return nil, ErrUnsupported
	}
	if !hasMasks {
		switch bpp {
		case 16: // BI_RGB 16 бит — 5-5-5
			rm, gm, bm = 0x7C00, 0x03E0, 0x001F
		case 32:
			rm, gm, bm = 0x00FF0000, 0x0000FF00, 0x000000FF
		}
	}

	// Палитра.
	var pal []color.RGBA
	if bpp <= 8 {
		n := clrUsed
		if n == 0 || n > 1<<bpp {
			n = 1 << bpp
		}
		entry := 4
		if hdr == coreHeaderSize {
			entry = 3
		}
		if len(data) < pos+n*entry {
			return nil, ErrFormat
		}
		pal = make([]color.RGBA, n)
		for i := range pal {
			p := data[pos+i*entry:]
			pal[i] = color.RGBA{R: p[2], G: p[1], B: p[0], A: 255}
		}
		pos += n * entry
	} else if clrUsed > 0 && hdr != coreHeaderSize {
		pos += clrUsed * 4 // палитра при истинном цвете — подсказка для 8-битных экранов
	}

	if pixOff >= 0 {
		pos = pixOff
	}
	if pos > len(data) {
		return nil, ErrFormat
	}
	stride := ((w*bpp + 31) / 32) * 4
	if need := stride * h; len(data)-pos < need {
		return nil, ErrFormat
	}
	px := data[pos:]

	useAlpha := am != 0
	var rs, gs, bs, as maskShift
	var out *image.NRGBA
	var outRGBA *image.RGBA
	if useAlpha {
		out = image.NewNRGBA(image.Rect(0, 0, w, h))
	} else {
		outRGBA = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	if bpp == 16 || bpp == 32 {
		rs, gs, bs, as = newMask(rm), newMask(gm), newMask(bm), newMask(am)
	}
	fast := bpp == 24 || (bpp == 32 && rm == 0x00FF0000 && gm == 0x0000FF00 && bm == 0x000000FF &&
		(am == 0 || am == 0xFF000000))

	for y := 0; y < h; y++ {
		sy := h - 1 - y
		if topDown {
			sy = y
		}
		row := px[sy*stride : sy*stride+stride]
		if fast {
			// Обычный BGR/BGRA: большие снимки экрана идут именно так, и
			// разбор по маскам на каждой точке стоил бы заметно дороже.
			if useAlpha {
				dst := out.Pix[y*out.Stride : y*out.Stride+w*4]
				for x := 0; x < w; x++ {
					dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = row[x*4+2], row[x*4+1], row[x*4], row[x*4+3]
				}
			} else {
				dst := outRGBA.Pix[y*outRGBA.Stride : y*outRGBA.Stride+w*4]
				step := bpp / 8
				for x := 0; x < w; x++ {
					dst[x*4], dst[x*4+1], dst[x*4+2], dst[x*4+3] = row[x*step+2], row[x*step+1], row[x*step], 255
				}
			}
			continue
		}
		for x := 0; x < w; x++ {
			var r, g, b, a uint8 = 0, 0, 0, 255
			switch bpp {
			case 1:
				c := pal[min(int(row[x>>3]>>(7-uint(x&7))&1), len(pal)-1)]
				r, g, b = c.R, c.G, c.B
			case 4:
				v := row[x>>1]
				if x&1 == 0 {
					v >>= 4
				}
				c := pal[min(int(v&15), len(pal)-1)]
				r, g, b = c.R, c.G, c.B
			case 8:
				c := pal[min(int(row[x]), len(pal)-1)]
				r, g, b = c.R, c.G, c.B
			case 16:
				v := uint32(le.Uint16(row[x*2:]))
				r, g, b = rs.get(v), gs.get(v), bs.get(v)
				if useAlpha {
					a = as.get(v)
				}
			case 24:
				b, g, r = row[x*3], row[x*3+1], row[x*3+2]
			case 32:
				v := le.Uint32(row[x*4:])
				r, g, b = rs.get(v), gs.get(v), bs.get(v)
				if useAlpha {
					a = as.get(v)
				}
			}
			if useAlpha {
				i := out.PixOffset(x, y)
				out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = r, g, b, a
			} else {
				i := outRGBA.PixOffset(x, y)
				outRGBA.Pix[i], outRGBA.Pix[i+1], outRGBA.Pix[i+2], outRGBA.Pix[i+3] = r, g, b, 255
			}
		}
	}
	if useAlpha {
		return out, nil
	}
	return outRGBA, nil
}

// maskShift вытаскивает канал из слова по маске и растягивает его до 8 бит.
type maskShift struct {
	mask  uint32
	shift uint
	bits  uint
}

func newMask(mask uint32) maskShift {
	m := maskShift{mask: mask}
	if mask == 0 {
		return m
	}
	for mask&1 == 0 {
		mask >>= 1
		m.shift++
	}
	for mask&1 == 1 {
		mask >>= 1
		m.bits++
	}
	return m
}

func (m maskShift) get(v uint32) uint8 {
	if m.mask == 0 || m.bits == 0 {
		return 0
	}
	c := (v & m.mask) >> m.shift
	if m.bits >= 8 {
		return uint8(c >> (m.bits - 8))
	}
	// Растяжение до 8 бит с сохранением крайних значений: 0 → 0, max → 255.
	maxv := uint32(1)<<m.bits - 1
	return uint8((c*255 + maxv/2) / maxv)
}
