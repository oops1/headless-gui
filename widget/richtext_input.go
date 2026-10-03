package widget

// richtext_input.go — ввод RichText: выделение мышью, копирование, ссылки,
// прокрутка, курсор и семантика для скринридера.

import (
	"image"
	"math"
)

// ─── Выделение ──────────────────────────────────────────────────────────────

// selRangeLocked — границы выделения [lo, hi); равные — выделения нет.
func (t *RichText) selRangeLocked() (lo, hi int) {
	if t.selAnchor < 0 || t.selAnchor == t.selCaret {
		return 0, 0
	}
	lo, hi = t.selAnchor, t.selCaret
	if hi < lo {
		lo, hi = hi, lo
	}
	// Документ мог стать короче после выделения (SetParagraphs сбрасывает его,
	// но защита от выхода за срез дешевле паники в потоке кадра).
	n := len(t.doc.runes)
	if hi > n {
		hi = n
	}
	if lo > hi {
		lo = hi
	}
	return lo, hi
}

// SelectedText — выделенный текст (пусто, если выделения нет).
func (t *RichText) SelectedText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lo, hi := t.selRangeLocked()
	return string(t.doc.runes[lo:hi])
}

// Select выделяет [from, to) в рунах документа (границы усекаются);
// from == to снимает выделение.
func (t *RichText) Select(from, to int) {
	t.mu.Lock()
	from, to = accessClampPair(from, to, len(t.doc.runes))
	if from == to {
		t.selAnchor, t.selCaret = -1, from
	} else {
		t.selAnchor, t.selCaret = from, to
	}
	t.caretJumpedLocked()
	t.mu.Unlock()
	t.Invalidate()
}

// SelectAll выделяет весь текст.
func (t *RichText) SelectAll() {
	t.mu.Lock()
	n := len(t.doc.runes)
	t.mu.Unlock()
	t.Select(0, n)
}

// ClearSelection снимает выделение.
func (t *RichText) ClearSelection() { t.Select(0, 0) }

// Copy кладёт выделенное в буфер обмена: простой текст и HTML рядом. Простой
// нужен Блокноту и терминалу, HTML — Word и почтовику, где вставка сохранит
// шрифты, цвета и ссылки. false — выделения нет, буфер не тронут.
func (t *RichText) Copy() bool {
	t.mu.Lock()
	lo, hi := t.selRangeLocked()
	if lo == hi {
		t.mu.Unlock()
		return false
	}
	plain := string(t.doc.runes[lo:hi])
	html := richSelectionHTML(t.paras, t.doc, lo, hi)
	t.mu.Unlock()
	// В буфер — вне замка: платформенный буфер может ходить в ОС долго.
	SetClipboardHTML(html, plain)
	return true
}

// ─── Фокус ──────────────────────────────────────────────────────────────────

// SetFocused — Focusable. Фокус нужен ради клавиатуры: Ctrl+C, Ctrl+A,
// листание.
func (t *RichText) SetFocused(f bool) {
	t.mu.Lock()
	changed := t.focused != f
	t.focused = f
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

func (t *RichText) IsFocused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.focused
}

// ─── Мышь ───────────────────────────────────────────────────────────────────

// SetCaptureManager инжектится движком: протяжка выделения за границы виджета.
func (t *RichText) SetCaptureManager(cm CaptureManager) { t.capMgr = cm }

// WantsCapture — захватываем мышь на нажатие левой кнопки в границах: без
// захвата протяжка обрывалась бы, стоило курсору выйти за край.
func (t *RichText) WantsCapture(e MouseEvent) bool {
	return e.Button == MouseLeft && e.Pressed && image.Pt(e.X, e.Y).In(t.Base.Bounds())
}

// contentPointLocked — точка в координатах содержимого (от левого верха
// области текста с учётом прокрутки).
func (t *RichText) contentPointLocked(x, y int) (int, int) {
	b := t.Base.Bounds()
	return x - (b.Min.X + t.PaddingX), y - (b.Min.Y + t.PaddingY - t.scrollY)
}

