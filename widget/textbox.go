package widget

import (
	"image"
	"image/color"
	"sync"
	"time"
)

// TextBox — многострочный текстовый редактор (WPF TextBox c
// AcceptsReturn="True"). Дополняет однострочный TextInput.
//
// Поддерживает:
//   - Перенос по словам (Wrap = TextWrapping="Wrap") либо горизонтальный скролл
//   - Вертикальный скролл: колесо мыши, PgUp/PgDn, тонкий индикатор
//   - Каретка, выделение мышью (drag), Shift+навигация, двойной клик — слово
//   - Стрелки, Ctrl+стрелки (по словам), Home/End, Ctrl+Home/End (документ)
//   - Ctrl+A/C/X/V, Ctrl+Z / Ctrl+Y (undo/redo)
//   - Enter — перевод строки; OnChange при каждом изменении
//   - Контекстное меню (Cut/Copy/Paste/Select All)
//   - Темизация (ApplyTheme), headless-ввод через engine.SendKeyEvent
//
// Компоновка текста (разбиение на строки) считается через MeasureUIText —
// работает и вне Draw (клавиатура/мышь), и в headless-режиме.
type TextBox struct {
	Base

	mu        sync.Mutex
	runes     []rune
	caret     int // позиция вставки (индекс в runes)
	selAnchor int // якорь выделения (-1 = нет); выделение = [min,max)(anchor, caret)
	scrollY   int // вертикальный сдвиг, px
	scrollX   int // горизонтальный сдвиг, px (используется только при Wrap=false)
	desiredX  int // целевая X (px) для Up/Down; -1 = не задана

	// Кэш компоновки: границы строк для текущего текста и ширины.
	lines   []tbLine
	layoutW int  // ширина текстовой области, для которой посчитан кэш
	dirty   bool // текст изменился — кэш недействителен

	dragging bool
	capMgr   CaptureManager

	contextMenu *PopupMenu

	lastClickMs int64
	lastClickX  int
	lastClickY  int

	undoStack []textEdit
	redoStack []textEdit

	// Wrap — переносить строки по словам (TextWrapping="Wrap").
	// false — длинные строки уходят вправо (горизонтальный скролл за кареткой).
	Wrap bool
	// ReadOnly — запрет редактирования (навигация и копирование работают).
	ReadOnly bool

	Placeholder string

	Background  color.RGBA
	BorderColor color.RGBA
	FocusBorder color.RGBA
	TextColor   color.RGBA
	PlaceColor  color.RGBA
	CaretColor  color.RGBA
	SelColor    color.RGBA

	PaddingX int
	PaddingY int

	FontSize float64 // pt (0 → DefaultFontSizePt)

	focused bool

	// OnChange вызывается при каждом изменении текста.
	OnChange func(text string)
}

// tbLine — одна визуальная строка: полуинтервал рун [start, end).
// Завершающий '\n' (если есть) в интервал не входит.
type tbLine struct {
	start, end int
}

// NewTextBox создаёт многострочный редактор с переносом по словам.
func NewTextBox(placeholder string) *TextBox {
	return &TextBox{
		Placeholder: placeholder,
		Wrap:        true,
		Background:  win10.InputBG,
		BorderColor: win10.InputBorder,
		FocusBorder: win10.InputFocus,
		TextColor:   win10.InputText,
		PlaceColor:  win10.InputPlaceholder,
		CaretColor:  win10.InputCaret,
		SelColor:    premulAlpha(win10.Accent, 110),
		PaddingX:    6,
		PaddingY:    4,
		selAnchor:   -1,
		desiredX:    -1,
		dirty:       true,
	}
}

// premulAlpha возвращает корректный premultiplied-цвет: c с прозрачностью a.
// color.RGBA в Go — альфа-премультиплицированный; каналы, превышающие альфу,
// при Over-блендинге переполняются (артефакт «мадженты» на светлом фоне).
func premulAlpha(c color.RGBA, a uint8) color.RGBA {
	m := uint32(a)
	return color.RGBA{
		R: uint8(uint32(c.R) * m / 255),
		G: uint8(uint32(c.G) * m / 255),
		B: uint8(uint32(c.B) * m / 255),
		A: a,
	}
}

