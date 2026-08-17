package widget

import (
	"os"
	"path/filepath"
	"testing"
)

// stubModal — заглушка показа модалок (движок в тесте не нужен).
type stubModal struct{ closed int }

func (s *stubModal) ShowModal(m ModalWidget)  {}
func (s *stubModal) CloseModal(m ModalWidget) { s.closed++ }

// newRootedDialog собирает браузер файлов без движка.
func newRootedDialog(roots ...string) *FileDialog {
	fd := &FileDialog{
		opts:  FileDialogOptions{Filters: []FileFilter{{Label: "*"}}},
		roots: normRoots(roots),
	}
	fd.crumb = newCrumbBar()
	fd.places = newPlaceList(buildPlaces(nil, fd.roots))
	fd.table = newFileTable()
	return fd
}

// С заданными корнями наружу не выйти, «Места» — только корни.
func TestFileDialog_AllowedRoots(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	fd := newRootedDialog(root)
	fd.navigate(root)
	if fd.CurrentDir() != filepath.Clean(root) {
		t.Fatalf("cur=%q, want %q", fd.CurrentDir(), root)
	}

	fd.goUp()
	if fd.CurrentDir() != filepath.Clean(root) {
		t.Fatalf("подъём выше корня: cur=%q", fd.CurrentDir())
	}
	fd.navigate(filepath.Join(root, "..", ".."))
	if fd.CurrentDir() != filepath.Clean(root) {
		t.Fatalf("выход по «..»: cur=%q", fd.CurrentDir())
	}
	fd.navigate(sub)
	if fd.CurrentDir() != sub {
		t.Fatalf("вход в подпапку: cur=%q", fd.CurrentDir())
	}

	places := fd.PlacePaths()
	if len(places) != 1 || places[0] != filepath.Clean(root) {
		t.Fatalf("места=%v, want [%s]", places, root)
	}
}

// Путь за пределами корней не подтверждается.
func TestFileDialog_FinishOutsideRejected(t *testing.T) {
	root := t.TempDir()
	fd := newRootedDialog(root)
	eng := &stubModal{}
	fd.eng = eng
	got := ""
	fd.onResult = func(path string, ok bool) { got = path }
	fd.navigate(root)

	fd.finish(filepath.Join(root, "..", "чужое.txt"))
	if got != "" || eng.closed != 0 {
		t.Fatalf("принят путь вне корней: %q", got)
	}
	inside := filepath.Join(root, "своё.txt")
	fd.finish(inside)
	if got != inside {
		t.Fatalf("путь внутри корня отклонён: %q", got)
	}
}

// Без корней ограничений нет — прежнее поведение.
func TestFileDialog_NoRootsUnrestricted(t *testing.T) {
	root := t.TempDir()
	fd := newRootedDialog()
	fd.navigate(root)
	parent := filepath.Dir(filepath.Clean(root))
	fd.goUp()
	if fd.CurrentDir() != parent {
		t.Fatalf("без корней подъём не сработал: cur=%q, want %q", fd.CurrentDir(), parent)
	}
	if len(fd.PlacePaths()) == 0 {
		t.Fatal("без корней список мест пуст")
	}
}

// SetAllowedRoots применяется к уже открытому диалогу.
func TestFileDialog_SetAllowedRoots(t *testing.T) {
	root := t.TempDir()
	fd := newRootedDialog()
	fd.navigate(filepath.Dir(filepath.Clean(root)))
	fd.SetAllowedRoots(root)
	if fd.CurrentDir() != filepath.Clean(root) {
		t.Fatalf("после SetAllowedRoots cur=%q, want %q", fd.CurrentDir(), root)
	}
	if got := fd.AllowedRoots(); len(got) != 1 || got[0] != filepath.Clean(root) {
		t.Fatalf("AllowedRoots=%v", got)
	}
	if places := fd.PlacePaths(); len(places) != 1 || places[0] != filepath.Clean(root) {
		t.Fatalf("места не пересобраны: %v", places)
	}
}

// Глобальный дефолт хранится и снимается.
func TestDefaultAllowedRoots(t *testing.T) {
	prev := DefaultAllowedRoots()
	t.Cleanup(func() { SetDefaultAllowedRoots(prev...) })

	root := t.TempDir()
	SetDefaultAllowedRoots(root)
	if got := DefaultAllowedRoots(); len(got) != 1 || got[0] != filepath.Clean(root) {
		t.Fatalf("дефолт=%v", got)
	}
	SetDefaultAllowedRoots()
	if got := DefaultAllowedRoots(); len(got) != 0 {
		t.Fatalf("дефолт не снят: %v", got)
	}
}

// Проверка границы по корню.
func TestWithinRoots(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	roots := []string{root}
	inside := []string{root, filepath.Join(root, "a"), filepath.Join(root, "a", "b")}
	for _, p := range inside {
		if !withinRoots(roots, p) {
			t.Errorf("%q должен быть внутри", p)
		}
	}
	outside := []string{filepath.Dir(root), filepath.Join(root, "..", "x"), root + "x"}
	for _, p := range outside {
		if withinRoots(roots, filepath.Clean(p)) {
			t.Errorf("%q не должен быть внутри", p)
		}
	}
	if !withinRoots(nil, filepath.Dir(root)) {
		t.Error("без корней должно быть разрешено всё")
	}
}
