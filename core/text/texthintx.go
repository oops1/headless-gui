package text

import "sort"

// Привязка вертикальных штрихов к пиксельной сетке (TextQuality.HintingX).
//
// Вертикальный автохинтинг (texthint.go) координату X не трогает, и на мелком
// кегле ствол «l», «i», «Н» шириной около пикселя ложится на границу двух
// пикселей: вместо одного плотного столбца выходят два серых. Здесь, как у
// хинтинга GDI и DirectWrite в режиме с выравниванием, для каждого глифа
// находятся вертикальные штрихи — пары «левая кромка, правая кромка» с общим
// участком по высоте, — их ширина округляется до целого числа пикселей (не
// меньше одного), а положение — к ближайшей целой границе. Контур между
// кромками растягивается по X кусочно-линейно, за крайними — сдвигается.
//
// Шаг глифа и кернинг не меняются: ширина строки и раскладка прежние, глиф
// сдвигается и сжимается внутри своей ячейки — штрих не дальше чем на
// полпикселя (в ряду стволов с равным шагом — до ¾ px), плюс до ¼ px сдвига
// к середине ячейки при целом шаге (texthint.go, hintedGlyph). Знак вне зон и
// шрифт без зон хинтинга сюда не попадают: их форма остаётся прежней.
// Кромки — прямые отрезки контура и концы кривых с вертикальной касательной
// (бока «о», «с», «е» в TrueType заданы именно так). Всё в целых 26.6.

// vEdge — вертикальный участок контура на абсциссе x от y0 к y1 (26.6, ось
// вверх; порядок — направление обхода, по нему видно, с какой стороны чернила).
type vEdge struct{ x, y0, y1 int }

// verticalSlope — участок считается вертикальным, если |dx|·verticalSlope ≤ |dy|
// (наклон меньше ~3,6°): у части шрифтов ствол чуть скошен округлением.
const verticalSlope = 16

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// noteVertical запоминает отрезок (x0,y0)→(x1,y1), если он вертикален.
func (s *hintScan) noteVertical(x0, y0, x1, y1 int) {
	if y0 == y1 || absInt(x1-x0)*verticalSlope > absInt(y1-y0) {
		return
	}
	s.vedges = append(s.vedges, vEdge{x: (x0 + x1) / 2, y0: y0, y1: y1})
}

// tangentEdge — кривая с концом (px, py) и соседней управляющей точкой
// (cx, cy): если касательная в конце вертикальна, кривая около него идёт
// вдоль вертикали — учитывается половина пути к управляющей точке. start —
// конец является началом кривой (обход идёт от него к управляющей точке).
func (s *hintScan) tangentEdge(px, py, cx, cy int, start bool) {
	if cy == py || absInt(cx-px)*verticalSlope > absInt(cy-py) {
		return
	}
	mid := py + (cy-py)/2
	if start {
		s.vedges = append(s.vedges, vEdge{x: px, y0: py, y1: mid})
	} else {
		s.vedges = append(s.vedges, vEdge{x: px, y0: mid, y1: py})
	}
}

// quadTo — квадратичная кривая cur → (x, y) с управляющей точкой (cx, cy).
func (s *hintScan) quadTo(cx, cy, x, y int) {
	if s.have {
		s.tangentEdge(s.cur[0], s.cur[1], cx, cy, true)
		s.tangentEdge(x, y, cx, cy, false)
	}
	s.seg(x, y, false)
}

// cubeTo — кубическая кривая cur → (x, y) с управляющими точками c1, c2.
func (s *hintScan) cubeTo(c1x, c1y, c2x, c2y, x, y int) {
	if s.have {
		s.tangentEdge(s.cur[0], s.cur[1], c1x, c1y, true)
		s.tangentEdge(x, y, c2x, c2y, false)
	}
	s.seg(x, y, false)
}

// mergeVEdges сливает участки одного направления на одной абсциссе (±1/32 px),
// которые касаются или перекрываются по высоте: бок «о» из двух половинок
// кривых, ствол из нескольких отрезков.
func mergeVEdges(in []vEdge) []vEdge {
	out := make([]vEdge, 0, len(in))
	for _, e := range in {
		up := e.y1 > e.y0
		lo, hi := min(e.y0, e.y1), max(e.y0, e.y1)
		merged := false
		for i := range out {
			o := &out[i]
			if (o.y1 > o.y0) != up || absInt(o.x-e.x) > 2 {
				continue
			}
			olo, ohi := min(o.y0, o.y1), max(o.y0, o.y1)
			if lo > ohi || hi < olo {
				continue
			}
			nlo, nhi := min(lo, olo), max(hi, ohi)
			if up {
				o.y0, o.y1 = nlo, nhi
			} else {
				o.y0, o.y1 = nhi, nlo
			}
			merged = true
			break
		}
		if !merged {
			out = append(out, e)
		}
	}
	return out
}

// vOverlap — длина общего участка двух кромок по высоте.
func vOverlap(a, b vEdge) int {
	lo := max(min(a.y0, a.y1), min(b.y0, b.y1))
	hi := min(max(a.y0, a.y1), max(b.y0, b.y1))
	return hi - lo
}

