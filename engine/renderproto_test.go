package engine

// renderproto_test.go — прототип модели рендера Р5–Р8 (задача 4.3 плана 4.0,
// docs/ARCHITECTURE_REVIEW_RU.md §4.6). В релиз не входит: это измерительный
// стенд, по которому владелец принимает решение об этапе 4а.
//
// Что проверяется:
//   - Р5: Draw дерева пишет команды в список (protoRecorder), а не пиксели;
//   - Р6: команды раскладываются по тайлам 64×64 и растеризуются параллельно
//     в пуле воркеров, каждый — в свой холст размером с тайл;
//   - Р7: у каждого тайла — хэш последовательности его команд; совпал с
//     прошлым кадром — тайл не рисуется и не сравнивается;
//   - Р8: у кадра один полный буфер (front): тайл сравнивается с ним прямо
//     из холста воркера и копируется, только если отличается.
//
// Команды, читающие пиксели кадра за пределами своего тайла (BlurBehind,
// Snapshot), требуют барьера: кадр делится на сегменты, сегменты до и после
// такой команды идут по тайлам, сама команда — на полном рабочем буфере.
// Для такого кадра второй буфер нужен, и это одна из цифр отчёта.

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"github.com/oops1/headless-gui/v3/core/geom"
	hgraster "github.com/oops1/headless-gui/v3/core/raster"
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/internal/workpool"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Команды ─────────────────────────────────────────────────────────────────

type protoOp uint8

const (
	opFillRect protoOp = iota
	opFillRectAlpha
	opFillRoundRect
	opDrawBorder
	opDrawRoundBorder
	opSetPixel
	opHLine
	opVLine
	opDrawImage
	opDrawImageScaled
	opDrawImageRect
	opText
	opTextRotated
	opFillEllipseAA
	opStrokeEllipseAA
	opFillPolygonAA
	opStrokePolylineAA
	opDrawLineAA
	opStrokeCubicAA
	opStrokePathAA
	opFillPathAA
	opSoftShadow
	opShadow
	opBlurBehind
	opMica
	opNoise
	opBlitPixels
	opBlitBGRA
	opOpacity
)

var protoOpNames = [...]string{"FillRect", "FillRectAlpha", "FillRoundRect", "DrawBorder", "DrawRoundBorder",
	"SetPixel", "DrawHLine", "DrawVLine", "DrawImage", "DrawImageScaled", "DrawImageRect", "DrawText*",
	"DrawTextRotated", "FillEllipseAA", "StrokeEllipseAA", "FillPolygonAA", "StrokePolylineAA", "DrawLineAA",
	"StrokeCubicAA", "StrokePathAA", "FillPathAA", "DrawSoftShadow", "DrawShadow", "BlurBehind", "MicaBehind",
	"DrawNoise", "BlitPixels", "BlitPixelsBGRA", "DrawWithOpacity"}

// protoCmd — одна команда рисования с её аргументами, действовавшим клипом
// и логическим охватом (для раскладки по тайлам).
type protoCmd struct {
	op             protoOp
	x, y, w, h, r  int
	i1, i2         int
	f1, f2, f3     float64
	f4, f5, f6, f7 float64
	col            color.RGBA
	s, font        string
	img            image.Image
	rgba           *image.RGBA
	rect, rect2    image.Rectangle
	pts            []image.Point
	ptsF           []geom.Point2F
	filter         geom.ImageFilter
	flag           bool
	sub            []protoCmd

	clip        image.Rectangle // действовавший прямоугольный клип (логический)
	round       image.Rectangle // скруглённый клип (нулевой — нет)
	roundR      int
	bbox        image.Rectangle // охват отрисовки с учётом клипа
	readsPixels bool            // читает пиксели кадра за пределами своего участка
	gen         uint64          // поколение картинки (InvalidateImageCache): хэш по адресу его не видит
	hash        uint64
}

const protoFNVOffset, protoFNVPrime = 14695981039346656037, 1099511628211

func protoMix(h uint64, b []byte) uint64 {
	for _, c := range b {
		h ^= uint64(c)
		h *= protoFNVPrime
	}
	return h
}

func protoMixU64(h, v uint64) uint64 {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	return protoMix(h, b[:])
}

// computeHash — хэш команды по всем аргументам; изображения — по адресу.
func (c *protoCmd) computeHash() {
	h := uint64(protoFNVOffset)
	var b [128]byte
	ints := [...]int{int(c.op), c.x, c.y, c.w, c.h, c.r, c.i1, c.i2, int(c.col.R), int(c.col.G), int(c.col.B), int(c.col.A),
		c.clip.Min.X, c.clip.Min.Y, c.clip.Max.X, c.clip.Max.Y, c.round.Min.X, c.round.Min.Y, c.round.Max.X, c.round.Max.Y, c.roundR,
		c.rect.Min.X, c.rect.Min.Y, c.rect.Max.X, c.rect.Max.Y,
		c.rect2.Min.X, c.rect2.Min.Y, c.rect2.Max.X, c.rect2.Max.Y, int(c.filter), int(c.gen)}
	for i, v := range ints {
		binary.LittleEndian.PutUint32(b[i*4:], uint32(v))
	}
	h = protoMix(h, b[:len(ints)*4])
	for _, f := range []float64{c.f1, c.f2, c.f3, c.f4, c.f5, c.f6, c.f7} {
		h = protoMixU64(h, math.Float64bits(f))
	}
	h = protoMix(h, []byte(c.s))
	h = protoMix(h, []byte(c.font))
	if c.img != nil {
		h = protoMixU64(h, uint64(reflect.ValueOf(c.img).Pointer()))
	}
	if c.rgba != nil {
		h = protoMixU64(h, uint64(reflect.ValueOf(c.rgba).Pointer()))
	}
	for _, p := range c.pts {
		h = protoMixU64(h, uint64(p.X)<<32|uint64(uint32(p.Y)))
	}
	for _, p := range c.ptsF {
		h = protoMixU64(h, math.Float64bits(p.X))
		h = protoMixU64(h, math.Float64bits(p.Y))
	}
	for i := range c.sub {
		c.sub[i].computeHash()
		h = protoMixU64(h, c.sub[i].hash)
	}
	if c.flag {
		h = protoMixU64(h, 1)
	}
	c.hash = h
}

// ─── Записывающий контекст (Р5) ──────────────────────────────────────────────

