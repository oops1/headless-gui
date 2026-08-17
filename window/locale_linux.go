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
//
// Код раскладки валидируется перед запуском: setxkbmap разбирает аргументы
// сам, и строка, начинающаяся с «-», стала бы ФЛАГОМ («-option …» меняет
// XKB-конфигурацию всей сессии). Shell тут не участвует, но аргументная
// инъекция возможна и без него; источник кода — меню раскладок, однако
// защита должна держаться на границе, а не на доверии к вызывающему.
// Допускаются только имена вида «us», «ru», «de_ch», «pt_br» — латиница и
// подчёркивание, без ведущего дефиса и прочих символов.
func (w *X11Window) ActivateLocaleCode(code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	if !validLayoutCode(code) {
		return false
	}
	// «--» завершает разбор флагов: даже если валидация когда-нибудь ослабнет,
	// код раскладки останется позиционным аргументом.
	return exec.Command("setxkbmap", "--", code).Run() == nil
}

// validLayoutCode — имя раскладки XKB: 2–8 символов [a-z_], не начинается с
// подчёркивания и не пустое.
func validLayoutCode(code string) bool {
	if len(code) < 2 || len(code) > 8 || code[0] == '_' {
		return false
	}
	for i := 0; i < len(code); i++ {
		c := code[i]
		if (c < 'a' || c > 'z') && c != '_' {
			return false
		}
	}
	return true
}
