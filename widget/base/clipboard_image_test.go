package base

import (
	"errors"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/clipimage"
	"github.com/oops1/headless-gui/v4/internal/dib"
)

func testClipImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 0, color.NRGBA{0, 255, 0, 128})
	img.SetNRGBA(2, 1, color.NRGBA{1, 2, 3, 4})
	return img
}

// Буфер в памяти: картинка уходит копией и возвращается копией, текст и
// файлы вытесняются, и наоборот.
func TestMemoryClipboard_ImageRoundTrip(t *testing.T) {
	UseMemoryClipboard()
	src := testClipImage()
	if err := ClipboardSetImage(src); err != nil {
		t.Fatal(err)
	}
	src.SetNRGBA(0, 0, color.NRGBA{9, 9, 9, 9}) // правка оригинала не доходит до буфера
	got, ok := ClipboardImage()
	if !ok {
		t.Fatal("изображения в буфере нет")
	}
	if c := color.NRGBAModel.Convert(got.At(0, 0)).(color.NRGBA); c != (color.NRGBA{255, 0, 0, 255}) {
		t.Errorf("точка (0,0) = %v: буфер хранит не копию", c)
	}
	if c := color.NRGBAModel.Convert(got.At(1, 0)).(color.NRGBA); c != (color.NRGBA{0, 255, 0, 128}) {
		t.Errorf("альфа потеряна: %v", c)
	}
	// Отданная копия тоже своя.
	got.(*image.NRGBA).SetNRGBA(0, 0, color.NRGBA{7, 7, 7, 7})
	again, _ := ClipboardImage()
	if c := color.NRGBAModel.Convert(again.At(0, 0)).(color.NRGBA); c.R != 255 {
		t.Error("правка отданной копии испортила буфер")
	}

	// Текст вытесняет картинку.
	ClipboardSetText("x")
	if _, ok := ClipboardImage(); ok {
		t.Error("после ClipboardSetText осталась картинка")
	}
	// А картинка — текст и файлы.
	ClipboardSetFiles([]string{"/a"}, false)
	if err := ClipboardSetImage(testClipImage()); err != nil {
		t.Fatal(err)
	}
	if s := ClipboardGetText(); s != "" {
		t.Errorf("текст после картинки: %q", s)
	}
	if _, _, ok := ClipboardFiles(); ok {
		t.Error("после картинки остались файлы")
	}
	ClipboardSetFiles([]string{"/a"}, false)
	if _, ok := ClipboardImage(); ok {
		t.Error("после ClipboardSetFiles осталась картинка")
	}
	SetClipboardHTML("<b>x</b>", "x")
	if _, ok := ClipboardImage(); ok {
		t.Error("после SetClipboardHTML осталась картинка")
	}
}

func TestClipboardSetImage_EmptyAndUnsupported(t *testing.T) {
	UseMemoryClipboard()
	if err := ClipboardSetImage(nil); !errors.Is(err, ErrClipboardImageEmpty) {
		t.Errorf("nil: %v", err)
	}
	if err := ClipboardSetImage(image.NewRGBA(image.Rect(0, 0, 0, 5))); !errors.Is(err, ErrClipboardImageEmpty) {
		t.Errorf("пустая: %v", err)
	}

	// Провайдер приложения без расширения: честная ошибка и «нет» на чтении.
	old := GetClipboardProvider()
	defer SetClipboardProvider(old)
	SetClipboardProvider(&textOnlyClipboard{})
	if err := ClipboardSetImage(testClipImage()); !errors.Is(err, ErrClipboardImageUnsupported) {
		t.Errorf("ждал ErrClipboardImageUnsupported, получил %v", err)
	}
	if img, ok := ClipboardImage(); ok || img != nil {
		t.Error("провайдер без картинок вернул картинку")
	}
}

// Порядок чтения на Windows: PNG → CF_DIBV5 → CF_DIB. Следующий формат
// запрашивается, только если предыдущего нет или он не разобрался.
func TestFirstDecodableImage_Order(t *testing.T) {
	src := testClipImage()
	pngData, err := clipimage.EncodePNG(src)
	if err != nil {
		t.Fatal(err)
	}
	v5 := dib.EncodeV5(src)
	plain := dib.EncodeDIB(src)

	var asked []string
	mk := func(name string, data []byte, dec func([]byte) (image.Image, error)) ClipImageSource {
		return ClipImageSource{Get: func() []byte { asked = append(asked, name); return data }, Decode: dec}
	}
	decPNG := clipimage.DecodePNG
	decDIB := dib.Decode

	// Есть всё — берётся PNG, остальные не трогаются.
	asked = nil
	img, ok := FirstDecodableImage(mk("png", pngData, decPNG), mk("v5", v5, decDIB), mk("dib", plain, decDIB))
	if !ok || !reflect.DeepEqual(asked, []string{"png"}) {
		t.Errorf("все три: ok=%v asked=%v", ok, asked)
	}
	if c := color.NRGBAModel.Convert(img.At(1, 0)).(color.NRGBA); c.A != 128 {
		t.Errorf("альфа из PNG потеряна: %v", c)
	}

	// PNG нет — DIBV5, с альфой.
	asked = nil
	img, ok = FirstDecodableImage(mk("png", nil, decPNG), mk("v5", v5, decDIB), mk("dib", plain, decDIB))
	if !ok || !reflect.DeepEqual(asked, []string{"png", "v5"}) {
		t.Errorf("без PNG: ok=%v asked=%v", ok, asked)
	}
	if c := color.NRGBAModel.Convert(img.At(1, 0)).(color.NRGBA); c.A != 128 {
		t.Errorf("альфа из CF_DIBV5 потеряна: %v", c)
	}

	// PNG битый, DIBV5 нет — CF_DIB, непрозрачный.
	asked = nil
	img, ok = FirstDecodableImage(mk("png", []byte("не png"), decPNG), mk("v5", nil, decDIB), mk("dib", plain, decDIB))
	if !ok || !reflect.DeepEqual(asked, []string{"png", "v5", "dib"}) {
		t.Errorf("битый PNG: ok=%v asked=%v", ok, asked)
	}
	if _, opaque := img.(*image.RGBA); !opaque {
		t.Errorf("CF_DIB ждал непрозрачным, получил %T", img)
	}

	// Ничего годного.
	if _, ok := FirstDecodableImage(mk("png", nil, decPNG), mk("v5", []byte{1, 2}, decDIB)); ok {
		t.Error("нечитаемое принято за картинку")
	}
}
