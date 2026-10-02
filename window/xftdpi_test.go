package window

import "testing"

// На Linux движок не спрашивал у системы масштаб интерфейса вовсе: на
// мониторе 4K всё выходило вдвое мельче положенного, и единственным способом
// это исправить была переменная HEADLESS_GUI_SCALE — то есть человек должен
// был знать о ней и выставлять руками.

func TestXftScaleFromResources(t *testing.T) {
	// Настоящая база ресурсов — десятки строк, и нужная стоит где придётся.
	const res = "*customization:\t-color\n" +
		"Xcursor.theme:\tAdwaita\n" +
		"Xft.antialias:\t1\n" +
		"Xft.dpi:\t192\n" +
		"Xft.hinting:\t1\n"

	k, ok := xftScaleFromResources(res)
	if !ok {
		t.Fatal("Xft.dpi не найден")
	}
	if k != 2 {
		t.Errorf("масштаб %v при 192 dpi, ждал 2", k)
	}
}

// Дробный масштаб — обычное дело: 144 dpi это 1.5, и округлять до двух
// нельзя, иначе интерфейс станет заметно крупнее заказанного.
func TestXftScaleFromResources_Fractional(t *testing.T) {
	k, ok := xftScaleFromResources("Xft.dpi:\t144\n")
	if !ok || k != 1.5 {
		t.Errorf("масштаб %v при 144 dpi, ждал 1.5", k)
	}
}

func TestXftScaleFromResources_Missing(t *testing.T) {
	if _, ok := xftScaleFromResources("Xft.antialias:\t1\n"); ok {
		t.Error("масштаб взялся там, где Xft.dpi нет")
	}
	if _, ok := xftScaleFromResources(""); ok {
		t.Error("масштаб взялся из пустой базы ресурсов")
	}
	if _, ok := xftScaleFromResources("Xft.dpi:\tнет\n"); ok {
		t.Error("нечисловое значение принято за масштаб")
	}
}

// Мусор в ресурсах (Xft.dpi: 1 вместо 96) не должен превращать интерфейс в
// точку или в стену.
func TestXftScaleFromResources_Clamped(t *testing.T) {
	if k, _ := xftScaleFromResources("Xft.dpi:\t1\n"); k < 0.5 {
		t.Errorf("масштаб %v — меньше нижней границы", k)
	}
	if k, _ := xftScaleFromResources("Xft.dpi:\t9600\n"); k > 4 {
		t.Errorf("масштаб %v — больше верхней границы", k)
	}
}

func TestEnvGDKScale(t *testing.T) {
	t.Setenv("GDK_SCALE", "2")
	if k := envGDKScale(); k != 2 {
		t.Errorf("GDK_SCALE=2 дал %v", k)
	}
	t.Setenv("GDK_SCALE", "")
	if k := envGDKScale(); k != 0 {
		t.Errorf("без переменной вернулось %v, ждал 0", k)
	}
	t.Setenv("GDK_SCALE", "мусор")
	if k := envGDKScale(); k != 0 {
		t.Errorf("мусор дал %v, ждал 0", k)
	}
}
