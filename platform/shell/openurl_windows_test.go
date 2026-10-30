//go:build windows

package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestWindowsOpenUsesShellExecuteOpen — службы OpenURL/OpenFile, которые
// зарегистрировал пакет, доходят до ShellExecute с глаголом "open" и нужной
// целью; настоящий вызов подменён.
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
	s := registered(t)
	if err := s.OpenURL("mailto:user@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenFile(doc); err != nil {
		t.Fatal(err)
	}
	want := []call{{"open", "mailto:user@example.com"}, {"open", doc}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("вызовы ShellExecute = %v, want %v", got, want)
	}
}

// TestWindowsRevealCommandLine — служба RevealFile запускает Проводник с сырой
// командной строкой /select,"путь" (с кавычками вокруг пути, а не вокруг
// всего параметра), мимо ShellExecute.
func TestWindowsRevealCommandLine(t *testing.T) {
	var cmds []*exec.Cmd
	old := openStartCmd
	openStartCmd = func(c *exec.Cmd) error { cmds = append(cmds, c); return nil }
	t.Cleanup(func() { openStartCmd = old })
	oldSE := winShellExecute
	winShellExecute = func(verb, target string) error {
		t.Errorf("ShellExecute(%q, %q) вместо Проводника", verb, target)
		return nil
	}
	t.Cleanup(func() { winShellExecute = oldSE })

	doc := filepath.Join(t.TempDir(), "мой файл.txt")
	if err := os.WriteFile(doc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := registered(t).RevealFile(doc); err != nil {
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
