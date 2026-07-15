// Package widget содержит интерфейсы и реализации UI-виджетов.
//
// Координатная система: все Bounds задаются в абсолютных пикселях
// пространства холста. Движок не применяет дополнительных трансформаций.
package widget

import (
	"image"
	"image/color"
)

// DefaultFontSizePt — размер шрифта по умолчанию (в пунктах) для DrawText.
// Должен совпадать с engine.DefaultFontSize.
const DefaultFontSizePt = 10.0

// AAShapes — опциональные сглаженные (antialiased) примитивы рисования.
// Реализуется engine.Canvas. Кастомные реализации DrawContext могут не
// поддерживать интерфейс — код виджетов проверяет его type-assert'ом и
// откатывается на ступенчатую отрисовку (Bresenham/scanline).
type AAShapes interface {
	// FillEllipseAA — залитый эллипс с центром (cx, cy) и полуосями rx, ry.
	FillEllipseAA(cx, cy, rx, ry int, col color.RGBA)
	// StrokeEllipseAA — контур эллипса толщиной thickness.
	StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA)
	// FillPolygonAA — залитый многоугольник.
	FillPolygonAA(pts []image.Point, col color.RGBA)
	// StrokePolylineAA — ломаная толщиной thickness (closed замыкает контур).
	StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA)
	// DrawLineAA — отрезок толщиной thickness.
	DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA)
}

// Snapshotter — опциональный интерфейс DrawContext, умеющий скопировать
// прямоугольную область уже отрисованного холста в самостоятельный *image.RGBA.
// Реализуется engine.Canvas (по образцу AAShapes: код виджетов проверяет его
// type-assert'ом и откатывается, если контекст интерфейс не поддерживает).
//
// Координаты r — ЛОГИЧЕСКИЕ; результат — в физических пикселях холста
// (premultiplied RGBA), пригодных для обратного блита через DrawImage/
// DrawImageScaled. Пустое пересечение r с холстом → nil.
//
// Используется DockManager.DrawOverlay для «призрака» перетаскиваемой панели
// (снимок панели следует за курсором, как в Visual Studio).
type Snapshotter interface {
	Snapshot(r image.Rectangle) *image.RGBA
}

// OverlayDrawer реализуется виджетами, которым нужно рисовать поверх всего дерева.
// Например, раскрытый выпадающий список должен перекрывать соседние виджеты.
//
// Движок вызывает DrawOverlay после отрисовки всего дерева виджетов.
// HasOverlay возвращает true, если overlay нужно рисовать в данный момент.
type OverlayDrawer interface {
	HasOverlay() bool
	DrawOverlay(ctx DrawContext)
}

// OverlayBoundsProvider — оверлей-виджет, умеющий сообщить прямоугольник
// своего оверлея в абсолютных ЛОГИЧЕСКИХ координатах холста. Пустой Rect
// означает, что активного оверлея нет.
//
// Реализуется popup-подобными оверлеями (Dropdown, PopupMenu, MenuBar,
// контекстные меню TextBox/TextInput). Движок использует это, чтобы вынести
// оверлей в отдельное нативное окно-попап ОС (см. engine.SetPopupSink) —
// тогда оверлей может выходить за границы родительского окна, как системное
// меню. Оверлеи БЕЗ этого интерфейса рисуются по-старому в основной холст.
type OverlayBoundsProvider interface {
	OverlayBounds() image.Rectangle
}

// DrawContext — API рисования, предоставляемый движком каждому виджету.
// Реализуется типом engine.Canvas.
//
// Все координаты — абсолютные пиксели холста.
type DrawContext interface {
	// ── Примитивы ────────────────────────────────────────────────────────────

	// FillRect заливает прямоугольник (x, y, x+w, y+h) сплошным цветом.
	FillRect(x, y, w, h int, col color.RGBA)

	// FillRectAlpha заливает прямоугольник с альфа-смешиванием (Over).
	FillRectAlpha(x, y, w, h int, col color.RGBA)

	// FillRoundRect заливает прямоугольник со скруглёнными углами радиуса r.
	FillRoundRect(x, y, w, h, r int, col color.RGBA)

	// DrawBorder рисует 1-пиксельный контур прямоугольника.
	DrawBorder(x, y, w, h int, col color.RGBA)

	// DrawRoundBorder рисует 1-пиксельный контур со скруглёнными углами.
	DrawRoundBorder(x, y, w, h, r int, col color.RGBA)

	// SetPixel устанавливает цвет одного пикселя.
	SetPixel(x, y int, col color.RGBA)

	// DrawHLine рисует горизонтальную линию.
	DrawHLine(x, y, length int, col color.RGBA)

	// DrawVLine рисует вертикальную линию.
	DrawVLine(x, y, length int, col color.RGBA)

	// ── Изображения ──────────────────────────────────────────────────────────

	// DrawImage рисует изображение в (x, y) в оригинальном размере.
	DrawImage(src image.Image, x, y int)

	// DrawImageScaled рисует изображение масштабированным до (w × h) в (x, y).
	DrawImageScaled(src image.Image, x, y, w, h int)

	// ── Текст ────────────────────────────────────────────────────────────────

	// DrawText выводит строку TTF-шрифтом «default» размером DefaultFontSizePt.
	DrawText(text string, x, y int, col color.RGBA)

	// DrawTextSize выводит строку шрифтом «default» произвольного размера.
	DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA)

	// DrawTextFont выводит строку именованным шрифтом (RegisterFont).
	// fontName="" → шрифт по умолчанию.
	DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA)

	// MeasureText возвращает ширину строки в пикселях (шрифт default, sizePt).
	MeasureText(text string, sizePt float64) int

	// MeasureTextFont возвращает ширину строки именованным шрифтом.
	MeasureTextFont(text string, sizePt float64, fontName string) int

	// MeasureRunePositions возвращает накопленную ширину после каждого символа.
	// Результат: len(text)+1 элементов; positions[0]==0, positions[n] — ширина text[:n].
	MeasureRunePositions(text string, sizePt float64) []int

	// ── Clip ─────────────────────────────────────────────────────────────────

	// SetClip ограничивает все последующие операции рисования прямоугольником r.
	SetClip(r image.Rectangle)

	// ClearClip снимает ограничение области рисования.
	ClearClip()

	// Clip возвращает текущую область отсечения (или полные границы холста,
	// если отсечение не задано). Позволяет виджету сохранить и восстановить
	// внешнее отсечение при вложенном клиппинге (контейнеры в контейнерах).
	Clip() image.Rectangle
}
