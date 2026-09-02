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

// Сглаженные дуги и тени — не текст.
//
// Маски альфы и цветные глифы — общий путь для букв, дуг скруглённых заливок
// и размытого силуэта тени. Пока вид объявлялся самим путём, потребитель
// применял к серому размытию текстовый кодек.
func TestRegions_ShapesAndShadowsAreNotText(t *testing.T) {
	const w, h = 192, 192
	eng := New(w, h, 30)

	root := widget.NewPanel(color.RGBA{R: 12, G: 14, B: 18, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	eng.SetRoot(root)
	eng.renderFrame()

	canvas := eng.canvas

	// Скруглённая заливка: её углы идут через маску альфы.
	canvas.resetTileMarks(image.Rect(0, 0, w, h))
	canvas.FillRoundRect(4, 4, 120, 120, 24, color.RGBA{R: 200, G: 90, B: 40, A: 255})
	if got := canvas.marks[0].kind; got == output.RegionText {
		t.Error("угол скруглённой заливки объявлен текстом")
	}

	// Мягкая тень: composited через цветной глиф.
	canvas.resetTileMarks(image.Rect(0, 0, w, h))
	canvas.DrawSoftShadow(image.Rect(70, 70, 180, 180), 12, 10,
		color.RGBA{R: 0, G: 0, B: 0, A: 90})
	shadowTile := canvas.marks[(h/2/64)*canvas.tilesX+(w/2/64)]
	if shadowTile.kind == output.RegionText {
		t.Error("мягкая тень объявлена текстом")
	}

	// А текст — по-прежнему текст.
	canvas.resetTileMarks(image.Rect(0, 0, w, h))
	canvas.DrawTextSize("Проверка", 8, 20, 14, color.RGBA{R: 240, G: 240, B: 240, A: 255})
	found := false
	for _, m := range canvas.marks {
		if m.touched && m.kind == output.RegionText {
			found = true
			break
		}
	}
	if !found {
		t.Error("надпись перестала помечаться текстом")
	}
}

// Тайл, у которого скруглённый клип срезает угол, не может быть сплошным.
//
// Заливка накрывает прямоугольник тайла целиком, но за дугой пиксели
// остаются прежними. Потребитель, выполняющий Solid командой заливки,
// нарисовал бы там квадратный угол чужим цветом.
func TestRegions_RoundClipCornerIsNotSolid(t *testing.T) {
	const w, h = 192, 192
	eng := New(w, h, 30)
	root := widget.NewPanel(color.RGBA{R: 12, G: 14, B: 18, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	eng.SetRoot(root)
	eng.renderFrame()

	canvas := eng.canvas
	canvas.resetTileMarks(image.Rect(0, 0, w, h))

	// Так рисует стеклянная панель: скруглённый клип, затем заливка области.
	area := image.Rect(0, 0, 160, 160)
	canvas.SetRoundClip(area, 40)
	canvas.FillRect(area.Min.X, area.Min.Y, area.Dx(), area.Dy(),
		color.RGBA{R: 200, G: 90, B: 40, A: 255})
	canvas.ClearRoundClip()

	// Угловой тайл — тот, что накрывает дугу.
	if got := canvas.marks[0].kind; got == output.RegionSolid {
		t.Error("угловой тайл под скруглённым клипом объявлен сплошным")
	}
	// Тайл в середине области — сплошной: там дуга ничего не режет.
	mid := (80 / 64) * canvas.tilesX + (80 / 64)
	if got := canvas.marks[mid].kind; got != output.RegionSolid {
		t.Errorf("середина залитой области объявлена %v, ждали solid", got)
	}
}

// Обои описываются, а не молчат.
//
// Фон кладётся в начале каждого кадра построчным копированием, и пометки этот
// путь не ставил вовсе: тайл, накрытый ТОЛЬКО фоном — на рабочем столе с
// обоями это почти весь экран, — оставался нетронутым, и кадр сообщал о нём
// «неизвестно что». Потребитель, выбирающий кодек по Regions, на обоях не
// получал ни одной подсказки.
func TestRegions_WallpaperIsDescribedAsImage(t *testing.T) {
	const w, h = 320, 320
	eng := New(w, h, 30)

	// Фотообои: пёстрая картинка, которую сплошным цветом не описать.
	wall := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			wall.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	if err := eng.SetBackground(wall); err != nil {
		t.Fatal(err)
	}

	// Пустой корень: поверх обоев не рисуется ничего.
	root := widget.NewPanel(color.RGBA{})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 0, 0))
	eng.SetRoot(root)

	frame := eng.renderFrame()
	if len(frame.Regions) == 0 {
		t.Fatal("кадр не принёс признаков тайлов")
	}

	mixed := 0
	for _, r := range frame.Regions {
		switch r.Kind {
		case output.RegionImage:
		case output.RegionMixed:
			mixed++
		default:
			t.Errorf("тайл %v объявлен %v, а под ним только обои", r.Rect, r.Kind)
		}
	}
	if mixed != 0 {
		t.Errorf("%d тайлов из %d описаны как «неизвестно что», хотя под ними только обои",
			mixed, len(frame.Regions))
	}
}

// Пустой фон — сплошная чёрная заливка, и сказать об этом стоит: потребитель
// отправит команду заливки вместо тайла с пикселями.
func TestRegions_EmptyBackgroundIsSolidBlack(t *testing.T) {
	const w, h = 320, 320
	eng := New(w, h, 30)

	root := widget.NewPanel(color.RGBA{})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 0, 0))
	eng.SetRoot(root)

	frame := eng.renderFrame()
	if len(frame.Regions) == 0 {
		t.Fatal("кадр не принёс признаков тайлов")
	}
	black := color.RGBA{A: 255}
	for _, r := range frame.Regions {
		if r.Kind != output.RegionSolid {
			t.Errorf("тайл %v объявлен %v, а под ним пустой чёрный фон", r.Rect, r.Kind)
			continue
		}
		if r.Color != black {
			t.Errorf("тайл %v объявлен цветом %v, ждали чёрный", r.Rect, r.Color)
		}
	}
}

// Виджет поверх обоев остаётся собой: сплошная панель, накрывшая тайл
// целиком, описывается заливкой, а не картинкой.
func TestRegions_WidgetOverWallpaperWins(t *testing.T) {
	const w, h = 320, 320
	eng := New(w, h, 30)

	wall := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range wall.Pix {
		wall.Pix[i] = uint8(i)
	}
	if err := eng.SetBackground(wall); err != nil {
		t.Fatal(err)
	}

	root := widget.NewPanel(color.RGBA{})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 0, 0))

	fillCol := color.RGBA{R: 200, G: 30, B: 40, A: 255}
	panel := widget.NewPanel(fillCol)
	panel.ShowHeader = false
	panel.SetBounds(image.Rect(0, 0, 128, 128)) // ровно четыре тайла
	root.AddChild(panel)
	eng.SetRoot(root)

	frame := eng.renderFrame()
	got, ok := regionAt(frame.Regions, 32, 32)
	if !ok {
		t.Fatal("тайл под панелью не попал в кадр")
	}
	if got.Kind != output.RegionSolid || got.Color != fillCol {
		t.Errorf("тайл под сплошной панелью объявлен %v цветом %v, ждали заливку %v",
			got.Kind, got.Color, fillCol)
	}

	// А тайл, где обои ничем не закрыты, — картинка.
	if got, ok := regionAt(frame.Regions, w-32, h-32); ok && got.Kind != output.RegionImage {
		t.Errorf("тайл с открытыми обоями объявлен %v, ждали картинку", got.Kind)
	}
}
