package engine

// raster_alias.go — прежние имена пакета engine, перенесённые в core/raster (задача 4.1
// плана 4.0): код потребителей не меняется.

import (
	hgraster "github.com/oops1/headless-gui/v3/core/raster"
	"image"
)

// BlurRGBA — см. raster.BlurRGBA.
func BlurRGBA(img *image.RGBA, radius int, passes int) { hgraster.BlurRGBA(img, radius, passes) }

// BlurRegion — см. raster.BlurRegion.
func BlurRegion(img *image.RGBA, r image.Rectangle, radius int, passes int) {
	hgraster.BlurRegion(img, r, radius, passes)
}

// Canvas — см. raster.Canvas.
type Canvas = hgraster.Canvas

// FormatBGRX — см. raster.FormatBGRX.
const FormatBGRX = hgraster.FormatBGRX

// FormatRGBA — см. raster.FormatRGBA.
const FormatRGBA = hgraster.FormatRGBA

// MaxCanvasPixels — см. raster.MaxCanvasPixels.
const MaxCanvasPixels = hgraster.MaxCanvasPixels

// MaxCanvasSide — см. raster.MaxCanvasSide.
const MaxCanvasSide = hgraster.MaxCanvasSide

// MinCanvasSide — см. raster.MinCanvasSide.
const MinCanvasSide = hgraster.MinCanvasSide

// OpaqueBands — см. raster.OpaqueBands.
func OpaqueBands(img *image.RGBA) []image.Rectangle { return hgraster.OpaqueBands(img) }

// PixelFormat — см. raster.PixelFormat.
type PixelFormat = hgraster.PixelFormat

// SolidBands — см. raster.SolidBands.
func SolidBands(img *image.RGBA) []image.Rectangle { return hgraster.SolidBands(img) }
