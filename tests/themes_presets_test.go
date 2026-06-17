package tests

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

func TestThemes_RegistryNamesAndLookup(t *testing.T) {
	names := widget.ThemeNames()
	want := []string{"Win10 Dark", "Win10 Light", "Win11 Dark", "Win11 Light", "Win2000", "Mac"}
	if len(names) != len(want) {
		t.Fatalf("ThemeNames: %v", names)
	}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("ThemeNames[%d] = %q, want %q", i, names[i], n)
		}
		if th := widget.ThemeByName(n); th == nil {
			t.Fatalf("ThemeByName(%q) == nil", n)
		}
	}
	// Без учёта регистра + неизвестное имя.
	if widget.ThemeByName("win2000") == nil {
		t.Fatal("lookup должен быть регистронезависимым")
	}
	if widget.ThemeByName("Amiga") != nil {
		t.Fatal("неизвестная тема должна давать nil")
	}
}

func TestThemes_StyleParameters(t *testing.T) {
	if st := widget.ThemeByName("Win2000").Style; !st.Classic3D || st.ControlCorner != 0 {
		t.Fatalf("Win2000 style: %+v", st)
	}
	if st := widget.ThemeByName("Win11 Dark").Style; st.ControlCorner != 6 || st.Classic3D {
		t.Fatalf("Win11 Dark style: %+v", st)
	}
	if st := widget.ThemeByName("Mac").Style; st.ControlCorner != 8 {
		t.Fatalf("Mac style: %+v", st)
	}
	if st := widget.ThemeByName("Win10 Dark").Style; st.ControlCorner != 0 || st.Classic3D {
		t.Fatalf("Win10 Dark style: %+v", st)
	}
	// Скругление окна и Mac-заголовок.
	if st := widget.ThemeByName("Win11 Dark").Style; st.WindowCorner != 8 || st.MacTitleBar {
		t.Fatalf("Win11 window style: %+v", st)
	}
	if st := widget.ThemeByName("Mac").Style; st.WindowCorner != 10 || !st.MacTitleBar {
		t.Fatalf("Mac window style: %+v", st)
	}
	if st := widget.ThemeByName("Win2000").Style; st.WindowCorner != 0 || st.MacTitleBar {
		t.Fatalf("Win2000 window style: %+v", st)
	}
}

// Window.ApplyTheme переносит форму окна из темы: скругление + стиль заголовка.
func TestThemes_WindowApplyShape(t *testing.T) {
	w := &widget.Window{Title: "x"}

	w.ApplyTheme(widget.ThemeByName("Mac"))
	if w.CornerRadius != 10 || w.TitleStyle != widget.WindowTitleMac {
		t.Fatalf("Mac: corner=%d style=%v", w.CornerRadius, w.TitleStyle)
	}
	w.ApplyTheme(widget.ThemeByName("Win11 Light"))
	if w.CornerRadius != 8 || w.TitleStyle != widget.WindowTitleWin {
		t.Fatalf("Win11: corner=%d style=%v", w.CornerRadius, w.TitleStyle)
	}
	w.ApplyTheme(widget.ThemeByName("Win2000"))
	if w.CornerRadius != 0 || w.TitleStyle != widget.WindowTitleWin {
		t.Fatalf("Win2000: corner=%d style=%v", w.CornerRadius, w.TitleStyle)
	}
}

// Переключение всех тем на живом движке: рендер не падает, пиксели меняются.
func TestThemes_SwitchOnLiveEngine(t *testing.T) {
	eng := engine.New(200, 120, 30)
	eng.SetTooltipsEnabled(false)

	root := widget.NewPanel(widget.DarkTheme().WindowBG)
	root.SetBounds(image.Rect(0, 0, 200, 120))
	btn := widget.NewButton("OK")
	btn.SetBounds(image.Rect(20, 20, 120, 52))
	root.AddChild(btn)
	ti := widget.NewTextInput("")
	ti.SetText("abc")
	ti.SetBounds(image.Rect(20, 60, 180, 90))
	root.AddChild(ti)

	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	prev := uint64(0)
	for _, name := range widget.ThemeNames() {
		eng.SetTheme(widget.ThemeByName(name))
		time.Sleep(80 * time.Millisecond) // пара кадров на перерисовку
		if rc := eng.RenderCount(); rc <= prev {
			t.Fatalf("тема %q: рендер не продвинулся (%d)", name, rc)
		} else {
			prev = rc
		}
	}
	// Возвращаем тему по умолчанию, чтобы не влиять на другие тесты.
	eng.SetTheme(widget.DarkTheme())
	time.Sleep(50 * time.Millisecond)
}
