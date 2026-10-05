package desktop_test

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// manyApps заполняет меню сотней приложений: список во весь рост.
func manyApps(s *win10Scene, n int) {
	var entries []desktop.StartEntry
	for i := 0; i < n; i++ {
		entries = append(entries, entry(fmt.Sprintf("app%03d", i), fmt.Sprintf("Приложение %03d", i), "", cBlue))
	}
	s.menu.SetSource(desktop.NewFakeStartSource(nil, desktop.GroupByLetter(entries)))
}

// Открытие «Пуска» Windows 10: первый кадр после открытия быстрее 100 мс — и
// холодный (первый кадр процесса: кэши глифов и растров пусты), и тёплый.
// Сколько занимает каждый — в сообщении теста.
func TestStartMenu10_FirstFrameUnder100ms(t *testing.T) {
	s := newWin10Scene(t, 1280, 720, false, 1)
	manyApps(s, 300)

	t0 := time.Now()
	s.menu.Open(s.start.Bounds())
	s.menu.Settle()
	s.frame()
	cold := time.Since(t0)

	s.menu.Close()
	s.menu.Settle()
	s.eng.Invalidate()
	s.frame()

	t0 = time.Now()
	s.menu.Open(s.start.Bounds())
	s.menu.Settle()
	s.frame()
	warm := time.Since(t0)

	t.Logf("первый кадр после открытия: холодный %v, тёплый %v", cold, warm)
	if lim := perfBudget(100 * time.Millisecond); cold > lim {
		t.Errorf("холодный первый кадр %v — дольше %v", cold, lim)
	}
	if lim := perfBudget(100 * time.Millisecond); warm > lim {
		t.Errorf("тёплый первый кадр %v — дольше %v", warm, lim)
	}
}

// Кадр после открытия не больше области меню: открытие панели не будит
// отрисовку всего экрана. Заявленные движку области лежат внутри меню.
func TestStartMenu10_OpenDamagesOnlyMenuArea(t *testing.T) {
	s := newWin10Scene(t, 1280, 720, false, 1)
	s.frame() // обои и панель нарисованы

	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	s.menu.Open(s.start.Bounds())
	s.menu.Settle()
	if fulls != 0 {
		t.Errorf("открытие вызвало %d полных перерисовок", fulls)
	}
	area := s.menu.OverlayBounds().Inset(-80) // запас на тень и анимационный сдвиг
	for _, r := range rects {
		if !r.In(area) && !r.Empty() {
			t.Errorf("заявлена область %v вне меню %v", r, s.menu.OverlayBounds())
		}
	}
}

// Смена темы, акцента и языка на открытом меню не пересоздаёт компонентов: тот
// же объект, меню открыто, кадр получается и отличается от прежнего.
func TestStartMenu10_ThemeAccentLanguageOnOpenMenu(t *testing.T) {
	s := newWin10Scene(t, 1280, 720, false, 1)
	s.open()
	menuBefore := s.menu
	base := s.frame()
	snap := func(img *image.RGBA) []uint8 { return append([]uint8(nil), img.Pix...) }
	same := func(a []uint8, b *image.RGBA) bool {
		for i := range a {
			if a[i] != b.Pix[i] {
				return false
			}
		}
		return true
	}
	baseline := snap(base)

	// Акцент: плитки перекрашиваются.
	// Угол первой плитки: рамка 1 + боковая панель 48 + список 260 + поле 21;
	// сверху рамка 1 + поле 18 + заголовок группы 32 + зазор 5.
	pm := s.menu.OverlayBounds()
	tile := image.Pt(pm.Min.X+1+48+260+21+10, pm.Min.Y+1+18+32+5+10)
	before := base.RGBAAt(tile.X, tile.Y)
	s.tm.SetAccent(theme.RGB(190, 30, 60))
	finishAnims()
	s.eng.Invalidate()
	img := s.frame()
	after := img.RGBAAt(tile.X, tile.Y)
	if before == after {
		t.Errorf("плитка не сменила цвет с акцентом: %v", after)
	}
	if after.R < after.B || after.R < 150 {
		t.Errorf("плитка %v не стала красной после акцента (190,30,60)", after)
	}

	// Тема: светлая панель.
	s.tm.SetFlag(theme.KeyTaskbarLight, true)
	s.eng.Invalidate()
	light := s.frame()
	if same(baseline, light) {
		t.Error("светлая тема не изменила кадр")
	}
	panel := s.menu.OverlayBounds()
	if c := light.RGBAAt(panel.Min.X+150, panel.Max.Y-10); c.R < 150 {
		t.Errorf("светлая панель %v слишком тёмная", c)
	}

	// Язык: сменился заголовок развёрнутой панели.
	s.menu.SetSidebarExpanded(true)
	finishAnims()
	widget.SetLanguage("EN")
	defer widget.SetLanguage("RU")
	s.eng.Invalidate()
	if s.frame() == nil {
		t.Error("кадр после смены языка не получен")
	}

	if s.menu != menuBefore || !s.menu.IsOpen() {
		t.Error("компонент пересоздан или закрыт при смене темы, акцента и языка")
	}
	_ = color.RGBA{}
}

// Кадры анимации ширины боковой панели дешёвые. Меряется медиана кадров, а
// не среднее: тест идёт параллельно с другими пакетами, и один кадр,
// застрявший за чужой работой, растягивал среднее за порог (52 мс при
// обычных 8).
func TestStartMenu10_SidebarAnimationFramesAreCheap(t *testing.T) {
	s := newWin10Scene(t, 1280, 720, false, 1)
	manyApps(s, 300)
	s.open()
	s.frame()

	s.menu.SetSidebarExpanded(true)
	t0 := time.Now()
	widget.StepAnimations(t0)
	const frames = 9
	times := make([]time.Duration, 0, frames)
	for i := 1; i <= frames; i++ {
		widget.StepAnimations(t0.Add(time.Duration(i) * 16 * time.Millisecond))
		start := time.Now()
		s.frame()
		times = append(times, time.Since(start))
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	med := times[frames/2]
	t.Logf("медианный кадр анимации панели: %v", med)
	if lim := perfBudget(50 * time.Millisecond); med > lim {
		t.Errorf("кадр анимации панели %v — дороже %v", med, lim)
	}
	widget.StepAnimations(t0.Add(time.Second))
}
