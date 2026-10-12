package dib

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

// testImage — картинка с разной прозрачностью и неквадратная (шаг строки DIB
// кратен четырём байтам, и ширина 3 это проверяет).
func testImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	cols := []color.NRGBA{
		{255, 0, 0, 255}, {0, 255, 0, 128}, {0, 0, 255, 0},
		{10, 20, 30, 255}, {200, 100, 50, 77}, {1, 2, 3, 254},
	}
	for i, c := range cols {
		img.SetNRGBA(i%3, i/3, c)
	}
	return img
}

func sameNRGBA(t *testing.T, got image.Image, want *image.NRGBA) {
	t.Helper()
	if got.Bounds().Dx() != want.Bounds().Dx() || got.Bounds().Dy() != want.Bounds().Dy() {
		t.Fatalf("размер %v, ждал %v", got.Bounds(), want.Bounds())
	}
	for y := 0; y < want.Bounds().Dy(); y++ {
		for x := 0; x < want.Bounds().Dx(); x++ {
			g := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
			w := want.NRGBAAt(x, y)
			if g != w {
				t.Errorf("(%d,%d): %v, ждал %v", x, y, g, w)
			}
		}
	}
}

// Круговая проверка CF_DIBV5: цвета и альфа точно, без премультипликации.
func TestV5RoundTripKeepsAlpha(t *testing.T) {
	src := testImage()
	d := EncodeV5(src)
	if got := binary.LittleEndian.Uint32(d[0:]); got != v5HeaderSize {
		t.Fatalf("размер заголовка %d", got)
	}
	if binary.LittleEndian.Uint16(d[14:]) != 32 || binary.LittleEndian.Uint32(d[16:]) != biBitfields {
		t.Fatal("ждал 32 бита и BI_BITFIELDS")
	}
	// Первая строка в блоке — нижняя строка картинки (снизу вверх): (0,1) = {10,20,30,255}.
	if p := d[v5HeaderSize : v5HeaderSize+4]; p[0] != 30 || p[1] != 20 || p[2] != 10 || p[3] != 255 {
		t.Errorf("первая точка блока BGRA = %v", p)
	}
	got, err := Decode(d)
	if err != nil {
		t.Fatal(err)
	}
	sameNRGBA(t, got, src)
	if _, ok := got.(*image.NRGBA); !ok {
		t.Errorf("с альфой ждал *image.NRGBA, получил %T", got)
	}
}

