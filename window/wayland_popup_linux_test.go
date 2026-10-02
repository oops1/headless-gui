//go:build linux && !android

package window

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

// Меню и выпадающие списки на Wayland рисовались ВНУТРИ окна и обрезались его
// краем: холст движка — это и есть окно. В узком окне (Блокнот WinLine,
// эталон 490 px) у меню «Файл» пропадал правый край и нижние пункты. Отдельным
// окном попап на Wayland быть не может — клиент не знает, где стоит его окно,
// — поэтому он создаётся как xdg_popup, поверхность-потомок носителя.

// popupImg — картинка кадра попапа нужного размера.
func popupImg(w, h int, a uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{R: 20, G: 20, B: 30, A: a}},
		image.Point{}, draw.Src)
	return img
}

func TestWaylandPopup_OpenSendsProtocol(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.inputSerial.Store(77)

	if err := c.w.OpenChildPopup(1, 40, 28, 200, 160); err != nil {
		t.Fatalf("OpenChildPopup: %v", err)
	}
	msgs := splitWlMsgs(t, c.drain(t))
	p := c.w.popups.byID[1]
	if p == nil {
		t.Fatal("попап не заведён")
	}

	if _, ok := findWlMsg(msgs, c.w.compositorID, wlCompositorCreateSurface); !ok {
		t.Error("поверхность попапа не создана")
	}
	posMsg, ok := findWlMsg(msgs, c.w.wmBaseID, xdgWmBaseCreatePositioner)
	if !ok || len(posMsg.args) != 1 {
		t.Fatalf("позиционер не создан, пришло: %+v", msgs)
	}
	posID := posMsg.args[0]

	// Позиционер: размер попапа и якорная точка в координатах окна.
	if m, ok := findWlMsg(msgs, posID, xdgPositionerSetSize); !ok ||
		len(m.args) != 2 || m.args[0] != 200 || m.args[1] != 160 {
		t.Errorf("set_size %v, ждал [200 160]", m.args)
	}
	if m, ok := findWlMsg(msgs, posID, xdgPositionerSetAnchorRect); !ok ||
		len(m.args) != 4 || m.args[0] != 40 || m.args[1] != 28 {
		t.Errorf("set_anchor_rect %v, ждал якорь в 40,28", m.args)
	}

	// get_popup: родитель — xdg_surface окна-носителя.
	gp, ok := findWlMsg(msgs, p.xdgSurfID, xdgSurfaceGetPopup)
	if !ok || len(gp.args) != 3 {
		t.Fatalf("get_popup не отправлен, пришло: %+v", msgs)
	}
	if gp.args[1] != c.w.xdgSurfaceID {
		t.Errorf("родитель попапа %d, ждал xdg_surface окна (%d)", gp.args[1], c.w.xdgSurfaceID)
	}

	// Захват ввода: с ним компоновщик сам закрывает меню по щелчку снаружи.
	grab, ok := findWlMsg(msgs, p.popupID, xdgPopupGrab)
	if !ok {
		t.Fatal("xdg_popup.grab не отправлен — меню не закроется щелчком мимо")
	}
	if len(grab.args) != 2 || grab.args[0] != 7 || grab.args[1] != 77 {
		t.Errorf("grab %v, ждал [7 77] (seat, serial ввода)", grab.args)
	}

	// Первый commit — без буфера: кадр показывается только после configure.
	if _, ok := findWlMsg(msgs, p.surfaceID, wlSurfaceAttach); ok {
		t.Error("буфер прикреплён до configure")
	}
}

// Пока жив попап с захватом ввода, следующие попапы обязаны быть его
// потомками: иначе компоновщик отвечает протокольной ошибкой, а это разрыв
// соединения.
func TestWaylandPopup_SecondIsChildOfFirst(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.inputSerial.Store(5)

	if err := c.w.OpenChildPopup(1, 40, 28, 100, 100); err != nil {
		t.Fatalf("первый попап: %v", err)
	}
	first := c.w.popups.byID[1]
	c.drain(t)

	if err := c.w.OpenChildPopup(2, 140, 68, 100, 100); err != nil {
		t.Fatalf("второй попап: %v", err)
	}
	msgs := splitWlMsgs(t, c.drain(t))
	second := c.w.popups.byID[2]
	if second == nil {
		t.Fatal("второй попап не заведён")
	}

	gp, ok := findWlMsg(msgs, second.xdgSurfID, xdgSurfaceGetPopup)
	if !ok || len(gp.args) != 3 {
		t.Fatalf("get_popup второго попапа не отправлен: %+v", msgs)
	}
	if gp.args[1] != first.xdgSurfID {
		t.Errorf("родитель второго попапа %d, ждал первый попап (%d)",
			gp.args[1], first.xdgSurfID)
	}
	// Якорь — в координатах родителя-попапа, а не окна.
	posMsg, ok := findWlMsg(msgs, c.w.wmBaseID, xdgWmBaseCreatePositioner)
	if !ok {
		t.Fatal("позиционер второго попапа не создан")
	}
	if m, ok := findWlMsg(msgs, posMsg.args[0], xdgPositionerSetAnchorRect); !ok ||
		int32(m.args[0]) != 100 || int32(m.args[1]) != 40 {
		t.Errorf("якорь второго попапа %v, ждал 100,40 относительно первого", m.args[:2])
	}
	// Захват — только у первого: второй его не повторяет.
	if _, ok := findWlMsg(msgs, second.popupID, xdgPopupGrab); ok {
		t.Error("второй попап запросил захват ввода повторно")
	}
}

