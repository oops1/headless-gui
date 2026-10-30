//go:build windows

package shell

// systheme_windows.go — тема ОС на Windows: чтение реестра и разбор
// WM_SETTINGCHANGE. Сообщение приходит окну бэкенда; обработчик пакет ставит
// через win32.AddMessageHook (register_windows.go), а подписки держит сам —
// по окну (*win32.Win32Window), так что они работают и до создания окна ОС.
//
// Реестр читается через golang.org/x/sys/windows/registry: пакет входит в тот
// же модуль golang.org/x/sys, что уже подключён ради windows, так что новых
// зависимостей это не добавляет.

import (
	"image/color"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/win32"
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
func detectSystemTheme() native.SystemTheme {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.QUERY_VALUE)
	if err != nil {
		return native.SystemThemeUnknown
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(appsUseLightTheme)
	if err != nil {
		return native.SystemThemeUnknown
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
// (watchSystemTheme): обе получают одно сообщение, и снятие одной другую не
// трогает.
func watchSystemAccent(nw native.NativeWindow, emit func(color.RGBA, bool)) (stop func()) {
	return watchSetting(nw, func(s *settingSubs) map[uint64]func(string) { return s.accent }, func(area string) {
		if isImmersiveColorSet(area) {
			emit(detectSystemAccent())
		}
	})
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
func watchSystemTheme(nw native.NativeWindow, emit func(native.SystemTheme)) (stop func()) {
	return watchSetting(nw, func(s *settingSubs) map[uint64]func(string) { return s.theme }, func(area string) {
		if isImmersiveColorSet(area) {
			emit(detectSystemTheme())
		}
	})
}

// wmSettingChange — WM_SETTINGCHANGE.
const wmSettingChange = 0x001A

// settingSubs — подписки окна на WM_SETTINGCHANGE, темы и акцента. У каждой
// подписки свой номер, и stop снимает только её, как у хаба портала на Linux:
// две одновременные подписки одного окна (ядро поднимает её и из bringUp, и из
// SetOn* приложения, а лишнюю снимает) не стирают друг друга.
type settingSubs struct {
	theme, accent map[uint64]func(area string)
}

// settingWins — подписки по окну. Ключ — само окно, а не hwnd: подписку
// просят и до Create, когда дескриптора ещё нет. Всё — под settingMu.
var (
	settingMu   sync.Mutex
	settingSeq  uint64
	settingWins = map[*win32.Win32Window]*settingSubs{}
)

// watchSetting добавляет fn в подписки окна nw (slot выбирает тему или
// акцент); stop снимает её одну, а окно без подписок забывается. nil — nw не
// окно Win32.
func watchSetting(nw native.NativeWindow, slot func(*settingSubs) map[uint64]func(string), fn func(area string)) (stop func()) {
	w, ok := nw.(*win32.Win32Window)
	if !ok || w == nil {
		return nil
	}
	settingMu.Lock()
	defer settingMu.Unlock()
	s := settingWins[w]
	if s == nil {
		s = &settingSubs{theme: map[uint64]func(string){}, accent: map[uint64]func(string){}}
		settingWins[w] = s
	}
	settingSeq++
	id := settingSeq
	slot(s)[id] = fn
	return func() {
		settingMu.Lock()
		defer settingMu.Unlock()
		delete(slot(s), id)
		if len(s.theme) == 0 && len(s.accent) == 0 && settingWins[w] == s {
			delete(settingWins, w)
		}
	}
}

// onSettingChange — обработчик WM_SETTINGCHANGE бэкенда (win32.AddMessageHook):
// передаёт строку подписчикам окна и всегда отвечает handled=false —
// сообщение идёт дальше (мониторы бэкенда по SPI_SETWORKAREA,
// DefWindowProc).
func onSettingChange(hwnd, _, lparam uintptr) (uintptr, bool) {
	w := win32.WindowOf(hwnd)
	if w == nil {
		return 0, false
	}
	settingMu.Lock()
	s := settingWins[w]
	settingMu.Unlock()
	if s != nil {
		s.handle(lparam)
	}
	return 0, false
}

// handle передаёт подписчикам строку из lParam сообщения WM_SETTINGCHANGE:
// сначала теме, потом акценту. Подписчиков берём снимком под замком, а зовём
// без него — подписчик вправе снять подписку из колбэка.
//
// Строку читаем только когда подписчик есть: lParam у этого сообщения то
// указатель на текст, то ноль, а у части системных параметров — не текст
// вовсе, и приложению, которому тема не нужна, незачем разыменовывать то, что
// ему не принадлежит.
func (s *settingSubs) handle(lparam uintptr) {
	settingMu.Lock()
	fns := make([]func(string), 0, len(s.theme)+len(s.accent))
	for _, fn := range s.theme {
		fns = append(fns, fn)
	}
	for _, fn := range s.accent {
		fns = append(fns, fn)
	}
	settingMu.Unlock()
	if len(fns) == 0 || lparam == 0 {
		return
	}
	// Указатель берём через адрес переменной, а не через unsafe.Pointer(lparam):
	// это то же самое, но go vet не принимает прямое превращение числа в
	// указатель, а память принадлежит системе, и vet в этом прав.
	p := *(*unsafe.Pointer)(unsafe.Pointer(&lparam))
	area := readUTF16Bounded(p, settingChangeAreaMax)
	for _, fn := range fns {
		fn(area)
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
