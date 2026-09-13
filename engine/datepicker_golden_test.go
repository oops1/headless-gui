package engine

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Эталонные кадры поля даты: поле в фокусе с выбранной датой и открытый
// календарь — стрелки месяцев, дни недели от первого дня культуры, числа
// соседних месяцев, выбранный день, сегодняшний, день под мышью и границы
// выбора. Язык и «сегодня» зафиксированы: кадр не должен зависеть ни от
// языка машины, ни от даты прогона.

func datePickerGoldenScene(t *testing.T, th *widget.Theme) *image.RGBA {
	t.Helper()
	prevLang := widget.Language()
	widget.SetLanguage("RU")
	t.Cleanup(func() { widget.SetLanguage(prevLang) })

	const w, h = 300, 300
	eng := New(w, h, 20)
	c := eng.canvas

	root := widget.NewPanel(th.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	p := widget.NewDatePicker()
	p.Now = func() time.Time { return time.Date(2026, time.September, 13, 12, 0, 0, 0, time.Local) }
	p.SetBounds(image.Rect(20, 20, 240, 52))
	root.AddChild(p)
	widget.ApplyThemeTree(root, th)

	p.SetDisplayDateRange(time.Date(2026, time.September, 3, 0, 0, 0, 0, time.Local), time.Time{})
	p.SetSelectedDate(time.Date(2026, time.September, 17, 0, 0, 0, 0, time.Local))
	p.SetFocused(true)
	p.SetDropDownOpen(true)
	// Мышь над числом во второй строке сетки.
	p.OnMouseMove(140, 181)

	c.blitBackground()
	root.Draw(c)
	p.DrawOverlay(c)
	return c.back
}

func TestGolden_DatePicker(t *testing.T) {
	withTheme(t, "Win11 Light", func(th *widget.Theme) {
		goldenCompare(t, "datepicker_win11_light", datePickerGoldenScene(t, th))
	})
	withTheme(t, "Win2000", func(th *widget.Theme) {
		goldenCompare(t, "datepicker_win2000", datePickerGoldenScene(t, th))
	})
}
