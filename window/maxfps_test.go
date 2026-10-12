package window

import "testing"

// fpsEngine — заглушка движка, принимающая потолок частоты кадров.
type fpsEngine struct {
	*stubEngine
	caps []int
}

func (e *fpsEngine) SetMaxFPS(fps int) { e.caps = append(e.caps, fps) }

// Window.SetMaxFPS доходит до движка окна (раньше менял только поле, которое
// читали модальные окна и док-хосты, и на главный движок не влиял).
func TestWindowSetMaxFPS_ReachesEngine(t *testing.T) {
	eng := &fpsEngine{stubEngine: newStubEngine()}
	win := New(eng, "t")
	if len(eng.caps) != 0 {
		t.Fatalf("New сам навязал движку частоту: %v — без вызова SetMaxFPS движок живёт на своей", eng.caps)
	}
	win.SetMaxFPS(30)
	if len(eng.caps) != 1 || eng.caps[0] != 30 {
		t.Fatalf("потолок не дошёл до движка: %v", eng.caps)
	}
	if win.maxFPS != 30 {
		t.Errorf("поле maxFPS = %d, ожидалось 30 (его читают модальные хосты)", win.maxFPS)
	}
}

// Движок без SetMaxFPS (EngineAPI не обязывает) вызов просто принимает.
func TestWindowSetMaxFPS_EngineWithoutSupport(t *testing.T) {
	win := New(newStubEngine(), "t")
	if win.SetMaxFPS(30) != win || win.maxFPS != 30 {
		t.Fatal("SetMaxFPS обязан вернуть окно и запомнить значение")
	}
}