// stemMinOverlap — сколько общей высоты (26.6) нужно кромкам, чтобы считаться
// одним штрихом: четверть пикселя.
const stemMinOverlap = 16

// stemMinWidth — самый узкий штрих (26.6), который выравнивается: треть
// пикселя. Уже — это не ствол, а стык контуров; округление до целого пикселя
// утолстило бы букву втрое.
const stemMinWidth = 21

// xStemHint строит отображение X для одного глифа по его вертикальным
// штрихам; nil — штрихов нет, X остаётся прежним. emPx — размер em в 26.6
// (ограничивает толщину штриха), std — стандартная толщина ствола шрифта на
// этом кегле (stdStem; 0 — неизвестна). Координаты сканера уже включают
// подпиксельный сдвиг пера: штрихи встают на целые пиксели маски.
func xStemHint(s *hintScan, emPx, std int) *vHint {
	s.closeContour()
	if s.area == 0 || len(s.vedges) < 2 {
		return nil
	}
	lefts, rights := splitVEdges(s)
	if len(lefts) == 0 || len(rights) == 0 {
		return nil
	}
	stems := pairStems(lefts, rights, emPx/4)
	if len(stems) == 0 {
		return nil
	}
	for i := range stems {
		stems[i].w = stemWidth(stems[i], std)
	}
	if h := regularStems(stems); h != nil {
		return h
	}
	return singleStems(stems)
}

// splitVEdges делит вертикальные кромки замкнутого контура на левые и правые
// (левые — по возрастанию X). Чернила справа от обхода при отрицательной
// площади (TrueType, ось вверх): участок вверх — левая кромка штриха, вниз —
// правая. У контуров PostScript знак площади обратный, и роли меняются (как
// у withStems).
func splitVEdges(s *hintScan) (lefts, rights []vEdge) {
	if s.area == 0 {
		return nil, nil
	}
	for _, e := range mergeVEdges(s.vedges) {
		if absInt(e.y1-e.y0) < stemMinLen {
			continue
		}
		if (e.y1 > e.y0) == (s.area < 0) {
			lefts = append(lefts, e)
		} else {
			rights = append(rights, e)
		}
	}
	sort.Slice(lefts, func(i, j int) bool { return lefts[i].x < lefts[j].x })
	return lefts, rights
}

// xStem — вертикальный штрих глифа: левая и правая кромки (26.6), длина
// общего участка кромок по высоте и выровненная толщина.
type xStem struct{ l, r, ov, w int }

// pairStems собирает штрихи из кромок: левой кромке — правая того же штриха.
func pairStems(lefts, rights []vEdge, maxW int) []xStem {
	var stems []xStem
	for _, l := range lefts {
		// Правая кромка того же штриха — справа, не дальше maxW, с самым
		// длинным общим участком по высоте (при равенстве — ближайшая). Не
		// ближайшая вообще: у ствола «а», «р», «ф» рядом бывают короткие
		// участки примыканий чаши, и пара с ними дала бы «штрих» в доли
		// пикселя, раздутый до целого.
		best, bestOv := -1, 0
		for i, r := range rights {
			if r.x-l.x < stemMinWidth || r.x-l.x > maxW {
				continue
			}
			ov := vOverlap(l, r)
			if ov < stemMinOverlap {
				continue
			}
			if best < 0 || ov > bestOv || (ov == bestOv && r.x < rights[best].x) {
				best, bestOv = i, ov
			}
		}
		if best < 0 {
			continue
		}
		st := xStem{l: l.x, r: rights[best].x, ov: bestOv}
		// Штрихи, перекрывающиеся по X, — один и тот же ствол или короткий
		// стык внутри него (угол «L», «E» у Roboto: кромка полки в толще
		// ствола). Остаётся тот, у кого общий участок по высоте длиннее; иначе
		// стык раздул бы ствол на пиксель-другой.
		if n := len(stems); n > 0 && st.l < stems[n-1].r {
			if st.ov > stems[n-1].ov {
				stems[n-1] = st
			}
			continue
		}
		stems = append(stems, st)
	}
	return stems
}

// stdStemTol — насколько (26.6) толщина штриха может отличаться от
// стандартной толщины шрифта на кегле, чтобы взять стандартную: треть
// пикселя.
const stdStemTol = 21

// stdStemRefs — эталонные буквы стандартной толщины вертикального ствола.
var stdStemRefs = []rune{'l', 'I', 'н', 'Н', '1'}

// stdStem — стандартная толщина вертикального ствола шрифта (26.6) по
// эталонной букве; 0 — эталона нет. ext сканирует контур буквы.
func stdStem(scan func(r rune) (*hintScan, bool), emPx int) int {
	for _, r := range stdStemRefs {
		s, ok := scan(r)
		if !ok {
			continue
		}
		s.closeContour()
		lefts, rights := splitVEdges(s)
		best := xStem{}
		for _, st := range pairStems(lefts, rights, emPx/4) {
			if st.ov > best.ov {
				best = st
			}
		}
		if best.ov > 0 {
			return best.r - best.l
		}
	}
	return 0
}

