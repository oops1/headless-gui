package core

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v4/output"
	"github.com/oops1/headless-gui/v4/platform/keyboard"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/pointer"
	"github.com/oops1/headless-gui/v4/widget"
)

// v3.39: сырой скан-код у клавиши, клавиши вне таблицы VK, сырое колесо,
// отпускание зажатых клавиш при потере активности, уход указателя. Всё, что не
// зависит от ОС, проверяется здесь через surface с заглушками окна и движка;
// разбор проводов X11/Wayland и сообщений Win32 — в платформенных файлах.

// wheelCall — сырое или пиксельное колесо, как его видит приложение (тесты
// трёх бэкендов).
type wheelCall struct {
	x, y    int
	dx, dy  float64
	precise bool
}

// ─── Заглушки ───────────────────────────────────────────────────────────────

// rkEngine — движок-заглушка: очередь Post выполняется по требованию теста,
// а всё, что дошло до движка, записывается строками.
type rkEngine struct {
	mu    sync.Mutex
	queue []func()
	log   []string
	keys  []widget.KeyEvent

	rawResult bool // что вернёт SendMouseWheelRaw («принято ли»)
	root      widget.Widget
}

func (e *rkEngine) Post(fn func()) {
	e.mu.Lock()
	e.queue = append(e.queue, fn)
	e.mu.Unlock()
}

// run выполняет очередь (в том числе то, что поставят сами задания).
func (e *rkEngine) run() {
	for {
		e.mu.Lock()
		if len(e.queue) == 0 {
			e.mu.Unlock()
			return
		}
		fn := e.queue[0]
		e.queue = e.queue[1:]
		e.mu.Unlock()
		fn()
	}
}

func (e *rkEngine) add(s string) { e.log = append(e.log, s) }

func (e *rkEngine) taken() []string {
	out := e.log
	e.log = nil
	return out
}

func (e *rkEngine) takenKeys() []widget.KeyEvent {
	out := e.keys
	e.keys = nil
	return out
}

func (e *rkEngine) Frames() <-chan output.Frame { return nil }
func (e *rkEngine) CanvasSize() (int, int)      { return 100, 100 }
func (e *rkEngine) Root() widget.Widget         { return e.root }
func (e *rkEngine) SendMouseMove(x, y int)      { e.add(fmt.Sprintf("move %d,%d", x, y)) }
func (e *rkEngine) SendMouseButton(x, y int, b widget.MouseButton, p bool) {
	e.add(fmt.Sprintf("button %d,%d %d %v", x, y, b, p))
}
func (e *rkEngine) SendKeyEvent(k widget.KeyEvent) { e.keys = append(e.keys, k) }
func (e *rkEngine) SetModifiers(m widget.KeyMod)   { e.add(fmt.Sprintf("mods %d", m)) }
func (e *rkEngine) CursorAt(x, y int) widget.Cursor {
	return widget.CursorArrow
}
func (e *rkEngine) SendMouseWheelPixels(x, y int, dx, dy float64) {
	e.add(fmt.Sprintf("pixels %d,%d %.1f,%.1f", x, y, dx, dy))
}
func (e *rkEngine) SendMouseWheelRaw(x, y int, dx, dy float64, precise bool) bool {
	e.add(fmt.Sprintf("raw %d,%d %.1f,%.1f precise=%v", x, y, dx, dy, precise))
	return e.rawResult
}
func (e *rkEngine) SendPointerLeave() { e.add("leave") }

// rkNative — окно ОС со всеми необязательными способностями этой темы.
type rkNative struct {
	native.NativeWindow
	onRaw      func(k keyboard.RawKey)
	onWheelRaw func(x, y int, dx, dy float64, precise bool, legacy func())
	onPixels   func(x, y int, dx, dy float64)
	onMove     func(x, y int)
	onChar     func(r rune)
	onActivate func(active bool)
}

