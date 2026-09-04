package tests

import (
	"image"
	"os"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Мигание интерфейса.
//
// Владелец видит в showcase, как на месте кнопок и всплывающего меню
// появляются пустые прямоугольники. Это почерк частичного кадра: движок
// перерисовывает только заявленные области, и если изменение области не
// заявило — или заявило меньше, чем изменилось, — на экране остаётся
// недорисованное, а следующий полный кадр его возвращает.
//
// Инвариант, который это ловит: ЧАСТИЧНЫЙ кадр обязан совпадать с ПОЛНЫМ
// кадром того же состояния.
//
// Сравнивать в лоб нельзя: на сцене есть анимации (свечение полосы прогресса,
// каретка), и они законно меняют картинку между двумя кадрами. Поэтому шум
// сначала измеряется — двумя полными кадрами подряд, — и расхождение
// засчитывается только там, где два полных кадра совпали, а частичный
// разошёлся. Это и есть мусор, оставшийся от незаявленной области.

func flickerScene(t *testing.T) (*engine.Engine, widget.Widget) {
	t.Helper()
	const path = "../assets/ui/showcase.xaml"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("разметки нет: %v", err)
	}
	root, _, err := widget.LoadUIFromXAMLFile(path)
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	eng := engine.New(1280, 900, 30)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce() // первый кадр — полный
	return eng, root
}

func fullFrame(eng *engine.Engine) *image.RGBA {
	eng.Invalidate()
	return snapshotRGBA(eng.RenderOnce())
}

// checkNoStaleContent ищет мусор, оставшийся от незаявленной области.
//
// Порядок здесь принципиален. СНАЧАЛА снимается то, что видит потребитель, и
// только потом рисуется полный кадр-эталон: полный кадр перерисовывает экран
// целиком и лечит недорисованное. Замер шума анимаций тоже идёт полными
// кадрами — поэтому он делается ПОСЛЕ сравнения, а не до. Раньше он стоял
// перед снимком и стирал ровно то, что тест ищет.
func checkNoStaleContent(t *testing.T, eng *engine.Engine, what string) {
	t.Helper()

	partial := snapshotRGBA(eng.RenderOnce())
	full := fullFrame(eng)
	if partial == nil || full == nil {
		t.Fatalf("%s: кадр не отрисован", what)
	}

	// Расхождения запоминаются точками: отсеивать их шумом придётся после
	// замера, а к тому времени оба кадра уже не те.
	var bad []image.Point
	for i := 0; i < len(partial.Pix); i++ {
		if partial.Pix[i] == full.Pix[i] {
			continue
		}
		bad = append(bad, image.Pt((i%partial.Stride)/4, i/partial.Stride))
		if len(bad) > 1<<20 {
			break // расхождение размером с экран — считать точки дальше незачем
		}
	}
	if len(bad) == 0 {
		return
	}

	// Шум анимаций: на сцене есть полоса прогресса с бегущим свечением, и её
	// голова сдвигается от кадра к кадру законно.
	//
	// Область исключается ЦЕЛИКОМ, а не по точкам: свечение едет непрерывно,
	// и точка, совпавшая в двух кадрах подряд, в третьем всё равно другая.
	// Всё остальное — а это почти весь экран, и там как раз кнопки и меню, на
	// которые жалуется владелец, — сравнивается честно.
	//
	// Замер идёт в два захода. Сперва дюжина кадров — обычно этого хватает.
	// Если после отсева расхождение осталось, замер продолжается ещё сотней:
	// дорогой заход платится лишь там, где тест иначе упал бы.
	noise := measureNoise(eng, full, 12)
	box, n := filterNoise(bad, noise)
	if n == 0 {
		return
	}

	noise = noise.Union(measureNoise(eng, fullFrame(eng), 100))
	if box, n = filterNoise(bad, noise); n != 0 {
		t.Errorf("%s: частичный кадр разошёлся с полным в %d байтах, область %v — "+
			"это и есть мигание", what, n, box)
	}
}

// measureNoise собирает область, где картинка меняется сама по себе, по
// нескольким полным кадрам подряд.
//
// Область расширяется на noiseMargin во все стороны. Причина в том, что
// замерить её можно только ПОСЛЕ сравнения — всякий полный кадр лечит
// недорисованное, а тест ищет именно недорисованное. Значит замер накрывает
// положения свечения ПОЗЖЕ сравниваемой пары, а её собственное положение —
// на шаг раньше и на несколько точек левее. Запас в два десятка точек эту
// разницу закрывает и при этом ничего не прячет: тест ищет пропавшие кнопки
// и меню, а они измеряются десятками и сотнями точек.
func measureNoise(eng *engine.Engine, prev *image.RGBA, frames int) image.Rectangle {
	noise := image.Rectangle{}
	for i := 0; i < frames; i++ {
		cur := fullFrame(eng)
		noise = noise.Union(diffBox(prev, cur))
		prev = cur
	}
	if noise.Empty() {
		return noise
	}
	return noise.Inset(-noiseMargin)
}

// noiseMargin — запас вокруг замеренной области анимации (см. measureNoise).
const noiseMargin = 24

