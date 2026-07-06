// aa.go — сглаженные (antialiased) примитивы канваса.
//
// Два механизма:
//
//  1. Скруглённые углы (FillRoundRect/DrawRoundBorder в canvas.go):
//     прямоугольное тело заливается быстрым FillRect (семантика Src, как
//     раньше), а сами углы блиттируются кэшированными альфа-масками
//     четверть-диска / четверть-кольца. Кэш по радиусу — маски строятся
//     один раз через golang.org/x/image/vector.
//
//  2. Произвольные фигуры (widget.AAShapes): эллипсы, линии, полилинии,
//     полигоны — путь строится и растеризуется по месту (без кэша:
//     геометрия произвольна, фигур в кадре немного).
//
// Полупрозрачные цвета (A<255) в скруглениях идут по старому не-AA пути:
// тело рисуется Src-заливкой, и смешение углов Over дало бы видимый шов.
package engine

import (
	"image"
	"image/color"
	"math"
	"sync"

	"golang.org/x/image/vector"
)

// kappa — коэффициент кубической аппроксимации четверти окружности.
const kappa = 0.55228475

// ─── Кэш угловых масок ───────────────────────────────────────────────────────

// cornerKind — вид угловой маски.
type cornerKind uint8

const (
	cornerFill cornerKind = iota // четверть-диск (заливка угла)
	cornerRing                   // четверть-кольцо толщиной 1px (контур угла)
)

type cornerMaskKey struct {
	r    int
	kind cornerKind
}

// cornerSet — четыре ориентации угловой маски: TL, TR, BL, BR.
type cornerSet [4]*image.Alpha

var (
	cornerMu    sync.Mutex
	cornerCache map[cornerMaskKey]*cornerSet
)

// cornersFor возвращает набор угловых масок радиуса r (кэшируется).
func cornersFor(r int, kind cornerKind) *cornerSet {
	key := cornerMaskKey{r: r, kind: kind}
	cornerMu.Lock()
	defer cornerMu.Unlock()
	if s, ok := cornerCache[key]; ok {
		return s
	}
	tl := buildCornerTL(r, kind)
	set := &cornerSet{
		tl,
		mirrorAlpha(tl, true, false),  // TR
		mirrorAlpha(tl, false, true),  // BL
		mirrorAlpha(tl, true, true),   // BR
	}
	if cornerCache == nil {
		cornerCache = make(map[cornerMaskKey]*cornerSet)
	}
	cornerCache[key] = set
	return set
}

// buildCornerTL растеризует верхне-левую угловую маску r×r.
// Координаты: (0,0) — внешний угол прямоугольника, центр дуги — (r,r).
func buildCornerTL(r int, kind cornerKind) *image.Alpha {
	fr := float32(r)
	k := float32(kappa) * fr
	z := vector.NewRasterizer(r, r)

	// Внешняя дуга: (r,0) → (0,r), выпуклостью к (0,0).
	z.MoveTo(fr, 0)
	z.CubeTo(fr-k, 0, 0, fr-k, 0, fr)

	if kind == cornerFill {
		// Клин до центра дуги.
		z.LineTo(fr, fr)
	} else {
		// Кольцо: назад по внутренней дуге радиуса r-1 (против часовой).
		ir := fr - 1
		ik := float32(kappa) * ir
		z.LineTo(fr-ir, fr)                                  // (1, r) — левый конец внутренней дуги
		z.CubeTo(fr-ir, fr-ik, fr-ik, fr-ir, fr, fr-ir)      // → (r, 1)
	}
	z.ClosePath()

	m := image.NewAlpha(image.Rect(0, 0, r, r))
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return m
}

// mirrorAlpha возвращает копию маски, отражённую по X и/или Y.
func mirrorAlpha(src *image.Alpha, flipX, flipY bool) *image.Alpha {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewAlpha(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := y
		if flipY {
			sy = h - 1 - y
		}
		for x := 0; x < w; x++ {
			sx := x
			if flipX {
				sx = w - 1 - x
			}
			dst.Pix[y*dst.Stride+x] = src.Pix[sy*src.Stride+sx]
		}
	}
	return dst
}

// drawCorners блиттирует четыре угловые маски радиуса r по углам (x,y,w,h).
func (c *Canvas) drawCorners(set *cornerSet, x, y, w, h, r int, col color.RGBA) {
	c.drawAlphaMask(set[0], x, y, col)         // TL
	c.drawAlphaMask(set[1], x+w-r, y, col)     // TR
	c.drawAlphaMask(set[2], x, y+h-r, col)     // BL
	c.drawAlphaMask(set[3], x+w-r, y+h-r, col) // BR
}

// ─── AA-фигуры произвольной геометрии (widget.AAShapes) ─────────────────────

// fillPath растеризует путь (замкнутый набор контуров, координаты холста,
// заданные функцией build относительно (bx, by)) и блиттирует цветом col.
func (c *Canvas) fillPath(bx, by, bw, bh int, col color.RGBA, build func(z *vector.Rasterizer, offX, offY float32)) {
	if bw <= 0 || bh <= 0 || bw > 8192 || bh > 8192 || col.A == 0 {
		return
	}
	z := vector.NewRasterizer(bw, bh)
	build(z, float32(bx), float32(by))
	z.ClosePath()
	m := image.NewAlpha(image.Rect(0, 0, bw, bh))
	z.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	c.drawAlphaMask(m, bx, by, col)
}

