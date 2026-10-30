package raster

// canvas_jobs.go — задания холста для пула воркеров (internal/workpool): дифф
// тайлов и непрозрачный блит по полосам.

import (
	"image"

	"github.com/oops1/headless-gui/v3/internal/workpool"
	"github.com/oops1/headless-gui/v3/output"
)

// diffJob — дифф тайлов по полосам рядов [ty0..ty1]: часть i — ряды
// [ty0+i*chunk .. ty0+(i+1)*chunk-1]. Полосы не пересекаются по памяти —
// сравнение, извлечение и синхронизация front каждая делает в своих рядах.
type diffJob struct {
	c                  *Canvas
	tx0, tx1, ty0, ty1 int
	chunk              int
	results            [][]output.DirtyTile
	batch              workpool.Batch
}

func (j *diffJob) RunPart(i int) {
	r0 := j.ty0 + i*j.chunk
	r1 := min(r0+j.chunk-1, j.ty1)
	if r0 > r1 {
		j.results[i] = nil
		return
	}
	j.results[i] = j.c.diffTileRows(j.tx0, r0, j.tx1, r1)
}

// blitJob — непрозрачный блит по горизонтальным полосам: часть i — строки
// [i*step .. (i+1)*step) прямоугольника r. Полосы не пересекаются по строкам
// back.
type blitJob struct {
	c      *Canvas
	r      image.Rectangle
	src    image.Image
	offset image.Point
	step   int
	batch  workpool.Batch
}

func (j *blitJob) RunPart(i int) {
	h := j.r.Dy()
	y := i * j.step
	if y >= h {
		return
	}
	y1 := min(y+j.step, h)
	band := image.Rect(j.r.Min.X, j.r.Min.Y+y, j.r.Max.X, j.r.Min.Y+y1)
	j.c.blitSrcBand(band, j.src, j.offset.Add(image.Pt(0, y)))
}
