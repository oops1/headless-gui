package shell

import (
	"errors"
	"github.com/oops1/headless-gui/v3/platform/dbus"
	"github.com/oops1/headless-gui/v3/platform/native"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

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
		if back, ok := native.FileURIToPath(pathToFileURL(in)); !ok {
			t.Errorf("native.FileURIToPath(%q) не распознал ссылку", pathToFileURL(in))
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
		m := &dbus.Message{Type: dbus.TypeMethodCall, Serial: 1, Path: "/x", Interface: "a.b", Member: name,
			Destination: "a.b", Sig: sig, Body: args}
		raw, err := m.Marshal()
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		back, err := dbus.Unmarshal(raw)
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
