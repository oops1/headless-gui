//go:build linux && !android

package window

import (
	"os/exec"
	"strings"
)

// localeProvider для Linux реализован через setxkbmap (X11/XKB).
//
// Ограничение: чтение АКТИВНОЙ группы раскладки без XKB-привязок ненадёжно,
// поэтому CurrentLocaleCode возвращает "" (слежение за системной комбинацией
// в реальном времени недоступно). Зато список раскладок и переключение из
// контекстного меню работают: ActivateLocaleCode вызывает setxkbmap, что меняет
// раскладку всей сессии.

// CurrentLocaleCode — активная раскладка. На Linux без XKB-привязок недоступна,
// возвращаем "" (начальная локаль берётся из первого доступного layout).
func (w *X11Window) CurrentLocaleCode() string {
	return ""
}

// AvailableLocaleCodes возвращает раскладки из `setxkbmap -query` (строка layout:).
func (w *X11Window) AvailableLocaleCodes() []string {
	out, err := exec.Command("setxkbmap", "-query").Output()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "layout:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, "layout:"))
		var codes []string
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				codes = append(codes, strings.ToUpper(p))
			}
		}
		return codes
	}
	return nil
}

// ActivateLocaleCode переключает раскладку X-сессии через setxkbmap.
func (w *X11Window) ActivateLocaleCode(code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return false
	}
	return exec.Command("setxkbmap", code).Run() == nil
}
