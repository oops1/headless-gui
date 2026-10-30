package native

import (
	"testing"
)

func TestWin32InitialCreate(t *testing.T) {
	const style = 0x90000000 | 0x10000 // WS_POPUP|WS_VISIBLE|…
	const swShow = 5

	// Ничего не просили: центр, стиль и показ прежние.
	x, y, st, cmd := Win32InitialCreate(InitialPlacement{}, style, swShow, 560, 240)
	if x != 560 || y != 240 || st != style || cmd != swShow {
		t.Errorf("без заказа: %d,%d %#x %d", x, y, st, cmd)
	}

	// Положение вместо центра; отрицательные координаты второго монитора допустимы.
	x, y, st, cmd = Win32InitialCreate(InitialPlacement{PosSet: true, X: -1800, Y: 120}, style, swShow, 560, 240)
	if x != -1800 || y != 120 || st != style || cmd != swShow {
		t.Errorf("с положением: %d,%d %#x %d", x, y, st, cmd)
	}

	// Развёрнутое окно создаётся скрытым и первым показом разворачивается.
	x, y, st, cmd = Win32InitialCreate(InitialPlacement{Maximized: true}, style, swShow, 560, 240)
	if st&initWsVisible != 0 {
		t.Errorf("развёрнутое окно создаётся видимым: %#x", st)
	}
	if st != style&^initWsVisible {
		t.Errorf("кроме WS_VISIBLE стиль меняться не должен: %#x → %#x", style, st)
	}
	if cmd != 3 {
		t.Errorf("команда показа %d, ждал SW_SHOWMAXIMIZED (3)", cmd)
	}
	if x != 560 || y != 240 {
		t.Errorf("без заказа положения центр съехал: %d,%d", x, y)
	}
}

func TestX11SizeHints(t *testing.T) {
	le := func(b []byte, off int) uint32 {
		return uint32(b[off]) | uint32(b[off+1])<<8 | uint32(b[off+2])<<16 | uint32(b[off+3])<<24
	}
	// Положение и размер: USPosition|PPosition|USSize|PSize и значения в полях.
	b := X11SizeHints(0, 0, InitialPlacement{PosSet: true, SizeSet: true}, -40, 25, 640, 480)
	if len(b) != 72 {
		t.Fatalf("длина %d, ждал 72", len(b))
	}
	if got, want := le(b, 0), uint32(X11USPosition|X11PPosition|X11USSize|X11PSize); got != want {
		t.Errorf("флаги %#x, ждал %#x", got, want)
	}
	if int32(le(b, 4)) != -40 || le(b, 8) != 25 || le(b, 12) != 640 || le(b, 16) != 480 {
		t.Errorf("поля x,y,w,h: %d %d %d %d", int32(le(b, 4)), le(b, 8), le(b, 12), le(b, 16))
	}
	if le(b, 20) != 0 || le(b, 24) != 0 {
		t.Error("минимум появился без заказа")
	}

	// Только положение: размер не объявляется.
	b = X11SizeHints(0, 0, InitialPlacement{PosSet: true}, 5, 6, 640, 480)
	if got := le(b, 0); got != X11USPosition|X11PPosition {
		t.Errorf("флаги только положения %#x", got)
	}
	if le(b, 12) != 0 || le(b, 16) != 0 {
		t.Error("размер записан без SizeSet")
	}

	// Минимальный размер сосуществует с положением.
	b = X11SizeHints(320, 200, InitialPlacement{PosSet: true, SizeSet: true}, 1, 2, 640, 480)
	if got, want := le(b, 0), uint32(X11PMinSize|X11USPosition|X11PPosition|X11USSize|X11PSize); got != want {
		t.Errorf("флаги %#x, ждал %#x", got, want)
	}
	if le(b, 20) != 320 || le(b, 24) != 200 {
		t.Errorf("минимум %d×%d, ждал 320×200", le(b, 20), le(b, 24))
	}

	// Согласованность с прежней сборкой одного минимума (флаги — отдельно).
	if string(X11SizeHints(320, 200, InitialPlacement{}, 0, 0, 0, 0)[4:]) != string(x11NormalHintsMinSizeBytes(320, 200)[4:]) {
		t.Error("без заказа положения поля должны совпадать с прежним минимумом")
	}
	if got := le(X11SizeHints(320, 200, InitialPlacement{}, 0, 0, 0, 0), 0); got != X11PMinSize {
		t.Errorf("флаги одного минимума %#x", got)
	}
}

func TestWlNormalSize(t *testing.T) {
	// Окно начало развёрнутым, теперь восстановлено, компоновщик размера не назвал.
	if w, h, used := WlNormalSize(0, 0, false, false, 800, 600); w != 800 || h != 600 || !used {
		t.Errorf("0×0 после восстановления: %d×%d used=%v, ждал 800×600 true", w, h, used)
	}
	// Всё ещё развёрнуто — запомненный размер не трогаем.
	if w, h, used := WlNormalSize(1920, 1080, true, false, 800, 600); w != 1920 || h != 1080 || used {
		t.Errorf("развёрнутое: %d×%d used=%v", w, h, used)
	}
	// Полный экран — тоже.
	if w, h, used := WlNormalSize(0, 0, false, true, 800, 600); w != 0 || h != 0 || used {
		t.Errorf("полный экран: %d×%d used=%v", w, h, used)
	}
	// Компоновщик назвал размер сам — он важнее, запомненный больше не нужен.
	if w, h, used := WlNormalSize(900, 700, false, false, 800, 600); w != 900 || h != 700 || !used {
		t.Errorf("размер от компоновщика: %d×%d used=%v", w, h, used)
	}
	// Окно не начинало развёрнутым — ничего не меняется.
	if w, h, used := WlNormalSize(0, 0, false, false, 0, 0); w != 0 || h != 0 || used {
		t.Errorf("без запомненного: %d×%d used=%v", w, h, used)
	}
}
