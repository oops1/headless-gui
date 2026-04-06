//go:build windows

package widget

// detectedTitleStyle возвращает стиль заголовка, соответствующий текущей ОС.
// На Windows — Windows-стиль (текст слева, кнопки справа).
func detectedTitleStyle() WindowTitleStyle {
	return WindowTitleWin
}

// detectedOS возвращает строковое имя текущей ОС.
func detectedOS() string {
	return "windows"
}