// ellipsePath добавляет в путь эллипс (cx,cy,rx,ry) кубиками (по часовой).
func ellipsePath(z *vector.Rasterizer, cx, cy, rx, ry, offX, offY float32) {
	kx, ky := float32(kappa)*rx, float32(kappa)*ry
	x0, y0 := cx-offX, cy-offY
	z.MoveTo(x0, y0-ry)
	z.CubeTo(x0+kx, y0-ry, x0+rx, y0-ky, x0+rx, y0)
	z.CubeTo(x0+rx, y0+ky, x0+kx, y0+ry, x0, y0+ry)
	z.CubeTo(x0-kx, y0+ry, x0-rx, y0+ky, x0-rx, y0)
	z.CubeTo(x0-rx, y0-ky, x0-kx, y0-ry, x0, y0-ry)
}

// FillEllipseAA рисует сглаженный залитый эллипс с центром (cx, cy).
func (c *Canvas) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	if rx <= 0 || ry <= 0 {
		return
	}
	bx, by := cx-rx-1, cy-ry-1
	bw, bh := rx*2+3, ry*2+3
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		ellipsePath(z, float32(cx)+0.5, float32(cy)+0.5, float32(rx), float32(ry), ox, oy)
	})
}

// StrokeEllipseAA рисует сглаженный контур эллипса толщиной thickness.
func (c *Canvas) StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA) {
	if rx <= 0 || ry <= 0 || thickness <= 0 {
		return
	}
	t := float32(thickness)
	bx, by := cx-rx-1-int(thickness), cy-ry-1-int(thickness)
	bw, bh := (rx+1+int(thickness))*2+1, (ry+1+int(thickness))*2+1
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		fcx, fcy := float32(cx)+0.5, float32(cy)+0.5
		// Внешний контур по часовой + внутренний против часовой = кольцо.
		ellipsePath(z, fcx, fcy, float32(rx)+t/2, float32(ry)+t/2, ox, oy)
		ellipsePathCCW(z, fcx, fcy, float32(rx)-t/2, float32(ry)-t/2, ox, oy)
	})
}

// ellipsePathCCW — эллипс против часовой (для вырезания внутренности).
func ellipsePathCCW(z *vector.Rasterizer, cx, cy, rx, ry, offX, offY float32) {
	if rx <= 0 || ry <= 0 {
		return
	}
	kx, ky := float32(kappa)*rx, float32(kappa)*ry
	x0, y0 := cx-offX, cy-offY
	z.MoveTo(x0, y0-ry)
	z.CubeTo(x0-kx, y0-ry, x0-rx, y0-ky, x0-rx, y0)
	z.CubeTo(x0-rx, y0+ky, x0-kx, y0+ry, x0, y0+ry)
	z.CubeTo(x0+kx, y0+ry, x0+rx, y0+ky, x0+rx, y0)
	z.CubeTo(x0+rx, y0-ky, x0+kx, y0-ry, x0, y0-ry)
}

// DrawLineAA рисует сглаженную линию толщиной thickness.
func (c *Canvas) DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA) {
	c.StrokePolylineAA([]image.Point{{X: x1, Y: y1}, {X: x2, Y: y2}}, thickness, false, col)
}

// FillPolygonAA рисует сглаженный залитый полигон.
func (c *Canvas) FillPolygonAA(pts []image.Point, col color.RGBA) {
	if len(pts) < 3 {
		return
	}
	bx, by, bw, bh := ptsBounds(pts, 1)
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		z.MoveTo(float32(pts[0].X)+0.5-ox, float32(pts[0].Y)+0.5-oy)
		for _, p := range pts[1:] {
			z.LineTo(float32(p.X)+0.5-ox, float32(p.Y)+0.5-oy)
		}
	})
}

// StrokePolylineAA рисует сглаженную ломаную толщиной thickness;
// closed=true замыкает последний сегмент с первым.
// Сегменты рисуются прямоугольниками без стыковых сочленений (v1):
// для непрозрачных цветов стыки визуально бесшовны.
func (c *Canvas) StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA) {
	if len(pts) < 2 || thickness <= 0 {
		return
	}
	pad := int(thickness) + 2
	bx, by, bw, bh := ptsBounds(pts, pad)
	half := float32(thickness) / 2
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		n := len(pts)
		last := n - 1
		if closed {
			last = n
		}
		for i := 0; i < last; i++ {
			p1, p2 := pts[i], pts[(i+1)%n]
			ax, ay := float32(p1.X)+0.5-ox, float32(p1.Y)+0.5-oy
			bx2, by2 := float32(p2.X)+0.5-ox, float32(p2.Y)+0.5-oy
			dx, dy := bx2-ax, by2-ay
			l := float32(math.Hypot(float64(dx), float64(dy)))
			if l == 0 {
				continue
			}
			// Перпендикуляр половинной толщины.
			px, py := -dy/l*half, dx/l*half
			z.MoveTo(ax+px, ay+py)
			z.LineTo(bx2+px, by2+py)
			z.LineTo(bx2-px, by2-py)
			z.LineTo(ax-px, ay-py)
			z.ClosePath()
		}
	})
}

// ptsBounds возвращает bbox точек с запасом pad (клампится дальше в блите).
func ptsBounds(pts []image.Point, pad int) (x, y, w, h int) {
	minX, minY := pts[0].X, pts[0].Y
	maxX, maxY := minX, minY
	for _, p := range pts[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return minX - pad, minY - pad, maxX - minX + 2*pad + 1, maxY - minY + 2*pad + 1
}
