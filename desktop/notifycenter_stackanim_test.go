package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// Плавное раскрытие и сворачивание стопки уведомлений Windows 11 (v3.34): токен
// notification.expand, «меньше движения» — мгновенно, перерисовывается только
// область панели.

// Клик по «+N»: сразу после нажатия вид тот же (без скачка), посреди движения
// группа между свёрнутой и раскрытой, кнопка уходит вместе с нижним краем
// карточки, а к концу раскладка равна статичной.
func TestStackAnim_ExpandIsGradual(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	finishAnimations()
	closed := stackGroup(t, w, "vpn")
	closedBounds := w.nc.OverlayBounds()
	closedH := closed.cards[0].rect.Dy()

	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneStack, app: "vpn"}).rect)
	// Сразу после нажатия движение только началось: тот же вид.
	start := stackGroup(t, w, "vpn")
	if !start.stackOpen || !start.anim {
		t.Fatalf("стопка не пошла раскрываться: open=%v anim=%v", start.stackOpen, start.anim)
	}
	if got := w.nc.OverlayBounds().Dy(); got != closedBounds.Dy() {
		t.Errorf("панель скакнула в момент нажатия: %d, было %d", got, closedBounds.Dy())
	}
	if len(start.cards) != 2 {
		t.Fatalf("в движении карточек %d, ждали обе", len(start.cards))
	}
	if c := start.cards[0]; c.rect.Dy() != closedH || c.stackRect.Empty() {
		t.Errorf("первая карточка сразу изменилась: высота %d (была %d), кнопка %v", c.rect.Dy(), closedH, c.stackRect)
	}

	stepAnim(75 * time.Millisecond)
	mid := stackGroup(t, w, "vpn")
	if !mid.anim {
		t.Fatal("посреди движения группа не помечена движущейся")
	}
	midH := w.nc.OverlayBounds().Dy()
	finishAnimations()
	open := stackGroup(t, w, "vpn")
	openBounds := w.nc.OverlayBounds()
	if open.anim || !open.stackOpen {
		t.Fatalf("после движения: anim=%v open=%v", open.anim, open.stackOpen)
	}
	if midH <= closedBounds.Dy() || midH >= openBounds.Dy() {
		t.Errorf("высота панели посреди движения %d не между %d и %d", midH, closedBounds.Dy(), openBounds.Dy())
	}
	for _, c := range open.cards {
		if !c.stackRect.Empty() || c.anim {
			t.Errorf("карточка %q после раскрытия: кнопка %v, anim=%v", c.n.Title, c.stackRect, c.anim)
		}
	}
	if mid.cards[0].rect.Dy() >= closedH || mid.cards[0].rect.Dy() <= open.cards[0].rect.Dy() {
		t.Errorf("первая карточка посреди движения %d не между раскрытой %d и свёрнутой %d",
			mid.cards[0].rect.Dy(), open.cards[0].rect.Dy(), closedH)
	}
	// Итог движения — как у статичной раскладки при том же состоянии.
	w2 := newW11Fix(t, theme.ProfileWindows11, true)
	w2.tm.SetFlag(theme.FlagMotionReduce, true)
	w2.open()
	w11Click(w2.nc, w2.zone(t, zoneKey{kind: zoneStack, app: "vpn"}).rect)
	if w2.nc.OverlayBounds().Dy() != openBounds.Dy() {
		t.Errorf("итог движения %d не равен мгновенному раскрытию %d", openBounds.Dy(), w2.nc.OverlayBounds().Dy())
	}
	if !containsText(w.drawn(w.nc).texts, "Отключено") {
		t.Error("раскрытая стопка не показала вторую карточку")
	}
}

