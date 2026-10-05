package desktop_test

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// win10Manager — менеджер со встроенными темами и активной темой name.
func win10Manager(t *testing.T, name string) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	m.SetIconResolver(widget.BuiltinIcons())
	return m
}

// countColor считает пиксели точного цвета c в области r кадра.
func countColor(img *image.RGBA, r image.Rectangle, c color.RGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y) == c {
				n++
			}
		}
	}
	return n
}

// TestSetAccent_RepaintsShellWithoutRecreating — смена акцента на менеджере
// перекрашивает уже собранную панель: те же компоненты, тот же движок, новый
// кадр. Полоса под активной кнопкой окна — акцентная.
func TestSetAccent_RepaintsShellWithoutRecreating(t *testing.T) {
	const w, h = 640, 220
	m := win10Manager(t, theme.ProfileWindows10)
	eng := engine.New(w, h, 30)
	root, bar := buildScene(t, m, w, h)
	defer bar.Close()
	eng.SetRoot(root)
	area := bar.Bounds()

	blue := theme.RGB(0, 120, 215)
	first := snapshot(eng.RenderOnce())
	if n := countColor(first, area, blue); n == 0 {
		t.Fatal("на панели нет акцентного цвета — проверять нечего")
	}

	red := theme.RGB(210, 40, 40)
	m.SetAccent(red) // без пересоздания панели и без SetTheme
	second := snapshot(eng.RenderOnce())

	if n := countColor(second, area, blue); n != 0 {
		t.Errorf("после смены акцента на панели осталось %d пикселей старого цвета", n)
	}
	if n := countColor(second, area, red); n == 0 {
		t.Error("новый акцент на панели не появился: подписчики не перерисовались")
	}
	// Кнопка «Пуск» и остальное то же: панель не пересобиралась.
	if bar.Bounds() != area {
		t.Error("панель сменила границы")
	}

	m.ResetAccent()
	third := snapshot(eng.RenderOnce())
	if !imagesEqual(first, third) {
		t.Error("после ResetAccent кадр не вернулся к исходному")
	}
}

