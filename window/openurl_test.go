package window

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestCheckOpenURL — какие ссылки пропускаются, какие нет. Главный случай —
// недоверенный документ со ссылкой [клик](javascript:...): она не должна
// дойти до системы ни в каком написании.
func TestCheckOpenURL(t *testing.T) {
	ok := []string{
		"https://example.com",
		"http://example.com/a?b=c#d",
		"HTTPS://EXAMPLE.COM/",
		"  https://example.com/пробелы-по-краям  ",
		"ftp://ftp.example.com/pub/file.zip",
		"mailto:user@example.com",
		"mailto:?subject=hi",
		"file:///home/user/doc.pdf",
		"file://localhost/home/user/doc.pdf",
		"file:///C:/Users/u/doc.pdf",
	}
	for _, s := range ok {
		got, err := checkOpenURL(s)
		if err != nil {
			t.Errorf("checkOpenURL(%q): неожиданный отказ: %v", s, err)
			continue
		}
		if got != strings.TrimSpace(s) {
			t.Errorf("checkOpenURL(%q) = %q, ссылка не должна меняться", s, got)
		}
	}

	scheme := []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)",
		"  javascript:alert(1)",
		"java\tscript:alert(1)",
		"java\nscript:alert(1)",
		"vbscript:msgbox(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"ms-msdt:/id PCWDiagnostic",
		"search-ms:query=x",
		"smb://server/share",
		"ssh://host",
		"tel:+100",
		"about:blank",
		"C:\\Windows\\System32\\calc.exe",
		"//evil.example.com/share",
		"example.com",
		"www.example.com/path",
		"file://evil.example.com/share/doc.pdf", // UNC: сетевой путь
		"file://server/share/doc.pdf",
	}
	for _, s := range scheme {
		if _, err := checkOpenURL(s); !errors.Is(err, ErrOpenScheme) {
			t.Errorf("checkOpenURL(%q) = %v, ждали ErrOpenScheme", s, err)
		}
	}

	bad := []string{
		"",
		"   ",
		"https://",
		"http:///path",
		`http:\\evil.example.com\x`,
		"file://",
		"https://exa mple.com",
	}
	for _, s := range bad {
		if _, err := checkOpenURL(s); err == nil {
			t.Errorf("checkOpenURL(%q): ждали ошибку", s)
		}
	}

	// Файл-программа по file:// — не «документ», а запуск.
	for _, s := range []string{"file:///tmp/run.exe", "file:///tmp/x.BAT", "file:///tmp/x.lnk."} {
		if _, err := checkOpenURL(s); !errors.Is(err, ErrOpenLaunchable) {
			t.Errorf("checkOpenURL(%q) = %v, ждали ErrOpenLaunchable", s, err)
		}
	}
}

// TestOpenURLPassesCheckedURL — в системную точку уходит проверенная ссылка, а
// отклонённая до неё не доходит вовсе.
func TestOpenURLPassesCheckedURL(t *testing.T) {
	var calls []string
	old := openURLImpl
	openURLImpl = func(u string) error { calls = append(calls, u); return nil }
	t.Cleanup(func() { openURLImpl = old })

	if err := OpenURL("  https://example.com/x  "); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"https://example.com/x"}) {
		t.Fatalf("вызовы = %q", calls)
	}

	calls = nil
	if err := OpenURL("javascript:alert(1)"); !errors.Is(err, ErrOpenScheme) {
		t.Fatalf("ждали ErrOpenScheme, получили %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("отклонённая ссылка дошла до системы: %q", calls)
	}

	// Ошибка системы пробрасывается как есть.
	boom := errors.New("нет браузера")
	openURLImpl = func(string) error { return boom }
	if err := OpenURL("https://example.com"); !errors.Is(err, boom) {
		t.Fatalf("ошибка системы потерялась: %v", err)
	}
}