// protoRecorder — DrawContext, который пишет команды вместо пикселей.
// Измерения отдаёт настоящему холсту (шрифты и метрики те же).
type protoRecorder struct {
	ref     *Canvas
	w, h    int
	cmds    []protoCmd
	hasClip bool
	clip    image.Rectangle
	round   image.Rectangle
	roundR  int
	groups  [][]protoCmd // вложенные списки DrawWithOpacity
	imgGen  map[image.Image]uint64
}

var (
	_ widget.DrawContext           = (*protoRecorder)(nil)
	_ hgdraw.AAShapes              = (*protoRecorder)(nil)
	_ hgdraw.PathShapes            = (*protoRecorder)(nil)
	_ hgdraw.ShadowDrawer          = (*protoRecorder)(nil)
	_ hgdraw.ShadowParamDrawer     = (*protoRecorder)(nil)
	_ hgdraw.BackdropDrawer        = (*protoRecorder)(nil)
	_ hgdraw.MicaDrawer            = (*protoRecorder)(nil)
	_ hgdraw.NoiseDrawer           = (*protoRecorder)(nil)
	_ hgdraw.RoundClipper          = (*protoRecorder)(nil)
	_ hgdraw.OpacityDrawer         = (*protoRecorder)(nil)
	_ hgdraw.RotatedTextDrawer     = (*protoRecorder)(nil)
	_ hgdraw.ImageRectDrawer       = (*protoRecorder)(nil)
	_ hgdraw.PixelBlitter          = (*protoRecorder)(nil)
	_ hgdraw.PixelBlitterBGRA      = (*protoRecorder)(nil)
	_ hgdraw.GlyphCoverage         = (*protoRecorder)(nil)
	_ hgdraw.ImageCacheInvalidator = (*protoRecorder)(nil)
	_ hgdraw.CullScope             = (*protoRecorder)(nil)
	_ hgdraw.OcclusionScope        = (*protoRecorder)(nil)
)

func newProtoRecorder(ref *Canvas) *protoRecorder {
	w, h := ref.LogicalSize()
	return &protoRecorder{ref: ref, w: w, h: h}
}

func (p *protoRecorder) reset() {
	p.cmds = p.cmds[:0]
	p.hasClip = false
	p.round = image.Rectangle{}
	p.groups = p.groups[:0]
}

// add записывает команду с текущим клипом; охват bbox пересекается с клипом,
// команда вне клипа отбрасывается.
func (p *protoRecorder) add(c protoCmd, bbox image.Rectangle) {
	c.clip = p.Clip()
	if p.round != (image.Rectangle{}) {
		c.round, c.roundR = p.round, p.roundR
		bbox = bbox.Intersect(p.round)
	}
	bbox = bbox.Intersect(c.clip)
	if bbox.Empty() {
		return
	}
	c.bbox = bbox
	if n := len(p.groups); n > 0 {
		p.groups[n-1] = append(p.groups[n-1], c)
		return
	}
	p.cmds = append(p.cmds, c)
}

func rectXYWH(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

func ptsBounds(pts []image.Point, pad int) image.Rectangle {
	if len(pts) == 0 {
		return image.Rectangle{}
	}
	r := image.Rectangle{Min: pts[0], Max: pts[0].Add(image.Pt(1, 1))}
	for _, q := range pts[1:] {
		r = r.Union(image.Rectangle{Min: q, Max: q.Add(image.Pt(1, 1))})
	}
	return r.Inset(-pad)
}

func ptsFBounds(pts []geom.Point2F, pad int) image.Rectangle {
	if len(pts) == 0 {
		return image.Rectangle{}
	}
	x0, y0, x1, y1 := pts[0].X, pts[0].Y, pts[0].X, pts[0].Y
	for _, q := range pts[1:] {
		x0, y0 = min(x0, q.X), min(y0, q.Y)
		x1, y1 = max(x1, q.X), max(y1, q.Y)
	}
	return image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1))+1, int(math.Ceil(y1))+1).Inset(-pad)
}

func (p *protoRecorder) FillRect(x, y, w, h int, col color.RGBA) {
	p.add(protoCmd{op: opFillRect, x: x, y: y, w: w, h: h, col: col}, rectXYWH(x, y, w, h))
}
func (p *protoRecorder) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	p.add(protoCmd{op: opFillRectAlpha, x: x, y: y, w: w, h: h, col: col}, rectXYWH(x, y, w, h))
}
func (p *protoRecorder) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	p.add(protoCmd{op: opFillRoundRect, x: x, y: y, w: w, h: h, r: r, col: col}, rectXYWH(x, y, w, h))
}
func (p *protoRecorder) DrawBorder(x, y, w, h int, col color.RGBA) {
	p.add(protoCmd{op: opDrawBorder, x: x, y: y, w: w, h: h, col: col}, rectXYWH(x, y, w, h))
}
func (p *protoRecorder) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	p.add(protoCmd{op: opDrawRoundBorder, x: x, y: y, w: w, h: h, r: r, col: col}, rectXYWH(x, y, w, h))
}
func (p *protoRecorder) SetPixel(x, y int, col color.RGBA) {
	p.add(protoCmd{op: opSetPixel, x: x, y: y, col: col}, rectXYWH(x, y, 1, 1))
}
func (p *protoRecorder) DrawHLine(x, y, length int, col color.RGBA) {
	p.add(protoCmd{op: opHLine, x: x, y: y, w: length, col: col}, rectXYWH(x, y, length, 1))
}
func (p *protoRecorder) DrawVLine(x, y, length int, col color.RGBA) {
	p.add(protoCmd{op: opVLine, x: x, y: y, h: length, col: col}, rectXYWH(x, y, 1, length))
}
func (p *protoRecorder) DrawImage(src image.Image, x, y int) {
	b := src.Bounds()
	p.add(protoCmd{op: opDrawImage, x: x, y: y, img: src, gen: p.imgGen[src]}, rectXYWH(x, y, b.Dx(), b.Dy()))
}
func (p *protoRecorder) DrawImageScaled(src image.Image, x, y, w, h int) {
	p.add(protoCmd{op: opDrawImageScaled, x: x, y: y, w: w, h: h, img: src, gen: p.imgGen[src]}, rectXYWH(x, y, w, h))
}
func (p *protoRecorder) DrawImageRect(src image.Image, sr, dr image.Rectangle, f geom.ImageFilter) {
	p.add(protoCmd{op: opDrawImageRect, img: src, rect: sr, rect2: dr, filter: f, gen: p.imgGen[src]}, dr)
}