// CF_DIB: 24 бита, прозрачность смешана с белым; непрозрачные точки — точно.
func TestDIBRoundTripCompositesOverWhite(t *testing.T) {
	src := testImage()
	d := EncodeDIB(src)
	if binary.LittleEndian.Uint16(d[14:]) != 24 {
		t.Fatal("ждал 24 бита")
	}
	got, err := Decode(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(*image.RGBA); !ok {
		t.Fatalf("без альфы ждал *image.RGBA, получил %T", got)
	}
	exact := func(x, y int, want color.RGBA) {
		t.Helper()
		if c := got.At(x, y).(color.RGBA); c != want {
			t.Errorf("(%d,%d) = %v, ждал %v", x, y, c, want)
		}
	}
	exact(0, 0, color.RGBA{255, 0, 0, 255})
	exact(0, 1, color.RGBA{10, 20, 30, 255})
	exact(2, 0, color.RGBA{255, 255, 255, 255}) // полностью прозрачная → белый
	// Полупрозрачный зелёный на белом: R,B → 255*127/255 ≈ 127, G → 255.
	c := got.At(1, 0).(color.RGBA)
	if c.G != 255 || c.R < 125 || c.R > 130 || c.B != c.R {
		t.Errorf("полупрозрачная точка на белом: %v", c)
	}
}

// Сверху-вниз (отрицательная высота) читается в том же порядке строк.
func TestDecodeTopDown(t *testing.T) {
	src := testImage()
	d := EncodeV5(src)
	// Перевернуть: отрицательная высота и строки в обратном порядке.
	w, h := 3, 2
	flipped := append([]byte(nil), d...)
	binary.LittleEndian.PutUint32(flipped[8:], uint32(int32(-h)))
	px := flipped[v5HeaderSize:]
	orig := d[v5HeaderSize:]
	for y := 0; y < h; y++ {
		copy(px[y*w*4:(y+1)*w*4], orig[(h-1-y)*w*4:(h-y)*w*4])
	}
	got, err := Decode(flipped)
	if err != nil {
		t.Fatal(err)
	}
	sameNRGBA(t, got, src)
}

// BI_BITFIELDS с заголовком INFO: маски идут за заголовком (так кладёт часть
// программ в CF_DIB).
func TestDecodeInfoHeaderWithBitfields(t *testing.T) {
	const w, h = 2, 1
	d := make([]byte, infoHeaderSize+12+w*4)
	le := binary.LittleEndian
	le.PutUint32(d[0:], infoHeaderSize)
	le.PutUint32(d[4:], w)
	le.PutUint32(d[8:], h)
	le.PutUint16(d[12:], 1)
	le.PutUint16(d[14:], 32)
	le.PutUint32(d[16:], biBitfields)
	// RGBA-порядок вместо BGRA: красный в младшем байте.
	le.PutUint32(d[40:], 0x000000FF)
	le.PutUint32(d[44:], 0x0000FF00)
	le.PutUint32(d[48:], 0x00FF0000)
	le.PutUint32(d[infoHeaderSize+12:], 0x00_30_20_10) // R=0x10 G=0x20 B=0x30
	le.PutUint32(d[infoHeaderSize+16:], 0x00_66_55_44)
	got, err := Decode(d)
	if err != nil {
		t.Fatal(err)
	}
	if c := got.At(0, 0).(color.RGBA); c != (color.RGBA{0x10, 0x20, 0x30, 255}) {
		t.Errorf("(0,0) = %v", c)
	}
	if c := got.At(1, 0).(color.RGBA); c != (color.RGBA{0x44, 0x55, 0x66, 255}) {
		t.Errorf("(1,0) = %v", c)
	}
}

// 32-битный BI_RGB: четвёртый байт — не альфа (его пишут нулём), картинка
// остаётся видимой.
func TestDecode32BitRGBIgnoresFourthByte(t *testing.T) {
	d := make([]byte, infoHeaderSize+4)
	le := binary.LittleEndian
	le.PutUint32(d[0:], infoHeaderSize)
	le.PutUint32(d[4:], 1)
	le.PutUint32(d[8:], 1)
	le.PutUint16(d[12:], 1)
	le.PutUint16(d[14:], 32)
	d[infoHeaderSize], d[infoHeaderSize+1], d[infoHeaderSize+2], d[infoHeaderSize+3] = 1, 2, 3, 0
	got, err := Decode(d)
	if err != nil {
		t.Fatal(err)
	}
	if c := got.At(0, 0).(color.RGBA); c != (color.RGBA{3, 2, 1, 255}) {
		t.Errorf("точка %v", c)
	}
}

// Палитровый DIB (8 и 4 бита) и 16-битный 5-6-5.
func TestDecodePaletteAnd16Bit(t *testing.T) {
	le := binary.LittleEndian
	// 8 бит, палитра из двух цветов.
	d := make([]byte, infoHeaderSize+8+4)
	le.PutUint32(d[0:], infoHeaderSize)
	le.PutUint32(d[4:], 2)
	le.PutUint32(d[8:], 1)
	le.PutUint16(d[12:], 1)
	le.PutUint16(d[14:], 8)
	le.PutUint32(d[32:], 2) // biClrUsed
	copy(d[infoHeaderSize:], []byte{0, 0, 255, 0 /* красный BGRX */, 255, 0, 0, 0 /* синий */})
	copy(d[infoHeaderSize+8:], []byte{1, 0, 0, 0})
	got, err := Decode(d)
	if err != nil {
		t.Fatal(err)
	}
	if c := got.At(0, 0).(color.RGBA); c != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("палитра[1] = %v", c)
	}
	if c := got.At(1, 0).(color.RGBA); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("палитра[0] = %v", c)
	}

	// 16 бит 5-6-5: чистый красный 0xF800 → (255,0,0).
	d = make([]byte, infoHeaderSize+12+4)
	le.PutUint32(d[0:], infoHeaderSize)
	le.PutUint32(d[4:], 1)
	le.PutUint32(d[8:], 1)
	le.PutUint16(d[12:], 1)
	le.PutUint16(d[14:], 16)
	le.PutUint32(d[16:], biBitfields)
	le.PutUint32(d[infoHeaderSize:], 0xF800)
	le.PutUint32(d[infoHeaderSize+4:], 0x07E0)
	le.PutUint32(d[infoHeaderSize+8:], 0x001F)
	le.PutUint16(d[infoHeaderSize+12:], 0xF800)
	got, err = Decode(d)
	if err != nil {
		t.Fatal(err)
	}
	if c := got.At(0, 0).(color.RGBA); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("565 красный = %v", c)
	}
}

// BMP-файл: тот же блок под файловым заголовком; смещение точек — из него.
func TestBMPRoundTrip(t *testing.T) {
	src := testImage()
	f := EncodeBMP(src)
	if string(f[:2]) != "BM" {
		t.Fatal("нет сигнатуры BM")
	}
	got, err := DecodeBMP(f)
	if err != nil {
		t.Fatal(err)
	}
	sameNRGBA(t, got, src)
	// DIB, поданный как BMP, и наоборот — ошибка формата, а не мусор.
	if _, err := DecodeBMP(EncodeV5(src)); err == nil {
		t.Error("блок без файлового заголовка принят как BMP")
	}
}

// Чужие блоки: усечённые, с вранём в заголовке, с неподдержанным сжатием.
func TestDecodeRejectsBadInput(t *testing.T) {
	good := EncodeV5(testImage())
	for name, d := range map[string][]byte{
		"пусто":          nil,
		"обрезан":        good[:len(good)-1],
		"только заголов": good[:v5HeaderSize],
		"заголовок":      good[:30],
	} {
		if _, err := Decode(d); err == nil {
			t.Errorf("%s: принят", name)
		}
	}
	le := binary.LittleEndian

	big := append([]byte(nil), good...)
	le.PutUint32(big[4:], 60000)
	le.PutUint32(big[8:], 60000)
	if _, err := Decode(big); err != ErrTooLarge {
		t.Errorf("гигантские размеры: %v", err)
	}

	rle := append([]byte(nil), good...)
	le.PutUint32(rle[16:], 1) // BI_RLE8
	if _, err := Decode(rle); err != ErrUnsupported {
		t.Errorf("RLE: %v", err)
	}

	zero := append([]byte(nil), good...)
	le.PutUint32(zero[4:], 0)
	if _, err := Decode(zero); err == nil {
		t.Error("нулевая ширина принята")
	}

	// Палитра, обещающая больше, чем есть.
	pal := make([]byte, infoHeaderSize)
	le.PutUint32(pal[0:], infoHeaderSize)
	le.PutUint32(pal[4:], 1)
	le.PutUint32(pal[8:], 1)
	le.PutUint16(pal[12:], 1)
	le.PutUint16(pal[14:], 8)
	if _, err := Decode(pal); err == nil {
		t.Error("палитра за границей принята")
	}
}
