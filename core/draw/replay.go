package draw

// replay.go — воспроизведение дерева списков команд в любой DrawContext
// (ADR-0004). Список или команда, чей охват не задевает повреждение,
// пропускается; клип ставится только при смене; по выходу холст приводится к
// клипу, который был у рекордера после Draw корня.

import (
	"image"
)

// ReplayOpts — параметры воспроизведения.
type ReplayOpts struct {
	// Damage — повреждение кадра (логическое); пустой список — рисовать всё.
	Damage []image.Rectangle
	// Offset оборачивает контекст сдвигом для прямых команд (OpDirect),
	// записанных под OffsetContext; nil — такие команды рисуются без сдвига.
	Offset func(ctx DrawContext, off image.Point) DrawContext
	// Direct рисует прямой виджет (OpDirect): движок передаёт widget.DrawTraced,
	// чтобы работали учёт рисующихся, политика сбоев и заглушка сломанного;
	// nil — Holder.Draw(ctx).
	Direct func(h ListHolder, ctx DrawContext)
	// Exit — клип, к которому привести контекст по окончании.
	Exit ClipState
}

// Replayer воспроизводит списки; хранит разобранные возможности контекста
// и переиспользуется между кадрами (движок держит один).
type Replayer struct {
	ctx      DrawContext
	o        ReplayOpts
	aa       AAShapes
	paths    PathShapes
	shadow   ShadowDrawer
	shadowP  ShadowParamDrawer
	backdrop BackdropDrawer
	mica     MicaDrawer
	noise    NoiseDrawer
	round    RoundClipper
	opacity  OpacityDrawer
	rotated  RotatedTextDrawer
	imgRect  ImageRectDrawer
	blit     PixelBlitter
	blitBGRA PixelBlitterBGRA
	known    bool
	cur      ClipState
	curPtr   *ClipState
}

// Replay воспроизводит дерево списков root в ctx.
func Replay(ctx DrawContext, root *List, o ReplayOpts) { new(Replayer).Replay(ctx, root, o) }

// Replay воспроизводит дерево списков root в ctx.
func (p *Replayer) Replay(ctx DrawContext, root *List, o ReplayOpts) {
	if root == nil {
		return
	}
	*p = Replayer{ctx: ctx, o: o}
	p.aa, _ = ctx.(AAShapes)
	p.paths, _ = ctx.(PathShapes)
	p.shadow, _ = ctx.(ShadowDrawer)
	p.shadowP, _ = ctx.(ShadowParamDrawer)
	p.backdrop, _ = ctx.(BackdropDrawer)
	p.mica, _ = ctx.(MicaDrawer)
	p.noise, _ = ctx.(NoiseDrawer)
	p.round, _ = ctx.(RoundClipper)
	p.opacity, _ = ctx.(OpacityDrawer)
	p.rotated, _ = ctx.(RotatedTextDrawer)
	p.imgRect, _ = ctx.(ImageRectDrawer)
	p.blit, _ = ctx.(PixelBlitter)
	p.blitBGRA, _ = ctx.(PixelBlitterBGRA)
	p.list(root)
	p.applyClip(o.Exit, nil)
	*p = Replayer{} // не держать контекст и списки между кадрами
}

func (p *Replayer) hits(b image.Rectangle) bool {
	if len(p.o.Damage) == 0 {
		return true
	}
	for _, d := range p.o.Damage {
		if b.Overlaps(d) {
			return true
		}
	}
	return false
}

// applyClip приводит клип контекста к s, если он отличается. ptr — то же
// состояние по указателю (команды под одним клипом делят его): совпадение
// указателей избавляет от сравнения.
func (p *Replayer) applyClip(s ClipState, ptr *ClipState) {
	if p.known && ((ptr != nil && ptr == p.curPtr) || s == p.cur) {
		p.curPtr = ptr
		return
	}
	if p.round != nil {
		if s.Round == (image.Rectangle{}) {
			if !p.known || p.cur.Round != (image.Rectangle{}) {
				p.round.ClearRoundClip()
			}
		} else if !p.known || s.Round != p.cur.Round || s.RoundR != p.cur.RoundR {
			p.round.SetRoundClip(s.Round, s.RoundR)
		}
	}
	if s.NoClip {
		p.ctx.ClearClip()
	} else {
		p.ctx.SetClip(s.Clip)
	}
	p.cur, p.curPtr, p.known = s, ptr, true
}

func (p *Replayer) list(l *List) {
	if l == nil || !p.hits(l.BBox) {
		return
	}
	for i := range l.Cmds {
		p.cmd(&l.Cmds[i])
	}
}

