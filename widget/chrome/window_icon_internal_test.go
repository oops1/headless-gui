package chrome

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// Подписи системного меню окна зарегистрированы при старте — для EN и RU, по
// всем шести ключам. (В пакете tests другие тесты зовут ClearStrings, поэтому
// здесь, в отдельном бинарнике, где никто таблицы не стирает.)
func TestWindowIcon_BuiltInStrings(t *testing.T) {
	keys := []string{"win.sys.restore", "win.sys.move", "win.sys.size",
		"win.sys.minimize", "win.sys.maximize", "win.sys.close"}
	for _, lang := range []string{"EN", "RU"} {
		for _, k := range keys {
			if got := base.TrIn(lang, k); got == k || got == "" {
				t.Errorf("%s: нет встроенной подписи для %q", lang, k)
			}
		}
	}
	if base.TrIn("RU", "win.sys.close") == base.TrIn("EN", "win.sys.close") {
		t.Error("русская и английская подписи «Закрыть» совпадают")
	}
}
