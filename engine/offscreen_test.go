package engine

import (
	"fmt"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/gomono"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// byNameT — именованный шрифт холста по имени (nil, если его нет).
func byNameT(c *Canvas, name string) *FontCache {
	fc, _ := c.NamedFonts().Get(name)
	return fc
}

// Движок вне экрана не забирает у окна измеритель текста: окно, пока идёт
// печать, продолжает мерить своим холстом и своими шрифтами (замечание
// WinLine после v3.33.0).
func TestOffscreen_DoesNotTakeMeasurer(t *testing.T) {
	testreset.Globals(t)
	win := New(320, 200, 20)
	defer win.Stop()
	// Шрифт, известный только окну: моноширинный, у него «iiii» шире, чем у
	// пропорционального шрифта по умолчанию.
	win.canvas.RegisterFont("OnlyWindow", gomono.TTF)
	widget.BumpTextMetricsRev()
	want := widget.MeasureUITextFont("iiii", 12, "OnlyWindow")

	job := NewOffscreen(800, 1100, 20)
	defer job.Stop()
	if !job.IsOffscreen() || win.IsOffscreen() {
		t.Fatalf("IsOffscreen: окно %v, задание %v", win.IsOffscreen(), job.IsOffscreen())
	}
	widget.BumpTextMetricsRev()
	if got := widget.MeasureUITextFont("iiii", 12, "OnlyWindow"); got != want {
		t.Fatalf("после NewOffscreen замер %d, у окна было %d — измеритель ушёл заданию", got, want)
	}

	// Для сравнения: обычный второй движок измеритель забирает, и шрифта
	// окна у него нет.
	other := New(320, 200, 20)
	widget.BumpTextMetricsRev()
	took := widget.MeasureUITextFont("iiii", 12, "OnlyWindow")
	other.Stop()
	if took == want {
		t.Fatalf("второй New не забрал измеритель — тест ничего не проверяет")
	}
}

// Шрифты регистрируются из одной горутины, пока другая меряет текст через
// измеритель процесса: так печать Блокнота регистрировала шрифты задания,
// пока окно мерило. Под -race гонки быть не должно.
func TestFonts_RegisterWhileMeasuring(t *testing.T) {
	testreset.Globals(t)
	e := New(320, 200, 20)
	defer e.Stop()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			e.canvas.RegisterFont(fmt.Sprintf("Job%d", i), gomono.TTF)
		}
	}()
	for i := 0; i < 200; i++ {
		widget.BumpTextMetricsRev()
		widget.MeasureUITextFont("Печать", 12, fmt.Sprintf("Job%d", i%20))
	}
	wg.Wait()
	if byNameT(e.canvas, "Job19") == nil {
		t.Fatal("шрифт, зарегистрированный из другой горутины, не появился")
	}
}
