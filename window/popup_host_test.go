package window

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// GG-80: хост попапов держал свой замок на всём проходе и звал под ним блит.
// С выкройкой окна по закрашенной части блит зовёт SetWindowRgn, а тот ждёт
// поток окна; поток окна в это время ждал замок хоста в обработчике движения
// мыши над попапом — окно «не отвечает» навсегда.

// popupFakeNative — окно ОС попапа: запоминает колбэки ввода, вызовы потока
// окна и умеет выкройку (как Win32).
type popupFakeNative struct {
	NativeWindow
	onMove   func(x, y int)
	onButton func(x, y, b int, p bool)
	invoked  []func()
	blitHook func()
	blits    int
}

func (f *popupFakeNative) SetOnMouseMove(fn func(x, y int))              { f.onMove = fn }
func (f *popupFakeNative) SetOnMouseButton(fn func(x, y, b int, p bool)) { f.onButton = fn }
func (f *popupFakeNative) InvokeOnUIThread(fn func())                    { f.invoked = append(f.invoked, fn) }
func (f *popupFakeNative) BlitRGBA(*image.RGBA)                          {}
func (f *popupFakeNative) GetPosition() (int, int)                       { return 0, 0 }
func (f *popupFakeNative) SetSize(int, int)                              {}
func (f *popupFakeNative) SetPosition(int, int)                          {}
func (f *popupFakeNative) Close()                                        {}

// blitPopupRegion — выкройка окна: на Windows этот вызов синхронно ждёт, пока
// поток окна разберёт сообщения.
func (f *popupFakeNative) blitPopupRegion(img *image.RGBA, bands []image.Rectangle) {
	f.blits++
	if f.blitHook != nil {
		f.blitHook()
	}
}

func popupHostFixture(t *testing.T) (*popupHost, *popupFakeNative, *hostedPopup, *queueEngine, *image.RGBA) {
	t.Helper()
	eng := &queueEngine{}
	carrier := &popupFakeNative{}
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	pop := &popupFakeNative{}
	img := image.NewRGBA(image.Rect(0, 0, 100, 80))
	hp := &hostedPopup{native: pop, rect: image.Rect(40, 30, 140, 110), img: img, w: 100, h: 80}
	hp.setOrigin(hp.rect, 1)
	h.windows[7] = hp
	h.setupPopupInput(pop, hp)
	return h, pop, hp, eng, img
}

// Пока идёт блит с выкройкой, ввод попапа проходит: замок хоста в это время не
// удерживается.
func TestPopupHost_BlitDoesNotBlockInput(t *testing.T) {
	h, pop, hp, eng, img := popupHostFixture(t)

	pop.blitHook = func() {
		// «Поток окна»: обработчик движения мыши над попапом.
		done := make(chan struct{})
		go func() {
			pop.onMove(5, 6)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("ввод попапа не прошёл во время блита — взаимоблокировка (GG-80)")
		}
	}

	h.apply([]engine.PopupFrame{{ID: 7, Rect: hp.rect, Img: img}})

	if pop.blits != 1 {
		t.Fatalf("блитов %d, ждал 1", pop.blits)
	}
	eng.run()
	// Начало координат оверлея (40,30) плюс локальные (5,6).
	wantLog(t, eng, "move 45,36")
}

// Ввод попапа не берёт замок хоста: пока замок держит горутина движка, поток
// окна обязан разбирать сообщения — иначе нативный вызов из-под замка будет
// ждать поток, который ждёт замок.
func TestPopupHost_InputDoesNotTakeHostLock(t *testing.T) {
	h, pop, _, eng, _ := popupHostFixture(t)

	h.mu.Lock()
	done := make(chan struct{})
	go func() {
		pop.onMove(5, 6)
		pop.onButton(5, 6, 0, true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		h.mu.Unlock()
		t.Fatal("ввод попапа ждёт замок хоста (GG-80)")
	}
	h.mu.Unlock()

	eng.run()
	wantLog(t, eng,
		"move 45,36",
		fmt.Sprintf("button 45,36 %d true", widget.MouseLeft),
	)
}

// Создание, перенос и закрытие окон тоже уходят на поток окна уже без замка:
// вызов, попавший туда, может звать хост обратно и не повиснуть.
func TestPopupHost_NativeCallsRunWithoutLock(t *testing.T) {
	h, pop, hp, _, img := popupHostFixture(t)
	carrier := h.carrier.(*popupFakeNative)

	// Оверлей переехал — перенос окна маршалится на поток окна.
	moved := hp.rect.Add(image.Pt(10, 0))
	h.apply([]engine.PopupFrame{{ID: 7, Rect: moved, Img: img}})
	if len(carrier.invoked) != 1 {
		t.Fatalf("на поток окна передано %d вызовов, ждал 1 (перенос)", len(carrier.invoked))
	}
	if ox, _ := hp.originXY(); ox != moved.Min.X {
		t.Fatalf("начало координат после переезда %d, ждал %d", ox, moved.Min.X)
	}
	// Выполняем отложенный вызов: он берёт замок хоста — значит, хост его уже
	// отпустил.
	done := make(chan struct{})
	go func() {
		carrier.invoked[0]()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("перенос окна ждёт замок хоста — он не был снят до маршалинга")
	}

	// Оверлея не стало — окно закрывается, тоже на потоке окна.
	carrier.invoked = nil
	h.apply(nil)
	if len(carrier.invoked) != 1 {
		t.Fatalf("на поток окна передано %d вызовов, ждал 1 (закрытие)", len(carrier.invoked))
	}
	if len(h.windows) != 0 {
		t.Fatalf("окно осталось в хосте: %d", len(h.windows))
	}
	_ = pop
}

// Ввод попапа доходит до движка в координатах носителя и в одной очереди с
// событиями окна (GG-68 — проверяем, что правка GG-80 этого не сломала).
func TestPopupHost_InputKeepsCarrierCoordinates(t *testing.T) {
	_, pop, _, eng, _ := popupHostFixture(t)

	pop.onMove(1, 2)
	pop.onButton(1, 2, 0, true)
	if n := eng.run(); n != 2 {
		t.Fatalf("в очереди движка %d событий, ждал 2", n)
	}
	wantLog(t, eng,
		"move 41,32",
		fmt.Sprintf("button 41,32 %d true", widget.MouseLeft),
	)
}
