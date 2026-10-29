package core

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// Значок перетаскивания: общий слой окна (BeginDragFilesImage) без ОС.

// imgNative — dragNative, умеющий значок.
type imgNative struct {
	dragNative
	imgs   []*image.RGBA
	hots   []image.Point
	paths  [][]string
	supprt bool
}

func (f *imgNative) BeginDragFilesImage(paths []string, effect native.DragEffect, img *image.RGBA, hot image.Point, done func(native.DragEffect)) bool {
	f.imgs = append(f.imgs, img)
	f.hots = append(f.hots, hot)
	f.paths = append(f.paths, paths)
	f.done = done
	return !f.refuse
}

func (f *imgNative) dragImageSupported() bool { return f.supprt }

func imgWindow() (*Window, *imgNative) {
	fn := &imgNative{supprt: true}
	win := &Window{surface: surface{eng: &immediateEngine{}, native: fn, scale: 1}}
	return win, fn
}

func TestBeginDragFilesImage_PassesImage(t *testing.T) {
	win, fn := imgWindow()
	var got []native.DragEffect
	win.SetOnDragFinished(func(e native.DragEffect) { got = append(got, e) })

	src := image.NewNRGBA(image.Rect(5, 7, 5+40, 7+30)) // начало не в нуле
	src.SetNRGBA(5, 7, color.NRGBA{R: 255, A: 128})
	if !win.BeginDragFilesImage([]string{"a.png"}, native.DragEffectCopy, src, image.Pt(12, 9)) {
		t.Fatal("BeginDragFilesImage отказал")
	}
	if len(fn.imgs) != 1 {
		t.Fatalf("бэкенд со значком вызван %d раз", len(fn.imgs))
	}
	img := fn.imgs[0]
	if img.Bounds() != image.Rect(0, 0, 40, 30) {
		t.Errorf("границы значка %v, ждал (0,0)-(40,30)", img.Bounds())
	}
	if fn.hots[0] != image.Pt(12, 9) {
		t.Errorf("точка захвата %v", fn.hots[0])
	}
	// RGBA премультиплицирован: 50% красного → R=128, A=128.
	if c := img.RGBAAt(0, 0); c.A != 128 || c.R < 127 || c.R > 129 {
		t.Errorf("пиксель (0,0) = %+v, ждал премультиплицированный {128 0 0 128}", c)
	}
	if len(fn.began) != 0 {
		t.Error("значок передан, но ещё и вызван BeginDragFiles")
	}
	// Окончание тем же SetOnDragFinished.
	fn.done(native.DragEffectCopy)
	if !reflect.DeepEqual(got, []native.DragEffect{native.DragEffectCopy}) {
		t.Errorf("SetOnDragFinished получил %v", got)
	}
}

// Без картинки — прежнее перетаскивание: бэкенд со значком не трогается.
func TestBeginDragFilesImage_NilFallsBack(t *testing.T) {
	win, fn := imgWindow()
	if !win.BeginDragFilesImage([]string{"a"}, native.DragEffectCopy, nil, image.Point{}) {
		t.Fatal("отказ без картинки")
	}
	if len(fn.imgs) != 0 || len(fn.began) != 1 {
		t.Errorf("значков %d, обычных запусков %d; ждал 0 и 1", len(fn.imgs), len(fn.began))
	}
	if !win.BeginDragFilesImage([]string{"a"}, native.DragEffectCopy, image.NewRGBA(image.Rectangle{}), image.Point{}) {
		t.Fatal("отказ с пустой картинкой")
	}
	if len(fn.imgs) != 0 || len(fn.began) != 2 {
		t.Errorf("пустая картинка: значков %d, обычных %d; ждал 0 и 2", len(fn.imgs), len(fn.began))
	}
}

