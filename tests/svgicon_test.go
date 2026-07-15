package tests

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Тестовые SVG (иконки Material-стиля) ────────────────────────────────────

const svgMenu = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <path fill="currentColor" d="M3 6h18v2H3V6zm0 5h18v2H3v-2zm0 5h18v2H3v-2z"/>
</svg>`

const svgClose = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <path fill="currentColor" d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
</svg>`

const svgFolder = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <path fill="currentColor" d="M10 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/>
</svg>`

// ─── Парсинг/загрузка ────────────────────────────────────────────────────────

func TestSVGIcon_SetSVG(t *testing.T) {
	ic := widget.NewSVGIcon()
	if err := ic.SetSVG([]byte(svgMenu)); err != nil {
		t.Fatalf("SetSVG error: %v", err)
	}
	doc := ic.Document()
	if doc == nil {
		t.Fatal("Document() == nil после SetSVG")
	}
	if doc.ViewBox != [4]float64{0, 0, 24, 24} {
		t.Errorf("viewBox=%v", doc.ViewBox)
	}
	if len(doc.Shapes) != 1 || !doc.Shapes[0].FillCurrent {
		t.Errorf("menu: ожидалась 1 фигура с currentColor, got %+v", doc.Shapes)
	}
}

func TestSVGIcon_AllIconsParse(t *testing.T) {
	for name, src := range map[string]string{"menu": svgMenu, "close": svgClose, "folder": svgFolder} {
		ic := widget.NewSVGIcon()
		if err := ic.SetSVG([]byte(src)); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if ic.Document() == nil || len(ic.Document().Shapes) == 0 {
			t.Errorf("%s: нет фигур", name)
		}
	}
}

func TestSVGIcon_BadXML(t *testing.T) {
	ic := widget.NewSVGIcon()
	if err := ic.SetSVG([]byte("<svg><path d=")); err == nil {
		t.Error("ожидалась ошибка разбора битого XML")
	}
	if ic.Err() == nil {
		t.Error("Err() должен вернуть ошибку")
	}
}

// ─── Темизация и перекраска ──────────────────────────────────────────────────

func TestSVGIcon_ApplyThemeUsesLabelText(t *testing.T) {
	ic := widget.NewSVGIcon()
	dark := widget.DarkTheme()
	ic.ApplyTheme(dark)
	if ic.Color() != dark.LabelText {
		t.Errorf("ApplyTheme: цвет=%v want LabelText=%v", ic.Color(), dark.LabelText)
	}
	light := widget.LightTheme()
	ic.ApplyTheme(light)
	if ic.Color() != light.LabelText {
		t.Errorf("после смены темы цвет=%v want %v", ic.Color(), light.LabelText)
	}
}

func TestSVGIcon_ExplicitColorNotOverriddenByTheme(t *testing.T) {
	ic := widget.NewSVGIcon()
	red := color.RGBA{255, 0, 0, 255}
	ic.SetColor(red)
	ic.ApplyTheme(widget.DarkTheme())
	if ic.Color() != red {
		t.Errorf("явный цвет перезаписан темой: %v", ic.Color())
	}
}

func TestSVGIcon_TintFlag(t *testing.T) {
	ic := widget.NewSVGIcon()
	if ic.Tint() {
		t.Error("Tint по умолчанию должен быть false")
	}
	ic.SetTint(true)
	if !ic.Tint() {
		t.Error("SetTint(true) не применился")
	}
}

// ─── Рендеринг через движок ──────────────────────────────────────────────────

// readFirstFrame собирает первый непустой кадр в RGBA-буфер (physical size).
func readFirstFrame(t *testing.T, eng *engine.Engine, w, h int) *image.RGBA {
	t.Helper()
	buf := image.NewRGBA(image.Rect(0, 0, w, h))
	frames := eng.Frames()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case f := <-frames:
			if len(f.Tiles) == 0 {
				continue
			}
			for _, tile := range f.Tiles {
				for ty := 0; ty < tile.H; ty++ {
					for tx := 0; tx < tile.W; tx++ {
						si := (ty*tile.W + tx) * 4
						px, py := tile.X+tx, tile.Y+ty
						if px < 0 || py < 0 || px >= w || py >= h {
							continue
						}
						di := buf.PixOffset(px, py)
						buf.Pix[di+0] = tile.Data[si+0]
						buf.Pix[di+1] = tile.Data[si+1]
						buf.Pix[di+2] = tile.Data[si+2]
						buf.Pix[di+3] = tile.Data[si+3]
					}
				}
			}
			return buf
		case <-deadline:
			return nil
		}
	}
}

// TestSVGIcon_RenderTinted: зелёная (tint) иконка-круг на чёрном фоне —
// центр bounds становится зелёным.
func TestSVGIcon_RenderTinted(t *testing.T) {
	const w, h = 128, 128
	eng := engine.New(w, h, 30)
	eng.SetTooltipsEnabled(false)

	root := widget.NewPanel(color.RGBA{0, 0, 0, 255})
	root.SetBounds(image.Rect(0, 0, w, h))

	ic := widget.NewSVGIcon()
	if err := ic.SetSVG([]byte(`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="11" fill="#ffffff"/></svg>`)); err != nil {
		t.Fatal(err)
	}
	ic.SetColor(color.RGBA{0, 255, 0, 255})
	ic.SetTint(true)
	ic.SetBounds(image.Rect(24, 24, 104, 104)) // 80×80 в центре
	root.AddChild(ic)

	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	buf := readFirstFrame(t, eng, w, h)
	if buf == nil {
		t.Fatal("кадр не получен")
	}

	// Центр иконки (64,64) — зелёный.
	r, g, b, a := buf.At(64, 64).RGBA()
	if !(g>>8 > 180 && r>>8 < 80 && b>>8 < 80 && a>>8 > 200) {
		t.Errorf("центр иконки rgba(%d,%d,%d,%d) не зелёный", r>>8, g>>8, b>>8, a>>8)
	}

	// Угол панели вне иконки (2,2) — чёрный.
	r2, g2, b2, _ := buf.At(2, 2).RGBA()
	if r2>>8 > 40 || g2>>8 > 40 || b2>>8 > 40 {
		t.Errorf("угол панели rgb(%d,%d,%d) не чёрный", r2>>8, g2>>8, b2>>8)
	}
}
