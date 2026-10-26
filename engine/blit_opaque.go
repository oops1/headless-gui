package engine

// blit_opaque.go — быстрый путь DrawImage/DrawImageScaled/DrawImageRect для
// непрозрачного источника 1:1 (v3.38, E15).
//
// Композиция Over читает альфу каждого пикселя и смешивает его с фоном; для
// картинки с альфой 255 результат — сам пиксель, то есть работа впустую.
// Просмотрщик, панорамирующий картинку 3840×2160, платил за это ~22 мс на
// кадр на одном ядре (stdraw.Draw в режиме Over). Здесь для источника,
// объявленного непрозрачным (widget.OpaqueImage или непрозрачный по типу),
// строки копируются как есть (с перестановкой R↔B под FormatBGRX), а большая
// область делится на горизонтальные полосы по ядрам.
//
// Результат совпадает с Over побайтно, пока обещание непрозрачности верно.

import (
	"image"
	stdraw "image/draw"
	"runtime"
)

const (
	// opaqueParallelMin — с какой площади (физических пикселей) копирование
	// делится по ядрам: на мелких областях горутины дороже выигрыша.
	opaqueParallelMin = 256 << 10
	// opaqueBandRows — наименьшая высота полосы.
	opaqueBandRows = 16
)

// opaqueFastType — источник, который blitSrcRect читает прямо из памяти
// (остальные типы идут через stdraw.Draw в режиме Src, но тоже без Over).
func opaqueFastType(src image.Image) bool {
	switch src.(type) {
	case *image.RGBA, *image.NRGBA, *image.YCbCr, *image.Gray, bgraImage:
		return true
	}
	return false
}

// blitSrcRect кладёт src в back композицией Src в физической области r
// (offset — точка src, соответствующая r.Min). Источник обязан быть
// непрозрачным (см. widget.OpaqueImage): иначе прозрачные места затрут фон.
//
// Область полностью перезаписывается, поэтому читать back перед записью не
// нужно — в отличие от blitOverRect, которому в FormatBGRX приходится
// «расколдовывать» фон во временный буфер.
func (c *Canvas) blitSrcRect(r image.Rectangle, src image.Image, offset image.Point) {
	// Защита от выхода за границы: stdraw.Draw подрезал бы сам, а прямое
	// копирование строк — нет.
	if r = r.Intersect(c.back.Bounds()); r.Empty() {
		return
	}
	sr := image.Rectangle{Min: offset, Max: offset.Add(r.Size())}
	if in := sr.Intersect(src.Bounds()); in != sr {
		if in.Empty() {
			return
		}
		r = image.Rectangle{Min: r.Min.Add(in.Min.Sub(sr.Min)), Max: r.Min.Add(in.Max.Sub(sr.Min))}
		offset, sr = in.Min, in
	}
	h := r.Dy()
	bands := 1
	if r.Dx()*h >= opaqueParallelMin {
		bands = min(runtime.GOMAXPROCS(0), h/opaqueBandRows)
	}
	if bands <= 1 {
		c.blitSrcBand(r, src, offset)
		return
	}
	// Полосы не пересекаются по строкам back — гонок нет. Раздаёт их
	// постоянный пул (workpool.go); первую делает текущая горутина.
	step := (h + bands - 1) / bands
	j := &c.blitWork
	j.c, j.r, j.src, j.offset, j.step = c, r, src, offset, step
	poolRun(j, &j.batch, (h+step-1)/step)
	j.src = nil // не держать картинку приложения до следующего блита
}

// blitSrcBand — одна полоса blitSrcRect, последовательно.
func (c *Canvas) blitSrcBand(r image.Rectangle, src image.Image, offset image.Point) {
	w, h := r.Dx(), r.Dy()
	bgrx := c.format == FormatBGRX
	var pix []byte
	var stride, pixOff0 int
	switch s := src.(type) {
	case *image.RGBA:
		pix, stride, pixOff0 = s.Pix, s.Stride, s.PixOffset(offset.X, offset.Y)
	case bgraImage:
		// Байты уже в порядке холста BGRX; на холсте RGBA их надо
		// переставить, как RGBA на холсте BGRX.
		pix, stride, pixOff0 = s.Pix, s.Stride, s.PixOffset(offset.X, offset.Y)
		bgrx = !bgrx
	case *image.NRGBA:
		// Непрозрачный NRGBA совпадает с premultiplied побайтно.
		pix, stride, pixOff0 = s.Pix, s.Stride, s.PixOffset(offset.X, offset.Y)
	}
	if pix != nil {
		for y := 0; y < h; y++ {
			sp := pixOff0 + y*stride
			dp := c.back.PixOffset(r.Min.X, r.Min.Y+y)
			drow := c.back.Pix[dp : dp+w*4]
			srow := pix[sp : sp+w*4]
			if !bgrx {
				copy(drow, srow)
				continue
			}
			for i := 0; i < len(drow); i += 4 {
				drow[i+0], drow[i+1], drow[i+2], drow[i+3] = srow[i+2], srow[i+1], srow[i+0], srow[i+3]
			}
		}
		return
	}
	// YCbCr, Gray и прочее: Src средствами image/draw.
	if !bgrx {
		stdraw.Draw(c.back, r, src, offset, stdraw.Src)
		return
	}
	// FormatBGRX: построчно во временную RGBA-строку и с перестановкой в back.
	tmp := image.NewRGBA(image.Rect(0, 0, w, 1))
	for y := 0; y < h; y++ {
		stdraw.Draw(tmp, tmp.Rect, src, offset.Add(image.Pt(0, y)), stdraw.Src)
		dp := c.back.PixOffset(r.Min.X, r.Min.Y+y)
		drow := c.back.Pix[dp : dp+w*4]
		for i := 0; i < len(drow); i += 4 {
			drow[i+0], drow[i+1], drow[i+2], drow[i+3] = tmp.Pix[i+2], tmp.Pix[i+1], tmp.Pix[i+0], tmp.Pix[i+3]
		}
	}
}
