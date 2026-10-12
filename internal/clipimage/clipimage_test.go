package clipimage

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/dib"
)

func picture() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 1, color.NRGBA{10, 200, 30, 90})
	img.SetNRGBA(3, 2, color.NRGBA{1, 2, 3, 255})
	return img
}

// Выбор типа: PNG всегда лучше BMP, в каком бы порядке ни пришли; синонимы BMP
// принимаются; чужое не берётся; возвращается тип в точности как объявлен.
func TestPickMime(t *testing.T) {
	for _, tc := range []struct {
		offered []string
		want    string
	}{
		{[]string{"image/png", "image/bmp"}, "image/png"},
		{[]string{"image/bmp", "image/png"}, "image/png"},
		{[]string{"text/plain", "image/bmp"}, "image/bmp"},
		{[]string{"image/x-bmp"}, "image/x-bmp"},
		{[]string{"image/x-MS-bmp", "image/x-bmp"}, "image/x-MS-bmp"},
		{[]string{"image/PNG; charset=binary"}, "image/PNG; charset=binary"},
		{[]string{"image/jpeg", "image/gif", "text/uri-list"}, ""},
		{nil, ""},
	} {
		if got := PickMime(tc.offered); got != tc.want {
			t.Errorf("PickMime(%v) = %q, ждал %q", tc.offered, got, tc.want)
		}
	}
}

func TestDecodeMime(t *testing.T) {
	src := picture()
	png, err := EncodePNG(src)
	if err != nil {
		t.Fatal(err)
	}
	bmp := dib.EncodeBMP(src)

	for name, tc := range map[string]struct {
		mime string
		data []byte
	}{
		"png":   {"image/png", png},
		"bmp":   {"image/bmp", bmp},
		"x-bmp": {"image/x-bmp", bmp},
	} {
		img, err := DecodeMime(tc.mime, tc.data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for y := 0; y < 3; y++ {
			for x := 0; x < 4; x++ {
				got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				if want := src.NRGBAAt(x, y); got != want {
					t.Errorf("%s (%d,%d) = %v, ждал %v", name, x, y, got, want)
				}
			}
		}
	}
	if _, err := DecodeMime("image/jpeg", png); err != ErrUnknownMime {
		t.Errorf("jpeg: %v", err)
	}
	// Тип и данные не совпали — ошибка, не мусор.
	if _, err := DecodeMime("image/png", bmp); err == nil {
		t.Error("BMP под видом PNG принят")
	}
	if _, err := DecodeMime("image/bmp", png); err == nil {
		t.Error("PNG под видом BMP принят")
	}
}

// PNG-бомба: заголовок обещает гигапиксели — отказ до выделения памяти.
func TestDecodePNGRejectsBomb(t *testing.T) {
	var b bytes.Buffer
	b.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 60000)
	binary.BigEndian.PutUint32(ihdr[4:], 60000)
	ihdr[8], ihdr[9] = 8, 6
	chunk := append([]byte("IHDR"), ihdr...)
	binary.Write(&b, binary.BigEndian, uint32(13))
	b.Write(chunk)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	if _, err := DecodePNG(b.Bytes()); err != ErrTooLarge {
		t.Errorf("бомба: %v", err)
	}
	if _, err := DecodePNG([]byte("не png")); err == nil {
		t.Error("мусор принят")
	}
}

func TestCloneIsIndependent(t *testing.T) {
	src := image.NewRGBA(image.Rect(3, 4, 6, 6)) // начало не в нуле, премультиплицированный тип
	src.Set(3, 4, color.NRGBA{200, 100, 50, 128})
	cp := Clone(src)
	if cp.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Errorf("границы %v", cp.Bounds())
	}
	src.Set(3, 4, color.NRGBA{0, 0, 0, 255})
	if got := cp.NRGBAAt(0, 0); got.A != 128 || got.R < 195 || got.R > 205 {
		t.Errorf("копия изменилась вместе с оригиналом или потеряла альфу: %v", got)
	}
}