// ─── Текст ───────────────────────────────────────────────────────────────────

// SetText устанавливает содержимое (каретка — в конец, скролл — в начало).
func (t *TextBox) SetText(text string) {
	t.mu.Lock()
	runes := []rune(text)
	changed := string(t.runes) != text
	t.runes = runes
	t.caret = len(runes)
	t.selAnchor = -1
	t.scrollY = 0
	t.scrollX = 0
	t.desiredX = -1
	t.dirty = true
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// GetText возвращает текущее содержимое.
func (t *TextBox) GetText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.runes)
}

// SelectedText возвращает выделенный фрагмент ("" если выделения нет).
func (t *TextBox) SelectedText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.selActive() {
		return ""
	}
	lo, hi := t.normSel()
	return string(t.runes[lo:hi])
}

// CaretPosition возвращает позицию каретки (индекс в рунах).
func (t *TextBox) CaretPosition() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.caret
}

// LineCount возвращает число визуальных строк при текущей ширине.
func (t *TextBox) LineCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureLayout()
	return len(t.lines)
}

// ScrollTop возвращает вертикальный сдвиг в пикселях (для автоматизации).
func (t *TextBox) ScrollTop() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scrollY
}

// Cursor — текстовый курсор (I-beam) над редактором.
func (t *TextBox) Cursor(x, y int) Cursor {
	if t.IsEnabled() {
		return CursorIBeam
	}
	return CursorArrow
}

// ─── Focusable / Animated ───────────────────────────────────────────────────

func (t *TextBox) SetFocused(focused bool) {
	t.mu.Lock()
	changed := t.focused != focused
	t.focused = focused
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

func (t *TextBox) IsFocused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.focused
}

// NeedsAnimation — пока редактор в фокусе, мигает каретка.
func (t *TextBox) NeedsAnimation() bool { return t.IsFocused() }

// ─── Геометрия и компоновка ──────────────────────────────────────────────────

const tbScrollbarW = 7 // зона тонкого вертикального скроллбара

func (t *TextBox) fontSize() float64 {
	if t.FontSize > 0 {
		return t.FontSize
	}
	return DefaultFontSizePt
}

// lineHeight — высота визуальной строки в px.
func (t *TextBox) lineHeight() int {
	return int(t.fontSize()*1.6) + 3
}

// textAreaW — ширина области текста (внутри рамки, без скроллбара).
func (t *TextBox) textAreaW() int {
	w := t.bounds.Dx() - 2*t.PaddingX - tbScrollbarW
	if w < 20 {
		w = 20
	}
	return w
}

// visibleLines — сколько строк помещается по высоте.
func (t *TextBox) visibleLines() int {
	n := (t.bounds.Dy() - 2*t.PaddingY) / t.lineHeight()
	if n < 1 {
		n = 1
	}
	return n
}

// ensureLayout перекомпоновывает строки при изменении текста или ширины.
// Вызывать под t.mu.
func (t *TextBox) ensureLayout() {
	w := t.textAreaW()
	if !t.dirty && t.layoutW == w && t.lines != nil {
		return
	}
	t.layoutW = w
	t.dirty = false
	t.lines = t.lines[:0]

	fs := t.fontSize()
	n := len(t.runes)
	parStart := 0
	for i := 0; i <= n; i++ {
		if i < n && t.runes[i] != '\n' {
			continue
		}
		// Параграф [parStart, i)
		if !t.Wrap {
			t.lines = append(t.lines, tbLine{start: parStart, end: i})
		} else {
			t.wrapParagraph(parStart, i, w, fs)
		}
		parStart = i + 1
	}
	if len(t.lines) == 0 {
		t.lines = []tbLine{{}}
	}
}

