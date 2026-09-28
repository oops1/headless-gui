package widget

import (
	"image"

	"testing"
)

// Длинную строку было видно обрезанной, а добраться до её конца — нечем:
// смещение вбок задавалось только горизонтальным колесом или жестом тачпада.
// Тесты держат новое: полоса под кодом, перетаскивание ползунка, Shift+колесо
// и подвод каретки по горизонтали.

const hbarLongLine = "x := (float64(px) + (float64(py) * 2.0)) / math.Sqrt(float64(w*w+h*h)) // очень длинная строка, которая заведомо не поместится в панель"

func TestHBarThumb(t *testing.T) {
	track := image.Rect(0, 0, 200, 8)

	// Содержимое помещается — полоса не нужна.
	if th := hbarThumb(track, 0, 0, 200, 150); !th.Empty() {
		t.Errorf("при maxScroll=0 ползунок %v, ждал пустой", th)
	}
	// Видна половина содержимого — ползунок в половину трека.
	th := hbarThumb(track, 0, 200, 200, 400)
	if th.Dx() != 100 || th.Min.X != 0 {
		t.Errorf("ползунок %v, ждал ширину 100 у левого края", th)
	}
	// Смещение до упора — ползунок у правого края.
	th = hbarThumb(track, 200, 200, 200, 400)
	if th.Max.X != track.Max.X {
		t.Errorf("при полном смещении ползунок %v, ждал у правого края %d", th, track.Max.X)
	}
	// Ползунок не ужимается в невидимую полоску, за которую не схватиться.
	th = hbarThumb(track, 0, 10000, 200, 20000)
	if th.Dx() < dvHBarMinW {
		t.Errorf("ползунок шириной %d, минимум %d", th.Dx(), dvHBarMinW)
	}
}

func TestHBarScrollAt(t *testing.T) {
	track := image.Rect(0, 0, 200, 8)
	const maxScroll, viewW, contentW = 200.0, 200.0, 400.0

	if got := hbarScrollAt(track, 0, maxScroll, viewW, contentW); got != 0 {
		t.Errorf("щелчок у левого края дал %v, ждал 0", got)
	}
	if got := hbarScrollAt(track, 200, maxScroll, viewW, contentW); got != maxScroll {
		t.Errorf("щелчок у правого края дал %v, ждал %v", got, maxScroll)
	}
	// Середина трека — середина содержимого.
	if got := hbarScrollAt(track, 100, maxScroll, viewW, contentW); got != 100 {
		t.Errorf("щелчок в середине дал %v, ждал 100", got)
	}
	// За пределы содержимого не уводит.
	if got := hbarScrollAt(track, 5000, maxScroll, viewW, contentW); got != maxScroll {
		t.Errorf("щелчок далеко справа дал %v, ждал %v", got, maxScroll)
	}
}

// Перетаскивание ведёт ползунок за курсором: содержимое едет ровно на
// столько, на сколько протянули, без скачка под курсор.
func TestHBarScrollForThumbX(t *testing.T) {
	track := image.Rect(0, 0, 200, 8)
	const maxScroll, viewW, contentW = 200.0, 200.0, 400.0

	if got := hbarScrollForThumbX(track, 0, maxScroll, viewW, contentW); got != 0 {
		t.Errorf("левый край ползунка в нуле дал %v, ждал 0", got)
	}
	if got := hbarScrollForThumbX(track, 50, maxScroll, viewW, contentW); got != 100 {
		t.Errorf("левый край ползунка на 50 дал %v, ждал 100 (полтрека = полсодержимого)", got)
	}
	if got := hbarScrollForThumbX(track, 100, maxScroll, viewW, contentW); got != maxScroll {
		t.Errorf("левый край ползунка в конце дал %v, ждал %v", got, maxScroll)
	}
}

// ─── DiffView ───────────────────────────────────────────────────────────────

func diffWithLongLine(t *testing.T) *DiffView {
	t.Helper()
	d := NewDiffView("", "")
	d.SetBounds(image.Rect(0, 0, 900, 500))
	d.SetText(DiffLeft, "old", "", "короткая\n"+hbarLongLine+"\nещё\n")
	d.SetText(DiffRight, "new", "", "короткая\nдругая\nещё\n")
	return d
}

