package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Вид выключенной кнопки — запрос GG-31.
//
// Общий оверлей выключенного виджета красит его ЧЁРНЫМ поверх всего и
// прямоугольником. В тёмной теме это читается как «погашено», а в светлой
// выключенная кнопка выходила темнее и контрастнее соседней рабочей —
// притягивала взгляд и читалась нажатой, — да ещё и с прямыми углами поверх
// скруглённых.

// buttonPair рисует две кнопки в теме и возвращает кадр и их границы.
func buttonPair(t *testing.T, th *widget.Theme) (*image.RGBA, image.Rectangle, image.Rectangle) {
	t.Helper()
	root := widget.NewPanel(th.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 340, 70))

	on := widget.NewButton("Сохранить")
	on.SetBounds(image.Rect(16, 18, 156, 52))
	root.AddChild(on)
	off := widget.NewButton("Закрыть")
	off.SetBounds(image.Rect(176, 18, 316, 52))
	off.SetEnabled(false)
	root.AddChild(off)

	eng := engine.New(340, 70, 30)
	eng.SetRoot(root)
	eng.SetTheme(th)
	eng.RenderOnce()
	return snapshotRGBA(eng.RenderOnce()), on.Bounds(), off.Bounds()
}

// meanLum — средняя яркость области кадра.
func meanLum(img *image.RGBA, r image.Rectangle) int {
	sum, n := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			i := img.PixOffset(x, y)
			sum += (299*int(img.Pix[i]) + 587*int(img.Pix[i+1]) + 114*int(img.Pix[i+2])) / 1000
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

// В светлой теме выключенная кнопка обязана быть НЕ ТЕМНЕЕ рабочей: иначе она
// тяжелее соседки и читается как нажатая.
func TestDisabledButton_LightThemeIsNotHeavier(t *testing.T) {
	img, on, off := buttonPair(t, widget.Win11LightTheme())

	// Сравниваем фон, а не всю кнопку: у рабочей текст чёрный и сильно
	// тянет среднее вниз. Полоса под подписью — чистый фон обеих.
	strip := func(r image.Rectangle) image.Rectangle {
		return image.Rect(r.Min.X+6, r.Max.Y-8, r.Max.X-6, r.Max.Y-3)
	}
	lOn, lOff := meanLum(img, strip(on)), meanLum(img, strip(off))
	if lOff < lOn {
		t.Errorf("в светлой теме выключенная кнопка темнее рабочей: %d против %d", lOff, lOn)
	}
}

// В тёмной теме — наоборот: выключенная не светлее рабочей.
func TestDisabledButton_DarkThemeStaysDim(t *testing.T) {
	img, on, off := buttonPair(t, widget.Win11DarkTheme())

	strip := func(r image.Rectangle) image.Rectangle {
		return image.Rect(r.Min.X+6, r.Max.Y-8, r.Max.X-6, r.Max.Y-3)
	}
	lOn, lOff := meanLum(img, strip(on)), meanLum(img, strip(off))
	if lOff > lOn {
		t.Errorf("в тёмной теме выключенная кнопка светлее рабочей: %d против %d", lOff, lOn)
	}
}

// Силуэт у выключенной тот же: скруглённые углы, а не прямоугольник поверх них.
func TestDisabledButton_KeepsRoundedCorners(t *testing.T) {
	th := widget.Win11LightTheme()
	img, on, off := buttonPair(t, th)

	corner := func(r image.Rectangle) (uint8, uint8, uint8) {
		i := img.PixOffset(r.Min.X, r.Min.Y)
		return img.Pix[i], img.Pix[i+1], img.Pix[i+2]
	}
	onR, onG, onB := corner(on)
	offR, offG, offB := corner(off)
	if onR != offR || onG != offG || onB != offB {
		t.Errorf("угол выключенной кнопки (%d,%d,%d) не совпал с углом рабочей (%d,%d,%d) — "+
			"скругление закрашено прямоугольником", offR, offG, offB, onR, onG, onB)
	}
}

// Текст выключенной кнопки приглушён: это и есть главный признак.
func TestDisabledButton_TextIsMuted(t *testing.T) {
	img, on, off := buttonPair(t, widget.Win11LightTheme())

	// Полоса по высоте подписи: у рабочей текст тёмный, у выключенной —
	// растворённый в фоне, значит средняя яркость выше.
	line := func(r image.Rectangle) image.Rectangle {
		mid := (r.Min.Y + r.Max.Y) / 2
		return image.Rect(r.Min.X+8, mid-6, r.Max.X-8, mid+6)
	}
	if lOff, lOn := meanLum(img, line(off)), meanLum(img, line(on)); lOff <= lOn {
		t.Errorf("подпись выключенной кнопки не приглушена: %d против %d", lOff, lOn)
	}
}