// wrapParagraph разбивает параграф [start, end) на строки шириной ≤ maxW px:
// перенос по пробелам, слишком длинные слова режутся по символам.
// Вызывать под t.mu (внутри ensureLayout).
func (t *TextBox) wrapParagraph(start, end, maxW int, fs float64) {
	if start >= end {
		t.lines = append(t.lines, tbLine{start: start, end: end})
		return
	}
	lineStart := start
	lastSpace := -1 // индекс последнего пробела в текущей строке
	for i := start; i < end; i++ {
		if t.runes[i] == ' ' {
			lastSpace = i
		}
		w := MeasureUIText(string(t.runes[lineStart:i+1]), fs)
		if w <= maxW {
			continue
		}
		// Символ i не влезает.
		switch {
		case lastSpace > lineStart:
			// Перенос по последнему пробелу; пробел остаётся в верхней строке.
			t.lines = append(t.lines, tbLine{start: lineStart, end: lastSpace})
			lineStart = lastSpace + 1
			lastSpace = -1
			// Каретка цикла остаётся на i: пересчитаем ширину от нового начала.
		case i > lineStart:
			// Одно слово шире области — режем по символам.
			t.lines = append(t.lines, tbLine{start: lineStart, end: i})
			lineStart = i
			lastSpace = -1
		default:
			// Даже один символ не влезает — кладём его целиком.
			t.lines = append(t.lines, tbLine{start: lineStart, end: i + 1})
			lineStart = i + 1
			lastSpace = -1
		}
	}
	t.lines = append(t.lines, tbLine{start: lineStart, end: end})
}

// caretLine возвращает индекс строки, содержащей каретку.
// Вызывать под t.mu (после ensureLayout).
func (t *TextBox) caretLine() int {
	for i, ln := range t.lines {
		if t.caret >= ln.start && t.caret <= ln.end {
			return i
		}
	}
	return len(t.lines) - 1
}

// lineTextW возвращает ширину префикса строки li длиной col рун (px).
// Вызывать под t.mu.
func (t *TextBox) lineTextW(li, col int) int {
	ln := t.lines[li]
	if col < 0 {
		col = 0
	}
	if col > ln.end-ln.start {
		col = ln.end - ln.start
	}
	return MeasureUIText(string(t.runes[ln.start:ln.start+col]), t.fontSize())
}

// colAtX возвращает колонку (0..len) строки li, ближайшую к px x.
// Вызывать под t.mu.
func (t *TextBox) colAtX(li, x int) int {
	ln := t.lines[li]
	length := ln.end - ln.start
	prev := 0
	for c := 1; c <= length; c++ {
		w := t.lineTextW(li, c)
		if x < (prev+w)/2 {
			return c - 1
		}
		prev = w
	}
	return length
}

// caretPoint возвращает (строка, x px) каретки. Вызывать под t.mu.
func (t *TextBox) caretPoint() (line, x int) {
	li := t.caretLine()
	return li, t.lineTextW(li, t.caret-t.lines[li].start)
}

// ensureCaretVisible прокручивает так, чтобы каретка была видима.
// Вызывать под t.mu (после ensureLayout).
func (t *TextBox) ensureCaretVisible() {
	lh := t.lineHeight()
	li, cx := t.caretPoint()
	top := li * lh
	viewH := t.visibleLines() * lh
	if top < t.scrollY {
		t.scrollY = top
	}
	if top+lh > t.scrollY+viewH {
		t.scrollY = top + lh - viewH
	}
	t.clampScroll()

	if !t.Wrap {
		w := t.textAreaW()
		if cx-t.scrollX > w-4 {
			t.scrollX = cx - w + 4
		}
		if cx-t.scrollX < 0 {
			t.scrollX = cx
		}
		if t.scrollX < 0 {
			t.scrollX = 0
		}
	}
}

// clampScroll ограничивает scrollY содержимым. Вызывать под t.mu.
func (t *TextBox) clampScroll() {
	lh := t.lineHeight()
	maxScroll := len(t.lines)*lh - t.visibleLines()*lh
	if maxScroll < 0 {
		maxScroll = 0
	}
	if t.scrollY > maxScroll {
		t.scrollY = maxScroll
	}
	if t.scrollY < 0 {
		t.scrollY = 0
	}
}

