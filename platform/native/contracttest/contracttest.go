// Package contracttest — общий набор сценариев для бэкендов окна (ADR-0013 §7,
// T4): контракт NativeWindow и его необязательных возможностей проверяется
// одними и теми же сценариями на каждом бэкенде. Бэкенд даёт окружение (Env):
// как создать окно, как прокачать его события, что он умеет; сценарии — здесь
// и обращаются к окну только через открытые методы.
//
// Пакет — для тестов бэкендов (импортирует testing); программы его не
// импортируют.
package contracttest

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/keyboard"
	"github.com/oops1/headless-gui/v4/platform/native"
)

// Env — окружение сценариев одного бэкенда.
type Env struct {
	// New — окно бэкенда, ещё не созданное: Create зовёт сценарий. Зовётся на
	// горутине сценария; бэкенду, которому окно нужно на закреплённом потоке
	// (Win32), здесь его и закрепить (открепить — t.Cleanup).
	New func(t *testing.T) native.NativeWindow
	// Pump прокачивает события окна, пока cond не станет true или не выйдет
	// timeout; ответ — cond() в конце.
	Pump func(cond func() bool, timeout time.Duration) bool
	// Destroy уничтожает окно после сценария.
	Destroy func(nw native.NativeWindow)
	// Started — окно создано (nil — ничего не нужно): бэкенду со своим
	// насосом событий на соединение окна (X11, Wayland) — запустить его.
	Started func(nw native.NativeWindow)

	// MinSizeEnforced — бэкенд держит минимальный размер и при программной
	// смене размера (SetSize меньше минимума даёт минимум).
	MinSizeEnforced bool
	// Requests — Activate, Maximize и SetFullscreen — просьбы оконному
	// менеджеру или компоновщику (X11, Wayland): ответ асинхронный, и тот
	// вправе отказать. Окно, состояние которого вовсе не изменилось, —
	// пропуск, а не ошибка; изменившееся не так — ошибка. Win32 — false:
	// там true от Activate значит «окно активно».
	Requests bool
	// ToggleLockKeys — сценарию можно на миг переключить Scroll Lock (и
	// вернуть): клавиатура настоящая.
	ToggleLockKeys bool
	// LockKeysSetOptional — переключать переключатели бэкенд умеет не везде
	// (Wayland — только у компоновщика с zwp_virtual_keyboard_manager_v1):
	// отказ SetLockKeys тогда — «здесь не умею», и сценарий пропускается.
	LockKeysSetOptional bool
	// Style — какие поля стиля бэкенд умеет (§4): остальные ApplyStyle
	// обязан честно отвечать false.
	Style native.Style
}

// Run гоняет все сценарии.
func Run(t *testing.T, env Env) {
	t.Run("CreateSize", func(t *testing.T) { createSize(t, env) })
	t.Run("SetSizeBeforeCreate", func(t *testing.T) { setSizeBeforeCreate(t, env) })
	t.Run("SetSizeAfterCreate", func(t *testing.T) { setSizeAfterCreate(t, env) })
	t.Run("MinSize", func(t *testing.T) { minSize(t, env) })
	t.Run("MaximizeRestore", func(t *testing.T) { maximizeRestore(t, env) })
	t.Run("Fullscreen", func(t *testing.T) { fullscreen(t, env) })
	t.Run("Activation", func(t *testing.T) { activation(t, env) })
	t.Run("LockKeys", func(t *testing.T) { lockKeys(t, env) })
	t.Run("Style", func(t *testing.T) { style(t, env) })
}

const wait = 3 * time.Second

// created — новое окно бэкенда, созданное размером w×h; уничтожается после
// сценария.
func created(t *testing.T, env Env, w, h int) native.NativeWindow {
	t.Helper()
	nw := env.New(t)
	if err := nw.Create("contract", w, h); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	started(t, env, nw)
	return nw
}

// started — окно создано: насос событий и уничтожение после сценария.
func started(t *testing.T, env Env, nw native.NativeWindow) {
	if env.Started != nil {
		env.Started(nw)
	}
	t.Cleanup(func() { env.Destroy(nw) })
}

// bigger — окно сейчас больше w×h по обеим сторонам.
func bigger(nw native.NativeWindow, w, h int) bool {
	gw, gh := nw.GetSize()
	return gw > w && gh > h
}

// sizeIs — размер окна сейчас w×h.
func sizeIs(nw native.NativeWindow, w, h int) func() bool {
	return func() bool { gw, gh := nw.GetSize(); return gw == w && gh == h }
}

// Окно создаётся заказанного размера.
func createSize(t *testing.T, env Env) {
	nw := created(t, env, 320, 200)
	if !env.Pump(sizeIs(nw, 320, 200), wait) {
		w, h := nw.GetSize()
		t.Errorf("после Create(320, 200) размер %d×%d", w, h)
	}
}

