package wayland

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// Тело события в формате wire: слова little-endian, координаты — wl_fixed 24.8.

func tw(vals ...uint32) []byte {
	b := make([]byte, 0, len(vals)*4)
	for _, v := range vals {
		b = binary.LittleEndian.AppendUint32(b, v)
	}
	return b
}

func fx(v float64) uint32 { return uint32(int32(v * 256)) }

func touchDownBody(serial, ms, surf uint32, id int32, x, y float64) []byte {
	return tw(serial, ms, surf, uint32(id), fx(x), fx(y))
}
func touchMotionBody(ms uint32, id int32, x, y float64) []byte {
	return tw(ms, uint32(id), fx(x), fx(y))
}
func touchUpBody(serial, ms uint32, id int32) []byte { return tw(serial, ms, uint32(id)) }

// sinkRig — приёмник касаний с записью всего, что он отдал окну и попапам.
type sinkRig struct {
	sink    wlTouchSink
	touches []base.TouchEvent
	popup   []string
	serials []uint32
}

const (
	mainSurf  = 11
	popupSurf = 31
	popupID   = uintptr(5)
)

func newSinkRig(scale float64) *sinkRig {
	r := &sinkRig{}
	r.sink = wlTouchSink{
		scale: func() float64 { return scale },
		popupFor: func(s uint32) (uintptr, bool) {
			if s == popupSurf {
				return popupID, true
			}
			return 0, false
		},
		onTouch: func(ev base.TouchEvent) { r.touches = append(r.touches, ev) },
		popup: func() native.ChildPopupHandlers {
			return native.ChildPopupHandlers{
				Move: func(id uintptr, x, y int) { r.popup = append(r.popup, fmt.Sprintf("move %d %d,%d", id, x, y)) },
				Button: func(id uintptr, x, y, b int, p bool) {
					r.popup = append(r.popup, fmt.Sprintf("button %d %d,%d b%d down=%v", id, x, y, b, p))
				},
			}
		},
		serial: func(s uint32) { r.serials = append(r.serials, s) },
	}
	return r
}

func TestWlTouch_ParseBodies(t *testing.T) {
	d, ok := parseWlTouchDown(touchDownBody(9, 1000, 11, 3, 10.5, 20.25))
	if !ok || d.Serial != 9 || d.Time != 1000 || d.Surface != 11 || d.ID != 3 || d.X != 10.5 || d.Y != 20.25 {
		t.Errorf("down: %+v ok=%v", d, ok)
	}
	if _, id, x, y, ok := parseWlTouchMotion(touchMotionBody(5, 2, -3, 7)); !ok || id != 2 || x != -3 || y != 7 {
		t.Errorf("motion: %d %v %v %v", id, x, y, ok)
	}
	if s, ms, id, ok := parseWlTouchUp(touchUpBody(4, 77, 6)); !ok || s != 4 || ms != 77 || id != 6 {
		t.Errorf("up: %d %d %d %v", s, ms, id, ok)
	}
	// Обрезанные тела отбрасываются, а не читают за границей.
	if _, ok := parseWlTouchDown(make([]byte, 20)); ok {
		t.Error("обрезанный down принят")
	}
	if _, _, _, _, ok := parseWlTouchMotion(make([]byte, 12)); ok {
		t.Error("обрезанный motion принят")
	}
	if _, _, _, ok := parseWlTouchUp(make([]byte, 8)); ok {
		t.Error("обрезанный up принят")
	}
}

func TestWlTouch_MainSurfaceSequence(t *testing.T) {
	r := newSinkRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(100, 1000, mainSurf, 0, 40, 50))
	r.sink.handle(wlTouchEvFrame, nil)
	r.sink.handle(wlTouchEvMotion, touchMotionBody(1016, 0, 45, 52))
	r.sink.handle(wlTouchEvMotion, touchMotionBody(1032, 0, 60, 70))
	r.sink.handle(wlTouchEvUp, touchUpBody(101, 1048, 0))

	if len(r.touches) != 4 {
		t.Fatalf("событий %d: %+v", len(r.touches), r.touches)
	}
	want := []base.TouchEvent{
		{ID: 0, X: 40, Y: 50, Phase: base.TouchBegan, Time: 1000 * time.Millisecond},
		{ID: 0, X: 45, Y: 52, Phase: base.TouchMoved, Time: 1016 * time.Millisecond},
		{ID: 0, X: 60, Y: 70, Phase: base.TouchMoved, Time: 1032 * time.Millisecond},
		// У up координат нет — берётся последняя точка.
		{ID: 0, X: 60, Y: 70, Phase: base.TouchEnded, Time: 1048 * time.Millisecond},
	}
	for i := range want {
		if r.touches[i] != want[i] {
			t.Errorf("событие %d: %+v, ждал %+v", i, r.touches[i], want[i])
		}
	}
	if len(r.popup) != 0 {
		t.Errorf("попап получил события окна: %v", r.popup)
	}
	if len(r.serials) != 2 || r.serials[0] != 100 || r.serials[1] != 101 {
		t.Errorf("serial'ы ввода %v", r.serials)
	}
	// Отпущенный палец забыт: запоздалый motion ничего не даёт.
	r.sink.handle(wlTouchEvMotion, touchMotionBody(1100, 0, 1, 1))
	if len(r.touches) != 4 {
		t.Error("motion после up обработан")
	}
}