// charIndexAtPoint возвращает позицию каретки для точки (абс. координаты).
// Вызывать под t.mu.
func (t *TextBox) charIndexAtPoint(absX, absY int) int {
	t.ensureLayout()
	b := t.bounds
	lh := t.lineHeight()
	li := (absY - b.Min.Y - t.PaddingY + t.scrollY) / lh
	if li < 0 {
		li = 0
	}
	if li >= len(t.lines) {
		li = len(t.lines) - 1
	}
	x := absX - (b.Min.X + t.PaddingX) + t.scrollX
	col := t.colAtX(li, x)
	return t.lines[li].start + col
}

// ─── Выделение / правка (helpers) ────────────────────────────────────────────

func (t *TextBox) selActive() bool { return t.selAnchor >= 0 && t.selAnchor != t.caret }

func (t *TextBox) normSel() (lo, hi int) {
	if t.selAnchor <= t.caret {
		return t.selAnchor, t.caret
	}
	return t.caret, t.selAnchor
}

// deleteSel удаляет выделенное. Возвращает true, если выделение было.
func (t *TextBox) deleteSel() bool {
	if !t.selActive() {
		return false
	}
	lo, hi := t.normSel()
	t.runes = append(t.runes[:lo], t.runes[hi:]...)
	t.caret = lo
	t.selAnchor = -1
	t.dirty = true
	return true
}

// insertRunes вставляет rs в позицию каретки (учитывая выделение).
func (t *TextBox) insertRunes(rs []rune) {
	t.deleteSel()
	if len(rs) == 0 {
		return
	}
	ins := make([]rune, len(t.runes)+len(rs))
	copy(ins, t.runes[:t.caret])
	copy(ins[t.caret:], rs)
	copy(ins[t.caret+len(rs):], t.runes[t.caret:])
	t.runes = ins
	t.caret += len(rs)
	t.dirty = true
}

func (t *TextBox) clampCaret() {
	if t.caret < 0 {
		t.caret = 0
	}
	if t.caret > len(t.runes) {
		t.caret = len(t.runes)
	}
}

// moveCaret переносит каретку в pos, поддерживая якорь при extend (Shift).
func (t *TextBox) moveCaret(pos int, extend bool) {
	if extend {
		if t.selAnchor < 0 {
			t.selAnchor = t.caret
		}
	} else {
		t.selAnchor = -1
	}
	t.caret = pos
	t.clampCaret()
}

// wordLeft возвращает позицию начала предыдущего слова.
func (t *TextBox) wordLeft(idx int) int {
	if idx <= 0 {
		return 0
	}
	i := idx - 1
	for i > 0 && !isWordRune(t.runes[i]) {
		i--
	}
	for i > 0 && isWordRune(t.runes[i-1]) {
		i--
	}
	return i
}

// wordRight возвращает позицию за концом следующего слова.
func (t *TextBox) wordRight(idx int) int {
	n := len(t.runes)
	i := idx
	for i < n && isWordRune(t.runes[i]) {
		i++
	}
	for i < n && !isWordRune(t.runes[i]) {
		i++
	}
	return i
}

func (t *TextBox) pushUndo(before textEdit) {
	t.undoStack = append(t.undoStack, before)
	if len(t.undoStack) > 200 {
		t.undoStack = t.undoStack[1:]
	}
	t.redoStack = nil
}

func (t *TextBox) undo() {
	if len(t.undoStack) == 0 {
		return
	}
	cur := textEdit{text: string(t.runes), caret: t.caret}
	last := t.undoStack[len(t.undoStack)-1]
	t.undoStack = t.undoStack[:len(t.undoStack)-1]
	t.redoStack = append(t.redoStack, cur)
	t.runes = []rune(last.text)
	t.caret = last.caret
	t.selAnchor = -1
	t.dirty = true
}

