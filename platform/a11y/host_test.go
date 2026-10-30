package a11y

import (
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// testWin — окно для тестов мостов: то, что мост видит у окна ядра (движок,
// заголовок, масштаб, окно ОС, решение SetAccessibilityEnabled). Имена — как у
// window/core.Window, чтобы тесты, переехавшие из ядра на 9.8, остались
// прежними; мосту окно отдаётся хостом host(), как ядро отдаёт a11yHost.
type testWin struct {
	// eng — движок как его видит ядро (EngineAPI): тесты спрашивают у него
	// необязательные методы утверждением типа.
	eng       any
	e         *engine.Engine
	title     string
	scale     float64
	native    native.NativeWindow
	a11yForce *bool
}

// New — окно поверх движка, как window/core.New (масштаб до bringUp — 0).
func New(eng *engine.Engine, title string) *testWin {
	return &testWin{eng: eng, e: eng, title: title}
}

// newTestWindow — окно поверх нового движка с корнем root.
func newTestWindow(root widget.Widget) *testWin {
	eng := engine.New(400, 300, 30)
	eng.SetRoot(root)
	return New(eng, "")
}

func (w *testWin) host() native.AccessHost { return testHost{w} }

func (w *testWin) accessibilitySnapshot() *a11ySnapshot { return accessibilitySnapshot(w.host()) }

func (w *testWin) accessibilityNodeAction(n *a11yNode, k widget.AccessActionKind) bool {
	return accessibilityNodeAction(w.host(), n, k)
}

func (w *testWin) accessibilityFocus(wd widget.Widget) bool { return w.host().FocusAccessible(wd) }
func (w *testWin) accessibilityActivate(wd widget.Widget) bool {
	return w.host().ActivateAccessible(wd)
}

// testHost — native.AccessHost тестового окна (как a11yHost ядра).
type testHost struct{ w *testWin }

var _ native.AccessHost = testHost{}

func (h testHost) AccessibilityTree() *widget.AccessNode { return h.w.e.AccessibilityTree() }

func (h testHost) FocusAccessible(wd widget.Widget) bool {
	return wd != nil && h.w.e.FocusAccessible(wd)
}

func (h testHost) ActivateAccessible(wd widget.Widget) bool {
	return wd != nil && h.w.e.ActivateAccessible(wd)
}

func (h testHost) AccessAction(wd widget.Widget, id string, k widget.AccessActionKind) bool {
	return h.w.e.AccessAction(wd, id, k)
}

func (h testHost) PostToEngine(fn func())      { h.w.e.Post(fn) }
func (h testHost) Native() native.NativeWindow { return h.w.native }
func (h testHost) Scale() float64              { return h.w.scale }
func (h testHost) Title() string               { return h.w.title }

func (h testHost) AccessibilityForced() (on, set bool) {
	if f := h.w.a11yForce; f != nil {
		return *f, true
	}
	return false, false
}

// winOf — тестовое окно моста.
func winOf(h native.AccessHost) *testWin { return h.(testHost).w }
