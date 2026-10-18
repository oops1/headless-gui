package widget

import (
	"testing"
	"time"
)

// Задержка подменю (menu.submenu.delay) отсчитывается от момента наведения, а
// не от первого кадра после него: в тесте без движка один шаг
// StepAnimations(time.Now().Add(задержка)) раскрывает подменю, и «первый шаг
// только заводит часы» больше не нужен (замечание WinLine после v3.32.0, п.5).
func TestPopupMenu_SubMenuDelayCountsFromHover(t *testing.T) {
	newMenu := func() *PopupMenu {
		m := NewPopupMenu()
		m.SubMenuDelay = 200
		m.SetItems([]MenuItem{
			{Text: "Создать", SubItems: []MenuItem{{Text: "Папку"}}},
			{Text: "Свойства"},
		})
		m.Show(10, 10)
		return m
	}
	rowY := func(m *PopupMenu, i int) int { return m.layout().itemsTop + i*m.ItemHeight + 5 }
	opened := func(m *PopupMenu) bool { c, _ := m.openChildOf(); return c != nil }

	// Один шаг по истечении задержки — без предварительного «заводящего».
	m := newMenu()
	defer m.Close()
	m.OnMouseMove(30, rowY(m, 0))
	if opened(m) {
		t.Fatal("подменю раскрылось мгновенно, хотя задана задержка")
	}
	StepAnimations(time.Now().Add(100 * time.Millisecond))
	if opened(m) {
		t.Fatal("подменю раскрылось раньше срока")
	}
	// Шаг до срока часы не перезапускает: срок считается от наведения.
	StepAnimations(time.Now().Add(250 * time.Millisecond))
	if !opened(m) {
		t.Fatal("подменю не раскрылось одним шагом по истечении задержки")
	}

	// Первый же кадр пришёл поздно (через секунду после наведения): подменю
	// раскрывается на нём, а не ждёт ещё одну задержку.
	m.OnMouseMove(30, rowY(m, 1)) // курсор ушёл: подменю закроется после задержки
	StepAnimations(time.Now().Add(time.Second))
	if opened(m) {
		t.Error("подменю не закрылось одним поздним шагом")
	}

	// Возврат на пункт до срока отменяет отсчёт, новый идёт от нового наведения.
	m.OnMouseMove(30, rowY(m, 0))
	m.OnMouseMove(30, rowY(m, 1))
	m.OnMouseMove(30, rowY(m, 0))
	StepAnimations(time.Now().Add(250 * time.Millisecond))
	if c, i := m.openChildOf(); c == nil || i != 0 {
		t.Error("после возврата на «Создать» подменю должно раскрыться одним шагом")
	}
}