// Поверхностные единицы -> пиксели буфера.
func TestWlTouch_ScaleToPixels(t *testing.T) {
	r := newSinkRig(2)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 0, mainSurf, 0, 40.5, 50))
	if r.touches[0].X != 81 || r.touches[0].Y != 100 {
		t.Errorf("при масштабе 2: %d,%d, ждал 81,100", r.touches[0].X, r.touches[0].Y)
	}
	r2 := newSinkRig(1.5)
	r2.sink.handle(wlTouchEvDown, touchDownBody(1, 0, mainSurf, 0, 10, 20))
	if r2.touches[0].X != 15 || r2.touches[0].Y != 30 {
		t.Errorf("при масштабе 1.5: %d,%d, ждал 15,30", r2.touches[0].X, r2.touches[0].Y)
	}
}

func TestWlTouch_TwoFingers(t *testing.T) {
	r := newSinkRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 0, mainSurf, 0, 10, 10))
	r.sink.handle(wlTouchEvDown, touchDownBody(2, 1, mainSurf, 1, 90, 10))
	r.sink.handle(wlTouchEvMotion, touchMotionBody(5, 1, 100, 10))
	r.sink.handle(wlTouchEvMotion, touchMotionBody(5, 0, 5, 10))
	r.sink.handle(wlTouchEvUp, touchUpBody(3, 9, 0))
	r.sink.handle(wlTouchEvUp, touchUpBody(4, 9, 1))
	var got []string
	for _, e := range r.touches {
		got = append(got, fmt.Sprintf("%d:%v@%d,%d", e.ID, e.Phase, e.X, e.Y))
	}
	want := []string{"0:Began@10,10", "1:Began@90,10", "1:Moved@100,10", "0:Moved@5,10", "0:Ended@5,10", "1:Ended@100,10"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("\n got  %v\n want %v", got, want)
	}
}

// wl_touch.cancel: компоновщик забрал все касания.
func TestWlTouch_CancelAll(t *testing.T) {
	r := newSinkRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 0, mainSurf, 0, 10, 11))
	r.sink.handle(wlTouchEvDown, touchDownBody(2, 0, mainSurf, 1, 20, 21))
	r.touches = nil
	r.sink.handle(wlTouchEvCancel, nil)
	if len(r.touches) != 2 {
		t.Fatalf("отмен %d: %+v", len(r.touches), r.touches)
	}
	for _, e := range r.touches {
		if e.Phase != base.TouchCancelled {
			t.Errorf("фаза %v", e.Phase)
		}
		if (e.ID == 0 && (e.X != 10 || e.Y != 11)) || (e.ID == 1 && (e.X != 20 || e.Y != 21)) {
			t.Errorf("отмена пальца %d в %d,%d", e.ID, e.X, e.Y)
		}
	}
	// После отмены запоздалые события забыты.
	r.touches = nil
	r.sink.handle(wlTouchEvUp, touchUpBody(3, 1, 0))
	if len(r.touches) != 0 {
		t.Error("up после cancel доставлен")
	}
}

// Палец на попапе — это мышь попапа, а не касание окна.
func TestWlTouch_PopupIsMouse(t *testing.T) {
	r := newSinkRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 0, popupSurf, 0, 12, 20))
	r.sink.handle(wlTouchEvMotion, touchMotionBody(1, 0, 14, 22))
	// Второй палец на попапе игнорируется: мышь одна.
	r.sink.handle(wlTouchEvDown, touchDownBody(2, 2, popupSurf, 1, 50, 50))
	r.sink.handle(wlTouchEvMotion, touchMotionBody(3, 1, 55, 55))
	r.sink.handle(wlTouchEvUp, touchUpBody(3, 4, 1))
	r.sink.handle(wlTouchEvUp, touchUpBody(4, 5, 0))

	want := []string{
		"move 5 12,20", "button 5 12,20 b0 down=true",
		"move 5 14,22",
		"move 5 14,22", "button 5 14,22 b0 down=false",
	}
	if fmt.Sprint(r.popup) != fmt.Sprint(want) {
		t.Errorf("мышь попапа:\n got  %v\n want %v", r.popup, want)
	}
	if len(r.touches) != 0 {
		t.Errorf("касание попапа ушло окну: %+v", r.touches)
	}
}