func TestDiffView_HBarAppearsForLongLine(t *testing.T) {
	d := diffWithLongLine(t)
	d.mu.Lock()
	defer d.mu.Unlock()

	g := d.geom()
	if !d.needHBarLocked(g) {
		t.Fatal("длинная строка не помещается, а полосы нет")
	}
	if d.maxHScrollLocked() <= 0 {
		t.Error("запас прокрутки нулевой")
	}
	tr := d.hbarTrack(g)
	if tr.Empty() || d.hbarThumbLocked(g).Empty() {
		t.Errorf("трек %v или ползунок пусты", tr)
	}
	// Полоса не наезжает на код: та высота, что ей досталась, у кода отнята.
	if tr.Min.Y < g.cy1 {
		t.Errorf("полоса начинается на %d, а код кончается на %d", tr.Min.Y, g.cy1)
	}
	if tr.Max.Y > d.Bounds().Max.Y {
		t.Errorf("полоса вылезла за границы виджета: %v при %v", tr, d.Bounds())
	}
}

func TestDiffView_NoHBarForShortLines(t *testing.T) {
	d := NewDiffView("", "")
	d.SetBounds(image.Rect(0, 0, 900, 500))
	d.SetText(DiffLeft, "old", "", "раз\nдва\n")
	d.SetText(DiffRight, "new", "", "раз\nтри\n")

	d.mu.Lock()
	defer d.mu.Unlock()
	g := d.geom()
	if d.needHBarLocked(g) {
		t.Error("короткие строки помещаются, полоса не нужна")
	}
	if !d.hbarTrack(g).Empty() {
		t.Error("трек полосы построен, хотя прокручивать нечего")
	}
	// Вся высота досталась коду.
	if g.cy1 != d.Bounds().Max.Y {
		t.Errorf("код кончается на %d, ждал %d", g.cy1, d.Bounds().Max.Y)
	}
}

func TestDiffView_ShiftWheelScrollsSideways(t *testing.T) {
	d := diffWithLongLine(t)

	d.OnMouseWheelPixelsMod(100, 200, 0, 40, ModShift)
	d.mu.Lock()
	h := d.hscroll
	v := d.scroll
	d.mu.Unlock()
	if h <= 0 {
		t.Errorf("Shift+колесо не увело код вбок: hscroll=%v", h)
	}
	if v != 0 {
		t.Errorf("Shift+колесо прокрутило и по вертикали: scroll=%v", v)
	}

	// Без Shift колесо по-прежнему листает вниз.
	d.OnMouseWheelPixelsMod(100, 200, 0, 40, 0)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hscroll != h {
		t.Errorf("колесо без Shift сдвинуло код вбок: %v → %v", h, d.hscroll)
	}
}

func TestDiffView_HBarDragMovesCode(t *testing.T) {
	d := diffWithLongLine(t)
	d.mu.Lock()
	g := d.geom()
	tr := d.hbarTrack(g)
	th := d.hbarThumbLocked(g)
	d.mu.Unlock()
	if tr.Empty() || th.Empty() {
		t.Fatal("полосы нет — тест бесполезен")
	}

	// Хватаем ползунок за левый край и тянем вправо.
	y := tr.Min.Y + tr.Dy()/2
	d.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: true, X: th.Min.X + 1, Y: y})
	d.OnMouseMove(th.Min.X+1+40, y)

	d.mu.Lock()
	h := d.hscroll
	d.mu.Unlock()
	if h <= 0 {
		t.Fatalf("перетаскивание ползунка не сдвинуло код: hscroll=%v", h)
	}
	d.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: false, X: th.Min.X + 41, Y: y})

	// После отпускания движение мыши код больше не двигает.
	d.OnMouseMove(th.Min.X+200, y)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hscroll != h {
		t.Errorf("код поехал после отпускания: %v → %v", h, d.hscroll)
	}
}

