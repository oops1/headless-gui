package widget

import (
	"image"
	"testing"
)

// Под Wayland окно не знает своей позиции и задать её не может: перемещение и
// ресайз ведёт компоновщик, клиент лишь просит начать их с текущего нажатия.
// Поэтому у окна есть два хука — OnNativeMove и OnNativeResize. Хост, который
// их не задал (Win32, X11), работает как раньше: дельты через OnDragMove, а
// край окна в нативном режиме выключен.

// nativeWin — окно в нативном режиме с разрешённым ресайзом.
func nativeWin() *Window {
	w := NewWindow("Окно", 400, 300)
	w.Resize = ResizeModeCanResize
	w.SetBounds(image.Rect(0, 0, 400, 300))
	w.SetNativeHosted(true)
	return w
}

func pressWin(w *Window, x, y int) bool {
	return w.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: true, X: x, Y: y})
}

func TestWindow_NativeMove_TitleDragAsksHost(t *testing.T) {
	w := nativeWin()
	moves, drags := 0, 0
	w.OnNativeMove = func() bool { moves++; return true }
	w.OnDragMove = func(dx, dy int) { drags++ }

	tb := w.titleBarRect()
	if tb.Empty() {
		t.Fatal("у окна нет полосы заголовка — не за что тащить")
	}
	if !pressWin(w, tb.Min.X+tb.Dx()/2, tb.Min.Y+tb.Dy()/2) {
		t.Fatal("нажатие на заголовок не обработано")
	}
	if moves != 1 {
		t.Errorf("хост спросили %d раз, ждал один", moves)
	}
	// Перемещение ведёт система: своего перетаскивания окно не начинает, и
	// дельты никуда не идут.
	if w.dragging {
		t.Error("окно осталось в собственном перетаскивании")
	}
	w.OnMouseMove(tb.Min.X+40, tb.Min.Y+20)
	if drags != 0 {
		t.Errorf("OnDragMove позвали %d раз, хотя окно тащит система", drags)
	}
}

func TestWindow_NativeMove_FalseKeepsOldBehaviour(t *testing.T) {
	w := nativeWin()
	w.OnNativeMove = func() bool { return false } // хост не смог
	drags := 0
	w.OnDragMove = func(dx, dy int) { drags++ }

	tb := w.titleBarRect()
	pressWin(w, tb.Min.X+tb.Dx()/2, tb.Min.Y+tb.Dy()/2)
	if !w.dragging {
		t.Fatal("отказ хоста не вернул окно к перетаскиванию дельтами")
	}
	w.OnMouseMove(tb.Min.X+tb.Dx()/2+15, tb.Min.Y+tb.Dy()/2+7)
	if drags == 0 {
		t.Error("OnDragMove не позвали")
	}
}

func TestWindow_NativeResize_EdgesReportedToHost(t *testing.T) {
	b := image.Rect(0, 0, 400, 300)
	cases := []struct {
		name string
		x, y int
		want int
	}{
		{"левый край", b.Min.X + 1, b.Min.Y + 150, NativeEdgeLeft},
		{"правый край", b.Max.X - 1, b.Min.Y + 150, NativeEdgeRight},
		{"верхний край", b.Min.X + 200, b.Min.Y, NativeEdgeTop},
		{"нижний край", b.Min.X + 200, b.Max.Y - 1, NativeEdgeBottom},
		{"левый верхний угол", b.Min.X, b.Min.Y, NativeEdgeTop | NativeEdgeLeft},
		{"правый нижний угол", b.Max.X - 1, b.Max.Y - 1, NativeEdgeBottom | NativeEdgeRight},
	}
	for _, c := range cases {
		w := nativeWin()
		got := -1
		w.OnNativeResize = func(edges int) bool { got = edges; return true }
		if !pressWin(w, c.x, c.y) {
			t.Errorf("%s: нажатие не обработано", c.name)
			continue
		}
		if got != c.want {
			t.Errorf("%s: хосту сообщили края %d, ждал %d", c.name, got, c.want)
		}
		if w.resizing {
			t.Errorf("%s: окно начало собственный ресайз, хотя размер меняет система", c.name)
		}
	}
}

func TestWindow_NativeResize_CursorOverEdge(t *testing.T) {
	w := nativeWin()

	// Без хука край в нативном режиме выключен: размер ведёт ОС своими
	// средствами, и курсор ресайза там показывать нечего (прежнее поведение).
	if got := w.Cursor(1, 150); got != CursorArrow {
		t.Errorf("без хука курсор у края %v, ждал стрелку", got)
	}

	w.OnNativeResize = func(edges int) bool { return true }
	if got := w.Cursor(1, 150); got != CursorSizeWE {
		t.Errorf("курсор у левого края %v, ждал CursorSizeWE", got)
	}
	if got := w.Cursor(399, 299); got != CursorSizeNWSE {
		t.Errorf("курсор в правом нижнем углу %v, ждал CursorSizeNWSE", got)
	}
}

func TestWindow_NativeResize_RefusalDoesNotResizeWidget(t *testing.T) {
	w := nativeWin()
	w.OnNativeResize = func(edges int) bool { return false } // хост не смог
	before := w.Bounds()

	if !pressWin(w, 399, 150) {
		t.Fatal("нажатие на край не обработано")
	}
	// Окно живёт в окне ОС: менять свои границы самим — рассинхрон с ним.
	if w.resizing {
		t.Error("окно начало собственный ресайз внутри нативного окна")
	}
	w.OnMouseMove(450, 150)
	if w.Bounds() != before {
		t.Errorf("границы поехали: было %v, стало %v", before, w.Bounds())
	}
}

func TestWindow_NativeEdgeBits(t *testing.T) {
	// Биты для хоста не зависят от порядка внутренних констант.
	cases := []struct {
		e    winEdge
		want int
	}{
		{edgeNone, 0},
		{edgeN, NativeEdgeTop},
		{edgeS, NativeEdgeBottom},
		{edgeW, NativeEdgeLeft},
		{edgeE, NativeEdgeRight},
		{edgeN | edgeW, NativeEdgeTop | NativeEdgeLeft},
		{edgeS | edgeE, NativeEdgeBottom | NativeEdgeRight},
	}
	for _, c := range cases {
		if got := nativeEdgeBits(c.e); got != c.want {
			t.Errorf("nativeEdgeBits(%d) = %d, ждал %d", c.e, got, c.want)
		}
	}
}
