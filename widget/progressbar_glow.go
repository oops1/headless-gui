// progressbar_glow.go — светящаяся полоса прогресса (ProgressStyleGlow).
//
// Дорожка-пилюля с тонкой рамкой, по ней едет яркая «голова» с ореолом, за
// ней тянется градиентный след от бирюзы к голубому, вдоль следа мерцают
// редкие искры. Так выглядят индикаторы ожидания в современных приложениях.
//
// Стиль применяется ТОЛЬКО в современных темах (Win10/Win11). В классике
// Win2000 и в Mac-теме полоса рисуется штатно для темы: там у прогресса
// есть свой канонический вид, и подменять его свечением неуместно.
//
// Мягкие переходы делаются через FillRectAlpha (Over, alpha-premultiplied):
// ореол — стопка вложенных эллипсов с падающей альфой, след — колонки с
// нарастающей яркостью. SetPixel здесь не годится — он пишет цвет как есть,
// без смешивания.
package widget

import (
	"image"
	"image/color"
	"math"
	"time"
)

// ProgressBarStyle — манера отрисовки ProgressBar.
type ProgressBarStyle int

const (
	// ProgressStyleBar — штатная полоса активной темы (по умолчанию).
	ProgressStyleBar ProgressBarStyle = iota
	// ProgressStyleGlow — светящаяся голова с градиентным следом.
	// В классике Win2000 и в Mac-теме подменяется штатным видом темы.
	ProgressStyleGlow
)

// Геометрия свечения (доли высоты дорожки, если не сказано иное).
const (
	glowTrailH   = 0.34 // толщина следа
	glowHaloRX   = 5.0  // горизонтальный радиус ореола
	glowHaloRY   = 2.6  // вертикальный радиус ореола
	glowSparks   = 22   // «звёзд» вдоль следа
	glowPulseDur = 1700 * time.Millisecond
	glowSweepDur = 2200 * time.Millisecond // один проход головы по дорожке
)

// glowEnabled — рисовать ли свечение: стиль запрошен и тема современная.
func (pb *ProgressBar) glowEnabled() bool {
	if pb.Style != ProgressStyleGlow {
		return false
	}
	st := currentStyle()
	return !st.Classic3D && !st.MacTitleBar
}

// glowColors возвращает цвета хвоста и головы: поля виджета, иначе тема,
// иначе — расчёт от цвета заливки (голова светлее, хвост уходит в бирюзу).
func (pb *ProgressBar) glowColors() (tail, head color.RGBA) {
	tail, head = pb.GlowTail, pb.GlowHead
	if tail.A == 0 {
		tail = win10.ProgressGlowTail
	}
	if head.A == 0 {
		head = win10.ProgressGlowHead
	}
	if tail.A == 0 {
		f := pb.FillColor
		tail = color.RGBA{R: f.R / 3, G: chanAdd(f.G, 70), B: chanScale(f.B, 0.75), A: 255}
	}
	if head.A == 0 {
		head = brighten(pb.FillColor, 90)
		head.A = 255
	}
	return tail, head
}