func (t *TextBox) redo() {
	if len(t.redoStack) == 0 {
		return
	}
	cur := textEdit{text: string(t.runes), caret: t.caret}
	next := t.redoStack[len(t.redoStack)-1]
	t.redoStack = t.redoStack[:len(t.redoStack)-1]
	t.undoStack = append(t.undoStack, cur)
	t.runes = []rune(next.text)
	t.caret = next.caret
	t.selAnchor = -1
	t.dirty = true
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (t *TextBox) OnKeyEvent(e KeyEvent) {
	if !t.IsEnabled() || !e.Pressed {
		return
	}
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.OnKeyEvent(e)
		return
	}

	ctrl := e.Mod&ModCtrl != 0
	shift := e.Mod&ModShift != 0

	t.mu.Lock()
	t.ensureLayout()

	changed := false
	isUndoRedo := false
	keepDesiredX := false
	before := textEdit{text: string(t.runes), caret: t.caret}
	caret0, sel0, scr0, scr0x := t.caret, t.selAnchor, t.scrollY, t.scrollX

	switch e.Code {
	case KeyLeft:
		pos := t.caret
		switch {
		case ctrl:
			pos = t.wordLeft(t.caret)
		case !shift && t.selActive():
			pos, _ = t.normSel()
		case t.caret > 0:
			pos = t.caret - 1
		}
		t.moveCaret(pos, shift)

	case KeyRight:
		pos := t.caret
		switch {
		case ctrl:
			pos = t.wordRight(t.caret)
		case !shift && t.selActive():
			_, pos = t.normSel()
		case t.caret < len(t.runes):
			pos = t.caret + 1
		}
		t.moveCaret(pos, shift)

	case KeyUp, KeyDown:
		keepDesiredX = true
		li, cx := t.caretPoint()
		if t.desiredX < 0 {
			t.desiredX = cx
		}
		if e.Code == KeyUp {
			li--
		} else {
			li++
		}
		if li >= 0 && li < len(t.lines) {
			col := t.colAtX(li, t.desiredX)
			t.moveCaret(t.lines[li].start+col, shift)
		} else if !shift {
			t.selAnchor = -1
		}

	case KeyPageUp, KeyPageDown:
		keepDesiredX = true
		li, cx := t.caretPoint()
		if t.desiredX < 0 {
			t.desiredX = cx
		}
		page := t.visibleLines()
		if e.Code == KeyPageUp {
			li -= page
		} else {
			li += page
		}
		if li < 0 {
			li = 0
		}
		if li >= len(t.lines) {
			li = len(t.lines) - 1
		}
		col := t.colAtX(li, t.desiredX)
		t.moveCaret(t.lines[li].start+col, shift)

	case KeyHome:
		if ctrl {
			t.moveCaret(0, shift)
		} else {
			li := t.caretLine()
			t.moveCaret(t.lines[li].start, shift)
		}

	case KeyEnd:
		if ctrl {
			t.moveCaret(len(t.runes), shift)
		} else {
			li := t.caretLine()
			t.moveCaret(t.lines[li].end, shift)
		}

	case KeyBackspace:
		if t.ReadOnly {
			break
		}
		if t.deleteSel() {
			changed = true
		} else if t.caret > 0 {
			t.runes = append(t.runes[:t.caret-1], t.runes[t.caret:]...)
			t.caret--
			t.dirty = true
			changed = true
		}

	case KeyDelete:
		if t.ReadOnly {
			break
		}
		if t.deleteSel() {
			changed = true
		} else if t.caret < len(t.runes) {
			t.runes = append(t.runes[:t.caret], t.runes[t.caret+1:]...)
			t.dirty = true
			changed = true
		}

	case KeyEnter:
		if !t.ReadOnly {
			t.insertRunes([]rune{'\n'})
			changed = true
		}

	default:
		if ctrl {
			switch e.Code {
			case KeyZ:
				if t.ReadOnly {
					break
				}
				if shift {
					t.redo()
				} else {
					t.undo()
				}
				isUndoRedo = true
				changed = true
			case KeyY:
				if t.ReadOnly {
					break
				}
				t.redo()
				isUndoRedo = true
				changed = true
			case KeyA:
				t.selAnchor = 0
				t.caret = len(t.runes)
			case KeyC:
				if t.selActive() {
					lo, hi := t.normSel()
					ClipboardSetText(string(t.runes[lo:hi]))
				}
			case KeyX:
				if !t.ReadOnly && t.selActive() {
					lo, hi := t.normSel()
					ClipboardSetText(string(t.runes[lo:hi]))
					t.deleteSel()
					changed = true
				}
			case KeyV:
				if t.ReadOnly {
					break
				}
				if clip := ClipboardGetText(); clip != "" {
					t.insertRunes([]rune(clip))
					changed = true
				}
			}
		} else if e.Rune >= 32 && !t.ReadOnly {
			t.insertRunes([]rune{e.Rune})
			changed = true
		}
	}

	if changed && !isUndoRedo {
		t.pushUndo(before)
	}
	if !keepDesiredX {
		t.desiredX = -1
	}
	t.clampCaret()
	t.ensureLayout()
	t.ensureCaretVisible()

	visChanged := changed || t.caret != caret0 || t.selAnchor != sel0 ||
		t.scrollY != scr0 || t.scrollX != scr0x
	text := string(t.runes)
	onCh := t.OnChange
	t.mu.Unlock()

	if visChanged {
		t.Invalidate()
	}
	if changed && onCh != nil {
		onCh(text)
	}
}

