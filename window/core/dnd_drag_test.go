package core

import (
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// v3.34: события приёмника перетаскивания и окно-источник. Общая часть —
// на любой платформе, с поддельным окном ОС.

// immediateEngine выполняет Post на месте: ответ over приходит до тайм-аута.
type immediateEngine struct{ queueEngine }

func (e *immediateEngine) Post(fn func()) { fn() }

// asyncEngine выполняет Post в отдельной горутине — как настоящий движок,
// чьи колбэки идут не на горутине насоса событий.
type asyncEngine struct{ queueEngine }

func (e *asyncEngine) Post(fn func()) { go fn() }

// dragNative — окно ОС, умеющее приёмник и источник перетаскивания.
type dragNative struct {
	NativeWindow
	enter func(x, y int)
	over  func(x, y int) bool
	leave func()

	mu       sync.Mutex
	began    [][]string
	effects  []DragEffect
	done     func(DragEffect)
	refuse   bool // BeginDragFiles вернёт false
	finished int
}

func (f *dragNative) SetOnDragEnter(fn func(x, y int))     { f.enter = fn }
func (f *dragNative) SetOnDragOver(fn func(x, y int) bool) { f.over = fn }
func (f *dragNative) SetOnDragLeave(fn func())             { f.leave = fn }
func (f *dragNative) BeginDragFiles(paths []string, effect DragEffect, done func(DragEffect)) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse {
		return false
	}
	f.began = append(f.began, paths)
	f.effects = append(f.effects, effect)
	f.done = done
	return true
}

func dragWindow(scale float64) (*Window, *dragNative, *immediateEngine) {
	eng := &immediateEngine{}
	fn := &dragNative{}
	win := &Window{surface: surface{eng: eng, native: fn, scale: scale}}
	win.setupFilesDrop()
	return win, fn, eng
}

func TestDragEffect_String(t *testing.T) {
	cases := map[DragEffect]string{
		0:                                   "none",
		DragEffectCopy:                      "copy",
		DragEffectCopy | DragEffectMove:     "copy|move",
		DragEffectMove | DragEffectLink:     "move|link",
		DragEffectCopy | DragEffectMove | 4: "copy|move|link",
		8:                                   "none", // неизвестные биты не считаются
	}
	for e, want := range cases {
		if got := e.String(); got != want {
			t.Errorf("DragEffect(%d).String() = %q, ждал %q", uint32(e), got, want)
		}
	}
}

func TestPickDragEffect(t *testing.T) {
	const c, m, l = DragEffectCopy, DragEffectMove, DragEffectLink
	cases := []struct {
		allowed, prefer, want DragEffect
	}{
		{c | m | l, 0, c},     // без пожеланий — копирование
		{m | l, 0, m},         // копирования нет — перемещение
		{l, 0, l},             // только ссылка
		{c | m, m, m},         // пожелание допустимо — берём его
		{c, m, c},             // пожелание недопустимо — то, что можно
		{c | m | l, l | m, m}, // из нескольких пожеланий — по старшинству
		{0, c, 0},             // нечего выбирать
	}
	for _, tc := range cases {
		if got := pickDragEffect(tc.allowed, tc.prefer); got != tc.want {
			t.Errorf("pickDragEffect(%v, %v) = %v, ждал %v", tc.allowed, tc.prefer, got, tc.want)
		}
	}
}

