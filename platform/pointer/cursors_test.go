package pointer

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

func TestWlCursorPixels(t *testing.T) {
	seen := map[string]int{}
	for c := 0; c < CursorCount; c++ {
		if c == CursorNone {
			continue // спрятанный курсор картинки не имеет
		}
		pix, w, h, hx, hy := CursorPixels(c)
		if w != CursorSize || h != CursorSize {
			t.Fatalf("курсор %d: размер %dx%d, ждал %dx%d", c, w, h, CursorSize, CursorSize)
		}
		if len(pix) != w*h*4 {
			t.Fatalf("курсор %d: %d байт при %dx%d", c, len(pix), w, h)
		}
		if hx < 0 || hx >= w || hy < 0 || hy >= h {
			t.Errorf("курсор %d: горячая точка (%d,%d) вне картинки", c, hx, hy)
		}
		opaque := 0
		for i := 3; i < len(pix); i += 4 {
			if pix[i] != 0 {
				opaque++
			}
		}
		if opaque == 0 {
			t.Errorf("курсор %d пустой — под окном остался бы чужой курсор", c)
		}
		seen[string(pix)]++
	}
	// Формы обязаны отличаться: одинаковая картинка у стрелки и текста
	// означала бы, что курсор над полем ввода не меняется.
	for _, n := range seen {
		if n > 1 {
			t.Error("две формы курсора нарисованы одинаково")
		}
	}

	// Неизвестная форма — стрелка, а не пустота.
	unknown, _, _, _, _ := CursorPixels(CursorCount + 5)
	arrow, _, _, _, _ := CursorPixels(CursorArrow)
	if string(unknown) != string(arrow) {
		t.Error("неизвестная форма нарисована не стрелкой")
	}
}

func TestWlCursorShape(t *testing.T) {
	// Имена форм wp_cursor_shape_device_v1: движок просит ровно те, что
	// описаны в расширении.
	want := map[int]uint32{
		CursorArrow:    1,  // default
		CursorIBeam:    9,  // text
		CursorHand:     4,  // pointer
		CursorSizeWE:   26, // ew-resize
		CursorSizeNS:   27, // ns-resize
		CursorSizeNWSE: 29, // nwse-resize
		CursorSizeNESW: 28, // nesw-resize
	}
	for c, w := range want {
		if got := WlCursorShape(c); got != w {
			t.Errorf("форма %d: shape %d, ждал %d", c, got, w)
		}
	}
	if got := WlCursorShape(CursorCount + 1); got != 0 {
		t.Errorf("неизвестная форма: shape %d, ждал 0 (рисовать самим)", got)
	}
}

// Нарисованные курсоры не больше 32×32: крупнее указатель не пройдёт через RDP
// (WinLine). Это же проверка, что у каждой новой формы есть картинка.
func TestCursorArt_FitsRDPLimit(t *testing.T) {
	if CursorSize > 32 {
		t.Fatalf("CursorSize = %d, предел 32", CursorSize)
	}
	for _, c := range []int{
		CursorArrow, CursorIBeam, CursorHand, CursorSizeWE, CursorSizeNS,
		CursorSizeNWSE, CursorSizeNESW, CursorGrab, CursorGrabbing,
		CursorSizeAll, CursorCrosshair, CursorWait,
	} {
		art := wlCursorShapes[c]
		if art.rows == nil {
			t.Errorf("форма %d: картинки нет", c)
			continue
		}
		if len(art.rows) > 32 {
			t.Errorf("форма %d: %d строк > 32", c, len(art.rows))
		}
		opaque := 0
		for y, row := range art.rows {
			if len(row) > 32 {
				t.Errorf("форма %d: строка %d шире 32", c, y)
			}
			for x := 0; x < len(row); x++ {
				if row[x] == 'X' || row[x] == 'o' {
					opaque++
				}
			}
		}
		if opaque == 0 {
			t.Errorf("форма %d: картинка пуста", c)
		}
		if art.hotX < 0 || art.hotX >= CursorSize || art.hotY < 0 || art.hotY >= CursorSize {
			t.Errorf("форма %d: горячая точка (%d,%d) вне картинки", c, art.hotX, art.hotY)
		}
	}
}

// Коды форм окна совпадают с числовыми значениями base.Cursor: SetCursor
// принимает число, а старые значения менять нельзя.
func TestCursorCodes_MatchWidget(t *testing.T) {
	want := map[string]int{
		"Arrow": CursorArrow, "IBeam": CursorIBeam, "Hand": CursorHand,
		"SizeWE": CursorSizeWE, "SizeNS": CursorSizeNS,
		"SizeNWSE": CursorSizeNWSE, "SizeNESW": CursorSizeNESW,
		"None": CursorNone, "Grab": CursorGrab, "Grabbing": CursorGrabbing,
		"SizeAll": CursorSizeAll, "Crosshair": CursorCrosshair, "Wait": CursorWait,
	}
	got := map[string]int{
		"Arrow": int(base.CursorArrow), "IBeam": int(base.CursorIBeam), "Hand": int(base.CursorHand),
		"SizeWE": int(base.CursorSizeWE), "SizeNS": int(base.CursorSizeNS),
		"SizeNWSE": int(base.CursorSizeNWSE), "SizeNESW": int(base.CursorSizeNESW),
		"None": int(base.CursorNone), "Grab": int(base.CursorGrab), "Grabbing": int(base.CursorGrabbing),
		"SizeAll": int(base.CursorSizeAll), "Crosshair": int(base.CursorCrosshair), "Wait": int(base.CursorWait),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: код окна %d, base.Cursor %d", k, v, got[k])
		}
	}
}
