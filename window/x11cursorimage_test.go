package window

import (
	"encoding/binary"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// X11: курсор из картинки — раскладка запросов RENDER и разбор ответов.

func x11ciImage(w, h int, c color.NRGBA, hx, hy int) *widget.CursorImage {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return widget.NewCursorImage(img, hx, hy)
}

func TestX11RenderRequests_Layout(t *testing.T) {
	v := x11RenderQueryVersionRequest(140)
	if len(v) != 12 || v[0] != 140 || v[1] != renderMinorQueryVersion ||
		binary.LittleEndian.Uint16(v[2:4]) != 3 || binary.LittleEndian.Uint32(v[8:12]) != 11 {
		t.Errorf("RenderQueryVersion: %v", v)
	}
	f := x11RenderQueryPictFormatsRequest(140)
	if len(f) != 4 || f[0] != 140 || f[1] != renderMinorQueryPictFormats || binary.LittleEndian.Uint16(f[2:4]) != 1 {
		t.Errorf("RenderQueryPictFormats: %v", f)
	}
	p := x11RenderCreatePictureRequest(140, 0x501, 0x502, 0x77)
	if len(p) != 20 || p[1] != renderMinorCreatePicture || binary.LittleEndian.Uint16(p[2:4]) != 5 ||
		binary.LittleEndian.Uint32(p[4:8]) != 0x501 || binary.LittleEndian.Uint32(p[8:12]) != 0x502 ||
		binary.LittleEndian.Uint32(p[12:16]) != 0x77 || binary.LittleEndian.Uint32(p[16:20]) != 0 {
		t.Errorf("RenderCreatePicture: %v", p)
	}
	c := x11RenderCreateCursorRequest(140, 0x600, 0x501, 12, 34)
	if len(c) != 16 || c[1] != renderMinorCreateCursor || binary.LittleEndian.Uint16(c[2:4]) != 4 ||
		binary.LittleEndian.Uint32(c[4:8]) != 0x600 || binary.LittleEndian.Uint32(c[8:12]) != 0x501 ||
		binary.LittleEndian.Uint16(c[12:14]) != 12 || binary.LittleEndian.Uint16(c[14:16]) != 34 {
		t.Errorf("RenderCreateCursor: %v", c)
	}
	fp := x11RenderFreePictureRequest(140, 0x501)
	if len(fp) != 8 || fp[1] != renderMinorFreePicture || binary.LittleEndian.Uint32(fp[4:8]) != 0x501 {
		t.Errorf("RenderFreePicture: %v", fp)
	}
}

func TestParseRenderVersion(t *testing.T) {
	r := make([]byte, 32)
	r[0] = 1
	binary.LittleEndian.PutUint32(r[8:], 0)
	binary.LittleEndian.PutUint32(r[12:], 11)
	maj, min, ok := parseRenderVersion(r)
	if !ok || maj != 0 || min != 11 || !renderVersionHasCursor(maj, min) {
		t.Errorf("0.11: %d.%d %v", maj, min, ok)
	}
	for _, tc := range []struct {
		maj, min int
		want     bool
	}{{0, 4, false}, {0, 5, true}, {1, 0, true}} {
		if renderVersionHasCursor(tc.maj, tc.min) != tc.want {
			t.Errorf("%d.%d: ждал %v", tc.maj, tc.min, tc.want)
		}
	}
	if _, _, ok := parseRenderVersion(r[:10]); ok {
		t.Error("обрезанный ответ принят")
	}
	r[0] = 0
	if _, _, ok := parseRenderVersion(r); ok {
		t.Error("ошибка принята за ответ")
	}
}

// xrFormat собирает PICTFORMINFO.
func xrFormat(id uint32, typ, depth byte, rs, rm, gs, gm, bs, bm, as, am uint16) []byte {
	b := make([]byte, 28)
	binary.LittleEndian.PutUint32(b[0:], id)
	b[4], b[5] = typ, depth
	for i, v := range []uint16{rs, rm, gs, gm, bs, bm, as, am} {
		binary.LittleEndian.PutUint16(b[8+2*i:], v)
	}
	return b
}

func xrFormatsReply(formats ...[]byte) []byte {
	r := make([]byte, 32)
	r[0] = 1
	binary.LittleEndian.PutUint32(r[8:], uint32(len(formats)))
	for _, f := range formats {
		r = append(r, f...)
	}
	binary.LittleEndian.PutUint32(r[4:8], uint32((len(r)-32)/4)) // длина ответа в словах сверх 32 байт
	return r
}

func TestParseRenderArgbFormat(t *testing.T) {
	rgb24 := xrFormat(0x20, 1, 24, 16, 255, 8, 255, 0, 255, 0, 0)
	a8 := xrFormat(0x21, 1, 8, 0, 0, 0, 0, 0, 0, 0, 255)
	indexed32 := xrFormat(0x22, 0, 32, 16, 255, 8, 255, 0, 255, 24, 255) // не Direct
	bgra := xrFormat(0x23, 1, 32, 8, 255, 16, 255, 24, 255, 0, 255)      // другой порядок
	argb := xrFormat(0x77, 1, 32, 16, 255, 8, 255, 0, 255, 24, 255)
	argb24 := xrFormat(0x24, 1, 24, 16, 255, 8, 255, 0, 255, 24, 255) // маски те же, но глубина 24

	if got := parseRenderArgbFormat(xrFormatsReply(rgb24, a8, indexed32, bgra, argb24, argb)); got != 0x77 {
		t.Errorf("формат ARGB32 = %#x, ждал 0x77", got)
	}
	if got := parseRenderArgbFormat(xrFormatsReply(rgb24, a8, indexed32, bgra)); got != 0 {
		t.Errorf("без ARGB32 нашёлся формат %#x", got)
	}
	// Ответ обрезан посреди списка.
	if got := parseRenderArgbFormat(xrFormatsReply(rgb24, argb)[:32+28+10]); got != 0 {
		t.Errorf("обрезанный ответ дал формат %#x", got)
	}
	if parseRenderArgbFormat(nil) != 0 {
		t.Error("пустой ответ дал формат")
	}
}

// Картинка режется на запросы под предел X11, строки не теряются и идут по
// порядку.
func TestX11PutImageRequests_Chunks(t *testing.T) {
	const w, h = 384, 384
	data := make([]byte, w*h*4)
	for i := range data {
		data[i] = byte(i * 7)
	}
	reqs := x11PutImageRequests(0x500, 0x501, w, h, 32, data)
	if len(reqs) < 2 {
		t.Fatalf("картинка 384×384 ушла %d запросом: превысила бы предел запроса", len(reqs))
	}
	rows, y := 0, 0
	for i, r := range reqs {
		if len(r) > 262140 {
			t.Errorf("запрос %d: %d байт, больше предела 262140", i, len(r))
		}
		if r[0] != 72 || r[1] != 2 || r[21] != 32 {
			t.Errorf("запрос %d: opcode %d format %d depth %d", i, r[0], r[1], r[21])
		}
		if int(binary.LittleEndian.Uint16(r[2:4]))*4 != len(r) {
			t.Errorf("запрос %d: длина в словах не совпадает с размером", i)
		}
		if binary.LittleEndian.Uint32(r[4:8]) != 0x500 || binary.LittleEndian.Uint32(r[8:12]) != 0x501 {
			t.Errorf("запрос %d: drawable/gc", i)
		}
		rw := int(binary.LittleEndian.Uint16(r[12:14]))
		rh := int(binary.LittleEndian.Uint16(r[14:16]))
		dy := int(binary.LittleEndian.Uint16(r[18:20]))
		if rw != w || dy != y {
			t.Errorf("запрос %d: ширина %d dst-y %d, ждал %d и %d", i, rw, dy, w, y)
		}
		if !reflect.DeepEqual(r[24:], data[y*w*4:(y+rh)*w*4]) {
			t.Errorf("запрос %d: пиксели искажены", i)
		}
		rows += rh
		y += rh
	}
	if rows != h {
		t.Errorf("передано строк %d из %d", rows, h)
	}
	if x11PutImageRequests(1, 2, 4, 4, 32, make([]byte, 10)) != nil {
		t.Error("нехватка данных не отвергнута")
	}
}

func TestX11RenderCursorRequests_Order(t *testing.T) {
	ci := x11ciImage(32, 32, color.NRGBA{R: 255, A: 128}, 5, 6)
	ids := x11RenderCursorIDs{cursor: 0x600, drawable: 0x400, pixmap: 0x601, gc: 0x602, picture: 0x603}
	reqs := x11RenderCursorRequests(ids, 140, 0x77, ci)

	want := []struct {
		op    byte
		minor int // для RENDER; -1 — ядро
	}{
		{x11OpCreatePixmap, -1}, {x11OpCreateGC, -1}, {72, -1},
		{140, renderMinorCreatePicture}, {140, renderMinorCreateCursor},
		{140, renderMinorFreePicture}, {x11OpFreeGC, -1}, {x11OpFreePixmap, -1},
	}
	if len(reqs) != len(want) {
		t.Fatalf("запросов %d, ждал %d", len(reqs), len(want))
	}
	for i, wnt := range want {
		if reqs[i][0] != wnt.op || (wnt.minor >= 0 && int(reqs[i][1]) != wnt.minor) {
			t.Errorf("запрос %d: opcode %d minor %d, ждал %d/%d", i, reqs[i][0], reqs[i][1], wnt.op, wnt.minor)
		}
	}
	if reqs[0][1] != 32 { // depth пиксмапа
		t.Errorf("глубина пиксмапа %d, ждал 32", reqs[0][1])
	}
	// Пиксели — премультиплицированный BGRA: R=255, A=128 → B=0 G=0 R=128 A=128.
	if got := reqs[2][24:28]; !reflect.DeepEqual(got, []byte{0, 0, 128, 128}) {
		t.Errorf("первый пиксель %v, ждал [0 0 128 128]", got)
	}
	cur := reqs[4]
	if binary.LittleEndian.Uint32(cur[8:12]) != 0x603 || binary.LittleEndian.Uint16(cur[12:14]) != 5 || binary.LittleEndian.Uint16(cur[14:16]) != 6 {
		t.Errorf("CreateCursor: источник %#x hotspot (%d,%d)", binary.LittleEndian.Uint32(cur[8:12]),
			binary.LittleEndian.Uint16(cur[12:14]), binary.LittleEndian.Uint16(cur[14:16]))
	}
	if binary.LittleEndian.Uint32(reqs[3][12:16]) != 0x77 {
		t.Error("CreatePicture без формата ARGB32")
	}
}

// Без RENDER курсор строится 1-битным: непрозрачные точки в маске, тёмные — в
// рисунке; прозрачные не попадают никуда.
func TestX11MonoImageCursorRequests(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})                         // чёрная
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255}) // белая
	img.SetNRGBA(2, 0, color.NRGBA{A: 0})                           // прозрачная
	img.SetNRGBA(3, 0, color.NRGBA{A: 200})                         // полупрозрачная чёрная: выше порога
	ci := widget.NewCursorImage(img, 1, 0)
	ids := x11CursorIDs{cursor: 9, drawable: 1, pixSrc: 2, pixMask: 3, gc0: 4, gc1: 5}
	reqs := x11MonoImageCursorRequests(ids, ci)

	var polys [][]byte
	var create []byte
	for _, r := range reqs {
		switch r[0] {
		case x11OpPolyPoint:
			polys = append(polys, r)
		case x11OpCreateCursor:
			create = r
		}
	}
	if len(polys) != 2 {
		t.Fatalf("PolyPoint: %d, ждал 2 (рисунок и маска)", len(polys))
	}
	points := func(r []byte) int { return (len(r) - 12) / 4 }
	if points(polys[0]) != 2 || points(polys[1]) != 3 {
		t.Errorf("точек в рисунке %d (ждал 2), в маске %d (ждал 3)", points(polys[0]), points(polys[1]))
	}
	if create == nil || binary.LittleEndian.Uint16(create[28:30]) != 1 || binary.LittleEndian.Uint16(create[30:32]) != 0 {
		t.Errorf("CreateCursor/горячая точка: %v", create)
	}
}

// Прежний набор форм не изменился после выноса общей части.
func TestX11BitmapCursorRequests_Unchanged(t *testing.T) {
	ids := x11CursorIDs{cursor: 9, drawable: 1, pixSrc: 2, pixMask: 3, gc0: 4, gc1: 5}
	none := x11BitmapCursorRequests(ids, wlCursorNone)
	if len(none) != 11 || none[0][0] != x11OpCreatePixmap || none[6][0] != x11OpCreateCursor {
		t.Errorf("невидимый курсор: %d запросов, [6]=%d", len(none), none[6][0])
	}
}
