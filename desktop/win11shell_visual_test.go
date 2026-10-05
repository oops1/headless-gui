package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Снимки панели Windows 11 (W11_OUT=dir сохраняет PNG, GOLDEN_OUT тоже годится).
// Без переменной тест проверяет, что кадры рисуются и различаются.

// renderMid рисует кадр в середине первого мигания «внимания»: анимации заведены
// рисованием и продвинуты на d, но не доведены до конца.
func (s *w11Scene) renderMid(scale float64, d time.Duration) *image.RGBA {
	eng := engine.New(w11W, w11H, 30)
	if scale != 1 {
		eng.SetScale(scale)
	}
	if err := eng.SetThemeProfile(s.tm, s.tm.Active().Name()); err != nil {
		s.t.Fatal(err)
	}
	eng.ApplyThemeProfile(s.tm)
	eng.SetRoot(s.root)
	eng.RenderOnce()
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(d))
	eng.Invalidate()
	return eng.RenderOnce()
}

func w11Save(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	savePNG(t, name, img)
}

func TestWin11Bar_Shots(t *testing.T) {
	type shot struct {
		name    string
		profile string
		center  bool
		search  SearchMode
		hover   bool // наведение на группу трея
		dnd     bool
	}
	shots := []shot{
		{"win11_light_center", theme.ProfileWindows11, true, SearchModeIconOnly, true, false},
		{"win11_dark_center", theme.ProfileWindows11Dark, true, SearchModeIconOnly, true, false},
		{"win11_light_left", theme.ProfileWindows11, false, SearchModeIconAndLabel, false, false},
		{"win11_dark_left", theme.ProfileWindows11Dark, false, SearchModeBox, false, true},
		{"win11_light_center_box", theme.ProfileWindows11, true, SearchModeBox, false, false},
		{"win11_dark_center_label", theme.ProfileWindows11Dark, true, SearchModeIconAndLabel, false, true},
	}
	var frames []*image.RGBA
	for _, sh := range shots {
		s := newW11Scene(t, sh.profile)
		s.bar.SetAlignment(BarAlignLeft)
		if sh.center {
			s.bar.SetAlignment(BarAlignCenter)
		}
		s.search.SetMode(sh.search)
		s.bell.SetDoNotDisturb(sh.dnd)
		if sh.hover {
			g := s.group.Bounds()
			s.group.OnMouseMove(g.Min.X+10, g.Min.Y+10)
		}
		img := s.render(1)
		frames = append(frames, img)
		w11Save(t, sh.name+".png", img)
		w11Save(t, sh.name+"_x2.png", s.render(2))
	}
	for i := 1; i < len(frames); i++ {
		if imagesEqualRGBA(frames[0], frames[i]) {
			t.Errorf("кадры %s и %s одинаковы", shots[0].name, shots[i].name)
		}
	}

	// Мигание «внимания» в середине и постоянная подсветка после него.
	s := newW11Scene(t, theme.ProfileWindows11)
	mid := s.renderMid(2, 225*time.Millisecond)
	w11Save(t, "win11_light_attention_blink_x2.png", mid)
}
