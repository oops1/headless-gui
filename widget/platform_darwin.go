//go:build darwin

package widget

// detectedTitleStyle возвращает стиль заголовка, соответствующий текущей ОС.
// На macOS — Mac-стиль (traffic lights слева, текст по центру).
func detectedTitleStyle() WindowTitleStyle {
	return WindowTitleMac
}

// detectedOS возвращает строковое имя текущей ОС.
func detectedOS() string {
	return "darwin"
}