// textBox — охват строки: шаги пера плюс запас в полкегля с каждой стороны
// (свес масок за шаг, отрицательный OffX курсива) и 4 px по вертикали, как у
// отсечки холста.
func (p *protoRecorder) textBox(text string, x, y int, sizePt float64, font string) image.Rectangle {
	w := p.ref.MeasureTextFont(text, sizePt, font)
	asc, desc, _ := p.ref.FontMetrics(font, sizePt)
	pad := int(sizePt*DefaultDPI/72)/2 + 4
	return image.Rect(x-pad, y-4, x+w+pad, y+asc+desc+4)
}
func (p *protoRecorder) DrawText(text string, x, y int, col color.RGBA) {
	p.DrawTextFont(text, x, y, hgdraw.DefaultFontSizePt, "", col)
}
func (p *protoRecorder) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	p.DrawTextFont(text, x, y, sizePt, "", col)
}
func (p *protoRecorder) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	if text == "" || col.A == 0 {
		return
	}
	p.add(protoCmd{op: opText, x: x, y: y, f1: sizePt, s: text, font: fontName, col: col}, p.textBox(text, x, y, sizePt, fontName))
}
func (p *protoRecorder) DrawTextRotated(text string, x, y int, sizePt float64, fontName string, angle int, col color.RGBA) bool {
	if turns := ((angle % 360) + 360) % 360; turns%90 != 0 {
		return false // как холст: виджет рисует сам
	}
	w := p.ref.MeasureTextFont(text, sizePt, fontName)
	asc, desc, _ := p.ref.FontMetrics(fontName, sizePt)
	s := w + asc + desc + 8
	p.add(protoCmd{op: opTextRotated, x: x, y: y, f1: sizePt, s: text, font: fontName, i1: angle, col: col},
		image.Rect(x-s, y-s, x+s, y+s))
	return true
}
func (p *protoRecorder) MeasureText(text string, sizePt float64) int {
	return p.ref.MeasureText(text, sizePt)
}
func (p *protoRecorder) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return p.ref.MeasureTextFont(text, sizePt, fontName)
}
func (p *protoRecorder) MeasureRunePositions(text string, sizePt float64) []int {
	return p.ref.MeasureRunePositions(text, sizePt)
}
func (p *protoRecorder) SetClip(r image.Rectangle) {
	p.clip = r.Intersect(image.Rect(0, 0, p.w, p.h))
	p.hasClip = true
}
func (p *protoRecorder) ClearClip() { p.hasClip = false }
func (p *protoRecorder) Clip() image.Rectangle {
	if p.hasClip {
		return p.clip
	}
	return image.Rect(0, 0, p.w, p.h)
}
func (p *protoRecorder) SetRoundClip(r image.Rectangle, radius int) {
	p.SetClip(r)
	p.round, p.roundR = r, radius
}
func (p *protoRecorder) ClearRoundClip() { p.round = image.Rectangle{} }
func (p *protoRecorder) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	p.add(protoCmd{op: opFillEllipseAA, x: cx, y: cy, w: rx, h: ry, col: col}, image.Rect(cx-rx-1, cy-ry-1, cx+rx+2, cy+ry+2))
}
func (p *protoRecorder) StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA) {
	t := int(math.Ceil(thickness)) + 1
	p.add(protoCmd{op: opStrokeEllipseAA, x: cx, y: cy, w: rx, h: ry, f1: thickness, col: col},
		image.Rect(cx-rx-t, cy-ry-t, cx+rx+t+1, cy+ry+t+1))
}
func (p *protoRecorder) FillPolygonAA(pts []image.Point, col color.RGBA) {
	p.add(protoCmd{op: opFillPolygonAA, pts: pts, col: col}, ptsBounds(pts, 1))
}
func (p *protoRecorder) StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA) {
	p.add(protoCmd{op: opStrokePolylineAA, pts: pts, f1: thickness, flag: closed, col: col}, ptsBounds(pts, int(math.Ceil(thickness))+2))
}
func (p *protoRecorder) DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA) {
	p.add(protoCmd{op: opDrawLineAA, x: x1, y: y1, i1: x2, i2: y2, f1: thickness, col: col},
		ptsBounds([]image.Point{{x1, y1}, {x2, y2}}, int(math.Ceil(thickness))+2))
}
func (p *protoRecorder) StrokeCubicAA(x0, y0, x1, y1, x2, y2, x3, y3, thickness float64, col color.RGBA) {
	pad := int(math.Ceil(thickness)) + 2
	p.add(protoCmd{op: opStrokeCubicAA, f1: x0, f2: y0, f3: x1, f4: y1, f5: x2, f6: y2, f7: x3, i1: int(math.Floor(y3)), col: col,
		ptsF: []geom.Point2F{{X: x3, Y: y3}, {X: thickness}}},
		ptsFBounds([]geom.Point2F{{X: x0, Y: y0}, {X: x1, Y: y1}, {X: x2, Y: y2}, {X: x3, Y: y3}}, pad))
}
func (p *protoRecorder) StrokePathAA(pts []geom.Point2F, thickness float64, closed bool, col color.RGBA) {
	p.add(protoCmd{op: opStrokePathAA, ptsF: pts, f1: thickness, flag: closed, col: col}, ptsFBounds(pts, int(math.Ceil(thickness))+2))
}
func (p *protoRecorder) FillPathAA(pts []geom.Point2F, col color.RGBA) {
	p.add(protoCmd{op: opFillPathAA, ptsF: pts, col: col}, ptsFBounds(pts, 1))
}
func (p *protoRecorder) DrawSoftShadow(r image.Rectangle, corner int, elevation float64, col color.RGBA) {
	pad := int(math.Ceil(elevation*3)) + 8
	p.add(protoCmd{op: opSoftShadow, rect: r, r: corner, f1: elevation, col: col}, r.Inset(-pad))
}
func (p *protoRecorder) DrawShadow(r image.Rectangle, corner int, blur, offsetX, offsetY float64, col color.RGBA) {
	pad := int(math.Ceil(blur))*2 + 4
	p.add(protoCmd{op: opShadow, rect: r, r: corner, f1: blur, f2: offsetX, f3: offsetY, col: col},
		r.Union(r.Add(image.Pt(int(math.Round(offsetX)), int(math.Round(offsetY))))).Inset(-pad))
}
func (p *protoRecorder) BlurBehind(r image.Rectangle, radius int, tint color.RGBA) {
	p.add(protoCmd{op: opBlurBehind, rect: r, r: radius, col: tint, readsPixels: true}, r)
}
func (p *protoRecorder) MicaBehind(r image.Rectangle, radius int, tint color.RGBA) bool {
	p.add(protoCmd{op: opMica, rect: r, r: radius, col: tint}, r)
	return p.ref.MicaBehind(image.Rectangle{}, radius, tint) // пустая область: только «есть ли обои»
}

