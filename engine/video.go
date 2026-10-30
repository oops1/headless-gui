package engine

// video.go — окна программ с видео в кадре (ADR-0002, 2.3).
//
// Поверхность (widget.ExternalSurface) сама считает, как часто обновляется, и
// по порогам ниже признаёт себя видео. Движок в каждом кадре спрашивает
// такие поверхности (их реестр общий на процесс и пуст у программы без окон
// программ), добавляет к тому, что над ними лежит в дереве, открытые меню и
// модальное окно, переводит в физические пиксели, помечает видимые тайлы
// окна как RegionVideo и отдаёт список в Frame.Videos.

import (
	"image"
	"math"
	"time"

	"github.com/oops1/headless-gui/v4/core/input"
	"github.com/oops1/headless-gui/v4/output"
	"github.com/oops1/headless-gui/v4/widget"
)

// Пороги видео. Переменные, а не константы: потребителю с другим балансом
// полосы и процессора их стоит подвинуть. Задавать до Start; читаются горутиной
// кадра.
var (
	// VideoRate — обновлений в секунду, начиная с которых окно без подсказки
	// программы считается видео. 12 — порог сессии WinLine.
	VideoRate = 12
	// VideoHold — сколько окно должно обновляться реже VideoRate/2, чтобы
	// перестать считаться видео. Гистерезис не даёт кодеку переключаться на
	// паузах между сценами.
	VideoHold = 5 * time.Second
)

// collectVideos собирает видео кадра и помечает его тайлы. nil — видео нет.
func (e *Engine) collectVideos(canvas *Canvas, root widget.Widget, modals []widget.ModalWidget) []output.VideoRegion {
	if root == nil || widget.VideoCandidates() == 0 {
		return nil
	}
	e.videoBuf = widget.CollectVideoSurfaces(root, time.Now(), VideoRate, VideoHold, e.videoBuf[:0])
	if len(e.videoBuf) == 0 {
		return nil
	}
	// Над всеми окнами лежат открытые меню и модальное окно.
	e.overBuf = collectOverlayRects(root, image.Point{}, 0, e.overBuf[:0])
	// Исчезающие оверлеи ещё видны (overlaytrans.go).
	e.overBuf = e.ovtCovers(e.overBuf)
	lw, lh := canvas.LogicalSize()
	for _, m := range modals {
		if !m.IsModal() {
			continue
		}
		if m.DimColor().A > 0 {
			e.overBuf = append(e.overBuf, image.Rect(0, 0, lw, lh))
		} else {
			e.overBuf = append(e.overBuf, widget.SubtreeRect(m))
		}
	}

	k := canvas.Scale()
	canvasRect := image.Rect(0, 0, canvas.W, canvas.H)
	out := make([]output.VideoRegion, 0, len(e.videoBuf))
	for _, v := range e.videoBuf {
		rect := physRectRound(v.Rect, k).Intersect(canvasRect)
		if rect.Empty() {
			continue
		}
		var cover []image.Rectangle
		add := func(r image.Rectangle) {
			if r = physRectOut(r, k).Intersect(rect); !r.Empty() {
				cover = append(cover, r)
			}
		}
		for _, r := range v.Cover {
			add(r)
		}
		for _, r := range e.overBuf {
			add(r)
		}
		canvas.MarkVideo(rect, cover, v.ID)
		out = append(out, output.VideoRegion{ID: v.ID, Rect: rect, Cover: cover})
	}
	clear(e.videoBuf) // не держать покрытия до следующего кадра
	return out
}

// collectOverlayRects — экранные (логические) прямоугольники открытых
// оверлеев дерева: меню, выпадающих списков. off — смещение кадра w, как в
// drawOverlaysAt.
func collectOverlayRects(w widget.Widget, off image.Point, depth int, dst []image.Rectangle) []image.Rectangle {
	if tooDeep(depth) || !widget.IsWidgetVisible(w) {
		return dst
	}
	if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
		if r := input.OverlayHitRect(w).Sub(off); !r.Empty() {
			dst = append(dst, r)
		}
	}
	children := w.Children()
	if len(children) == 0 {
		return dst
	}
	coff := off.Add(input.ContentShift(w))
	for _, c := range children {
		dst = collectOverlayRects(c, coff, depth+1, dst)
	}
	return dst
}

// physRectRound — логический прямоугольник в физических пикселях с тем же
// округлением краёв, что у ExternalSurface.
func physRectRound(r image.Rectangle, k float64) image.Rectangle {
	if k == 1 {
		return r
	}
	return image.Rect(
		int(math.Round(float64(r.Min.X)*k)), int(math.Round(float64(r.Min.Y)*k)),
		int(math.Round(float64(r.Max.X)*k)), int(math.Round(float64(r.Max.Y)*k)),
	)
}

// physRectOut — логический прямоугольник в физических пикселях, наружу:
// покрытие не должно оставить непокрытой полосу на дробном масштабе.
func physRectOut(r image.Rectangle, k float64) image.Rectangle {
	if k == 1 {
		return r
	}
	return image.Rect(
		int(math.Floor(float64(r.Min.X)*k)), int(math.Floor(float64(r.Min.Y)*k)),
		int(math.Ceil(float64(r.Max.X)*k)), int(math.Ceil(float64(r.Max.Y)*k)),
	)
}
