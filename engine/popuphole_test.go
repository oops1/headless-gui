package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Дыра в картинке вынесенного оверлея.
//
// Заказчик Go.Git видит в настоящем окне Windows чёрный прямоугольник при
// раскрытии каскадного подменю: показаны нижние пункты дочернего меню, а выше
// и правее — незакрашенная область. Он проверил, что тайлы кадра верны, и
// заподозрил путь вывода окна.
//
// Причина глубже и лежит в движке. Область вынесенного оверлея — ОБЪЕДИНЕНИЕ
// прямоугольников: полоса меню, раскрытое подменю и его каскадный потомок,
// который стоит правее и ниже. Между ними остаётся площадь, не принадлежащая
// никому: её не закрашивает никто, и в картинке она остаётся прозрачной.
//
// В холсте это незаметно — под прозрачным виден рабочий стол. В отдельном
// окне ОС видно чёрное: окно непрозрачно, и «ничего» показывается чернотой.

// popupImageFor рендерит вынесенный оверлей виджета так же, как это делает
// хост попапов, и возвращает картинку.
func popupImageFor(t *testing.T, c *Canvas, od widget.OverlayDrawer, r image.Rectangle) *image.RGBA {
	t.Helper()
	ent := &popupEntry{}
	oc := ent.overlayCanvas(c, r)
	return renderOverlayInto(oc, od, r)
}

func TestPopupHost_CascadeLeavesATransparentHole(t *testing.T) {
	e := New(900, 400, 60)
	c := e.canvas

	mb := widget.NewMenuBar()
	mb.SetBounds(image.Rect(200, 40, 900, 68))
	mb.AddMenu("Репозиторий",
		widget.MenuItem{Text: "Ветки", SubItems: []widget.MenuItem{
			{Text: "Создать"},
			{Text: "Удалить"},
		}},
		widget.MenuItem{Text: "Журнал"},
	)

	// Раскрываем меню и его каскад так же, как это делает мышь: сначала
	// клик по пункту полосы, затем наведение на пункт с подменю.
	r0 := mb.Bounds()
	press := widget.MouseEvent{X: r0.Min.X + 10, Y: r0.Min.Y + 10, Button: widget.MouseLeft, Pressed: true}
	mb.OnMouseButton(press)
	mb.OnMouseButton(widget.MouseEvent{X: press.X, Y: press.Y, Button: widget.MouseLeft})
	if !mb.HasOverlay() {
		t.Skip("меню не раскрылось — проверять нечего")
	}
	// Наведение на пункт с подменю раскрывает каскад вправо.
	sub := mb.OverlayBounds()
	mb.OnMouseMove(sub.Min.X+10, sub.Min.Y+10)

	r := mb.OverlayBounds()
	if r.Empty() {
		t.Fatal("у раскрытого меню пустая область оверлея")
	}
	img := popupImageFor(t, c, mb, r)

	// Прозрачные точки ВНУТРИ прямоугольника попапа — это и есть дыра: окно
	// ОС покажет здесь черноту, потому что оно непрозрачно.
	holes := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] == 0 {
			holes++
		}
	}
	total := len(img.Pix) / 4
	if holes == 0 {
		t.Fatal("в картинке каскада нет прозрачных точек — сцена не воспроизвела случай")
	}
	t.Logf("прозрачных точек: %d из %d в области %v", holes, total, r)

	// Выкройка обязана эти точки ИСКЛЮЧИТЬ: по ней потребитель обрежет окно,
	// и черноте неоткуда взяться.
	bands := OpaqueBands(img)
	if len(bands) == 0 {
		t.Fatal("выкройка пуста — окно окажется невидимым")
	}
	covered := func(x, y int) bool {
		for _, b := range bands {
			if image.Pt(x, y).In(b) {
				return true
			}
		}
		return false
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			opaque := img.RGBAAt(x, y).A != 0
			if opaque != covered(x, y) {
				t.Fatalf("точка (%d,%d): закрашена=%v, попала в выкройку=%v",
					x, y, opaque, covered(x, y))
			}
		}
	}
	// И выкройка обязана быть КОРОЧЕ, чем построчная: иначе окно из тысяч
	// полос не построить.
	if len(bands) > b.Dy()/2 {
		t.Errorf("выкройка из %d полос при высоте %d — полосы не склеиваются",
			len(bands), b.Dy())
	}
	t.Logf("выкройка: %d полос", len(bands))
}

