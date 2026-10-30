package shell

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// registered — службы оболочки, которые положил в реестр init пакета
// (register_<ОС>.go): тесты идут через них тем же путём, что и ядро.
func registered(t *testing.T) native.Shell {
	t.Helper()
	s, ok := native.ShellServices()
	if !ok {
		t.Fatal("init пакета не зарегистрировал службы оболочки")
	}
	return s
}

// Набор служб по ОС (ADR-0013 §10, 9.8b; ожидание — wantShell в
// register_<ОС>_test.go), и в каждом поле своя реализация, а не соседняя с
// той же сигнатурой: перепутанные OpenFile и RevealFile открывали бы файл
// вместо Проводника.
func TestRegister_ShellServices(t *testing.T) {
	got, want := reflect.ValueOf(registered(t)), reflect.ValueOf(wantShell())
	for i := 0; i < got.NumField(); i++ {
		name := got.Type().Field(i).Name
		g, w := got.Field(i), want.Field(i)
		switch {
		case w.IsNil() && !g.IsNil():
			t.Errorf("%s: служба зарегистрирована, а на %s её быть не должно", name, runtime.GOOS)
		case !w.IsNil() && g.IsNil():
			t.Errorf("%s: служба не зарегистрирована", name)
		case !w.IsNil() && g.Pointer() != w.Pointer():
			t.Errorf("%s: в поле чужая реализация", name)
		}
	}
	if s := registered(t); s.Accent != nil {
		if c, ok := s.Accent(); ok && c.A != 255 {
			t.Errorf("акцент прозрачный: %v", c)
		}
	}
}
