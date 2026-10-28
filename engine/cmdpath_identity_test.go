package engine

// cmdpath_identity_test.go — И5 (ADR-0004): кадр через запись и
// воспроизведение побайтно равен кадру прямого пути при том же дереве,
// повреждении и масштабе. Две одинаковые сцены в двух движках: один с путём
// команд, другой без; одинаковые правки — одинаковые кадры.

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// cmdPair — две одинаковые сцены.
type cmdPair struct {
	on, off         *Engine
	rootOn, rootOff widget.Widget
	closeAll        func()
}

// cmdCompare сравнивает кадры пары на шаге name.
func cmdCompare(t *testing.T, p cmdPair, name string) {
	t.Helper()
	a, b := p.on.RenderOnce(), p.off.RenderOnce()
	if a.Rect != b.Rect {
		t.Fatalf("%s: размеры кадров %v и %v", name, a.Rect, b.Rect)
	}
	if bytes.Equal(a.Pix, b.Pix) {
		return
	}
	n, first := 0, image.Point{-1, -1}
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				if n == 0 {
					first = image.Pt(x, y)
				}
				n++
			}
		}
	}
	t.Errorf("%s: кадры расходятся в %d пикселях, первый %v (команды %v, прямой %v)",
		name, n, first, a.RGBAAt(first.X, first.Y), b.RGBAAt(first.X, first.Y))
}

// labelsOf собирает лейблы дерева в порядке обхода.
func labelsOf(w widget.Widget, out *[]*widget.Label) {
	if l, ok := w.(*widget.Label); ok {
		*out = append(*out, l)
	}
	for _, c := range w.Children() {
		labelsOf(c, out)
	}
}

// cmdRunSteps гонит пару через полный кадр, точечные правки лейблов,
// случайные ничейные повреждения и кадр без повреждения.
func cmdRunSteps(t *testing.T, p cmdPair, seed int64) {
	t.Helper()
	p.on.SetRenderOnDemand(true)
	p.off.SetRenderOnDemand(true)
	cmdCompare(t, p, "полный кадр")
	cmdCompare(t, p, "кадр без повреждения")

	var la, lb []*widget.Label
	labelsOf(p.rootOn, &la)
	labelsOf(p.rootOff, &lb)
	if len(la) != len(lb) {
		t.Fatalf("сцены разные: %d и %d лейблов", len(la), len(lb))
	}
	rng := rand.New(rand.NewSource(seed))
	w, h := p.on.canvas.LogicalSize()
	for i := 0; i < 12; i++ {
		switch {
		case len(la) > 0 && i%3 != 2:
			k := rng.Intn(len(la))
			text := la[k].Text() + "·"
			la[k].SetText(text)
			lb[k].SetText(text)
			cmdCompare(t, p, "правка лейбла")
		default:
			r := image.Rect(rng.Intn(w), rng.Intn(h), rng.Intn(w), rng.Intn(h)).Canon()
			if r.Empty() {
				r = image.Rect(0, 0, 64, 64)
			}
			p.on.InvalidateRect(r)
			p.off.InvalidateRect(r)
			cmdCompare(t, p, "случайное повреждение")
		}
	}
	// Две правки в далёких местах — список областей, не объединение.
	if len(la) > 3 {
		la[0].SetText("α")
		lb[0].SetText("α")
		la[len(la)-1].SetText("ω")
		lb[len(lb)-1].SetText("ω")
		cmdCompare(t, p, "две далёкие правки")
	}
}

func cmdTextPair(k float64) cmdPair {
	on, rootOn := benchTextScene(1280, 800)
	off, rootOff := benchTextScene(1280, 800)
	off.SetCommandPath(false)
	on.SetScale(k)
	off.SetScale(k)
	return cmdPair{on: on, off: off, rootOn: rootOn, rootOff: rootOff, closeAll: func() { on.Stop(); off.Stop() }}
}

func TestCmdPath_IdentityTextScene(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25, 1.5} {
		t.Run(scaleName(k), func(t *testing.T) {
			p := cmdTextPair(k)
			defer p.closeAll()
			cmdRunSteps(t, p, 11)
		})
	}
}

func scaleName(k float64) string {
	switch k {
	case 1:
		return "1x"
	case 1.25:
		return "1.25x"
	case 1.5:
		return "1.5x"
	}
	return "k"
}

