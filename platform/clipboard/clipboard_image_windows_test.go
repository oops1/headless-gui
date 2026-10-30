//go:build windows

package clipboard

import (
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/clipimage"
	"github.com/oops1/headless-gui/v4/internal/dib"
)

// Тесты настоящего буфера обмена Windows. Они затирают его содержимое, поэтому
// на машине разработчика запускаются только явно
// (HEADLESS_GUI_TEST_SYSCLIPBOARD=1), а на CI — сами.
func needSysClipboard(t *testing.T) *winClipboard {
	t.Helper()
	if os.Getenv("HEADLESS_GUI_TEST_SYSCLIPBOARD") == "" && os.Getenv("CI") == "" {
		t.Skip("затирает системный буфер обмена: HEADLESS_GUI_TEST_SYSCLIPBOARD=1 для запуска")
	}
	return &winClipboard{} // не глобальный провайдер: соседние тесты подменяют его на память
}

// rawFormat достаёт блок буфера в указанном формате ("" → нет формата).
func rawFormat(t *testing.T, format uintptr) []byte {
	t.Helper()
	if !openClipboardRetry() {
		t.Skip("буфер обмена занят другим процессом")
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(format)
	if h == 0 {
		return nil
	}
	return clipboardBlock(h)
}

func winTestImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 5, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(40 * x), uint8(80 * y), 200, 255})
		}
	}
	img.SetNRGBA(1, 1, color.NRGBA{10, 200, 30, 100}) // полупрозрачная
	return img
}

// SetImage кладёт все три формата, и каждый разбирается обратно.
func TestWinClipboard_SetImageAllFormats(t *testing.T) {
	c := needSysClipboard(t)
	src := winTestImage()
	if err := c.SetImage(src); err != nil {
		t.Fatal(err)
	}

	pngBlock := rawFormat(t, pngFormat())
	if pngBlock == nil {
		t.Fatal("формат PNG не положен")
	}
	pimg, err := clipimage.DecodePNG(pngBlock)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(pimg.At(1, 1)).(color.NRGBA); got != (color.NRGBA{10, 200, 30, 100}) {
		t.Errorf("PNG (1,1) = %v", got)
	}

	v5 := rawFormat(t, cfDibV5)
	if len(v5) < 124 || binary.LittleEndian.Uint16(v5[14:]) != 32 {
		t.Fatalf("CF_DIBV5: ждал 32 бита, блок %d байт", len(v5))
	}
	vimg, err := dib.Decode(v5)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(vimg.At(1, 1)).(color.NRGBA); got != (color.NRGBA{10, 200, 30, 100}) {
		t.Errorf("CF_DIBV5 (1,1) = %v: альфа или порядок каналов", got)
	}

	plain := rawFormat(t, cfDib)
	if len(plain) < 40 {
		t.Fatalf("CF_DIB не положен (%d байт)", len(plain))
	}
	if _, err := dib.Decode(plain); err != nil {
		t.Errorf("CF_DIB не разбирается: %v", err)
	}

	// И сквозное чтение через widget.
	got, ok := c.GetImage()
	if !ok || got.Bounds().Dx() != 5 || got.Bounds().Dy() != 3 {
		t.Fatalf("GetImage: %v %v", got, ok)
	}
}

// Порядок чтения: если PNG нет, берётся CF_DIBV5 (с альфой), если нет и его —
// CF_DIB.
func TestWinClipboard_GetImageFallbackOrder(t *testing.T) {
	c := needSysClipboard(t)
	src := winTestImage()

	put := func(formats map[uintptr][]byte) {
		t.Helper()
		if !openClipboardRetry() {
			t.Skip("буфер обмена занят другим процессом")
		}
		defer procCloseClipboard.Call()
		procEmptyClipboard.Call()
		for f, d := range formats {
			setClipboardBytes(f, d)
		}
	}

	put(map[uintptr][]byte{cfDibV5: dib.EncodeV5(src)})
	got, ok := c.GetImage()
	if !ok {
		t.Fatal("CF_DIBV5 без PNG не прочитан")
	}
	if a := color.NRGBAModel.Convert(got.At(1, 1)).(color.NRGBA).A; a != 100 {
		t.Errorf("альфа из CF_DIBV5 = %d", a)
	}

	put(map[uintptr][]byte{cfDib: dib.EncodeDIB(src)})
	got, ok = c.GetImage()
	if !ok {
		t.Fatal("CF_DIB без остальных не прочитан")
	}
	if a := color.NRGBAModel.Convert(got.At(0, 0)).(color.NRGBA).A; a != 255 {
		t.Errorf("CF_DIB ждал непрозрачным, альфа %d", a)
	}

	// Текст в буфере — не картинка.
	c.SetText("не картинка")
	if _, ok := c.GetImage(); ok {
		t.Error("текст принят за изображение")
	}
}