// DrawNoise: зерно холста привязано к координатам пикселя холста (noise.go), в
// холсте воркера они локальные — команда идёт на полном буфере, как читающая.
func (p *protoRecorder) DrawNoise(r image.Rectangle, amount float64) {
	p.add(protoCmd{op: opNoise, rect: r, f1: amount, readsPixels: true}, r)
}
func (p *protoRecorder) BlitPixels(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool) {
	p.add(protoCmd{op: opBlitPixels, rgba: src, rect: sr, x: x, y: y, flag: opaque}, rectXYWH(x, y, sr.Dx(), sr.Dy()))
}
func (p *protoRecorder) BlitPixelsBGRA(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool) {
	p.add(protoCmd{op: opBlitBGRA, rgba: src, rect: sr, x: x, y: y, flag: opaque}, rectXYWH(x, y, sr.Dx(), sr.Dy()))
}
func (p *protoRecorder) DrawWithOpacity(r image.Rectangle, alpha float64, draw func()) {
	p.groups = append(p.groups, nil)
	draw()
	sub := p.groups[len(p.groups)-1]
	p.groups = p.groups[:len(p.groups)-1]
	bbox, reads := r, false
	for i := range sub {
		bbox = bbox.Union(sub[i].bbox)
		reads = reads || sub[i].readsPixels
	}
	if len(sub) == 0 {
		return // холст тоже ничего не нарисовал бы
	}
	p.add(protoCmd{op: opOpacity, rect: r, f1: alpha, sub: sub, readsPixels: reads}, bbox)
}
func (p *protoRecorder) HasGlyph(r rune) bool { return p.ref.HasGlyph(r) }

// InvalidateImageCache — картинка переписана на месте: поколение растёт и
// входит в хэш команд с ней (адрес тот же, пиксели другие).
func (p *protoRecorder) InvalidateImageCache(src image.Image) {
	if p.imgGen == nil {
		p.imgGen = make(map[image.Image]uint64)
	}
	p.imgGen[src]++
	p.ref.InvalidateImageCache(src)
}
func (p *protoRecorder) DrawDamage() []image.Rectangle { return nil }
func (p *protoRecorder) SubtreeCullingEnabled() bool   { return false }
func (p *protoRecorder) OcclusionEnabled() bool        { return true }

// ─── Воспроизведение команды на холсте тайла ─────────────────────────────────

// protoReplay выполняет команду на холсте через сдвигающий контекст; клип
// переключается, только когда он отличается от действующего.
type protoRound struct {
	r      image.Rectangle
	radius int
}

func protoReplay(t *translatingContext, cur *image.Rectangle, round *protoRound, c *protoCmd) {
	if c.round != round.r || c.roundR != round.radius {
		if c.round == (image.Rectangle{}) {
			t.ClearRoundClip()
		} else {
			t.SetRoundClip(c.round, c.roundR)
		}
		round.r, round.radius = c.round, c.roundR
		*cur = c.clip
		t.SetClip(c.clip)
	} else if c.clip != *cur {
		t.SetClip(c.clip)
		*cur = c.clip
	}
	switch c.op {
	case opFillRect:
		t.FillRect(c.x, c.y, c.w, c.h, c.col)
	case opFillRectAlpha:
		t.FillRectAlpha(c.x, c.y, c.w, c.h, c.col)
	case opFillRoundRect:
		t.FillRoundRect(c.x, c.y, c.w, c.h, c.r, c.col)
	case opDrawBorder:
		t.DrawBorder(c.x, c.y, c.w, c.h, c.col)
	case opDrawRoundBorder:
		t.DrawRoundBorder(c.x, c.y, c.w, c.h, c.r, c.col)
	case opSetPixel:
		t.SetPixel(c.x, c.y, c.col)
	case opHLine:
		t.DrawHLine(c.x, c.y, c.w, c.col)
	case opVLine:
		t.DrawVLine(c.x, c.y, c.h, c.col)
	case opDrawImage:
		t.DrawImage(c.img, c.x, c.y)
	case opDrawImageScaled:
		t.DrawImageScaled(c.img, c.x, c.y, c.w, c.h)
	case opDrawImageRect:
		t.DrawImageRect(c.img, c.rect, c.rect2, c.filter)
	case opText:
		t.DrawTextFont(c.s, c.x, c.y, c.f1, c.font, c.col)
	case opTextRotated:
		t.DrawTextRotated(c.s, c.x, c.y, c.f1, c.font, c.i1, c.col)
	case opFillEllipseAA:
		t.FillEllipseAA(c.x, c.y, c.w, c.h, c.col)
	case opStrokeEllipseAA:
		t.StrokeEllipseAA(c.x, c.y, c.w, c.h, c.f1, c.col)
	case opFillPolygonAA:
		t.FillPolygonAA(c.pts, c.col)
	case opStrokePolylineAA:
		t.StrokePolylineAA(c.pts, c.f1, c.flag, c.col)
	case opDrawLineAA:
		t.DrawLineAA(c.x, c.y, c.i1, c.i2, c.f1, c.col)
	case opStrokeCubicAA:
		dx, dy := float64(t.dx), float64(t.dy)
		t.inner.StrokeCubicAA(c.f1-dx, c.f2-dy, c.f3-dx, c.f4-dy, c.f5-dx, c.f6-dy, c.f7-dx, c.ptsF[0].Y-dy, c.ptsF[1].X, c.col)
	case opStrokePathAA:
		t.inner.StrokePathAA(protoShiftF(c.ptsF, t.dx, t.dy), c.f1, c.flag, c.col)
	case opFillPathAA:
		t.inner.FillPathAA(protoShiftF(c.ptsF, t.dx, t.dy), c.col)
	case opSoftShadow:
		t.DrawSoftShadow(c.rect, c.r, c.f1, c.col)
	case opShadow:
		t.DrawShadow(c.rect, c.r, c.f1, c.f2, c.f3, c.col)
	case opBlurBehind:
		t.BlurBehind(c.rect, c.r, c.col)
	case opMica:
		t.MicaBehind(c.rect, c.r, c.col)
	case opNoise:
		t.inner.DrawNoise(c.rect.Sub(image.Pt(t.dx, t.dy)), c.f1)
	case opBlitPixels:
		t.BlitPixels(c.rgba, c.rect, c.x, c.y, c.flag)
	case opBlitBGRA:
		t.inner.BlitPixelsBGRA(c.rgba, c.rect, c.x-t.dx, c.y-t.dy, c.flag)
	case opOpacity:
		t.DrawWithOpacity(c.rect, c.f1, func() {
			for i := range c.sub {
				protoReplay(t, cur, round, &c.sub[i])
			}
		})
	}
}

