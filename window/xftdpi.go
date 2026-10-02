package window

// xftdpi.go — чистая часть масштаба интерфейса на X11: разбор базы ресурсов
// X-сервера.
//
// Файл без платформенного суффикса намеренно, как waylandwire.go: соединение
// с X-сервером есть только под Linux, а разбор текста проверяется на любой
// машине и идёт в общем прогоне тестов.

import (
	"os"
	"strconv"
	"strings"
)

// xftScaleFromResources достаёт Xft.dpi из базы ресурсов X и переводит в
// масштаб.
//
// База ресурсов — текст построчно «имя:<tab>значение»; имён там десятки, и
// нужное может стоять где угодно. Значение бывает дробным (144.5), поэтому
// читается как число, а не как целое.
func xftScaleFromResources(res string) (float64, bool) {
	for _, line := range strings.Split(res, "\n") {
		name, val, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != "Xft.dpi" {
			continue
		}
		dpi, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil || dpi <= 0 {
			return 0, false
		}
		return clampScale(dpi / 96), true
	}
	return 0, false
}

// clampScale держит масштаб в разумных пределах.
//
// Те же границы, что у HEADLESS_GUI_SCALE: мусор в ресурсах (Xft.dpi: 1
// вместо 96) не должен превращать интерфейс в точку или в стену.
func clampScale(k float64) float64 {
	if k < 0.5 {
		return 0.5
	}
	if k > 4 {
		return 4
	}
	return k
}

// envGDKScale читает GDK_SCALE — целый масштаб, которым пользуются приложения
// GTK там, где базы ресурсов нет.
func envGDKScale() float64 {
	v := strings.TrimSpace(os.Getenv("GDK_SCALE"))
	if v == "" {
		return 0
	}
	k, err := strconv.ParseFloat(v, 64)
	if err != nil || k <= 0 {
		return 0
	}
	return clampScale(k)
}
