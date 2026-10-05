package widget

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// matRec — контекст, записывающий, что слой попросил у рисования.
type matRec struct {
	DrawContext
	fills  []color.RGBA
	blur   int
	noise  int
	mica   int
	micaOK bool
	shadow []string
}

func (c *matRec) FillRect(x, y, w, h int, col color.RGBA)      { c.fills = append(c.fills, col) }
func (c *matRec) FillRectAlpha(x, y, w, h int, col color.RGBA) { c.fills = append(c.fills, col) }
func (c *matRec) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.fills = append(c.fills, col)
}

// Три разновидности контекста: только блюр, плюс Mica, плюс параметры тени.
type blurCtx struct{ *matRec }

func (c blurCtx) BlurBehind(r image.Rectangle, radius int, tint color.RGBA) { c.blur++ }
func (c blurCtx) DrawNoise(r image.Rectangle, amount float64)               { c.noise++ }

type micaCtx struct{ *matRec }

func (c micaCtx) MicaBehind(r image.Rectangle, radius int, tint color.RGBA) bool {
	c.mica++
	return c.micaOK
}

type shadowCtx struct{ *matRec }

func (c shadowCtx) DrawShadow(r image.Rectangle, corner int, blur, ox, oy float64, col color.RGBA) {
	c.shadow = append(c.shadow, "param")
}
func (c shadowCtx) DrawSoftShadow(r image.Rectangle, corner int, elevation float64, col color.RGBA) {
	c.shadow = append(c.shadow, "soft")
}

type softOnlyCtx struct {
	*matRec
	r image.Rectangle
	e float64
}

func (c *softOnlyCtx) DrawSoftShadow(r image.Rectangle, corner int, elevation float64, col color.RGBA) {
	c.r, c.e = r, elevation
}

var testRect = image.Rect(10, 20, 110, 120)

func micaSpec() theme.BackdropSpec {
	return theme.BackdropSpec{
		Mode: theme.BackdropBlur, Material: theme.MaterialMica, Radius: 40,
		Tint: theme.RGBA(240, 240, 240, 120), Fallback: theme.RGB(243, 243, 243),
	}
}

func TestPaintMaterial_DefaultIsNotHandled(t *testing.T) {
	rec := &matRec{}
	if PaintMaterial(rec, testRect, 0, theme.BackdropSpec{Mode: theme.BackdropBlur, Tint: theme.RGBA(1, 1, 1, 50)}) {
		t.Error("материал не назван, а PaintMaterial взялся рисовать")
	}
	if len(rec.fills) != 0 {
		t.Error("PaintMaterial нарисовал слой без материала")
	}
}

func TestPaintMaterial_Solid(t *testing.T) {
	rec := &matRec{}
	b := micaSpec()
	b.Material = theme.MaterialSolid
	if !PaintMaterial(rec, testRect, 0, b) {
		t.Fatal("Solid не обработан")
	}
	if len(rec.fills) != 1 || rec.fills[0] != theme.RGB(243, 243, 243) {
		t.Errorf("Solid должен лечь сплошным Fallback: %v", rec.fills)
	}
	// Без Fallback — подкраска.
	rec = &matRec{}
	b.Fallback = color.RGBA{}
	PaintMaterial(rec, testRect, 0, b)
	if len(rec.fills) != 1 || rec.fills[0] != b.Tint {
		t.Errorf("Solid без Fallback: %v", rec.fills)
	}
}

func TestPaintMaterial_Acrylic(t *testing.T) {
	rec := &matRec{}
	b := theme.BackdropSpec{Material: theme.MaterialAcrylic, Radius: 20, Tint: theme.RGBA(30, 30, 30, 200),
		Noise: 0.02, Fallback: theme.RGB(31, 31, 31)}
	// Контекст без блюра — сплошной цвет темы.
	PaintMaterial(rec, testRect, 0, b)
	if len(rec.fills) != 1 || rec.fills[0] != theme.RGB(31, 31, 31) {
		t.Errorf("Acrylic без размытия: %v", rec.fills)
	}
	rec = &matRec{}
	PaintMaterial(blurCtx{rec}, testRect, 0, b)
	if rec.blur != 1 || rec.noise != 1 || len(rec.fills) != 0 {
		t.Errorf("Acrylic с размытием: blur=%d noise=%d fills=%v", rec.blur, rec.noise, rec.fills)
	}
}

func TestPaintMaterial_Mica(t *testing.T) {
	// Контекст умеет Mica и обои есть — Fallback не нужен.
	rec := &matRec{micaOK: true}
	mc := micaCtx{rec}
	if !PaintMaterial(mc, testRect, 0, micaSpec()) {
		t.Fatal("Mica не обработана")
	}
	if len(rec.fills) != 0 {
		t.Errorf("Mica с обоями дорисовала сплошной цвет: %v", rec.fills)
	}
	// Обоев нет (MicaBehind вернул false) — сплошной цвет темы.
	rec = &matRec{micaOK: false}
	PaintMaterial(micaCtx{rec}, testRect, 0, micaSpec())
	if len(rec.fills) != 1 || rec.fills[0] != theme.RGB(243, 243, 243) {
		t.Errorf("Mica без обоев: %v", rec.fills)
	}
	// Контекст вовсе не умеет Mica — то же самое, MicaAlt так же.
	rec = &matRec{}
	b := micaSpec()
	b.Material = theme.MaterialMicaAlt
	PaintMaterial(rec, testRect, 0, b)
	if len(rec.fills) != 1 || rec.fills[0] != theme.RGB(243, 243, 243) {
		t.Errorf("MicaAlt в контексте без Mica: %v", rec.fills)
	}
}

