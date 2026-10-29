package core

import (
	"encoding/binary"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Невидимый курсор и руки на X11 строятся из 1-битных карт запросами ядра
// протокола — шрифт «cursor» их не содержит. Проверяем сам поток запросов.

var testCursorIDs = x11CursorIDs{cursor: 100, drawable: 1, pixSrc: 101, pixMask: 102, gc0: 103, gc1: 104}

// opcodes возвращает коды запросов потока.
func opcodes(reqs [][]byte) []byte {
	var out []byte
	for _, r := range reqs {
		out = append(out, r[0])
	}
	return out
}

// Каждый запрос выровнен на 4 байта, и поле длины совпадает с его размером:
// иначе сервер потеряет границу следующего запроса.
func checkLengths(t *testing.T, reqs [][]byte) {
	t.Helper()
	for i, r := range reqs {
		if len(r)%4 != 0 {
			t.Errorf("запрос %d (opcode %d): длина %d не кратна 4", i, r[0], len(r))
		}
		if got := int(binary.LittleEndian.Uint16(r[2:4])); got*4 != len(r) {
			t.Errorf("запрос %d (opcode %d): поле длины %d слов при %d байтах", i, r[0], got, len(r))
		}
	}
}

func TestX11BitmapCursor_NoneIsInvisible(t *testing.T) {
	reqs := x11BitmapCursorRequests(testCursorIDs, wlCursorNone)
	checkLengths(t, reqs)

	want := []byte{
		x11OpCreatePixmap, x11OpCreatePixmap,
		x11OpCreateGC, x11OpCreateGC,
		x11OpPolyFillRectangle, x11OpPolyFillRectangle, // очистка обеих карт
		x11OpCreateCursor,
		x11OpFreeGC, x11OpFreeGC, x11OpFreePixmap, x11OpFreePixmap,
	}
	got := opcodes(reqs)
	if string(got) != string(want) {
		t.Fatalf("запросы %v, ждал %v", got, want)
	}
	// Ни одной точки не нарисовано: маска пуста, курсор не виден.
	for _, r := range reqs {
		if r[0] == x11OpPolyPoint {
			t.Fatal("у невидимого курсора не должно быть закрашенных точек")
		}
	}
	// Карты 1×1, глубина 1.
	pm := reqs[0]
	if pm[1] != 1 || binary.LittleEndian.Uint16(pm[12:14]) != 1 || binary.LittleEndian.Uint16(pm[14:16]) != 1 {
		t.Errorf("карта невидимого курсора: depth=%d %dx%d, ждал 1, 1x1", pm[1],
			binary.LittleEndian.Uint16(pm[12:14]), binary.LittleEndian.Uint16(pm[14:16]))
	}
	// Очистка рисует цветом 0 (контекст gc0), а не 1.
	if gc := binary.LittleEndian.Uint32(reqs[4][8:12]); gc != testCursorIDs.gc0 {
		t.Errorf("очистка идёт контекстом %d, ждал gc0=%d", gc, testCursorIDs.gc0)
	}
	// CreateCursor ссылается на созданные карты.
	cc := reqs[6]
	if binary.LittleEndian.Uint32(cc[4:8]) != testCursorIDs.cursor ||
		binary.LittleEndian.Uint32(cc[8:12]) != testCursorIDs.pixSrc ||
		binary.LittleEndian.Uint32(cc[12:16]) != testCursorIDs.pixMask {
		t.Error("CreateCursor ссылается не на те ресурсы")
	}
}

func TestX11BitmapCursor_HandsDrawn(t *testing.T) {
	for _, c := range []int{wlCursorGrab, wlCursorGrabbing} {
		reqs := x11BitmapCursorRequests(testCursorIDs, c)
		checkLengths(t, reqs)
		var srcPts, maskPts int
		for _, r := range reqs {
			if r[0] != x11OpPolyPoint {
				continue
			}
			n := (len(r) - 12) / 4
			switch binary.LittleEndian.Uint32(r[4:8]) {
			case testCursorIDs.pixSrc:
				srcPts = n
			case testCursorIDs.pixMask:
				maskPts = n
			}
		}
		if srcPts == 0 || maskPts == 0 {
			t.Fatalf("форма %d: контур %d точек, маска %d — рука не нарисована", c, srcPts, maskPts)
		}
		// Маска — контур плюс заливка, значит строго больше контура.
		if maskPts <= srcPts {
			t.Errorf("форма %d: маска (%d) должна быть шире контура (%d)", c, maskPts, srcPts)
		}
		// Горячая точка внутри картинки.
		cc := reqs[len(reqs)-5]
		hx, hy := binary.LittleEndian.Uint16(cc[28:30]), binary.LittleEndian.Uint16(cc[30:32])
		if hx >= wlCursorSize || hy >= wlCursorSize {
			t.Errorf("форма %d: горячая точка (%d,%d) вне картинки", c, hx, hy)
		}
	}
}

// Рука раскрытая и сжатая — разные картинки.
func TestX11BitmapCursor_GrabDiffersFromGrabbing(t *testing.T) {
	a := x11BitmapCursorRequests(testCursorIDs, wlCursorGrab)
	b := x11BitmapCursorRequests(testCursorIDs, wlCursorGrabbing)
	same := len(a) == len(b)
	if same {
		for i := range a {
			if string(a[i]) != string(b[i]) {
				same = false
				break
			}
		}
	}
	if same {
		t.Error("Grab и Grabbing нарисованы одинаково")
	}
}

// Нарисованные курсоры не больше 32×32: крупнее указатель не пройдёт через RDP
// (WinLine). Это же проверка, что у каждой новой формы есть картинка.
func TestCursorArt_FitsRDPLimit(t *testing.T) {
	if wlCursorSize > 32 {
		t.Fatalf("wlCursorSize = %d, предел 32", wlCursorSize)
	}
	for _, c := range []int{
		wlCursorArrow, wlCursorIBeam, wlCursorHand, wlCursorSizeWE, wlCursorSizeNS,
		wlCursorSizeNWSE, wlCursorSizeNESW, wlCursorGrab, wlCursorGrabbing,
		wlCursorSizeAll, wlCursorCrosshair, wlCursorWait,
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
		if art.hotX < 0 || art.hotX >= wlCursorSize || art.hotY < 0 || art.hotY >= wlCursorSize {
			t.Errorf("форма %d: горячая точка (%d,%d) вне картинки", c, art.hotX, art.hotY)
		}
	}
}

// Коды форм окна совпадают с числовыми значениями widget.Cursor: SetCursor
// принимает число, а старые значения менять нельзя.
func TestCursorCodes_MatchWidget(t *testing.T) {
	want := map[string]int{
		"Arrow": wlCursorArrow, "IBeam": wlCursorIBeam, "Hand": wlCursorHand,
		"SizeWE": wlCursorSizeWE, "SizeNS": wlCursorSizeNS,
		"SizeNWSE": wlCursorSizeNWSE, "SizeNESW": wlCursorSizeNESW,
		"None": wlCursorNone, "Grab": wlCursorGrab, "Grabbing": wlCursorGrabbing,
		"SizeAll": wlCursorSizeAll, "Crosshair": wlCursorCrosshair, "Wait": wlCursorWait,
	}
	got := map[string]int{
		"Arrow": int(widget.CursorArrow), "IBeam": int(widget.CursorIBeam), "Hand": int(widget.CursorHand),
		"SizeWE": int(widget.CursorSizeWE), "SizeNS": int(widget.CursorSizeNS),
		"SizeNWSE": int(widget.CursorSizeNWSE), "SizeNESW": int(widget.CursorSizeNESW),
		"None": int(widget.CursorNone), "Grab": int(widget.CursorGrab), "Grabbing": int(widget.CursorGrabbing),
		"SizeAll": int(widget.CursorSizeAll), "Crosshair": int(widget.CursorCrosshair), "Wait": int(widget.CursorWait),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: код окна %d, widget.Cursor %d", k, v, got[k])
		}
	}
}
