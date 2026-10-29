//go:build windows

package core

// systheme_windows.go — тема ОС на Windows: чтение реестра и разбор
// WM_SETTINGCHANGE.
//
// Реестр читается через golang.org/x/sys/windows/registry: пакет входит в тот
// же модуль golang.org/x/sys, что уже подключён ради windows, так что новых
// зависимостей это не добавляет.

import (
	"image/color"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/oops1/headless-gui/v3/platform/native"
)

const (
	// personalizeKey — раздел, где Windows 10 и 11 хранят выбор светлой или
	// тёмной темы. Лежит в ветке пользователя: тема у каждого своя, а запись
	// туда не требует прав администратора, поэтому читать можно всегда.
	personalizeKey = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`

	// appsUseLightTheme — тема ОКОН ПРИЛОЖЕНИЙ. Соседний SystemUsesLightTheme
	// относится к панели задач и меню «Пуск» и расходится с первым, когда
	// человек выбрал «Свой» режим: берёшь не тот — получаешь светлое окно на
	// тёмной системе.
	appsUseLightTheme = "AppsUseLightTheme"

	// dwmKey — раздел DWM пользователя: там Windows хранит акцентный цвет.
	// AccentColor — DWORD ABGR, выбранный в параметрах «Цвета»;
	// ColorizationColor (ARGB) — запасной, его пишут и старые версии.
	dwmKey               = `Software\Microsoft\Windows\DWM`
	dwmAccentColor       = "AccentColor"
	dwmColorizationColor = "ColorizationColor"

	// settingChangeAreaMax — сколько знаков строки lParam читаем. Нужная нам
	// строка («ImmersiveColorSet») втрое короче; предел нужен, чтобы чужое
	// сообщение с мусором в lParam не уводило чтение далеко за строку.
	settingChangeAreaMax = 64
)

// detectSystemTheme читает AppsUseLightTheme.
//
// Нет раздела или значения — Unknown: Windows до 10 1607 тёмной темы не знает
// вовсе, а сборки вроде Server Core обходятся без оформления. Считать это
// «светлой» нельзя по той же причине, что и на других системах: см.
// SystemThemeUnknown.
func detectSystemTheme() SystemTheme {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.QUERY_VALUE)
	if err != nil {
		return SystemThemeUnknown
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(appsUseLightTheme)
	if err != nil {
		return SystemThemeUnknown
	}
	return systemThemeFromAppsUseLight(v)
}

// detectSystemAccent читает акцентный цвет пользователя из реестра. Нет
// раздела или значений (Windows до 8, Server Core) — ok=false.
func detectSystemAccent() (color.RGBA, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, dwmKey, registry.QUERY_VALUE)
	if err != nil {
		return color.RGBA{}, false
	}
	defer k.Close()
	if v, _, err := k.GetIntegerValue(dwmAccentColor); err == nil {
		return accentFromDWMAccent(uint32(v)), true
	}
	if v, _, err := k.GetIntegerValue(dwmColorizationColor); err == nil {
		return accentFromColorization(uint32(v)), true
	}
	return color.RGBA{}, false
}

// watchSystemAccent подписывается на смену акцентного цвета. Тот же
// WM_SETTINGCHANGE "ImmersiveColorSet", что и у темы: Windows относит оба
// параметра к одной группе, так что отдельного сообщения для акцента нет, а
// значение перечитывается из реестра. Подписка независима от подписки на тему
// (watchSystemTheme): у каждой своя ячейка окна.
func watchSystemAccent(nw native.NativeWindow, emit func(color.RGBA, bool)) (stop func()) {
	w, ok := nw.(*Win32Window)
	if !ok {
		return nil
	}
	fn := func(area string) {
		if isImmersiveColorSet(area) {
			emit(detectSystemAccent())
		}
	}
	w.onAccentChange.Store(&fn)
	return func() { w.onAccentChange.Store(nil) }
}

// watchSystemTheme подписывается на смену темы через WM_SETTINGCHANGE окна.
//
// Отдельного сообщения «тема сменилась» в Windows нет: приходит общее
// WM_SETTINGCHANGE со строкой ImmersiveColorSet, а что именно сменилось —
// светлая тема на тёмную или акцентный цвет, — по нему не понять. Поэтому тему
// мы перечитываем из реестра, а отсев повторов делает вызывающий (themeEdge).
//
// Своя подписка на реестр (RegNotifyChangeKeyValue) не нужна и была бы
// хуже: ей потребовалась бы отдельная горутина и событие, тогда как окно и так
// получает сообщение в своём цикле.
func watchSystemTheme(nw native.NativeWindow, emit func(SystemTheme)) (stop func()) {
	w, ok := nw.(*Win32Window)
	if !ok {
		return nil
	}
	fn := func(area string) {
		if isImmersiveColorSet(area) {
			emit(detectSystemTheme())
		}
	}
	w.onSettingChange.Store(&fn)
	return func() { w.onSettingChange.Store(nil) }
}

// handleSettingChange передаёт подписчику строку из lParam сообщения
// WM_SETTINGCHANGE.
//
// Строку читаем только когда подписчик есть: lParam у этого сообщения то
// указатель на текст, то ноль, а у части системных параметров — не текст
// вовсе, и приложению, которому тема не нужна, незачем разыменовывать то, что
// ему не принадлежит.
func (w *Win32Window) handleSettingChange(lparam uintptr) {
	fn, fa := w.onSettingChange.Load(), w.onAccentChange.Load()
	if (fn == nil && fa == nil) || lparam == 0 {
		return
	}
	// Указатель берём через адрес переменной, а не через unsafe.Pointer(lparam):
	// это то же самое, но go vet не принимает прямое превращение числа в
	// указатель, а память принадлежит системе, и vet в этом прав.
	p := *(*unsafe.Pointer)(unsafe.Pointer(&lparam))
	area := readUTF16Bounded(p, settingChangeAreaMax)
	if fn != nil {
		(*fn)(area)
	}
	if fa != nil {
		(*fa)(area)
	}
}

// readUTF16Bounded читает строку UTF-16 с нулём на конце, но не больше max
// знаков.
func readUTF16Bounded(p unsafe.Pointer, max int) string {
	buf := make([]uint16, 0, max)
	for i := 0; i < max; i++ {
		u := *(*uint16)(unsafe.Add(p, i*2))
		if u == 0 {
			break
		}
		buf = append(buf, u)
	}
	return windows.UTF16ToString(buf)
}
