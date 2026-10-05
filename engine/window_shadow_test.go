package engine

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

func windowEngine(t *testing.T, bg bool) (*Engine, *widget.Window, *widget.Canvas) {
	t.Helper()
	e := New(400, 300, 30)
	e.SetRenderOnDemand(true)
	if bg {
		if err := e.SetBackground(stripes(400, 300)); err != nil {
			t.Fatal(err)
		}
	}
	root := widget.NewCanvas()
	root.SetBounds(image.Rect(0, 0, 400, 300))
	w := widget.NewWindow("W", 160, 120)
	w.MainWindow = false
	w.SetBounds(image.Rect(100, 80, 260, 200))
	root.AddChild(w)
	e.SetRoot(root)
	return e, w, root
}

func softShadow() theme.ShadowSpec {
	return theme.ShadowSpec{Blur: 20, OffsetY: 8, Color: theme.RGBA(0, 0, 0, 160)}
}

// По умолчанию окно без тени: кадр с токенами по умолчанию побайтно такой же,
// как раньше, и запаса под тень нет.
func TestWindowShadow_OffByDefault(t *testing.T) {
	e, w, _ := windowEngine(t, true)
	if w.DrawMargin() != 0 {
		t.Errorf("DrawMargin окна без тени = %d", w.DrawMargin())
	}
	plain := bytes.Clone(e.RenderOnce().Pix)

	e2, w2, _ := windowEngine(t, true)
	w2.SetShadow(theme.ShadowSpec{}) // явное «пусто» не меняет кадра
	if !bytes.Equal(plain, bytes.Clone(e2.RenderOnce().Pix)) {
		t.Fatal("пустая тень изменила кадр")
	}
	_ = e
}

// Тень окна ложится за его границами: пиксели вокруг темнее обоев, внутри окна
// — как было; DrawMargin сообщает запас.
func TestWindowShadow_DrawnOutsideBounds(t *testing.T) {
	e0, _, _ := windowEngine(t, true)
	base := e0.RenderOnce().RGBAAt(180, 215) // под окном, в зоне тени

	e, w, _ := windowEngine(t, true)
	w.SetShadow(softShadow())
	if got := w.DrawMargin(); got < 48 {
		t.Errorf("DrawMargin = %d, ждали не меньше 2×размытие+смещение", got)
	}
	img := e.RenderOnce()
	lit := img.RGBAAt(180, 215)
	if int(lit.R)+int(lit.G)+int(lit.B) >= int(base.R)+int(base.G)+int(base.B) {
		t.Errorf("тень не затемнила пиксель под окном: %v против %v", lit, base)
	}
	// Далеко от окна тени нет.
	if far, ref := img.RGBAAt(10, 10), e0.RenderOnce().RGBAAt(10, 10); far != ref {
		t.Errorf("тень достала до далёкой точки: %v против %v", far, ref)
	}
}

// Окно с тенью, сдвинутое частичной перерисовкой, оставляет тот же кадр, что и
// полная отрисовка на новом месте: кольцо тени вокруг старого положения убрано.
func TestWindowShadow_MoveLeavesNoGhost(t *testing.T) {
	moved := image.Rect(180, 120, 340, 240)

	e, w, _ := windowEngine(t, true)
	w.SetShadow(softShadow())
	e.RenderOnce()
	w.SetBounds(moved)
	got := bytes.Clone(e.RenderOnce().Pix)

	ref, wr, _ := windowEngine(t, true)
	wr.SetShadow(softShadow())
	wr.SetBounds(moved)
	ref.Invalidate()
	want := bytes.Clone(ref.RenderOnce().Pix)

	if !bytes.Equal(got, want) {
		diff := 0
		for i := range got {
			if got[i] != want[i] {
				diff++
			}
		}
		t.Fatalf("после перемещения окна с тенью остался след: %d байт расходятся с полным кадром", diff)
	}
}

