package svg

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// strokeImg разбирает src и растеризует с заданными опциями на холсте w×h
// (масштаб 1: viewBox совпадает с пикселями).
func strokeImg(t *testing.T, src string, w, h int, o Options) *image.RGBA {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc.RasterizeWith(w, h, color.RGBA{A: 255}, false, o)
}

var strokeOn = Options{StrokeJoins: true}

// alphaSum — суммарное покрытие (в пикселях).
func alphaSum(img *image.RGBA) float64 {
	var s float64
	for i := 3; i < len(img.Pix); i += 4 {
		s += float64(img.Pix[i]) / 255
	}
	return s
}

func a(img *image.RGBA, x, y int) uint8 { return img.RGBAAt(x, y).A }

const joinSrc = `<svg viewBox="0 0 48 48"><path d="M8 40 L24 8 L40 40" fill="none" stroke="#000" stroke-width="6" stroke-linejoin="%s" stroke-miterlimit="%s"/></svg>`

func joinImg(t *testing.T, join, limit string, o Options) *image.RGBA {
	src := []byte(joinSrc)
	out := make([]byte, 0, len(src)+16)
	// подстановка двух %s без fmt, чтобы не плодить импорты в тестах
	n := 0
	for i := 0; i < len(src); i++ {
		if src[i] == '%' && i+1 < len(src) && src[i+1] == 's' {
			if n == 0 {
				out = append(out, join...)
			} else {
				out = append(out, limit...)
			}
			n++
			i++
			continue
		}
		out = append(out, src[i])
	}
	return strokeImg(t, string(out), 48, 48, o)
}

func TestStrokeJoins_MiterRoundBevelDiffer(t *testing.T) {
	miter := alphaSum(joinImg(t, "miter", "10", strokeOn))
	round := alphaSum(joinImg(t, "round", "10", strokeOn))
	bevel := alphaSum(joinImg(t, "bevel", "10", strokeOn))
	if !(miter > round+1 && round > bevel+0.5) {
		t.Errorf("площадь обводки должна убывать miter > round > bevel: %.1f, %.1f, %.1f", miter, round, bevel)
	}
	// Шип miter поднимается над точкой излома (y=8) до y≈1,3, у bevel — срез.
	if a(joinImg(t, "miter", "10", strokeOn), 23, 4) < 200 {
		t.Error("miter: вершина стыка не закрашена")
	}
	if a(joinImg(t, "bevel", "10", strokeOn), 23, 4) > 20 {
		t.Error("bevel: вершина стыка должна быть срезана")
	}
}

func TestStrokeJoins_OffByDefaultIsLegacy(t *testing.T) {
	// Без флага stroke-linejoin ни на что не влияет: прежняя обводка.
	m := alphaSum(joinImg(t, "miter", "10", Options{}))
	b := alphaSum(joinImg(t, "bevel", "10", Options{}))
	if m != b {
		t.Errorf("без флага join не должен действовать: %.2f и %.2f", m, b)
	}
}

func TestStrokeJoins_MiterLimitFallsBackToBevel(t *testing.T) {
	// Острый угол: отношение шипа к толщине ≈ 3,6; лимит 1,2 срезает, лимит 10 — нет.
	cut := joinImg(t, "miter", "1.2", strokeOn)
	bevel := joinImg(t, "bevel", "1.2", strokeOn)
	long := joinImg(t, "miter", "10", strokeOn)
	if math.Abs(alphaSum(cut)-alphaSum(bevel)) > 0.5 {
		t.Errorf("miterlimit=1.2 должен дать срез как bevel: %.2f против %.2f", alphaSum(cut), alphaSum(bevel))
	}
	if alphaSum(long) <= alphaSum(cut)+2 {
		t.Error("miterlimit=10 должен оставить шип")
	}
}