// Бэкенд без значка (X11): перетаскивание всё равно начинается.
func TestBeginDragFilesImage_BackendWithoutImage(t *testing.T) {
	win, fn, _ := dragWindow(1)
	if !win.BeginDragFilesImage([]string{"a"}, native.DragEffectMove, image.NewRGBA(image.Rect(0, 0, 8, 8)), image.Pt(1, 1)) {
		t.Fatal("перетаскивание не началось на бэкенде без значка")
	}
	if len(fn.began) != 1 || fn.effects[0] != native.DragEffectMove {
		t.Errorf("бэкенд получил %v %v", fn.began, fn.effects)
	}
	if win.DragImageSupported() {
		t.Error("DragImageSupported true у бэкенда без значка")
	}
}

func TestBeginDragFilesImage_Refusals(t *testing.T) {
	win, fn := imgWindow()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	if win.BeginDragFilesImage(nil, native.DragEffectCopy, img, image.Point{}) ||
		win.BeginDragFilesImage([]string{"a"}, 0, img, image.Point{}) {
		t.Error("пустой список / маска приняты")
	}
	fn.refuse = true
	if win.BeginDragFilesImage([]string{"a"}, native.DragEffectCopy, img, image.Point{}) {
		t.Error("отказ бэкенда не дошёл до вызывающего")
	}
	bare := &Window{surface: surface{eng: &immediateEngine{}, native: &inputNative{}, scale: 1}}
	if bare.BeginDragFilesImage([]string{"a"}, native.DragEffectCopy, img, image.Point{}) || bare.DragImageSupported() {
		t.Error("бэкенд без перетаскивания принял значок")
	}
}

func TestDragImageSupported(t *testing.T) {
	win, fn := imgWindow()
	if !win.DragImageSupported() {
		t.Error("бэкенд со значком сообщает, что не умеет")
	}
	fn.supprt = false // бэкенд умеет, но сейчас нет (Win32 без OLE)
	if win.DragImageSupported() {
		t.Error("dragImageSupported()==false не дошёл до окна")
	}
}

// prepareDragImage: слишком большая картинка уменьшается с сохранением
// пропорций, точка захвата — вместе с ней; точка вне картинки прижимается.
func TestPrepareDragImage(t *testing.T) {
	big := image.NewRGBA(image.Rect(0, 0, 1024, 512))
	img, hot, ok := prepareDragImage(big, image.Pt(512, 256))
	if !ok || img.Bounds() != image.Rect(0, 0, 512, 256) {
		t.Fatalf("уменьшение: %v ok=%v", img.Bounds(), ok)
	}
	if hot != image.Pt(256, 128) {
		t.Errorf("точка захвата при уменьшении %v, ждал (256,128)", hot)
	}

	small := image.NewRGBA(image.Rect(0, 0, 10, 20))
	_, hot, _ = prepareDragImage(small, image.Pt(-5, 99))
	if hot != image.Pt(0, 19) {
		t.Errorf("точка вне картинки %v, ждал (0,19)", hot)
	}

	// Узкая полоса не вырождается в нулевую сторону.
	strip := image.NewRGBA(image.Rect(0, 0, 4000, 2))
	if img, _, _ := prepareDragImage(strip, image.Point{}); img.Bounds().Dy() < 1 || img.Bounds().Dx() != 512 {
		t.Errorf("полоса %v", img.Bounds())
	}

	if _, _, ok := prepareDragImage(nil, image.Point{}); ok {
		t.Error("nil принят")
	}
	if _, _, ok := prepareDragImage(image.NewRGBA(image.Rectangle{}), image.Point{}); ok {
		t.Error("пустая картинка принята")
	}

	// Прозрачность сохраняется при уменьшении.
	tr := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for i := 3; i < len(tr.Pix); i += 4 {
		tr.Pix[i] = 0 // всё прозрачно
	}
	img, _, _ = prepareDragImage(tr, image.Point{})
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 {
			t.Fatalf("прозрачный значок после уменьшения непрозрачен (альфа %d)", img.Pix[i])
		}
	}
}