func (p *Replayer) cmd(c *Cmd) {
	switch c.Op {
	case OpChild:
		if c.Holder != nil {
			p.list(c.Holder.CmdList())
		}
		return
	case OpDirect:
		if c.Holder == nil || !p.hits(c.BBox) {
			return
		}
		p.applyClip(*c.Clip, c.Clip)
		ctx := p.ctx
		if off := image.Pt(int(c.X), int(c.Y)); off != (image.Point{}) && p.o.Offset != nil {
			ctx = p.o.Offset(ctx, off)
		}
		if p.o.Direct != nil {
			p.o.Direct(c.Holder, ctx)
		} else {
			c.Holder.Draw(ctx)
		}
		p.known = false // виджет мог оставить любой клип
		return
	}
	if !p.hits(c.BBox) {
		return
	}
	p.applyClip(*c.Clip, c.Clip)
	ctx := p.ctx
	x, y, w, h, rad := int(c.X), int(c.Y), int(c.W), int(c.H), int(c.R)
	switch c.Op {
	case OpFillRect:
		ctx.FillRect(x, y, w, h, c.Col)
	case OpFillRectAlpha:
		ctx.FillRectAlpha(x, y, w, h, c.Col)
	case OpFillRoundRect:
		ctx.FillRoundRect(x, y, w, h, rad, c.Col)
	case OpDrawBorder:
		ctx.DrawBorder(x, y, w, h, c.Col)
	case OpDrawRoundBorder:
		ctx.DrawRoundBorder(x, y, w, h, rad, c.Col)
	case OpSetPixel:
		ctx.SetPixel(x, y, c.Col)
	case OpHLine:
		ctx.DrawHLine(x, y, w, c.Col)
	case OpVLine:
		ctx.DrawVLine(x, y, h, c.Col)
	case OpDrawImage:
		ctx.DrawImage(c.Ext.Img, x, y)
	case OpDrawImageScaled:
		ctx.DrawImageScaled(c.Ext.Img, x, y, w, h)
	case OpDrawImageRect:
		if p.imgRect != nil {
			p.imgRect.DrawImageRect(c.Ext.Img, c.rect(), c.Ext.Rect, c.Ext.Filter)
		}
	case OpText:
		ctx.DrawTextFont(c.S, x, y, c.F[0], c.Font, c.Col)
	case OpTextRotated:
		if p.rotated != nil {
			p.rotated.DrawTextRotated(c.S, x, y, c.F[0], c.Font, int(c.I1), c.Col)
		}
	case OpFillEllipseAA:
		if p.aa != nil {
			p.aa.FillEllipseAA(x, y, w, h, c.Col)
		}
	case OpStrokeEllipseAA:
		if p.aa != nil {
			p.aa.StrokeEllipseAA(x, y, w, h, c.F[0], c.Col)
		}
	case OpFillPolygonAA:
		if p.aa != nil {
			p.aa.FillPolygonAA(c.Ext.Pts, c.Col)
		}
	case OpStrokePolylineAA:
		if p.aa != nil {
			p.aa.StrokePolylineAA(c.Ext.Pts, c.F[0], c.Flag, c.Col)
		}
	case OpDrawLineAA:
		if p.aa != nil {
			p.aa.DrawLineAA(x, y, int(c.I1), int(c.I2), c.F[0], c.Col)
		}
	case OpStrokeCubicAA:
		if p.paths != nil {
			f := &c.Ext.F
			p.paths.StrokeCubicAA(f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], c.Col)
		}
	case OpStrokePathAA:
		if p.paths != nil {
			p.paths.StrokePathAA(c.Ext.PtsF, c.F[0], c.Flag, c.Col)
		}
	case OpFillPathAA:
		if p.paths != nil {
			p.paths.FillPathAA(c.Ext.PtsF, c.Col)
		}
	case OpSoftShadow:
		if p.shadow != nil {
			p.shadow.DrawSoftShadow(c.rect(), rad, c.F[0], c.Col)
		}
	case OpShadow:
		if p.shadowP != nil {
			f := &c.Ext.F
			p.shadowP.DrawShadow(c.rect(), rad, f[0], f[1], f[2], c.Col)
		}
	case OpBlurBehind:
		if p.backdrop != nil {
			p.backdrop.BlurBehind(c.rect(), rad, c.Col)
		}
	case OpMica:
		if p.mica != nil {
			p.mica.MicaBehind(c.rect(), rad, c.Col)
		}
	case OpNoise:
		if p.noise != nil {
			p.noise.DrawNoise(c.rect(), c.F[0])
		}
	case OpBlitPixels:
		if p.blit != nil {
			p.blit.BlitPixels(c.Ext.RGBA, c.Ext.Rect, x, y, c.Flag)
		}
	case OpBlitBGRA:
		if p.blitBGRA != nil {
			p.blitBGRA.BlitPixelsBGRA(c.Ext.RGBA, c.Ext.Rect, x, y, c.Flag)
		}
	case OpOpacity:
		if p.opacity != nil {
			p.opacity.DrawWithOpacity(c.rect(), c.F[0], func() { p.list(c.Ext.Sub) })
		} else {
			p.list(c.Ext.Sub)
		}
	}
}