func capImg(t *testing.T, cap string, o Options) *image.RGBA {
	return strokeImg(t, `<svg viewBox="0 0 48 48"><path d="M12 24 H36" stroke="#000" stroke-width="8" stroke-linecap="`+cap+`"/></svg>`, 48, 48, o)
}

func TestStrokeCaps(t *testing.T) {
	butt, round, square := capImg(t, "butt", strokeOn), capImg(t, "round", strokeOn), capImg(t, "square", strokeOn)
	// За концом (x=10): butt пусто, round — внутри полукруга, square — сплошь.
	if a(butt, 10, 24) != 0 {
		t.Errorf("butt: за концом должно быть пусто, а=%d", a(butt, 10, 24))
	}
	if a(round, 10, 24) < 250 || a(square, 10, 24) != 255 {
		t.Errorf("round/square за концом: %d, %d", a(round, 10, 24), a(square, 10, 24))
	}
	// Угол квадрата (пиксель 8,20) закрашен у square, но не у round (радиус 4
	// вокруг (12,24) его не достаёт).
	if a(square, 8, 20) != 255 || a(round, 8, 20) > 40 {
		t.Errorf("угол колпачка: square=%d (ждали 255), round=%d (ждали ~0)", a(square, 8, 20), a(round, 8, 20))
	}
}

func TestStrokeCaps_OffByDefaultIsLegacy(t *testing.T) {
	if a(capImg(t, "square", Options{}), 10, 24) != 0 {
		t.Error("без флага square-колпачок рисоваться не должен")
	}
}

func TestStrokeDash(t *testing.T) {
	src := func(extra string) string {
		return `<svg viewBox="0 0 48 48"><path d="M2 24 H46" stroke="#000" stroke-width="4" stroke-dasharray="4 4" ` + extra + `/></svg>`
	}
	img := strokeImg(t, src(""), 48, 48, strokeOn)
	// штрих 2..6, пробел 6..10, штрих 10..14
	if a(img, 4, 24) != 255 || a(img, 8, 24) != 0 || a(img, 12, 24) != 255 {
		t.Errorf("пунктир 4/4: (4)=%d (8)=%d (12)=%d", a(img, 4, 24), a(img, 8, 24), a(img, 12, 24))
	}
	// смещение 4: сначала пробел
	img = strokeImg(t, src(`stroke-dashoffset="4"`), 48, 48, strokeOn)
	if a(img, 4, 24) != 0 || a(img, 8, 24) != 255 {
		t.Errorf("dashoffset=4: (4)=%d (8)=%d", a(img, 4, 24), a(img, 8, 24))
	}
	// без флага — сплошная линия
	img = strokeImg(t, src(""), 48, 48, Options{})
	if a(img, 8, 24) == 0 {
		t.Error("без флага пунктира быть не должно")
	}
}

func TestStrokeDash_OddCountRepeats(t *testing.T) {
	if got := parseDashArray("3 1 2"); len(got) != 6 || got[3] != 3 {
		t.Errorf("нечётный список должен удваиваться: %v", got)
	}
	for _, bad := range []string{"none", "", "0 0", "-1 2", "a b"} {
		if parseDashArray(bad) != nil {
			t.Errorf("%q: пунктира быть не должно", bad)
		}
	}
}

func TestStrokeDash_RoundDots(t *testing.T) {
	// Штрихи нулевой длины с круглым колпачком — точки.
	img := strokeImg(t, `<svg viewBox="0 0 48 48"><path d="M6 24 H42" stroke="#000" stroke-width="6" stroke-linecap="round" stroke-dasharray="0 12"/></svg>`, 48, 48, strokeOn)
	if a(img, 6, 24) < 250 || a(img, 12, 24) != 0 || a(img, 18, 24) < 250 {
		t.Errorf("точки через 12: (6)=%d (12)=%d (18)=%d", a(img, 6, 24), a(img, 12, 24), a(img, 18, 24))
	}
}

