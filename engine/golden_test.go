// golden_test.go — пиксельные снапшот-тесты рендера (golden files).
//
// Каждая сцена рендерится в PNG и побайтово сравнивается с эталоном из
// testdata/golden/. Регенерация эталонов после ОСОЗНАННОГО изменения рендера:
//
//	GOLDEN_UPDATE=1 go test ./engine/ -run TestGolden
//
// Детерминированность: сцены используют только встроенные шрифты (Go Regular
// + Go Bold/Italic) и руны, покрытые ими, — системные fallback-шрифты не
// задействуются, поэтому эталоны совпадают на всех ОС. Шейпинг сложных
// скриптов в golden не входит (зависит от шрифтов системы) — он покрыт
// юнит-тестами в shaper_test.go.
package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// goldenCompare сравнивает кадр со снапшотом testdata/golden/<name>.png.
func goldenCompare(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".png")

	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		savePNG(img, path)
		t.Logf("эталон обновлён: %s", path)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("эталон %s отсутствует (сгенерируйте: GOLDEN_UPDATE=1 go test -run TestGolden): %v", path, err)
	}
	defer f.Close()
	wantImg, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}

	// Сравниваем в ЕДИНОМ представлении: текущий кадр прогоняется через тот же
	// PNG-roundtrip, что и эталон. PNG хранит непремультиплицированный цвет —
	// у AA-краёв глифов альфа бывает 254, и прямое сравнение сырых Pix с
	// декодированным эталоном давало ложные расхождения при идентичной картинке.
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	gotImg, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("decode roundtrip: %v", err)
	}

	if wantImg.Bounds() != gotImg.Bounds() {
		t.Fatalf("%s: размер %v, эталон %v", name, gotImg.Bounds(), wantImg.Bounds())
	}

	// Побайтовое сравнение в NRGBA.
	toN := func(src image.Image) *image.NRGBA {
		if n, ok := src.(*image.NRGBA); ok {
			return n
		}
		b := src.Bounds()
		n := image.NewNRGBA(b)
		stdDraw(n, src)
		return n
	}
	want, got := toN(wantImg), toN(gotImg)
	if bytes.Equal(want.Pix, got.Pix) {
		return
	}

	// Расхождение: считаем отличающиеся пиксели и сохраняем полученный кадр.
	diff := 0
	for i := 0; i < len(got.Pix); i += 4 {
		if got.Pix[i] != want.Pix[i] || got.Pix[i+1] != want.Pix[i+1] ||
			got.Pix[i+2] != want.Pix[i+2] || got.Pix[i+3] != want.Pix[i+3] {
			diff++
		}
	}
	gotPath := filepath.Join(os.TempDir(), fmt.Sprintf("golden_got_%s.png", name))
	savePNG(img, gotPath)
	t.Errorf("%s: %d пикселей отличаются от эталона %s; полученный кадр: %s",
		name, diff, path, gotPath)
}

// stdDraw копирует изображение в NRGBA-приёмник (нормализация формата).
func stdDraw(dst *image.NRGBA, src image.Image) {
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
}

// withTheme применяет тему на время сцены и восстанавливает Win10 Dark.
func withTheme(t *testing.T, name string, fn func(th *widget.Theme)) {
	t.Helper()
	th := widget.ThemeByName(name)
	if th == nil {
		t.Fatalf("тема %q не найдена", name)
	}
	widget.ApplyGlobalTheme(th)
	defer widget.ApplyGlobalTheme(widget.ThemeByName("Win10 Dark"))
	fn(th)
}

// themeSlug — имя файла-эталона из имени темы.
func themeSlug(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, r+32)
		case r == ' ':
			out = append(out, '_')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// TestGolden_WidgetsSampler — сэмплер основных виджетов в каждой теме.