// TestEngineSetAccent_ReappliesProfile — Engine.SetAccent меняет акцент и
// перекрашивает виджеты со своей палитрой (widget.Theme.Accent).
func TestEngineSetAccent_ReappliesProfile(t *testing.T) {
	m := win10Manager(t, theme.ProfileWindows10)
	eng := engine.New(200, 100, 30)
	// Палитра виджетов глобальна на процесс: возвращаем её, чтобы тест не
	// влиял на соседей по пакету.
	prev := widget.CurrentTheme()
	defer eng.SetTheme(prev)
	if err := eng.SetThemeProfile(m, theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	cb := widget.NewCheckBox("OK")
	cb.SetChecked(true)
	cb.SetBounds(image.Rect(10, 10, 110, 40))
	root := widget.NewPanel(color.RGBA{R: 240, G: 240, B: 240, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 100))
	root.AddChild(cb)
	eng.SetRoot(root)

	orange := theme.RGB(255, 140, 0)
	if err := eng.SetAccent(m, orange); err != nil {
		t.Fatal(err)
	}
	if got := widget.CurrentTheme().Accent; got != orange {
		t.Errorf("акцент виджетной темы = %v", got)
	}
	img := snapshot(eng.RenderOnce())
	if countColor(img, cb.Bounds(), orange) == 0 {
		t.Error("отмеченный флажок не стал оранжевым")
	}
	if err := eng.SetAccent(nil, orange); err == nil {
		t.Error("SetAccent без менеджера не вернул ошибку")
	}
	if err := eng.ApplyThemeProfile(theme.NewManager()); err == nil {
		t.Error("ApplyThemeProfile без активной темы не вернул ошибку")
	}
}

// probe — виджет, отдающий свой контекст рисования тесту.
type probe struct {
	widget.Base
	draw func(ctx widget.DrawContext)
}

func (p *probe) Draw(ctx widget.DrawContext) { p.draw(ctx) }

// plainContext скрывает необязательные возможности холста (размытие, шум,
// тени, скругление): остаётся только DrawContext, как у контекста без
// BackdropDrawer.
type plainContext struct{ widget.DrawContext }

// paintOnCheckers рисует стиль s в прямоугольнике r поверх клетчатых обоев и
// возвращает кадр. wrap оборачивает контекст перед рисованием.
func paintOnCheckers(t *testing.T, s *theme.Style, r image.Rectangle, wrap func(widget.DrawContext) widget.DrawContext) *image.RGBA {
	t.Helper()
	const w, h = 400, 120
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	pr := &probe{draw: func(ctx widget.DrawContext) {
		for y := 0; y < h; y += 12 {
			for x := 0; x < w; x += 12 {
				c := color.RGBA{R: 220, G: 80, B: 60, A: 255}
				if (x/12+y/12)%2 == 0 {
					c = color.RGBA{R: 40, G: 90, B: 200, A: 255}
				}
				ctx.FillRect(x, y, 12, 12, c)
			}
		}
		if wrap != nil {
			ctx = wrap(ctx)
		}
		desktop.PaintStyle(ctx, r, s)
	}}
	pr.SetBounds(image.Rect(0, 0, w, h))
	root.AddChild(pr)
	eng := engine.New(w, h, 30)
	eng.SetRoot(root)
	return snapshot(eng.RenderOnce())
}

// TestPaintStyle_Windows10Acrylic — на холсте с размытием панель затемняет и
// размывает обои (клетки исчезают), поверх лежит зерно; в соседней точке
// значения различаются.
func TestPaintStyle_Windows10Acrylic(t *testing.T) {
	m := win10Manager(t, theme.ProfileWindows10)
	s := m.GetStyle("taskbar", "", theme.StateNormal)
	r := image.Rect(20, 40, 380, 80)

	img := paintOnCheckers(t, s, r, nil)

	// Клетчатость сглажена: в области нет крупных скачков между соседними
	// пикселями, кроме зерна (его амплитуда мала).
	jump := 0
	distinct := map[color.RGBA]bool{}
	for y := r.Min.Y + 6; y < r.Max.Y-6; y++ {
		for x := r.Min.X + 6; x < r.Max.X-6; x++ {
			a, b := img.RGBAAt(x, y), img.RGBAAt(x+1, y)
			if d := int(a.R) - int(b.R); d > 12 || d < -12 {
				jump++
			}
			distinct[a] = true
		}
	}
	if jump != 0 {
		t.Errorf("%d резких скачков внутри панели: размытия нет", jump)
	}
	if len(distinct) < 10 {
		t.Errorf("в панели всего %d разных цветов — зерна нет", len(distinct))
	}
	// Подкраска тёмная: панель заметно темнее обоев.
	mid := img.RGBAAt(200, 60)
	if int(mid.R)+int(mid.G)+int(mid.B) > 3*110 {
		t.Errorf("панель не затемнена: %v", mid)
	}
	// Шум действительно рисуется: без него кадры различаются.
	quiet := *s
	quiet.Backdrop.Noise = 0
	if imagesEqual(img, paintOnCheckers(t, &quiet, r, nil)) {
		t.Error("кадр не зависит от Noise — зерно не нарисовано")
	}
}

// TestPaintStyle_AcrylicFallbackIsSolid — без размытия вместо стекла
// сплошной запасной цвет: ровно RGB(31,31,31), как была панель до акрила.
func TestPaintStyle_AcrylicFallbackIsSolid(t *testing.T) {
	m := win10Manager(t, theme.ProfileWindows10)
	s := m.GetStyle("taskbar", "", theme.StateNormal)
	r := image.Rect(20, 40, 380, 80)

	img := paintOnCheckers(t, s, r, func(c widget.DrawContext) widget.DrawContext { return plainContext{c} })
	want := theme.RGB(31, 31, 31)
	if n, total := countColor(img, r, want), r.Dx()*r.Dy(); n != total {
		t.Errorf("запасной цвет занял %d пикселей из %d", n, total)
	}
	if got := img.RGBAAt(r.Min.X-3, 60); got == want {
		t.Error("запасной цвет вылез за область панели")
	}

	// Тема без запасного цвета (Windows 11) ведёт себя как раньше: подкраска
	// плёнкой поверх неразмытых обоев.
	m11 := win10Manager(t, theme.ProfileWindows11)
	s11 := m11.GetStyle("taskbar", "", theme.StateNormal)
	img11 := paintOnCheckers(t, s11, r, func(c widget.DrawContext) widget.DrawContext { return plainContext{c} })
	if img11.RGBAAt(30, 50) == img11.RGBAAt(30+12, 50) {
		t.Error("Windows 11 без размытия перестала просвечивать обоями")
	}
}

// TestPaintStyle_LightTaskbarIsLighter — флаг светлой панели делает акрил
// светлым, в обоих режимах отрисовки.
func TestPaintStyle_LightTaskbarIsLighter(t *testing.T) {
	m := win10Manager(t, theme.ProfileWindows10)
	dark := m.GetStyle("taskbar", "", theme.StateNormal)
	m.SetFlag(theme.KeyTaskbarLight, true)
	light := m.GetStyle("taskbar", "", theme.StateNormal)
	r := image.Rect(20, 40, 380, 80)

	sum := func(c color.RGBA) int { return int(c.R) + int(c.G) + int(c.B) }
	for name, wrap := range map[string]func(widget.DrawContext) widget.DrawContext{
		"размытие":   nil,
		"без стекла": func(c widget.DrawContext) widget.DrawContext { return plainContext{c} },
	} {
		d := paintOnCheckers(t, dark, r, wrap).RGBAAt(200, 60)
		l := paintOnCheckers(t, light, r, wrap).RGBAAt(200, 60)
		if sum(l) < sum(d)+3*100 {
			t.Errorf("%s: светлая панель %v недостаточно светлее тёмной %v", name, l, d)
		}
	}
}

// TestGolden_Windows10Acrylic — светлая и тёмная панель Windows 10 целиком, с
// содержимым. Кадры сохраняются при GOLDEN_OUT — смотреть глазами.
func TestGolden_Windows10Acrylic(t *testing.T) {
	const w, h = 640, 220
	for _, tc := range []struct {
		name  string
		light bool
	}{{"dark", false}, {"light", true}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := win10Manager(t, theme.ProfileWindows10)
			m.SetFlag(theme.KeyTaskbarLight, tc.light)
			eng := engine.New(w, h, 30)
			root, bar := buildScene(t, m, w, h)
			defer bar.Close()
			eng.SetRoot(root)
			img := eng.RenderOnce()
			if !barIsVisible(img, bar.Bounds().Min.Y) {
				t.Error("панель не нарисована")
			}
			if dir := os.Getenv("GOLDEN_OUT"); dir != "" {
				f, err := os.Create(filepath.Join(dir, "taskbar_win10_acrylic_"+tc.name+".png"))
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				if err := png.Encode(f, img); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
