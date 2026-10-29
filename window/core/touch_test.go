package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// touchRecorder — движок-заглушка: пишет мышь и касания в общий журнал, чтобы
// порядок событий был виден целиком. Касание считается «взятым», если хозяин
// теста его заказал (claim).
type touchRecorder struct {
	root    widget.Widget
	log     []string
	claim   bool
	touches []widget.TouchEvent
	cancels int
}

func (r *touchRecorder) Frames() <-chan output.Frame { return nil }
func (r *touchRecorder) CanvasSize() (int, int)      { return 100, 100 }
func (r *touchRecorder) Root() widget.Widget         { return r.root }
func (r *touchRecorder) SendMouseMove(x, y int) {
	r.log = append(r.log, fmt.Sprintf("move %d,%d", x, y))
}
func (r *touchRecorder) SendMouseButton(x, y int, b widget.MouseButton, p bool) {
	r.log = append(r.log, fmt.Sprintf("button %v %d,%d down=%v", b, x, y, p))
}
func (r *touchRecorder) SendKeyEvent(widget.KeyEvent)    {}
func (r *touchRecorder) CursorAt(int, int) widget.Cursor { return 0 }
func (r *touchRecorder) SendTouch(ev widget.TouchEvent) bool {
	r.touches = append(r.touches, ev)
	r.log = append(r.log, fmt.Sprintf("touch %d %v %d,%d", ev.ID, ev.Phase, ev.X, ev.Y))
	return r.claim
}
func (r *touchRecorder) CancelTouches() { r.cancels++ }

func ev(id, x, y int, ph widget.TouchPhase) widget.TouchEvent {
	return widget.TouchEvent{ID: id, X: x, Y: y, Phase: ph}
}

func newTouchSurface(claim bool) (*surface, *touchRecorder) {
	r := &touchRecorder{claim: claim}
	return &surface{eng: r}, r
}

// Касание, которого не взял ни один виджет, становится левой кнопкой мыши: без
// этого под пальцем не работали бы обычные кнопки и меню.
func TestTouch_UnclaimedBecomesMouse(t *testing.T) {
	s, r := newTouchSurface(false)
	s.deliverTouch(ev(1, 10, 20, widget.TouchBegan))
	s.deliverTouch(ev(1, 15, 25, widget.TouchMoved))
	s.deliverTouch(ev(1, 16, 26, widget.TouchEnded))

	var mouse []string
	for _, l := range r.log {
		if l[:5] != "touch" {
			mouse = append(mouse, l)
		}
	}
	want := []string{
		"move 10,20", "button 0 10,20 down=true",
		"move 15,25",
		"move 16,26", "button 0 16,26 down=false",
	}
	if fmt.Sprint(mouse) != fmt.Sprint(want) {
		t.Errorf("мышь из касания:\n got  %v\n want %v", mouse, want)
	}
	if s.emu.active {
		t.Error("после отрыва эмуляция осталась взведённой")
	}
}

// Касание, взятое виджетом, мышью не становится.
func TestTouch_ClaimedIsNotMouse(t *testing.T) {
	s, r := newTouchSurface(true)
	s.deliverTouch(ev(1, 10, 20, widget.TouchBegan))
	s.deliverTouch(ev(1, 15, 25, widget.TouchMoved))
	s.deliverTouch(ev(1, 16, 26, widget.TouchEnded))
	for _, l := range r.log {
		if l[:5] != "touch" {
			t.Errorf("взятое касание породило мышь: %q", l)
		}
	}
	if len(r.touches) != 3 {
		t.Errorf("движок получил %d событий", len(r.touches))
	}
}

// Мышь одна: пока ведётся первый «ничей» палец, остальные не мешают ей.
func TestTouch_OnlyFirstUnclaimedFingerIsMouse(t *testing.T) {
	s, r := newTouchSurface(false)
	s.deliverTouch(ev(1, 10, 10, widget.TouchBegan))
	s.deliverTouch(ev(2, 90, 90, widget.TouchBegan))
	s.deliverTouch(ev(2, 91, 91, widget.TouchMoved))
	s.deliverTouch(ev(2, 91, 91, widget.TouchEnded))
	for _, l := range r.log {
		if l == "move 90,90" || l == "move 91,91" {
			t.Errorf("второй палец управлял мышью: %q", l)
		}
	}
	if !s.emu.active || s.emu.id != 1 {
		t.Errorf("первый палец потерян: %+v", s.emu)
	}
	s.deliverTouch(ev(1, 11, 11, widget.TouchEnded))
	if s.emu.active {
		t.Error("отрыв первого пальца не освободил мышь")
	}
}