// Сворачивание — то же в обратную сторону и приходит ровно к статичной свёрнутой
// раскладке (кнопка «+N» на месте).
func TestStackAnim_CollapseIsGradual(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneStack, app: "vpn"}).rect)
	finishAnimations()
	openBounds := w.nc.OverlayBounds()

	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneGroup, app: "vpn"}).rect)
	g := stackGroup(t, w, "vpn")
	if g.stackOpen || !g.anim {
		t.Fatalf("сворачивание не пошло: open=%v anim=%v", g.stackOpen, g.anim)
	}
	if got := w.nc.OverlayBounds().Dy(); got != openBounds.Dy() {
		t.Errorf("панель скакнула в момент нажатия: %d, было %d", got, openBounds.Dy())
	}
	stepAnim(75 * time.Millisecond)
	midH := w.nc.OverlayBounds().Dy()
	finishAnimations()
	closedBounds := w.nc.OverlayBounds()
	g = stackGroup(t, w, "vpn")
	if g.anim || g.stackOpen || len(g.cards) != 1 || g.cards[0].stackRect.Empty() {
		t.Fatalf("после сворачивания: anim=%v open=%v карточек %d", g.anim, g.stackOpen, len(g.cards))
	}
	if midH >= openBounds.Dy() || midH <= closedBounds.Dy() {
		t.Errorf("высота панели посреди движения %d не между %d и %d", midH, closedBounds.Dy(), openBounds.Dy())
	}
	// Движение прервали на полпути обратным щелчком: стартует с достигнутого, без скачка.
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneStack, app: "vpn"}).rect)
	stepAnim(75 * time.Millisecond)
	h1 := w.nc.OverlayBounds().Dy()
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneGroup, app: "vpn"}).rect)
	if h2 := w.nc.OverlayBounds().Dy(); h2 < h1-2 || h2 > h1+2 {
		t.Errorf("встречный щелчок дал скачок высоты: %d → %d", h1, h2)
	}
}

// «Меньше движения»: раскрытие мгновенно, без промежуточных кадров.
func TestStackAnim_ReduceMotionIsInstant(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.tm.SetFlag(theme.FlagMotionReduce, true)
	w.open()
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneStack, app: "vpn"}).rect)
	g := stackGroup(t, w, "vpn")
	if g.anim || !g.stackOpen || len(g.cards) != 2 {
		t.Fatalf("при «меньше движения»: anim=%v open=%v карточек %d", g.anim, g.stackOpen, len(g.cards))
	}
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneGroup, app: "vpn"}).rect)
	if g := stackGroup(t, w, "vpn"); g.anim || g.stackOpen || len(g.cards) != 1 {
		t.Errorf("сворачивание не мгновенное: anim=%v open=%v карточек %d", g.anim, g.stackOpen, len(g.cards))
	}
}

// Шаги движения перерисовывают только область панели (прежнюю и новую), а не экран.
func TestStackAnim_InvalidatesOnlyPanel(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.open()
	finishAnimations()
	before := w.nc.OverlayBounds()
	// Панель открыта, её выезд закончен. Дальше считаем заявки.
	inv := watchInvalidations(t)
	w11Click(w.nc, w.zone(t, zoneKey{kind: zoneStack, app: "vpn"}).rect)
	inv.reset()
	stepAnim(75 * time.Millisecond)
	rects := inv.get()
	if len(rects) == 0 {
		t.Fatal("шаг движения ничего не заявил")
	}
	after := w.nc.OverlayBounds()
	area := before.Union(after).Inset(-40) // с запасом на тень
	for _, r := range rects {
		if !r.In(area) {
			t.Errorf("шаг движения заявил %v вне панели %v", r, area)
		}
	}
	var union image.Rectangle
	for _, r := range rects {
		union = union.Union(r)
	}
	if !(image.Pt(after.Min.X+10, after.Min.Y+10).In(union)) {
		t.Errorf("верх выросшей панели %v не заявлен: %v", after, union)
	}
	finishAnimations()
}

// Расклад в тосте и без флага стопки не меняется: движения стопки нет.
func TestStackAnim_NoStackNoSlide(t *testing.T) {
	w := newW11Fix(t, theme.ProfileWindows11, true)
	w.tm.SetFlag(theme.KeyNotifyStack, false)
	w.open()
	for _, g := range w.nc.view.layout(w.nc.rect()).groups {
		if g.stack || g.anim {
			t.Errorf("группа %q при погашенном флаге: stack=%v anim=%v", g.app, g.stack, g.anim)
		}
	}
	for k := range w.nc.view.slides {
		if k.kind == slideStack {
			t.Errorf("движение стопки %+v заведено без флага", k)
		}
	}
}
