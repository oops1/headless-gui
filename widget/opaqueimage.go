package widget

import (
	"image"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
)

// Признак «изображение непрозрачно» для быстрого пути холста (v3.38) —
// core/draw (opaque.go); здесь — прежние имена.

// OpaqueHinter — изображение, объявляющее себя непрозрачным; см. draw.OpaqueHinter.
type OpaqueHinter = hgdraw.OpaqueHinter

// OpaqueSource — обёртка с пометкой «непрозрачно»; см. draw.OpaqueSource.
type OpaqueSource = hgdraw.OpaqueSource

// OpaqueImage помечает изображение непрозрачным; см. draw.OpaqueImage.
func OpaqueImage(img image.Image) image.Image { return hgdraw.OpaqueImage(img) }

// OpaqueByType — непрозрачно ли изображение по своему типу; см. draw.OpaqueByType.
func OpaqueByType(img image.Image) bool { return hgdraw.OpaqueByType(img) }

// IsOpaqueImage снимает пометку и сообщает, непрозрачно ли изображение; см.
// draw.IsOpaqueImage.
func IsOpaqueImage(img image.Image) (inner image.Image, opaque bool) {
	return hgdraw.IsOpaqueImage(img)
}