// Отмена касания отпускает кнопку вне окна: клик не засчитывается.
func TestTouch_CancelReleasesOutside(t *testing.T) {
	s, r := newTouchSurface(false)
	s.deliverTouch(ev(1, 10, 10, widget.TouchBegan))
	r.log = nil
	s.deliverTouch(ev(1, 10, 10, widget.TouchCancelled))
	got := fmt.Sprint(r.log[1:]) // [0] — само касание
	want := fmt.Sprint([]string{"move -1,-1", "button 0 -1,-1 down=false"})
	if got != want {
		t.Errorf("отмена эмулируемой мыши:\n got  %v\n want %v", got, want)
	}
	if s.emu.active {
		t.Error("эмуляция не снята")
	}
}

// cancelTouches (потеря фокуса) отменяет и касания виджетов, и мышь.
func TestTouch_CancelTouches(t *testing.T) {
	s, r := newTouchSurface(false)
	s.deliverTouch(ev(1, 5, 5, widget.TouchBegan))
	s.cancelTouches()
	if r.cancels != 1 {
		t.Errorf("движок не извещён: %d", r.cancels)
	}
	if s.emu.active {
		t.Error("мышь осталась зажатой")
	}
}

// Letterbox: касание переводится в координаты контента, как мышь.
func TestTouch_FitOffset(t *testing.T) {
	s, r := newTouchSurface(true)
	s.fitOX, s.fitOY = 7, 3
	s.deliverTouch(ev(1, 17, 13, widget.TouchBegan))
	if got := r.touches[0]; got.X != 10 || got.Y != 10 {
		t.Errorf("координаты %d,%d, ждал 10,10", got.X, got.Y)
	}
}

// ─── Решение окна: включать ли касания ──────────────────────────────────────

// touchNative — бэкенд с поддержкой касаний; запоминает, что ему велено.
type touchNative struct {
	fakeCloseNative
	enabled []bool
	onTouch func(widget.TouchEvent)
}

func (n *touchNative) SetTouchEnabled(on bool)               { n.enabled = append(n.enabled, on) }
func (n *touchNative) SetOnTouch(fn func(widget.TouchEvent)) { n.onTouch = fn }

type touchRoot struct{ widget.Panel }

func (*touchRoot) OnTouch(widget.TouchEvent) bool { return true }

func TestTouchEnabled_Decision(t *testing.T) {
	plain := New(&postEngine{root: widget.NewWindow("a", 100, 100)}, "a")
	if plain.TouchEnabled() {
		t.Error("окно без касаний включено само")
	}
	plain.SetTouchEnabled(true)
	if !plain.TouchEnabled() {
		t.Error("явное включение не действует")
	}

	auto := New(&postEngine{root: &touchRoot{}}, "b")
	if !auto.TouchEnabled() {
		t.Error("корень-TouchHandler не включил касания")
	}
	auto.SetTouchEnabled(false)
	if auto.TouchEnabled() {
		t.Error("явный отказ важнее корня")
	}
}

// Бэкенд вообще не трогают, если касания не просили: так приложения, не
// знающие о касаниях, остаются в точности такими, какими были.
func TestTouch_BackendUntouchedByDefault(t *testing.T) {
	eng := &touchRecorder{root: widget.NewWindow("a", 100, 100)}
	win := New(eng, "a")
	n := &touchNative{}
	win.native = n

	win.prepareTouch()
	win.setupTouch()
	if len(n.enabled) != 0 || n.onTouch != nil {
		t.Errorf("бэкенд задет без просьбы: enabled=%v onTouch=%v", n.enabled, n.onTouch != nil)
	}

	win.SetTouchEnabled(true)
	win.prepareTouch()
	win.setupTouch()
	if len(n.enabled) == 0 || !n.enabled[len(n.enabled)-1] {
		t.Errorf("бэкенду не велено включить касания: %v", n.enabled)
	}
	if n.onTouch == nil {
		t.Fatal("приёмник касаний не подписан")
	}

	// И событие бэкенда доходит до движка.
	eng.claim = true
	n.onTouch(ev(3, 4, 5, widget.TouchBegan))
	if len(eng.touches) != 1 || eng.touches[0].ID != 3 {
		t.Errorf("движок получил %+v", eng.touches)
	}
}