// drawGlow рисует светящуюся полосу для значения v ∈ [0,1]. В неопределённом
// режиме голова ходит по дорожке сама, значение не используется.
func (pb *ProgressBar) drawGlow(ctx DrawContext, b image.Rectangle, v float64) {
	da, ok := ctx.(DrawContextAlpha)
	if !ok {
		// Контекст без смешивания (тесты, экзотический хост) — рисуем
		// штатную полосу темы, чтобы прогресс всё равно был виден.
		pb.drawThemed(ctx, b, v)
		return
	}
	pb.markDrawn()
	pb.ensureGlowAnim()

	h := b.Dy()
	if h < 4 || b.Dx() < 8 {
		return
	}
	rad := h / 2
	tail, head := pb.glowColors()

	// ── Дорожка: приглушённая заливка и тонкая рамка-пилюля ──────────────
	ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), h, rad, pb.Background)
	if pb.ShowBorder {
		ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), h, rad, mixRGBA(pb.BorderColor, tail, 0.35))
	}

	// ── Положение головы ─────────────────────────────────────────────────
	inset := rad // голова не выезжает за скругления дорожки
	x0 := b.Min.X + inset
	x1 := b.Max.X - inset
	if x1 <= x0 {
		return
	}
	var headX int
	erasing := false
	if pb.indeterminate.Load() {
		var t float64
		t, erasing = glowSweep()
		headX = x0 + int(t*float64(x1-x0))
	} else {
		headX = x0 + int(math.Round(v*float64(x1-x0)))
	}
	cy := b.Min.Y + h/2

	// Подсвеченный участок дорожки. На рисующем проходе он тянется от начала
	// до головы, на гасящем — от головы до конца: голова «съедает» линию,
	// оставленную предыдущим проходом.
	segFrom, segTo := x0, headX
	if erasing {
		segFrom, segTo = headX, x1
	}

	// Пульсация ореола: заметная, но не мигающая.
	pulse := 0.82 + 0.18*math.Sin(glowPhase(glowPulseDur)*2*math.Pi)

	// ── След: колонки от начала дорожки до головы ────────────────────────
	trailH := int(math.Round(float64(h) * glowTrailH))
	if trailH < 2 {
		trailH = 2
	}
	// Свечение — приём тёмного интерфейса: на светлой дорожке белое ядро
	// сливается с фоном, а ореол выглядит кляксой. Там «горячий» цвет
	// оставляем в синеве и приглушаем ореол.
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	hot, haloK := white, 1.0
	if lightBG(pb.Background) {
		hot = lerpRGBA(head, white, 0.35)
		haloK = 0.55
	}
	trailTop := cy - trailH/2
	span := float64(segTo - segFrom)
	// Вертикальное свечение вокруг следа: три слоя, чем дальше от центра,
	// тем прозрачнее — от них след светится, а не выглядит нарисованным.
	haze := []struct {
		h int
		a float64
	}{
		{trailH * 5, 0.06},
		{trailH * 3, 0.10},
		{trailH * 2, 0.16},
	}
	for x := segFrom; x <= segTo; x += 2 {
		// f — близость к голове: 1 у неё, 0 у дальнего конца участка.
		f := 1.0
		if span > 0 {
			if erasing {
				f = float64(segTo-x) / span
			} else {
				f = float64(x-segFrom) / span
			}
		}
		w := 2
		if x+w > segTo+1 {
			w = segTo + 1 - x
		}
		if w <= 0 {
			continue
		}
		// Цвет уходит от бирюзы к голубому и у самой головы — в белый.
		col := lerpRGBA(tail, head, f)
		if f > 0.8 {
			col = lerpRGBA(col, hot, (f-0.8)/0.2*0.8)
		}
		// Яркость нарастает к голове: хвост едва тлеет, у головы — в полную.
		a := 0.30 + 0.70*f*f
		for _, hz := range haze {
			da.FillRectAlpha(x, cy-hz.h/2, w, hz.h, premul(col, hz.a*a*pulse))
		}
		da.FillRectAlpha(x, trailTop, w, trailH, premul(col, a))
		// Тонкая светлая жила по центру следа — «лазерная» сердцевина.
		if f > 0.25 {
			da.FillRectAlpha(x, cy, w, 1, premul(hot, 0.55*(f-0.25)/0.75))
		}
	}

	// ── Искры вдоль следа ────────────────────────────────────────────────
	// ── Звёздное небо вдоль следа ────────────────────────────────────────
	// Огоньки стоят на месте (позиции детерминированы), но мерцают каждый
	// со своей фазой — след не выглядит ровной чертой и не гаснет разом.
	if span > 20 {
		ph := glowPhase(3200 * time.Millisecond)
		for i := 0; i < glowSparks; i++ {
			// Золотое сечение даёт равномерно-неравномерную россыпь.
			r := math.Mod(float64(i)*0.6180339887+0.13, 1)
			sx := segFrom + int(r*span)
			if sx < segFrom+2 || sx > segTo-2 {
				continue
			}
			// Своя фаза и своя скорость мерцания у каждой звезды.
			tw := 0.5 + 0.5*math.Sin((ph*(1+0.35*math.Mod(float64(i)*0.37, 1))+r)*2*math.Pi)
			// Ближе к голове ярче — и на рисующем проходе, и на гасящем.
			near := r
			if erasing {
				near = 1 - r
			}
			a := (0.12 + 0.5*tw) * (0.25 + 0.75*near)
			// Каждая четвёртая — крупнее и чуть выше/ниже линии: «созвездие»,
			// а не пунктир.
			sz, dy := 1, 0
			if i%4 == 0 {
				sz = 2
				dy = int(math.Round(math.Sin(float64(i)) * float64(trailH)))
			}
			da.FillRectAlpha(sx, cy+dy-sz/2, sz, sz, premul(hot, a))
			if sz > 1 {
				// Мягкий ореол вокруг крупной звезды.
				da.FillRectAlpha(sx-1, cy+dy-2, 4, 4, premul(head, a*0.25))
			}
		}
	}

	// ── Ореол головы ─────────────────────────────────────────────────────
	rx := int(float64(h) * glowHaloRX * pulse)
	ry := int(float64(h) * glowHaloRY * pulse)
	// Двойной ореол: широкий мягкий и узкий плотный — так свет выглядит
	// объёмным, а не одним ровным пятном.
	drawGlowHalo(da, headX, cy, rx, ry, head, 0.55*pulse*haloK, 3)
	drawGlowHalo(da, headX, cy, rx/2, ry/2, lerpRGBA(head, hot, 0.4), 0.7*pulse*haloK, 2)

	// ── Ядро головы: яркая точка со светлым центром ──────────────────────
	core := rad
	if core < 3 {
		core = 3
	}
	// Лучи: длинный горизонтальный и короткий вертикальный — так свет
	// читается звездой, а не круглой точкой.
	// Лучи короткие: это лёгкий блик, а не «звезда во всё окно».
	drawStarRays(da, headX, cy, int(float64(h)*1.4*pulse), int(float64(h)*0.65*pulse),
		hot, 0.5*pulse*haloK)
	fillCircle(ctx, headX, cy, core, head)
	fillCircle(ctx, headX, cy, core*2/3, lerpRGBA(head, hot, 0.55))
	fillCircle(ctx, headX, cy, core/3, hot)
}