// Включение и снятие тени перерисовывает кольцо вокруг окна.
func TestWindowShadow_ToggleRedrawsRing(t *testing.T) {
	e, w, _ := windowEngine(t, true)
	plain := bytes.Clone(e.RenderOnce().Pix)
	w.SetShadow(softShadow())
	with := bytes.Clone(e.RenderOnce().Pix)
	if bytes.Equal(plain, with) {
		t.Fatal("тень не появилась после SetShadow")
	}
	w.SetShadow(theme.ShadowSpec{})
	back := bytes.Clone(e.RenderOnce().Pix)
	if !bytes.Equal(plain, back) {
		t.Fatal("после снятия тени кадр не вернулся к исходному")
	}
}

// Материал Mica у окна: фон клиентской области — размытые обои, а не
// Background; без обоев — сплошной Fallback.
func TestWindowBackdrop_Mica(t *testing.T) {
	spec := theme.BackdropSpec{Mode: theme.BackdropBlur, Material: theme.MaterialMica, Radius: 60,
		Tint: theme.RGBA(240, 240, 240, 120), Fallback: theme.RGB(243, 243, 243)}

	e, w, _ := windowEngine(t, true)
	plain := e.RenderOnce().RGBAAt(200, 170)
	w.SetBackdrop(spec)
	got := e.RenderOnce().RGBAAt(200, 170)
	if got == plain {
		t.Error("материал не изменил фон окна")
	}
	if w.OpaqueRegion() != nil {
		t.Error("окно с материалом не должно объявлять себя непрозрачным")
	}

	e2, w2, _ := windowEngine(t, false)
	w2.SetBackdrop(spec)
	if px := e2.RenderOnce().RGBAAt(200, 170); px != (color.RGBA{243, 243, 243, 255}) {
		t.Errorf("Mica окна без обоев = %v, ждали Fallback", px)
	}
}

// Тень диалога по токенам: мягкая тень вместо полос, вид без токенов прежний.
func TestDialogShadow_Tokens(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	render := func(tok bool) *image.RGBA {
		m.SetFlag(theme.FlagShadowSoft, tok)
		e := New(400, 300, 30)
		if err := e.ApplyThemeProfile(m); err != nil {
			t.Fatal(err)
		}
		defer func() {
			m.SetFlag(theme.FlagShadowSoft, false)
			_ = e.ApplyThemeProfile(m) // глобальная палитра вернулась к прежней
		}()
		if got := widget.CurrentThemeStyle().DialogShadow.IsZero(); got == tok {
			t.Fatalf("токены тени диалога: задан=%v, ждали %v", !got, tok)
		}

		e.SetRenderOnDemand(true)
		// Светлый фон: на пустом (чёрном) холсте тень неразличима, а
		// затемнение диалога теперь полупрозрачное и фон не закрывает.
		bg := image.NewRGBA(image.Rect(0, 0, 400, 300))
		for i := range bg.Pix {
			bg.Pix[i] = 255
		}
		if err := e.SetBackground(bg); err != nil {
			t.Fatal(err)
		}
		root := widget.NewCanvas()
		root.SetBounds(image.Rect(0, 0, 400, 300))
		e.SetRoot(root)
		d := widget.NewDialog("T", 160, 100)
		d.Dim = color.RGBA{}
		e.ShowModal(d)
		return e.RenderOnce()
	}
	legacy, soft := render(false), render(true)
	// Точка в зоне тени ниже диалога (он по центру: y 100..220).
	lp, sp := legacy.RGBAAt(200, 240), soft.RGBAAt(200, 240)
	if sp == lp {
		t.Errorf("токены тени не изменили вид диалога: %v", sp)
	}
}

// Mica при масштабе 2: слой рисуется в физических пикселях без паники и без
// провалов в размытии.
func TestMica_Scale2(t *testing.T) {
	e, _ := newMicaEngine(t, true)
	e.SetScale(2)
	img := e.RenderOnce()
	if img.Bounds().Dx() != 800 {
		t.Fatalf("ширина кадра %d", img.Bounds().Dx())
	}
	a, b := img.RGBAAt(2*60, 2*150), img.RGBAAt(2*330, 2*150)
	if a == b {
		t.Error("при масштабе 2 слой Mica не получил обои")
	}
}
