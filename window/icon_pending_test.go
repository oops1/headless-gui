package window

import (
	"errors"
	"image"
	"testing"
)

// Значок, заданный до Run — запрос GG-38.
//
// SetIcon определял поддержку приведением w.native к интерфейсу, а поле native
// заполняет первая же строка Run: до неё оно nil, приведение на nil-интерфейсе
// всегда даёт «не поддерживается», и всякий вызов до Run возвращал ошибку —
// независимо от бэкенда. Это расходилось с собственной документацией метода и
// делало недостижимой уже написанную буферизацию внутри бэкендов.

// fakeIconBackend — бэкенд, умеющий значок, но не окно.
type fakeIconBackend struct {
	NativeWindow
	got []image.Image
	n   int
}

func (f *fakeIconBackend) setIcon(icons []image.Image) error {
	f.got = icons
	f.n++
	return nil
}

// deafBackend — бэкенд без поддержки значка.
type deafBackend struct{ NativeWindow }

func icon16() image.Image { return image.NewRGBA(image.Rect(0, 0, 16, 16)) }

// До Run вызов не ошибка: значок откладывается.
func TestSetIcon_BeforeRunIsRemembered(t *testing.T) {
	win := &Window{}

	if err := win.SetIcon(icon16(), icon16()); err != nil {
		t.Fatalf("SetIcon до Run вернул %v", err)
	}
	if !win.iconWant || len(win.pendingIcons) != 2 {
		t.Fatalf("значок не отложен: want=%v, %d картинок", win.iconWant, len(win.pendingIcons))
	}

	// Run отдаёт отложенный значок бэкенду сразу после его создания.
	be := &fakeIconBackend{}
	win.native = be
	win.applyPendingIcon()

	if be.n != 1 || len(be.got) != 2 {
		t.Errorf("бэкенду досталось %d вызовов и %d картинок", be.n, len(be.got))
	}
}

// Пустые и nil-картинки отсеиваются до бэкенда.
func TestSetIcon_SkipsEmptyImages(t *testing.T) {
	win := &Window{}
	_ = win.SetIcon(nil, image.NewRGBA(image.Rectangle{}), icon16())

	if len(win.pendingIcons) != 1 {
		t.Errorf("отложено %d картинок, ждали одну", len(win.pendingIcons))
	}
}

// После Run вызов идёт прямо в бэкенд.
func TestSetIcon_AfterRunGoesToBackend(t *testing.T) {
	be := &fakeIconBackend{}
	win := &Window{}
	win.native = be

	if err := win.SetIcon(icon16()); err != nil {
		t.Fatalf("SetIcon вернул %v", err)
	}
	if be.n != 1 {
		t.Errorf("бэкенд получил %d вызовов", be.n)
	}
}

// Бэкенд без поддержки отвечает ошибкой, а не молчанием, — но только когда он
// уже есть. До Run сказать «не поддерживается» нельзя: бэкенда ещё нет.
func TestSetIcon_UnsupportedBackendReportsError(t *testing.T) {
	win := &Window{}
	win.native = &deafBackend{}

	if err := win.SetIcon(icon16()); !errors.Is(err, ErrIconUnsupported) {
		t.Errorf("бэкенд без поддержки вернул %v", err)
	}
}

// Ничего не задавали — применять нечего, бэкенд не трогаем.
func TestSetIcon_NothingPendingIsNoOp(t *testing.T) {
	be := &fakeIconBackend{}
	win := &Window{}
	win.native = be
	win.applyPendingIcon()

	if be.n != 0 {
		t.Errorf("бэкенд получил %d вызовов без заданного значка", be.n)
	}
}