// ─── Мышь ────────────────────────────────────────────────────────────────────

// SetCaptureManager инжектится движком (drag-выделение за пределами bounds).
func (t *TextBox) SetCaptureManager(cm CaptureManager) { t.capMgr = cm }

// WantsCapture — захватываем мышь на ЛКМ внутри редактора.
func (t *TextBox) WantsCapture(e MouseEvent) bool {
	return e.Button == MouseLeft && e.Pressed
}

func (t *TextBox) OnMouseButton(e MouseEvent) bool {
	if !t.IsEnabled() {
		return false
	}

	// Контекстное меню.
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		if e.Button == MouseRight && !e.Pressed {
			return true
		}
		if image.Pt(e.X, e.Y).In(t.contextMenu.Bounds()) {
			return t.contextMenu.OnMouseButton(e)
		}
		t.contextMenu.Close()
	}
	if e.Button == MouseRight && e.Pressed {
		t.showContextMenu(e.X, e.Y)
		return true
	}

	inside := image.Pt(e.X, e.Y).In(t.bounds)

	// Колесо — вертикальная прокрутка.
	if inside && (e.Button == MouseWheelUp || e.Button == MouseWheelDown) && e.Pressed {
		t.mu.Lock()
		t.ensureLayout()
		old := t.scrollY
		step := 3 * t.lineHeight()
		if e.Button == MouseWheelUp {
			t.scrollY -= step
		} else {
			t.scrollY += step
		}
		t.clampScroll()
		moved := t.scrollY != old
		t.mu.Unlock()
		if moved {
			t.Invalidate()
		}
		return true
	}

	if e.Button != MouseLeft {
		return false
	}

	t.mu.Lock()
	caret0, sel0 := t.caret, t.selAnchor
	defer func() {
		changed := t.caret != caret0 || t.selAnchor != sel0
		t.mu.Unlock()
		if changed {
			t.Invalidate()
		}
	}()

	if e.Pressed {
		idx := t.charIndexAtPoint(e.X, e.Y)

		// Двойной клик — выделить слово.
		nowMs := time.Now().UnixMilli()
		dx, dy := e.X-t.lastClickX, e.Y-t.lastClickY
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		if nowMs-t.lastClickMs <= 400 && dx <= 4 && dy <= 4 {
			lo, hi := t.wordBounds(idx)
			t.selAnchor = lo
			t.caret = hi
			t.dragging = false
			t.lastClickMs = 0
			return true
		}
		t.lastClickMs = nowMs
		t.lastClickX, t.lastClickY = e.X, e.Y

		t.caret = idx
		t.selAnchor = idx
		t.dragging = true
		t.desiredX = -1
	} else {
		t.dragging = false
		if t.selAnchor == t.caret {
			t.selAnchor = -1
		}
		if t.capMgr != nil {
			t.capMgr.ReleaseCapture()
		}
	}
	return true
}