// drawGlowHalo рисует радиальный ореол: для каждой клетки эллипса берётся
// нормированное расстояние до центра и по нему — альфа. Ступенчатые кольца
// (стопка вложенных эллипсов) на градиенте были бы видны как полосы, а
// поклеточный спад даёт ровное свечение.
//
// step — ширина клетки: чем меньше, тем плавнее по горизонтали и тем больше
// вызовов заливки; по вертикали шаг всегда 1 пиксель.
func drawGlowHalo(da DrawContextAlpha, cx, cy, rx, ry int, col color.RGBA, peak float64, step int) {
	if rx < 2 || ry < 1 || peak <= 0 {
		return
	}
	if step < 1 {
		step = 1
	}
	for dy := -ry; dy <= ry; dy++ {
		ty := float64(dy) / float64(ry)
		if ty*ty >= 1 {
			continue
		}
		hw := int(float64(rx) * math.Sqrt(1-ty*ty)) // полуширина строки
		for x := -hw; x <= hw; x += step {
			tx := float64(x) / float64(rx)
			d := math.Sqrt(tx*tx + ty*ty) // 0 — центр, 1 — край
			if d >= 1 {
				continue
			}
			e := 1 - d
			a := peak * e * e // мягкий спад к краю
			if a < 0.004 {
				continue
			}
			w := step
			if x+w > hw {
				w = hw - x + 1
			}
			if w > 0 {
				da.FillRectAlpha(cx+x, cy+dy, w, 1, premul(col, a))
			}
		}
	}
}

// drawStarRays рисует крестообразный блик звезды: луч сужается и гаснет к
// концу, поэтому у центра он сливается с ядром, а на краю тает.
func drawStarRays(da DrawContextAlpha, cx, cy, lenX, lenY int, col color.RGBA, peak float64) {
	if peak <= 0 {
		return
	}
	// Горизонтальный луч (в обе стороны).
	for dx := 1; dx <= lenX; dx++ {
		f := 1 - float64(dx)/float64(lenX)
		a := peak * f * f * f // резкий спад: длинный, но лёгкий хвост луча
		if a < 0.004 {
			continue
		}
		th := 1
		if f > 0.6 {
			th = 2 // у основания луч толще
		}
		pc := premul(col, a)
		da.FillRectAlpha(cx+dx, cy-th/2, 1, th, pc)
		da.FillRectAlpha(cx-dx, cy-th/2, 1, th, pc)
	}
	// Вертикальный луч — короче: так блик выглядит «объективным», как в оптике.
	for dy := 1; dy <= lenY; dy++ {
		f := 1 - float64(dy)/float64(lenY)
		a := peak * f * f * f
		if a < 0.004 {
			continue
		}
		pc := premul(col, a)
		da.FillRectAlpha(cx, cy+dy, 1, 1, pc)
		da.FillRectAlpha(cx, cy-dy, 1, 1, pc)
	}
}

// ─── Анимация ───────────────────────────────────────────────────────────────