func TestDrawShadowSpec(t *testing.T) {
	sp := theme.ShadowSpec{Blur: 20, OffsetX: 3, OffsetY: 8, Color: theme.RGBA(0, 0, 0, 80)}

	rec := &matRec{}
	DrawShadowSpec(shadowCtx{rec}, testRect, 8, sp)
	if len(rec.shadow) != 1 || rec.shadow[0] != "param" {
		t.Errorf("контекст с параметрами тени должен получить DrawShadow: %v", rec.shadow)
	}

	// Старый контекст: смещение пересчитывается так, чтобы тень легла туда же
	// (DrawSoftShadow сам сдвигает вниз на blur/2).
	old := &softOnlyCtx{matRec: &matRec{}}
	DrawShadowSpec(old, testRect, 8, sp)
	if old.e != 20 {
		t.Errorf("высота для DrawSoftShadow = %v", old.e)
	}
	if old.r != testRect.Add(image.Pt(3, 8-10)) {
		t.Errorf("сдвинутая область %v, ждали %v", old.r, testRect.Add(image.Pt(3, -2)))
	}

	// Пустая тень и пустая область ничего не рисуют.
	rec = &matRec{}
	DrawShadowSpec(shadowCtx{rec}, testRect, 8, theme.ShadowSpec{})
	DrawShadowSpec(shadowCtx{rec}, image.Rectangle{}, 8, sp)
	if len(rec.shadow) != 0 {
		t.Errorf("пустая тень нарисована: %v", rec.shadow)
	}
}

// Materialize переносит токены тени стилей menu/window/dialog в ThemeStyle
// только когда профиль их объявил.
func TestMaterialize_ShadowTokens(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	st := Materialize(m.Active()).Style
	if !st.MenuShadow.IsZero() || !st.WindowShadow.IsZero() || !st.DialogShadow.IsZero() {
		t.Fatalf("без флага токены тени не должны попадать в ThemeStyle: %+v", st)
	}

	m.SetFlag(theme.FlagShadowSoft, true)
	st = Materialize(m.Active()).Style
	for name, sp := range map[string]theme.ShadowSpec{"menu": st.MenuShadow, "window": st.WindowShadow, "dialog": st.DialogShadow} {
		if sp.IsZero() || sp.Blur < 20 {
			t.Errorf("%s: тень %+v", name, sp)
		}
	}
	// Тени окна и диалога крупнее тени меню.
	if st.WindowShadow.Blur <= st.MenuShadow.Blur {
		t.Errorf("тень окна %v не крупнее тени меню %v", st.WindowShadow.Blur, st.MenuShadow.Blur)
	}
}

func TestMenuShadow_FollowsCurrentStyle(t *testing.T) {
	prev := win10.Style
	defer func() { win10.Style = prev }()
	win10.Style.MenuShadow = theme.ShadowSpec{}
	if _, ok := MenuShadow(); ok {
		t.Fatal("тень меню без токенов должна быть не задана")
	}
	win10.Style.MenuShadow = theme.ShadowSpec{Blur: 16, OffsetY: 6, Color: theme.RGBA(0, 0, 0, 70)}
	sp, ok := MenuShadow()
	if !ok || sp.Blur != 16 {
		t.Errorf("MenuShadow = %+v ok=%v", sp, ok)
	}
}

func TestMotionDur(t *testing.T) {
	defer SetReduceMotion(false)
	SetReduceMotion(false)
	if d := MotionDur(120 * time.Millisecond); d != 120*time.Millisecond {
		t.Errorf("без режима длительность изменилась: %v", d)
	}
	SetReduceMotion(true)
	if !ReduceMotion() {
		t.Fatal("режим не включился")
	}
	if d := MotionDur(120 * time.Millisecond); d != 0 {
		t.Errorf("при «меньше движения» длительность %v", d)
	}
	// Переключатель и затухание диалога идут через MotionDur: ручка встаёт на
	// место за первый шаг, а не за 120 мс.
	knobDur := func(ts *ToggleSwitch) (time.Duration, bool) {
		anim.mu.Lock()
		defer anim.mu.Unlock()
		for _, a := range anim.active {
			if a.owner == ts && a.tag == "knob" && !a.stopped {
				return a.duration, true
			}
		}
		return 0, false
	}
	ts := NewToggleSwitch("x")
	ts.SetBounds(image.Rect(0, 0, 80, 24))
	ts.SetOn(true)
	if d, ok := knobDur(ts); !ok || d != 0 {
		t.Errorf("ручка при «меньше движения»: длительность %v, анимация есть=%v", d, ok)
	}
	SetReduceMotion(false)
	ts2 := NewToggleSwitch("y")
	ts2.SetBounds(image.Rect(0, 0, 80, 24))
	ts2.SetOn(true)
	if d, ok := knobDur(ts2); !ok || d != toggleAnimDur {
		t.Errorf("ручка без режима: длительность %v, анимация есть=%v", d, ok)
	}
}
