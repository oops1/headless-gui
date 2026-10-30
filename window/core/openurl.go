// openurl.go — открыть ссылку или файл тем приложением, что назначено в
// системе, и показать файл в файловом менеджере. Здесь — проверка входа, она
// действует перед любой реализацией: политика схем и запрет UNC — граница
// безопасности, запрет файлов-программ — защита от ошибки приложения (см.
// OpenFile). Сами системные вызовы — возможность platform/shell
// (ShellExecute, портал xdg-desktop-portal по D-Bus, open). Без неё
// (window/minimal) проверенный вход получает ErrOpenUnsupported.
//
// Зачем это движку. Раньше каждое приложение на движке делало своё: на Linux
// звало xdg-open подпроцессом. Для сессии WinLine это не годится: пакет
// ставится без зависимостей, бинарь статический, внешних утилит в нём нет,
// так что «открыть ссылку» упиралось в отсутствие xdg-open. Здесь системный
// путь выбран первым, а подпроцесс — только последний запасной.
package core

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// ErrOpenScheme — у ссылки схема, которую открывать нельзя. Различимо через
// errors.Is: приложение может показать «ссылка не открыта по соображениям
// безопасности», а не общую ошибку.
var ErrOpenScheme = errors.New("window: схема ссылки не разрешена к открытию")

// ErrOpenLaunchable — файл такого типа не открывается просмотрщиком, а
// запускается как программа (.exe, .bat, .lnk, .desktop …). Отказ — намеренный.
var ErrOpenLaunchable = errors.New("window: файл такого типа запускает программу, открывать его отказано")

// ErrOpenUnsupported — открытие не реализовано на этой платформе или службы
// оболочки не подключены (window/minimal без platform/shell).
var ErrOpenUnsupported = errors.New("window: открытие ссылок и файлов на этой платформе не поддержано")

// Точки вызова служб оболочки (возможность platform/shell, ADR-0013 §3) —
// переменные: тесты проверяют, ЧТО было бы вызвано, а не открывают настоящий
// браузер. Без возможности (window/minimal) — ErrOpenUnsupported.
var (
	openURLImpl = func(u string) error {
		if s, ok := native.ShellServices(); ok && s.OpenURL != nil {
			return s.OpenURL(u)
		}
		return ErrOpenUnsupported
	}
	openFileImpl = func(p string) error {
		if s, ok := native.ShellServices(); ok && s.OpenFile != nil {
			return s.OpenFile(p)
		}
		return ErrOpenUnsupported
	}
	revealFileImpl = func(p string) error {
		if s, ok := native.ShellServices(); ok && s.RevealFile != nil {
			return s.RevealFile(p)
		}
		return ErrOpenUnsupported
	}
)

// OpenURL открывает ссылку приложением, назначенным в системе: браузер для
// https://…, почтовый клиент для mailto:, просмотрщик для file://.
//
// Разрешены только схемы http, https, ftp, mailto и file. Остальное
// отклоняется ошибкой (errors.Is(err, ErrOpenScheme)): приложение, которое
// показывает ссылки из недоверенного документа (Markdown в Блокноте), иначе
// открыло бы javascript:, ms-msdt: или любой другой протокол-обработчик, а
// это запуск чужого кода одним щелчком. Для той же цели file:// с чужим
// хостом (UNC-путь, при открытии которого Windows сама идёт в сеть и отдаёт
// хеш пароля) и файлы-программы тоже отклоняются.
//
// Успех означает «система приняла запрос», а не «окно уже появилось»: на
// Linux портал отвечает сразу после приёма. Вызов может заблокировать
// вызывающего на время обращения к системе (до нескольких секунд при
// холодном старте портала), поэтому из обработчика кадра его лучше звать в
// горутине.
//
// Без служб оболочки (window/minimal) проверенная ссылка получает
// ErrOpenUnsupported; проверка схем действует и там.
func OpenURL(rawURL string) error {
	u, err := checkOpenURL(rawURL)
	if err != nil {
		return err
	}
	return openURLImpl(u)
}

// OpenFile открывает файл приложением, назначенным для его типа.
//
// Путь должен существовать. Файлы-программы (.exe, .bat, .lnk, .desktop,
// .app …) отклоняются (errors.Is(err, ErrOpenLaunchable)): «открыть» их
// системой значит запустить, а путь к такому файлу может прийти из
// недоверенного документа. Это защита от ошибки приложения, а не граница
// безопасности: список расширений конечен.
//
// Без служб оболочки (window/minimal) проверенный путь получает
// ErrOpenUnsupported; проверка файлов действует и там.
func OpenFile(path string) error {
	p, err := checkOpenFile(path)
	if err != nil {
		return err
	}
	return openFileImpl(p)
}