// linkAtLocked — ссылка ровно под точкой (абсолютные координаты) или "".
func (t *RichText) linkAtLocked(x, y int) string {
	lay := t.layoutLocked()
	b := t.Base.Bounds()
	if !image.Pt(x, y).In(b) || (t.barOn && x >= b.Max.X-richBarW) {
		return ""
	}
	cx, cy := t.contentPointLocked(x, y)
	if s := lay.segmentAt(cx, cy); s != nil {
		return s.Run.Link
	}
	return ""
}

// wheelStepLocked — шаг колеса: три строки.
func (t *RichText) wheelStepLocked() int {
	lh := int(fontSizeOrDefault(t.FontSize)*1.5 + 0.5)
	if t.lay != nil && len(t.lay.Lines) > 0 {
		lh = t.lay.Lines[0].Height
	}
	return 3 * lh
}

// OnMouseButton обрабатывает кнопки и колесо.
func (t *RichText) OnMouseButton(e MouseEvent) bool {
	if !t.IsEnabled() {
		return false
	}
	b := t.Base.Bounds()
	inside := image.Pt(e.X, e.Y).In(b)

	// Колесо: листаем, если есть куда; иначе событие уходит родителю — иначе
	// RichText внутри прокручиваемой страницы «съедал» бы колесо.
	if e.Button == MouseWheelUp || e.Button == MouseWheelDown {
		if !inside {
			return false
		}
		t.mu.Lock()
		t.layoutLocked()
		if t.maxScrollLocked() == 0 {
			t.mu.Unlock()
			return false
		}
		if !e.Pressed { // движок шлёт нажатие и отпускание колеса парой
			t.mu.Unlock()
			return true
		}
		step := t.wheelStepLocked()
		if e.Button == MouseWheelUp {
			step = -step
		}
		changed := t.setScrollLocked(t.scrollY + step)
		t.mu.Unlock()
		if changed {
			t.Invalidate()
		}
		return true
	}
	if e.Button != MouseLeft {
		return false
	}

	if !e.Pressed {
		return t.mouseRelease(e)
	}
	if !inside {
		return false
	}

	t.mu.Lock()
	t.layoutLocked()

	// Полоса прокрутки — раньше текста: нажатие на неё не должно начинать
	// выделение под ней.
	if track, work, thumb := t.barGeomLocked(); !track.Empty() && image.Pt(e.X, e.Y).In(track) {
		changed := t.barPressLocked(e.Y, work, thumb)
		t.mu.Unlock()
		if changed {
			t.Invalidate()
		}
		return true
	}

	cx, cy := t.contentPointLocked(e.X, e.Y)
	off, eol := t.lay.hitCaret(t.measurer, cx, cy)
	n := clicksOf(e, &t.clicks)
	t.pressLink = ""
	switch {
	case n == 2:
		lo, hi := t.doc.wordBounds(off)
		t.setSelLocked(lo, hi)
		t.dragging = false
	case n >= 3:
		pi := t.doc.paraOf(off)
		t.setSelLocked(t.doc.paraStart[pi], t.doc.paraEnd(pi))
		t.dragging = false
	default:
		if e.Mod&ModShift != 0 {
			// Shift+щелчок: якорь остаётся, каретка переезжает под курсор.
			// Без выделения якорем служит прежняя каретка. Ссылка при этом
			// не открывается: так человек расширяет выделение, а не нажимает.
			if t.selAnchor < 0 {
				t.selAnchor = t.caretLocked()
			}
			t.selCaret = off
		} else {
			t.selAnchor, t.selCaret = off, off
			// Ссылка запоминается на нажатии: щелчком считается нажатие и
			// отпускание на одной и той же ссылке без протяжки.
			t.pressLink = t.linkAtLocked(e.X, e.Y)
		}
		t.dragging = true
		t.caretJumpedLocked()
		// Щелчок правее строки с мягким переносом оставляет каретку на ней.
		t.caretEOL = eol
	}
	t.mu.Unlock()
	t.Invalidate()
	return true
}

// setSelLocked ставит выделение [lo, hi); пустое — «нет выделения».
func (t *RichText) setSelLocked(lo, hi int) {
	t.caretJumpedLocked()
	if lo == hi {
		t.selAnchor, t.selCaret = -1, lo
		return
	}
	t.selAnchor, t.selCaret = lo, hi
}

