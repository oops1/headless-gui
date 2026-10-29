package core

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Обещание виртуальных файлов владельца выделения: общая часть X11 и Wayland.

func virtTestFiles() []widget.VirtualFile {
	return []widget.VirtualFile{
		{Name: "a.txt", Size: 3},
		{Name: "Папка", IsDir: true},
		{Name: "Папка/b.txt", Size: 5},
		{Name: "Папка/вложенная", IsDir: true},
		{Name: "Папка/вложенная/c.txt"},
		{Name: "z.bin", Size: 1},
	}
}

// Пути просят только у записей верхнего уровня, по порядку списка; вложенное
// приложение кладёт само вместе с каталогом.
func TestClipVirtual_PathsAskTopLevelOnly(t *testing.T) {
	var asked []int
	v, err := newClipVirtual(virtTestFiles(), nil, widget.VirtualFilesOptions{
		Materialize: func(i int) (string, error) {
			asked = append(asked, i)
			return "/tmp/app/" + virtTestFiles()[i].Name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 0 {
		t.Fatal("Materialize вызван до запроса")
	}
	got, err := v.Paths()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/tmp/app/a.txt", "/tmp/app/Папка", "/tmp/app/z.bin"}; !reflect.DeepEqual(got, want) {
		t.Errorf("пути: %v, ждал %v", got, want)
	}
	if !reflect.DeepEqual(asked, []int{0, 1, 5}) {
		t.Errorf("спрошены записи %v, ждал [0 1 5]", asked)
	}
	// Повторный запрос (gnome-copied-files после uri-list) приложение не тревожит.
	got2, _ := v.Paths()
	if !reflect.DeepEqual(got2, got) || len(asked) != 3 {
		t.Errorf("повторный запрос: %v, вызовов %d", got2, len(asked))
	}
	// Ответ — копия: правка вызывающим не портит запомненное.
	got2[0] = "/испорчено"
	if got3, _ := v.Paths(); got3[0] != "/tmp/app/a.txt" {
		t.Errorf("запомненный ответ изменён снаружи: %v", got3)
	}
	if want := "a.txt\nПапка\nПапка/b.txt\nПапка/вложенная\nПапка/вложенная/c.txt\nz.bin"; v.names() != want {
		t.Errorf("текст-имена: %q", v.names())
	}
}

func TestClipVirtual_Validation(t *testing.T) {
	mat := func(int) (string, error) { return "/x", nil }
	if _, err := newClipVirtual(virtTestFiles(), nil, widget.VirtualFilesOptions{}); !errors.Is(err, widget.ErrClipboardVirtualNoSource) {
		t.Errorf("без Materialize: %v", err)
	}
	// Вложенная запись без объявленного каталога верхнего уровня.
	_, err := newClipVirtual([]widget.VirtualFile{{Name: "d/x"}}, nil, widget.VirtualFilesOptions{Materialize: mat})
	if !errors.Is(err, widget.ErrVirtualFileName) {
		t.Errorf("каталог не объявлен: %v", err)
	}
}

// Ошибка и относительный путь — отказ, и он не запоминается: следующая
// вставка пробует снова.
func TestClipVirtual_ErrorNotCached(t *testing.T) {
	var calls atomic.Int32
	bad := errors.New("сеть пропала")
	v, _ := newClipVirtual([]widget.VirtualFile{{Name: "a"}}, nil, widget.VirtualFilesOptions{
		Materialize: func(int) (string, error) {
			switch calls.Add(1) {
			case 1:
				return "", bad
			case 2:
				return "относительный/путь", nil
			}
			return "/ok/a", nil
		},
	})
	if _, err := v.Paths(); !errors.Is(err, bad) {
		t.Errorf("ошибка приложения: %v", err)
	}
	if _, err := v.Paths(); err == nil {
		t.Error("относительный путь принят")
	}
	if p, err := v.Paths(); err != nil || !reflect.DeepEqual(p, []string{"/ok/a"}) {
		t.Errorf("третья попытка: %v %v", p, err)
	}
	if calls.Load() != 3 {
		t.Errorf("вызовов %d, ждал 3", calls.Load())
	}
}

// Одновременные запросы (uri-list и gnome-copied-files от одного вставляющего)
// получают один ответ при одном обходе приложения.
func TestClipVirtual_ConcurrentSingleFlight(t *testing.T) {
	var calls atomic.Int32
	gate := make(chan struct{})
	v, _ := newClipVirtual([]widget.VirtualFile{{Name: "a"}}, nil, widget.VirtualFilesOptions{
		Materialize: func(int) (string, error) {
			calls.Add(1)
			<-gate
			return "/ok/a", nil
		},
	})
	var wg sync.WaitGroup
	res := make([][]string, 8)
	for i := range res {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i], _ = v.Paths()
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()
	for i, p := range res {
		if !reflect.DeepEqual(p, []string{"/ok/a"}) {
			t.Errorf("запрос %d получил %v", i, p)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("Materialize вызван %d раз, ждал 1", calls.Load())
	}
}

// Снятие обещания: WithRelease один раз, дальше пути не выдаются, а идущий
// обход останавливается между записями.
func TestClipVirtual_Drop(t *testing.T) {
	released := make(chan struct{}, 3)
	var calls atomic.Int32
	var v *clipVirtual
	v, _ = newClipVirtual([]widget.VirtualFile{{Name: "a"}, {Name: "b"}}, nil, widget.VirtualFilesOptions{
		Materialize: func(i int) (string, error) {
			calls.Add(1)
			v.drop() // обещание снято посреди обхода
			return "/ok", nil
		},
		Release: func() { released <- struct{}{} },
	})
	if _, err := v.Paths(); !errors.Is(err, widget.ErrVirtualFileNotFound) {
		t.Errorf("обход снятого обещания: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("после снятия приложение спрошено %d раз, ждал 1", calls.Load())
	}
	v.drop()
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("WithRelease не вызван")
	}
	select {
	case <-released:
		t.Error("WithRelease вызван дважды")
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := v.Paths(); !errors.Is(err, widget.ErrVirtualFileNotFound) {
		t.Errorf("Paths после снятия: %v", err)
	}
	(*clipVirtual)(nil).drop() // nil допустим
}

func TestClipVirtual_OpenFile(t *testing.T) {
	var v *clipVirtual
	if _, err := v.openFile(0); !errors.Is(err, widget.ErrVirtualFileNotFound) {
		t.Errorf("nil: %v", err)
	}
	v, _ = newClipVirtual([]widget.VirtualFile{{Name: "d", IsDir: true}, {Name: "f"}}, nil, widget.VirtualFilesOptions{
		Materialize: func(int) (string, error) { return "/x", nil },
	})
	if _, err := v.openFile(1); !errors.Is(err, widget.ErrVirtualFileNotFound) {
		t.Errorf("open не задан: %v", err)
	}
}