func (t *TextBox) OnMouseMove(x, y int) {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.OnMouseMove(x, y)
	}
	t.mu.Lock()
	caret0, scr0 := t.caret, t.scrollY
	if t.dragging {
		t.caret = t.charIndexAtPoint(x, y)
		t.ensureCaretVisible()
	}
	changed := t.caret != caret0 || t.scrollY != scr0
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// wordBounds — границы слова вокруг idx (как в TextInput).
func (t *TextBox) wordBounds(idx int) (int, int) {
	n := len(t.runes)
	if n == 0 {
		return 0, 0
	}
	if idx >= n {
		idx = n - 1
	}
	if idx < 0 {
		idx = 0
	}
	cls := isWordRune(t.runes[idx])
	lo := idx
	for lo > 0 && isWordRune(t.runes[lo-1]) == cls && t.runes[lo-1] != '\n' {
		lo--
	}
	hi := idx + 1
	for hi < n && isWordRune(t.runes[hi]) == cls && t.runes[hi] != '\n' {
		hi++
	}
	return lo, hi
}

// ─── Контекстное меню ────────────────────────────────────────────────────────

func (t *TextBox) showContextMenu(x, y int) {
	t.mu.Lock()
	hasSel := t.selActive()
	hasText := len(t.runes) > 0
	ro := t.ReadOnly
	t.mu.Unlock()
	hasClip := ClipboardGetText() != ""

	edit := func(action func()) func() {
		return func() {
			action()
			t.mu.Lock()
			t.clampCaret()
			t.ensureLayout()
			t.ensureCaretVisible()
			text := string(t.runes)
			onCh := t.OnChange
			t.mu.Unlock()
			t.Invalidate()
			if onCh != nil {
				onCh(text)
			}
		}
	}

	menu := NewPopupMenu()
	menu.SetItems([]MenuItem{
		{Text: "Cut", Disabled: !hasSel || ro, OnClick: edit(func() {
			t.mu.Lock()
			if t.selActive() {
				lo, hi := t.normSel()
				ClipboardSetText(string(t.runes[lo:hi]))
				t.deleteSel()
			}
			t.mu.Unlock()
		})},
		{Text: "Copy", Disabled: !hasSel, OnClick: func() {
			t.mu.Lock()
			if t.selActive() {
				lo, hi := t.normSel()
				ClipboardSetText(string(t.runes[lo:hi]))
			}
			t.mu.Unlock()
		}},
		{Text: "Paste", Disabled: !hasClip || ro, OnClick: edit(func() {
			if clip := ClipboardGetText(); clip != "" {
				t.mu.Lock()
				t.insertRunes([]rune(clip))
				t.mu.Unlock()
			}
		})},
		{Separator: true},
		{Text: "Select All", Disabled: !hasText, OnClick: func() {
			t.mu.Lock()
			t.selAnchor = 0
			t.caret = len(t.runes)
			t.mu.Unlock()
			t.Invalidate()
		}},
	})
	menu.Show(x, y)
	t.contextMenu = menu
}

// ─── Draw ────────────────────────────────────────────────────────────────────

