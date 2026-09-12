package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Эталонные кадры контрола слияния MergeView: три стороны сверху, итог снизу,
// выровненные по блокам строки, полосы конфликтов, кнопки решения, маркеры git
// в нерешённом конфликте и полоса-обзор.
//
// В тексте только символы встроенных Go-шрифтов: глиф из системного запасного
// шрифта у каждой ОС свой, и эталон расходился бы между CI-машинами.

const mergeGoldenBase = `package config

type Options struct {
	Name    string
	Enabled bool
}

func New(name string) Options {
	return Options{Name: name}
}

func (o Options) Valid() bool {
	return o.Name != ""
}
`

const mergeGoldenOurs = `package config

type Options struct {
	Name    string
	Enabled bool
	Order   int
}

func New(name string) Options {
	return Options{Name: name, Order: 1}
}

func (o Options) Valid() bool {
	return o.Name != ""
}
`

const mergeGoldenTheirs = `package config

type Options struct {
	Name    string
	Enabled bool
	Tag     string
}

func New(name string) Options {
	return Options{Name: name, Tag: "ui"}
}

func (o Options) Valid() bool {
	return o.Name != ""
}
`

func mergeGoldenScene(t *testing.T, th *widget.Theme, prep func(m *widget.MergeView)) *image.RGBA {
	t.Helper()
	const w, h = 1100, 520
	eng := New(w, h, 20)
	c := eng.canvas

	root := widget.NewPanel(th.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	m := widget.NewMergeView("", "")
	m.SetBounds(image.Rect(0, 0, w, h))
	m.SetTexts(mergeGoldenBase, mergeGoldenOurs, mergeGoldenTheirs)
	m.SetSides(
		widget.MergeSideInfo{Title: "main", Note: "config.go"},
		widget.MergeSideInfo{Title: "merge-base", Note: "config.go"},
		widget.MergeSideInfo{Title: "feature/tag", Note: "config.go"},
	)
	root.AddChild(m)
	widget.ApplyThemeTree(root, th)
	prep(m)

	c.blitBackground()
	root.Draw(c)
	return c.back
}

func TestGolden_MergeView(t *testing.T) {
	withTheme(t, "Win11 Light", func(th *widget.Theme) {
		img := mergeGoldenScene(t, th, func(m *widget.MergeView) {
			m.SetFocused(true)
			m.GoToConflict(0)
			// Курсор над кнопкой «взять наше» первого конфликта.
			m.OnMouseMove(330, 150)
		})
		goldenCompare(t, "mergeview_win11_light", img)
	})
	withTheme(t, "Win10 Dark", func(th *widget.Theme) {
		img := mergeGoldenScene(t, th, func(m *widget.MergeView) {
			// Без базы, первый конфликт закрыт нашей стороной, каретка в итоге.
			m.SetShowBase(false)
			m.SetStyle(widget.MergeStyleDiff3)
			m.SetFocused(true)
			m.Resolve(1, widget.MergeTakeOurs)
			m.SetCaret(4, 0)
		})
		goldenCompare(t, "mergeview_win10_dark", img)
	})
}