// filterNoise отбрасывает точки, попавшие в область анимации, и возвращает
// охватывающий прямоугольник оставшихся с их числом.
func filterNoise(bad []image.Point, noise image.Rectangle) (image.Rectangle, int) {
	var box image.Rectangle
	n := 0
	for _, pt := range bad {
		if pt.In(noise) {
			continue
		}
		r := image.Rect(pt.X, pt.Y, pt.X+1, pt.Y+1)
		if n == 0 {
			box = r
		} else {
			box = box.Union(r)
		}
		n++
	}
	return box, n
}

// Проводка курсором через всю сцену: подсветка обязана заявлять свою область
// целиком.
func TestShowcase_HoverDoesNotFlicker(t *testing.T) {
	eng, root := flickerScene(t)

	b := root.Bounds()
	for y := b.Min.Y + 8; y < b.Max.Y; y += 37 {
		for x := b.Min.X + 8; x < b.Max.X; x += 53 {
			eng.SendMouseMove(x, y)
		}
	}
	checkNoStaleContent(t, eng, "после проводки курсором")
}

// Щелчки: открытие и закрытие меню, переключение вкладок, нажатия кнопок.
func TestShowcase_ClicksDoNotFlicker(t *testing.T) {
	eng, root := flickerScene(t)

	b := root.Bounds()
	for y := b.Min.Y + 20; y < b.Max.Y; y += 101 {
		for x := b.Min.X + 20; x < b.Max.X; x += 149 {
			eng.SendMouseMove(x, y)
			eng.SendMouseButton(x, y, widget.MouseLeft, true)
			eng.SendMouseButton(x, y, widget.MouseLeft, false)
			checkNoStaleContent(t, eng, "после щелчка")
			if t.Failed() {
				t.Logf("щелчок был в (%d,%d)", x, y)
				return
			}
		}
	}
}

// Прокрутка колесом: список обязан заявлять всю область, которую сдвинул.
func TestShowcase_WheelDoesNotFlicker(t *testing.T) {
	eng, root := flickerScene(t)

	b := root.Bounds()
	for y := b.Min.Y + 40; y < b.Max.Y; y += 137 {
		for x := b.Min.X + 40; x < b.Max.X; x += 211 {
			eng.SendMouseMove(x, y)
			for i := 0; i < 3; i++ {
				eng.SendMouseWheelPixels(x, y, 0, 40)
			}
			checkNoStaleContent(t, eng, "после прокрутки")
			if t.Failed() {
				t.Logf("прокрутка была в (%d,%d)", x, y)
				return
			}
		}
	}
}

// diffBox — охватывающий прямоугольник расхождений двух кадров.
func diffBox(a, b *image.RGBA) image.Rectangle {
	var box image.Rectangle
	n := 0
	for i := range a.Pix {
		if a.Pix[i] == b.Pix[i] {
			continue
		}
		y := i / a.Stride
		x := (i % a.Stride) / 4
		r := image.Rect(x, y, x+1, y+1)
		if n == 0 {
			box = r
		} else {
			box = box.Union(r)
		}
		n++
	}
	return box
}

// Вычитание перекрытого не меняет картинку.
//
// Поддерево, целиком накрытое непрозрачным соседом, не рисуется. Если сосед
// объявил непрозрачной площадь, которую закрашивает не полностью, под ней
// останется дыра — и на экране это будет выглядеть как пропавший
// прямоугольник, ровно как на снимках владельца.
//
// Сравнение идёт НА ОДНОМ движке: два разных движка дают разную картинку из-за
// общих на процесс мелочей (шрифты, измеритель), и сравнивать их бессмысленно.
func TestShowcase_OcclusionKeepsThePicture(t *testing.T) {
	eng, root := flickerScene(t)

	// Проводим курсором и щёлкаем: часть виджетов меняет вид, вкладки
	// переключаются — сцена перестаёт быть исходной.
	b := root.Bounds()
	for y := b.Min.Y + 30; y < b.Max.Y; y += 157 {
		for x := b.Min.X + 30; x < b.Max.X; x += 233 {
			eng.SendMouseMove(x, y)
		}
	}

	eng.SetOcclusionCulling(true)
	with := fullFrame(eng)
	eng.SetOcclusionCulling(false)
	without := fullFrame(eng)
	eng.SetOcclusionCulling(true)
	withAgain := fullFrame(eng)

	// Шум анимаций — по двум кадрам с ОДНОЙ настройкой.
	noise := diffBox(with, withAgain)

	var box image.Rectangle
	n := 0
	for i := range with.Pix {
		if with.Pix[i] == without.Pix[i] {
			continue
		}
		y := i / with.Stride
		x := (i % with.Stride) / 4
		if image.Pt(x, y).In(noise) {
			continue
		}
		r := image.Rect(x, y, x+1, y+1)
		if n == 0 {
			box = r
		} else {
			box = box.Union(r)
		}
		n++
	}
	if n != 0 {
		t.Errorf("вычитание перекрытого изменило картинку: %d байт, область %v — "+
			"кто-то объявил непрозрачным то, чего не закрашивает", n, box)
	}
}
