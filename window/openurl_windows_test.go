//go:build windows

package window

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestWindowsOpenUsesShellExecuteOpen — OpenURL/OpenFile доходят до
// ShellExecute с глаголом "open" и нужной целью; настоящий вызов подменён.
func TestWindowsOpenUsesShellExecuteOpen(t *testing.T) {
	type call struct{ verb, target string }
	var got []call
	old := winShellExecute
	winShellExecute = func(verb, target string) error { got = append(got, call{verb, target}); return nil }
	t.Cleanup(func() { winShellExecute = old })

	doc := filepath.Join(t.TempDir(), "a b.txt")
	if err := os.WriteFile(doc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := OpenURL("mailto:user@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := OpenFile(doc); err != nil {
		t.Fatal(err)
	}
	want := []call{{"open", "mailto:user@example.com"}, {"open", doc}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("вызовы ShellExecute = %v, want %v", got, want)
	}
}

// TestWindowsRevealCommandLine — Проводник получает сырую командную строку
// /select,"путь" (с кавычками вокруг пути, а не вокруг всего параметра).
func TestWindowsRevealCommandLine(t *testing.T) {
	var cmds []*exec.Cmd
	old := openStartCmd
	openStartCmd = func(c *exec.Cmd) error { cmds = append(cmds, c); return nil }
	t.Cleanup(func() { openStartCmd = old })

	doc := filepath.Join(t.TempDir(), "мой файл.txt")
	if err := os.WriteFile(doc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RevealFile(doc); err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 {
		t.Fatalf("запусков %d, ждали 1", len(cmds))
	}
	line := cmds[0].SysProcAttr.CmdLine
	if !strings.HasSuffix(line, ` /select,"`+doc+`"`) {
		t.Errorf("командная строка %q", line)
	}
	if !strings.HasSuffix(strings.ToLower(cmds[0].Path), "explorer.exe") {
		t.Errorf("запускается %q, ждали explorer.exe", cmds[0].Path)
	}
}
