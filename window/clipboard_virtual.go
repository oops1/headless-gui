package window

// clipboard_virtual.go — виртуальные файлы в буфере обмена X11 и Wayland:
// обещание, общее для обоих владельцев выделения (тестируется в общем прогоне,
// без соединения).
//
// Протокола «виртуальных файлов» на Linux нет: вставляющему нужны настоящие
// пути (text/uri-list, x-special/gnome-copied-files). Движок пути не делает —
// он спрашивает их у приложения (widget.WithMaterialize) в момент запроса
// этих форматов и отдаёт то, что приложение вернуло. Никаких временных папок,
// скачивания и очистки здесь нет: куда класть файлы и когда убирать (WithRelease),
// решает приложение.

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/oops1/headless-gui/v3/widget"
)

// clipVirtual — обещание виртуальных файлов, которым мы владеем как владелец
// выделения.
type clipVirtual struct {
	files []widget.VirtualFile
	top   []int // индексы записей верхнего уровня: пути просят только у них
	open  func(int) (io.ReadCloser, error)
	opts  widget.VirtualFilesOptions

	mu      sync.Mutex
	busy    chan struct{} // не nil, пока кто-то зовёт колбэки; закрывается по окончании
	paths   []string      // готовый ответ; nil — ещё не получен
	dropped bool
}

// newClipVirtual проверяет обещание: без Materialize файлов на Linux не
// предложить, а каталог верхнего уровня должен быть в списке явной записью.
func newClipVirtual(files []widget.VirtualFile, open func(int) (io.ReadCloser, error), opts widget.VirtualFilesOptions) (*clipVirtual, error) {
	if opts.Materialize == nil {
		return nil, widget.ErrClipboardVirtualNoSource
	}
	top, err := widget.VirtualTopLevel(files)
	if err != nil {
		return nil, err
	}
	return &clipVirtual{files: files, top: top, open: open, opts: opts}, nil
}

// names — относительные имена простым текстом (для приложений, которые берут
// только текст).
func (v *clipVirtual) names() string {
	names := make([]string, len(v.files))
	for i, f := range v.files {
		names[i] = f.Name
	}
	return strings.Join(names, "\n")
}

// Paths спрашивает у приложения пути записей верхнего уровня (по порядку
// списка) и запоминает их до снятия обещания. Параллельные запросы ждут один
// и тот же ответ: приложению не приходится материализовать одно и то же дважды.
// Ошибка колбэка не запоминается — следующая вставка попробует снова.
func (v *clipVirtual) Paths() ([]string, error) {
	for {
		v.mu.Lock()
		switch {
		case v.dropped:
			v.mu.Unlock()
			return nil, widget.ErrVirtualFileNotFound
		case v.paths != nil:
			p := append([]string(nil), v.paths...)
			v.mu.Unlock()
			return p, nil
		case v.busy != nil:
			wait := v.busy
			v.mu.Unlock()
			<-wait
			continue
		}
		v.busy = make(chan struct{})
		v.mu.Unlock()

		paths, err := v.materialize()

		v.mu.Lock()
		if err == nil && !v.dropped {
			v.paths = paths
		}
		close(v.busy)
		v.busy = nil
		dropped := v.dropped
		v.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if dropped {
			return nil, widget.ErrVirtualFileNotFound
		}
		return append([]string(nil), paths...), nil
	}
}

// materialize зовёт колбэк приложения для каждой записи верхнего уровня.
func (v *clipVirtual) materialize() ([]string, error) {
	paths := make([]string, 0, len(v.top))
	for _, i := range v.top {
		v.mu.Lock()
		dropped := v.dropped
		v.mu.Unlock()
		if dropped {
			return nil, widget.ErrVirtualFileNotFound
		}
		p, err := v.opts.Materialize(i)
		if err != nil {
			return nil, fmt.Errorf("window: материализация %q: %w", v.files[i].Name, err)
		}
		// text/uri-list несёт только абсолютные пути: относительный вставляющий
		// понял бы по-своему.
		if !strings.HasPrefix(p, "/") {
			return nil, fmt.Errorf("window: материализация %q вернула не абсолютный путь %q", v.files[i].Name, p)
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// drop снимает обещание и сообщает об этом приложению (один раз, в своей
// горутине). nil допустим.
func (v *clipVirtual) drop() {
	if v == nil {
		return
	}
	v.mu.Lock()
	first := !v.dropped
	v.dropped = true
	v.mu.Unlock()
	if first && v.opts.Release != nil {
		go v.opts.Release()
	}
}

// openFile открывает содержимое файла index, если приложение дало open.
func (v *clipVirtual) openFile(index int) (io.ReadCloser, error) {
	if v == nil || v.open == nil || index < 0 || index >= len(v.files) || v.files[index].IsDir {
		return nil, widget.ErrVirtualFileNotFound
	}
	return v.open(index)
}