// RevealFile показывает файл в файловом менеджере, выделив его (Проводник,
// Finder, Nautilus/Dolphin/Thunar …). Если выделение менеджер не умеет —
// откроется хотя бы папка с файлом. Сам файл не запускается, поэтому запрет
// на файлы-программы здесь не действует.
//
// Без служб оболочки (window/minimal) проверенный путь получает
// ErrOpenUnsupported.
func RevealFile(path string) error {
	p, err := checkRevealPath(path)
	if err != nil {
		return err
	}
	return revealFileImpl(p)
}

// ─── Проверка входа (чистая логика) ──────────────────────────────────────────

// checkOpenURL проверяет ссылку и возвращает её в виде, пригодном для передачи
// системе. Разбор схемы нужен именно здесь, до любых платформенных вызовов:
// у ShellExecute, портала и open политики схем нет вовсе — открывают всё.
func checkOpenURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("window: пустая ссылка")
	}
	// Управляющие символы внутри схемы ("java\tscript:") браузеры молча
	// выбрасывают, и «javascript:» оказывается там, где фильтр его не видел.
	// Проще отказать, чем гадать, как это поймёт обработчик.
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%w: в ссылке управляющий символ", ErrOpenScheme)
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("window: ссылка не разобрана: %w", err)
	}
	// url.Parse приводит схему к нижнему регистру: "JavaScript:" не прячется.
	switch u.Scheme {
	case "http", "https", "ftp":
		// "http:\\evil" разбирается как «непрозрачная» ссылка без хоста —
		// отказываем: браузер понял бы её по-своему.
		if u.Hostname() == "" {
			return "", fmt.Errorf("window: в ссылке %s: нет адреса узла", u.Scheme)
		}
	case "mailto":
		// Адресата может не быть ("mailto:?subject=x") — это допустимо.
	case "file":
		if h := u.Hostname(); h != "" && !strings.EqualFold(h, "localhost") {
			return "", fmt.Errorf("%w: file:// с узлом %q (сетевой путь)", ErrOpenScheme, h)
		}
		if u.Path == "" {
			return "", errors.New("window: в file:// нет пути")
		}
		if isLaunchableName(u.Path) {
			return "", fmt.Errorf("%w: %s", ErrOpenLaunchable, filepath.Base(u.Path))
		}
	case "":
		return "", fmt.Errorf("%w: в ссылке нет схемы", ErrOpenScheme)
	default:
		return "", fmt.Errorf("%w: %q", ErrOpenScheme, u.Scheme)
	}
	return s, nil
}

// checkOpenFile приводит путь к абсолютному и проверяет, что файл есть и это
// не программа.
func checkOpenFile(path string) (string, error) {
	p, err := absExistingPath(path)
	if err != nil {
		return "", err
	}
	if isLaunchableName(p) {
		return "", fmt.Errorf("%w: %s", ErrOpenLaunchable, filepath.Base(p))
	}
	return p, nil
}

// checkRevealPath — то же без запрета на программы.
func checkRevealPath(path string) (string, error) {
	return absExistingPath(path)
}

// absExistingPath делает путь абсолютным и проверяет существование. Проверка
// нужна заранее: Windows на несуществующий файл показывает собственное окно
// ошибки, а Linux-портал отвечает «успехом» и не делает ничего. Абсолютный
// путь заодно не может начинаться с '-', то есть не прикинется параметром
// для xdg-open/open.
func absExistingPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("window: пустой путь")
	}
	if strings.ContainsRune(path, 0) {
		return "", errors.New("window: в пути нулевой байт")
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("window: путь %q: %w", path, err)
	}
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("window: %w", err)
	}
	return p, nil
}

// launchableExts — расширения, которые система при «открытии» запускает как
// программу, а не показывает. Список по Windows (исполняемые, сценарии,
// ярлыки, пакеты установки), macOS (.app, .command) и freedesktop (.desktop).
var launchableExts = map[string]bool{
	".exe": true, ".com": true, ".scr": true, ".pif": true, ".cpl": true,
	".bat": true, ".cmd": true, ".ps1": true, ".psm1": true, ".psd1": true,
	".vbs": true, ".vbe": true, ".js": true, ".jse": true, ".wsf": true, ".wsh": true,
	".hta": true, ".msi": true, ".msp": true, ".msc": true, ".reg": true,
	".lnk": true, ".url": true, ".appref-ms": true, ".gadget": true, ".jar": true,
	".app": true, ".command": true, ".workflow": true,
	".desktop": true,
}

// isLaunchableName сообщает, запускает ли система файл с таким именем.
// Хвостовые точки и пробелы срезаются: Windows их игнорирует, и
// "run.exe." — это тот же run.exe.
func isLaunchableName(name string) bool {
	name = strings.TrimRight(name, ". \t")
	return launchableExts[strings.ToLower(filepath.Ext(name))]
}
