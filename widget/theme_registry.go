package widget

import (
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
)

var (
	defaultThemeOnce sync.Once
	defaultTheme     *theme.Manager
)

// DefaultThemeManager возвращает менеджер тем, в котором уже зарегистрированы
// все встроенные пресеты — те же, что отдаёт ThemeNames.
//
// Мост работает в обе стороны, и приложению не нужно выбирать сторону сразу:
// старый путь (ThemeByName + Engine.SetTheme) продолжает работать, а этот
// менеджер даёт те же темы в новой модели — с наследованием, покомпонентными
// правилами и подпиской на смену. Профиль можно унаследовать от встроенного
// и переопределить десяток токенов, не переписывая палитру.
//
// Менеджер один на процесс — как и активная тема, которую он держит.
// Приложению, которому нужен свой набор (несколько окон с разным
// оформлением), стоит завести собственный theme.NewManager.
func DefaultThemeManager() *theme.Manager {
	defaultThemeOnce.Do(func() {
		m := theme.NewManager()
		for _, name := range ThemeNames() {
			t := ThemeByName(name)
			if t == nil {
				continue
			}
			// Ошибка регистрации здесь означала бы пустое имя у встроенного
			// пресета — этого не бывает, реестр пресетов задан константами.
			_ = m.RegisterTheme(ProfileFromTheme(t))
		}
		defaultTheme = m
	})
	return defaultTheme
}

// ThemeFromProfile собирает плоскую тему по имени профиля из менеджера.
// Возвращает nil, если такого профиля нет или его не удалось разрешить
// (не зарегистрирован родитель, цикл наследования).
func ThemeFromProfile(m *theme.Manager, name string) *Theme {
	if m == nil {
		return nil
	}
	rt, ok := m.GetTheme(name)
	if !ok {
		return nil
	}
	return Materialize(rt)
}
