//go:build !windows && !darwin && !(linux && !android)

package widget

import "runtime"

// detectedTitleStyle возвращает стиль заголовка для неизвестных платформ.
// Используется runtime.GOOS как fallback.
func detectedTitleStyle() WindowTitleStyle {
	if runtime.GOOS == "darwin" {
		return WindowTitleMac
	}
	return WindowTitleWin
}

// detectedOS возвращает строковое имя текущей ОС.
func detectedOS() string {
	return runtime.GOOS
}
