// shadow.go — мягкая тень под прямоугольником (в том числе со скруглёнными
// углами), на замену плоской тени со сдвигом в пару пикселей
// (см. widget/popupmenu.go).
//
// Тень строится в три шага:
//  1. Силуэт (скруглённый прямоугольник цветом тени) рисуется во временный
//     буфер *image.RGBA с запасом по краям — без запаса размытие обрезалось
//     бы о границу буфера (BlurRegion зажимает координаты к границам
//     размываемой области, см. blur.go).
//  2. Буфер размывается BlurRGBA в 2 прохода — этого достаточно для гладкого
//     края (см. комментарий в blur.go про сходимость box-размытия к гауссову).
//  3. Буфер композитится в back операцией Over со сдвигом вниз через уже
//     существующий приватный метод drawColorGlyph (canvas.go) — он уважает
//     и прямоугольное (clip/hasClip), и скруглённое (round) отсечение
//     канваса, поэтому отдельный цикл композиции здесь не нужен.
package engine

import (
	"image"
	"image/color"
	"math"
)

// DrawSoftShadow рисует мягкую тень под прямоугольником r со скруглением
// corner. elevation — высота над подложкой в логических пикселях: от неё
// зависят радиус размытия (≈ elevation) и смещение вниз (≈ elevation/2).
// col — цвет тени (alpha-premultiplied); прозрачный цвет или elevation<=0 —
// ничего не рисует.
//
// Координаты r и corner — ЛОГИЧЕСКИЕ, как во всей публичной отрисовке
// канваса; масштабирование в физические пиксели (HiDPI) происходит внутри.
//
// Тень рисуется целым силуэтом, без вычитания площади самого прямоугольника
// (проще и дешевле, чем вырезать «бублик»): в реальном использовании сверху
// в любом случае ложится непрозрачный фон/содержимое прямоугольника, которое
// перекрывает эту часть тени.
//
// Возможное улучшение: временный буфер аллоцируется на каждый вызов; его
// можно было бы закэшировать в Canvas по (w,h,corner,elevation), но это
// потребовало бы нового поля в структуре Canvas (canvas.go), которая в
// рамках этой задачи не редактируется.
func (c *Canvas) DrawSoftShadow(r image.Rectangle, corner int, elevation float64, col color.RGBA) {
	if elevation <= 0 || col.A == 0 || r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}

	// Радиус размытия и смещение вниз — из elevation, в ЛОГИЧЕСКИХ пикселях.
	blurRadius := int(math.Round(elevation))
	if blurRadius < 1 {
		blurRadius = 1
	}
	offsetY := int(math.Round(elevation / 2))

	// Переход в физические координаты (единожды, как и в остальном Canvas).
	pr := c.sRect(r)
	pw, ph := pr.Dx(), pr.Dy()
	if pw <= 0 || ph <= 0 {
		return
	}
	pCorner := c.st(corner)
	if pCorner > pw/2 {
		pCorner = pw / 2
	}
	if pCorner > ph/2 {
		pCorner = ph / 2
	}
	pRadius := c.st(blurRadius)
	pOffset := c.sx(offsetY)

	// Запас по краям буфера: за 2 прохода box-размытия эффективный разброс
	// растёт быстрее одного радиуса — запас 2×radius не даёт границе буфера
	// обрезать затухание тени (что дало бы жёсткий край вместо мягкого).
	margin := pRadius * 2
	if margin < 1 {
		margin = 1
	}

	tw := pw + margin*2
	th := ph + margin*2
	tmp := image.NewRGBA(image.Rect(0, 0, tw, th))

	fillSilhouette(tmp, margin, margin, pw, ph, pCorner, col)
	BlurRGBA(tmp, pRadius, 2)

	// Левый верхний угол размытого буфера на холсте: силуэт нарисован в
	// буфере со сдвигом (margin, margin), плюс вертикальный сдвиг тени вниз.
	gx := pr.Min.X - margin
	gy := pr.Min.Y - margin + pOffset

	// drawColorGlyph — существующий приватный композитор Over с учётом
	// clip/hasClip и скруглённого round-отсечения (см. canvas.go); он не
	// красит источник цветом (он уже цветной), что здесь и нужно.
	c.drawColorGlyph(tmp, gx, gy)
}

// fillSilhouette рисует непрозрачный силуэт скруглённого прямоугольника
// цветом col в буфер img со смещением (ox, oy) от его начала координат.
// Без сглаживания краёв — силуэт всё равно размывается BlurRGBA сразу после,
// поэтому AA здесь избыточна. Формула отступа по строке — та же, что в
// fillRoundRectLegacy (canvas.go) и SetRoundClip (clipround.go): совпадение
// с остальной отрисовкой скруглённых углов важно только визуально (тень —
// отдельный, размытый силуэт), а не побитово.
func fillSilhouette(img *image.RGBA, ox, oy, w, h, r int, col color.RGBA) {
	if r <= 0 {
		fillRectImg(img, image.Rect(ox, oy, ox+w, oy+h), col)
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	fillRectImg(img, image.Rect(ox, oy+r, ox+w, oy+h-r), col)
	rf := float64(r)
	for i := 0; i < r; i++ {
		dy := float64(r - i - 1)
		inset := r - int(math.Round(math.Sqrt(rf*rf-dy*dy)))
		lineW := w - 2*inset
		if lineW > 0 {
			fillRectImg(img, image.Rect(ox+inset, oy+i, ox+inset+lineW, oy+i+1), col)     // верх
			fillRectImg(img, image.Rect(ox+inset, oy+h-1-i, ox+inset+lineW, oy+h-i), col) // низ
		}
	}
}

// fillRectImg — простая Src-заливка прямоугольника в произвольном
// *image.RGBA (не в back-буфере канваса — временный буфер тени ещё не
// имеет клипа и масштаба, поэтому Canvas.fillRectPx здесь не подходит).
func fillRectImg(img *image.RGBA, r image.Rectangle, col color.RGBA) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		off := img.PixOffset(r.Min.X, y)
		row := img.Pix[off : off+r.Dx()*4]
		for x := 0; x < len(row); x += 4 {
			row[x+0] = col.R
			row[x+1] = col.G
			row[x+2] = col.B
			row[x+3] = col.A
		}
	}
}