func (f *rkNative) SetOnRawKey(fn func(k keyboard.RawKey)) { f.onRaw = fn }
func (f *rkNative) SetOnWheelRaw(fn func(x, y int, dx, dy float64, precise bool, legacy func())) {
	f.onWheelRaw = fn
}
func (f *rkNative) SetOnMouseWheelPixels(fn func(x, y int, dx, dy float64)) { f.onPixels = fn }
func (f *rkNative) SetOnMouseMove(fn func(x, y int))                        { f.onMove = fn }
func (f *rkNative) SetOnMouseButton(func(x, y, b int, p bool))              {}
func (f *rkNative) SetOnKeyDown(func(vk int))                               {}
func (f *rkNative) SetOnKeyUp(func(vk int))                                 {}
func (f *rkNative) SetOnChar(fn func(r rune))                               { f.onChar = fn }
func (f *rkNative) SetOnActivate(fn func(active bool))                      { f.onActivate = fn }
func (f *rkNative) SetOnClose(func() bool)                                  {}

func rkSurface(t *testing.T) (*surface, *rkNative, *rkEngine) {
	t.Helper()
	eng := &rkEngine{}
	fn := &rkNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()
	return s, fn, eng
}

// ─── Чистые функции ─────────────────────────────────────────────────────────

func TestVKToKeyCode_NewKeys(t *testing.T) {
	cases := map[int]widget.KeyCode{
		native.VK_BROWSER_BACK:     widget.KeyBrowserBack,
		native.VK_BROWSER_HOME:     widget.KeyBrowserHome,
		native.VK_VOLUME_MUTE:      widget.KeyVolumeMute,
		native.VK_VOLUME_UP:        widget.KeyVolumeUp,
		native.VK_MEDIA_PLAY_PAUSE: widget.KeyMediaPlayPause,
		native.VK_LAUNCH_MAIL:      widget.KeyLaunchMail,
		native.VK_LAUNCH_APP2:      widget.KeyLaunchApp2,
		native.VK_KANA:             widget.KeyKana,
		native.VK_KANJI:            widget.KeyKanji,
		native.VK_CONVERT:          widget.KeyConvert,
		native.VK_NONCONVERT:       widget.KeyNonConvert,
		native.VK_SLEEP:            widget.KeySleep,
		native.VK_CLEAR:            widget.KeyClear,
		native.VK_PROCESSKEY:       widget.KeyIMEProcess,
	}
	for vk, want := range cases {
		if got := vkToKeyCode(vk); got != want {
			t.Errorf("vkToKeyCode(0x%02X) = %d, ждал %d", vk, got, want)
		}
		if int(want) != vk {
			t.Errorf("значение KeyCode %d разошлось с VK 0x%02X: ряд кодов Windows нарушен", want, vk)
		}
	}
	// Новые коды не затронули прежние значения и не пересеклись с ними.
	if widget.KeyAlt != 18 || widget.KeyPause != 19 || widget.KeyCapsLock != 20 || widget.KeyF12 != 123 {
		t.Error("прежние KeyCode изменились")
	}
}

func TestX11KeycodeToVK_RareKeys(t *testing.T) {
	// keycode = evdev + 8.
	cases := map[int]int{
		183 + 8: native.VK_F13,
		194 + 8: native.VK_F24,
		113 + 8: native.VK_VOLUME_MUTE,
		115 + 8: native.VK_VOLUME_UP,
		164 + 8: native.VK_MEDIA_PLAY_PAUSE, // KEY_PLAYPAUSE
		163 + 8: native.VK_MEDIA_NEXT_TRACK,
		165 + 8: native.VK_MEDIA_PREV_TRACK,
		92 + 8:  native.VK_CONVERT,    // KEY_HENKAN
		94 + 8:  native.VK_NONCONVERT, // KEY_MUHENKAN
		93 + 8:  native.VK_KANA,       // KEY_KATAKANAHIRAGANA
		122 + 8: native.VK_KANA,       // KEY_HANGEUL
		123 + 8: native.VK_KANJI,      // KEY_HANJA
		158 + 8: native.VK_BROWSER_BACK,
		155 + 8: native.VK_LAUNCH_MAIL,
		142 + 8: native.VK_SLEEP,
	}
	for kc, want := range cases {
		if got := keyboard.X11KeycodeToVK(kc); got != want {
			t.Errorf("keycode %d: %#x, ждал %#x", kc, got, want)
		}
	}
	// Сквозной путь до widget.KeyCode.
	if got := vkToKeyCode(keyboard.X11KeycodeToVK(186 + 8)); got != widget.KeyF16 {
		t.Errorf("evdev 186 дошёл как %d, ждал KeyF16", got)
	}
}