// Кадр, пришедший до configure, придерживается: прикреплять буфер до ответа
// компоновщика протокол запрещает.
func TestWaylandPopup_BlitWaitsConfigure(t *testing.T) {
	c := newWlTestWindow(t)
	if err := c.w.OpenChildPopup(1, 0, 0, 64, 48); err != nil {
		t.Fatalf("OpenChildPopup: %v", err)
	}
	p := c.w.popups.byID[1]
	c.drain(t)

	c.w.BlitChildPopup(1, popupImg(64, 48, 255), nil)
	if p.pending == nil {
		t.Error("кадр до configure не придержан")
	}

	// Компоновщик ответил на первый commit — попап можно показывать.
	c.w.handleEvent(p.xdgSurfID, xdgSurfaceEvConfigure, wlU32(99))
	msgs := splitWlMsgs(t, c.drain(t))

	if ack, ok := findWlMsg(msgs, p.xdgSurfID, xdgSurfaceAckConfigure); !ok || ack.args[0] != 99 {
		t.Errorf("ack_configure не отправлен или не с тем serial: %+v", msgs)
	}
	if _, ok := findWlMsg(msgs, p.surfaceID, wlSurfaceAttach); !ok {
		t.Error("кадр не прикреплён после configure")
	}
	if _, ok := findWlMsg(msgs, p.surfaceID, wlSurfaceCommit); !ok {
		t.Error("поверхность попапа не закоммичена")
	}
}

// Пока курсор над попапом, ввод принадлежит ему, а не окну: события
// motion/button приходят без поверхности, её сообщает только enter.
func TestWaylandPopup_PointerRoutedToPopup(t *testing.T) {
	c := newWlTestWindow(t)
	if err := c.w.OpenChildPopup(1, 10, 10, 80, 60); err != nil {
		t.Fatalf("OpenChildPopup: %v", err)
	}
	p := c.w.popups.byID[1]
	c.drain(t)

	var toWindow int
	c.w.onMouseMove = func(x, y int) { toWindow++ }
	c.w.onMouseButton = func(x, y, b int, pressed bool) { toWindow++ }

	var gotID uintptr
	var gotX, gotY int
	var clicked bool
	c.w.SetChildPopupHandlers(childPopupHandlers{
		Move:   func(id uintptr, x, y int) { gotID, gotX, gotY = id, x, y },
		Button: func(id uintptr, x, y, b int, pressed bool) { clicked = pressed },
	})

	// enter на поверхность попапа, затем движение и щелчок.
	enter := append(wlU32(1, p.surfaceID), wlFixed(12, 20)...)
	c.w.handlePointer(wlPointerEvEnter, enter)
	c.w.handlePointer(wlPointerEvMotion, append(wlU32(0), wlFixed(30, 42)...))
	c.w.handlePointer(wlPointerEvButton, wlU32(2, 0, btnLeft, 1))

	if gotID != 1 || gotX != 30 || gotY != 42 {
		t.Errorf("движение дошло как id=%d %d,%d; ждал id=1 30,42", gotID, gotX, gotY)
	}
	if !clicked {
		t.Error("щелчок не дошёл до попапа")
	}
	if toWindow != 0 {
		t.Errorf("%d событий ушло окну, хотя курсор над попапом", toWindow)
	}

	// Курсор вернулся в окно — ввод снова его.
	c.w.handlePointer(wlPointerEvEnter, append(wlU32(3, c.w.surfaceID), wlFixed(5, 5)...))
	c.w.handlePointer(wlPointerEvMotion, append(wlU32(0), wlFixed(6, 6)...))
	if toWindow != 1 {
		t.Errorf("после возврата курсора окно получило %d событий, ждал 1", toWindow)
	}
}

