package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// SetScaleWithin держит физический размер и выводит логический; кадры
// остаются физическими; события делятся на масштаб.
func TestScaleWithin_KeepsPhysicalSize(t *testing.T) {
	testreset.Globals(t)
	eng := New(1920, 1080, 20)
	defer eng.Stop()
	root := widget.NewPanel(color.RGBA{R: 20, G: 40, B: 60, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 1920, 1080))
	eng.SetRoot(root)
	eng.RenderOnce()

	eng.SetScaleWithin(1.5)
	if !eng.ScaleWithin() {
		t.Fatal("режим не включился")
	}
	if pw, ph := eng.PhysicalSize(); pw != 1920 || ph != 1080 {
		t.Fatalf("PhysicalSize %dx%d, ждали 1920x1080", pw, ph)
	}
	if lw, lh := eng.CanvasSize(); lw != 1280 || lh != 720 {
		t.Fatalf("CanvasSize %dx%d, ждали 1280x720", lw, lh)
	}
	if b := root.Bounds(); b != image.Rect(0, 0, 1280, 720) {
		t.Fatalf("корень %v, ждали 1280x720", b)
	}
	img := eng.RenderOnce()
	if img.Rect.Dx() != 1920 || img.Rect.Dy() != 1080 {
		t.Fatalf("кадр %v, ждали 1920x1080", img.Rect)
	}
	// Правый нижний физический пиксель закрашен корнем: сцена накрыла кадр.
	if c := img.RGBAAt(1919, 1079); c.A != 255 || c.B != 60 {
		t.Fatalf("угол кадра не закрашен: %v", c)
	}

	// Событие в физических пикселях приходит виджету в логических.
	clicked := false
	btn := widget.NewButton("x")
	btn.SetBounds(image.Rect(1200, 650, 1280, 720))
	btn.OnClick = func() { clicked = true }
	root.AddChild(btn)
	eng.SendMouseButton(1860, 1040, widget.MouseLeft, true) // 1240,693 логических
	eng.SendMouseButton(1860, 1040, widget.MouseLeft, false)
	if !clicked {
		t.Fatal("щелчок в физических пикселях не дошёл до кнопки в логических")
	}

	// Текст крупнее: та же метка занимает в кадре в полтора раза больше
	// физических столбцов. MeasureText тут не годится — он логический и от
	// масштаба не зависит по определению.
	w1 := inkWidth(t, eng, root)
	eng.SetScaleWithin(1)
	w0 := inkWidth(t, eng, root)
	if w1 < w0*14/10 {
		t.Fatalf("текст при 1,5 не крупнее: %d против %d физических столбцов", w1, w0)
	}
}

// inkWidth рисует метку в углу корня и считает физические столбцы, где есть
// её чернила.
func inkWidth(t *testing.T, eng *Engine, root *widget.Panel) int {
	t.Helper()
	lbl := widget.NewLabel("Масштаб", color.RGBA{R: 255, G: 255, B: 255, A: 255})
	lbl.SetBounds(image.Rect(0, 0, 300, 40))
	root.AddChild(lbl)
	defer root.RemoveChild(lbl)
	img := eng.RenderOnce()
	cols := 0
	for x := 0; x < img.Rect.Dx() && x < 600; x++ {
		for y := 0; y < 80 && y < img.Rect.Dy(); y++ {
			if c := img.RGBAAt(x, y); c.R > 200 && c.G > 200 {
				cols++
				break
			}
		}
	}
	if cols == 0 {
		t.Fatal("метка не нарисована")
	}
	return cols
}

// SetScale в режиме держит физический размер; SetPhysicalResolution меняет
// кадр, не трогая масштаб; SetResolution выходит из режима.
func TestScaleWithin_ModeTransitions(t *testing.T) {
	testreset.Globals(t)
	eng := New(1366, 768, 20)
	defer eng.Stop()
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 1366, 768))
	eng.SetRoot(root)

	eng.SetScaleWithin(1.25)
	eng.SetScale(2) // в режиме — как SetScaleWithin
	if pw, ph := eng.PhysicalSize(); pw != 1366 || ph != 768 {
		t.Fatalf("SetScale в режиме сменил физический размер: %dx%d", pw, ph)
	}
	if lw, lh := eng.CanvasSize(); lw != 683 || lh != 384 || eng.Scale() != 2 {
		t.Fatalf("после SetScale(2): %dx%d, k=%v", lw, lh, eng.Scale())
	}

	eng.SetPhysicalResolution(1920, 1080)
	if pw, ph := eng.PhysicalSize(); pw != 1920 || ph != 1080 || eng.Scale() != 2 {
		t.Fatalf("SetPhysicalResolution: %dx%d, k=%v", pw, ph, eng.Scale())
	}
	if lw, lh := eng.CanvasSize(); lw != 960 || lh != 540 || root.Bounds().Max != image.Pt(960, 540) {
		t.Fatalf("после SetPhysicalResolution логический %dx%d, корень %v", lw, lh, root.Bounds())
	}

	eng.SetResolution(800, 600) // логический: выход из режима
	if eng.ScaleWithin() {
		t.Fatal("SetResolution не вышел из режима")
	}
	if pw, ph := eng.PhysicalSize(); pw != 1600 || ph != 1200 {
		t.Fatalf("после SetResolution при k=2 физический %dx%d, ждали 1600x1200", pw, ph)
	}
	eng.SetScale(1) // обычный путь: физический = логический
	if pw, ph := eng.PhysicalSize(); pw != 800 || ph != 600 {
		t.Fatalf("после SetScale(1) физический %dx%d", pw, ph)
	}
}

// Буферы SetSurfaces в режиме задаются под физический размер и работают.
func TestScaleWithin_SurfacesPhysical(t *testing.T) {
	testreset.Globals(t)
	eng := New(640, 480, 20)
	defer eng.Stop()
	root := widget.NewPanel(color.RGBA{R: 90, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 640, 480))
	eng.SetRoot(root)
	eng.SetScaleWithin(2)
	bufs := [][]byte{make([]byte, 640*480*4), make([]byte, 640*480*4)}
	if err := eng.SetSurfaces(bufs, 640*4, FormatRGBA); err != nil {
		t.Fatal(err)
	}
	fr := eng.RenderFrameNow()
	if fr.Surface != 1 || fr.Width != 640 || fr.Height != 480 {
		t.Fatalf("кадр: Surface %d, %dx%d", fr.Surface, fr.Width, fr.Height)
	}
	if bufs[0][(479*640+639)*4] != 90 {
		t.Fatal("угол буфера не закрашен корнем в логических 320x240")
	}
}