// До Create GetSize отвечает заказанным размером, а размер окна задаёт Create.
func setSizeBeforeCreate(t *testing.T, env Env) {
	nw := env.New(t)
	nw.SetSize(360, 240)
	if w, h := nw.GetSize(); w != 360 || h != 240 {
		t.Errorf("до Create после SetSize(360, 240) размер %d×%d", w, h)
	}
	if err := nw.Create("contract", 300, 200); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	started(t, env, nw)
	if !env.Pump(sizeIs(nw, 300, 200), wait) {
		w, h := nw.GetSize()
		t.Errorf("Create(300, 200) после SetSize: размер %d×%d", w, h)
	}
}

// SetSize после показа меняет размер, и о нём узнаёт SetOnResize.
func setSizeAfterCreate(t *testing.T, env Env) {
	nw := created(t, env, 320, 200)
	env.Pump(sizeIs(nw, 320, 200), wait)
	var last image.Point
	nw.SetOnResize(func(w, h int) { last = image.Pt(w, h) })
	nw.SetSize(420, 260)
	if !env.Pump(func() bool { return sizeIs(nw, 420, 260)() && last == image.Pt(420, 260) }, wait) {
		w, h := nw.GetSize()
		t.Errorf("SetSize(420, 260): размер %d×%d, SetOnResize получил %v", w, h, last)
	}
}

// Размер не меньше минимального (если бэкенд держит его и при программной
// смене размера), а больше минимума — тот, что заказан.
func minSize(t *testing.T, env Env) {
	nw := created(t, env, 520, 360)
	nw.SetMinSize(480, 320)
	nw.SetSize(600, 400)
	if !env.Pump(sizeIs(nw, 600, 400), wait) {
		w, h := nw.GetSize()
		t.Errorf("SetSize(600, 400) при минимуме 480×320: размер %d×%d", w, h)
	}
	if !env.MinSizeEnforced {
		return
	}
	nw.SetSize(200, 150)
	if !env.Pump(sizeIs(nw, 480, 320), wait) {
		w, h := nw.GetSize()
		t.Errorf("SetSize(200, 150) при минимуме 480×320: размер %d×%d, ждал минимум", w, h)
	}
}

// Развернуть и восстановить: IsMaximized отвечает правду, восстановленное окно
// возвращается к прежнему размеру.
func maximizeRestore(t *testing.T, env Env) {
	nw := created(t, env, 400, 300)
	env.Pump(sizeIs(nw, 400, 300), wait)
	nw.Maximize()
	if !env.Pump(func() bool { return nw.IsMaximized() && bigger(nw, 400, 300) }, wait) {
		if env.Requests && !nw.IsMaximized() && sizeIs(nw, 400, 300)() {
			t.Skip("оконный менеджер не развернул окно")
		}
		w, h := nw.GetSize()
		t.Fatalf("после Maximize: развёрнуто %v, размер %d×%d — ждал больше нормального 400×300", nw.IsMaximized(), w, h)
	}
	nw.Restore()
	if !env.Pump(func() bool { return !nw.IsMaximized() && sizeIs(nw, 400, 300)() }, wait) {
		if env.Requests && nw.IsMaximized() {
			t.Skip("оконный менеджер не восстановил окно")
		}
		w, h := nw.GetSize()
		t.Errorf("после Restore: развёрнуто %v, размер %d×%d, ждал 400×300", nw.IsMaximized(), w, h)
	}
}

// fullscreener — полный экран (window/core.fullscreenBackend).
type fullscreener interface {
	SetFullscreen(on bool) bool
	IsFullscreen() bool
}

// Полный экран и выход: окно становится больше и возвращается к прежнему
// размеру; заказанный в полном экране размер достаётся окну при выходе.
func fullscreen(t *testing.T, env Env) {
	nw := created(t, env, 400, 300)
	fs, ok := nw.(fullscreener)
	if !ok {
		t.Skip("бэкенд полного экрана не умеет")
	}
	env.Pump(sizeIs(nw, 400, 300), wait)
	if !fs.SetFullscreen(true) {
		t.Fatal("SetFullscreen(true) отказал")
	}
	if !env.Pump(func() bool { return fs.IsFullscreen() && bigger(nw, 400, 300) }, wait) {
		if env.Requests && !fs.IsFullscreen() && sizeIs(nw, 400, 300)() {
			t.Skip("оконный менеджер не перевёл окно в полный экран")
		}
		w, h := nw.GetSize()
		t.Fatalf("после входа: полный экран %v, размер %d×%d — ждал больше нормального 400×300", fs.IsFullscreen(), w, h)
	}
	if nw.IsMaximized() {
		t.Error("IsMaximized true у полноэкранного окна, не бывшего развёрнутым")
	}
	nw.SetSize(440, 320)
	if !fs.SetFullscreen(false) {
		t.Fatal("SetFullscreen(false) отказал")
	}
	if !env.Pump(func() bool { return !fs.IsFullscreen() && sizeIs(nw, 440, 320)() }, wait) {
		if env.Requests && fs.IsFullscreen() {
			t.Skip("оконный менеджер не вернул окно из полного экрана")
		}
		w, h := nw.GetSize()
		t.Errorf("после выхода: полный экран %v, размер %d×%d, ждал заказанный в полном экране 440×320", fs.IsFullscreen(), w, h)
	}
}

