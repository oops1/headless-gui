package shell

// openurl.go — общие помощники открытия ссылок и файлов для реализаций под
// ОС (openurl_windows.go, openurl_linux.go, openurl_darwin.go). Проверка входа
// — в ядре окна (window/core/openurl.go): сюда приходит уже проверенное.

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

// openStartCmd запускает подпроцесс и НЕ ждёт его, но собирает код
// завершения в отдельной горутине: без Wait завершившийся потомок остаётся
// зомби до конца работы приложения. Переменная — тесты подменяют запуск.
var openStartCmd = func(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// ─── Помощники для платформенных реализаций ──────────────────────────────────

// pathToFileURL строит file://-ссылку из абсолютного пути. Нужна там, где
// система принимает только ссылки (портал OpenURI, FileManager1.ShowItems).
// Пробелы, '#', '?' и не-ASCII кодируются процентами: иначе путь
// «/home/u/отчёт #1.pdf» превратился бы в ссылку с фрагментом.
// Windows-путь с диском ("C:\a\b") получает вид file:///C:/a/b.
func pathToFileURL(abs string) string {
	p := strings.ReplaceAll(abs, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// shellExecuteErrText переводит код возврата ShellExecute (значения до 32 —
// это ошибки, а не дескриптор) в понятный текст. Системная часть сама по себе
// вместо кода даёт малоинформативное «параметр задан неверно».
func shellExecuteErrText(code uintptr) string {
	switch code {
	case 0:
		return "не хватает памяти или ресурсов"
	case 2, 3:
		return "файл или путь не найден"
	case 5:
		return "отказано в доступе"
	case 8:
		return "не хватает памяти"
	case 26:
		return "нарушение совместного доступа"
	case 27, 31:
		return "для этого типа нет назначенного приложения"
	case 28, 29, 30:
		return "приложение не ответило (DDE)"
	case 32:
		return "не найдена библиотека приложения"
	}
	return fmt.Sprintf("код %d", code)
}

// explorerSelectCmdLine собирает командную строку Проводника для показа файла
// с выделением. Строка нужна «сырая»: Go-шное экранирование аргументов
// превращает /select,C:\a b\c.txt в "/select,C:\a b\c.txt" целиком в кавычках,
// а Проводник такой формы не понимает и открывает «Документы». Ему нужна
// именно форма /select,"путь".
func explorerSelectCmdLine(explorer, path string) (string, error) {
	// В именах файлов Windows кавычек нет; значит, кавычка в пути — не путь, а
	// попытка выйти из кавычек и дописать Проводнику свои параметры.
	if strings.ContainsRune(path, '"') {
		return "", errors.New("window: в пути недопустимая кавычка")
	}
	return `"` + explorer + `" /select,"` + path + `"`, nil
}
