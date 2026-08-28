package widget

import (
	"image/color"
	"reflect"
	"testing"
)

// ApplyGlobalTheme переносит палитру в глобальные цвета поимённо, строка за
// строкой. Забытое поле не ломает сборку и ничем себя не выдаёт: тема его
// задаёт, а виджеты берут ноль. Так уже случилось с OutlineDragFill —
// цвет контура перетаскивания из темы не доезжал вовсе.
//
// Тест сверяет перенос по всем полям через рефлексию: следующее добавленное
// поле либо попадёт в ApplyGlobalTheme, либо будет названо здесь.
func TestApplyGlobalTheme_CoversEveryColorField(t *testing.T) {
	saved := win10
	defer func() { win10 = saved }()

	src := &Theme{}
	v := reflect.ValueOf(src).Elem()
	typ := v.Type()
	rgba := reflect.TypeOf(color.RGBA{})

	// Каждому полю — свой цвет, чтобы перепутанные местами присваивания
	// были видны так же, как забытые.
	n := 0
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type != rgba {
			continue
		}
		n++
		v.Field(i).Set(reflect.ValueOf(color.RGBA{
			R: uint8(n*3 + 1), G: uint8(n*5 + 2), B: uint8(n*7 + 3), A: 255,
		}))
	}
	if n < 70 {
		t.Fatalf("нашлось всего %d цветовых полей — тест смотрит не туда", n)
	}

	ApplyGlobalTheme(src)

	got := reflect.ValueOf(win10)
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type != rgba {
			continue
		}
		want := v.Field(i).Interface()
		if have := got.Field(i).Interface(); have != want {
			t.Errorf("поле %s: тема задаёт %v, в глобальной палитре %v — перенос пропущен",
				f.Name, want, have)
		}
	}
}
