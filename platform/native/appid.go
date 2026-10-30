package native

// appid.go — app_id / WM_CLASS программы по умолчанию.

import (
	"strings"
)

// DefaultAppID — имя исполняемого файла без пути и расширения.
//
// По app_id среда рабочего стола ищет <app_id>.desktop и значок программы.
// Общее имя движка ставило все программы на нём в одну кучу: панель задач не
// отличала проводник от калькулятора. Так же поступают GTK и Qt.
//
//facadegen:skip
func DefaultAppID(argv0 string) string {
	// Путь режем сами, а не filepath.Base: разделитель тут от ЧУЖОЙ системы
	// (argv0 из теста или из кросс-сборки), а filepath знает только про свою.
	base := strings.ReplaceAll(argv0, "\\", "/")
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".exe")
	if base == "" || base == "." {
		return "headless-gui"
	}
	return base
}