// protoShiftF сдвигает дробные точки на −(dx, dy) (сдвигающий контекст
// движка путей не умеет).
func protoShiftF(pts []geom.Point2F, dx, dy int) []geom.Point2F {
	out := make([]geom.Point2F, len(pts))
	for i, p := range pts {
		out[i] = geom.Point2F{X: p.X - float64(dx), Y: p.Y - float64(dy)}
	}
	return out
}

// ─── Кадр по тайлам (Р6–Р8) ──────────────────────────────────────────────────

// protoWorker — холст размером с тайл и сдвигающий контекст над ним.
type protoWorker struct {
	c   *Canvas
	ctx *translatingContext
}

// protoFrame — состояние прототипа между кадрами: единственный полный буфер
// front, хэши тайлов прошлого кадра, раскладка команд по тайлам.
type protoFrame struct {
	w, h, ts       int
	tilesX, tilesY int
	front          *image.RGBA
	work           *Canvas // полный рабочий холст — только для кадров с читающими командами
	bins           [][]int32
	hash, prevHash []uint64
	changed        []int32 // тайлы, чей хэш не совпал с прошлым кадром
	cmds           []protoCmd
	segStart       int // команды текущего сегмента [segStart, segEnd) (для RunPart)
	segEnd         int
	seg            int // номер сегмента: 0 — тайл начинается с фона, иначе с work
	workers        chan *protoWorker
	batch          workpool.Batch
	segmented      bool // кадр с читающими командами: сегменты через work, без Р7/Р8
	skipped        int
	dirty          int
	rendered       atomic.Int32
	tileDirty      []bool
	parallel       bool

	// Варианты размытия подложки (см. renderproto_blur_test.go).
	mode       protoBlurMode
	blurRect   image.Rectangle // объединение областей читающих команд кадра
	halo       int             // запас ореола вокруг тайла под размытием
	haloMu     sync.Mutex
	haloPool   map[image.Point]chan *protoWorker // холсты ореола по размеру
	haloSrc    *Canvas                           // образец для холстов ореола
	blurCache  map[uint64]*image.RGBA
	blurRegion *Canvas // холст области чтения для промаха кэша
	blurHits   int
	blurMisses int
}

// newProtoFrame строит стенд. ownFonts — у каждого воркера свой вид шрифта по
// умолчанию (личный L1, общий L2 из 4.2): проверка, сколько стоит общий замок
// одного вида между воркерами; годится только сценам без именованных шрифтов.
func newProtoFrame(src *Canvas, parallel, ownFonts bool) *protoFrame {
	w, h := src.LogicalSize()
	ts := output.TileSize
	f := &protoFrame{w: w, h: h, ts: ts, tilesX: (w + ts - 1) / ts, tilesY: (h + ts - 1) / ts, parallel: parallel}
	f.front = image.NewRGBA(image.Rect(0, 0, w, h))
	n := f.tilesX * f.tilesY
	f.bins = make([][]int32, n)
	f.hash = make([]uint64, n)
	f.prevHash = make([]uint64, n)
	f.tileDirty = make([]bool, n)
	nw := runtime.GOMAXPROCS(0) + 1
	f.workers = make(chan *protoWorker, nw)
	for i := 0; i < nw; i++ {
		var c *Canvas
		if ownFonts {
			// Тот же состав, что у холста движка (engine.New): свой вид основного
			// шрифта, свои ленивые запасные и встроенные начертания — путь
			// отрисовки тот же (с запасной цепочкой), замки видов личные.
			c = hgraster.NewCanvasScaled(ts, ts, 1, hgtext.NewFontCacheFromData(goregular.TTF, DefaultDPI))
			c.AddDefaultFonts()
		} else {
			c = src.CloneForSize(ts, ts, 1, nil)
		}
		c.InheritWallpaper(src) // обои и их кэш — у холста движка, как у буфера попапа
		f.workers <- &protoWorker{c: c, ctx: &translatingContext{inner: c}}
	}
	f.haloSrc = src
	return f
}

// bin раскладывает команды кадра по тайлам и считает хэш каждого тайла.
func (f *protoFrame) bin(cmds []protoCmd) {
	f.cmds = cmds
	for i := range f.bins {
		f.bins[i] = f.bins[i][:0]
		f.hash[i] = protoFNVOffset
	}
	ts := f.ts
	for ci := range cmds {
		c := &cmds[ci]
		b := c.bbox
		tx0, ty0 := b.Min.X/ts, b.Min.Y/ts
		tx1, ty1 := (b.Max.X-1)/ts, (b.Max.Y-1)/ts
		if tx0 < 0 {
			tx0 = 0
		}
		if ty0 < 0 {
			ty0 = 0
		}
		if tx1 >= f.tilesX {
			tx1 = f.tilesX - 1
		}
		if ty1 >= f.tilesY {
			ty1 = f.tilesY - 1
		}
		for ty := ty0; ty <= ty1; ty++ {
			for tx := tx0; tx <= tx1; tx++ {
				i := ty*f.tilesX + tx
				f.bins[i] = append(f.bins[i], int32(ci))
				f.hash[i] = protoMixU64(f.hash[i], c.hash)
			}
		}
	}
}

// hasReads — есть ли в кадре команды, читающие пиксели за пределами тайла.
func (f *protoFrame) hasReads() bool {
	for i := range f.cmds {
		if f.cmds[i].readsPixels {
			return true
		}
	}
	return false
}

// render рисует кадр: тайлы, чей хэш совпал с прошлым, пропускаются; без
// читающих команд тайл сравнивается с front прямо из холста воркера (один
// буфер); с ними кадр идёт сегментами через рабочий буфер.
func (f *protoFrame) render(cmds []protoCmd) {
	f.bin(cmds)
	f.skipped, f.dirty = 0, 0
	f.rendered.Store(0)
	for i := range f.tileDirty {
		f.tileDirty[i] = false
	}
	switch {
	case f.mode == blurCache && f.hasReads():
		// Кэш размытия: читающие команды заменяются блитом готового результата,
		// дальше кадр идёт как без чтений (Р7 и Р8 действуют).
		f.bin(f.resolveBlurCache(cmds))
	case f.mode == blurHalo && f.hasReads():
		f.prepareHalo()
	}
	f.segmented = f.hasReads() && f.mode == blurBarrier
	if !f.segmented {
		f.segStart, f.segEnd, f.seg = 0, len(f.cmds), 0
		f.changed = f.changed[:0]
		for i := range f.hash {
			if f.hash[i] != f.prevHash[i] {
				f.changed = append(f.changed, int32(i))
			}
		}
		f.skipped = len(f.hash) - len(f.changed)
		f.runTiles()
	} else {
		f.renderSegmented()
	}
	copy(f.prevHash, f.hash)
}

