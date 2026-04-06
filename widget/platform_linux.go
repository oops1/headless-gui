//go:build linux && !android

package widget

// detectedTitleStyle возвращает стиль заголовка, соответствующий текущей ОС.
// На Linux — Windows-стиль (наиболее распространённый в GTK/Qt).
func detectedTitleStyle() WindowTitleStyle {
	return WindowTitleWin
}

// detectedOS возвращает строковое имя текущей ОС.
func detectedOS() string {
	return "linux"
}