// Щелчок мимо ползунка переносит видимую часть туда, куда ткнули.
func TestDiffView_HBarClickJumps(t *testing.T) {
	d := diffWithLongLine(t)
	d.mu.Lock()
	g := d.geom()
	tr := d.hbarTrack(g)
	d.mu.Unlock()
	if tr.Empty() {
		t.Fatal("полосы нет — тест бесполезен")
	}

	d.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: true,
		X: tr.Max.X - 2, Y: tr.Min.Y + tr.Dy()/2})
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hscroll < d.maxHScrollLocked()-1 {
		t.Errorf("щелчок у правого края дал hscroll=%v, ждал около %v",
			d.hscroll, d.maxHScrollLocked())
	}
}

// ─── MergeView ──────────────────────────────────────────────────────────────

func mergeWithLongLine(t *testing.T) *MergeView {
	t.Helper()
	m := NewMergeView("", "")
	m.SetBounds(image.Rect(0, 0, 1000, 600))
	// SetTexts(base, ours, theirs): длинная строка — в нашей стороне, она же
	// уедет в итог.
	m.SetTexts(
		"общая\nстрока\n",
		"общая\n"+hbarLongLine+"\n",
		"общая\nдругая\n",
	)
	return m
}

func TestMergeView_HScrollMoves(t *testing.T) {
	m := mergeWithLongLine(t)

	m.mu.Lock()
	maxH := m.maxHScrollLocked()
	m.mu.Unlock()
	if maxH <= 0 {
		t.Fatal("запас прокрутки нулевой, хотя строка заведомо длиннее панели")
	}

	// Раньше hscroll не менялся ничем: поле читалось в отрисовке, но ему
	// нигде не присваивалось значение.
	m.OnMouseWheelPixelsMod(200, 200, 0, 40, ModShift)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hscroll <= 0 {
		t.Errorf("Shift+колесо не увело код вбок: hscroll=%v", m.hscroll)
	}
}

func TestMergeView_HBarGeometry(t *testing.T) {
	m := mergeWithLongLine(t)
	m.mu.Lock()
	defer m.mu.Unlock()

	g := m.geom()
	tr := m.hbarTrackLocked(g)
	if tr.Empty() || m.hbarThumbLocked(g).Empty() {
		t.Fatalf("трек %v или ползунок пусты", tr)
	}
	if tr.Min.Y < g.ry1 {
		t.Errorf("полоса начинается на %d, а итог кончается на %d", tr.Min.Y, g.ry1)
	}
	if tr.Max.Y > m.Bounds().Max.Y {
		t.Errorf("полоса вылезла за границы виджета: %v при %v", tr, m.Bounds())
	}

	// Короткие строки — полосы нет.
	m.mu.Unlock()
	short := NewMergeView("", "")
	short.SetBounds(image.Rect(0, 0, 1000, 600))
	short.SetTexts("раз\n", "раз\n", "раз\n")
	short.mu.Lock()
	need := short.needHBarLocked(short.geom())
	short.mu.Unlock()
	m.mu.Lock()
	if need {
		t.Error("короткие строки помещаются, полоса не нужна")
	}
}

func TestMergeView_HBarDragMovesCode(t *testing.T) {
	m := mergeWithLongLine(t)
	m.mu.Lock()
	g := m.geom()
	tr := m.hbarTrackLocked(g)
	th := m.hbarThumbLocked(g)
	m.mu.Unlock()
	if tr.Empty() || th.Empty() {
		t.Fatal("полосы нет — тест бесполезен")
	}

	y := tr.Min.Y + tr.Dy()/2
	m.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: true, X: th.Min.X + 1, Y: y})
	m.OnMouseMove(th.Min.X+1+60, y)
	m.mu.Lock()
	h := m.hscroll
	m.mu.Unlock()
	if h <= 0 {
		t.Fatalf("перетаскивание ползунка не сдвинуло код: hscroll=%v", h)
	}
	m.OnMouseButton(MouseEvent{Button: MouseLeft, Pressed: false, X: th.Min.X + 61, Y: y})
	m.OnMouseMove(th.Min.X+250, y)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hscroll != h {
		t.Errorf("код поехал после отпускания: %v → %v", h, m.hscroll)
	}
}