// runTiles рисует только изменившиеся тайлы (Р7): без изменений пул не
// трогается вовсе.
func (f *protoFrame) runTiles() {
	n := len(f.changed)
	if n == 0 {
		return
	}
	if f.parallel && n > 1 {
		workpool.Run(f, &f.batch, n)
	} else {
		for i := 0; i < n; i++ {
			f.RunPart(i)
		}
	}
	for _, d := range f.tileDirty {
		if d {
			f.dirty++
		}
	}
}

// RunPart — один тайл (workpool.Task): без читающих команд части — это
// изменившиеся тайлы, с ними — все тайлы сегмента.
func (f *protoFrame) RunPart(i int) {
	if !f.segmented {
		i = int(f.changed[i])
	}
	f.rendered.Add(1)
	tx, ty := i%f.tilesX, i/f.tilesX
	ox, oy := tx*f.ts, ty*f.ts
	if f.mode == blurHalo && f.halo > 0 {
		if tile := image.Rect(ox, oy, ox+f.ts, oy+f.ts); tile.Overlaps(f.blurRect) {
			f.runHaloTile(i, tile.Intersect(image.Rect(0, 0, f.w, f.h)))
			return
		}
	}
	w := <-f.workers
	defer func() { f.workers <- w }()
	c, t := w.c, w.ctx
	t.dx, t.dy = ox, oy
	c.ClearRoundClip()
	c.ClearClip()
	tile := image.Rect(ox, oy, ox+f.ts, oy+f.ts).Intersect(image.Rect(0, 0, f.w, f.h))
	back := c.Back()
	if f.segmented && f.seg > 0 {
		protoCopyTile(back, image.Point{}, f.work.Back(), tile.Min, tile.Size())
	}
	// Иначе тайл начинается с первой команды кадра — непрозрачной заливки
	// всего холста (protoRecord), отдельной очистки не нужно.
	cur, round := image.Rect(0, 0, f.w, f.h), protoRound{}
	t.SetClip(cur)
	for _, ci := range f.bins[i] {
		if int(ci) < f.segStart || int(ci) >= f.segEnd {
			continue
		}
		protoReplay(t, &cur, &round, &f.cmds[ci])
	}
	c.ClearRoundClip()
	if f.segmented {
		protoCopyTile(f.work.Back(), tile.Min, back, image.Point{}, tile.Size())
		return
	}
	// Р8: сравнение с единственным буфером и копирование только отличий.
	if !protoTileEqual(back, f.front, tile) {
		protoCopyTile(f.front, tile.Min, back, image.Point{}, tile.Size())
		f.tileDirty[i] = true
	}
}

// renderSegmented — кадр с читающими командами: сегменты между ними идут по
// тайлам через рабочий буфер, сами команды — на полном холсте.
func (f *protoFrame) renderSegmented() {
	if f.work == nil {
		w := <-f.workers
		f.work = w.c.CloneForSize(f.w, f.h, 1, nil)
		f.workers <- w
	}
	start := 0
	f.seg = 0
	for i := 0; i <= len(f.cmds); i++ {
		if i < len(f.cmds) && !f.cmds[i].readsPixels {
			continue
		}
		f.segStart, f.segEnd = start, i
		if i > start {
			f.runTilesSegment()
		}
		if i < len(f.cmds) {
			c := &f.cmds[i]
			full := &translatingContext{inner: f.work}
			cur, round := image.Rect(0, 0, f.w, f.h), protoRound{}
			f.work.ClearRoundClip()
			f.work.SetClip(cur)
			protoReplay(full, &cur, &round, c)
			f.work.ClearRoundClip()
			f.work.ClearClip()
		}
		start = i + 1
		f.seg++
	}
	// Дифф рабочего буфера с front — как сегодня, по тайлам.
	n := f.tilesX * f.tilesY
	for i := 0; i < n; i++ {
		tx, ty := i%f.tilesX, i/f.tilesX
		tile := image.Rect(tx*f.ts, ty*f.ts, (tx+1)*f.ts, (ty+1)*f.ts).Intersect(image.Rect(0, 0, f.w, f.h))
		if !protoTileEqualAt(f.work.Back(), tile, f.front, tile) {
			protoCopyTile(f.front, tile.Min, f.work.Back(), tile.Min, tile.Size())
			f.dirty++
		}
	}
}

func (f *protoFrame) runTilesSegment() {
	n := f.tilesX * f.tilesY
	if f.parallel {
		workpool.Run(f, &f.batch, n)
	} else {
		for i := 0; i < n; i++ {
			f.RunPart(i)
		}
	}
}

func protoCopyTile(dst *image.RGBA, dp image.Point, src *image.RGBA, sp image.Point, size image.Point) {
	for y := 0; y < size.Y; y++ {
		d := dst.PixOffset(dp.X, dp.Y+y)
		s := src.PixOffset(sp.X, sp.Y+y)
		copy(dst.Pix[d:d+size.X*4], src.Pix[s:s+size.X*4])
	}
}

// protoTileEqual — тайл холста воркера (с нуля) против области tile буфера.
func protoTileEqual(tileBuf *image.RGBA, front *image.RGBA, tile image.Rectangle) bool {
	return protoTileEqualAt(tileBuf, image.Rect(0, 0, tile.Dx(), tile.Dy()), front, tile)
}

func protoTileEqualAt(a *image.RGBA, ra image.Rectangle, b *image.RGBA, rb image.Rectangle) bool {
	n := ra.Dx() * 4
	for y := 0; y < ra.Dy(); y++ {
		oa := a.PixOffset(ra.Min.X, ra.Min.Y+y)
		ob := b.PixOffset(rb.Min.X, rb.Min.Y+y)
		if string(a.Pix[oa:oa+n]) != string(b.Pix[ob:ob+n]) {
			return false
		}
	}
	return true
}

// ─── Сцены и запись ───────────────────────────────────────────────────────────

