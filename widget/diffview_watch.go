package widget

import (
	"image"
	"os"
	"strings"
	"time"
)

// diffview_watch.go — слежение за файлами на диске, приём файлов из
// проводника и доступность.

// SetWatchFiles включает опрос файлов сторон раз в секунду: правка файла извне
// приходит событием OnFileChangedOnDisk (и командой FileChangedCommand) в потоке
// движка.
func (d *DiffView) SetWatchFiles(on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !on {
		if d.watchStop != nil {
			close(d.watchStop)
			d.watchStop = nil
		}
		return
	}
	if d.watchStop != nil {
		return
	}
	stop := make(chan struct{})
	d.watchStop = stop
	go d.watch(stop)
}

// Close останавливает фоновую работу контрола.
func (d *DiffView) Close() { d.SetWatchFiles(false) }

func (d *DiffView) watch(stop chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		for i := range d.docs {
			d.checkDisk(DiffSide(i))
		}
	}
}

func (d *DiffView) checkDisk(side DiffSide) {
	d.mu.Lock()
	s := d.docs[side]
	path, t0, n0 := s.path, s.diskTime, s.diskSize
	d.mu.Unlock()
	if path == "" || t0.IsZero() {
		return
	}
	st, err := os.Stat(path)
	deleted := err != nil
	if !deleted && st.ModTime().Equal(t0) && st.Size() == n0 {
		return
	}
	d.mu.Lock()
	// Сторона сменилась или отметку уже обновило своё же сохранение.
	if d.docs[side] != s || s.path != path ||
		(!deleted && st.ModTime().Equal(s.diskTime) && st.Size() == s.diskSize) {
		d.mu.Unlock()
		return
	}
	// Отметка сдвигается сразу: об одной правке сообщаем один раз.
	if deleted {
		s.diskTime = time.Time{}
	} else {
		s.diskTime, s.diskSize = st.ModTime(), st.Size()
	}
	f, post := d.OnFileChangedOnDisk, d.post
	cmd := d.commands["FileChangedCommand"]
	d.mu.Unlock()
	if f == nil && cmd == nil {
		return
	}
	call := func() {
		if f != nil {
			f(side, path, deleted)
		}
		if ev := (DiffFileChange{Side: side, Path: path, Deleted: deleted}); cmd != nil && cmd.CanExecute(ev) {
			cmd.Execute(ev)
		}
	}
	// Наблюдатель — фоновая горутина, а обработчик почти наверняка тронет UI
	// (перечитает сторону, покажет вопрос). Без движка (тесты, контрол вне
	// дерева) зовём прямо.
	if post != nil {
		post(call)
	} else {
		call()
	}
}

// OnFilesDropped — контракт FileDropTarget: файлы из проводника. Два файла —
// в обе стороны, один — в сторону под курсором.
func (d *DiffView) OnFilesDropped(x, y int, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	if len(paths) >= 2 {
		d.LoadFile(DiffLeft, paths[0])
		d.LoadFile(DiffRight, paths[1])
		return true
	}
	g := d.geom()
	side := DiffLeft
	if x >= (g.lx1+g.rx0)/2 {
		side = DiffRight
	}
	d.LoadFile(side, paths[0])
	return true
}

// ─── Доступность ────────────────────────────────────────────────────────────

// AccessInfo — контракт Accessible: контрол целиком — группа «сравнение
// файлов».
func (d *DiffView) AccessInfo() AccessInfo {
	return AccessInfo{Role: RoleGroup, Name: Tr("diff.a11y.view"), Bounds: d.Bounds()}
}

// AccessChildren — контракт AccessChildrenProvider: две панели как два
// текстовых элемента. Без них скринридер видел бы одну безымянную группу и не
// мог прочитать ни строчки — текст контрол рисует сам, а не детьми-виджетами.
func (d *DiffView) AccessChildren() []AccessInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	g := d.geom()
	out := make([]AccessInfo, 0, 2)
	for i, s := range d.docs {
		name := s.title
		if name == "" {
			name = Tr("diff.empty")
		}
		key, x0, x1 := "diff.a11y.left", g.lx0, g.lx1
		if i == 1 {
			key, x0, x1 = "diff.a11y.right", g.rx0, g.rx1
		}
		info := AccessInfo{
			Role:   RoleTextInput,
			Name:   Trf(key, name),
			Value:  strings.Join(s.text.Lines, "\n"),
			Bounds: image.Rect(x0, g.cy0, x1, g.cy1),
		}
		if s.readOnly {
			info.States = append(info.States, StateReadOnly)
		}
		if d.focused && d.active == DiffSide(i) {
			info.States = append(info.States, StateFocused)
		}
		out = append(out, info)
	}
	return out
}