// mouseRelease — отпускание левой кнопки: конец протяжки и, возможно, щелчок
// по ссылке.
func (t *RichText) mouseRelease(e MouseEvent) bool {
	t.mu.Lock()
	wasActive := t.dragging || t.barDrag
	t.dragging, t.barDrag = false, false
	if t.selAnchor == t.selCaret {
		t.selAnchor = -1
	}
	var url string
	if t.pressLink != "" && t.selAnchor < 0 && e.Clicks <= 1 {
		if t.linkAtLocked(e.X, e.Y) == t.pressLink {
			url = t.pressLink
		}
	}
	t.pressLink = ""
	t.mu.Unlock()
	if t.capMgr != nil {
		t.capMgr.ReleaseCapture()
	}
	if url != "" && t.OnLinkClick != nil {
		// Вне замка: обработчик вправе звать методы виджета.
		t.OnLinkClick(url)
	}
	t.Invalidate()
	return wasActive || url != ""
}

// barPressLocked — нажатие на полосу: на ползунке начинается перетаскивание
// без скачка; мимо него (на треке) текст прыгает так, чтобы ползунок встал
// серединой под курсор, и перетаскивание продолжается; на кнопках ▲▼
// классической темы — шаг. true — прокрутка сместилась.
func (t *RichText) barPressLocked(y int, work, thumb image.Rectangle) bool {
	if currentStyle().Classic3D {
		btn := classicSBBtnH(richBarW)
		track := t.Base.Bounds()
		const arrowStep = 40
		if y < track.Min.Y+btn {
			return t.setScrollLocked(t.scrollY - arrowStep)
		}
		if y >= track.Max.Y-btn {
			return t.setScrollLocked(t.scrollY + arrowStep)
		}
	}
	if thumb.Empty() {
		return false
	}
	t.dragging = false
	t.barDrag = true
	if y >= thumb.Min.Y && y < thumb.Max.Y {
		t.barGrab = y - thumb.Min.Y
		return false
	}
	t.barGrab = thumb.Dy() / 2
	return t.barDragToLocked(y)
}

// barDragToLocked ведёт ползунок за курсором: текст едет ровно на столько, на
// сколько протянули ползунок.
func (t *RichText) barDragToLocked(y int) bool {
	_, work, _ := t.barGeomLocked()
	b := t.Base.Bounds()
	v := hbarScrollForThumbX(transposeRect(work), y-t.barGrab, float64(t.maxScrollLocked()),
		float64(b.Dy()), float64(t.lay.Height+2*t.PaddingY))
	return t.setScrollLocked(int(math.Round(v)))
}

// OnMouseMove — протяжка выделения, перетаскивание ползунка, подсветка.
func (t *RichText) OnMouseMove(x, y int) {
	t.mu.Lock()
	t.layoutLocked()
	changed := false
	switch {
	case t.barDrag:
		changed = t.barDragToLocked(y)
	case t.dragging:
		b := t.Base.Bounds()
		// За верхним и нижним краем текст подъезжает сам: иначе выделить то,
		// что не помещается в виджет, было бы нечем.
		if y < b.Min.Y {
			t.setScrollLocked(t.scrollY - minInt((b.Min.Y-y)/2+1, 60))
		} else if y > b.Max.Y {
			t.setScrollLocked(t.scrollY + minInt((y-b.Max.Y)/2+1, 60))
		}
		cx, cy := t.contentPointLocked(x, y)
		off, eol := t.lay.hitCaret(t.measurer, cx, cy)
		if off != t.selCaret || eol != t.caretEOL {
			t.selCaret, t.caretEOL = off, eol
			t.wantXOk = false
			t.caretStamp = richNowMs()
		}
		changed = true
	default:
		_, _, thumb := t.barGeomLocked()
		hov := !thumb.Empty() && image.Pt(x, y).In(thumb)
		if hov != t.barHover {
			t.barHover = hov
			changed = true
		}
	}
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// OnMouseWheelPixels — плавная прокрутка точной дельтой (тачпад). dy > 0 —
// вниз. false — прокручивать нечего или упёрлись в край: событие уйдёт
// родителю.
func (t *RichText) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	if !image.Pt(x, y).In(t.Base.Bounds()) || dy == 0 {
		return false
	}
	t.mu.Lock()
	t.layoutLocked()
	maxS := t.maxScrollLocked()
	if maxS == 0 || (dy < 0 && t.scrollY <= 0) || (dy > 0 && t.scrollY >= maxS) {
		t.mu.Unlock()
		return false
	}
	t.scrollFrac += dy
	whole := math.Trunc(t.scrollFrac)
	t.scrollFrac -= whole
	changed := t.setScrollLocked(t.scrollY + int(whole))
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
	return true
}

