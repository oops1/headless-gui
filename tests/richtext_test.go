package tests

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// RichText на настоящем движке: снимок кадра проверяется пикселями, а события
// идут тем же путём, что от человека (нажатия, номер серии, фокус, Ctrl+C).

// richEngine — движок с одним RichText на тёмном фоне, текст белый.
func richEngine(t *testing.T, bounds image.Rectangle, paras ...widget.RichParagraph) (*engine.Engine, *widget.RichText) {
	t.Helper()
	widget.UseMemoryClipboard()
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 600, 400))

	rt := widget.NewRichText()
	rt.Background = color.RGBA{A: 255}
	rt.TextColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	rt.SetBounds(bounds)
	rt.SetParagraphs(paras)
	root.AddChild(rt)

	eng := engine.New(600, 400, 20)
	eng.SetTooltipsEnabled(false)
	eng.SetRoot(root)
	return eng, rt
}

// ink — охватывающий прямоугольник «чернил»: пикселей, заметно отличающихся от
// чёрного фона, внутри области r.
func richInk(img *image.RGBA, r image.Rectangle) image.Rectangle {
	box := image.Rectangle{Min: image.Pt(1<<20, 1<<20), Max: image.Pt(-1, -1)}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if int(c.R)+int(c.G)+int(c.B) > 3*100 {
				box.Min.X, box.Min.Y = min(box.Min.X, x), min(box.Min.Y, y)
				box.Max.X, box.Max.Y = max(box.Max.X, x+1), max(box.Max.Y, y+1)
			}
		}
	}
	if box.Max.X < 0 {
		return image.Rectangle{}
	}
	return box
}

// Главный довод: мелкий и крупный ран стоят на ОДНОЙ базовой линии, а крупный
// не обрезан сверху — строка выше базового кегля виджета.
func TestRichTextRender_MixedSizesShareBaseline(t *testing.T) {
	const small, big = 10.0, 40.0
	bounds := image.Rect(20, 20, 420, 220)
	eng, rt := richEngine(t, bounds,
		widget.RichParagraph{Runs: []widget.RichRun{
			{Text: "xxxx", Size: small},
			{Text: "HHHH", Size: big},
		}},
		widget.RichParagraph{Runs: []widget.RichRun{{Text: "below"}}},
	)
	frame := eng.RenderOnce()
	if frame == nil {
		t.Fatal("кадра нет")
	}

	// Опорные координаты — из тех же источников, что у раскладки.
	wSmall := widget.MeasureUITextFont("xxxx", small, "")
	wBig := widget.MeasureUITextFont("HHHH", big, "")
	left := bounds.Min.X + rt.PaddingX
	top := bounds.Min.Y + rt.PaddingY
	fmBig := widget.MeasureUIFontMetrics(big, "")
	fmSmall := widget.MeasureUIFontMetrics(small, "")

	lineH := fmBig.Ascent + fmBig.Descent + rt.LineSpacing
	rowsBand := func(x0, x1 int) image.Rectangle { return image.Rect(x0, top, x1, top+lineH) }

	smallInk := richInk(frame, rowsBand(left, left+wSmall))
	bigInk := richInk(frame, rowsBand(left+wSmall, left+wSmall+wBig))
	if smallInk.Empty() || bigInk.Empty() {
		t.Fatalf("раны не нарисованы: мелкий %v, крупный %v", smallInk, bigInk)
	}

	// Одна базовая линия: «x» и «H» стоят на ней плоским низом.
	if smallInk.Max.Y != bigInk.Max.Y {
		t.Errorf("низ мелкого рана на %d, крупного на %d — разные базовые линии",
			smallInk.Max.Y, bigInk.Max.Y)
	}
	// И эта линия — подъём крупного шрифта от верха строки (±1 на округление).
	if want := top + fmBig.Ascent; richAbs(bigInk.Max.Y, want) > 1 {
		t.Errorf("базовая линия на %d, по метрикам движка ждали около %d", bigInk.Max.Y, want)
	}
	// Крупный ран не обрезан: его верх внутри строки, а не прижат к краю
	// виджета (ошибка «высота по базовому кеглю» срезала бы его сверху).
	if bigInk.Min.Y < top {
		t.Errorf("крупный ран вылез выше строки: чернила с %d, строка с %d", bigInk.Min.Y, top)
	}
	if bigInk.Dy() < 2*smallInk.Dy() {
		t.Errorf("крупный ран не крупнее: %d против %d px", bigInk.Dy(), smallInk.Dy())
	}
	// Мелкий ран отстоит от верха строки на разницу подъёмов — он не «прилип»
	// к верху строки, а встал на базовую линию.
	if smallInk.Min.Y-top < fmBig.Ascent-fmSmall.Ascent {
		t.Errorf("мелкий ран выше своего места: верх %d", smallInk.Min.Y)
	}

	// Следующий абзац начинается под всей первой строкой, включая спуск.
	belowInk := richInk(frame, image.Rect(left, top+lineH-2, bounds.Max.X, bounds.Max.Y))
	if belowInk.Empty() {
		t.Fatal("второй абзац не нарисован")
	}
	if belowInk.Min.Y < top+lineH-2 || belowInk.Min.Y < bigInk.Max.Y {
		t.Errorf("второй абзац наехал на первую строку: чернила с %d, база первой %d",
			belowInk.Min.Y, bigInk.Max.Y)
	}
}