// Движок без SendTouch касаний не получает и бэкенд не подписывается.
func TestTouch_EngineWithoutSupport(t *testing.T) {
	eng := &postEngine{root: widget.NewWindow("a", 100, 100)}
	win := New(eng, "a")
	n := &touchNative{}
	win.native = n
	win.SetTouchEnabled(true)
	win.setupTouch()
	if n.onTouch != nil {
		t.Error("подписка на касания при движке, который их не принимает")
	}
}

// ─── Долгое нажатие «ничьего» пальца: правая кнопка ─────────────────────────

// mouseLog — только мышиные строки журнала (без самих касаний).
func mouseLog(r *touchRecorder) []string {
	var out []string
	for _, l := range r.log {
		if len(l) < 5 || l[:5] != "touch" {
			out = append(out, l)
		}
	}
	return out
}

// clockedSurface — поверхность с ручными часами прихода событий.
func clockedSurface(claim bool) (*surface, *touchRecorder, *time.Time) {
	s, r := newTouchSurface(claim)
	now := time.Unix(1000, 0)
	s.emuNow = func() time.Time { return now }
	return s, r, &now
}

// Палец, пролежавший на месте дольше срока, при отрыве даёт правый щелчок: левая
// кнопка отпускается вне окна (не кликая), затем в точке отрыва — правая.
func TestTouchEmu_LongPressIsRightClick(t *testing.T) {
	s, r, now := clockedSurface(false)
	s.deliverTouch(ev(1, 10, 20, widget.TouchBegan))
	*now = now.Add(600 * time.Millisecond)
	s.deliverTouch(ev(1, 12, 21, widget.TouchEnded)) // дрожь в пределах порога

	want := []string{
		"move 10,20", "button 0 10,20 down=true",
		"move -1,-1", "button 0 -1,-1 down=false",
		"move 12,21", "button 1 12,21 down=true", "button 1 12,21 down=false",
	}
	if got := mouseLog(r); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("долгое нажатие:\n got  %v\n want %v", got, want)
	}
	if s.emu.active {
		t.Error("эмуляция осталась взведённой")
	}
}

// Короткое касание остаётся обычным левым щелчком.
func TestTouchEmu_ShortPressStaysLeftClick(t *testing.T) {
	s, r, now := clockedSurface(false)
	s.deliverTouch(ev(1, 10, 20, widget.TouchBegan))
	*now = now.Add(200 * time.Millisecond)
	s.deliverTouch(ev(1, 10, 20, widget.TouchEnded))
	want := []string{"move 10,20", "button 0 10,20 down=true", "move 10,20", "button 0 10,20 down=false"}
	if got := mouseLog(r); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("короткое касание:\n got  %v\n want %v", got, want)
	}
}

// Палец, уходивший дальше порога тапа, долгим нажатием уже не станет, даже
// вернувшись на место: так «целился и потянул» остаётся левой кнопкой.
func TestTouchEmu_MovedFingerIsNotLongPress(t *testing.T) {
	s, r, now := clockedSurface(false)
	s.deliverTouch(ev(1, 10, 20, widget.TouchBegan))
	s.deliverTouch(ev(1, 60, 20, widget.TouchMoved))
	s.deliverTouch(ev(1, 10, 20, widget.TouchMoved))
	*now = now.Add(2 * time.Second)
	s.deliverTouch(ev(1, 10, 20, widget.TouchEnded))
	for _, l := range mouseLog(r) {
		if l == "button 1 10,20 down=true" {
			t.Fatalf("сдвинутый палец дал правую кнопку: %v", mouseLog(r))
		}
	}
	last := mouseLog(r)
	if last[len(last)-1] != "button 0 10,20 down=false" {
		t.Errorf("отрыв не левой кнопкой: %v", last)
	}

	// Отрыв далеко от места касания — тоже не «лежал».
	s2, r2, now2 := clockedSurface(false)
	s2.deliverTouch(ev(1, 10, 20, widget.TouchBegan))
	*now2 = now2.Add(2 * time.Second)
	s2.deliverTouch(ev(1, 80, 20, widget.TouchEnded))
	if got := mouseLog(r2); got[len(got)-1] != "button 0 80,20 down=false" {
		t.Errorf("отрыв вдали: %v", got)
	}
}

