//go:build linux && !android

package x11

import (
	"encoding/binary"
	"errors"
	"github.com/oops1/headless-gui/v3/widget/base"
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"time"
)

// helpers_linux_test.go — копии помощников тестов window/core: тестовые
// помощники между пакетами не делятся.

const (
	fmtsText  = base.ClipboardFormatText
	fmtsHTML  = base.ClipboardFormatHTML
	fmtsImage = base.ClipboardFormatImage
	fmtsFiles = base.ClipboardFormatFiles
)

// virtApp — «приложение»: Materialize считает вызовы и отдаёт пути.
type virtApp struct {
	calls    atomic.Int32
	fail     atomic.Bool
	released chan struct{}
	files    []base.VirtualFile
}

func newVirtApp() *virtApp {
	return &virtApp{released: make(chan struct{}, 4), files: []base.VirtualFile{
		{Name: "Отчёт.txt", Size: 3},
		{Name: "Папка", IsDir: true},
		{Name: "Папка/в.txt", Size: 1},
	}}
}

func (a *virtApp) opts() base.VirtualFilesOptions {
	return base.VirtualFilesOptions{
		Materialize: func(i int) (string, error) {
			a.calls.Add(1)
			if a.fail.Load() {
				return "", errors.New("не скачалось")
			}
			return "/home/u/Загрузки/" + a.files[i].Name, nil
		},
		Release: func() { a.released <- struct{}{} },
	}
}

func (a *virtApp) waitRelease(t *testing.T, why string) {
	t.Helper()
	select {
	case <-a.released:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: WithRelease не вызван", why)
	}
}

func (a *virtApp) noRelease(t *testing.T, why string) {
	t.Helper()
	select {
	case <-a.released:
		t.Fatalf("%s: WithRelease вызван", why)
	case <-time.After(30 * time.Millisecond):
	}
}

const (
	virtWantURIs  = "file:///home/u/%D0%97%D0%B0%D0%B3%D1%80%D1%83%D0%B7%D0%BA%D0%B8/%D0%9E%D1%82%D1%87%D1%91%D1%82.txt\r\nfile:///home/u/%D0%97%D0%B0%D0%B3%D1%80%D1%83%D0%B7%D0%BA%D0%B8/%D0%9F%D0%B0%D0%BF%D0%BA%D0%B0\r\n"
	virtWantGnome = "copy\nfile:///home/u/%D0%97%D0%B0%D0%B3%D1%80%D1%83%D0%B7%D0%BA%D0%B8/%D0%9E%D1%82%D1%87%D1%91%D1%82.txt\nfile:///home/u/%D0%97%D0%B0%D0%B3%D1%80%D1%83%D0%B7%D0%BA%D0%B8/%D0%9F%D0%B0%D0%BF%D0%BA%D0%B0"
	virtWantNames = "Отчёт.txt\nПапка\nПапка/в.txt"
)

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// testClipPicture — картинка с прозрачностью: проверяет, что альфа доезжает.
func testClipPicture() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(60 * x), uint8(90 * y), 128, 255})
		}
	}
	img.SetNRGBA(1, 1, color.NRGBA{200, 100, 50, 77})
	return img
}

func samePicture(t *testing.T, got image.Image, want *image.NRGBA) {
	t.Helper()
	if got == nil || got.Bounds().Dx() != want.Bounds().Dx() || got.Bounds().Dy() != want.Bounds().Dy() {
		t.Fatalf("размер %v, ждал %v", got, want.Bounds())
	}
	for y := 0; y < want.Bounds().Dy(); y++ {
		for x := 0; x < want.Bounds().Dx(); x++ {
			g := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
			if w := want.NRGBAAt(x, y); g != w {
				t.Errorf("(%d,%d) = %v, ждал %v", x, y, g, w)
			}
		}
	}
}

// wheelCall — сырое или пиксельное колесо, как его видит приложение (тесты
// трёх бэкендов).
type wheelCall struct {
	x, y    int
	dx, dy  float64
	precise bool
}

func fsWaitFor(cond func() bool) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

type mouseCallLinux struct {
	x, y, b int
	p       bool
}

func atomsBytes(atoms ...uint32) []byte {
	b := make([]byte, 0, len(atoms)*4)
	for _, a := range atoms {
		b = binary.LittleEndian.AppendUint32(b, a)
	}
	return b
}