// activator — вывести окно вперёд (native.ActivateBackend) и узнать о фокусе
// (window/core.activationNotifier).
type activator interface {
	Activate() bool
	SetOnActivate(fn func(active bool))
}

// Активация: если ОС дала окну передний план, последнее, что сообщил
// SetOnActivate, — «активно». Колбэк ставится до Create, как его ставит ядро:
// о первой активации Win32 сообщает ещё при создании окна, и успешный
// Activate уже активного окна второго уведомления не шлёт.
func activation(t *testing.T, env Env) {
	nw := env.New(t)
	a, ok := nw.(activator)
	if !ok {
		t.Skip("бэкенд активации не умеет")
	}
	var active, seen bool
	a.SetOnActivate(func(on bool) { active, seen = on, true })
	if err := nw.Create("contract", 320, 200); err != nil {
		t.Skipf("окно не создаётся (нет рабочего стола?): %v", err)
	}
	started(t, env, nw)
	if !a.Activate() {
		t.Skip("ОС не дала окну передний план (фоновый процесс)")
	}
	if !env.Pump(func() bool { return seen && active }, wait) {
		if env.Requests {
			t.Skipf("запрос активации отправлен, но фокус окну не передан (SetOnActivate: вызван %v, активно %v)", seen, active)
		}
		t.Errorf("Activate принят, а SetOnActivate: вызван %v, активно %v", seen, active)
	}
}

// lockKeyer — переключатели клавиатуры (window/core.lockKeysBackend и
// lockKeysSetter).
type lockKeyer interface {
	LockKeys() (keyboard.LockKeys, bool)
	SetLockKeys(want keyboard.LockKeys) bool
}

// lockKeysSettler — состояние переключателей приходит с фокусом окна (X11,
// Wayland; window/core.lockKeysSettler).
type lockKeysSettler interface{ LockKeysSettled() bool }

// Переключатели: созданное окно знает их состояние; заказ того же состояния —
// успех без перемен; Scroll Lock переключается и возвращается.
func lockKeys(t *testing.T, env Env) {
	nw := created(t, env, 320, 200)
	lk, ok := nw.(lockKeyer)
	if !ok {
		t.Skip("бэкенд переключателей не знает")
	}
	if s, ok := nw.(lockKeysSettler); ok && !env.Pump(s.LockKeysSettled, wait) {
		t.Skip("состояние переключателей приходит с фокусом, а фокус окну не передан")
	}
	var cur keyboard.LockKeys
	if !env.Pump(func() bool { var ok bool; cur, ok = lk.LockKeys(); return ok }, wait) {
		t.Fatal("созданное окно не знает состояния переключателей")
	}
	if !lk.SetLockKeys(cur) {
		if env.LockKeysSetOptional {
			t.Skipf("состояние известно (%v), а переключать его бэкенд здесь не умеет", cur)
		}
		t.Errorf("SetLockKeys(%v) — того же состояния — отказал", cur)
	}
	if got, _ := lk.LockKeys(); got != cur {
		t.Errorf("заказ того же состояния изменил его: было %v, стало %v", cur, got)
	}
	if !env.ToggleLockKeys {
		return
	}
	want := cur ^ keyboard.ScrollLock
	t.Cleanup(func() {
		if now, _ := lk.LockKeys(); now&keyboard.ScrollLock != cur&keyboard.ScrollLock {
			lk.SetLockKeys(now ^ keyboard.ScrollLock)
			env.Pump(func() bool { now, _ := lk.LockKeys(); return now&keyboard.ScrollLock == cur&keyboard.ScrollLock }, wait)
		}
	})
	if !lk.SetLockKeys(want) {
		t.Fatalf("SetLockKeys(%v) отказал", want)
	}
	if !env.Pump(func() bool { got, _ := lk.LockKeys(); return got == want }, wait) {
		got, _ := lk.LockKeys()
		t.Errorf("после SetLockKeys(%v) состояние %v", want, got)
	}
}

// Стиль (§4): ApplyStyle отвечает стилем, который действует после вызова, —
// заказанное и умеемое включается, неумеемое честно false, сброс — нулевой
// стиль.
func style(t *testing.T, env Env) {
	nw := created(t, env, 320, 200)
	st, ok := nw.(native.Styler)
	if !ok {
		t.Skip("бэкенд стиля не умеет")
	}
	for _, req := range []native.Style{{Borderless: true}, {Tool: true}, {Borderless: true, Tool: true}} {
		want := native.Style{
			Borderless: req.Borderless && env.Style.Borderless,
			Tool:       req.Tool && env.Style.Tool,
		}
		if got := st.ApplyStyle(req); got != want {
			t.Errorf("ApplyStyle(%+v) = %+v, ждал %+v", req, got, want)
		}
		if got := st.ApplyStyle(native.Style{}); got != (native.Style{}) {
			t.Errorf("сброс стиля после %+v: %+v", req, got)
		}
	}
}
