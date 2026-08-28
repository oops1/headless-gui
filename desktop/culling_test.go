package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Тик часов не будит весь рабочий стол.
//
// Это и был главный довод за пропуск поддеревьев: часы на панели меняют две
// цифры раз в минуту, а кадр обходил ради этого всё дерево. Проверяется
// счётчиком вызовов Draw.

// countingItem — элемент панели, считающий свои отрисовки.
type countingItem struct {
	Item
	draws int
}

func (c *countingItem) Draw(ctx widget.DrawContext) {
	c.draws++
	c.Item.Draw(ctx)
}

func TestCulling_ClockTickDoesNotWakeTheDesktop(t *testing.T) {
	const w, h = 900, 600

	tm := managerFor(t, theme.ProfileWindows11)
	tm.SetIconResolver(widget.BuiltinIcons())

	root := widget.NewPanel(theme.RGB(20, 40, 80))
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	// Обои: то, что не должно перерисовываться из-за часов.
	wallpaper := &countingItem{Item: newWallPanel(image.Rect(0, 0, w, h-60))}
	root.AddChild(wallpaper)

	wm := NewFakeWindowModel(WindowInfo{ID: 1, Title: "Окно", Active: true})
	clk := NewFakeClock(time.Date(2026, 8, 28, 14, 35, 0, 0, time.UTC))

	bar := NewTaskbar(tm)
	defer bar.Close()
	apps := NewRunningApplications(tm, wm)
	clock := NewClock(tm, clk)
	bar.AddItem(SlotStart, NewStartButton(tm))
	bar.AddItem(SlotApps, apps)
	bar.AddItem(SlotTray, clock)
	bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))
	root.AddChild(bar)

	eng := engine.New(w, h, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce() // первый кадр рисует всё
	// Второй кадр съедает полную инвалидацию, которую оставил запуск
	// секундного тика часов: он случается при первой отрисовке.
	eng.RenderOnce()

	before := wallpaper.draws

	// Минута прошла: часы меняют строку и заявляют свою область.
	clk.Advance(time.Minute)
	clock.Invalidate()
	eng.RenderOnce()

	if wallpaper.draws != before {
		t.Errorf("обои перерисованы %d раз из-за тика часов — поддерево не пропущено",
			wallpaper.draws-before)
	}
}

// newWallPanel — простая панель-обои.
func newWallPanel(r image.Rectangle) Item {
	p := widget.NewPanel(theme.RGB(30, 60, 110))
	p.ShowHeader = false
	p.SetBounds(r)
	return panelItem{p}
}

// panelItem превращает панель в элемент панели задач: тестам нужен только
// Draw, а PreferredSize спрашивают у настоящих элементов.
type panelItem struct{ *widget.Panel }

func (p panelItem) PreferredSize(avail image.Point) image.Point { return avail }