func TestStrokeDash_CircleStartsAtRightPoint(t *testing.T) {
	// SVG: окружность начинается справа (cx+r, cy) и идёт вниз. Первый штрих
	// длиной 20 покрывает правую нижнюю дугу, верх остаётся пустым.
	img := strokeImg(t, `<svg viewBox="0 0 48 48"><circle cx="24" cy="24" r="18" fill="none" stroke="#000" stroke-width="4" stroke-dasharray="20 1000"/></svg>`, 48, 48, strokeOn)
	if a(img, 40, 30) < 250 {
		t.Errorf("правая нижняя часть должна быть закрашена: %d", a(img, 42, 30))
	}
	if a(img, 24, 6) != 0 || a(img, 6, 24) != 0 {
		t.Errorf("верх и лево должны быть пусты: %d, %d", a(img, 24, 6), a(img, 6, 24))
	}
}

func TestStrokeClosedPathHasHoleAndCorners(t *testing.T) {
	// Замкнутый контур: внутри дыра (кольцо не должно заполниться), внешние
	// углы — по join.
	src := func(join string) string {
		return `<svg viewBox="0 0 48 48"><path d="M10 10 H38 V38 H10 Z" fill="none" stroke="#000" stroke-width="6" stroke-linejoin="` + join + `"/></svg>`
	}
	m := strokeImg(t, src("miter"), 48, 48, strokeOn)
	r := strokeImg(t, src("round"), 48, 48, strokeOn)
	if a(m, 24, 24) != 0 {
		t.Errorf("внутри замкнутого контура должна быть дыра, а=%d", a(m, 24, 24))
	}
	if a(m, 7, 7) != 255 {
		t.Errorf("miter: внешний угол не закрашен: %d", a(m, 7, 7))
	}
	if a(r, 7, 7) > 100 {
		t.Errorf("round: внешний угол должен быть скруглён: %d", a(r, 7, 7))
	}
	if a(m, 10, 24) != 255 || a(m, 24, 10) != 255 {
		t.Error("стороны контура должны быть сплошными")
	}
}

func TestStrokeRoundCapDot(t *testing.T) {
	img := strokeImg(t, `<svg viewBox="0 0 48 48"><path d="M24 24 h0" stroke="#000" stroke-width="12" stroke-linecap="round"/></svg>`, 48, 48, strokeOn)
	if a(img, 24, 24) != 255 || a(img, 24, 19) < 200 || a(img, 24, 16) != 0 {
		t.Errorf("точка r=6: центр=%d (24,19)=%d (24,16)=%d", a(img, 24, 24), a(img, 24, 19), a(img, 24, 16))
	}
	if alphaSum(strokeImg(t, `<svg viewBox="0 0 48 48"><path d="M24 24 h0" stroke="#000" stroke-width="12"/></svg>`, 48, 48, strokeOn)) != 0 {
		t.Error("butt: нулевой отрезок не должен рисоваться")
	}
}

func TestStrokeCrossingSegmentsDoNotCancel(t *testing.T) {
	// Два перекрещенных штриха в разные стороны: на пересечении покрытие
	// складывается, а не вычитается (все контуры обводки одного направления).
	img := strokeImg(t, `<svg viewBox="0 0 48 48"><g stroke="#000" stroke-width="6" fill="none"><path d="M6 6 L42 42"/><path d="M42 6 L6 42"/><path d="M42 42 L6 6"/></g></svg>`, 48, 48, strokeOn)
	if a(img, 24, 24) != 255 {
		t.Errorf("пересечение: а=%d, ожидалось 255", a(img, 24, 24))
	}
}

func TestStrokeRingAreaIsAccurate(t *testing.T) {
	// Кольцо радиуса R=16 толщиной 5: площадь 2πR·w. Настоящая обводка держит
	// её в пределах процента; упрощённая даёт «гребень» на кривой.
	const src = `<svg viewBox="0 0 48 48"><circle cx="24" cy="24" r="16" fill="none" stroke="#000" stroke-width="5"/></svg>`
	want := 2 * math.Pi * 16 * 5
	got := alphaSum(strokeImg(t, src, 48, 48, strokeOn))
	if math.Abs(got-want)/want > 0.01 {
		t.Errorf("площадь кольца %.1f, ожидалась %.1f (±1%%)", got, want)
	}
}

