package engine

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// micaPanel — панель с материалом Mica (или любым другим) и квадратиком, цвет
// которого меняется, как подсветка кнопки.
type micaPanel struct {
	widget.Base
	spec theme.BackdropSpec
	lit  bool
}

func (m *micaPanel) Draw(ctx widget.DrawContext) {
	b := m.Bounds()
	widget.PaintMaterial(ctx, b, 0, m.spec)
	c := color.RGBA{200, 200, 200, 255}
	if m.lit {
		c = color.RGBA{255, 120, 0, 255}
	}
	ctx.FillRect(b.Min.X+100, b.Min.Y+10, 20, 20, c)
}

func micaSpec() theme.BackdropSpec {
	return theme.BackdropSpec{
		Mode: theme.BackdropBlur, Material: theme.MaterialMica, Radius: 40,
		Tint: theme.RGBA(240, 240, 240, 120), Fallback: theme.RGB(243, 243, 243),
	}
}

func newMicaEngine(t *testing.T, bg bool) (*Engine, *micaPanel) {
	t.Helper()
	e := New(400, 200, 30)
	e.SetRenderOnDemand(true)
	if bg {
		if err := e.SetBackground(stripes(400, 200)); err != nil {
			t.Fatal(err)
		}
	}
	p := &micaPanel{spec: micaSpec()}
	p.SetBounds(image.Rect(40, 60, 360, 190))
	root := widget.NewCanvas()
	root.SetBounds(image.Rect(0, 0, 400, 200))
	root.AddChild(p)
	e.SetRoot(root)
	return e, p
}

// Mica размывает ОБОИ: рисунок обоев под слоем сглажен (разброс значений
// резко меньше, чем у самих обоев), а цвет смещён к подкраске.
func TestMica_BlursWallpaper(t *testing.T) {
	e, p := newMicaEngine(t, true)
	img := e.RenderOnce()

	spread := func(r image.Rectangle, ch int) int {
		lo, hi := 255, 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				v := int(img.Pix[img.PixOffset(x, y)+ch])
				lo, hi = min(lo, v), max(hi, v)
			}
		}
		return hi - lo
	}
	area := image.Rect(150, 90, 330, 180) // без квадратика подсветки
	if s := spread(area, 0); s > 90 {
		t.Errorf("под Mica разброс красного %d — обои не размыты (у обоев ≈ 255)", s)
	}
	// Но это не сплошной цвет: плавный перепад обоев (красный растёт слева
	// направо) сохраняется.
	grad := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			grad.SetRGBA(x, y, color.RGBA{uint8(x * 255 / 400), 90, 90, 255})
		}
	}
	if err := e.SetBackground(grad); err != nil {
		t.Fatal(err)
	}
	img = e.RenderOnce()
	l, r := img.RGBAAt(60, 150).R, img.RGBAAt(340, 150).R
	if int(r)-int(l) < 40 {
		t.Errorf("слой Mica не следует за обоями: %d слева, %d справа", l, r)
	}
	// Вне панели обои нетронуты: полоса красного от 0 до 255.
	if s := spread(image.Rect(0, 0, 400, 50), 0); s < 200 {
		t.Errorf("обои вне панели испорчены, разброс %d", s)
	}
	_ = p
}

// Частичная перерисовка внутри Mica даёт те же пиксели, что полный кадр —
// слой не зависит от прошлого кадра и от повреждённой области.
func TestMica_PartialRedrawMatchesFull(t *testing.T) {
	render := func(partial bool) []byte {
		e, p := newMicaEngine(t, true)
		e.RenderOnce()
		p.lit = true
		if partial {
			e.InvalidateRect(image.Rect(130, 64, 190, 100))
		} else {
			e.Invalidate()
		}
		return bytes.Clone(e.RenderOnce().Pix)
	}
	full, part := render(false), render(true)
	if !bytes.Equal(full, part) {
		diff := 0
		for i := range full {
			if full[i] != part[i] {
				diff++
			}
		}
		t.Fatalf("частичная перерисовка Mica расходится с полной: %d байт", diff)
	}
}

// Mica детерминирована: два кадра подряд побайтно равны (никакого размытия
// поверх размытия).
func TestMica_StableAcrossFrames(t *testing.T) {
	e, p := newMicaEngine(t, true)
	a := bytes.Clone(e.RenderOnce().Pix)
	p.Invalidate()
	e.Invalidate()
	b := bytes.Clone(e.RenderOnce().Pix)
	if !bytes.Equal(a, b) {
		t.Fatal("повторный кадр Mica отличается от первого")
	}
}

// Без обоев контекст не может нарисовать Mica: MicaBehind возвращает false, а
// слой рисуется сплошным цветом темы (Fallback).
func TestMica_NoWallpaperFallsBackToSolid(t *testing.T) {
	e, _ := newMicaEngine(t, false)
	if e.canvas.MicaBehind(image.Rect(0, 0, 10, 10), 20, color.RGBA{}) {
		t.Error("MicaBehind без обоев вернула true")
	}
	img := e.RenderOnce()
	if got := img.RGBAAt(250, 150); got != (color.RGBA{243, 243, 243, 255}) {
		t.Errorf("без обоев слой Mica = %v, ждали Fallback 243", got)
	}
}