// protoRecord записывает кадр дерева: фон движка + Draw корня.
func protoRecord(rec *protoRecorder, root widget.Widget) []protoCmd {
	rec.reset()
	rec.FillRect(0, 0, rec.w, rec.h, color.RGBA{0, 0, 0, 255})
	root.Draw(rec)
	for i := range rec.cmds {
		rec.cmds[i].computeHash()
	}
	return rec.cmds
}

// protoDirectFrame — эталон: полный кадр движка (как сегодня).
func protoDirectFrame(eng *Engine) *image.RGBA {
	eng.Invalidate()
	eng.renderFrame()
	src := eng.canvas.Front()
	out := image.NewRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	return out
}

func protoMismatch(a, b *image.RGBA) (tiles int, pixels int) {
	ts := output.TileSize
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	for ty := 0; ty*ts < h; ty++ {
		for tx := 0; tx*ts < w; tx++ {
			r := image.Rect(tx*ts, ty*ts, (tx+1)*ts, (ty+1)*ts).Intersect(a.Bounds())
			bad := 0
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					o := a.PixOffset(x, y)
					if string(a.Pix[o:o+4]) != string(b.Pix[o:o+4]) {
						bad++
					}
				}
			}
			if bad > 0 {
				tiles++
				pixels += bad
			}
		}
	}
	return
}

// protoOpStats — сколько команд каждого вида и сколько читают пиксели.
func protoOpStats(cmds []protoCmd) (counts map[string]int, reads int) {
	counts = map[string]int{}
	var walk func(cs []protoCmd)
	walk = func(cs []protoCmd) {
		for i := range cs {
			counts[protoOpNames[cs[i].op]]++
			if cs[i].readsPixels {
				reads++
			}
			walk(cs[i].sub)
		}
	}
	walk(cmds)
	return
}

// ─── Тесты: тождественность кадра ────────────────────────────────────────────

func protoCheckScene(t *testing.T, name string, eng *Engine, root widget.Widget) {
	t.Helper()
	want := protoDirectFrame(eng)
	rec := newProtoRecorder(eng.canvas)
	cmds := protoRecord(rec, root)
	f := newProtoFrame(eng.canvas, true, false)
	f.render(cmds)
	counts, reads := protoOpStats(cmds)
	badTiles, badPx := protoMismatch(f.front, want)
	t.Logf("%s: команд %d (читающих %d), тайлов %d, нарисовано/грязных %d, расхождение: тайлов %d, пикселей %d; виды: %v",
		name, len(cmds), reads, len(f.bins), f.dirty, badTiles, badPx, counts)
	if badPx != 0 {
		t.Errorf("%s: кадр по тайлам отличается от прямого на %d пикселей в %d тайлах", name, badPx, badTiles)
	}
	// Второй кадр без изменений: без читающих команд ни один тайл не
	// рисуется (Р7); с ними сегменты рисуются целиком, а дифф пуст.
	cmds = protoRecord(rec, root)
	f.render(cmds)
	if f.dirty != 0 {
		t.Errorf("%s: кадр без изменений дал %d грязных тайлов", name, f.dirty)
	}
	if reads == 0 && (f.skipped != len(f.hash) || f.rendered.Load() != 0) {
		t.Errorf("%s: кадр без изменений: пропущено %d из %d, нарисовано %d", name, f.skipped, len(f.hash), f.rendered.Load())
	}
}

func TestRenderProto_TextSceneMatchesDirect(t *testing.T) {
	testreset.Globals(t)
	eng, root := benchTextScene(1280, 800)
	defer eng.Stop()
	protoCheckScene(t, "текст 1280×800", eng, root)
}

func TestRenderProto_DesktopSceneMatchesDirect(t *testing.T) {
	testreset.Globals(t)
	sc := buildPipelineDesktop()
	defer sc.closeAll()
	defer sc.eng.Stop()
	protoCheckScene(t, "рабочий стол 1280×800", sc.eng, sc.eng.root)
}

// Изменение одного лейбла перерисовывает только его тайлы, и кадр остаётся
// тождественным прямому.
// Свои виды шрифта у воркеров дают тот же кадр: маски — из общего кэша 4.2.
func TestRenderProto_OwnFontsMatchesDirect(t *testing.T) {
	testreset.Globals(t)
	eng, root := benchTextScene(1280, 800)
	defer eng.Stop()
	want := protoDirectFrame(eng)
	f := newProtoFrame(eng.canvas, true, true)
	f.render(protoRecord(newProtoRecorder(eng.canvas), root))
	if _, bad := protoMismatch(f.front, want); bad != 0 {
		t.Errorf("кадр со своими видами шрифта отличается на %d пикселей", bad)
	}
}

// Кадр с читающей командой, затем кадр без неё тем же стендом: режим
// сегментов не прилипает, front тождественен прямому кадру.
func TestRenderProto_AlternatingModes(t *testing.T) {
	testreset.Globals(t)
	eng, root := benchTextScene(1280, 800)
	defer eng.Stop()
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, false)
	cmds := protoRecord(rec, root)
	withBlur := append(append([]protoCmd(nil), cmds...), protoCmd{op: opBlurBehind, rect: image.Rect(100, 100, 400, 300), r: 12,
		col: color.RGBA{255, 255, 255, 40}, readsPixels: true, clip: image.Rect(0, 0, 1280, 800), bbox: image.Rect(100, 100, 400, 300)})
	withBlur[len(withBlur)-1].computeHash()
	f.render(withBlur)
	if !f.segmented || f.dirty == 0 {
		t.Fatalf("кадр с размытием: segmented=%v, грязных %d", f.segmented, f.dirty)
	}
	f.render(cmds)
	if f.segmented {
		t.Error("режим сегментов прилип к следующему кадру")
	}
	if _, bad := protoMismatch(f.front, protoDirectFrame(eng)); bad != 0 {
		t.Errorf("после кадра с размытием кадр без него отличается на %d пикселей", bad)
	}
}

func TestRenderProto_PartialChange(t *testing.T) {
	testreset.Globals(t)
	eng, root := benchTextScene(1280, 800)
	defer eng.Stop()
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, false)
	f.render(protoRecord(rec, root))
	p := root.(*widget.Panel)
	lbl := p.Children()[7].(*widget.Label)
	lbl.SetText("изменилось")
	f.render(protoRecord(rec, root))
	want := protoDirectFrame(eng)
	if _, bad := protoMismatch(f.front, want); bad != 0 {
		t.Errorf("после изменения лейбла кадр отличается на %d пикселей", bad)
	}
	if f.dirty == 0 || f.dirty > 6 {
		t.Errorf("после изменения одного лейбла грязных тайлов %d", f.dirty)
	}
}

