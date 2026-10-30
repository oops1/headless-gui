package core

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/platform/native"
)

// Без служб оболочки (window/minimal, тестовый бинарь ядра) ничего не
// открывается, язык не задан, тема неизвестна, но проверка входа прежняя:
// плохая ссылка — ErrOpenScheme, а не «не поддержано».
func TestShell_NoCapability(t *testing.T) {
	if _, ok := native.ShellServices(); ok {
		t.Skip("службы оболочки подключены в этом бинаре")
	}
	if err := OpenURL("https://example.com"); !errors.Is(err, ErrOpenUnsupported) {
		t.Errorf("OpenURL без оболочки: %v, ждал ErrOpenUnsupported", err)
	}
	if err := OpenURL("javascript:alert(1)"); !errors.Is(err, ErrOpenScheme) {
		t.Errorf("плохая ссылка без оболочки: %v, ждал ErrOpenScheme", err)
	}
	doc := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(doc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := OpenFile(doc); !errors.Is(err, ErrOpenUnsupported) {
		t.Errorf("OpenFile без оболочки: %v", err)
	}
	if err := RevealFile(doc); !errors.Is(err, ErrOpenUnsupported) {
		t.Errorf("RevealFile без оболочки: %v", err)
	}
	if l := SystemLanguages(); l != nil {
		t.Errorf("SystemLanguages без оболочки = %q", l)
	}
	if th := DetectSystemTheme(); th != native.SystemThemeUnknown {
		t.Errorf("DetectSystemTheme без оболочки = %v", th)
	}
	if _, ok := DetectSystemAccent(); ok {
		t.Error("DetectSystemAccent без оболочки ok")
	}
	if watchSystemTheme(nil, func(native.SystemTheme) {}) != nil || watchSystemAccent(nil, func(color.RGBA, bool) {}) != nil {
		t.Error("наблюдение без оболочки поднято")
	}
}

// С оболочкой ядро отдаёт ей проверенный вход и её ответы.
func TestShell_Dispatch(t *testing.T) {
	var calls []string
	boom := errors.New("нет браузера")
	t.Cleanup(native.RegisterShell(native.Shell{
		OpenURL:    func(u string) error { calls = append(calls, "url "+u); return nil },
		OpenFile:   func(p string) error { calls = append(calls, "file "+p); return nil },
		RevealFile: func(p string) error { calls = append(calls, "reveal "+p); return boom },
		Languages:  func() []string { return []string{"ru-RU", "en-US"} },
		Theme:      func() native.SystemTheme { return native.SystemThemeDark },
		Accent:     func() (color.RGBA, bool) { return color.RGBA{R: 1, A: 255}, true },
	}))
	if err := OpenURL("  https://example.com/x  "); err != nil {
		t.Fatal(err)
	}
	if err := OpenURL("javascript:alert(1)"); !errors.Is(err, ErrOpenScheme) {
		t.Errorf("плохая ссылка дошла до проверки иначе: %v", err)
	}
	dir := t.TempDir()
	doc := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(doc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := OpenFile(doc); err != nil {
		t.Fatal(err)
	}
	if err := RevealFile(doc); !errors.Is(err, boom) {
		t.Errorf("ошибка оболочки потерялась: %v", err)
	}
	want := []string{"url https://example.com/x", "file " + doc, "reveal " + doc}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("вызовы оболочки %q, ждал %q", calls, want)
	}
	if l := SystemLanguages(); !reflect.DeepEqual(l, []string{"ru-RU", "en-US"}) {
		t.Errorf("SystemLanguages = %q", l)
	}
	if a := DetectSystemAppearance(); a.Theme != native.SystemThemeDark || !a.HasAccent || a.Accent.R != 1 {
		t.Errorf("DetectSystemAppearance = %+v", a)
	}
}

// Окно подписывается на смену темы и акцента через службы оболочки: им
// отдаётся окно ОС, смена доходит до колбэка приложения, подписка одна на оба
// колбэка и живёт, пока задан хотя бы один из них (в любом порядке снятия),
// а снимается ровно один раз.
func TestShell_WindowWatchWired(t *testing.T) {
	for _, appFirst := range []bool{false, true} {
		var (
			seen, seenA    native.NativeWindow
			emitT          func(native.SystemTheme)
			emitA          func(color.RGBA, bool)
			watchT, watchA int
			stopsT, stopsA int
		)
		restore := native.RegisterShell(native.Shell{
			Theme:  func() native.SystemTheme { return native.SystemThemeLight },
			Accent: func() (color.RGBA, bool) { return color.RGBA{}, false },
			WatchTheme: func(nw native.NativeWindow, emit func(native.SystemTheme)) func() {
				watchT++
				seen, emitT = nw, emit
				return func() { stopsT++ }
			},
			WatchAccent: func(nw native.NativeWindow, emit func(color.RGBA, bool)) func() {
				watchA++
				seenA, emitA = nw, emit
				return func() { stopsA++ }
			},
		})
		win, _ := appearanceTestWin()
		nw := &fakeCloseNative{}
		win.native = nw
		var themes []native.SystemTheme
		var looks []SystemAppearance
		win.SetOnSystemThemeChanged(func(th native.SystemTheme) { themes = append(themes, th) })
		win.SetOnSystemAppearanceChanged(func(a SystemAppearance) { looks = append(looks, a) })
		if seen != native.NativeWindow(nw) || seenA != native.NativeWindow(nw) || emitT == nil || emitA == nil {
			t.Fatalf("подписка не дошла до оболочки: окно темы %v, окно акцента %v, тема %v, акцент %v", seen, seenA, emitT != nil, emitA != nil)
		}
		if watchT != 1 || watchA != 1 || stopsT != 0 || stopsA != 0 {
			t.Fatalf("после двух колбэков: подписок %d/%d, снятий %d/%d; ждал одну живую", watchT, watchA, stopsT, stopsA)
		}
		emitT(native.SystemThemeDark)
		emitA(color.RGBA{B: 9, A: 255}, true)
		if len(themes) != 1 || themes[0] != native.SystemThemeDark {
			t.Errorf("колбэк темы получил %v", themes)
		}
		if len(looks) == 0 || !looks[len(looks)-1].HasAccent || looks[len(looks)-1].Accent.B != 9 {
			t.Errorf("колбэк оформления получил %+v", looks)
		}
		first, second := func() { win.SetOnSystemThemeChanged(nil) }, func() { win.SetOnSystemAppearanceChanged(nil) }
		if appFirst {
			first, second = second, first
		}
		first()
		if stopsT != 0 || stopsA != 0 {
			t.Errorf("снят один колбэк (оформление первым: %v), а подписка снята: %d/%d", appFirst, stopsT, stopsA)
		}
		second()
		if stopsT != 1 || stopsA != 1 || watchT != 1 || watchA != 1 {
			t.Errorf("сняты оба колбэка (оформление первым: %v): подписок %d/%d, снятий %d/%d; ждал по одной", appFirst, watchT, watchA, stopsT, stopsA)
		}
		restore()
	}
}

// Подписка не переживает окно: после закрытия (tearDown) SetOn* её не
// поднимает, поднятая в гонке с закрытием или с неудачным созданием окна
// снимается сразу — иначе снять её было бы некому, и обработчик оболочки
// держал бы окно с движком до конца процесса; новый bringUp (повторный Run
// после неудачи) подписку снова разрешает.
func TestShell_WatchNotAfterClose(t *testing.T) {
	var (
		watches, stops int
		midway         func() // зовётся посреди подъёма подписки
	)
	t.Cleanup(native.RegisterShell(native.Shell{
		Theme: func() native.SystemTheme { return native.SystemThemeLight },
		WatchTheme: func(native.NativeWindow, func(native.SystemTheme)) func() {
			watches++
			if f := midway; f != nil {
				midway = nil
				f()
			}
			return func() { stops++ }
		},
	}))
	fn := func(native.SystemTheme) {}

	win, _ := initBringUp(t, "1", func(win *Window) { win.SetOnSystemThemeChanged(fn) })
	if watches != 1 || stops != 0 {
		t.Fatalf("bringUp: подписок %d, снятий %d; ждал одну живую", watches, stops)
	}
	win.tearDown()
	if stops != 1 {
		t.Fatalf("tearDown не снял подписку: снятий %d", stops)
	}
	win.SetOnSystemThemeChanged(nil)
	win.SetOnSystemThemeChanged(fn)
	win.SetOnSystemAppearanceChanged(func(SystemAppearance) {})
	if watches != 1 {
		t.Fatalf("после закрытия окна подписка поднята заново: подписок %d", watches)
	}

	// Окно закрывается, пока подписка поднимается (SetOn* с горутины
	// приложения, tearDown — с потока окна).
	win2, _ := initBringUp(t, "1", nil)
	midway = win2.tearDown
	win2.SetOnSystemThemeChanged(fn)
	if watches != 2 || stops != 2 {
		t.Fatalf("поднятая в гонке с закрытием подписка не снята: подписок %d, снятий %d", watches, stops)
	}

	// Окно ОС не создалось, а подписку успели поднять после выбора бэкенда.
	f := &initFake{}
	var win3 *Window
	old := newNativeWindow
	newNativeWindow = func() native.NativeWindow {
		return failCreateFake{initFake: f, create: func() { win3.SetOnSystemThemeChanged(fn) }}
	}
	t.Cleanup(func() { newNativeWindow = old })
	win3 = New(engine.New(100, 100, 30), "fail")
	if err := win3.bringUp(); err == nil {
		t.Fatal("bringUp при неудачном Create не вернул ошибку")
	}
	if watches != 3 || stops != 3 {
		t.Fatalf("подписка окна, которое не создалось, не снята: подписок %d, снятий %d", watches, stops)
	}
	win3.SetOnSystemAppearanceChanged(func(SystemAppearance) {})
	if watches != 3 {
		t.Fatalf("подписка поднята на окне, которое не создалось: подписок %d", watches)
	}

	// Повторный Run после неудачи: окно ОС создаётся, подписка снова есть.
	newNativeWindow = func() native.NativeWindow { return &initFake{} }
	if err := win3.bringUp(); err != nil {
		t.Fatal(err)
	}
	if watches != 4 || stops != 3 {
		t.Fatalf("повторный bringUp: подписок %d, снятий %d; ждал новую живую", watches, stops)
	}
	win3.tearDown()
	if stops != 4 {
		t.Errorf("tearDown не снял подписку повторного bringUp: снятий %d", stops)
	}
}

// failCreateFake — бэкенд, у которого окно ОС не создаётся; create зовётся
// посреди Create.
type failCreateFake struct {
	*initFake
	create func()
}

func (f failCreateFake) Create(string, int, int) error {
	f.create()
	return errors.New("окно ОС не создаётся")
}
