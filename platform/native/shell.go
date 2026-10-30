package native

// shell.go — возможность служб оболочки (ADR-0013 §3): открыть ссылку или
// файл назначенным в системе приложением, показать файл в файловом менеджере,
// языки системы, системная тема и акцентный цвет и их наблюдение. Живёт в
// platform/shell и регистрируется здесь; ядро окна проверяет вход (политика
// схем и файлов) и зовёт её, а без неё отвечает «не поддерживается».

import "image/color"

// Shell — службы оболочки. Поле nil — ОС этого не умеет (ядро отвечает так
// же, как без возможности).
//
//facadegen:skip
type Shell struct {
	// OpenURL, OpenFile и RevealFile получают уже проверенный вход: ссылку
	// разрешённой схемы, абсолютный существующий путь.
	OpenURL    func(url string) error
	OpenFile   func(path string) error
	RevealFile func(path string) error
	// Languages — языки интерфейса, выбранные в системе, по убыванию
	// предпочтения (теги BCP 47); nil — не задано.
	Languages func() []string
	// Theme и Accent — тема и акцентный цвет прямо сейчас.
	Theme  func() SystemTheme
	Accent func() (color.RGBA, bool)
	// WatchTheme и WatchAccent подписывают окно nw на смену темы и акцента;
	// stop снимает подписку, nil — наблюдения нет. emit зовётся с потока,
	// на котором ОС сообщила о смене.
	WatchTheme  func(nw NativeWindow, emit func(SystemTheme)) (stop func())
	WatchAccent func(nw NativeWindow, emit func(color.RGBA, bool)) (stop func())
}

var shellSlot = newCapSlot[Shell]("shell")

// RegisterShell подключает службы оболочки (init пакета platform/shell).
//
//facadegen:skip
func RegisterShell(s Shell) (restore func()) { return shellSlot.register(s) }

// ShellServices — службы оболочки; ok=false — возможность не подключена
// (window/minimal).
//
//facadegen:skip
func ShellServices() (s Shell, ok bool) { return shellSlot.get() }