func TestCmdPath_IdentityDesktopScene(t *testing.T) {
	testreset.Globals(t)
	a := buildPipelineDesktop()
	b := buildPipelineDesktop()
	defer a.closeAll()
	defer b.closeAll()
	defer a.eng.Stop()
	defer b.eng.Stop()
	b.eng.SetCommandPath(false)
	p := cmdPair{on: a.eng, off: b.eng, rootOn: a.eng.root, rootOff: b.eng.root}
	cmdRunSteps(t, p, 5)
	// Наведение на «Пуск» — меняется кнопка, остальное удержано.
	btn := a.startBtn.Bounds()
	onX, onY := (btn.Min.X+btn.Max.X)/2, (btn.Min.Y+btn.Max.Y)/2
	a.eng.SendMouseMove(onX, onY)
	b.eng.SendMouseMove(onX, onY)
	cmdCompare(t, p, "наведение на кнопку Пуск")
	a.eng.SendMouseMove(onX, pipeH/2)
	b.eng.SendMouseMove(onX, pipeH/2)
	cmdCompare(t, p, "снятие наведения")
}

// cmdSampler — сцена сэмплера виджетов (как в golden_test), в двух движках.
// Группы радиокнопок — общие на процесс: у каждой сцены своя, иначе выбор во
// второй снимал бы выбор в первой.
func cmdSampler(th *widget.Theme) cmdPair {
	build := func(group string) (*Engine, widget.Widget) {
		eng := New(420, 320, 20)
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
		rb := widget.NewRadioButton("Выбор", group)
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
		sv := widget.NewScrollView()
		sv.SetBounds(image.Rect(16, 240, 400, 310))
		sv.ContentHeight = 400
		for i := 0; i < 8; i++ {
			l := widget.NewLabel("строка прокрутки", th.LabelText)
			l.SetBounds(image.Rect(20, 244+i*40, 380, 244+i*40+24))
			sv.AddChild(l)
		}
		sv.SetScrollY(60)
		root.AddChild(sv)
		widget.ApplyThemeTree(root, th)
		eng.SetRoot(root)
		return eng, root
	}
	on, rootOn := build("cmd-on")
	off, rootOff := build("cmd-off")
	off.SetCommandPath(false)
	return cmdPair{on: on, off: off, rootOn: rootOn, rootOff: rootOff, closeAll: func() { on.Stop(); off.Stop() }}
}

func TestCmdPath_IdentityWidgetSampler(t *testing.T) {
	testreset.Globals(t)
	for _, themeName := range []string{"Win10 Dark", "Win11 Light", "Mac"} {
		themeName := themeName
		t.Run(themeName, func(t *testing.T) {
			withTheme(t, themeName, func(th *widget.Theme) {
				p := cmdSampler(th)
				defer p.closeAll()
				cmdRunSteps(t, p, 3)
				p.on.SetScale(1.25)
				p.off.SetScale(1.25)
				cmdCompare(t, p, "масштаб 1,25")
				p.on.InvalidateRect(image.Rect(20, 250, 300, 300))
				p.off.InvalidateRect(image.Rect(20, 250, 300, 300))
				cmdCompare(t, p, "повреждение над прокруткой на 1,25")
			})
		})
	}
}

// Переключение пути на живом движке — тот же кадр.
func TestCmdPath_ToggleKeepsFrame(t *testing.T) {
	testreset.Globals(t)
	eng, _ := benchTextScene(640, 400)
	defer eng.Stop()
	root := eng.root
	var ls []*widget.Label
	labelsOf(root, &ls)
	a := eng.RenderOnce()
	eng.SetCommandPath(false)
	b := eng.RenderOnce()
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Error("кадр после выключения пути команд отличается")
	}
	// Переключение перерисовывает кадр целиком — сравнивать с полным кадром
	// прямого пути (частичный кадр прямого пути отличается от полного и сам
	// по себе: глифы за границами лейбла обрезаются повреждением).
	ls[3].SetText("другой текст")
	eng.RenderOnce()
	eng.Invalidate()
	b = eng.RenderOnce()
	eng.SetCommandPath(true)
	a = eng.RenderOnce()
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Error("кадр после включения пути команд отличается")
	}
	_ = color.RGBA{}
}