// Щелчок мимо меню компоновщик обрабатывает сам и присылает popup_done.
// Движок должен об этом узнать, иначе оверлей останется открытым без окна.
func TestWaylandPopup_DoneReported(t *testing.T) {
	c := newWlTestWindow(t)
	if err := c.w.OpenChildPopup(7, 0, 0, 40, 40); err != nil {
		t.Fatalf("OpenChildPopup: %v", err)
	}
	p := c.w.popups.byID[7]
	c.drain(t)

	var done uintptr
	c.w.SetChildPopupHandlers(childPopupHandlers{Done: func(id uintptr) { done = id }})
	c.w.handleEvent(p.popupID, xdgPopupEvPopupDone, nil)

	if done != 7 {
		t.Errorf("о закрытии сообщили как о %d, ждал 7", done)
	}
}

// Дыры каскадного меню не должны ловить щелчок: он должен считаться щелчком
// снаружи — тогда компоновщик закроет меню, как закрывает системные.
func TestWaylandPopup_InputRegionFromBands(t *testing.T) {
	c := newWlTestWindow(t)
	if err := c.w.OpenChildPopup(1, 0, 0, 64, 48); err != nil {
		t.Fatalf("OpenChildPopup: %v", err)
	}
	p := c.w.popups.byID[1]
	c.w.handleEvent(p.xdgSurfID, xdgSurfaceEvConfigure, wlU32(1))
	c.drain(t)

	bands := []image.Rectangle{image.Rect(0, 0, 64, 20), image.Rect(20, 20, 64, 48)}
	c.w.BlitChildPopup(1, popupImg(64, 48, 255), bands)
	msgs := splitWlMsgs(t, c.drain(t))

	if _, ok := findWlMsg(msgs, c.w.compositorID, wlCompositorCreateRegion); !ok {
		t.Fatalf("область не создана, пришло: %+v", msgs)
	}
	set, ok := findWlMsg(msgs, p.surfaceID, wlSurfaceSetInputRegion)
	if !ok || len(set.args) != 1 {
		t.Fatal("область ввода не задана — щелчок в дыру попадёт в меню")
	}
	// Полосы складываются в ту самую область, которую повесили на попап.
	region := set.args[0]
	var adds int
	for _, m := range msgs {
		if m.obj == region && m.opcode == wlRegionAdd {
			adds++
		}
	}
	if adds != len(bands) {
		t.Errorf("в области ввода %d полос, ждал %d", adds, len(bands))
	}
}

func TestWaylandPopup_CloseDestroysObjects(t *testing.T) {
	c := newWlTestWindow(t)
	if err := c.w.OpenChildPopup(1, 0, 0, 32, 32); err != nil {
		t.Fatalf("OpenChildPopup: %v", err)
	}
	p := c.w.popups.byID[1]
	c.drain(t)

	c.w.CloseChildPopup(1)
	msgs := splitWlMsgs(t, c.drain(t))

	for _, want := range []struct {
		obj    uint32
		opcode uint16
		name   string
	}{
		{p.popupID, xdgPopupDestroy, "xdg_popup"},
		{p.xdgSurfID, xdgSurfaceDestroy, "xdg_surface"},
		{p.surfaceID, wlSurfaceDestroy, "wl_surface"},
		{p.bufID, wlBufferDestroy, "wl_buffer"},
		{p.poolID, wlShmPoolDestroy, "wl_shm_pool"},
	} {
		if _, ok := findWlMsg(msgs, want.obj, want.opcode); !ok {
			t.Errorf("%s не уничтожен", want.name)
		}
	}
	if c.w.popups.byID[1] != nil {
		t.Error("попап остался в реестре")
	}
	if c.w.popups.root != nil {
		t.Error("закрытый попап остался корневым — следующий станет его потомком")
	}
}

// ─── Хелперы ───────────────────────────────────────────────────────────────

// wlU32 — тело события из слов.
func wlU32(vals ...uint32) []byte {
	b := make([]byte, 0, len(vals)*4)
	for _, v := range vals {
		var w [4]byte
		binary.LittleEndian.PutUint32(w[:], v)
		b = append(b, w[:]...)
	}
	return b
}

// wlFixed — координаты в формате wl_fixed (24.8).
func wlFixed(x, y int) []byte {
	return wlU32(uint32(int32(x)<<8), uint32(int32(y)<<8))
}

// drain читает всё, что окно успело отправить, до тишины в сокете.
//
// Одного Read мало: создание попапа — это десяток сообщений, а буфер
// передаётся отдельным вызовом (с файловым дескриптором), и поток в сокете
// рвётся на части.
func (c *wlTestConn) drain(t *testing.T) []byte {
	t.Helper()
	var out []byte
	buf := make([]byte, 8192)
	for {
		c.peer.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		n, err := c.peer.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			break
		}
	}
	if len(out) == 0 {
		t.Fatal("компоновщик ничего не получил")
	}
	return out
}