// ─── Бенчмарки ───────────────────────────────────────────────────────────────

// BenchmarkProtoRecord_Text — стоимость записи команд (обход дерева + список)
// против прямой отрисовки того же дерева.
func BenchmarkProtoRecord_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	rec := newProtoRecorder(eng.canvas)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protoRecord(rec, root)
	}
}

func BenchmarkProtoRecord_Desktop(b *testing.B) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	rec := newProtoRecorder(sc.eng.canvas)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protoRecord(rec, sc.eng.root)
	}
}

func benchProtoFull(b *testing.B, eng *Engine, root widget.Widget, parallel, ownFonts bool) {
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, parallel, ownFonts)
	cmds := protoRecord(rec, root)
	f.render(cmds)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range f.prevHash {
			f.prevHash[j] = 0 // полный кадр: все тайлы «изменились»
		}
		f.render(cmds)
	}
	b.ReportMetric(float64(f.dirty), "dirty-tiles")
}

// BenchmarkProtoTiled_Text — полный кадр по тайлам в пуле (все тайлы
// рисуются и сравниваются с front); пара к RenderFrameFull.
func BenchmarkProtoTiled_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	benchProtoFull(b, eng, root, true, false)
}

// BenchmarkProtoTiledOwnFonts_Text — то же, но у каждого воркера свой вид
// шрифта (личный L1): цена общего замка вида между воркерами.
func BenchmarkProtoTiledOwnFonts_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	benchProtoFull(b, eng, root, true, true)
}

func BenchmarkProtoTiledSerialOwnFonts_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	benchProtoFull(b, eng, root, false, true)
}

// BenchmarkProtoTiledSerial_Text — то же одной горутиной: цена самой модели
// без параллелизма.
func BenchmarkProtoTiledSerial_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	benchProtoFull(b, eng, root, false, false)
}

// BenchmarkProtoTiled_Desktop — рабочий стол: есть читающие команды, кадр идёт
// сегментами через рабочий буфер; пара к Pipeline_FullFrame.
func BenchmarkProtoTiled_Desktop(b *testing.B) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	benchProtoFull(b, sc.eng, sc.eng.root, true, false)
}

// BenchmarkProtoRecordTiled_Text — запись + раскладка + растр + сравнение:
// полный кадр целиком по новой модели.
func BenchmarkProtoRecordTiled_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, false)
	f.render(protoRecord(rec, root))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range f.prevHash {
			f.prevHash[j] = 0
		}
		f.render(protoRecord(rec, root))
	}
}

// BenchmarkProtoRecordTiledOwnFonts_Text — честная пара к RenderFrameFull:
// запись команд всего дерева + раскладка + растр по тайлам + сравнение.
func BenchmarkProtoRecordTiledOwnFonts_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, true)
	f.render(protoRecord(rec, root))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range f.prevHash {
			f.prevHash[j] = 0
		}
		f.render(protoRecord(rec, root))
	}
}

// BenchmarkProtoRecordTiled_Desktop — пара к Pipeline_FullFrame и верхняя
// оценка любого кадра рабочего стола с читающей командой (Р7/Р8 там не
// действуют): запись + сегменты через рабочий буфер + полный дифф.
func BenchmarkProtoRecordTiled_Desktop(b *testing.B) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	rec := newProtoRecorder(sc.eng.canvas)
	f := newProtoFrame(sc.eng.canvas, true, false)
	f.render(protoRecord(rec, sc.eng.root))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.render(protoRecord(rec, sc.eng.root))
	}
}

// BenchmarkProtoNoChange_Text — кадр без изменений 1280×800 по готовому
// списку: раскладка и хэши (цена, входящая в частичный кадр).
func BenchmarkProtoNoChange_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, false)
	cmds := protoRecord(rec, root)
	f.render(cmds)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.render(cmds)
	}
}

// BenchmarkProtoNoChange_TextFullHD — кадр без изменений 1920×1080: раскладка
// и хэши по готовому списку команд; пара к DiffFullNoChange (110 мкс).
func BenchmarkProtoNoChange_TextFullHD(b *testing.B) {
	eng, root := benchTextScene(1920, 1080)
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, false)
	cmds := protoRecord(rec, root)
	f.render(cmds)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.render(cmds)
	}
	if f.dirty != 0 || f.rendered.Load() != 0 || f.skipped != len(f.hash) {
		b.Fatalf("кадр без изменений: грязных %d, нарисовано %d, пропущено %d из %d", f.dirty, f.rendered.Load(), f.skipped, len(f.hash))
	}
}

// BenchmarkProtoNoChangeRecord_TextFullHD — то же, но с повторной записью
// команд (без удержания списков у виджетов, Р5 не применён).
func BenchmarkProtoNoChangeRecord_TextFullHD(b *testing.B) {
	eng, root := benchTextScene(1920, 1080)
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, false)
	f.render(protoRecord(rec, root))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.render(protoRecord(rec, root))
	}
}

// BenchmarkProtoPartialRetained_Text — меняется одна команда в удержанном
// списке (Р5: неизменённые виджеты заново не записываются): хэши, растр
// только задетых тайлов, сравнение с front; пара к RenderFramePartial.
func BenchmarkProtoPartialRetained_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, false)
	cmds := protoRecord(rec, root)
	f.render(cmds)
	ci := 0
	for i := range cmds {
		if cmds[i].op == opText {
			ci = i
			break
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	c := &cmds[ci]
	for i := 0; i < b.N; i++ {
		c.s = fmt.Sprintf("кадр %d", i)
		c.bbox = rec.textBox(c.s, c.x, c.y, c.f1, c.font).Intersect(c.clip)
		c.computeHash()
		f.render(cmds)
	}
	b.ReportMetric(float64(f.dirty), "dirty-tiles")
	b.ReportMetric(float64(f.rendered.Load()), "rendered-tiles")
}

// BenchmarkProtoPartial_Text — меняется один лейбл: запись, хэши, растр
// только его тайлов; пара к RenderFramePartial.
func BenchmarkProtoPartial_Text(b *testing.B) {
	eng, root := benchTextScene(1280, 800)
	rec := newProtoRecorder(eng.canvas)
	f := newProtoFrame(eng.canvas, true, false)
	f.render(protoRecord(rec, root))
	lbl := root.(*widget.Panel).Children()[40].(*widget.Label)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lbl.SetText(fmt.Sprintf("кадр %d", i))
		f.render(protoRecord(rec, root))
	}
}