func TestDragEffectForKeys(t *testing.T) {
	const all = DragEffectCopy | DragEffectMove | DragEffectLink
	cases := []struct {
		name             string
		allowed          DragEffect
		ctrl, shift, alt bool
		want             DragEffect
	}{
		{"без клавиш", all, false, false, false, DragEffectCopy},
		{"Ctrl", all, true, false, false, DragEffectCopy},
		{"Shift", all, false, true, false, DragEffectMove},
		{"Ctrl+Shift", all, true, true, false, DragEffectLink},
		{"Alt", all, false, false, true, DragEffectLink},
		{"Shift, но перемещать нельзя", DragEffectCopy, false, true, false, DragEffectCopy},
		{"только перемещение", DragEffectMove, false, false, false, DragEffectMove},
		{"ничего не разрешено", 0, true, false, false, 0},
	}
	for _, tc := range cases {
		if got := dragEffectForKeys(tc.allowed, tc.ctrl, tc.shift, tc.alt); got != tc.want {
			t.Errorf("%s: %v, ждал %v", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeDragPaths(t *testing.T) {
	abs, _ := filepath.Abs("a.txt")
	got := normalizeDragPaths([]string{"", "a.txt", abs})
	if !reflect.DeepEqual(got, []string{abs, abs}) {
		t.Errorf("normalizeDragPaths = %q, ждал два абсолютных пути без пустого", got)
	}
}

// Координаты приёмника уходят приложению логическими: физические делятся на
// масштаб и сдвигаются на поля letterbox.
func TestDragEvents_LogicalCoordinates(t *testing.T) {
	win, fn, _ := dragWindow(2)
	win.fitOX, win.fitOY = 20, 10

	var log []string
	win.SetOnDragEnter(func(x, y int) { log = append(log, "enter", itoa(x), itoa(y)) })
	win.SetOnDragOver(func(x, y int) bool { log = append(log, "over", itoa(x), itoa(y)); return true })
	win.SetOnDragLeave(func() { log = append(log, "leave") })

	fn.enter(220, 110)
	if !fn.over(220, 110) {
		t.Error("over вернул false, хотя приложение принимает")
	}
	fn.leave()
	want := []string{"enter", "100", "50", "over", "100", "50", "leave"}
	if !reflect.DeepEqual(log, want) {
		t.Errorf("события: %v, ждал %v", log, want)
	}
}

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

// Без колбэка over сброс принимается везде — как до появления событий.
func TestDragEvents_NoCallbacksAccepts(t *testing.T) {
	_, fn, _ := dragWindow(1)
	fn.enter(1, 1) // не должно падать
	if !fn.over(1, 1) {
		t.Error("без колбэка сброс должен приниматься")
	}
	fn.leave()
}

// Ответ приложения «нет» доходит до бэкенда.
func TestDragEvents_OverRejects(t *testing.T) {
	win, fn, _ := dragWindow(1)
	win.SetOnDragOver(func(x, y int) bool { return x < 50 })
	if !fn.over(10, 0) {
		t.Error("в зоне сброса ответ должен быть «да»")
	}
	if fn.over(80, 0) {
		t.Error("вне зоны ответ должен быть «нет»")
	}
}

// Медленный колбэк не подвешивает источник: бэкенд ждёт ответ недолго и
// отвечает последним известным.
func TestDragEvents_SlowOverUsesLastAnswer(t *testing.T) {
	win, fn, _ := dragWindow(1)
	win.eng = &asyncEngine{}
	release := make(chan struct{})
	calls := 0
	win.SetOnDragOver(func(x, y int) bool {
		calls++
		if calls == 1 {
			return false
		}
		<-release
		return true
	})
	fn.enter(0, 0)
	if fn.over(0, 0) {
		t.Fatal("первый ответ «нет» потерян")
	}
	start := time.Now()
	got := fn.over(0, 0) // колбэк завис: ждём не дольше тайм-аута
	elapsed := time.Since(start)
	close(release)
	if got {
		t.Error("при тайм-ауте ждали последний ответ (нет)")
	}
	if elapsed > dragAnswerTimeout*8 {
		t.Errorf("ожидание ответа заняло %v", elapsed)
	}
}

func TestBeginDragFiles_PassesToBackend(t *testing.T) {
	win, fn, _ := dragWindow(1)
	var got []DragEffect
	win.SetOnDragFinished(func(e DragEffect) { got = append(got, e) })

	if !win.BeginDragFiles([]string{"x.png"}, DragEffectCopy|DragEffectMove) {
		t.Fatal("BeginDragFiles отказал")
	}
	abs, _ := filepath.Abs("x.png")
	if !reflect.DeepEqual(fn.began, [][]string{{abs}}) {
		t.Errorf("бэкенд получил %q, ждал абсолютный путь", fn.began)
	}
	if fn.effects[0] != DragEffectCopy|DragEffectMove {
		t.Errorf("маска действий: %v", fn.effects[0])
	}
	fn.done(DragEffectMove | 8) // посторонние биты отсекаются
	fn.done(0)
	if !reflect.DeepEqual(got, []DragEffect{DragEffectMove, 0}) {
		t.Errorf("SetOnDragFinished получил %v", got)
	}
}

func TestBeginDragFiles_Refusals(t *testing.T) {
	win, fn, _ := dragWindow(1)
	if win.BeginDragFiles(nil, DragEffectCopy) {
		t.Error("пустой список принят")
	}
	if win.BeginDragFiles([]string{""}, DragEffectCopy) {
		t.Error("список из пустых путей принят")
	}
	if win.BeginDragFiles([]string{"a"}, 0) {
		t.Error("пустая маска принята")
	}
	if win.BeginDragFiles([]string{"a"}, 8) {
		t.Error("маска из неизвестных битов принята")
	}
	fn.refuse = true
	if win.BeginDragFiles([]string{"a"}, DragEffectCopy) {
		t.Error("отказ бэкенда не дошёл до вызывающего")
	}
	// Бэкенд без поддержки.
	bare := &Window{surface: surface{eng: &immediateEngine{}, native: &inputNative{}, scale: 1}}
	if bare.BeginDragFiles([]string{"a"}, DragEffectCopy) {
		t.Error("бэкенд без перетаскивания вернул true")
	}
	bare.setupFilesDrop() // не падает на бэкенде без интерфейсов
}
