package core

import (
	"errors"
	"os"
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
