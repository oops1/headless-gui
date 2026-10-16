package window

import (
	"encoding/binary"
	"image"
	"testing"
)

// Байты ShapeRectangles: Set, Bounding, Unsorted, окно, нулевое смещение и
// прямоугольники x, y, ширина, высота.
func TestX11ShapeRectanglesRequest(t *testing.T) {
	req := x11ShapeRectanglesRequest(130, 0x400001, []image.Rectangle{
		image.Rect(2, 3, 12, 8), image.Rect(0, 8, 20, 30),
	})
	if len(req) != 32 || req[0] != 130 || req[1] != 1 || binary.LittleEndian.Uint16(req[2:4]) != 8 {
		t.Fatalf("заголовок %v", req[:4])
	}
	if req[4] != 0 || req[5] != 0 || req[6] != 0 || binary.LittleEndian.Uint32(req[8:12]) != 0x400001 {
		t.Fatalf("op/kind/ordering/окно: %v", req[4:12])
	}
	got := []uint16{}
	for o := 16; o < len(req); o += 2 {
		got = append(got, binary.LittleEndian.Uint16(req[o:o+2]))
	}
	want := []uint16{2, 3, 10, 5, 0, 8, 20, 22}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("прямоугольники %v, ждали %v", got, want)
		}
	}
	if empty := x11ShapeRectanglesRequest(130, 1, nil); len(empty) != 16 || binary.LittleEndian.Uint16(empty[2:4]) != 4 {
		t.Fatalf("пустая форма: %v", empty)
	}
	if r := x11GetSelectionOwnerRequest(77); r[0] != 23 || binary.LittleEndian.Uint16(r[2:4]) != 2 || binary.LittleEndian.Uint32(r[4:8]) != 77 {
		t.Fatalf("GetSelectionOwner: %v", r)
	}
}
