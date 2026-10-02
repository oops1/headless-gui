//go:build windows

package window

// doubleclick_windows.go — системный интервал двойного щелчка.
//
// Его задаёт человек в параметрах мыши, и ровно он решает, считать ли второе
// нажатие двойным щелчком. Пока движок этого значения не знал, каждый виджет
// высматривал двойной щелчок сам, со своим порогом в 400 мс.

import "time"

var procGetDoubleClickTime = user32.NewProc("GetDoubleClickTime")

// nativeDoubleClickTime возвращает системный интервал двойного щелчка.
// 0 — узнать не удалось: тогда у движка остаётся своё значение.
func nativeDoubleClickTime() time.Duration {
	ms, _, _ := procGetDoubleClickTime.Call()
	if ms == 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}