// Порог тапа — в логических пикселях: при масштабе 2 дрожь в 15 физических
// пикселей ещё «на месте».
func TestTouchEmu_SlopFollowsScale(t *testing.T) {
	s, r, now := clockedSurface(false)
	s.scale = 2
	s.deliverTouch(ev(1, 100, 100, widget.TouchBegan))
	s.deliverTouch(ev(1, 115, 100, widget.TouchMoved))
	*now = now.Add(time.Second)
	s.deliverTouch(ev(1, 115, 100, widget.TouchEnded))
	got := mouseLog(r)
	if got[len(got)-1] != "button 1 115,100 down=false" {
		t.Errorf("при масштабе 2: %v", got)
	}
}

// Срок берётся по часам события, когда бэкенд их знает: опоздавшая доставка не
// превращает быстрый тап в долгое нажатие.
func TestTouchEmu_UsesEventClock(t *testing.T) {
	s, r, now := clockedSurface(false)
	b := ev(1, 10, 20, widget.TouchBegan)
	b.Time = 5 * time.Second
	s.deliverTouch(b)
	*now = now.Add(3 * time.Second) // очередь запоздала
	e := ev(1, 10, 20, widget.TouchEnded)
	e.Time = 5*time.Second + 100*time.Millisecond
	s.deliverTouch(e)
	got := mouseLog(r)
	if got[len(got)-1] != "button 0 10,20 down=false" {
		t.Errorf("быстрый тап стал долгим: %v", got)
	}

	// И наоборот: по часам события лежал долго, хотя доставили сразу.
	s2, r2, _ := clockedSurface(false)
	b2 := ev(1, 10, 20, widget.TouchBegan)
	b2.Time = time.Second
	s2.deliverTouch(b2)
	e2 := ev(1, 10, 20, widget.TouchEnded)
	e2.Time = 2 * time.Second
	s2.deliverTouch(e2)
	got = mouseLog(r2)
	if got[len(got)-1] != "button 1 10,20 down=false" {
		t.Errorf("долгое по часам события: %v", got)
	}
}

func TestTouchEmu_LongPressDurationSetting(t *testing.T) {
	// Свой срок.
	s, r, now := clockedSurface(false)
	s.emuLongPress.Store(int64(2 * time.Second))
	s.deliverTouch(ev(1, 1, 1, widget.TouchBegan))
	*now = now.Add(time.Second)
	s.deliverTouch(ev(1, 1, 1, widget.TouchEnded))
	if got := mouseLog(r); got[len(got)-1] != "button 0 1,1 down=false" {
		t.Errorf("срок 2с, секунда — не долго: %v", got)
	}
	// Выключено.
	s2, r2, now2 := clockedSurface(false)
	s2.emuLongPress.Store(int64(-1))
	s2.deliverTouch(ev(1, 1, 1, widget.TouchBegan))
	*now2 = now2.Add(10 * time.Second)
	s2.deliverTouch(ev(1, 1, 1, widget.TouchEnded))
	if got := mouseLog(r2); got[len(got)-1] != "button 0 1,1 down=false" {
		t.Errorf("отключённое долгое нажатие сработало: %v", got)
	}
	// Публичный метод пишет в то же поле.
	win := New(&touchRecorder{root: widget.NewWindow("a", 100, 100)}, "a")
	win.SetTouchLongPress(750 * time.Millisecond)
	if d := win.emuLongPressDuration(); d != 750*time.Millisecond {
		t.Errorf("SetTouchLongPress: %v", d)
	}
	win.SetTouchLongPress(0)
	if d := win.emuLongPressDuration(); d != 500*time.Millisecond {
		t.Errorf("по умолчанию: %v", d)
	}
}

// Взятое виджетом касание правой кнопкой не становится ни при каком сроке, а
// отменённое долгое нажатие не даёт и левого щелчка.
func TestTouchEmu_ClaimedAndCancelled(t *testing.T) {
	s, r, now := clockedSurface(true)
	s.deliverTouch(ev(1, 10, 20, widget.TouchBegan))
	*now = now.Add(2 * time.Second)
	s.deliverTouch(ev(1, 10, 20, widget.TouchEnded))
	if got := mouseLog(r); len(got) != 0 {
		t.Errorf("взятое касание породило мышь: %v", got)
	}

	s2, r2, now2 := clockedSurface(false)
	s2.deliverTouch(ev(1, 10, 20, widget.TouchBegan))
	*now2 = now2.Add(2 * time.Second)
	s2.deliverTouch(ev(1, 10, 20, widget.TouchCancelled))
	for _, l := range mouseLog(r2) {
		if l == "button 1 10,20 down=true" {
			t.Errorf("отмена дала правый щелчок: %v", mouseLog(r2))
		}
	}
}
