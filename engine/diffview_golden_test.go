package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Эталонные кадры контрола сравнения DiffView: карточки, S-коннекторы,
// внутристрочная разница, подсветка синтаксиса, кнопки переноса, полоса-обзор,
// каретка с выделением и свёртка одинаковых строк.
//
// В тексте только символы встроенных Go-шрифтов: глиф из системного запасного
// шрифта у каждой ОС свой, и эталон расходился бы между CI-машинами.

const diffGoldenLeft = `package config

import "strings"

// Options — настройки панели.
type Options struct {
	Name    string
	Enabled bool
}

func New(name string) Options {
	return Options{Name: name, Enabled: true}
}

func (o Options) Title() string {
	return strings.ToUpper(o.Name)
}

func (o Options) Valid() bool {
	return o.Name != ""
}

func (o Options) Key() string {
	return "panel/" + o.Name
}
`

const diffGoldenRight = `package config

import "strings"

// Options — настройки панели.
type Options struct {
	Name    string
	Enabled bool
	Order   int
}

func New(name string) Options {
	return Options{Name: name, Enabled: false}
}

func (o Options) Valid() bool {
	return len(o.Name) > 0 && o.Order >= 0
}

func (o Options) Key() string {
	return "panel/" + o.Name
}
`

func diffGoldenScene(t *testing.T, th *widget.Theme, prep func(d *widget.DiffView)) *image.RGBA {
	t.Helper()
	const w, h = 1040, 440
	eng := New(w, h, 20)
	c := eng.canvas

	root := widget.NewPanel(th.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	d := widget.NewDiffView("", "")
	d.SetBounds(image.Rect(0, 0, w, h))
	d.SetText(widget.DiffLeft, "config.go", "base/pkg/config", diffGoldenLeft)
	d.SetText(widget.DiffRight, "config.go", "work/pkg/config", diffGoldenRight)
	root.AddChild(d)
	widget.ApplyThemeTree(root, th)
	prep(d)

	c.blitBackground()
	root.Draw(c)
	return c.back
}

func TestGolden_DiffView(t *testing.T) {
	withTheme(t, "Win11 Light", func(th *widget.Theme) {
		img := diffGoldenScene(t, th, func(d *widget.DiffView) {
			d.SetReadOnly(widget.DiffLeft, true)
			d.SetFocused(true)
			d.SetCaret(widget.DiffRight, 8, 13)
			// Правка помечает сторону изменённой.
			d.InsertText(" // порядок")
			// Выделение «false}» до конца строки.
			d.SetCaret(widget.DiffRight, 12, 37)
			d.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnd, Mod: widget.ModShift, Pressed: true})
			d.OnMouseMove(520, 300)
		})
		goldenCompare(t, "diffview_win11_light", img)
	})
	withTheme(t, "Win10 Dark", func(th *widget.Theme) {
		img := diffGoldenScene(t, th, func(d *widget.DiffView) {
			d.SetContextLines(1)
			d.SetHideUnchanged(true)
			d.GoToChange(1)
		})
		goldenCompare(t, "diffview_win10_dark", img)
	})
}