// Cursor — рука над ссылкой, стрелка над полосой, I-образный над текстом
// (текст можно выделять).
func (t *RichText) Cursor(x, y int) Cursor {
	if !t.IsEnabled() {
		return CursorArrow
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.layoutLocked()
	b := t.Base.Bounds()
	if t.barOn && x >= b.Max.X-richBarW {
		return CursorArrow
	}
	if t.linkAtLocked(x, y) != "" {
		return CursorHand
	}
	return CursorIBeam
}

// ─── Клавиатура ─────────────────────────────────────────────────────────────

// OnKeyEvent: Ctrl+C / Ctrl+Insert — копировать, Ctrl+A — выделить всё;
// стрелки, Home/End, PgUp/PgDn — двигают каретку и выделение (с Shift — как
// расширение выделения), правила — в navigateLocked. После каждого движения
// каретка прокручивается в видимую область.
func (t *RichText) OnKeyEvent(e KeyEvent) {
	if !t.IsEnabled() || !e.Pressed {
		return
	}
	ctrl := e.Mod&ModCtrl != 0
	shift := e.Mod&ModShift != 0
	switch {
	case ctrl && (e.Code == KeyC || e.Code == KeyInsert):
		t.Copy()
		return
	case ctrl && e.Code == KeyA:
		t.SelectAll()
		return
	}
	// Alt+стрелка — «назад/вперёд» и прочие команды приложения, не навигация.
	if e.Mod&ModAlt != 0 {
		return
	}

	t.mu.Lock()
	t.layoutLocked()
	oldScroll, oldAnchor, oldCaret, oldEOL := t.scrollY, t.selAnchor, t.selCaret, t.caretEOL
	handled := t.navigateLocked(e.Code, ctrl, shift)
	changed := handled && (t.scrollY != oldScroll || t.selAnchor != oldAnchor ||
		t.selCaret != oldCaret || t.caretEOL != oldEOL)
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// ─── Доступность ────────────────────────────────────────────────────────────

// AccessInfo — роль «документ»: скринридер читает такой текст по строкам и
// абзацам и объявляет его документом, а не надписью.
func (t *RichText) AccessInfo() AccessInfo {
	info := AccessInfo{
		Role:        RoleDocument,
		Bounds:      t.Base.Bounds(),
		Description: t.GetToolTip(),
		Value:       t.Text(),
		States:      []string{StateReadOnly},
	}
	if !t.IsEnabled() {
		info.States = append(info.States, StateDisabled)
	}
	return info
}

// AccessText — весь текст документа; абзацы разделены '\n'.
func (t *RichText) AccessText() string { return t.Text() }

// AccessCaret — каретка: подвижный конец выделения. Есть и в режиме
// просмотра (когда она не рисуется): скринридеру нужна точка отсчёта.
func (t *RichText) AccessCaret() int { return t.CaretPosition() }

// AccessSelection — выделение; равные границы означают, что его нет.
func (t *RichText) AccessSelection() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if lo, hi := t.selRangeLocked(); lo != hi {
		return lo, hi
	}
	c := t.caretLocked()
	return c, c
}

// AccessReadOnly — текст только для чтения.
func (t *RichText) AccessReadOnly() bool { return true }

// AccessSetCaret — скринридер ставит каретку: выделение снимается, текст
// прокручивается к ней.
func (t *RichText) AccessSetCaret(pos int) bool {
	t.SetCaretPosition(pos)
	return true
}

// AccessSetSelection — скринридер выделяет диапазон: так диктор подсвечивает
// прочитанное слово.
func (t *RichText) AccessSetSelection(from, to int) bool {
	t.Select(from, to)
	return true
}
