package native

// systheme.go — тема оформления, выбранная в ОС.

// SystemTheme — тема оформления, выбранная в ОС.
type SystemTheme int

const (
	// SystemThemeUnknown — узнать не удалось: ОС без понятия темы (Windows до
	// 10 1607), на Linux нет ни GTK_THEME, ни портала рабочего стола, macOS
	// и window/minimal (службы оболочки platform/shell темы там не дают).
	// Это не «светлая»: приложение вправе взять
	// свою тему по умолчанию, и именно об этом ему и сообщается — подставить
	// светлую молча значило бы показать белое окно человеку, у которого
	// просто не работает портал.
	SystemThemeUnknown SystemTheme = iota
	// SystemThemeLight — светлая тема.
	SystemThemeLight
	// SystemThemeDark — тёмная тема.
	SystemThemeDark
)

// String — имя темы для журналов и сообщений об ошибках.
func (t SystemTheme) String() string {
	switch t {
	case SystemThemeLight:
		return "light"
	case SystemThemeDark:
		return "dark"
	}
	return "unknown"
}
