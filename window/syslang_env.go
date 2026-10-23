//go:build !windows

package window

// syslang_env.go — языки интерфейса на Linux и macOS: переменные среды.
//
// У Linux нет единого списка языков вне сессии рабочего стола, но каждая
// сессия (GNOME, KDE, systemd --user, login) выставляет те же переменные, по
// которым работает gettext, — а значит, ровно по ним выбирают язык и остальные
// программы. Читать настройки среды отдельно (AccountsService, kdeglobals)
// значило бы разойтись с ними в ответе.
//
// macOS: язык ОС лежит в NSLocale.preferredLanguages (бэкенд Cocoa написан на
// purego, и проверить ещё один ручной вызов objc_msgSend на Mac, которого у
// проекта нет, негде — см. systheme_darwin.go). Пока читаем те же переменные
// среды: из терминала они обычно выставлены, из .app — нет, и тогда ответ
// пуст, а язык выбирает приложение.

func systemLanguages() []string { return systemLanguagesEnv() }