// ─── Клавиши через surface ──────────────────────────────────────────────────

func rkRun(s *surface, eng *rkEngine, fn func()) []widget.KeyEvent {
	fn()
	eng.run()
	eng.taken()
	return eng.takenKeys()
}

// Скан-код и признак расширенной клавиши доходят до движка вместе с кодом.
func TestRawKey_ScanCodeAndExtendedReachEngine(t *testing.T) {
	s, fn, eng := rkSurface(t)
	got := rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_A, Scan: 0x1E, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_RIGHT, Scan: 0x4D, Extended: true, Pressed: true, Repeat: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_RIGHT, Scan: 0x4D, Extended: true})
	})
	want := []widget.KeyEvent{
		{Code: widget.KeyA, Pressed: true, ScanCode: 0x1E},
		{Code: widget.KeyRight, Pressed: true, Repeat: true, ScanCode: 0x4D, Extended: true},
		{Code: widget.KeyRight, ScanCode: 0x4D, Extended: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
}

// Клавиша вне таблицы VK приходит с KeyUnknown и скан-кодом, а не пропадает.
func TestRawKey_UnnamedKeyReachesEngine(t *testing.T) {
	s, fn, eng := rkSurface(t)
	got := rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: 0, Scan: 0x7E, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: 0, Scan: 0x7E})
		// Безымянная и без скан-кода (например, VK_PACKET от SendInput) —
		// по-прежнему не событие.
		fn.onRaw(keyboard.RawKey{VK: 0xE7, Pressed: true})
	})
	want := []widget.KeyEvent{
		{Code: widget.KeyUnknown, Pressed: true, ScanCode: 0x7E},
		{Code: widget.KeyUnknown, ScanCode: 0x7E},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
	for _, k := range got {
		if k.Rune != 0 {
			t.Error("безымянная клавиша принесла руну")
		}
	}
}

// Бэкенд без скан-кодов ведёт себя как раньше: безымянные клавиши отброшены,
// ScanCode пуст, модификаторы событием не ходят.
func TestRawKey_LegacyPathUnchanged(t *testing.T) {
	s, rec := newAltSurface()
	s.keyEvent(0xE7, true)            // нет в таблице
	s.keyEvent(native.VK_SHIFT, true) // модификатор без скан-кода
	s.keyEvent(native.VK_CONTROL, true)
	s.keyEventRepeat(native.VK_A, true, true)
	want := []widget.KeyEvent{
		{Code: widget.KeyA, Mod: widget.ModShift | widget.ModCtrl, Pressed: true, Repeat: true},
	}
	if !reflect.DeepEqual(rec.keys, want) {
		t.Errorf("до движка дошло: %+v, ждал: %+v", rec.keys, want)
	}
}

