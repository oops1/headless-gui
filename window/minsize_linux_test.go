package window

import (
	"encoding/binary"
	"testing"
)

// TestX11NormalHintsMinSize проверяет байтовую раскладку WM_NORMAL_HINTS
// (ICCCM §4.1.2.3): бит PMinSize и поля min_width/min_height на месте,
// остальные 18 CARD32-полей нулевые, а «нет минимума» (0/отрицательные)
// не выставляет флаг вовсе — иначе WM решил бы, что минимум явно запрошен
// нулевым, и (в теории) стал бы вести себя не так, как без хинта совсем.
func TestX11NormalHintsMinSize(t *testing.T) {
	const pMinSize = 0x10

	cases := []struct {
		name          string
		width, height int
		wantFlags     uint32
		wantMinW      uint32
		wantMinH      uint32
	}{
		{"обычный минимум 800x600", 800, 600, pMinSize, 800, 600},
		{"нулевые — минимума нет", 0, 0, 0, 0, 0},
		{"отрицательные — минимума нет", -1, -1, 0, 0, 0},
		{"только ширина", 320, 0, pMinSize, 320, 0},
		{"только высота", 0, 240, pMinSize, 0, 240},
		{"ширина отрицательная, высота задана", -5, 240, pMinSize, 0, 240},
		{"единица — тоже минимум, не «нет»", 1, 1, pMinSize, 1, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := x11NormalHintsMinSize(c.width, c.height)

			if len(buf) != 18*4 {
				t.Fatalf("длина тела WM_NORMAL_HINTS = %d, ждали %d (18×CARD32)", len(buf), 18*4)
			}

			flags := binary.LittleEndian.Uint32(buf[0:4])
			if flags != c.wantFlags {
				t.Errorf("flags = %#x, ждали %#x", flags, c.wantFlags)
			}
			minW := binary.LittleEndian.Uint32(buf[20:24])
			minH := binary.LittleEndian.Uint32(buf[24:28])
			if minW != c.wantMinW || minH != c.wantMinH {
				t.Errorf("min_width,min_height = %d,%d; ждали %d,%d", minW, minH, c.wantMinW, c.wantMinH)
			}

			// Остальные поля (max_size, resize_inc, aspect, base_size,
			// win_gravity) — байты 28..71 — этот бэкенд не заполняет вовсе.
			for i := 28; i < len(buf); i++ {
				if buf[i] != 0 {
					t.Fatalf("byte[%d] = %d, ждали 0 (поле вне min_width/min_height не используется)", i, buf[i])
				}
			}
		})
	}
}

// TestX11NormalHintsMinSize_PadUntouched проверяет, что устаревшие
// pad-поля (offset 4..19, old_x/old_y/old_width/old_height) остаются
// нулевыми — их заполнение не входит в контракт SetMinSize и могло бы
// сбить с толку клиентов, читающих эти поля буквально.
func TestX11NormalHintsMinSize_PadUntouched(t *testing.T) {
	buf := x11NormalHintsMinSize(800, 600)
	for i := 4; i < 20; i++ {
		if buf[i] != 0 {
			t.Fatalf("pad byte[%d] = %d, ждали 0", i, buf[i])
		}
	}
}

// TestWlMinSizeArgs проверяет перевод физических пикселей в surface-local
// координаты xdg_toplevel.set_min_size: без масштаба (scale=1) значения не
// меняются, на HiDPI — делятся на scale с округлением, а 0/отрицательные
// по каждой оси независимо превращаются в 0 («минимума нет» по спецификации
// xdg_toplevel — value of zero means no limit in that dimension).
func TestWlMinSizeArgs(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
		scale         float64
		wantW, wantH  int32
	}{
		{"scale=1 — без пересчёта", 800, 600, 1, 800, 600},
		{"scale=2 — HiDPI делит пополам", 800, 600, 2, 400, 300},
		{"scale=1.5 — округление к ближайшему", 801, 601, 1.5, 534, 401}, // 801/1.5=534.0, 601/1.5=400.67→401
		{"нулевые — минимума нет", 0, 0, 1, 0, 0},
		{"отрицательные — минимума нет", -10, -20, 2, 0, 0},
		{"только ширина", 400, 0, 2, 200, 0},
		{"scale<=0 — трактуем как 1 (нет данных о масштабе)", 800, 600, 0, 800, 600},
		{"scale отрицательный — тоже как 1", 800, 600, -3, 800, 600},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotW, gotH := wlMinSizeArgs(c.width, c.height, c.scale)
			if gotW != c.wantW || gotH != c.wantH {
				t.Errorf("wlMinSizeArgs(%d,%d,%v) = %d,%d; ждали %d,%d",
					c.width, c.height, c.scale, gotW, gotH, c.wantW, c.wantH)
			}
		})
	}
}

// Минимум, заданный ДО создания окна, не теряется.
//
// WM читает WM_NORMAL_HINTS при показе окна, поэтому свойство обязано попасть
// в него до MapWindow. Раньше SetMinSize с нулевым wid просто выходил, а
// Window звал его уже после Create — то есть после показа.
func TestX11SetMinSize_BeforeCreateIsRemembered(t *testing.T) {
	w := &X11Window{}
	w.SetMinSize(800, 600)

	if !w.minWant || w.minW != 800 || w.minH != 600 {
		t.Fatalf("минимум не отложен: want=%v %dx%d", w.minWant, w.minW, w.minH)
	}

	// applyPendingMinSize при нулевом wid снова отложит его же — проверяем,
	// что признак снимается и повторного применения не будет.
	w.applyPendingMinSize()
	if w.minW != 800 || w.minH != 600 {
		t.Errorf("после применения запомнено %dx%d", w.minW, w.minH)
	}
}

// Ничего не задано — применять нечего.
func TestX11SetMinSize_NothingPending(t *testing.T) {
	w := &X11Window{}
	w.applyPendingMinSize()
	if w.minWant {
		t.Error("признак отложенного минимума взялся из ниоткуда")
	}
}

// То же для Wayland: Create фиксирует размер, и отложенный минимум обязан
// применяться ПОСЛЕ фиксации, иначе она его затрёт.
func TestWaylandSetMinSize_BeforeCreateIsRemembered(t *testing.T) {
	w := &WaylandWindow{}
	w.SetMinSize(640, 480)

	if !w.minWant || w.minW != 640 || w.minH != 480 {
		t.Errorf("минимум не отложен: want=%v %dx%d", w.minWant, w.minW, w.minH)
	}
}