func TestStrokeThickCurveHasNoGaps(t *testing.T) {
	// Толстая дуга, где легаси-обводка оставляет щели на внешней стороне
	// стыков: внутри ленты не должно быть полупрозрачных пикселей.
	img := strokeImg(t, `<svg viewBox="0 0 96 96"><path d="M10 80 A40 40 0 0 1 86 80" fill="none" stroke="#000" stroke-width="14"/></svg>`, 96, 96, strokeOn)
	// Центр дуги (48; 92,5), радиус 40; точки лежат на середине ленты.
	for deg := 25.0; deg < 156; deg += 5 {
		rad := deg * math.Pi / 180
		x := int(48 + 40*math.Cos(rad))
		y := int(92.5 - 40*math.Sin(rad))
		if v := a(img, x, y); v != 255 {
			t.Fatalf("щель в центре ленты на %.0f°: (%d,%d) а=%d", deg, x, y, v)
		}
	}
}

func TestStrokeNoMinimumWidthInPreciseMode(t *testing.T) {
	const src = `<svg viewBox="0 0 48 48"><path d="M4 24 H44" stroke="#000" stroke-width="0.25"/></svg>`
	legacy := alphaSum(strokeImg(t, src, 48, 48, Options{}))
	precise := alphaSum(strokeImg(t, src, 48, 48, strokeOn))
	if !(precise < legacy*0.6) {
		t.Errorf("точный режим не должен утолщать до 0,75 px: %.2f против %.2f", precise, legacy)
	}
}

func TestStrokeDegenerateInputsDoNotPanic(t *testing.T) {
	for _, src := range []string{
		`<svg viewBox="0 0 8 8"><path d="M1 1" stroke="#000"/></svg>`,
		`<svg viewBox="0 0 8 8"><path d="M1 1 L1 1 L1 1" stroke="#000" stroke-linecap="square"/></svg>`,
		`<svg viewBox="0 0 8 8"><path d="M1 1 L7 1 L1 1" stroke="#000" stroke-linejoin="round" stroke-width="3"/></svg>`,
		`<svg viewBox="0 0 8 8"><path d="M1 1 L7 1 Z" stroke="#000" stroke-width="2" stroke-dasharray="0.0001"/></svg>`,
		`<svg viewBox="0 0 8 8"><circle cx="4" cy="4" r="3" stroke="#000" stroke-width="1e9"/></svg>`,
		`<svg viewBox="0 0 8 8"><path d="M0 0 L8 8" stroke="#000" stroke-dasharray="1e-9 1e-9"/></svg>`,
	} {
		strokeImg(t, src, 16, 16, PreciseOptions)
	}
}

func TestStrokeDefaultsParsed(t *testing.T) {
	doc, _ := Parse([]byte(`<svg viewBox="0 0 8 8"><g stroke="#000" stroke-linejoin="round" stroke-linecap="square" stroke-miterlimit="7" stroke-dasharray="2,1" stroke-dashoffset="0.5"><path d="M1 1 L7 7" transform="scale(2)"/></g></svg>`))
	sh := doc.Shapes[0]
	if sh.StrokeJoin != JoinRound || sh.StrokeCap != CapSquare || sh.MiterLimit != 7 {
		t.Errorf("join/cap/limit: %v %v %v", sh.StrokeJoin, sh.StrokeCap, sh.MiterLimit)
	}
	if len(sh.Dash) != 2 || sh.Dash[0] != 4 || sh.Dash[1] != 2 || sh.DashOffset != 1 {
		t.Errorf("пунктир должен масштабироваться transform: %v +%v", sh.Dash, sh.DashOffset)
	}
}
