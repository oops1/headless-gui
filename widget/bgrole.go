// bgrole.go — роль фона у контейнеров раскладки: откуда брать цвет при смене
// темы.
//
// StackPanel и DockPanel тему не принимали вовсе: панель инструментов с
// непрозрачным фоном оставалась белой в тёмной теме, и приложению приходилось
// оборачивать панель ради одного ApplyTheme.
//
// «Непрозрачный фон — значит бери из темы», как у Label и Panel, здесь не
// годится: у контейнеров раскладки фон чаще всего задан СВОЙ (в документации
// пример с Background="#2D2D30"), и смена темы молча стирала бы его. А внутри
// диалога — сразу при показе: ShowModal применяет к нему текущую тему. Поэтому
// следовать теме панель начинает, только когда ей это сказали.
package widget

import (
	"image/color"
	"strings"
)

// BackgroundRole — откуда контейнер берёт фон при смене темы.
type BackgroundRole int

const (
	// BackgroundCustom — фон такой, как задан (прежнее поведение): тема его не
	// трогает.
	BackgroundCustom BackgroundRole = iota
	// BackgroundPanel — фон панелей темы (Theme.PanelBG): панель инструментов,
	// строка состояния.
	BackgroundPanel
	// BackgroundWindow — фон окна (Theme.WindowBG): основная область.
	BackgroundWindow
)

// themeBackground — цвет роли в теме; false — роль тему не читает.
func themeBackground(role BackgroundRole, t *Theme) (color.RGBA, bool) {
	switch role {
	case BackgroundPanel:
		return t.PanelBG, true
	case BackgroundWindow:
		return t.WindowBG, true
	}
	return color.RGBA{}, false
}

// currentThemeBackground — цвет роли в ТЕКУЩЕЙ теме: нужен, чтобы роль,
// назначенная после применения темы, не ждала следующей смены.
func currentThemeBackground(role BackgroundRole) (color.RGBA, bool) {
	t := CurrentTheme()
	return themeBackground(role, t)
}

// parseBackgroundRole разбирает значение атрибута Background="{Theme PanelBG}".
//
// Имена — те же, что у полей Theme: разметку пишут, глядя на тему, и
// изобретать для неё свои слова значило бы держать ещё один словарь.
func parseBackgroundRole(s string) (BackgroundRole, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return BackgroundCustom, false
	}
	f := strings.Fields(strings.Trim(s, "{}"))
	if len(f) != 2 || !strings.EqualFold(f[0], "Theme") {
		return BackgroundCustom, false
	}
	switch strings.ToLower(f[1]) {
	case "panelbg":
		return BackgroundPanel, true
	case "windowbg":
		return BackgroundWindow, true
	}
	return BackgroundCustom, false
}

// ─── StackPanel ─────────────────────────────────────────────────────────────

// SetBackgroundRole задаёт, откуда панель берёт фон, и сразу применяет цвет
// из текущей темы.
//
//	toolbar.SetBackgroundRole(widget.BackgroundPanel)
//
// В разметке — Background="{Theme PanelBG}" или "{Theme WindowBG}".
func (sp *StackPanel) SetBackgroundRole(r BackgroundRole) {
	sp.BackgroundRole = r
	if c, ok := currentThemeBackground(r); ok {
		sp.Background = c
		sp.UseAlpha = c.A < 255
	}
	sp.Invalidate()
}

// ─── DockPanel ──────────────────────────────────────────────────────────────

// SetBackgroundRole задаёт, откуда панель берёт фон, и сразу применяет цвет
// из текущей темы. В разметке — Background="{Theme WindowBG}".
func (dp *DockPanel) SetBackgroundRole(r BackgroundRole) {
	dp.BackgroundRole = r
	if c, ok := currentThemeBackground(r); ok {
		dp.Background = c
		dp.UseAlpha = c.A < 255
	}
	dp.Invalidate()
}