func (t *TextBox) Draw(ctx DrawContext) {
	b := t.bounds
	if b.Empty() {
		return
	}

	t.mu.Lock()
	t.ensureLayout()
	lines := make([]tbLine, len(t.lines))
	copy(lines, t.lines)
	runes := make([]rune, len(t.runes))
	copy(runes, t.runes)
	caret := t.caret
	selLo, selHi := -1, -1
	if t.selActive() {
		selLo, selHi = t.normSel()
	}
	scrollY, scrollX := t.scrollY, t.scrollX
	focused := t.focused
	fs := t.fontSize()
	lh := t.lineHeight()
	t.mu.Unlock()

	st := currentStyle()

	// Фон и рамка — в стиле TextInput активной темы.
	switch {
	case st.Classic3D:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Background)
		drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
	case st.ControlCorner > 0:
		cr := st.ControlCorner
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.Background)
		if focused {
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.FocusBorder)
		} else {
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.BorderColor)
		}
	default:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Background)
		if focused {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.FocusBorder)
		} else {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.BorderColor)
		}
	}

	inner := image.Rect(b.Min.X+1, b.Min.Y+1, b.Max.X-1, b.Max.Y-1)
	ctx.SetClip(inner)

	textX := b.Min.X + t.PaddingX - scrollX
	topY := b.Min.Y + t.PaddingY

	if len(runes) == 0 {
		ctx.DrawTextSize(t.Placeholder, b.Min.X+t.PaddingX, topY+2, fs, t.PlaceColor)
	} else {
		first := scrollY / lh
		last := (scrollY + b.Dy()) / lh
		for li := first; li <= last && li < len(lines); li++ {
			ln := lines[li]
			y := topY + li*lh - scrollY
			// Подсветка выделения в пределах строки.
			if selLo >= 0 && selLo < ln.end+1 && selHi > ln.start {
				lo := selLo
				if lo < ln.start {
					lo = ln.start
				}
				hi := selHi
				if hi > ln.end {
					hi = ln.end
				}
				x0 := textX + ctx.MeasureText(string(runes[ln.start:lo]), fs)
				x1 := textX + ctx.MeasureText(string(runes[ln.start:hi]), fs)
				if selHi > ln.end { // выделение уходит на следующую строку
					x1 += 5
				}
				if x1 > x0 {
					ctx.FillRectAlpha(x0, y, x1-x0, lh, t.SelColor)
				}
			}
			ctx.DrawTextSize(string(runes[ln.start:ln.end]), textX, y+2, fs, t.TextColor)
		}
	}

	// Каретка.
	if focused && (time.Now().UnixMilli()/530)%2 == 0 {
		li := 0
		for i, ln := range lines {
			if caret >= ln.start && caret <= ln.end {
				li = i
				break
			}
			li = i
		}
		cx := textX + ctx.MeasureText(string(runes[lines[li].start:caret]), fs)
		cy := topY + li*lh - scrollY
		ctx.DrawVLine(cx, cy+1, lh-2, t.CaretColor)
	}

	// Тонкий вертикальный скроллбар при переполнении.
	contentH := len(lines) * lh
	viewH := b.Dy() - 2*t.PaddingY
	if contentH > viewH {
		trackH := b.Dy() - 8
		thumbH := trackH * viewH / contentH
		if thumbH < 20 {
			thumbH = 20
		}
		maxScroll := contentH - viewH
		ty := b.Min.Y + 4 + (trackH-thumbH)*scrollY/maxScroll
		ctx.FillRoundRect(b.Max.X-6, ty, 4, thumbH, 2, win10.ScrollThumbBG)
	}

	ctx.ClearClip()
	t.drawChildren(ctx)
	t.drawDisabledOverlay(ctx)
}

// ─── Bounds / Overlay (контекстное меню) ─────────────────────────────────────

// Bounds включает открытое контекстное меню (для hit-теста движка).
func (t *TextBox) Bounds() image.Rectangle {
	b := t.Base.Bounds()
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		return b.Union(t.contextMenu.Bounds())
	}
	return b
}

// HasOverlay — контекстное меню открыто.
func (t *TextBox) HasOverlay() bool {
	return t.contextMenu != nil && t.contextMenu.IsOpen()
}

// DrawOverlay рисует контекстное меню поверх всего UI.
func (t *TextBox) DrawOverlay(ctx DrawContext) {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.DrawOverlay(ctx)
	}
}

// Dismiss закрывает контекстное меню (Dismissable).
func (t *TextBox) Dismiss() {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.Close()
	}
}

// ─── Themeable ───────────────────────────────────────────────────────────────

func (t *TextBox) ApplyTheme(th *Theme) {
	t.Background = th.InputBG
	t.BorderColor = th.InputBorder
	t.FocusBorder = th.InputFocus
	t.TextColor = th.InputText
	t.PlaceColor = th.InputPlaceholder
	t.CaretColor = th.InputCaret
	t.SelColor = premulAlpha(th.Accent, 110)
}