// Модификаторы доходят как клавиши (со стороной через скан-код) и при этом
// по-прежнему меняют Mod следующих событий.
func TestRawKey_ModifiersAreKeys(t *testing.T) {
	s, fn, eng := rkSurface(t)
	got := rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_SHIFT, Scan: 0x2A, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_CONTROL, Scan: 0x1D, Extended: true, Pressed: true}) // правый Ctrl
		fn.onRaw(keyboard.RawKey{VK: native.VK_A, Scan: 0x1E, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_CONTROL, Scan: 0x1D, Extended: true})
	})
	want := []widget.KeyEvent{
		{Code: widget.KeyShift, Mod: widget.ModShift, Pressed: true, ScanCode: 0x2A},
		{Code: widget.KeyControl, Mod: widget.ModShift | widget.ModCtrl, Pressed: true, ScanCode: 0x1D, Extended: true},
		{Code: widget.KeyA, Mod: widget.ModShift | widget.ModCtrl, Pressed: true, ScanCode: 0x1E},
		{Code: widget.KeyControl, Mod: widget.ModShift, ScanCode: 0x1D, Extended: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
}

// Alt: удержание — клавиша KeyAltKey со скан-кодом, а жест «нажал-отпустил»
// KeyAlt остаётся тем же, что и был.
func TestRawKey_AltKeyAndTapGesture(t *testing.T) {
	s, fn, eng := rkSurface(t)
	got := rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_ALT, Scan: 0x38, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_ALT, Scan: 0x38})
	})
	want := []widget.KeyEvent{
		{Code: widget.KeyAltKey, Mod: widget.ModAlt, Pressed: true, ScanCode: 0x38},
		{Code: widget.KeyAltKey, ScanCode: 0x38},
		{Code: widget.KeyAlt, Pressed: true},
		{Code: widget.KeyAlt},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло:\n  %+v\nждал:\n  %+v", got, want)
	}

	// Alt+F — сочетание: жеста нет, но клавиши Alt пришли.
	got = rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_ALT, Scan: 0x38, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_F, Scan: 0x21, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_F, Scan: 0x21})
		fn.onRaw(keyboard.RawKey{VK: native.VK_ALT, Scan: 0x38})
	})
	for _, k := range got {
		if k.Code == widget.KeyAlt {
			t.Errorf("после сочетания пришёл жест Alt: %+v", got)
		}
	}
	if len(got) != 4 {
		t.Errorf("событий %d, ждал 4 (Alt, F, F, Alt): %+v", len(got), got)
	}
}

// Текстовый ввод идёт как прежде: руна с KeyUnknown и без скан-кода.
func TestRawKey_TextInputUnbroken(t *testing.T) {
	s, fn, eng := rkSurface(t)
	got := rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_S, Scan: 0x1F, Pressed: true})
		fn.onChar('ы')
		fn.onChar('\x01') // управляющий символ
	})
	want := []widget.KeyEvent{
		{Code: widget.KeyS, Pressed: true, ScanCode: 0x1F},
		{Code: widget.KeyUnknown, Rune: 'ы', Pressed: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
}

// Автоповтор доходит вместе со скан-кодом, а в список зажатых не добавляется
// заново.
func TestRawKey_RepeatKeepsScan(t *testing.T) {
	s, fn, eng := rkSurface(t)
	got := rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_Z, Scan: 0x2C, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_Z, Scan: 0x2C, Pressed: true, Repeat: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_Z, Scan: 0x2C, Pressed: true, Repeat: true})
	})
	if len(got) != 3 || got[0].Repeat || !got[1].Repeat || !got[2].Repeat {
		t.Fatalf("пометки повтора: %+v", got)
	}
	for _, k := range got {
		if k.ScanCode != 0x2C {
			t.Errorf("повтор потерял скан-код: %+v", k)
		}
	}
	if len(s.held) != 1 {
		t.Errorf("в списке зажатых %d клавиш, ждал 1", len(s.held))
	}
}

// ─── Отпускание при потере активности ───────────────────────────────────────

