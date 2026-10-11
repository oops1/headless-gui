//go:build windows

package window

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"
)

// Сквозная проверка с НАСТОЯЩИМ OLE: два окна в одном процессе, настоящее
// перетаскивание мышью (SendInput) из одного в другое. Двигает курсор
// пользователя, поэтому выключена по умолчанию:
//
//	HEADLESS_GUI_DND_E2E=1 go test -run TestDndE2E ./window
//
// Здесь, а не в обычных тестах, проверяется то, что подделкой не воспроизвести:
// как OLE реально вызывает методы (соглашение о вызовах, POINTL по значению,
// порядок слотов), как ведёт себя DoDragDrop с нашими IDataObject/IDropSource и
// что приходит в IDropTarget окна.

var procSendInput = user32.NewProc("SendInput")
var procSetCursorPos = user32.NewProc("SetCursorPos")

type mouseInput struct {
	typ         uint32
	_           uint32
	dx, dy      int32
	mouseData   uint32
	flags       uint32
	time        uint32
	_           uint32
	dwExtraInfo uintptr
}

const (
	miLeftDown = 0x0002
	miLeftUp   = 0x0004
)

//go:uintptrescapes
func sendMouse(flags uint32) {
	in := mouseInput{typ: 0, flags: flags}
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func moveCursor(x, y int) { procSetCursorPos.Call(uintptr(x), uintptr(y)) }

func TestDndE2E_DragBetweenWindows(t *testing.T) {
	if os.Getenv("HEADLESS_GUI_DND_E2E") != "1" {
		t.Skip("включается HEADLESS_GUI_DND_E2E=1: двигает курсор пользователя")
	}

	files := []string{
		filepath.Join(t.TempDir(), "один.png"),
		filepath.Join(t.TempDir(), "two words.jpg"),
	}

	var (
		mu       sync.Mutex
		log      []string
		dropped  [][]string
		finished = make(chan DragEffect, 1)
		ready    = make(chan struct{})
		loopDone = make(chan struct{})
		a, b     *Win32Window
		bx, by   int
	)
	note := func(s string) { mu.Lock(); log = append(log, s); mu.Unlock() }

	go func() {
		runtime.LockOSThread()
		defer close(loopDone)
		a, b = &Win32Window{}, &Win32Window{}
		if err := a.Create("A", 300, 200); err != nil {
			t.Errorf("Create A: %v", err)
			close(ready)
			return
		}
		if err := b.Create("B", 300, 200); err != nil {
			t.Errorf("Create B: %v", err)
			close(ready)
			return
		}
		b.noQuit = true
		a.SetPosition(50, 80)
		b.SetPosition(450, 80)
		// Поверх всех: иначе окно чужой программы может оказаться под
		// курсором и перехватить перетаскивание.
		for _, w := range []*Win32Window{a, b} {
			const swpNoSizeMoveActivate = 0x0001 | 0x0002 | 0x0010
			procSetWindowPos.Call(uintptr(w.hwnd), ^uintptr(0), 0, 0, 0, 0, swpNoSizeMoveActivate)
		}
		b.SetOnDragEnter(func(x, y int) { note("enter") })
		b.SetOnDragOver(func(x, y int) bool { note("over"); return true })
		b.SetOnDragLeave(func() { note("leave") })
		b.SetOnFilesDropped(func(p []string, x, y int) {
			mu.Lock()
			dropped = append(dropped, p)
			mu.Unlock()
			note("drop")
		})
		bx, by = 450+150, 80+100
		close(ready)
		a.RunEventLoop()
	}()
	<-ready
	if a == nil || b == nil || a.hwnd == 0 || b.hwnd == 0 {
		t.Fatal("окна не созданы")
	}
	if !a.oleReady || b.dropTarget == nil {
		t.Fatalf("OLE не поднят: a.ole=%v b.target=%v", a.oleReady, b.dropTarget != nil)
	}
	t.Cleanup(func() {
		b.Close()
		a.Close()
		select {
		case <-loopDone:
		case <-time.After(3 * time.Second):
		}
	})

	// Зажимаем кнопку над окном A, начинаем перетаскивание, ведём к B, отпускаем.
	moveCursor(50+150, 80+100)
	time.Sleep(150 * time.Millisecond)
	sendMouse(miLeftDown)
	time.Sleep(100 * time.Millisecond)
	if !a.BeginDragFiles(files, DragEffectCopy|DragEffectMove, func(e DragEffect) { finished <- e }) {
		sendMouse(miLeftUp)
		t.Fatal("BeginDragFiles отказал при зажатой кнопке")
	}
	time.Sleep(300 * time.Millisecond)
	for i := 1; i <= 20; i++ {
		moveCursor(200+(bx-200)*i/20, 180+(by-180)*i/20)
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	sendMouse(miLeftUp)

	{
		var pt point
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		wfp, _, _ := user32.NewProc("WindowFromPoint").Call(uintptr(pt.X) | uintptr(pt.Y)<<32)
		var ra, rb rect
		procGetWindowRect.Call(uintptr(a.hwnd), uintptr(unsafe.Pointer(&ra)))
		procGetWindowRect.Call(uintptr(b.hwnd), uintptr(unsafe.Pointer(&rb)))
		t.Logf("cursor=%v WindowFromPoint=%#x a=%#x%v b=%#x%v", pt, wfp, a.hwnd, ra, b.hwnd, rb)
	}
	select {
	case eff := <-finished:
		if eff != DragEffectCopy {
			t.Errorf("итоговое действие %v, ждал copy", eff)
		}
	case <-time.After(5 * time.Second):
		mu.Lock()
		l := append([]string(nil), log...)
		mu.Unlock()
		sendMouse(miLeftUp)
		t.Fatalf("DoDragDrop не завершился; события приёмника: %v", l)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(log) < 3 || log[0] != "enter" || log[len(log)-1] != "drop" {
		t.Errorf("события приёмника: %v, ждал enter, over..., drop", log)
	}
	if len(dropped) != 1 || !reflect.DeepEqual(dropped[0], files) {
		t.Errorf("сброшено %q, ждал %q", dropped, files)
	}
}
