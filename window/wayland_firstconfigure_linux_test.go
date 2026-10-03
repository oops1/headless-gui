//go:build linux

package window

import (
	"encoding/binary"
	"errors"
	"image"
	"net"
	"os"
	"testing"
	"time"
)

// readAllWl собирает всё, что окно отправило, пока провод не затих.
func (c *wlTestConn) readAllWl(t *testing.T) []wlWireMsg {
	t.Helper()
	var all []byte
	buf := make([]byte, 1<<16)
	for {
		c.peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, err := c.peer.Read(buf)
		all = append(all, buf[:n]...)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() || errors.Is(err, os.ErrDeadlineExceeded) {
				break
			}
			t.Fatalf("чтение провода: %v", err)
		}
	}
	return splitWlMsgs(t, all)
}

func toplevelConfigure(w, h int) []byte {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b[0:4], uint32(int32(w)))
	binary.LittleEndian.PutUint32(b[4:8], uint32(int32(h)))
	// states: пустой массив
	return b
}

// newFirstConfigureWindow — окно сразу после wl_surface и области просмотра,
// холст программы 1097×680, как у блокнота WinLine.
func newFirstConfigureWindow(t *testing.T) *wlTestConn {
	c := newWlTestWindow(t)
	c.w.width, c.w.height = 1097, 680
	c.w.scale.viewportID = 40
	return c
}

func destinations(msgs []wlWireMsg) [][]uint32 {
	var out [][]uint32
	for _, m := range msgs {
		if m.obj == 40 && m.opcode == wpViewportSetDestination {
			out = append(out, m.args)
		}
	}
	return out
}

// Компоновщик назначил размер в первом configure, ещё до кадра: окно
// сообщает его (ClientSize), кадр прежнего размера не показывает, а область
// просмотра назначается по размеру буфера. Раньше первым уходил буфер
// 1097×680 с назначением 960×516 — картинка сжималась, щелчки промахивались.
func TestWayland_FirstConfigureSize_ReachesFirstBuffer(t *testing.T) {
	c := newFirstConfigureWindow(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, toplevelConfigure(960, 516))

	if w, h := c.w.ClientSize(); w != 960 || h != 516 {
		t.Fatalf("ClientSize = %dx%d, ждали 960x516", w, h)
	}

	// Кадр, нарисованный под прежний холст, не коммитится.
	c.w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 1097, 680)))
	if c.w.hasFrame {
		t.Fatal("закоммичен кадр прежнего размера")
	}
	// Кадр нужного размера — первый показанный.
	c.w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 960, 516)))
	if !c.w.hasFrame || c.w.poolW != 960 || c.w.poolH != 516 {
		t.Fatalf("первый буфер %dx%d, ждали 960x516", c.w.poolW, c.w.poolH)
	}
	d := destinations(c.readAllWl(t))
	if len(d) == 0 {
		t.Fatal("set_destination не отправлен")
	}
	for _, a := range d {
		if a[0] != 960 || a[1] != 516 {
			t.Errorf("set_destination %v, ждали [960 516]", a)
		}
	}
}

// Configure 0×0 — «размер выбирает программа»: буфер в размере холста, и
// назначение того же размера.
func TestWayland_ZeroConfigure_KeepsCanvasSize(t *testing.T) {
	c := newFirstConfigureWindow(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, toplevelConfigure(0, 0))

	c.w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 1097, 680)))
	if !c.w.hasFrame {
		t.Fatal("кадр в размере холста не показан")
	}
	d := destinations(c.readAllWl(t))
	if len(d) == 0 {
		t.Fatal("set_destination не отправлен")
	}
	for _, a := range d {
		if a[0] != 1097 || a[1] != 680 {
			t.Errorf("set_destination %v, ждали [1097 680]", a)
		}
	}
}

// Ответ на ресайз так и не пришёл: окно всё равно показывается — не дольше
// maxStaleFrames кадров ожидания.
func TestWayland_StaleFrameSkip_IsBounded(t *testing.T) {
	c := newFirstConfigureWindow(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, toplevelConfigure(960, 516))
	for i := 0; i <= maxStaleFrames; i++ {
		c.w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 1097, 680)))
	}
	if !c.w.hasFrame {
		t.Fatal("окно так и не показалось")
	}
	// Даже в этом случае назначение — по буферу: окно не сжато.
	dd := destinations(c.readAllWl(t))
	if len(dd) == 0 {
		t.Fatal("set_destination не отправлен")
	}
	for _, a := range dd {
		if a[0] != 1097 || a[1] != 680 {
			t.Errorf("set_destination %v при буфере 1097x680", a)
		}
	}
}