// Отмена касания попапа отпускает кнопку вне его: пункт меню не срабатывает.
func TestWlTouch_PopupCancelReleasesOutside(t *testing.T) {
	r := newSinkRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 0, popupSurf, 0, 12, 20))
	r.popup = nil
	r.sink.handle(wlTouchEvCancel, nil)
	want := []string{"move 5 -1,-1", "button 5 -1,-1 b0 down=false"}
	if fmt.Sprint(r.popup) != fmt.Sprint(want) {
		t.Errorf("\n got  %v\n want %v", r.popup, want)
	}
}

// Палец на окне при открытом попапе остаётся касанием окна.
func TestWlTouch_MixedWindowAndPopup(t *testing.T) {
	r := newSinkRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 0, popupSurf, 0, 5, 5))
	r.sink.handle(wlTouchEvDown, touchDownBody(2, 0, mainSurf, 1, 70, 80))
	r.sink.handle(wlTouchEvMotion, touchMotionBody(1, 1, 71, 81))
	if len(r.touches) != 2 || r.touches[0].ID != 1 || r.touches[1].Phase != base.TouchMoved {
		t.Errorf("касания окна: %+v", r.touches)
	}
}

// Мусор в теле события не роняет разбор.
func TestWlTouch_ShortBodiesIgnored(t *testing.T) {
	r := newSinkRig(1)
	for _, op := range []uint16{wlTouchEvDown, wlTouchEvUp, wlTouchEvMotion} {
		r.sink.handle(op, []byte{1, 2, 3})
		r.sink.handle(op, nil)
	}
	r.sink.handle(99, tw(1, 2, 3))
	if len(r.touches) != 0 || len(r.popup) != 0 {
		t.Errorf("мусор породил события: %v %v", r.touches, r.popup)
	}
}

// ─── Форма контакта (wl_touch версии 6) ─────────────────────────────────────

func shapeBody(id int32, major, minor float64) []byte {
	return tw(uint32(id), fx(major), fx(minor))
}
func orientBody(id int32, deg float64) []byte { return tw(uint32(id), fx(deg)) }

func TestWlTouch_ParseShapeBodies(t *testing.T) {
	if id, a, b, ok := parseWlTouchShape(shapeBody(3, 12.5, 6.25)); !ok || id != 3 || a != 12.5 || b != 6.25 {
		t.Errorf("shape: %d %v %v %v", id, a, b, ok)
	}
	if id, deg, ok := parseWlTouchOrientation(orientBody(4, -45.5)); !ok || id != 4 || deg != -45.5 {
		t.Errorf("orientation: %d %v %v", id, deg, ok)
	}
	if _, _, _, ok := parseWlTouchShape(make([]byte, 11)); ok {
		t.Error("обрезанное shape принято")
	}
	if _, _, ok := parseWlTouchOrientation(make([]byte, 7)); ok {
		t.Error("обрезанное orientation принято")
	}
}

func shapedRig(scale float64) *sinkRig {
	r := newSinkRig(scale)
	r.sink.shapes = func() bool { return true }
	return r
}

// События копятся до frame и уходят с формой контакта, переведённой в пиксели
// буфера; давления у Wayland нет.
func TestWlTouch_ShapeAttachedAtFrame(t *testing.T) {
	r := shapedRig(2)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 100, mainSurf, 1, 10, 20))
	if len(r.touches) != 0 {
		t.Fatalf("событие ушло до frame: %+v", r.touches)
	}
	r.sink.handle(wlTouchEvShape, shapeBody(1, 20, 10))
	r.sink.handle(wlTouchEvOrientation, orientBody(1, 30))
	r.sink.handle(wlTouchEvFrame, nil)
	if len(r.touches) != 1 {
		t.Fatalf("после frame событий %d", len(r.touches))
	}
	ev := r.touches[0]
	if ev.Phase != base.TouchBegan || ev.X != 20 || ev.Y != 40 {
		t.Errorf("основа: %+v", ev)
	}
	if ev.Major != 40 || ev.Minor != 20 || ev.Orientation != 30 {
		t.Errorf("форма: %v x %v под %v, ждал 40 x 20 под 30 (масштаб 2)", ev.Major, ev.Minor, ev.Orientation)
	}
	if ev.Pressure != 0 {
		t.Errorf("у wl_touch нет давления, а Pressure = %v", ev.Pressure)
	}

	// Форма не повторяется, пока не меняется: Moved наследует прежнюю.
	r.sink.handle(wlTouchEvMotion, touchMotionBody(116, 1, 11, 20))
	r.sink.handle(wlTouchEvFrame, nil)
	if got := r.touches[1]; got.Phase != base.TouchMoved || got.Major != 40 || got.Minor != 20 {
		t.Errorf("Moved без новой формы: %+v", got)
	}
	// А изменившаяся подхватывается.
	r.sink.handle(wlTouchEvMotion, touchMotionBody(132, 1, 12, 20))
	r.sink.handle(wlTouchEvShape, shapeBody(1, 30, 15))
	r.sink.handle(wlTouchEvFrame, nil)
	if got := r.touches[2]; got.Major != 60 || got.Minor != 30 || got.Orientation != 30 {
		t.Errorf("новая форма: %+v", got)
	}
	// Отрыв несёт последнюю форму и стирает её: следующий палец с тем же номером
	// начинает без неё.
	r.sink.handle(wlTouchEvUp, touchUpBody(2, 150, 1))
	r.sink.handle(wlTouchEvFrame, nil)
	if got := r.touches[3]; got.Phase != base.TouchEnded || got.Major != 60 {
		t.Errorf("отрыв: %+v", got)
	}
	r.sink.handle(wlTouchEvDown, touchDownBody(3, 200, mainSurf, 1, 5, 5))
	r.sink.handle(wlTouchEvFrame, nil)
	if got := r.touches[4]; got.Major != 0 || got.Minor != 0 || got.Orientation != 0 {
		t.Errorf("форма прежнего пальца протекла в новый: %+v", got)
	}
}

