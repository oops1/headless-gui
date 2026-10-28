package raster

// canvas_size.go — допустимый размер холста.

import "log"

// ─── Границы размера холста (SEC-10) ────────────────────────────────────────

const (
	// MinCanvasSide — минимальная сторона холста. Ноль и отрицательные
	// значения давали image.NewRGBA с пустым прямоугольником и мусорную
	// тайловую арифметику в NewCanvas (tilesX/tilesY ≤ 0).
	MinCanvasSide = 1

	// MaxCanvasSide — максимальная сторона холста в логических пикселях.
	// 16384 — верх того, что осмысленно рисовать программно; всё, что выше,
	// это опечатка или подставленное снаружи число.
	MaxCanvasSide = 16384

	// MaxCanvasPixels — максимальная площадь холста (в ФИЗИЧЕСКИХ пикселях,
	// то есть уже с учётом HiDPI-масштаба).
	//
	// Цена пикселя — 4 байта, а буфера два (front+back): 64 Мпикс это
	// 2 × 256 МБ. Выше поднимать нельзя — предел в 256 Мпикс всё ещё давал
	// на New(1<<20, 1<<20) два гигабайтных буфера, то есть ровно то, от чего
	// защищаемся. Ниже — тоже нельзя: 64 Мпикс с запасом покрывает 8K
	// (7680×4320 ≈ 33 Мпикс) и 4K при scale=2 (те же 33 Мпикс).
	//
	// Без этой границы New(1<<20, 1<<20) просил у рантайма терабайты и валил
	// процесс на месте.
	MaxCanvasPixels = 64 << 20
)

// ClampCanvasSize приводит запрошенный размер холста к допустимому диапазону.
// При изменении пишет в журнал: молча отдать не тот размер, что просили, —
// худший из вариантов (приложение будет считать координаты по своему числу).
func ClampCanvasSize(width, height int) (int, int) {
	w, h := width, height
	if w < MinCanvasSide {
		w = MinCanvasSide
	}
	if h < MinCanvasSide {
		h = MinCanvasSide
	}
	if w > MaxCanvasSide {
		w = MaxCanvasSide
	}
	if h > MaxCanvasSide {
		h = MaxCanvasSide
	}
	// Площадь: ужимаем пропорционально, чтобы не ломать соотношение сторон.
	for int64(w)*int64(h) > MaxCanvasPixels {
		if w >= h {
			w /= 2
		} else {
			h /= 2
		}
		if w < MinCanvasSide {
			w = MinCanvasSide
		}
		if h < MinCanvasSide {
			h = MinCanvasSide
		}
	}
	if w != width || h != height {
		log.Printf("engine: размер холста %dx%d вне допустимых границ — приведён к %dx%d "+
			"(сторона %d…%d, площадь ≤ %d пикс.)",
			width, height, w, h, MinCanvasSide, MaxCanvasSide, int64(MaxCanvasPixels))
	}
	return w, h
}
