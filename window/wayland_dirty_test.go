//go:build linux && !android

package window

import (
	"image"
	"testing"
	"time"
)

// TestWlDirtyTrackerDoubleBuffer проверяет, что буфер, пропустивший кадр,
// дописывает область соседнего: при чередовании ничего не теряется.
func TestWlDirtyTrackerDoubleBuffer(t *testing.T) {
	var tr wlDirtyTracker
	r1 := image.Rect(0, 0, 10, 10)
	if got := tr.next(0, r1); got != r1 {
		t.Fatalf("первый кадр в buf0 = %v, ожидалось %v", got, r1)
	}
	// Второй кадр в buf1 обязан покрыть и область первого.
	r2 := image.Rect(50, 50, 60, 60)
	want := r1.Union(r2)
	if got := tr.next(1, r2); got != want {
		t.Fatalf("кадр в buf1 = %v, ожидалось %v", got, want)
	}
	// buf0 устарел ровно на то, что ушло в buf1.
	r3 := image.Rect(0, 0, 1, 1)
	want = want.Union(r3)
	if got := tr.next(0, r3); got != want {
		t.Fatalf("кадр в buf0 = %v, ожидалось %v", got, want)
	}
}

// TestWlDirtyTrackerSkipMerges проверяет объединение пропущенных кадров:
// область скопленного попадает в следующий записанный буфер.
func TestWlDirtyTrackerSkipMerges(t *testing.T) {
	var tr wlDirtyTracker
	tr.next(0, image.Rect(0, 0, 4, 4)) // buf0 актуален, buf1 устарел на 0,0-4,4

	tr.skip(image.Rect(20, 20, 30, 30))
	tr.skip(image.Rect(100, 0, 110, 5))

	want := image.Rect(0, 0, 4, 4).
		Union(image.Rect(20, 20, 30, 30)).
		Union(image.Rect(100, 0, 110, 5))
	got := tr.next(1, image.Rectangle{})
	if got != want {
		t.Fatalf("после пропусков = %v, ожидалось %v", got, want)
	}
	// Накопленное сброшено: следующий кадр несёт только своё и устаревшее buf0.
	if tr.pending != (image.Rectangle{}) {
		t.Fatalf("pending не сброшен: %v", tr.pending)
	}
}

// TestWlDirtyTrackerFullFrameClears проверяет, что полный кадр гасит долг:
// после него оба буфера не тянут старых областей сверх собственного.
func TestWlDirtyTrackerFullFrameClears(t *testing.T) {
	var tr wlDirtyTracker
	full := image.Rect(0, 0, 100, 100)
	tr.next(0, full)
	tr.next(1, full)
	if got := tr.next(0, image.Rect(1, 1, 2, 2)); got != full {
		t.Fatalf("ожидалось покрытие устаревшего %v, получено %v", full, got)
	}
	if got := tr.next(1, image.Rect(1, 1, 2, 2)); got != full {
		t.Fatalf("ожидалось покрытие устаревшего %v, получено %v", full, got)
	}
}

// TestWaitBufFreeIdle проверяет, что свободный буфер не заставляет ждать.
func TestWaitBufFreeIdle(t *testing.T) {
	w := &WaylandWindow{bufRelease: make(chan struct{}, 1)}
	start := time.Now()
	if !w.waitBufFree() {
		t.Fatal("свободный буфер признан занятым")
	}
	if d := time.Since(start); d > wlBufWaitTimeout {
		t.Fatalf("ожидание свободного буфера заняло %v", d)
	}
}

// TestWaitBufFreeTimeout проверяет пропуск кадра: буфер занят и release
// не приходит — блит обязан сдаться по таймауту.
func TestWaitBufFreeTimeout(t *testing.T) {
	w := &WaylandWindow{bufRelease: make(chan struct{}, 1)}
	w.bufBusy[0] = true
	start := time.Now()
	if w.waitBufFree() {
		t.Fatal("занятый буфер признан свободным")
	}
	if d := time.Since(start); d < wlBufWaitTimeout {
		t.Fatalf("сдался слишком рано: %v", d)
	}
}

// TestWaitBufFreeWakesOnRelease проверяет, что release будит ожидающий блит
// раньше таймаута.
func TestWaitBufFreeWakesOnRelease(t *testing.T) {
	w := &WaylandWindow{bufRelease: make(chan struct{}, 1)}
	w.bufBusy[0] = true
	go func() {
		time.Sleep(2 * time.Millisecond)
		w.mu.Lock()
		w.bufBusy[0] = false
		w.mu.Unlock()
		w.bufRelease <- struct{}{}
	}()
	start := time.Now()
	if !w.waitBufFree() {
		t.Fatal("release не разбудил ожидание")
	}
	if d := time.Since(start); d >= wlBufWaitTimeout {
		t.Fatalf("проснулся только по таймауту: %v", d)
	}
}

// TestPropertyTooBig проверяет отсечку неправдоподобной длины свойства XDND.
func TestPropertyTooBig(t *testing.T) {
	cases := []struct {
		words int
		want  bool
	}{
		{0, false},
		{1, false},
		{maxDnDBytes / 4, false},
		{maxDnDBytes/4 + 1, true},
		{1 << 28, true}, // 0x1FFFFFFF-класс: сотни МБ
		{-1, true},
	}
	for _, c := range cases {
		if got := propertyTooBig(c.words); got != c.want {
			t.Errorf("propertyTooBig(%d) = %v, ожидалось %v", c.words, got, c.want)
		}
	}
}