// TestOpenFileAndReveal — абсолютизация пути, существование и запрет на
// программы; Reveal программу разрешает (её не запускают, а показывают).
func TestOpenFileAndReveal(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "отчёт #1.txt")
	exe := filepath.Join(dir, "run.exe")
	for _, p := range []string{doc, exe} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var opened, revealed []string
	oldOpen, oldReveal := openFileImpl, revealFileImpl
	openFileImpl = func(p string) error { opened = append(opened, p); return nil }
	revealFileImpl = func(p string) error { revealed = append(revealed, p); return nil }
	t.Cleanup(func() { openFileImpl, revealFileImpl = oldOpen, oldReveal })

	if err := OpenFile(doc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(opened, []string{doc}) {
		t.Fatalf("OpenFile передал %q, ждали %q", opened, doc)
	}

	// Относительный путь становится абсолютным.
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	opened = nil
	if err := OpenFile("отчёт #1.txt"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || !filepath.IsAbs(opened[0]) {
		t.Fatalf("путь не стал абсолютным: %q", opened)
	}

	opened = nil
	if err := OpenFile(exe); !errors.Is(err, ErrOpenLaunchable) {
		t.Fatalf("OpenFile(.exe) = %v, ждали ErrOpenLaunchable", err)
	}
	if err := OpenFile(filepath.Join(dir, "нет-такого.txt")); err == nil {
		t.Fatal("OpenFile несуществующего файла: ждали ошибку")
	}
	if err := OpenFile(""); err == nil {
		t.Fatal("OpenFile(\"\"): ждали ошибку")
	}
	if err := OpenFile("a\x00b"); err == nil {
		t.Fatal("OpenFile с нулевым байтом: ждали ошибку")
	}
	if len(opened) != 0 {
		t.Fatalf("отклонённые файлы дошли до системы: %q", opened)
	}

	if err := RevealFile(exe); err != nil {
		t.Fatalf("RevealFile(.exe): %v", err)
	}
	if !reflect.DeepEqual(revealed, []string{exe}) {
		t.Fatalf("RevealFile передал %q", revealed)
	}
	if err := RevealFile(filepath.Join(dir, "нет-такого.txt")); err == nil {
		t.Fatal("RevealFile несуществующего: ждали ошибку")
	}
}

func TestIsLaunchableName(t *testing.T) {
	yes := []string{"a.exe", "A.EXE", "run.exe.", "run.exe ", "x.Bat", "s.ps1", "l.lnk",
		"i.desktop", "Safari.app", "c.command", "p.js", "/tmp/dir.with.dots/prog.cmd"}
	no := []string{"a.txt", "a.pdf", "readme", "exe", ".exe.txt", "a.exe.pdf", "photo.jpg", "dir.exe/readme.md"}
	for _, n := range yes {
		if !isLaunchableName(n) {
			t.Errorf("isLaunchableName(%q) = false", n)
		}
	}
	for _, n := range no {
		if isLaunchableName(n) {
			t.Errorf("isLaunchableName(%q) = true", n)
		}
	}
}

func TestPathToFileURL(t *testing.T) {
	cases := map[string]string{
		"/home/user/a.txt":       "file:///home/user/a.txt",
		"/home/u/отчёт #1.pdf":   "file:///home/u/%D0%BE%D1%82%D1%87%D1%91%D1%82%20%231.pdf",
		"/tmp/a b?c.txt":         "file:///tmp/a%20b%3Fc.txt",
		`C:\Users\u\my file.txt`: "file:///C:/Users/u/my%20file.txt",
		"C:/Users/u/a.txt":       "file:///C:/Users/u/a.txt",
		"/":                      "file:///",
	}
	for in, want := range cases {
		if got := pathToFileURL(in); got != want {
			t.Errorf("pathToFileURL(%q) = %q, want %q", in, got, want)
		}
		// Обратное преобразование движка (dnd.go) обязано вернуть исходный путь.
		if back, ok := fileURIToPath(pathToFileURL(in)); !ok {
			t.Errorf("fileURIToPath(%q) не распознал ссылку", pathToFileURL(in))
		} else if strings.HasPrefix(in, "/") && back != in {
			t.Errorf("круг путь→ссылка→путь: %q → %q", in, back)
		}
	}
}

func TestShellExecuteErrText(t *testing.T) {
	if got := shellExecuteErrText(31); !strings.Contains(got, "нет назначенного приложения") {
		t.Errorf("31 → %q", got)
	}
	if got := shellExecuteErrText(2); !strings.Contains(got, "не найден") {
		t.Errorf("2 → %q", got)
	}
	if got := shellExecuteErrText(4242); !strings.Contains(got, "4242") {
		t.Errorf("неизвестный код потерян: %q", got)
	}
}

func TestExplorerSelectCmdLine(t *testing.T) {
	got, err := explorerSelectCmdLine(`C:\Windows\explorer.exe`, `C:\Мои документы\a b.txt`)
	if err != nil {
		t.Fatal(err)
	}
	want := `"C:\Windows\explorer.exe" /select,"C:\Мои документы\a b.txt"`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := explorerSelectCmdLine("explorer.exe", `C:\a" /root,"\\evil\x`); err == nil {
		t.Error("кавычка в пути должна отклоняться")
	}
}

// ─── Порядок действий на Linux (проверяется на любой ОС) ─────────────────────

// callRec — запись одного вызова D-Bus.
type callRec struct {
	dest, path, iface, member, sig string
	args                           []any
}

// fakeOpenBackend собирает openBackend с заглушками. dbusErr выбирает ответ
// по получателю вызова; started получает запущенные подпроцессы.
type fakeOpenBackend struct {
	calls   []callRec
	dbusErr map[string]error // по dest
	xdgPath string           // "" — xdg-open в PATH нет
	started []*exec.Cmd
	startEr error
}

func (f *fakeOpenBackend) backend() openBackend {
	return openBackend{
		call: func(dest, path, iface, member, sig string, args []any) error {
			f.calls = append(f.calls, callRec{dest, path, iface, member, sig, args})
			return f.dbusErr[dest]
		},
		lookPath: func(file string) (string, error) {
			if file == "xdg-open" && f.xdgPath != "" {
				return f.xdgPath, nil
			}
			return "", exec.ErrNotFound
		},
		start: func(cmd *exec.Cmd) error {
			f.started = append(f.started, cmd)
			return f.startEr
		},
	}
}

var errNoPortal = errors.New("org.freedesktop.DBus.Error.ServiceUnknown: The name org.freedesktop.portal.Desktop was not provided by any .service files")

func TestPortalOpenURI(t *testing.T) {
	f := &fakeOpenBackend{}
	if err := f.backend().open("https://example.com/", "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || len(f.started) != 0 {
		t.Fatalf("вызовов %d, подпроцессов %d; ждали 1 и 0", len(f.calls), len(f.started))
	}
	c := f.calls[0]
	if c.dest != "org.freedesktop.portal.Desktop" || c.path != "/org/freedesktop/portal/desktop" ||
		c.iface != "org.freedesktop.portal.OpenURI" || c.member != "OpenURI" || c.sig != "ssa{sv}" {
		t.Errorf("вызов портала: %+v", c)
	}
	if len(c.args) != 3 || c.args[0] != "" || c.args[1] != "https://example.com/" {
		t.Errorf("аргументы: %#v", c.args)
	}
}

// TestPortalArgsMarshal — аргументы, что мы отдаём порталу, наш же клиент D-Bus
// умеет упаковать в сообщение: ошибка типа проявилась бы на живой шине.
func TestPortalArgsMarshal(t *testing.T) {
	for name, build := range map[string]func() (string, []any){
		"OpenURI":   func() (string, []any) { return portalOpenURIArgs("file:///tmp/a%20b") },
		"ShowItems": func() (string, []any) { return showItemsArgs("file:///tmp/a%20b") },
	} {
		sig, args := build()
		m := &dbusMessage{Type: dbusTypeMethodCall, Serial: 1, Path: "/x", Interface: "a.b", Member: name,
			Destination: "a.b", Sig: sig, Body: args}
		raw, err := m.marshal()
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		back, err := dbusUnmarshal(raw)
		if err != nil {
			t.Fatalf("%s: unmarshal: %v", name, err)
		}
		if back.Sig != sig {
			t.Errorf("%s: сигнатура %q, ждали %q", name, back.Sig, sig)
		}
	}
}

func TestPortalMissingFallsBackToXdgOpen(t *testing.T) {
	f := &fakeOpenBackend{dbusErr: map[string]error{"org.freedesktop.portal.Desktop": errNoPortal}, xdgPath: "/usr/bin/xdg-open"}
	if err := f.backend().open("file:///tmp/a.txt", "/tmp/a.txt"); err != nil {
		t.Fatal(err)
	}
	if len(f.started) != 1 {
		t.Fatalf("подпроцессов %d, ждали 1", len(f.started))
	}
	if got := f.started[0].Args; !reflect.DeepEqual(got, []string{"/usr/bin/xdg-open", "/tmp/a.txt"}) {
		t.Errorf("команда: %q", got)
	}
}

func TestPortalMissingNoXdgOpen(t *testing.T) {
	f := &fakeOpenBackend{dbusErr: map[string]error{"org.freedesktop.portal.Desktop": errNoPortal}}
	err := f.backend().open("https://example.com", "https://example.com")
	if !errors.Is(err, errOpenNoXdgOpen) {
		t.Fatalf("ждали errOpenNoXdgOpen, получили %v", err)
	}
	if len(f.started) != 0 {
		t.Fatal("подпроцесс не должен запускаться без xdg-open")
	}
}

// TestPortalRefusalNotBypassed — портал жив и ответил отказом: xdg-open не
// зовём, иначе обошли бы его политику.
func TestPortalRefusalNotBypassed(t *testing.T) {
	f := &fakeOpenBackend{
		dbusErr: map[string]error{"org.freedesktop.portal.Desktop": errors.New("org.freedesktop.portal.Error.NotAllowed: нельзя")},
		xdgPath: "/usr/bin/xdg-open",
	}
	if err := f.backend().open("https://example.com", "https://example.com"); err == nil {
		t.Fatal("ждали ошибку портала")
	}
	if len(f.started) != 0 {
		t.Fatal("xdg-open не должен звать при отказе живого портала")
	}
}

func TestPortalNoSessionBus(t *testing.T) {
	err := errors.Join(errOpenNoBus, errors.New("connect: no such file"))
	if !dbusServiceAbsent(err) {
		t.Error("отсутствие шины должно считаться «сервиса нет»")
	}
	if dbusServiceAbsent(nil) || dbusServiceAbsent(errors.New("таймаут вызова")) {
		t.Error("таймаут и nil — не «сервиса нет»")
	}
	for _, s := range []string{"x: org.freedesktop.DBus.Error.NameHasNoOwner: n", "org.freedesktop.DBus.Error.Spawn.ServiceNotFound: x"} {
		if !dbusServiceAbsent(errors.New(s)) {
			t.Errorf("dbusServiceAbsent(%q) = false", s)
		}
	}
}

func TestRevealShowItems(t *testing.T) {
	f := &fakeOpenBackend{}
	if err := f.backend().reveal("/home/u/my file.txt"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("вызовов %d", len(f.calls))
	}
	c := f.calls[0]
	if c.dest != "org.freedesktop.FileManager1" || c.member != "ShowItems" || c.sig != "ass" {
		t.Errorf("вызов: %+v", c)
	}
	if uris, _ := c.args[0].([]string); !reflect.DeepEqual(uris, []string{"file:///home/u/my%20file.txt"}) {
		t.Errorf("ссылки: %#v", c.args[0])
	}
}

// TestRevealWithoutFileManagerOpensFolder — менеджера с FileManager1 нет:
// открывается содержащая папка через портал.
func TestRevealWithoutFileManagerOpensFolder(t *testing.T) {
	f := &fakeOpenBackend{dbusErr: map[string]error{
		"org.freedesktop.FileManager1": errors.New("org.freedesktop.DBus.Error.ServiceUnknown: нет"),
	}}
	if err := f.backend().reveal("/home/u/a.txt"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("вызовов %d, ждали 2 (ShowItems, затем OpenURI)", len(f.calls))
	}
	c := f.calls[1]
	if c.member != "OpenURI" || c.args[1] != "file:///home/u" {
		t.Errorf("второй вызов: %+v", c)
	}
}

func TestRevealFileManagerRefusal(t *testing.T) {
	f := &fakeOpenBackend{dbusErr: map[string]error{
		"org.freedesktop.FileManager1": errors.New("org.freedesktop.DBus.Error.Failed: x"),
	}}
	if err := f.backend().reveal("/home/u/a.txt"); err == nil {
		t.Fatal("ждали ошибку менеджера")
	}
	if len(f.calls) != 1 {
		t.Fatalf("после отказа живого менеджера обходных вызовов быть не должно, вызовов %d", len(f.calls))
	}
}
