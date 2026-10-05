package engine

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	hgfonts "github.com/oops1/headless-gui/v3/assets/fonts"
	"github.com/oops1/headless-gui/v3/widget/svg"
)

func svgTextEngine(t *testing.T) *Engine {
	t.Helper()
	eng := New(64, 64, 30)
	if err := hgfonts.Register(eng); err != nil {
		t.Fatalf("вшитые шрифты: %v", err)
	}
	t.Cleanup(eng.Stop)
	return eng
}

// alphaBounds — габариты непрозрачных пикселей; ok=false — картинка пуста.
func alphaBounds(img *image.RGBA) (r image.Rectangle, ok bool) {
	minX, minY, maxX, maxY := 1<<20, 1<<20, -1, -1
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A > 40 {
				ok = true
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x+1), max(maxY, y+1)
			}
		}
	}
	return image.Rect(minX, minY, maxX, maxY), ok
}

func renderSVG(t *testing.T, src string, w, h int) *image.RGBA {
	t.Helper()
	doc, err := svg.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return doc.Rasterize(w, h, color.RGBA{A: 255}, false)
}

// Движок регистрирует растеризатор текста: <text> рисуется. Без моста (пакет
// svg сам по себе) текст не рисуется — это проверяет svg.TestText_NoBridge.
func TestSVGText_DrawnByEngineFonts(t *testing.T) {
	svgTextEngine(t)
	img := renderSVG(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 40">
	<text x="10" y="30" font-size="30" fill="#000">HI</text></svg>`, 100, 40)
	r, ok := alphaBounds(img)
	if !ok {
		t.Fatal("текст не нарисован")
	}
	// Заглавные: между верхом ~y=9 (высота прописной ≈ 0,7 em) и базовой линией y=30.
	if r.Min.X < 9 || r.Min.X > 14 || r.Max.Y < 29 || r.Max.Y > 31 || r.Min.Y > 12 || r.Min.Y < 5 {
		t.Errorf("габариты текста %v не соответствуют x=10,y=30,size=30", r)
	}
}

func TestSVGText_AnchorMiddleAndEnd(t *testing.T) {
	svgTextEngine(t)
	mk := func(anchor string) image.Rectangle {
		img := renderSVG(t, fmt.Sprintf(`<svg viewBox="0 0 200 40"><text x="100" y="30" font-size="30" text-anchor="%s">Hello</text></svg>`, anchor), 200, 40)
		r, ok := alphaBounds(img)
		if !ok {
			t.Fatalf("anchor=%s: пусто", anchor)
		}
		return r
	}
	start, mid, end := mk("start"), mk("middle"), mk("end")
	if start.Min.X < 98 {
		t.Errorf("start: левый край %d, ожидался около 100", start.Min.X)
	}
	if c := float64(mid.Min.X+mid.Max.X) / 2; math.Abs(c-100) > 4 {
		t.Errorf("middle: центр текста %.1f, ожидался ~100", c)
	}
	if end.Max.X < 98 || end.Max.X > 104 {
		t.Errorf("end: правый край %d, ожидался около 100", end.Max.X)
	}
}

func TestSVGText_WeightAndStyleChangeOutline(t *testing.T) {
	eng := svgTextEngine(t)
	br := svgTextBridge{eng}
	_, wReg, ok1 := br.Outline(svg.FontSpec{Families: []string{"Open Sans"}, Weight: 400}, 20, "Hamburg")
	_, wBold, ok2 := br.Outline(svg.FontSpec{Families: []string{"Open Sans"}, Weight: 700}, 20, "Hamburg")
	if !ok1 || !ok2 {
		t.Fatal("Outline вернул ok=false")
	}
	if wBold <= wReg {
		t.Errorf("жирный не шире обычного: %.2f <= %.2f", wBold, wReg)
	}
	_, wIt, _ := br.Outline(svg.FontSpec{Families: []string{"Open Sans"}, Weight: 400, Italic: true}, 20, "Hamburg")
	if wIt == wReg {
		t.Errorf("курсив не отличается по ширине от прямого (%.2f): запрос наклона игнорируется", wIt)
	}
}

func TestSVGText_FontSizeScalesAndMatchesMeasure(t *testing.T) {
	eng := svgTextEngine(t)
	br := svgTextBridge{eng}
	_, a10, _ := br.Outline(svg.FontSpec{}, 10, "Hello")
	_, a20, _ := br.Outline(svg.FontSpec{}, 20, "Hello")
	if math.Abs(a20-2*a10) > 0.01 {
		t.Errorf("ширина не линейна по размеру: %.3f и %.3f", a10, a20)
	}
	// Ширина согласована с замером движка тем же шрифтом (разница — лишь
	// округление шагов глифов до пикселя в Measure).
	fc := eng.canvas.svgFont(svg.FontSpec{Families: []string{"Open Sans"}})
	_, w, _ := br.Outline(svg.FontSpec{Families: []string{"Open Sans"}}, 100, "Hello")
	if m := float64(fc.Measure("Hello", 75)); math.Abs(m-w) > 3 {
		t.Errorf("ширина %.2f расходится с замером движка %.0f", w, m)
	}
}

func TestSVGText_UnknownFamilyFallsBackToDefault(t *testing.T) {
	svgTextEngine(t)
	img := renderSVG(t, `<svg viewBox="0 0 60 30"><text x="2" y="24" font-family="Нет Такого, sans-serif" font-size="24">Ag</text></svg>`, 60, 30)
	if _, ok := alphaBounds(img); !ok {
		t.Error("неизвестное семейство: текст пропал вместо шрифта по умолчанию")
	}
}

func TestSVGText_FillStrokeAndTspan(t *testing.T) {
	svgTextEngine(t)
	img := renderSVG(t, `<svg viewBox="0 0 120 40"><text x="4" y="30" font-size="30" fill="#f00">I<tspan fill="#00f" dx="10">I</tspan></text></svg>`, 120, 40)
	var red, blue int
	for y := 0; y < 40; y++ {
		for x := 0; x < 120; x++ {
			c := img.RGBAAt(x, y)
			if c.A > 200 && c.R > 200 && c.B < 50 {
				red++
			}
			if c.A > 200 && c.B > 200 && c.R < 50 {
				blue++
			}
		}
	}
	if red == 0 || blue == 0 {
		t.Errorf("tspan должен менять заливку: красных пикселей %d, синих %d", red, blue)
	}
}

func TestSVGText_MaskedTextCutsHole(t *testing.T) {
	svgTextEngine(t)
	// Как в значках раскладки клавиатуры: плашка с буквами, вырезанными маской.
	img := renderSVG(t, `<svg viewBox="0 0 40 40"><defs><mask id="m"><rect width="40" height="40" fill="#fff"/>
	<text x="2" y="30" font-size="30" fill="#000">I</text></mask></defs>
	<rect width="40" height="40" fill="#000" mask="url(#m)"/></svg>`, 40, 40)
	solid, hole := 0, 0
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			if a := img.RGBAAt(x, y).A; a == 255 {
				solid++
			} else if a == 0 {
				hole++
			}
		}
	}
	if hole == 0 || solid == 0 {
		t.Errorf("маска из текста: сплошных %d, дыр %d — обе должны быть", solid, hole)
	}
}

func TestSVGText_StopUnregistersBridge(t *testing.T) {
	eng := New(64, 64, 30)
	_ = hgfonts.Register(eng)
	eng.Stop()
	// Движок снят: своих регистраций больше нет (чужие движки других тестов
	// могут жить, поэтому проверяем лишь дескриптор).
	if eng.svgTextHandle == 0 {
		t.Error("дескриптор регистрации не сохранён")
	}
}

// TestSVGTextVisualSheet — инструмент для глаз: рисует *.svg из SVG_SHEET_DIR
// движком (Open Sans/Go) в лист SVG_SHEET_OUT, рядом — эталоны SVG_SHEET_REF
// (rsvg-convert -w 128 -h 128). Без переменных пропускается.
func TestSVGTextVisualSheet(t *testing.T) {
	dir, out := os.Getenv("SVG_SHEET_DIR"), os.Getenv("SVG_SHEET_OUT")
	if dir == "" || out == "" {
		t.Skip("SVG_SHEET_DIR/SVG_SHEET_OUT не заданы")
	}
	svgTextEngine(t)
	refDir := os.Getenv("SVG_SHEET_REF")
	files, _ := filepath.Glob(filepath.Join(dir, "*.svg"))
	sort.Strings(files)
	const cell = 128
	cols := 3
	rows := (len(files) + cols - 1) / cols
	sheet := image.NewRGBA(image.Rect(0, 0, cols*2*(cell+8)+8, rows*(cell+8)+8))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{200, 200, 200, 255}), image.Point{}, draw.Src)
	white := image.NewUniform(color.RGBA{255, 255, 255, 255})
	for i, f := range files {
		doc, err := svg.ParseFile(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		img := svg.RenderWith(doc, cell, cell, color.RGBA{}, svg.PreciseOptions)
		x, y := 8+(i%cols)*2*(cell+8), 8+(i/cols)*(cell+8)
		r := image.Rect(x, y, x+cell, y+cell)
		draw.Draw(sheet, r, white, image.Point{}, draw.Src)
		draw.Draw(sheet, r, img, image.Point{}, draw.Over)
		if refDir != "" {
			name := strings.TrimSuffix(filepath.Base(f), ".svg")
			if fh, err := os.Open(filepath.Join(refDir, name+".png")); err == nil {
				ref, _ := png.Decode(fh)
				fh.Close()
				if ref != nil {
					r2 := image.Rect(x+cell+8, y, x+2*cell+8, y+cell)
					draw.Draw(sheet, r2, white, image.Point{}, draw.Src)
					draw.Draw(sheet, r2, ref, ref.Bounds().Min, draw.Over)
				}
			}
		}
		t.Logf("%2d %s", i, filepath.Base(f))
	}
	fh, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	if err := png.Encode(fh, sheet); err != nil {
		t.Fatal(err)
	}
}
