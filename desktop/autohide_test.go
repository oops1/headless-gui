package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Панель, уезжающая за край экрана.

const screenBottom = 600

// hidingBar — панель с автоскрытием у нижнего края экрана 800×600.
func hidingBar(t *testing.T, profile string) *Taskbar {
	t.Helper()
	tm := managerFor(t, profile)
	bar := NewTaskbar(tm)
	t.Cleanup(bar.Close)
	bar.AddItem(SlotStart, NewStartButton(tm))

	h := bar.Height()
	if h <= 0 {
		t.Fatalf("тема %s не задала высоту панели", profile)
	}
	bar.SetBounds(image.Rect(0, screenBottom-h, 800, screenBottom))
	return bar
}

// settle доводит анимацию выезда до конца: ждать её в тестах нельзя, а
// проверять надо конечное положение. Время подаётся вручную — движок так же
// шагает анимации своим часами кадра.
func settle() {
	now := time.Now()
	for i := 0; i < 60 && widget.AnimationsActive(); i++ {
		now = now.Add(16 * time.Millisecond)
		widget.StepAnimations(now)
	}
}

func TestAutoHide_ConcealsAndRevealsOnEdge(t *testing.T) {
	bar := hidingBar(t, theme.ProfileWindows10)
	full := bar.Bounds()

	bar.SetAutoHide(true)
	if bar.IsRevealed() {
		t.Error("включённое автоскрытие оставило панель на экране")
	}
	if b := bar.Bounds(); b.Overlaps(image.Rect(0, 0, 800, screenBottom-1)) {
		t.Errorf("скрытая панель %v всё ещё на экране (полные границы %v)", b, full)
	}

	// Курсор у самого края — панель выезжает.
	bar.OnMouseMove(400, screenBottom-1)
	settle()
	if !bar.IsRevealed() {
		t.Fatal("курсор у края не выдвинул панель")
	}
	if got := bar.Bounds(); got != full {
		t.Errorf("выдвинутая панель встала в %v, ждали %v", got, full)
	}

	// Курсор ушёл вверх — панель убирается.
	bar.OnMouseMove(400, 100)
	settle()
	if bar.IsRevealed() {
		t.Error("уход курсора не убрал панель")
	}
}

// Скрытая панель не занимает места: развёрнутое окно идёт под неё.
func TestAutoHide_ReservedAreaIsEmpty(t *testing.T) {
	bar := hidingBar(t, theme.ProfileWindows10)
	if bar.ReservedArea().Empty() {
		t.Fatal("обычная панель не занимает места")
	}
	bar.SetAutoHide(true)
	if !bar.ReservedArea().Empty() {
		t.Error("автоскрытая панель всё ещё резервирует место")
	}
	bar.SetAutoHide(false)
	if bar.ReservedArea().Empty() {
		t.Error("панель, вернувшаяся из автоскрытия, места не занимает")
	}
}

// Верхняя панель (строка меню macOS) уезжает ВВЕРХ, а не вниз.
func TestAutoHide_TopBarSlidesUp(t *testing.T) {
	tm := managerFor(t, theme.ProfileMacOS)
	bar := NewTaskbar(tm)
	defer bar.Close()
	h := bar.Height()
	bar.SetBounds(image.Rect(0, 0, 800, h))

	if bar.Edge() != EdgeTop {
		t.Fatal("тема macOS не объявила панель верхней")
	}
	bar.SetAutoHide(true)
	if got := bar.Bounds(); got.Max.Y > 0 {
		t.Errorf("верхняя панель уехала в %v — вниз вместо верха", got)
	}
}

// Панель без автоскрытия ведёт себя как раньше и мышью не двигается.
func TestAutoHide_OffKeepsBarInPlace(t *testing.T) {
	bar := hidingBar(t, theme.ProfileWindows10)
	before := bar.Bounds()

	bar.OnMouseMove(400, 100)
	bar.OnMouseMove(400, screenBottom-1)
	settle()

	if got := bar.Bounds(); got != before {
		t.Errorf("панель без автоскрытия сдвинулась: %v → %v", before, got)
	}
}

// Смена границ (другое разрешение) не путает скрытую панель: она остаётся
// скрытой и возвращается на НОВОЕ место.
func TestAutoHide_SurvivesResize(t *testing.T) {
	bar := hidingBar(t, theme.ProfileWindows10)
	bar.SetAutoHide(true)

	h := bar.Height()
	newFull := image.Rect(0, 400-h, 640, 400)
	bar.SetBounds(newFull)
	if bar.IsRevealed() {
		t.Error("смена разрешения выдвинула скрытую панель")
	}

	bar.OnMouseMove(300, 399)
	settle()
	if got := bar.Bounds(); got != newFull {
		t.Errorf("после смены разрешения панель выехала в %v, ждали %v", got, newFull)
	}
}