// stemWidth — выровненная толщина штриха: целое число пикселей, не меньше
// одного. Толщина, близкая к стандартной std (±треть пикселя), берётся
// стандартной: ствол «l» 1,47 px и бок «о» 1,52 px у Open Sans на 18 px иначе
// округлились бы в 1 и 2 px, и буквы одной строки вышли бы разного веса.
func stemWidth(s xStem, std int) int {
	w := s.r - s.l
	if std > 0 && absInt(w-std) <= stdStemTol {
		w = std
	}
	// Вверх уже с доли 0,4 пикселя (как высота строчных, hintXHeightBias):
	// ствол 1,47 px у Open Sans на 18 px в 1 px терял бы треть веса.
	if w = (w + 64 - hintXHeightBias) >> 6 << 6; w > 64 {
		return w
	}
	return 64
}

// stemGapTol — насколько (26.6) могут расходиться шаги между штрихами, чтобы
// считаться одинаковыми: четверть пикселя.
const stemGapTol = 16

// regularMaxShift — на сколько (26.6) середина штриха может уйти от исходной
// при выравнивании ряда с равным шагом: три четверти пикселя. Больше — ряд
// отменяется, штрихи выравниваются по одному (каждый не дальше полпикселя).
const regularMaxShift = 48

// regularStems выравнивает штрихи, идущие с одинаковым шагом («Ш», «ш», «m»,
// «Н», «п», «и»), как одну группу: шаг округляется до целого пикселя один раз,
// и просветы выходят равными, как у хинтинга Windows. Округление каждого
// штриха по отдельности давало у «Ш» на 11 px просветы 3 и 4 px. Группа
// ставится так, чтобы её середина ушла от исходной меньше чем на полпикселя.
// nil — шаги разные или выравнивание группой слепило бы штрихи; тогда
// singleStems.
func regularStems(stems []xStem) *vHint {
	if len(stems) < 2 {
		return nil
	}
	g0 := stems[1].l - stems[0].l
	sum, maxOv := 0, 0
	for _, s := range stems {
		maxOv = max(maxOv, s.ov)
	}
	for i := 1; i < len(stems); i++ {
		g := stems[i].l - stems[i-1].l
		if absInt(g-g0) > stemGapTol {
			return nil
		}
		sum += g
	}
	for _, s := range stems {
		// Группа — только из стволов сопоставимой высоты: у «ф» бока чаши
		// короткие, а ствол во всю высоту — это не ряд одинаковых штрихов.
		if s.ov*2 < maxOv {
			return nil
		}
	}
	step := round26(sum / (len(stems) - 1))
	// Положение группы: среднее по штрихам того, куда встал бы левый край
	// первого, если бы этот штрих стоял точно (по середине своей кромки).
	acc := 0
	for i, s := range stems {
		acc += (s.l+s.r)/2 - s.w/2 - i*step
	}
	base := round26(divRoundInt(acc, len(stems)))
	h := &vHint{}
	for i, s := range stems {
		w := s.w
		if w >= step {
			return nil // просвет схлопнулся бы
		}
		dl := base + i*step
		if absInt(dl+w/2-(s.l+s.r)/2) > regularMaxShift {
			return nil // ряд уехал бы слишком далеко — по одному
		}
		h.src = append(h.src, s.l, s.r)
		h.dst = append(h.dst, dl, dl+w)
	}
	for i := 1; i < len(h.src); i++ {
		if h.src[i] <= h.src[i-1] {
			return nil // штрихи перекрываются по X (друг над другом) — по одному
		}
	}
	if !h.strictlyIncreasing() {
		return nil
	}
	return h
}

// singleStems выравнивает штрихи по одному. Ширина и положение — от исходного
// контура, а не от уже выровненных соседей: иначе ошибки округления копятся
// слева направо, и у «ш», «m» правый край уезжает почти на пиксель —
// промежуток до соседней буквы заметно меняется. Так каждый штрих сдвигается
// не больше чем на полпикселя.
func singleStems(stems []xStem) *vHint {
	var cur *vHint // nil — тождественное отображение
	for _, s := range stems {
		if cur != nil && cur.nodeNear(s.l, 2) >= 0 && cur.nodeNear(s.r, 2) >= 0 {
			continue // штрих уже выровнен (общие кромки)
		}
		w := s.w
		dl := round26((s.l+s.r)/2 - w/2)
		var next *vHint
		if cur == nil {
			next = &vHint{src: []int{s.l, s.r}, dst: []int{dl, dl + w}}
		} else {
			next = cur.with(s.l, dl).with(s.r, dl+w)
		}
		if next.strictlyIncreasing() {
			cur = next
		}
	}
	return cur
}

// strictlyIncreasing — цели строго возрастают: штрихи не слиплись, просвет
// между ними не схлопнулся в ноль.
func (h *vHint) strictlyIncreasing() bool {
	for i := 1; i < len(h.dst); i++ {
		if h.dst[i] <= h.dst[i-1] {
			return false
		}
	}
	return true
}