// glowPhase возвращает фазу [0,1) периода dur по стенным часам: свечение
// не хранит состояния и одинаково выглядит на любом кадре.
func glowPhase(dur time.Duration) float64 {
	if dur <= 0 {
		return 0
	}
	return float64(time.Now().UnixNano()%int64(dur)) / float64(dur)
}

// glowSweep — ход головы в неопределённом режиме. Цикл состоит из ДВУХ
// проходов, и оба идут слева направо: первый тянет за собой линию, второй
// её гасит. Возврата назад нет вовсе, а раз линия к началу каждого прохода
// в нужном состоянии, нет и рывка на стыке.
//
// Возвращает положение головы t ∈ [0,1] и признак гасящего прохода.
func glowSweep() (t float64, erasing bool) {
	p := glowPhase(2 * glowSweepDur)
	if p < 0.5 {
		return p * 2, false
	}
	return (p - 0.5) * 2, true
}

// markDrawn запоминает момент отрисовки: по нему зацикленная анимация
// понимает, что виджет ещё на экране.
func (pb *ProgressBar) markDrawn() { pb.lastDrawn.Store(time.Now().UnixNano()) }

// ensureGlowAnim держит кадры идущими, пока светящаяся полоса на экране.
// Виджет в диалоге не получает фокуса, поэтому NeedsAnimation его не спасёт:
// нужен зацикленный Animate, который движок учитывает через AnimationsActive.
// Анимация снимает себя сама, когда виджет перестал рисоваться (диалог
// закрыли, вкладку переключили) — иначе движок никогда бы не засыпал.
func (pb *ProgressBar) ensureGlowAnim() {
	if pb.glowAnim != nil && pb.glowAnim.Running() {
		return
	}
	var a *Animation
	a = AnimateOwned(pb, "glow", 100*time.Millisecond, EaseLinear, func(float64) {
		// Зацикленная анимация не получает OnDone — самоснятие делаем здесь.
		if time.Since(time.Unix(0, pb.lastDrawn.Load())) > 500*time.Millisecond {
			a.Stop()
			return
		}
		notifyRectChanged(pb.glowDamageRect())
	})
	a.Loop = true
	pb.glowAnim = a
}

// glowDamageRect — область, которую нужно перерисовать. У свечения ореол
// выходит за границы виджета, и damage строго по bounds оставил бы за ними
// старый кадр; штатной полосе хватает собственных границ.
func (pb *ProgressBar) glowDamageRect() image.Rectangle {
	b := pb.Bounds()
	if b.Empty() || !pb.glowEnabled() {
		return b
	}
	h := b.Dy()
	return b.Inset(-int(float64(h)*glowHaloRY) - h)
}

// marqueePhase — фаза бегущей полосы (классика Win2000): период 1.4 с.
func marqueePhase() float64 { return glowPhase(1400 * time.Millisecond) }

// ─── Цветовые помощники ─────────────────────────────────────────────────────

// lightBG — светлая ли подложка (яркость по Rec. 601, порог как у
// contrastText). От этого зависит, насколько ярким можно быть свечению.
func lightBG(c color.RGBA) bool {
	return (299*int(c.R)+587*int(c.G)+114*int(c.B))/1000 > 140
}

// premul переводит straight-цвет в alpha-premultiplied с альфой a ∈ [0,1] —
// в таком виде его ждёт FillRectAlpha (Over по модели Go).
func premul(c color.RGBA, a float64) color.RGBA {
	if a <= 0 {
		return color.RGBA{}
	}
	if a > 1 {
		a = 1
	}
	al := uint8(a * 255)
	m := func(v uint8) uint8 { return uint8(uint32(v) * uint32(al) / 255) }
	return color.RGBA{R: m(c.R), G: m(c.G), B: m(c.B), A: al}
}

// lerpRGBA смешивает цвета по t ∈ [0,1] (0 — a, 1 — b).
func lerpRGBA(a, b color.RGBA, t float64) color.RGBA {
	return mixRGBA(a, b, t)
}

// chanAdd / chanScale — арифметика канала без переполнения.
func chanAdd(v uint8, d int) uint8 {
	s := int(v) + d
	if s > 255 {
		return 255
	}
	if s < 0 {
		return 0
	}
	return uint8(s)
}

func chanScale(v uint8, k float64) uint8 {
	s := float64(v) * k
	if s > 255 {
		return 255
	}
	if s < 0 {
		return 0
	}
	return uint8(s)
}
