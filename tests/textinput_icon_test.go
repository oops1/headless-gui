package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Значок в начале поля ввода — запрос GG-46.
//
// Отдельным виджетом поверх поля это не решалось: клик по значку уходил
// значку, а не полю, каретка считалась от левого края и вставала под значок, а
// отступ приходилось задавать вручную и держать в согласии с картинкой.

// redIcon — заметная картинка, чтобы её было видно на кадре.
func redIcon(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	return img
}

func inputScene(t *testing.T, ti *widget.TextInput) (*engine.Engine, *image.RGBA) {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 24, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 80))
	ti.SetBounds(image.Rect(20, 20, 280, 52))
	root.AddChild(ti)

	eng := engine.New(300, 80, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	return eng, snapshotRGBA(eng.RenderOnce())
}

// countRed — сколько красных точек в кадре.
func countRed(img *image.RGBA) int {
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 200 && img.Pix[i+1] < 80 && img.Pix[i+2] < 80 {
			n++
		}
	}
	return n
}

// Значок виден на экране.
func TestLeadingIcon_IsDrawn(t *testing.T) {
	plain := widget.NewTextInput("поиск")
	_, before := inputScene(t, plain)
	if got := countRed(before); got != 0 {
		t.Fatalf("на пустой сцене уже %d красных точек", got)
	}

	withIcon := widget.NewTextInput("поиск")
	withIcon.LeadingIcon = redIcon(16)
	_, after := inputScene(t, withIcon)

	if got := countRed(after); got == 0 {
		t.Error("значок не нарисован")
	}
}

// Текст и подсказка сдвигаются вправо: значок не наезжает на них.
func TestLeadingIcon_ShiftsText(t *testing.T) {
	// Ищем самый левый закрашенный столбец текста в поле без значка и с ним.
	leftMostText := func(ti *widget.TextInput) int {
		_, img := inputScene(t, ti)
		// Полоса по центру поля, но правее рамки и правее зоны значка.
		for x := 22; x < 278; x++ {
			for y := 26; y < 46; y++ {
				i := img.PixOffset(x, y)
				r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
				// Красный — это значок, его пропускаем.
				if r > 200 && g < 80 && b < 80 {
					continue
				}
				// Текст светлее фона поля.
				if r > 120 && g > 120 && b > 120 {
					return x
				}
			}
		}
		return -1
	}

	plain := widget.NewTextInput("")
	plain.SetText("Пример")
	x1 := leftMostText(plain)

	iconed := widget.NewTextInput("")
	iconed.SetText("Пример")
	iconed.LeadingIcon = redIcon(16)
	x2 := leftMostText(iconed)

	if x1 < 0 || x2 < 0 {
		t.Fatalf("текст не найден: без значка %d, со значком %d", x1, x2)
	}
	if x2 <= x1 {
		t.Errorf("текст со значком начинается на %d, без значка — на %d: сдвига нет", x2, x1)
	}
}

// Щелчок ставит каретку туда, куда щёлкнули, а не левее на ширину значка.
//
// Позицию каретки наружу никто не отдаёт, да и незачем: важно, куда попадёт
// набранный символ. Это и проверяем — так же, как это увидит человек.
func TestLeadingIcon_CaretFollowsTheClick(t *testing.T) {
	insertPos := func(icon image.Image) int {
		ti := widget.NewTextInput("")
		ti.LeadingIcon = icon
		ti.SetText("ААААААААА")
		eng, _ := inputScene(t, ti)
		// Точка недалеко от начала текста: у конца строки каретка встаёт в
		// конец при любом отступе, и сдвига было бы не различить.
		eng.SendMouseButton(60, 36, widget.MouseLeft, true)
		eng.SendMouseButton(60, 36, widget.MouseLeft, false)
		ti.SetFocused(true)
		ti.OnKeyEvent(widget.KeyEvent{Rune: 'Б', Pressed: true})
		return indexOfB(ti.GetText())
	}

	plain := insertPos(nil)
	iconed := insertPos(redIcon(16))

	if plain == 0 {
		t.Fatal("щелчок по середине поля не сдвинул каретку — проверять нечего")
	}
	// Со значком та же точка приходится на более ранний символ: текст уехал
	// вправо. Если бы хит-тест не знал про значок, позиция совпала бы.
	if iconed >= plain {
		t.Errorf("символ вставлен на позицию %d со значком и %d без него — "+
			"хит-тест не учёл значок", iconed, plain)
	}
}

// indexOfB — на какой позиции оказался набранный символ, то есть куда встала
// каретка после щелчка.
func indexOfB(s string) int {
	for i, c := range []rune(s) {
		if c == 'Б' {
			return i
		}
	}
	return -1
}

// Поле без значка ведёт себя ровно как раньше.
func TestLeadingIcon_AbsentChangesNothing(t *testing.T) {
	a := widget.NewTextInput("подсказка")
	_, img1 := inputScene(t, a)

	b := widget.NewTextInput("подсказка")
	b.LeadingIconSize = 24 // размер без картинки ничего не значит
	_, img2 := inputScene(t, b)

	for i := range img1.Pix {
		if img1.Pix[i] != img2.Pix[i] {
			t.Fatal("поле без значка нарисовано иначе")
		}
	}
}
