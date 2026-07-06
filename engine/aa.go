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
// Координаты логические; при HiDPI скейлятся здесь.
func (c *Canvas) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	if rx <= 0 || ry <= 0 {
		return
	}
	k := c.scale
	fcx, fcy := (float64(cx)+0.5)*k, (float64(cy)+0.5)*k
	frx, fry := float64(rx)*k, float64(ry)*k
	bx, by := int(fcx-frx)-1, int(fcy-fry)-1
	bw, bh := int(frx*2)+3, int(fry*2)+3
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		ellipsePath(z, float32(fcx), float32(fcy), float32(frx), float32(fry), ox, oy)
	})
}

// StrokeEllipseAA рисует сглаженный контур эллипса толщиной thickness
// (логические единицы; при HiDPI скейлятся здесь).
func (c *Canvas) StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA) {
	if rx <= 0 || ry <= 0 || thickness <= 0 {
		return
	}
	k := c.scale
	t := float32(thickness * k)
	fcx, fcy := (float64(cx)+0.5)*k, (float64(cy)+0.5)*k
	frx, fry := float64(rx)*k, float64(ry)*k
	pad := int(float64(t)) + 2
	bx, by := int(fcx-frx)-pad, int(fcy-fry)-pad
	bw, bh := int(frx*2)+2*pad+1, int(fry*2)+2*pad+1
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		// Внешний контур по часовой + внутренний против часовой = кольцо.
		ellipsePath(z, float32(fcx), float32(fcy), float32(frx)+t/2, float32(fry)+t/2, ox, oy)
		ellipsePathCCW(z, float32(fcx), float32(fcy), float32(frx)-t/2, float32(fry)-t/2, ox, oy)
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

// FillPolygonAA рисует сглаженный залитый полигон (логические координаты).
func (c *Canvas) FillPolygonAA(pts []image.Point, col color.RGBA) {
	if len(pts) < 3 {
		return
	}
	k := float32(c.scale)
	bx, by, bw, bh := ptsBoundsScaled(pts, 1, c.scale)
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		z.MoveTo((float32(pts[0].X)+0.5)*k-ox, (float32(pts[0].Y)+0.5)*k-oy)
		for _, p := range pts[1:] {
			z.LineTo((float32(p.X)+0.5)*k-ox, (float32(p.Y)+0.5)*k-oy)
		}
	})
}

// StrokePolylineAA рисует сглаженную ломаную толщиной thickness (логические
// единицы); closed=true замыкает последний сегмент с первым.
// Сегменты рисуются прямоугольниками без стыковых сочленений (v1):
// для непрозрачных цветов стыки визуально бесшовны.
func (c *Canvas) StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA) {
	if len(pts) < 2 || thickness <= 0 {
		return
	}
	k := float32(c.scale)
	pad := int(thickness*c.scale) + 2
	bx, by, bw, bh := ptsBoundsScaled(pts, pad, c.scale)
	half := float32(thickness*c.scale) / 2
	c.fillPath(bx, by, bw, bh, col, func(z *vector.Rasterizer, ox, oy float32) {
		n := len(pts)
		last := n - 1
		if closed {
			last = n
		}
		for i := 0; i < last; i++ {
			p1, p2 := pts[i], pts[(i+1)%n]
			ax, ay := (float32(p1.X)+0.5)*k-ox, (float32(p1.Y)+0.5)*k-oy
			bx2, by2 := (float32(p2.X)+0.5)*k-ox, (float32(p2.Y)+0.5)*k-oy
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

// ptsBoundsScaled возвращает bbox точек в ФИЗИЧЕСКИХ координатах с запасом
// pad физических пикселей (клампится дальше в блите).
func ptsBoundsScaled(pts []image.Point, pad int, k float64) (x, y, w, h int) {
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
	pMinX := int(float64(minX) * k)
	pMinY := int(float64(minY) * k)
	pMaxX := int(math.Ceil(float64(maxX+1) * k))
	pMaxY := int(math.Ceil(float64(maxY+1) * k))
	return pMinX - pad, pMinY - pad, pMaxX - pMinX + 2*pad, pMaxY - pMinY + 2*pad
}