// Ctrl+Shift+A зажаты, окно потеряло фокус: движок получает отпускания в
// порядке «обычные, потом модификаторы» с тем же Mod, с каким нажимали, и
// затем нулевые модификаторы. Жест Alt не срабатывает.
func TestReleaseHeldKeys_OnDeactivate(t *testing.T) {
	s, fn, eng := rkSurface(t)
	rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_CONTROL, Scan: 0x1D, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_SHIFT, Scan: 0x2A, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_ALT, Scan: 0x38, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_A, Scan: 0x1E, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: 0, Scan: 0x7E, Pressed: true}) // безымянная тоже зажата
	})

	s.releaseHeldKeys()
	eng.run()
	got := eng.takenKeys()
	log := eng.taken()

	all := widget.ModCtrl | widget.ModShift | widget.ModAlt
	want := []widget.KeyEvent{
		{Code: widget.KeyA, Mod: all, ScanCode: 0x1E},
		{Code: widget.KeyUnknown, Mod: all, ScanCode: 0x7E},
		{Code: widget.KeyControl, Mod: widget.ModShift | widget.ModAlt, ScanCode: 0x1D},
		{Code: widget.KeyShift, Mod: widget.ModAlt, ScanCode: 0x2A},
		{Code: widget.KeyAltKey, ScanCode: 0x38},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("отпускания:\n  %+v\nждал:\n  %+v", got, want)
	}
	if n := len(log); n == 0 || log[n-1] != "mods 0" {
		t.Errorf("движок остался с модификаторами: %q", log)
	}
	if s.currentMod() != 0 {
		t.Errorf("модификаторы окна не сброшены: %v", s.currentMod())
	}
	if len(s.held) != 0 {
		t.Errorf("список зажатых не пуст: %v", s.held)
	}

	// Повторный вызов ничего не отпускает, кроме нулевых модификаторов.
	s.releaseHeldKeys()
	eng.run()
	if k := eng.takenKeys(); len(k) != 0 {
		t.Errorf("повторная деактивация породила события клавиш: %+v", k)
	}
}

// Alt, зажатый один, при потере фокуса отпускается без жеста «открыть меню»:
// открывать его в окне, которое уже потеряло фокус, некому.
func TestReleaseHeldKeys_NoAltTapGesture(t *testing.T) {
	s, fn, eng := rkSurface(t)
	rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_ALT, Scan: 0x38, Pressed: true})
	})
	s.releaseHeldKeys()
	eng.run()
	got := eng.takenKeys()
	want := []widget.KeyEvent{{Code: widget.KeyAltKey, ScanCode: 0x38}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("отпускания: %+v, ждал: %+v", got, want)
	}
}

// Модификатор мог остаться выставленным без записи о клавише (нажатие
// проглотила система): деактивация всё равно обнуляет и окно, и движок.
func TestReleaseHeldKeys_StrayModifiersCleared(t *testing.T) {
	s, _, eng := rkSurface(t)
	s.modCtrl.Store(true)
	s.modMeta.Store(true)
	s.releaseHeldKeys()
	eng.run()
	if s.currentMod() != 0 {
		t.Errorf("модификаторы окна: %v", s.currentMod())
	}
	if got := eng.taken(); len(got) != 1 || got[0] != "mods 0" {
		t.Errorf("движку не сообщено об обнулении: %q", got)
	}
}

// Клавиша, отпущенная до потери фокуса, при деактивации не отпускается второй раз.
func TestReleaseHeldKeys_ReleasedKeyNotRepeated(t *testing.T) {
	s, fn, eng := rkSurface(t)
	rkRun(s, eng, func() {
		fn.onRaw(keyboard.RawKey{VK: native.VK_CONTROL, Scan: 0x1D, Pressed: true})
		fn.onRaw(keyboard.RawKey{VK: native.VK_CONTROL, Scan: 0x1D})
	})
	s.releaseHeldKeys()
	eng.run()
	if k := eng.takenKeys(); len(k) != 0 {
		t.Errorf("лишние отпускания: %+v", k)
	}
}