// Два пальца в одном кадре получают каждый свою форму; down и up одного пальца в
// одном кадре доходят оба и по порядку.
func TestWlTouch_ShapePerFingerAndOrder(t *testing.T) {
	r := shapedRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 10, mainSurf, 1, 5, 5))
	r.sink.handle(wlTouchEvDown, touchDownBody(2, 10, mainSurf, 2, 50, 50))
	r.sink.handle(wlTouchEvShape, shapeBody(2, 9, 7))
	r.sink.handle(wlTouchEvShape, shapeBody(1, 4, 3))
	r.sink.handle(wlTouchEvFrame, nil)
	if len(r.touches) != 2 || r.touches[0].ID != 1 || r.touches[1].ID != 2 {
		t.Fatalf("порядок: %+v", r.touches)
	}
	if r.touches[0].Major != 4 || r.touches[0].Minor != 3 || r.touches[1].Major != 9 || r.touches[1].Minor != 7 {
		t.Errorf("формы: %+v", r.touches)
	}

	r2 := shapedRig(1)
	r2.sink.handle(wlTouchEvDown, touchDownBody(1, 10, mainSurf, 1, 5, 5))
	r2.sink.handle(wlTouchEvUp, touchUpBody(2, 20, 1))
	r2.sink.handle(wlTouchEvFrame, nil)
	if len(r2.touches) != 2 || r2.touches[0].Phase != base.TouchBegan || r2.touches[1].Phase != base.TouchEnded {
		t.Errorf("down+up в одном кадре: %+v", r2.touches)
	}
}

// Без версии 6 события идут сразу, как и раньше, а форму не присылают и не ждут.
func TestWlTouch_NoShapeWithoutV6(t *testing.T) {
	r := newSinkRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 10, mainSurf, 1, 5, 5))
	if len(r.touches) != 1 {
		t.Fatalf("без версии 6 событие задержано: %+v", r.touches)
	}
	r.sink.handle(wlTouchEvShape, shapeBody(1, 9, 9)) // чужое событие не ломает
	r.sink.handle(wlTouchEvFrame, nil)
	if len(r.touches) != 1 || r.touches[0].Major != 0 {
		t.Errorf("форма появилась без версии 6: %+v", r.touches)
	}
}

// Отмена доставляет накопленное до себя; выключение молча сбрасывает.
func TestWlTouch_ShapeCancelFlushesPending(t *testing.T) {
	r := shapedRig(1)
	r.sink.handle(wlTouchEvDown, touchDownBody(1, 10, mainSurf, 1, 5, 5))
	r.sink.handle(wlTouchEvShape, shapeBody(1, 8, 6))
	r.sink.handle(wlTouchEvCancel, nil)
	if len(r.touches) != 2 || r.touches[0].Phase != base.TouchBegan || r.touches[0].Major != 8 ||
		r.touches[1].Phase != base.TouchCancelled {
		t.Errorf("отмена: %+v", r.touches)
	}

	r2 := shapedRig(1)
	r2.sink.handle(wlTouchEvDown, touchDownBody(1, 10, mainSurf, 1, 5, 5))
	r2.sink.drop()
	r2.sink.handle(wlTouchEvFrame, nil)
	if len(r2.touches) != 0 {
		t.Errorf("сброшенное событие доставлено: %+v", r2.touches)
	}
	// Форма неизвестного пальца игнорируется.
	r2.sink.handle(wlTouchEvShape, shapeBody(77, 1, 1))
	if len(r2.sink.shape) != 0 {
		t.Errorf("форма чужого пальца запомнена: %v", r2.sink.shape)
	}
}
