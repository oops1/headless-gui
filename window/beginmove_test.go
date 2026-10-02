package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Системное перемещение и ресайз подключались сами только там, где корень —
// widget.Window. Приложению со своим корнем (Проводник, Блокнот) приходилось
// утверждать тип над Native() на неэкспортируемый интерфейс — то есть лезть
// во внутренности движка.

// moverStub — нативное окно, умеющее системное перемещение.
type moverStub struct {
	NativeWindow
	moved   int
	resized []int
	answer  bool
}

func (m *moverStub) BeginMove() bool {
	m.moved++
	return m.answer
}

func (m *moverStub) BeginResize(edges int) bool {
	m.resized = append(m.resized, edges)
	return m.answer
}

// plainStub — бэкенд без этой способности (Win32, X11, macOS).
type plainStub struct{ NativeWindow }

func TestWindow_BeginMove(t *testing.T) {
	mv := &moverStub{answer: true}
	win := &Window{}
	win.native = mv

	if !win.BeginMove() {
		t.Error("BeginMove вернул false, хотя бэкенд согласился")
	}
	if mv.moved != 1 {
		t.Errorf("бэкенд спросили %d раз, ждал один", mv.moved)
	}

	// Отказ системы — не ошибка: приложение подвинет окно само.
	mv.answer = false
	if win.BeginMove() {
		t.Error("BeginMove вернул true при отказе бэкенда")
	}
}

func TestWindow_BeginResize(t *testing.T) {
	mv := &moverStub{answer: true}
	win := &Window{}
	win.native = mv

	edges := widget.NativeEdgeBottom | widget.NativeEdgeRight
	if !win.BeginResize(edges) {
		t.Error("BeginResize вернул false")
	}
	if len(mv.resized) != 1 || mv.resized[0] != edges {
		t.Errorf("бэкенду передали края %v, ждал %d", mv.resized, edges)
	}
}

// Бэкенд, который так не умеет, отвечает честным false — и приложение
// двигает окно прежним путём.
func TestWindow_BeginMoveUnsupported(t *testing.T) {
	win := &Window{}
	win.native = &plainStub{}

	if win.BeginMove() {
		t.Error("BeginMove вернул true на бэкенде без этой способности")
	}
	if win.BeginResize(widget.NativeEdgeTop) {
		t.Error("BeginResize вернул true на бэкенде без этой способности")
	}
}

// До Run окна ОС ещё нет — просить некого, но и падать не на чем.
func TestWindow_BeginMoveBeforeRun(t *testing.T) {
	win := &Window{}
	if win.BeginMove() || win.BeginResize(widget.NativeEdgeLeft) {
		t.Error("без нативного окна методы ответили true")
	}
}