// Window.SetOnActivate: колбэк зовётся на горутине движка и только при смене;
// при потере активности сначала отпускаются клавиши.
func TestWindow_SetOnActivate(t *testing.T) {
	eng := &rkEngine{}
	fn := &rkNative{}
	win := &Window{}
	win.surface = surface{eng: eng, native: fn, scale: 1}
	win.setupInput()
	win.setupActivation()
	if fn.onActivate == nil {
		t.Fatal("окно не подписалось на активность бэкенда")
	}
	var seq []string
	win.SetOnActivate(func(active bool) {
		seq = append(seq, fmt.Sprintf("activate=%v keysHeld=%d", active, len(win.held)))
	})

	fn.onActivate(true)
	fn.onActivate(true) // Wayland шлёт состояние с каждым configure
	eng.run()
	if want := []string{"activate=true keysHeld=0"}; !reflect.DeepEqual(seq, want) {
		t.Fatalf("активация: %q, ждал %q", seq, want)
	}
	if !win.IsActive() {
		t.Error("IsActive == false после активации")
	}

	fn.onRaw(keyboard.RawKey{VK: native.VK_CONTROL, Scan: 0x1D, Pressed: true})
	eng.run()
	eng.takenKeys()
	seq = nil

	fn.onActivate(false)
	eng.run()
	if want := []string{"activate=false keysHeld=0"}; !reflect.DeepEqual(seq, want) {
		t.Fatalf("деактивация: %q, ждал %q", seq, want)
	}
	if win.IsActive() {
		t.Error("IsActive == true после деактивации")
	}
	keys := eng.takenKeys()
	if len(keys) != 1 || keys[0].Code != widget.KeyControl || keys[0].Pressed {
		t.Errorf("Ctrl не отпущен при деактивации: %+v", keys)
	}
	// Освобождение клавиш — до колбэка приложения (в очереди оно раньше).
	if win.currentMod() != 0 {
		t.Error("модификаторы остались после деактивации")
	}

	fn.onActivate(true)
	eng.run()
	if len(seq) != 2 || seq[1] != "activate=true keysHeld=0" {
		t.Errorf("повторная активация: %q", seq)
	}

	// nil отключает колбэк.
	win.SetOnActivate(nil)
	seq = nil
	fn.onActivate(false)
	eng.run()
	if len(seq) != 0 {
		t.Errorf("отключённый колбэк вызван: %q", seq)
	}
}

// ─── Колесо через surface ───────────────────────────────────────────────────

// Принятое виджетами сырое колесо не запускает прежний путь.
func TestWheelRaw_ConsumedSkipsLegacy(t *testing.T) {
	_, fn, eng := rkSurface(t)
	eng.rawResult = true
	legacyRan := false
	fn.onWheelRaw(10, 20, 0, 120, false, func() { legacyRan = true })
	eng.run()
	if legacyRan {
		t.Error("прежний путь запущен, хотя сырое колесо принято")
	}
	if got, want := eng.taken(), []string{"raw 10,20 0.0,120.0 precise=false"}; !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло %q, ждал %q", got, want)
	}
}

// Не принятое сырое колесо доходит прежним путём: пиксели получают виджеты,
// которые ничего не знают про WheelEvent.
func TestWheelRaw_UnconsumedFallsBackToPixels(t *testing.T) {
	_, fn, eng := rkSurface(t)
	eng.rawResult = false
	fn.onWheelRaw(10, 20, 0, 120, false, func() { fn.onPixels(10, 20, 0, 40) })
	eng.run()
	want := []string{"raw 10,20 0.0,120.0 precise=false", "pixels 10,20 0.0,40.0"}
	if got := eng.taken(); !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло %q, ждал %q", got, want)
	}
}

// Движок без сырого колеса: приёмник не ставится, бэкенд работает как раньше.
func TestWheelRaw_EngineWithoutRawKeepsLegacy(t *testing.T) {
	eng := &queueEngine{}
	fn := &rkNative{}
	s := &surface{eng: eng, native: fn, scale: 1}
	s.setupInput()
	if fn.onWheelRaw != nil {
		t.Error("приёмник сырого колеса поставлен движку, который его не принимает")
	}
}

// ─── Уход указателя ─────────────────────────────────────────────────────────

func TestPointerLeave_ReachesEngineAfterMove(t *testing.T) {
	_, fn, eng := rkSurface(t)
	fn.onMove(30, 40)
	eng.run()
	eng.taken()

	fn.onMove(pointer.Outside, pointer.Outside)
	eng.run()
	if got, want := eng.taken(), []string{"move -1,-1", "leave"}; !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло %q, ждал %q", got, want)
	}

	// Обычное движение «ушло» не порождает.
	fn.onMove(5, 5)
	eng.run()
	if got, want := eng.taken(), []string{"move 5,5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("до движка дошло %q, ждал %q", got, want)
	}
}