// Отдельный источник обоев главнее фона и переживает смену разрешения.
func TestMica_WallpaperSourceOverridesBackground(t *testing.T) {
	e, _ := newMicaEngine(t, true)
	red := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for i := 0; i < len(red.Pix); i += 4 {
		red.Pix[i], red.Pix[i+3] = 255, 255
	}
	if err := e.SetWallpaperSource(red); err != nil {
		t.Fatal(err)
	}
	img := e.RenderOnce()
	got := img.RGBAAt(250, 150)
	// Красные обои и светлая подкраска: красный канал заметно выше зелёного.
	if int(got.R)-int(got.G) < 40 {
		t.Errorf("Mica по красному источнику = %v — источник не применён", got)
	}
	// Вне панели остался фон движка, а не красный источник.
	if bg := img.RGBAAt(5, 5); int(bg.R)-int(bg.G) > 40 {
		t.Errorf("фон движка заменён источником Mica: %v", bg)
	}
	// Снятие источника возвращает Mica к фону.
	if err := e.SetWallpaperSource(nil); err != nil {
		t.Fatal(err)
	}
	img = e.RenderOnce()
	if got2 := img.RGBAAt(250, 150); int(got2.R)-int(got2.G) > 30 {
		t.Errorf("после снятия источника Mica всё ещё красная: %v", got2)
	}
	if err := e.SetWallpaperSource(image.NewRGBA(image.Rect(0, 0, 0, 0))); err == nil {
		t.Error("пустой источник должен давать ошибку")
	}
}

// Подкраска темнит: Mica с тёмной подкраской темнее светлой на тех же обоях.
func TestMica_TintDarkens(t *testing.T) {
	lum := func(spec theme.BackdropSpec) int {
		e, p := newMicaEngine(t, true)
		p.spec = spec
		img := e.RenderOnce()
		c := img.RGBAAt(250, 150)
		return int(c.R) + int(c.G) + int(c.B)
	}
	mica := micaSpec()
	alt := mica
	alt.Material = theme.MaterialMicaAlt
	alt.Tint = theme.RGBA(14, 14, 14, 220)
	if lum(alt) >= lum(mica) {
		t.Error("MicaAlt с тёмной подкраской не темнее Mica")
	}
}

// Буфер всплывающего оверлея берёт обои у основного холста: тот же пиксель
// Mica и в окне, и в попапе (со сдвигом).
func TestMica_PopupCanvasUsesParentWallpaper(t *testing.T) {
	e, _ := newMicaEngine(t, true)
	r := image.Rect(150, 100, 250, 160)
	oc := e.canvas.cloneForSize(r.Dx(), r.Dy(), e.canvas.scale, nil)
	oc.wallParent = e.canvas.wallOwner()
	tc := &translatingContext{inner: oc, dx: r.Min.X, dy: r.Min.Y}
	if !tc.MicaBehind(r, 40, color.RGBA{}) {
		t.Fatal("попап не получил обои основного холста")
	}
	// Тот же слой прямо на основном холсте.
	e2, _ := newMicaEngine(t, true)
	direct := e2.canvas
	direct.MicaBehind(r, 40, color.RGBA{})
	for _, pt := range []image.Point{{0, 0}, {50, 30}, {99, 59}} {
		a := oc.back.RGBAAt(pt.X, pt.Y)
		b := direct.back.RGBAAt(r.Min.X+pt.X, r.Min.Y+pt.Y)
		if a != b {
			t.Errorf("пиксель %v: попап %v, основной холст %v", pt, a, b)
		}
	}
}

// DrawShadow с параметрами DrawSoftShadow побайтно совпадает с ним: прежние
// тени не изменились.
func TestDrawShadow_LegacyIdentical(t *testing.T) {
	draw := func(f func(c *Canvas)) []byte {
		c := newCanvasScaled(160, 120, 1, nil)
		c.FillRect(0, 0, 160, 120, color.RGBA{230, 230, 230, 255})
		f(c)
		return bytes.Clone(c.back.Pix)
	}
	r := image.Rect(40, 30, 120, 80)
	col := color.RGBA{0, 0, 0, 70}
	a := draw(func(c *Canvas) { c.DrawSoftShadow(r, 8, 12, col) })
	b := draw(func(c *Canvas) { c.DrawShadow(r, 8, 12, 0, 6, col) })
	if !bytes.Equal(a, b) {
		t.Fatal("DrawSoftShadow и DrawShadow(blur=h, dy=h/2) расходятся")
	}
}

// Смещение по X двигает тень вбок, непрозрачность (альфа цвета) ослабляет.
func TestDrawShadow_OffsetAndOpacity(t *testing.T) {
	shadowAt := func(ox, oy float64, a uint8, x, y int) uint8 {
		c := newCanvasScaled(200, 160, 1, nil)
		c.FillRect(0, 0, 200, 160, color.RGBA{255, 255, 255, 255})
		c.DrawShadow(image.Rect(60, 50, 140, 100), 6, 10, ox, oy, color.RGBA{0, 0, 0, a})
		return c.back.RGBAAt(x, y).R
	}
	// Точка справа от окна: со сдвигом вправо темнее, чем со сдвигом влево.
	right := shadowAt(12, 0, 255, 148, 75)
	left := shadowAt(-12, 0, 255, 148, 75)
	if right >= left {
		t.Errorf("сдвиг вправо не затемнил точку справа: %d против %d", right, left)
	}
	// Точка над окном: сдвиг вниз её светлит.
	down := shadowAt(0, 14, 255, 100, 42)
	none := shadowAt(0, 0, 255, 100, 42)
	if down <= none {
		t.Errorf("сдвиг вниз не высветлил точку над окном: %d против %d", down, none)
	}
	// Меньшая непрозрачность — светлее.
	strong := shadowAt(0, 6, 255, 100, 106)
	weak := shadowAt(0, 6, 90, 100, 106)
	if weak <= strong {
		t.Errorf("слабая тень %d не светлее сильной %d", weak, strong)
	}
	// Тени нет за пределами запаса.
	if far := shadowAt(0, 6, 255, 5, 5); far != 255 {
		t.Errorf("тень достала до далёкой точки: %d", far)
	}
}