// Каретка, уехавшая за правый край, подтягивает смещение за собой: печатать
// вслепую за краем панели нельзя.
func TestMergeView_CaretPullsHScroll(t *testing.T) {
	m := mergeWithLongLine(t)

	m.mu.Lock()
	defer m.mu.Unlock()
	before := m.hscroll
	s := m.docs[MergeResult]
	line := -1
	for i, l := range s.text.Lines {
		if len([]rune(l)) > 60 {
			line = i
			break
		}
	}
	if line < 0 {
		t.Fatal("длинная строка не попала в итог — тест бесполезен")
	}
	s.caret = dvPos{line, len([]rune(s.text.Lines[line]))}
	m.ensureColVisibleLocked(MergeResult)

	if m.hscroll <= before {
		t.Errorf("каретка в конце длинной строки не сдвинула код: %v → %v", before, m.hscroll)
	}
}

// Смещение общее для всех панелей: строки сопоставлены построчно, и
// разъехавшиеся вбок панели читать невозможно.
func TestMergeView_HScrollSharedByPanes(t *testing.T) {
	m := mergeWithLongLine(t)
	m.OnMouseWheelPixelsMod(200, 200, 0, 60, ModShift)

	m.mu.Lock()
	defer m.mu.Unlock()
	h := m.hscroll
	if h <= 0 {
		t.Fatal("код не сдвинулся")
	}
	// Перевод экранной точки в колонку у каждой панели учитывает то же
	// смещение — иначе клик попадал бы не по той букве.
	g := m.geom()
	for _, side := range []MergeSide{MergeOurs, MergeTheirs, MergeResult} {
		_, _, codeX := m.paneCodeXLocked(g, side)
		y := g.ty0 + dvLineH/2
		if side == MergeResult {
			y = g.ry0 + dvLineH/2
		}
		pos := m.hitPosLocked(side, codeX+1, y)
		if pos.col == 0 {
			t.Errorf("панель %v не учла смещение: под левым краем колонка 0", side)
		}
	}
}

// End уводит каретку в конец длинной строки, а с ней и смещение вбок;
// Home возвращает обратно. На мыши без горизонтального колеса клавиатура —
// второй способ добраться до конца строки.
func TestDiffView_HomeEndMovesHScroll(t *testing.T) {
	d := diffWithLongLine(t)
	d.SetActiveSide(DiffLeft)
	d.SetCaret(DiffLeft, 1, 0)
	d.SetFocused(true)

	d.OnKeyEvent(KeyEvent{Code: KeyEnd, Pressed: true})
	d.mu.Lock()
	end := d.hscroll
	d.mu.Unlock()
	if end <= 0 {
		t.Fatalf("End не увёл код вбок: hscroll=%v", end)
	}

	d.OnKeyEvent(KeyEvent{Code: KeyHome, Pressed: true})
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hscroll != 0 {
		t.Errorf("Home не вернул код к началу строки: hscroll=%v", d.hscroll)
	}
}

func TestMergeView_HomeEndMovesHScroll(t *testing.T) {
	m := mergeWithLongLine(t)
	m.SetFocused(true)

	// Каретка — в длинной строке итога.
	m.mu.Lock()
	s := m.docs[MergeResult]
	line := -1
	for i, l := range s.text.Lines {
		if len([]rune(l)) > 60 {
			line = i
			break
		}
	}
	if line < 0 {
		m.mu.Unlock()
		t.Fatal("длинная строка не попала в итог — тест бесполезен")
	}
	m.active = MergeResult
	s.caret = dvPos{line, 0}
	m.mu.Unlock()

	m.OnKeyEvent(KeyEvent{Code: KeyEnd, Pressed: true})
	m.mu.Lock()
	end := m.hscroll
	m.mu.Unlock()
	if end <= 0 {
		t.Fatalf("End не увёл код вбок: hscroll=%v", end)
	}

	m.OnKeyEvent(KeyEvent{Code: KeyHome, Pressed: true})
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hscroll != 0 {
		t.Errorf("Home не вернул код к началу строки: hscroll=%v", m.hscroll)
	}
}
