package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Диалог с AllowedRoots не выходит из корня, места — только корни.
func TestFileDialog_AllowedRootsOption(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)

	fd := mb.ShowOpenFile(widget.FileDialogOptions{
		StartDir:     sub,
		AllowedRoots: []string{root},
	}, func(string, bool) {})

	if fd.CurrentDir() != sub {
		t.Fatalf("старт: cur=%q, want %q", fd.CurrentDir(), sub)
	}
	if places := fd.PlacePaths(); len(places) != 1 || places[0] != filepath.Clean(root) {
		t.Fatalf("места=%v, want [%s]", places, root)
	}

	fd.SetFileName("..")
	fd.Activate(0) // первый элемент списка
	if got := fd.CurrentDir(); got != root && got != sub {
		t.Fatalf("ушли за корень: %q", got)
	}
}

// Старт вне корней сажает диалог на первый корень.
func TestFileDialog_StartOutsideRoots(t *testing.T) {
	root := t.TempDir()
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)

	fd := mb.ShowPickFolder(widget.FileDialogOptions{
		StartDir:     filepath.Dir(filepath.Clean(root)),
		AllowedRoots: []string{root},
	}, func(string, bool) {})

	if fd.CurrentDir() != filepath.Clean(root) {
		t.Fatalf("cur=%q, want %q", fd.CurrentDir(), root)
	}
}

// Глобальный дефолт действует на новые диалоги.
func TestFileDialog_DefaultAllowedRoots(t *testing.T) {
	prev := widget.DefaultAllowedRoots()
	t.Cleanup(func() { widget.SetDefaultAllowedRoots(prev...) })

	root := t.TempDir()
	widget.SetDefaultAllowedRoots(root)

	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	fd := mb.ShowOpenFile(widget.FileDialogOptions{StartDir: root}, func(string, bool) {})
	if got := fd.AllowedRoots(); len(got) != 1 || got[0] != filepath.Clean(root) {
		t.Fatalf("AllowedRoots=%v", got)
	}
	if places := fd.PlacePaths(); len(places) != 1 || places[0] != filepath.Clean(root) {
		t.Fatalf("места=%v", places)
	}
}
