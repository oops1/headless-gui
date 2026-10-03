package engine

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Метрики шрифта, открытые приложениям через widget.MeasureUIFontMetrics.
//
// Раньше подъём и спуск жили только внутри движка, и Блокнот разбирал TTF сам
// ради одной цифры. Проверяется главное: числа согласованы, масштабируются с
// кеглем и HiDPI и — самое важное — совпадают с тем, что делает отрисовка.

// loadAssetFont читает шрифт из assets/fonts репозитория и регистрирует его
// под именем name. Файла нет — тест пропускается: шрифты опциональны.
func loadAssetFont(t *testing.T, e *Engine, name, file string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "assets", "fonts", file))
	if err != nil {
		t.Skipf("шрифт %s недоступен: %v", file, err)
	}
	e.RegisterFont(name, data)
}

// metricsEngine создаёт движок (он регистрирует измеритель метрик) и снимает
// его по окончании теста.
func metricsEngine(t *testing.T) *Engine {
	t.Helper()
	e := New(200, 100, 20)
	t.Cleanup(e.Stop)
	return e
}

func checkConsistent(t *testing.T, label string, m widget.FontMetrics) {
	t.Helper()
	if m.Ascent <= 0 || m.Descent <= 0 || m.LineGap < 0 {
		t.Errorf("%s: метрики не положительны: %+v", label, m)
	}
	if m.Ascent <= m.Descent {
		t.Errorf("%s: подъём %d не больше спуска %d", label, m.Ascent, m.Descent)
	}
	if m.Height != m.Ascent+m.Descent+m.LineGap {
		t.Errorf("%s: Height %d != %d+%d+%d", label, m.Height, m.Ascent, m.Descent, m.LineGap)
	}
}

func TestFontMetrics_BuiltinConsistent(t *testing.T) {
	metricsEngine(t)
	for _, fam := range []string{"", widget.BuiltinFontBold, widget.BuiltinFontMono, widget.BuiltinFontItalic} {
		m := widget.MeasureUIFontMetrics(widget.DefaultFontSizePt, fam)
		t.Logf("family=%q: %+v", fam, m)
		checkConsistent(t, "family="+fam, m)
	}
}

// Крупный кегль даёт пропорционально большие метрики.
func TestFontMetrics_ScalesWithSize(t *testing.T) {
	metricsEngine(t)
	small := widget.MeasureUIFontMetrics(10, "")
	big := widget.MeasureUIFontMetrics(40, "")
	checkConsistent(t, "40pt", big)
	// Учетверённый кегль — учетверённые метрики с точностью до округления
	// каждой из частей (до 4 px на 4-кратном увеличении ошибки в 1 px).
	for _, c := range []struct {
		name       string
		small, big int
	}{
		{"Ascent", small.Ascent, big.Ascent},
		{"Descent", small.Descent, big.Descent},
		{"Height", small.Height, big.Height},
	} {
		if d := c.big - 4*c.small; d < -4 || d > 4 {
			t.Errorf("%s: 10pt=%d, 40pt=%d — не пропорционально", c.name, c.small, c.big)
		}
		if c.big <= c.small {
			t.Errorf("%s не вырос с кеглем: %d -> %d", c.name, c.small, c.big)
		}
	}
}

// Два разных шрифта дают разные метрики; незарегистрированное имя — шрифт по
// умолчанию, как и при отрисовке.
func TestFontMetrics_DifferentFonts(t *testing.T) {
	e := metricsEngine(t)
	loadAssetFont(t, e, "TestRoboto", "Roboto-Regular.ttf")
	loadAssetFont(t, e, "TestLiberation", "LiberationSans-Regular.ttf")

	a := widget.MeasureUIFontMetrics(40, "TestRoboto")
	b := widget.MeasureUIFontMetrics(40, "TestLiberation")
	t.Logf("Roboto 40pt: %+v; Liberation 40pt: %+v", a, b)
	checkConsistent(t, "Roboto", a)
	checkConsistent(t, "Liberation", b)
	if a == b {
		t.Errorf("разные шрифты дали одинаковые метрики: %+v", a)
	}

	def := widget.MeasureUIFontMetrics(40, "")
	if got := widget.MeasureUIFontMetrics(40, "нет-такого-шрифта"); got != def {
		t.Errorf("незарегистрированное имя: %+v, шрифт по умолчанию: %+v", got, def)
	}
}

// inkBottom возвращает нижнюю закрашенную строку буфера (физическую) или -1.
func inkBottom(c *Canvas) int {
	for y := c.H - 1; y >= 0; y-- {
		for x := 0; x < c.W; x++ {
			if c.back.Pix[c.back.PixOffset(x, y)+3] != 0 {
				return y
			}
		}
	}
	return -1
}

// Базовая линия DrawText лежит ровно на y + Ascent: заглавная «H» без свесов
// стоит на базовой линии, и последняя закрашенная строка — предыдущая перед ней.
// Это и есть обещание, ради которого метрики открыты: выровненный по Ascent
// текст встаёт туда, где его нарисует движок.
func TestFontMetrics_BaselineMatchesDrawText(t *testing.T) {
	e := metricsEngine(t)
	loadAssetFont(t, e, "TestRoboto", "Roboto-Regular.ttf")

	for _, scale := range []float64{1, 2, 1.5} {
		e.SetScale(scale)
		c := e.canvas
		for _, fam := range []string{"", "TestRoboto", widget.BuiltinFontMono} {
			for _, size := range []float64{10, 14, 24} {
				for i := range c.back.Pix {
					c.back.Pix[i] = 0
				}
				const y = 20
				c.DrawTextFont("H", 10, y, size, fam, color.RGBA{R: 255, G: 255, B: 255, A: 255})

				asc, _, _ := c.FontMetrics(fam, size)
				// Логический подъём — округление физического, поэтому
				// сверяем по физической базовой линии.
				physAsc := c.fontFor(fam).vMetricsFull(size).ascent
				want := c.sx(y) + physAsc - 1
				if got := inkBottom(c); got != want {
					t.Errorf("scale=%v fam=%q size=%v: низ глифа %d, ожидался %d (y+Ascent-1)",
						scale, fam, size, got, want)
				}
				// Логический Ascent отличается от физического/scale не
				// более чем на половину пикселя.
				if d := float64(asc) - float64(physAsc)/scale; d < -0.5 || d > 0.5 {
					t.Errorf("scale=%v fam=%q size=%v: Ascent %d при физическом %d", scale, fam, size, asc, physAsc)
				}
			}
		}
	}
}

// Метрики в логических пикселях: масштаб меняет их не более чем на пиксель
// округления — как ширины MeasureUIText.
func TestFontMetrics_LogicalAcrossScale(t *testing.T) {
	e := metricsEngine(t)
	base := widget.MeasureUIFontMetrics(14, "")
	for _, scale := range []float64{1.25, 1.5, 2, 3} {
		e.SetScale(scale)
		m := widget.MeasureUIFontMetrics(14, "")
		checkConsistent(t, "scale", m)
		if d := m.Height - base.Height; d < -2 || d > 2 {
			t.Errorf("scale=%v: Height %d, при 1x — %d", scale, m.Height, base.Height)
		}
		if d := m.Ascent - base.Ascent; d < -1 || d > 1 {
			t.Errorf("scale=%v: Ascent %d, при 1x — %d", scale, m.Ascent, base.Ascent)
		}
	}
}
