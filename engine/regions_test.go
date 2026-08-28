package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// Классификация содержимого тайлов.
//
// Движок знает, чем залил каждый участок; раньше это знание терялось на
// выходе, и потребитель восстанавливал его вторым проходом кодека.

// regionAt возвращает признак тайла, накрывающего точку (x, y).
func regionAt(regions []output.Region, x, y int) (output.Region, bool) {
	for _, r := range regions {
		if image.Pt(x, y).In(r.Rect) {
			return r, true
		}
	}
	return output.Region{}, false
}

// paintScene — сцена из заливки, изображения и надписи, каждая в своём углу,
// чтобы они не попали в один тайл.
func paintScene(t *testing.T) output.Frame {
	t.Helper()

	const w, h = 320, 320
	eng := New(w, h, 30)

	root := widget.NewPanel(color.RGBA{R: 10, G: 10, B: 10, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	// Заливка — панель во весь тайл.
	fill := widget.NewPanel(color.RGBA{R: 200, G: 30, B: 40, A: 255})
	fill.ShowHeader = false
	fill.SetBounds(image.Rect(0, 0, 64, 64))
	root.AddChild(fill)

	// Изображение — картинка в правом верхнем тайле.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	pic := widget.NewImageWidget()
	pic.SetImage(img)
	pic.SetBounds(image.Rect(256, 0, 320, 64))
	root.AddChild(pic)

	// Надпись — в левом нижнем.
	label := widget.NewLabel("Текст", color.RGBA{R: 240, G: 240, B: 240, A: 255})
	label.SetBounds(image.Rect(4, 260, 120, 300))
	root.AddChild(label)

	eng.SetRoot(root)
	return eng.renderFrame()
}

func TestRegions_ClassifiesFillImageAndText(t *testing.T) {
	frame := paintScene(t)
	if len(frame.Regions) == 0 {
		t.Fatal("кадр не принёс ни одного признака")
	}
	if len(frame.Regions) != len(frame.Tiles) {
		t.Errorf("признаков %d на %d тайлов — списки обязаны идти рядом",
			len(frame.Regions), len(frame.Tiles))
	}

	solid, ok := regionAt(frame.Regions, 32, 32)
	if !ok {
		t.Fatal("тайл с заливкой не попал в кадр")
	}
	if solid.Kind != output.RegionSolid {
		t.Errorf("заливка распознана как %v, ждали solid", solid.Kind)
	}
	if solid.Color.R != 200 || solid.Color.G != 30 || solid.Color.B != 40 {
		t.Errorf("цвет сплошной области %v — не тот, которым залили", solid.Color)
	}

	pic, ok := regionAt(frame.Regions, 288, 32)
	if !ok {
		t.Fatal("тайл с изображением не попал в кадр")
	}
	if pic.Kind != output.RegionImage {
		t.Errorf("изображение распознано как %v, ждали image", pic.Kind)
	}

	text, ok := regionAt(frame.Regions, 40, 280)
	if !ok {
		t.Fatal("тайл с надписью не попал в кадр")
	}
	if text.Kind != output.RegionText {
		t.Errorf("надпись распознана как %v, ждали text", text.Kind)
	}
}

// Заливка поверх картинки в одном тайле даёт честное «не знаю»: часть
// площади — картинка, часть — заливка, и обещать потребителю что-то одно
// нельзя. Обратный порядок (картинка поверх фона) — не тот случай: там
// содержимое легло НА фон, и признаком считается содержимое.
func TestRegions_MixedWhenTwoKindsShareTile(t *testing.T) {
	const w, h = 128, 128
	eng := New(w, h, 30)

	root := widget.NewPanel(color.RGBA{R: 10, G: 10, B: 10, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	fill := widget.NewPanel(color.RGBA{R: 60, G: 160, B: 90, A: 255})
	fill.ShowHeader = false
	fill.SetBounds(image.Rect(0, 0, 64, 64))
	root.AddChild(fill)

	// Картинка, а поверх неё — заливка, не накрывающая тайл целиком.
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	pic := widget.NewImageWidget()
	pic.SetImage(img)
	pic.SetBounds(image.Rect(0, 0, 40, 40))
	root.AddChild(pic)

	patch := widget.NewPanel(color.RGBA{R: 200, G: 40, B: 40, A: 255})
	patch.ShowHeader = false
	patch.SetBounds(image.Rect(10, 10, 30, 30))
	root.AddChild(patch)

	eng.SetRoot(root)
	frame := eng.renderFrame()

	got, ok := regionAt(frame.Regions, 20, 20)
	if !ok {
		t.Fatal("тайл не попал в кадр")
	}
	if got.Kind != output.RegionMixed {
		t.Errorf("заливка с картинкой в одном тайле дали %v, ждали mixed", got.Kind)
	}
}

// Частичная заливка тайла — тоже «не знаю»: рядом осталось что-то ещё.
func TestRegions_PartialFillIsMixed(t *testing.T) {
	eng := New(128, 128, 30)
	root := widget.NewPanel(color.RGBA{R: 10, G: 10, B: 10, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 128, 128))

	patch := widget.NewPanel(color.RGBA{R: 90, G: 90, B: 200, A: 255})
	patch.ShowHeader = false
	patch.SetBounds(image.Rect(8, 8, 40, 40)) // меньше тайла
	root.AddChild(patch)

	eng.SetRoot(root)
	frame := eng.renderFrame()

	got, ok := regionAt(frame.Regions, 20, 20)
	if !ok {
		t.Fatal("тайл не попал в кадр")
	}
	if got.Kind == output.RegionSolid {
		t.Error("частичная заливка объявлена сплошной — потребитель зальёт весь тайл одним цветом")
	}
}
