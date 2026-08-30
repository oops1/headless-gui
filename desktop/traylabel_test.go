package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Надпись в трее.
//
// Раскладку клавиатуры Windows показывает словом («РУС»), а не фигурой, и
// это читается мгновенно. Положить в трей текст было нечем: значки сети,
// звука и питания рисуются фигурами, а элемента с надписью у трея не было.

func trayLabelTheme(t *testing.T, minWidth float64) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("TrayLabelTest")
	p.SetStyle(ComponentTrayLabel, "", theme.StateNormal, theme.StyleDelta{
		Text: theme.C(theme.RGB(230, 230, 230)),
		PadX: theme.N(4),
		Font: &theme.FontSpec{Size: 10},
	})
	p.SetStyle(ComponentTrayLabel, "", theme.StateHover, theme.StyleDelta{
		Fill: theme.C(theme.RGB(70, 70, 70)),
	})
	p.SetMetric(KeyTrayLabelMinWidth, minWidth)
	if err := m.RegisterTheme(p); err != nil {
		t.Fatalf("RegisterTheme: %v", err)
	}
	if err := m.SetTheme("TrayLabelTest"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	return m
}

func TestTrayLabel_DrawsItsText(t *testing.T) {
	l := NewTrayLabel(trayLabelTheme(t, 0), "РУС")
	l.SetBounds(image.Rect(0, 0, 40, 24))

	ctx := &recCtx{}
	l.Draw(ctx)

	found := false
	for _, tx := range ctx.texts {
		if tx.text == "РУС" {
			found = true
		}
	}
	if !found {
		t.Errorf("надпись не нарисована: тексты %+v", ctx.texts)
	}
}

// Ширина не меньше заданной темой: иначе соседние значки прыгали бы при
// каждой смене раскладки.
func TestTrayLabel_KeepsMinimumWidth(t *testing.T) {
	const min = 40
	l := NewTrayLabel(trayLabelTheme(t, min), "EN")

	if got := l.PreferredSize(image.Pt(200, 24)); got.X < min {
		t.Errorf("ширина %d меньше заданной темой %d — соседи будут прыгать", got.X, min)
	}

	// Длинная строка минимум перерастает.
	l.SetText("Русский")
	long := l.PreferredSize(image.Pt(200, 24))
	l.SetText("EN")
	short := l.PreferredSize(image.Pt(200, 24))
	if long.X < short.X {
		t.Errorf("длинная надпись уже короткой: %d против %d", long.X, short.X)
	}
}

// Пустая надпись места не занимает: трей не должен держать дыру под текст,
// которого нет.
func TestTrayLabel_EmptyTakesNoRoom(t *testing.T) {
	l := NewTrayLabel(trayLabelTheme(t, 40), "")
	if got := l.PreferredSize(image.Pt(200, 24)); got.X != 0 {
		t.Errorf("пустая надпись просит %d точек ширины", got.X)
	}

	l.SetBounds(image.Rect(0, 0, 40, 24))
	ctx := &recCtx{}
	l.Draw(ctx)
	if len(ctx.texts) != 0 || len(ctx.fills) != 0 {
		t.Errorf("пустая надпись что-то нарисовала: %+v %+v", ctx.texts, ctx.fills)
	}
}

// Щелчок доходит до оболочки — на нём висит переключение раскладки.
func TestTrayLabel_ClickReachesTheShell(t *testing.T) {
	l := NewTrayLabel(trayLabelTheme(t, 0), "РУС")
	l.SetBounds(image.Rect(0, 0, 40, 24))

	clicks := 0
	l.OnClick = func() { clicks++ }

	press := widget.MouseEvent{X: 20, Y: 12, Button: widget.MouseLeft, Pressed: true}
	release := widget.MouseEvent{X: 20, Y: 12, Button: widget.MouseLeft}
	l.OnMouseButton(press)
	l.OnMouseButton(release)

	if clicks != 1 {
		t.Errorf("щелчков дошло %d, ждали один", clicks)
	}

	// Отпускание в стороне не считается — как у всех элементов панели.
	l.OnMouseButton(press)
	l.OnMouseButton(widget.MouseEvent{X: 200, Y: 200, Button: widget.MouseLeft})
	if clicks != 1 {
		t.Errorf("отпускание вне надписи сработало как щелчок: %d", clicks)
	}
}

// Наведение видно: тема задаёт для него свою заливку.
func TestTrayLabel_HoverIsVisible(t *testing.T) {
	l := NewTrayLabel(trayLabelTheme(t, 0), "РУС")
	l.SetBounds(image.Rect(0, 0, 40, 24))

	plain := &recCtx{}
	l.Draw(plain)

	l.OnMouseMove(20, 12)
	hovered := &recCtx{}
	l.Draw(hovered)

	if len(hovered.fills) <= len(plain.fills) {
		t.Error("наведение не изменило отрисовку — заданная темой заливка не видна")
	}
}

// Смена текста — из горутины оболочки, отрисовка — из горутины кадра.
func TestTrayLabel_TextIsSafeAcrossGoroutines(t *testing.T) {
	l := NewTrayLabel(trayLabelTheme(t, 0), "РУС")
	l.SetBounds(image.Rect(0, 0, 40, 24))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			if i%2 == 0 {
				l.SetText("РУС")
			} else {
				l.SetText("ENG")
			}
		}
	}()
	for i := 0; i < 200; i++ {
		l.Draw(&recCtx{})
		l.PreferredSize(image.Pt(200, 24))
	}
	<-done
}
