package x11

// x11shape.go — форма окна-попапа X11 через расширение SHAPE: сборка запросов.
//
// Файл без платформенного суффикса — по той же причине, что x11touch.go и
// x11argb.go: раскладку байтов проверяют тесты на любой ОС.

import (
	"encoding/binary"
	"image"
)

const (
	x11ShapeRectanglesMinor = 1 // ShapeRectangles
	x11ShapeOpSet           = 0 // ShapeSet
	x11ShapeKindBounding    = 0 // ShapeBounding: и видимость, и ввод
	x11ShapeUnsorted        = 0 // порядок прямоугольников не обещаем
	x11GetSelectionOwnerOp  = 23
)

// x11ShapeRectanglesRequest — ShapeRectangles(Set, Bounding, Unsorted): форма
// окна — объединение rects (координаты окна). Пустой список — окно целиком
// невидимо: это честная форма для кадра без единой сплошной точки.
func x11ShapeRectanglesRequest(major byte, wid uint32, rects []image.Rectangle) []byte {
	buf := make([]byte, 16+8*len(rects))
	buf[0] = major
	buf[1] = x11ShapeRectanglesMinor
	binary.LittleEndian.PutUint16(buf[2:4], uint16(len(buf)/4))
	buf[4] = x11ShapeOpSet
	buf[5] = x11ShapeKindBounding
	buf[6] = x11ShapeUnsorted
	binary.LittleEndian.PutUint32(buf[8:12], wid)
	// Смещение формы (12:16) — ноль: прямоугольники уже в координатах окна.
	for i, r := range rects {
		o := 16 + 8*i
		binary.LittleEndian.PutUint16(buf[o:o+2], uint16(int16(r.Min.X)))
		binary.LittleEndian.PutUint16(buf[o+2:o+4], uint16(int16(r.Min.Y)))
		binary.LittleEndian.PutUint16(buf[o+4:o+6], uint16(r.Dx()))
		binary.LittleEndian.PutUint16(buf[o+6:o+8], uint16(r.Dy()))
	}
	return buf
}

// x11GetSelectionOwnerRequest — GetSelectionOwner(selection).
func x11GetSelectionOwnerRequest(selection uint32) []byte {
	buf := make([]byte, 8)
	buf[0] = x11GetSelectionOwnerOp
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint32(buf[4:8], selection)
	return buf
}
