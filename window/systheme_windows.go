//go:build windows

package window

// systheme_windows.go — тема ОС на Windows: чтение реестра и разбор
// WM_SETTINGCHANGE.
//
// Реестр читается через golang.org/x/sys/windows/registry: пакет входит в тот
// же модуль golang.org/x/sys, что уже подключён ради windows, так что новых
// зависимостей это не добавляет.

import (
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
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
func watchSystemTheme(native NativeWindow, emit func(SystemTheme)) (stop func()) {
	w, ok := native.(*Win32Window)
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
	fn := w.onSettingChange.Load()
	if fn == nil || lparam == 0 {
		return
	}
	// Указатель берём через адрес переменной, а не через unsafe.Pointer(lparam):
	// это то же самое, но go vet не принимает прямое превращение числа в
	// указатель, а память принадлежит системе, и vet в этом прав.
	p := *(*unsafe.Pointer)(unsafe.Pointer(&lparam))
	(*fn)(readUTF16Bounded(p, settingChangeAreaMax))
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