// Подчёркивание: положение и толщина идут от метрик шрифта — у крупного
// шрифта линия толще и лежит ниже базовой линии, но в пределах спуска.
func TestRichTextRender_UnderlineScalesWithFont(t *testing.T) {
	bounds := image.Rect(20, 20, 500, 260)
	eng, rt := richEngine(t, bounds,
		widget.RichParagraph{Runs: []widget.RichRun{{Text: "WWWW", Size: 10, Underline: true}}},
		widget.RichParagraph{Runs: []widget.RichRun{{Text: "WWWW", Size: 48, Underline: true}}, SpaceBefore: 20},
	)
	frame := eng.RenderOnce()

	left := bounds.Min.X + rt.PaddingX
	fm10 := widget.MeasureUIFontMetrics(10, "")
	fm48 := widget.MeasureUIFontMetrics(48, "")
	lh := func(fm widget.FontMetrics) int { return fm.Ascent + fm.Descent + rt.LineSpacing }

	// Линия подчёркивания — строка пикселей, залитая почти по всей ширине рана.
	thickness := func(top int, fm widget.FontMetrics, size float64) (rows int, firstBelow int) {
		w := widget.MeasureUITextFont("WWWW", size, "")
		baseline := top + fm.Ascent
		firstBelow = -1
		for y := baseline; y < baseline+fm.Descent+2; y++ {
			n := 0
			for x := left; x < left+w; x++ {
				c := frame.RGBAAt(x, y)
				if int(c.R)+int(c.G)+int(c.B) > 3*100 {
					n++
				}
			}
			if n*10 >= w*9 {
				rows++
				if firstBelow < 0 {
					firstBelow = y - baseline
				}
			}
		}
		return rows, firstBelow
	}
	top1 := bounds.Min.Y + rt.PaddingY
	top2 := top1 + lh(fm10) + 20
	r10, off10 := thickness(top1, fm10, 10)
	r48, off48 := thickness(top2, fm48, 48)
	if r10 == 0 || r48 == 0 {
		t.Fatalf("подчёркивание не найдено: 10 pt — %d рядов, 48 pt — %d", r10, r48)
	}
	if r48 <= r10 {
		t.Errorf("линия не толще у крупного шрифта: %d и %d рядов", r10, r48)
	}
	if off48 <= off10 {
		t.Errorf("линия не дальше от базовой у крупного шрифта: %d и %d", off10, off48)
	}
	if off10 < 1 || off48 >= fm48.Descent {
		t.Errorf("линия вне спуска: смещения %d и %d, спуск 48 pt — %d", off10, off48, fm48.Descent)
	}
}

