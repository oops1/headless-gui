//go:build linux && !android

package window

import (
	"image"
	"os"
	"testing"
	"time"
)

// TestX11ShmE2E — живая проверка MIT-SHM против настоящего X-сервера (Xvfb
// или WSLg/XWayland через DISPLAY). Если DISPLAY не задан или соединение не
// поднимается — тест пропускается (Skip), а не падает: в окружениях без
// X-сервера (обычный `go test ./...` без дисплея) это ожидаемо.
//
// Проверяемый инвариант — НЕ конкретный исход (SHM активен или fallback), а
// то, что оба исхода безопасны:
//   - Xvfb: ShmAttach реально проходит на сервере, кадры доходят через SHM,
//     ShmCompletion приходит и снимает busy — путь используется по назначению.
//   - WSLg/XWayland: клиент и Xwayland-процесс WSLg живут в разных IPC
//     namespace для SysV shm, поэтому серверный ShmAttach молча отклоняется.
//     ShmCompletion для этого сегмента никогда не придёт, busy останется
//     взведённым навсегда — и BlitRGBA обязана самостоятельно откатиться на
//     PutImage за каждый следующий кадр, а не зависнуть в ожидании сервера.
func TestX11ShmE2E(t *testing.T) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}

	w := &X11Window{}
	if err := w.Create("x11shm e2e", 64, 48); err != nil {
		t.Skipf("не удалось поднять окно на DISPLAY=%s: %v", display, err)
	}

	shmActive := w.shm != nil && !w.shm.fallback
	t.Logf("DISPLAY=%s: MIT-SHM активирован=%v", display, shmActive)

	loopDone := make(chan struct{})
	go func() {
		w.RunEventLoop() // должен читать ShmCompletion наравне с прочими событиями
		close(loopDone)
	}()

	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := range img.Pix {
		img.Pix[i] = 0xAA
	}

	// Серия кадров подряд: если SHM активен, первый уйдёт через него и
	// взведёт busy; пока ShmCompletion не пришёл, последующие обязаны сами
	// откатиться на PutImage, не дожидаясь сервера (см. x11ShmBlit).
	blitDone := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			w.BlitRGBA(img)
			time.Sleep(5 * time.Millisecond)
		}
		close(blitDone)
	}()

	select {
	case <-blitDone:
	case <-time.After(10 * time.Second):
		w.Close()
		t.Fatal("BlitRGBA завис — SHM-путь не должен блокировать рендер")
	}

	if shmActive {
		// Даём событийному циклу время дочитать ShmCompletion последнего
		// кадра — если сервер реально принял ShmAttach (Xvfb), busy рано или
		// поздно снимется. Если нет (WSLg/XWayland) — останется true навсегда,
		// это и есть ожидаемый негативный кейс, тест его не проваливает.
		cleared := false
		for i := 0; i < 40; i++ {
			if !w.shm.busy.Load() {
				cleared = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Logf("после серии кадров: ShmCompletion получен=%v (shmid=%d, %dx%d)", cleared, w.shm.shmid, w.shm.width, w.shm.height)
		if !cleared {
			t.Logf("busy не снялся — сервер, вероятно, отклонил ShmAttach в другом IPC namespace (ожидаемо для WSLg/XWayland); дальнейшие кадры идут через PutImage")
		}
	} else {
		t.Logf("SHM недоступен на этом сервере — используется PutImage (ожидаемо для XWayland/WSLg с чужим IPC namespace)")
	}

	w.Close()
	select {
	case <-loopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunEventLoop не завершился после Close()")
	}
}

// TestX11ShmE2EResize проверяет пересоздание SHM-сегмента при ресайзе окна
// (x11ShmResize из ConfigureNotify). Ресайз шлём НАПРЯМУЮ низкоуровневым
// x11ConfigureWindow, а не публичным SetSize: SetSize пишет w.width/w.height
// ДО отправки запроса, поэтому к моменту прихода своего же ConfigureNotify
// условие "newW != w.width" в handleX11Event уже ложно и ветка ресайза (в т.ч.
// onResize) не срабатывает — так эмулируется РЕАЛЬНЫЙ внешний ресайз (WM/
// перетаскивание рамки пользователем), где w.width на момент события ещё
// старый. Под голым Xvfb нет WM — ConfigureWindow применяется сразу же и
// возвращается тем же соединением как обычный (не synthetic) ConfigureNotify.
func TestX11ShmE2EResize(t *testing.T) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		t.Skip("DISPLAY не задан — нет X-сервера для e2e")
	}

	w := &X11Window{}
	if err := w.Create("x11shm e2e resize", 64, 48); err != nil {
		t.Skipf("не удалось поднять окно на DISPLAY=%s: %v", display, err)
	}
	defer w.Close()

	if w.shm == nil || w.shm.fallback {
		t.Skip("MIT-SHM недоступен на этом сервере — нечего проверять в ресайзе")
	}
	firstShmid := w.shm.shmid
	t.Logf("исходный сегмент: shmid=%d %dx%d", w.shm.shmid, w.shm.width, w.shm.height)

	loopDone := make(chan struct{})
	go func() {
		w.RunEventLoop()
		close(loopDone)
	}()

	w.x11ConfigureWindow(w.wid, 128, 96)

	// Ждём, пока ConfigureNotify дойдёт через event loop и x11ShmResize
	// пересоздаст сегмент под новый размер.
	resized := false
	for i := 0; i < 40; i++ {
		w.blitMu.Lock()
		ok := w.shm.width == 128 && w.shm.height == 96
		w.blitMu.Unlock()
		if ok {
			resized = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !resized {
		w.Close()
		<-loopDone
		t.Fatalf("сегмент не пересоздан под новый размер за отведённое время: shm.width=%d shm.height=%d", w.shm.width, w.shm.height)
	}
	if w.shm.fallback {
		t.Fatalf("после ресайза SHM неожиданно ушёл в fallback (shmid до=%d)", firstShmid)
	}
	if w.shm.shmid == firstShmid {
		t.Fatalf("shmid не изменился после ресайза (%d) — сегмент не пересоздан", firstShmid)
	}
	t.Logf("после ресайза: shmid=%d %dx%d (был %d)", w.shm.shmid, w.shm.width, w.shm.height, firstShmid)

	// Блит новым размером не должен падать/висеть после пересоздания сегмента.
	img := image.NewRGBA(image.Rect(0, 0, 128, 96))
	blitDone := make(chan struct{})
	go func() {
		w.BlitRGBA(img)
		close(blitDone)
	}()
	select {
	case <-blitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("BlitRGBA после ресайза завис")
	}

	w.Close()
	select {
	case <-loopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunEventLoop не завершился после Close()")
	}
}