func TestGolden_WidgetsSampler(t *testing.T) {
	for _, themeName := range []string{"Win10 Dark", "Win11 Light", "Win2000", "Mac"} {
		themeName := themeName
		t.Run(themeName, func(t *testing.T) {
			withTheme(t, themeName, func(th *widget.Theme) {
				eng := New(420, 320, 20)
				c := eng.canvas

				root := widget.NewPanel(th.WindowBG)
				root.SetBounds(image.Rect(0, 0, 420, 320))

				btn := widget.NewButton("Кнопка OK")
				btn.SetBounds(image.Rect(16, 16, 140, 48))
				root.AddChild(btn)

				btnH := widget.NewButton("Hover")
				btnH.SetBounds(image.Rect(152, 16, 250, 48))
				btnH.SetHovered(true)
				root.AddChild(btnH)

				cb := widget.NewCheckBox("Флажок")
				cb.SetBounds(image.Rect(16, 60, 180, 82))
				cb.SetChecked(true)
				root.AddChild(cb)

				rb := widget.NewRadioButton("Выбор", "g")
				rb.SetBounds(image.Rect(200, 60, 340, 82))
				rb.SetSelected(true)
				root.AddChild(rb)

				ts := widget.NewToggleSwitch("Тумблер")
				ts.SetBounds(image.Rect(16, 96, 180, 120))
				ts.SetOn(true)
				root.AddChild(ts)

				sl := widget.NewSlider()
				sl.SetBounds(image.Rect(200, 96, 400, 120))
				sl.SetValue(0.5)
				root.AddChild(sl)

				pb := widget.NewProgressBar()
				pb.SetBounds(image.Rect(16, 136, 400, 152))
				pb.SetValue(0.6)
				root.AddChild(pb)

				ti := widget.NewTextInput("Текст ввода")
				ti.SetBounds(image.Rect(16, 168, 250, 196))
				root.AddChild(ti)

				dd := widget.NewDropdown("Пункт 1", "Пункт 2")
				dd.SetBounds(image.Rect(266, 168, 400, 196))
				root.AddChild(dd)

				lbl := widget.NewLabel("Метка Label — Приветствие", th.LabelText)
				lbl.SetBounds(image.Rect(16, 212, 400, 232))
				root.AddChild(lbl)

				widget.ApplyThemeTree(root, th)
				c.blitBackground()
				root.Draw(c)
				goldenCompare(t, "widgets_"+themeSlug(themeName), c.back)
			})
		})
	}
}

// TestGolden_WindowChrome — окно активное/неактивное в Win2000 и Win11 Light.
func TestGolden_WindowChrome(t *testing.T) {
	for _, themeName := range []string{"Win2000", "Win11 Light"} {
		themeName := themeName
		t.Run(themeName, func(t *testing.T) {
			withTheme(t, themeName, func(th *widget.Theme) {
				eng := New(560, 230, 20)
				c := eng.canvas
				c.blitBackground()

				mk := func(x int, active bool, title string) {
					w := widget.NewWindow(title, 250, 190)
					w.SetBounds(image.Rect(x, 20, x+250, 210))
					widget.ApplyThemeTree(w, th)
					w.SetActive(active)
					w.Draw(c)
				}
				mk(20, true, "Активное")
				mk(290, false, "Неактивное")
				goldenCompare(t, "window_"+themeSlug(themeName), c.back)
			})
		})
	}
}

// TestGolden_AAShapes — сглаженные примитивы (углы, эллипсы, линии, полигон).
func TestGolden_AAShapes(t *testing.T) {
	eng := New(420, 240, 20)
	c := eng.canvas
	c.blitBackground()

	accent := color.RGBA{R: 0, G: 120, B: 212, A: 255}
	white := color.RGBA{R: 235, G: 235, B: 235, A: 255}
	orange := color.RGBA{R: 240, G: 150, B: 60, A: 255}

	for i, r := range []int{3, 8, 14} {
		c.FillRoundRect(16+i*110, 16, 96, 36, r, accent)
		c.DrawRoundBorder(16+i*110, 62, 96, 36, r, white)
	}
	c.FillEllipseAA(60, 150, 40, 26, accent)
	c.StrokeEllipseAA(160, 150, 40, 26, 3, orange)
	c.FillPolygonAA([]image.Point{{X: 230, Y: 124}, {X: 270, Y: 176}, {X: 230, Y: 176}}, white)
	for i := 0; i < 4; i++ {
		c.DrawLineAA(290, 128+i*14, 404, 122+i*18, 1+float64(i), white)
	}
	goldenCompare(t, "aa_shapes", c.back)
}

// TestGolden_HiDPI2x — та же кнопочная сцена при масштабе 2: текст должен
// растеризоваться в физическом размере (не растянутый 1x).
func TestGolden_HiDPI2x(t *testing.T) {
	eng := New(220, 80, 20)
	eng.SetScale(2)
	c := eng.canvas
	c.blitBackground()
	c.FillRoundRect(12, 12, 130, 34, 8, color.RGBA{R: 0, G: 120, B: 212, A: 255})
	c.DrawTextSize("Кнопка Button", 24, 22, 12, color.RGBA{R: 235, G: 235, B: 235, A: 255})
	c.DrawRoundBorder(150, 12, 60, 34, 8, color.RGBA{R: 235, G: 235, B: 235, A: 255})
	goldenCompare(t, "hidpi_2x", c.back)
}