// Прокрутка через движок: колесо двигает текст, и в кадре видны другие строки.
func TestRichTextRender_ScrollingShowsOtherLines(t *testing.T) {
	var paras []widget.RichParagraph
	for i := 0; i < 80; i++ {
		paras = append(paras, widget.RichParagraph{Runs: []widget.RichRun{{Text: "WWWWWWWW line"}}})
	}
	bounds := image.Rect(20, 20, 320, 120)
	eng, rt := richEngine(t, bounds, paras...)
	before := eng.RenderOnce()

	eng.SendMouseWheelPixels(100, 60, 0, 120)
	if rt.ScrollY() == 0 {
		t.Fatal("колесо не прокрутило текст")
	}
	after := eng.RenderOnce()
	if richImagesEqual(before, after, bounds) {
		t.Error("кадр не изменился после прокрутки")
	}
}

func richImagesEqual(a, b *image.RGBA, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

func richAbs(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// Ссылка, двойной щелчок и Ctrl+C — всё через движок: номер серии считает он,
// фокус получает виджет по щелчку, клавиша идёт в сфокусированный виджет.
func TestRichTextEngine_LinkDoubleClickAndCopy(t *testing.T) {
	bounds := image.Rect(20, 20, 420, 120)
	eng, rt := richEngine(t, bounds, widget.RichParagraph{Runs: []widget.RichRun{
		{Text: "see "},
		{Text: "here", Link: "https://example.com", Font: widget.BuiltinFontBold},
		{Text: " plain end", Color: color.RGBA{R: 200, A: 255}},
	}})
	eng.RenderOnce()

	var opened []string
	rt.OnLinkClick = func(u string) { opened = append(opened, u) }

	left := bounds.Min.X + rt.PaddingX
	y := bounds.Min.Y + rt.PaddingY + 6
	wSee := widget.MeasureUITextFont("see ", widget.DefaultFontSizePt, "")
	wHere := widget.MeasureUITextFont("here", widget.DefaultFontSizePt, widget.BuiltinFontBold)
	linkX := left + wSee + wHere/2

	// Курсор над ссылкой — рука, над соседним текстом — I-образный.
	if c := rt.Cursor(linkX, y); c != widget.CursorHand {
		t.Errorf("курсор над ссылкой %v", c)
	}
	if c := rt.Cursor(left+2, y); c != widget.CursorIBeam {
		t.Errorf("курсор над текстом %v", c)
	}

	// Щелчок по ссылке — колбэк с адресом.
	eng.SendMouseButton(linkX, y, widget.MouseLeft, true)
	eng.SendMouseButton(linkX, y, widget.MouseLeft, false)
	if len(opened) != 1 || opened[0] != "https://example.com" {
		t.Fatalf("щелчок по ссылке: вызовы %v", opened)
	}

	// Двойной щелчок на слове «plain» выделяет его — и только его.
	plainX := left + wSee + wHere + widget.MeasureUITextFont(" pl", widget.DefaultFontSizePt, "")
	for i := 0; i < 2; i++ {
		eng.SendMouseButton(plainX, y, widget.MouseLeft, true)
		eng.SendMouseButton(plainX, y, widget.MouseLeft, false)
	}
	if got := rt.SelectedText(); got != "plain" {
		t.Fatalf("двойной щелчок выделил %q, ждали plain", got)
	}
	if len(opened) != 1 {
		t.Errorf("двойной щелчок по обычному тексту открыл ссылку: %v", opened)
	}

	// Ctrl+C: в буфере и простой текст, и HTML.
	rt.SelectAll()
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyC, Mod: widget.ModCtrl, Pressed: true})
	if got := widget.ClipboardGetText(); got != "see here plain end" {
		t.Errorf("простой текст в буфере %q", got)
	}
	html, ok := widget.ClipboardHTML()
	if !ok {
		t.Fatal("HTML в буфер не попал")
	}
	for _, want := range []string{
		`<a href="https://example.com">`, "font-weight:bold", "color:#c80000", "plain end",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("в HTML нет %q:\n%s", want, html)
		}
	}
}
