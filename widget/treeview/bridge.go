package treeview

import (
	"image"
	"image/color"
)

// DrawContextBridge — минимальный интерфейс рисования без импорта widget.
// Реализуется адаптером из widget.DrawContext.
type DrawContextBridge interface {
	FillRect(x, y, w, h int, col color.RGBA)
	FillRectAlpha(x, y, w, h int, col color.RGBA)
	DrawBorder(x, y, w, h int, col color.RGBA)
	DrawText(text string, x, y int, col color.RGBA)
	DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA)
	MeasureText(text string, sizePt float64) int
	SetClip(r image.Rectangle)
	ClearClip()
	DrawHLine(x, y, length int, col color.RGBA)
	DrawVLine(x, y, length int, col color.RGBA)
	DrawImageScaled(src image.Image, x, y, w, h int)
	SetPixel(x, y int, col color.RGBA)
}

// ─── TreeViewTheme ─────────────────────────────────────────────────────────

// TreeViewTheme — цветовая тема TreeView.
type TreeViewTheme struct {
	Background      color.RGBA
	Foreground      color.RGBA // цвет текста
	SelectColor     color.RGBA // фон выбранного элемента
	HoverColor      color.RGBA // фон при наведении
	ArrowColor      color.RGBA // цвет стрелок ▸/▾
	FocusBorderColor color.RGBA // цвет рамки фокуса
	ScrollTrackBG   color.RGBA
	ScrollThumbBG   color.RGBA
	ScrollThumbHover color.RGBA
	IndentGuideColor color.RGBA // цвет линий иерархии (опционально)
}

// DefaultDarkTheme возвращает тёмную тему по умолчанию.
func DefaultDarkTheme() TreeViewTheme {
	return TreeViewTheme{
		Background:       color.RGBA{R: 37, G: 37, B: 38, A: 255},
		Foreground:       color.RGBA{R: 204, G: 204, B: 204, A: 255},
		SelectColor:      color.RGBA{R: 0, G: 120, B: 215, A: 80},
		HoverColor:       color.RGBA{R: 62, G: 62, B: 66, A: 255},
		ArrowColor:       color.RGBA{R: 140, G: 140, B: 140, A: 255},
		FocusBorderColor: color.RGBA{R: 0, G: 120, B: 215, A: 255},
		ScrollTrackBG:    color.RGBA{R: 46, G: 46, B: 48, A: 255},
		ScrollThumbBG:    color.RGBA{R: 77, G: 77, B: 80, A: 255},
		ScrollThumbHover: color.RGBA{R: 0, G: 120, B: 215, A: 255},
		IndentGuideColor: color.RGBA{R: 50, G: 50, B: 52, A: 255},
	}
}