// Хост обязан узнать, какие части картинки закрашены: по ним он выкроит окно
// и чернота не появится.
func TestOpaqueBands_FindsThePaintedPart(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 6))
	// Две закрашенные полосы с прозрачным промежутком между ними.
	fill := func(r image.Rectangle) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetRGBA(x, y, color.RGBA{R: 200, A: 255})
			}
		}
	}
	fill(image.Rect(0, 0, 4, 6))  // левая колонка
	fill(image.Rect(6, 3, 10, 6)) // правый нижний угол

	bands := OpaqueBands(img)
	if len(bands) == 0 {
		t.Fatal("закрашенных полос не найдено")
	}

	// Каждая точка с ненулевой альфой обязана попасть хотя бы в одну полосу,
	// а прозрачная — ни в одну: иначе окно либо обрежет нарисованное, либо
	// оставит черноту.
	covered := func(x, y int) bool {
		for _, b := range bands {
			if image.Pt(x, y).In(b) {
				return true
			}
		}
		return false
	}
	for y := 0; y < 6; y++ {
		for x := 0; x < 10; x++ {
			opaque := img.RGBAAt(x, y).A != 0
			if opaque != covered(x, y) {
				t.Errorf("точка (%d,%d): закрашена=%v, накрыта полосой=%v", x, y, opaque, covered(x, y))
			}
		}
	}
}

// Сплошная картинка выкройки не требует: окно остаётся прямоугольником.
func TestOpaqueBands_FullImageNeedsNoRegion(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 5))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	if got := OpaqueBands(img); got != nil {
		t.Errorf("сплошная картинка дала выкройку %v", got)
	}
}

// Скруглённые углы выкройки НЕ требуют.
//
// Это главное в контракте. Углы меню прозрачны всегда, и если считать их
// дырой, потребитель будет перестраивать форму окна на каждый кадр — на
// каждое движение курсора по пунктам, — а окно от этого мигает на глазах.
// Выигрыш при этом нулевой: несколько точек в углах.
func TestOpaqueBands_RoundedCornersAreNotAHole(t *testing.T) {
	const w, h, r = 200, 120, 10
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		// Срезаем угол: чем ближе к краю по вертикали, тем больше отступ.
		cut := 0
		if y < r {
			cut = r - y
		} else if y >= h-r {
			cut = r - (h - 1 - y)
		}
		for x := cut; x < w-cut; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 40, G: 40, B: 48, A: 255})
		}
	}
	if got := OpaqueBands(img); got != nil {
		t.Errorf("скруглённые углы приняты за дыру: %d полос %v", len(got), got)
	}
}

// А настоящая дыра — строка, разорванная надвое, — выкройку требует.
func TestOpaqueBands_SplitRowIsAHole(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 200; x++ {
			if x >= 90 && x < 110 {
				continue // сквозной просвет посередине
			}
			img.SetRGBA(x, y, color.RGBA{R: 40, A: 255})
		}
	}
	if got := OpaqueBands(img); len(got) == 0 {
		t.Error("разорванная строка не признана дырой")
	}
}

// Пустая и полностью прозрачная картинки полос не дают.
func TestOpaqueBands_EmptyAndTransparent(t *testing.T) {
	if got := OpaqueBands(nil); got != nil {
		t.Errorf("nil дал %v", got)
	}
	if got := OpaqueBands(image.NewRGBA(image.Rect(0, 0, 4, 4))); got != nil {
		t.Errorf("прозрачная картинка дала %v", got)
	}
}
