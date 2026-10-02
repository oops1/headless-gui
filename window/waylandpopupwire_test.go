package window

import (
	"encoding/binary"
	"testing"
)

// Позиционер — единственный способ сказать компоновщику, где показать меню:
// экранных координат у клиента Wayland нет.

func TestWlPositionerRequests(t *testing.T) {
	reqs := wlPositionerRequests(42, 40, 28, 200, 160)
	if len(reqs) != 5 {
		t.Fatalf("запросов %d, ждал 5", len(reqs))
	}

	got := map[uint16][]uint32{}
	for _, raw := range reqs {
		if len(raw) < 8 {
			t.Fatalf("короткое сообщение: %v", raw)
		}
		if obj := binary.LittleEndian.Uint32(raw[0:4]); obj != 42 {
			t.Errorf("сообщение адресовано объекту %d, а не позиционеру", obj)
		}
		if size := int(binary.LittleEndian.Uint16(raw[6:8])); size != len(raw) {
			t.Errorf("в заголовке длина %d при %d байтах", size, len(raw))
		}
		op := binary.LittleEndian.Uint16(raw[4:6])
		var args []uint32
		for i := 8; i+4 <= len(raw); i += 4 {
			args = append(args, binary.LittleEndian.Uint32(raw[i:i+4]))
		}
		got[op] = args
	}

	if a := got[xdgPositionerSetSize]; len(a) != 2 || a[0] != 200 || a[1] != 160 {
		t.Errorf("set_size %v", a)
	}
	// Якорная область — точка 1×1 в нужном месте: попап цепляется за неё
	// углом и растёт вправо-вниз, как его рисует движок.
	if a := got[xdgPositionerSetAnchorRect]; len(a) != 4 ||
		a[0] != 40 || a[1] != 28 || a[2] != 1 || a[3] != 1 {
		t.Errorf("set_anchor_rect %v, ждал точку 40,28 1x1", a)
	}
	if a := got[xdgPositionerSetAnchor]; len(a) != 1 || a[0] != xdgAnchorTopLeft {
		t.Errorf("set_anchor %v", a)
	}
	if a := got[xdgPositionerSetGravity]; len(a) != 1 || a[0] != xdgGravityBottomRight {
		t.Errorf("set_gravity %v", a)
	}
	// Меню у нижнего края экрана должно раскрываться вверх, а не терять
	// нижние пункты.
	a := got[xdgPositionerSetConstraintAdjustment]
	if len(a) != 1 || a[0]&xdgConstraintFlipY == 0 || a[0]&xdgConstraintSlideX == 0 {
		t.Errorf("constraint_adjustment %v — попап у края экрана не подвинется", a)
	}
}

// Нулевой размер компоновщик считает протокольной ошибкой, а это разрыв
// соединения — то есть падение программы на пустом меню.
func TestWlPositionerRequests_ZeroSize(t *testing.T) {
	reqs := wlPositionerRequests(1, 0, 0, 0, 0)
	size := reqs[0]
	w := int32(binary.LittleEndian.Uint32(size[8:12]))
	h := int32(binary.LittleEndian.Uint32(size[12:16]))
	if w < 1 || h < 1 {
		t.Errorf("set_size %dx%d — компоновщик разорвёт соединение", w, h)
	}
}

func TestWlParsePopupConfigure(t *testing.T) {
	b := make([]byte, 16)
	shift := int32(-30)
	binary.LittleEndian.PutUint32(b[0:4], uint32(shift))
	binary.LittleEndian.PutUint32(b[4:8], 120)
	binary.LittleEndian.PutUint32(b[8:12], 200)
	binary.LittleEndian.PutUint32(b[12:16], 160)

	x, y, w, h, ok := wlParsePopupConfigure(b)
	if !ok {
		t.Fatal("configure не разобран")
	}
	// Отрицательный сдвиг законен: компоновщик подвинул попап влево, чтобы
	// тот влез на экран.
	if x != -30 || y != 120 || w != 200 || h != 160 {
		t.Errorf("разобрано %d,%d %dx%d", x, y, w, h)
	}

	if _, _, _, _, ok := wlParsePopupConfigure(b[:10]); ok {
		t.Error("обрезанное событие принято за целое")
	}
}
