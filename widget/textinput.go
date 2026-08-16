package widget

import (
	"image"
	"image/color"
	"sync"
	"time"
)

// TextInput — однострочное текстовое поле в стиле Windows 10 Dark.
//
// Поддерживает:
//   - Ввод Unicode-символов (включая кириллицу)
//   - Backspace (удалить слева) / Delete (удалить справа)
//   - Стрелки влево/вправо, Home, End
//   - Shift+стрелки / Shift+Home / Shift+End — выделение текста
//   - Ctrl+A — выделить всё
//   - Ctrl+C / Ctrl+X — копирование/вырезание (внутренний буфер)
//   - Ctrl+V — вставка из внутреннего буфера
//   - Мигающий курсор (~530 мс, стиль Windows)
//   - Клик мышью позиционирует курсор
//   - Горизонтальный скролл при переполнении поля
//   - OnEnter — callback при нажатии Enter
//   - OnChange — callback при каждом изменении текста
type TextInput struct {
	Base

	mu       sync.Mutex
	runes    []rune         // содержимое как []rune
	caretPos int            // позиция вставки (индекс в runes)
	selStart int            // начало выделения (-1 = нет)
	selEnd   int            // конец выделения
	scrollX  int            // горизонтальный сдвиг, пикселей
	dragging bool           // true — идёт выделение мышью (зажата ЛКМ)
	capMgr   CaptureManager // инжектится движком через SetCaptureManager

	// Контекстное меню (правый клик).
	contextMenu *PopupMenu

	// Позиции символов от последнего Draw: positions[i] = X-сдвиг i-го символа от начала текста.
	// Обновляется в Draw(), используется в OnMouseButton().
	charPositions []int

	// Кэш раскладки строки (PERF-11). Прежний Draw на КАЖДОМ кадре копировал
	// руны, собирал строку (а в режиме пароля ещё и маску) и заново мерил
	// позиции всех рун — при том что текст между кадрами обычно не меняется.
	// Ключ кэша: отображаемая строка + кегль + ревизия метрик текста
	// (widget.TextMetricsRev растёт при смене DPI/HiDPI-масштаба).
	layoutText     string
	layoutSize     float64
	layoutRev      uint64
	layoutMasked   bool // раскладка снята с маски пароля, а не с текста
	layoutMaskRune rune // символ маски, которым снята раскладка
	layoutRuneLen  int  // длина содержимого в рунах на момент снятия
	layoutValid    bool

	// caretPhase — фаза мигания каретки, ФАКТИЧЕСКИ отрисованная последним
	// Draw; caretPhaseKnown — была ли каретка вообще нарисована.
	// См. NeedsAnimation.
	caretPhase      bool
	caretPhaseKnown bool

	Placeholder string

	Background  color.RGBA
	BorderColor color.RGBA
	FocusBorder color.RGBA
	ErrorBorder color.RGBA // рамка при ошибке валидации (красная)
	TextColor   color.RGBA
	PlaceColor  color.RGBA
	CaretColor  color.RGBA
	SelColor    color.RGBA

	focused bool

	// Password mode
	isPassword   bool // true — режим пароля
	showPassword bool // true — показать пароль (по нажатию глазика)
	eyeHovered   bool // курсор над кнопкой-глазиком
	MaskRune     rune // символ маски (по умолчанию '●')

	PaddingX int
	PaddingY int

	FontName string  // именованный шрифт (RegisterFont); "" → default
	FontSize float64 // размер шрифта в pt (0 → DefaultFontSizePt)

	// AcceptsReturn: true — многострочный режим (WPF AcceptsReturn="True").
	// Enter вставляет перевод строки вместо вызова OnEnter.
	AcceptsReturn bool

	// MaxLength — максимум символов (0 = без ограничения, WPF MaxLength).
	MaxLength int

	// История правок для Undo/Redo (Ctrl+Z / Ctrl+Y).
	undoStack []textEdit
	redoStack []textEdit

	// Для детекции двойного клика (выделение слова).
	lastClickMs int64
	lastClickX  int

	// Состояние ошибки валидации (WPF Validation.HasError). "" = нет ошибки.
	validationError string

	// fireEnter взводится внутри OnKeyEvent (под t.mu) и вызывает OnEnter
	// после освобождения мьютекса — синхронно, но без риска дедлока.
	fireEnter bool

	// OnEnter вызывается при нажатии Enter (только если AcceptsReturn=false).
	OnEnter func()
	// OnChange вызывается при каждом изменении текста.
	OnChange func(text string)
}

// textEdit — снимок состояния поля для Undo/Redo.
type textEdit struct {
	text  string
	caret int
}

// NewTextInput создаёт текстовое поле в стиле Windows 10 Dark.
func NewTextInput(placeholder string) *TextInput {
	return &TextInput{
		Placeholder: placeholder,
		Background:  win10.InputBG,
		BorderColor: win10.InputBorder,
		FocusBorder: win10.InputFocus,
		ErrorBorder: color.RGBA{R: 232, G: 17, B: 35, A: 255}, // #E81123 Win10 error red
		TextColor:   win10.InputText,
		PlaceColor:  win10.InputPlaceholder,
		CaretColor:  win10.InputCaret,
		SelColor:    premulAlpha(win10.Accent, 110),
		PaddingX:    6,
		PaddingY:    4,
		selStart:    -1,
	}
}

// NewPasswordInput создаёт текстовое поле в режиме пароля.
// Текст маскируется символом ●, копирование заблокировано.
// Кнопка-глазик справа позволяет показать/скрыть пароль.
func NewPasswordInput(placeholder string) *TextInput {
	ti := NewTextInput(placeholder)
	ti.isPassword = true
	ti.MaskRune = '●'
	return ti
}

// SetPasswordMode включает/выключает режим пароля; при включении стирает undo.
func (t *TextInput) SetPasswordMode(v bool) {
	t.mu.Lock()
	changed := t.isPassword != v || (!v && t.showPassword)
	t.isPassword = v
	if v {
		t.wipeHistory()
	} else {
		t.showPassword = false
	}
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// wipeHistory затирает и отпускает стеки undo/redo. Вызывать под t.mu.
func (t *TextInput) wipeHistory() {
	clear(t.undoStack)
	clear(t.redoStack)
	t.undoStack, t.redoStack = nil, nil
}

// IsPasswordMode возвращает true, если поле в режиме пароля.
func (t *TextInput) IsPasswordMode() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.isPassword
}

// SetShowPassword показывает/скрывает пароль (только в password mode).
func (t *TextInput) SetShowPassword(v bool) {
	t.mu.Lock()
	changed := t.showPassword != v
	t.showPassword = v
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// ToggleShowPassword переключает видимость пароля.
func (t *TextInput) ToggleShowPassword() {
	t.mu.Lock()
	t.showPassword = !t.showPassword
	t.mu.Unlock()
	t.Invalidate() // переключение всегда меняет отображение
}

// ─── Текст ───────────────────────────────────────────────────────────────────

// SetText устанавливает содержимое поля и сбрасывает курсор в конец.
func (t *TextInput) SetText(text string) {
	t.mu.Lock()
	runes := []rune(text)
	changed := string(t.runes) != text || t.caretPos != len(runes) ||
		t.selStart != -1 || t.scrollX != 0
	if t.isPassword {
		// Прежний пароль затираем в памяти, а не отпускаем.
		clear(t.runes)
		t.wipeHistory()
	}
	t.runes = runes
	t.caretPos = len(runes)
	t.selStart = -1
	t.scrollX = 0
	t.mu.Unlock()
	if changed {
		t.Invalidate()
	}
}

// GetText возвращает текущее содержимое поля.
func (t *TextInput) GetText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.runes)
}

// Cursor возвращает текстовый курсор (I-beam) над полем ввода (CursorProvider).
func (t *TextInput) Cursor(x, y int) Cursor {
	if t.IsEnabled() {
		return CursorIBeam
	}
	return CursorArrow
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (t *TextInput) SetFocused(focused bool) {
	t.mu.Lock()
	changed := t.focused != focused
	t.focused = focused
	t.mu.Unlock()
	if changed {
		t.Invalidate() // рамка фокуса и каретка
	}
}

func (t *TextInput) IsFocused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.focused
}

// ─── Вспомогательные ─────────────────────────────────────────────────────────

func (t *TextInput) selActive() bool {
	return t.selStart >= 0 && t.selStart != t.selEnd
}

func (t *TextInput) normSel() (lo, hi int) {
	if t.selStart <= t.selEnd {
		return t.selStart, t.selEnd
	}
	return t.selEnd, t.selStart
}

// deleteSel удаляет выделенный фрагмент. Возвращает true если было выделение.
func (t *TextInput) deleteSel() bool {
	if !t.selActive() {
		return false
	}
	lo, hi := t.normSel()
	t.runes = append(t.runes[:lo], t.runes[hi:]...)
	t.caretPos = lo
	t.selStart = -1
	return true
}

func (t *TextInput) clampCaret() {
	if t.caretPos < 0 {
		t.caretPos = 0
	}
	if t.caretPos > len(t.runes) {
		t.caretPos = len(t.runes)
	}
}

// undo откатывает последнее изменение (вызывать под t.mu).
func (t *TextInput) undo() {
	if len(t.undoStack) == 0 {
		return
	}
	cur := textEdit{text: string(t.runes), caret: t.caretPos}
	last := t.undoStack[len(t.undoStack)-1]
	t.undoStack = t.undoStack[:len(t.undoStack)-1]
	t.redoStack = append(t.redoStack, cur)
	t.runes = []rune(last.text)
	t.caretPos = last.caret
	t.selStart = -1
}

// redo повторяет отменённое изменение (вызывать под t.mu).
func (t *TextInput) redo() {
	if len(t.redoStack) == 0 {
		return
	}
	cur := textEdit{text: string(t.runes), caret: t.caretPos}
	next := t.redoStack[len(t.redoStack)-1]
	t.redoStack = t.redoStack[:len(t.redoStack)-1]
	t.undoStack = append(t.undoStack, cur)
	t.runes = []rune(next.text)
	t.caretPos = next.caret
	t.selStart = -1
}

// runesEqualString сравнивает срез рун со строкой без аллокаций
// (string(rs) == s создало бы копию на каждом кадре — см. Draw).
func runesEqualString(rs []rune, s string) bool {
	i := 0
	for _, r := range s {
		if i >= len(rs) || rs[i] != r {
			return false
		}
		i++
	}
	return i == len(rs)
}

// caretBlinkHalfPeriodMs — полупериод мигания каретки (мс, стиль Windows).
const caretBlinkHalfPeriodMs = 530

// caretPhaseAt возвращает фазу мигания каретки (true — каретка видима)
// для момента времени ms (Unix-миллисекунды).
func caretPhaseAt(ms int64) bool { return (ms/caretBlinkHalfPeriodMs)%2 == 0 }

// NeedsAnimation — «нужен ли движку кадр ради мигающей каретки».
//
// PERF-11: раньше здесь возвращалось просто IsFocused(), и движок в on-demand
// режиме рендерил ПОЛНЫЙ кадр всего дерева на целевом FPS (60 кадров в секунду
// ради каретки, меняющейся ~2 раза в секунду) — весь смысл рендера по запросу
// терялся, стоило поставить курсор в поле ввода.
//
// Теперь: пока фаза мигания совпадает с уже нарисованной — кадр не нужен
// (false). Когда фаза сменилась — виджет инвалидирует ТОЛЬКО свой прямоугольник
// и всё равно возвращает false: движок увидит новое поколение инвалидации на
// следующем тике и отрисует ЧАСТИЧНЫЙ кадр по damage-области поля. Возврат true
// здесь дал бы полный кадр (damage пуст → partial=false в renderFrame), то есть
// ровно то, от чего мы уходим. Задержка — один тик (≤ 1/FPS) на полупериод
// 530 мс, визуально незаметна.
func (t *TextInput) NeedsAnimation() bool {
	phase := caretPhaseAt(time.Now().UnixMilli())
	t.mu.Lock()
	if !t.focused || (t.caretPhaseKnown && t.caretPhase == phase) {
		t.mu.Unlock()
		return false // фазу менять не пора — перерисовывать нечего
	}
	// Запрошенную фазу фиксируем здесь же: иначе, если кадр по какой-то причине
	// не дойдёт до Draw (виджет скрыт, кадр отброшен), мы бы инвалидировали на
	// КАЖДОМ тике. Пропущенное мигание безобиднее непрерывной перерисовки.
	t.caretPhase, t.caretPhaseKnown = phase, true
	t.mu.Unlock()
	t.Invalidate() // точечный damage по bounds поля
	return false
}

// ─── ValidationAware ──────────────────────────────────────────────────────────

// SetValidationError переводит поле в состояние ошибки (красная рамка) и
// помещает текст ошибки в ToolTip. Пустая строка снимает ошибку.
func (t *TextInput) SetValidationError(msg string) {
	t.mu.Lock()
	changed := t.validationError != msg
	t.validationError = msg
	t.mu.Unlock()
	t.SetToolTip(msg)
	if changed {
		t.Invalidate() // появление/снятие красной рамки
	}
}

// ValidationError возвращает текущий текст ошибки ("" если поле корректно).
func (t *TextInput) ValidationError() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.validationError
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (t *TextInput) OnKeyEvent(e KeyEvent) {
	if !t.IsEnabled() || !e.Pressed {
		return
	}

	// Если контекстное меню открыто — делегируем клавиши ему.
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.OnKeyEvent(e)
		return
	}

	ctrl := e.Mod&ModCtrl != 0
	shift := e.Mod&ModShift != 0

	t.mu.Lock()
	changed := false
	isUndoRedo := false
	before := textEdit{text: string(t.runes), caret: t.caretPos}
	// Снимок визуального состояния — для авто-инвалидации при фактическом изменении.
	caret0, sel0, sel1 := t.caretPos, t.selStart, t.selEnd

	switch e.Code {
	case KeyLeft:
		if shift {
			if t.selStart < 0 {
				t.selStart = t.caretPos
			}
			if t.caretPos > 0 {
				t.caretPos--
			}
			t.selEnd = t.caretPos
		} else {
			if t.selActive() {
				lo, _ := t.normSel()
				t.caretPos = lo
			} else if t.caretPos > 0 {
				t.caretPos--
			}
			t.selStart = -1
		}

	case KeyRight:
		if shift {
			if t.selStart < 0 {
				t.selStart = t.caretPos
			}
			if t.caretPos < len(t.runes) {
				t.caretPos++
			}
			t.selEnd = t.caretPos
		} else {
			if t.selActive() {
				_, hi := t.normSel()
				t.caretPos = hi
			} else if t.caretPos < len(t.runes) {
				t.caretPos++
			}
			t.selStart = -1
		}

	case KeyHome:
		if shift {
			if t.selStart < 0 {
				t.selStart = t.caretPos
			}
			t.caretPos = 0
			t.selEnd = 0
		} else {
			t.caretPos = 0
			t.selStart = -1
		}

	case KeyEnd:
		if shift {
			if t.selStart < 0 {
				t.selStart = t.caretPos
			}
			t.caretPos = len(t.runes)
			t.selEnd = t.caretPos
		} else {
			t.caretPos = len(t.runes)
			t.selStart = -1
		}

	case KeyBackspace:
		if t.deleteSel() {
			changed = true
		} else if ctrl {
			// Ctrl+Backspace — удалить слово НАЗАД от каретки.
			if t.caretPos > 0 {
				start := t.wordLeft(t.caretPos)
				if start < t.caretPos {
					t.runes = append(t.runes[:start], t.runes[t.caretPos:]...)
					t.caretPos = start
					changed = true
				}
			}
		} else if t.caretPos > 0 {
			t.runes = append(t.runes[:t.caretPos-1], t.runes[t.caretPos:]...)
			t.caretPos--
			changed = true
		}

	case KeyInsert:
		if ctrl {
			// Ctrl+Insert = копировать (как Ctrl+C); в password-режиме запрещено.
			t.copySelection()
		} else if shift {
			// Shift+Insert = вставить (как Ctrl+V).
			if t.pasteFromClipboard() {
				changed = true
			}
		}

	case KeyDelete:
		if shift {
			// Shift+Delete = вырезать (как Ctrl+X) — приоритетнее обычного Delete.
			if t.cutSelection() {
				changed = true
			}
		} else if t.deleteSel() {
			changed = true
		} else if ctrl {
			// Ctrl+Delete — удалить слово ВПЕРЁД от каретки.
			if t.caretPos < len(t.runes) {
				end := t.wordRight(t.caretPos)
				if end > t.caretPos {
					t.runes = append(t.runes[:t.caretPos], t.runes[end:]...)
					changed = true
				}
			}
		} else if t.caretPos < len(t.runes) {
			t.runes = append(t.runes[:t.caretPos], t.runes[t.caretPos+1:]...)
			changed = true
		}

	case KeyEnter:
		if t.AcceptsReturn {
			// Многострочный режим: вставляем перевод строки
			t.deleteSel()
			t.runes = append(t.runes, 0) // расширяем
			copy(t.runes[t.caretPos+1:], t.runes[t.caretPos:])
			t.runes[t.caretPos] = '\n'
			t.caretPos++
			changed = true
		} else if t.OnEnter != nil {
			t.fireEnter = true // вызовем после t.mu.Unlock (в конце OnKeyEvent)
		}

	default:
		if ctrl {
			switch e.Code {
			case KeyZ:
				if shift {
					t.redo()
				} else {
					t.undo()
				}
				isUndoRedo = true
				changed = true
			case KeyY:
				t.redo()
				isUndoRedo = true
				changed = true
			case KeyA:
				t.selStart = 0
				t.selEnd = len(t.runes)
				t.caretPos = len(t.runes)
			case KeyC:
				// В режиме пароля копирование запрещено (см. copySelection).
				t.copySelection()
			case KeyX:
				// В режиме пароля вырезание запрещено (см. cutSelection).
				if t.cutSelection() {
					changed = true
				}
			case KeyV:
				if t.pasteFromClipboard() {
					changed = true
				}
			}
		} else if e.Rune >= 32 {
			t.deleteSel()
			if t.MaxLength <= 0 || len(t.runes) < t.MaxLength {
				ins := make([]rune, len(t.runes)+1)
				copy(ins, t.runes[:t.caretPos])
				ins[t.caretPos] = e.Rune
				copy(ins[t.caretPos+1:], t.runes[t.caretPos:])
				t.runes = ins
				t.caretPos++
				changed = true
			}
		}
	}

	// Запись в историю Undo (кроме самих операций undo/redo).
	//
	// В режиме пароля историю НЕ ведём: каждая запись — полный снимок текста
	// в открытом виде, и после очистки поля Ctrl+Z восстанавливал бы пароль
	// (аудит SEC-12). Copy/Cut в этом режиме уже заблокированы — undo был
	// последней лазейкой.
	if changed && !isUndoRedo && !t.isPassword {
		t.undoStack = append(t.undoStack, before)
		if len(t.undoStack) > 200 {
			t.undoStack = t.undoStack[1:]
		}
		t.redoStack = nil
	}

	t.clampCaret()
	visChanged := changed || t.caretPos != caret0 || t.selStart != sel0 || t.selEnd != sel1
	text := string(t.runes)
	onCh := t.OnChange
	fireEnter := t.fireEnter
	t.fireEnter = false
	onEnter := t.OnEnter
	t.mu.Unlock()

	if visChanged {
		t.Invalidate() // текст/каретка/выделение фактически изменились
	}

	// Синхронный вызов вне t.mu: сохраняет порядок изменений (writeBack в
	// модель идёт строго в порядке нажатий) и единую модель исполнения.
	if changed && onCh != nil {
		onCh(text)
	}
	if fireEnter && onEnter != nil {
		onEnter()
	}
}

// ─── Mouse Capture (drag-выделение текста) ──────────────────────────────────

// SetCaptureManager инжектит менеджер захвата мыши (вызывается движком при SetRoot).
func (t *TextInput) SetCaptureManager(cm CaptureManager) {
	t.capMgr = cm
}

// WantsCapture возвращает true для ЛКМ внутри текстового поля —
// захватываем мышь, чтобы mouseUp и mouseMove приходили сюда даже за пределами bounds.
func (t *TextInput) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	// Не захватываем, если клик по глазику (isPassword — под t.mu, SEC-18).
	t.mu.Lock()
	isPassword := t.isPassword
	t.mu.Unlock()
	if isPassword {
		b := t.Bounds()
		if e.X >= b.Max.X-eyeButtonWidth && e.X <= b.Max.X {
			return false
		}
	}
	return true
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// charIndexAtX возвращает индекс символа (позицию каретки) для абсолютной X-координаты.
// Вызывать под t.mu.Lock().
func (t *TextInput) charIndexAtX(absX int) int {
	b := t.bounds
	textX := b.Min.X + t.PaddingX - t.scrollX
	relX := absX - textX

	pos := t.charPositions
	if len(pos) == 0 {
		return 0
	}
	best := len(pos) - 1
	for i := 0; i < len(pos)-1; i++ {
		mid := (pos[i] + pos[i+1]) / 2
		if relX <= mid {
			best = i
			break
		}
	}
	return best
}

// wordBoundsAt возвращает [lo, hi) — границы слова под индексом idx.
// Вызывать под t.mu.Lock().
func (t *TextInput) wordBoundsAt(idx int) (int, int) {
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
	for lo > 0 && isWordRune(t.runes[lo-1]) == cls {
		lo--
	}
	hi := idx + 1
	for hi < n && isWordRune(t.runes[hi]) == cls {
		hi++
	}
	return lo, hi
}

// wordLeft возвращает позицию начала предыдущего слова относительно idx
// (симметрично TextBox.wordLeft). Вызывать под t.mu.Lock().
func (t *TextInput) wordLeft(idx int) int {
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

// wordRight возвращает позицию за концом следующего слова относительно idx
// (симметрично TextBox.wordRight). Вызывать под t.mu.Lock().
func (t *TextInput) wordRight(idx int) int {
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

// copySelection копирует выделение в буфер обмена. В password-режиме
// копирование запрещено. Вызывать под t.mu.Lock().
func (t *TextInput) copySelection() {
	if t.isPassword || !t.selActive() {
		return
	}
	lo, hi := t.normSel()
	ClipboardSetText(string(t.runes[lo:hi]))
}

// cutSelection копирует выделение в буфер обмена и удаляет его. В password-
// режиме вырезание запрещено. Возвращает true, если что-то вырезано.
// Вызывать под t.mu.Lock().
func (t *TextInput) cutSelection() bool {
	if t.isPassword || !t.selActive() {
		return false
	}
	lo, hi := t.normSel()
	ClipboardSetText(string(t.runes[lo:hi]))
	t.deleteSel()
	return true
}

// pasteFromClipboard вставляет текст из буфера обмена в позицию каретки
// (с учётом выделения и MaxLength). Возвращает true, если текст фактически
// вставлен. Вызывать под t.mu.Lock().
func (t *TextInput) pasteFromClipboard() bool {
	clipText := ClipboardGetText()
	if len(clipText) == 0 {
		return false
	}
	t.deleteSel()
	paste := []rune(clipText)
	if t.MaxLength > 0 { // обрезаем под лимит
		room := t.MaxLength - len(t.runes)
		if room < 0 {
			room = 0
		}
		if len(paste) > room {
			paste = paste[:room]
		}
	}
	n := len(paste)
	if n == 0 {
		return false
	}
	ins := make([]rune, len(t.runes)+n)
	copy(ins, t.runes[:t.caretPos])
	copy(ins[t.caretPos:], paste)
	copy(ins[t.caretPos+n:], t.runes[t.caretPos:])
	t.runes = ins
	t.caretPos += n
	return true
}

// isWordRune — true для букв/цифр/подчёркивания (символы одного «слова»).
func isWordRune(r rune) bool {
	if r == '_' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
		return true
	}
	// Не-ASCII (кириллица и т.п.) считаем частью слова, пробелы — нет.
	return r > 127 && r != ' '
}

// ─── MouseClickHandler ───────────────────────────────────────────────────────

func (t *TextInput) OnMouseButton(e MouseEvent) bool {
	if !t.IsEnabled() {
		return false
	}

	// Если контекстное меню открыто — делегируем ему клики.
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		// Поглощаем отпускание правой кнопки — это та же кнопка,
		// которой открыли меню; без этого popup закроется мгновенно.
		if e.Button == MouseRight && !e.Pressed {
			return true
		}
		pr := t.contextMenu.Bounds()
		if image.Pt(e.X, e.Y).In(pr) {
			return t.contextMenu.OnMouseButton(e)
		}
		// Клик за пределами меню — закрываем.
		t.contextMenu.Close()
		// Не возвращаем true: пусть клик обработается нормально.
	}

	// Правый клик — контекстное меню.
	if e.Button == MouseRight && e.Pressed {
		t.showContextMenu(e.X, e.Y)
		return true
	}

	if e.Button != MouseLeft {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Авто-инвалидация: defer выполняется до Unlock (LIFO), поля читаем ещё
	// под мьютексом; инвалидируем только при фактическом изменении.
	caret0, sel0, sel1, eye0 := t.caretPos, t.selStart, t.selEnd, t.showPassword
	defer func() {
		if t.caretPos != caret0 || t.selStart != sel0 || t.selEnd != sel1 || t.showPassword != eye0 {
			t.Invalidate()
		}
	}()

	b := t.bounds

	if e.Pressed {
		// ── MouseDown (ЛКМ) ──

		// Клик по кнопке-глазику (правая часть поля)
		if t.isPassword && e.X >= b.Max.X-eyeButtonWidth && e.X <= b.Max.X {
			t.showPassword = !t.showPassword
			return true
		}

		idx := t.charIndexAtX(e.X)

		// Детекция двойного клика → выделение слова.
		nowMs := time.Now().UnixMilli()
		dx := e.X - t.lastClickX
		if dx < 0 {
			dx = -dx
		}
		if nowMs-t.lastClickMs <= 400 && dx <= 4 {
			lo, hi := t.wordBoundsAt(idx)
			t.selStart = lo
			t.selEnd = hi
			t.caretPos = hi
			t.dragging = false
			t.lastClickMs = 0 // сбрасываем, чтобы тройной клик не путался
			return true
		}
		t.lastClickMs = nowMs
		t.lastClickX = e.X

		t.caretPos = idx
		t.selStart = idx // якорь выделения
		t.selEnd = idx   // пока совпадает — выделения нет
		t.dragging = true
	} else {
		// ── MouseUp (ЛКМ) ──
		t.dragging = false
		// Если selStart == selEnd → выделения не было, сбрасываем.
		if t.selStart == t.selEnd {
			t.selStart = -1
		}
		// Освобождаем захват мыши.
		if t.capMgr != nil {
			t.capMgr.ReleaseCapture()
		}
	}
	return true
}

// showContextMenu создаёт и показывает контекстное меню (Cut/Copy/Paste/Select All).
func (t *TextInput) showContextMenu(x, y int) {
	t.mu.Lock()
	hasSel := t.selActive()
	hasText := len(t.runes) > 0
	isPwd := t.isPassword
	t.mu.Unlock()

	clipText := ClipboardGetText()
	hasClip := len(clipText) > 0

	menu := NewPopupMenu()

	// Cut
	menu.SetItems([]MenuItem{
		{
			Text:     "Cut",
			Disabled: !hasSel || isPwd,
			OnClick: func() {
				t.mu.Lock()
				if t.selActive() && !t.isPassword {
					lo, hi := t.normSel()
					ClipboardSetText(string(t.runes[lo:hi]))
					t.deleteSel()
					t.clampCaret()
					text := string(t.runes)
					onCh := t.OnChange
					t.mu.Unlock()
					t.Invalidate() // текст изменился (было выделение)
					if onCh != nil {
						onCh(text) // синхронно — вне t.mu
					}
				} else {
					t.mu.Unlock()
				}
			},
		},
		{
			Text:     "Copy",
			Disabled: !hasSel || isPwd,
			OnClick: func() {
				t.mu.Lock()
				if t.selActive() && !t.isPassword {
					lo, hi := t.normSel()
					ClipboardSetText(string(t.runes[lo:hi]))
				}
				t.mu.Unlock()
			},
		},
		{
			Text:     "Paste",
			Disabled: !hasClip,
			OnClick: func() {
				ct := ClipboardGetText()
				if len(ct) == 0 {
					return
				}
				t.mu.Lock()
				hadSel := t.deleteSel()
				paste := []rune(ct)
				if t.MaxLength > 0 { // обрезаем под лимит (как при Ctrl+V)
					room := t.MaxLength - len(t.runes)
					if room < 0 {
						room = 0
					}
					if len(paste) > room {
						paste = paste[:room]
					}
				}
				n := len(paste)
				ins := make([]rune, len(t.runes)+n)
				copy(ins, t.runes[:t.caretPos])
				copy(ins[t.caretPos:], paste)
				copy(ins[t.caretPos+n:], t.runes[t.caretPos:])
				t.runes = ins
				t.caretPos += n
				t.clampCaret()
				text := string(t.runes)
				onCh := t.OnChange
				t.mu.Unlock()
				if hadSel || n > 0 {
					t.Invalidate() // текст фактически изменился
				}
				if onCh != nil {
					onCh(text) // синхронно — вне t.mu
				}
			},
		},
		{Separator: true},
		{
			Text:     "Select All",
			Disabled: !hasText,
			OnClick: func() {
				t.mu.Lock()
				changed := t.selStart != 0 || t.selEnd != len(t.runes) ||
					t.caretPos != len(t.runes)
				t.selStart = 0
				t.selEnd = len(t.runes)
				t.caretPos = len(t.runes)
				t.mu.Unlock()
				if changed {
					t.Invalidate()
				}
			},
		},
	})

	menu.Show(x, y)
	t.contextMenu = menu
}

// OnMouseMove обрабатывает hover для кнопки-глазика, контекстного меню и drag-выделение.
func (t *TextInput) OnMouseMove(x, y int) {
	// Делегируем hover контекстному меню.
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.OnMouseMove(x, y)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Авто-инвалидация при фактическом изменении (LIFO — выполняется до Unlock).
	caret0, sel1, eye0 := t.caretPos, t.selEnd, t.eyeHovered
	defer func() {
		if t.caretPos != caret0 || t.selEnd != sel1 || t.eyeHovered != eye0 {
			t.Invalidate()
		}
	}()

	// Drag-выделение: если ЛКМ зажата — обновляем selEnd и caretPos.
	if t.dragging {
		idx := t.charIndexAtX(x)
		t.selEnd = idx
		t.caretPos = idx
	}

	if t.isPassword {
		b := t.bounds
		t.eyeHovered = x >= b.Max.X-eyeButtonWidth && x <= b.Max.X &&
			y >= b.Min.Y && y <= b.Max.Y
	}
}

// ─── Draw ────────────────────────────────────────────────────────────────────

// eyeButtonWidth — ширина области кнопки-глазика для пароля.
const eyeButtonWidth = 28

func (t *TextInput) Draw(ctx DrawContext) {
	const sizePt = DefaultFontSizePt
	metricsRev := TextMetricsRev()

	t.mu.Lock()
	isFocused := t.focused
	caretPos := t.caretPos
	selStart := t.selStart
	selEnd := t.selEnd
	isPwd := t.isPassword
	showPwd := t.showPassword
	maskRune := t.MaskRune
	eyeHov := t.eyeHovered
	hasError := t.validationError != ""
	if maskRune == 0 {
		maskRune = '●'
	}
	// Отображаемая строка: маска (скрытый пароль) или сам текст.
	//
	// PERF-11: сперва пробуем ПОПАСТЬ В КЭШ, не собирая строку — иначе на каждом
	// кадре была бы аллокация (string(runes) плюс срез маски). Сравнение с
	// закэшированной строкой идёт по рунам, без промежуточных копий.
	// В режиме скрытого пароля открытый текст в строку не собирается вовсе
	// (прежний код делал string(runes) всегда, даже под маской).
	masked := isPwd && !showPwd && len(t.runes) > 0
	var displayText string
	var positions []int
	if t.layoutValid && t.layoutSize == sizePt && t.layoutRev == metricsRev &&
		t.layoutMasked == masked && t.layoutRuneLen == len(t.runes) &&
		((masked && t.layoutMaskRune == maskRune) || (!masked && runesEqualString(t.runes, t.layoutText))) {
		displayText, positions = t.layoutText, t.charPositions
	} else if masked {
		buf := make([]rune, len(t.runes))
		for i := range buf {
			buf[i] = maskRune
		}
		displayText = string(buf)
	} else {
		displayText = string(t.runes)
	}
	t.mu.Unlock()

	b := t.bounds
	if b.Empty() {
		return
	}

	st := currentStyle()

	// Фон + рамка по стилю темы.
	switch {
	case st.Classic3D:
		// Классика: утопленное поле (sunken bevel), прямые углы.
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Background)
		drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
		if hasError {
			ctx.DrawBorder(b.Min.X+2, b.Min.Y+2, b.Dx()-4, b.Dy()-4, t.ErrorBorder)
		}
	case st.ControlCorner > 0:
		cr := st.ControlCorner
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.Background)
		switch {
		case hasError:
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.ErrorBorder)
			ctx.DrawRoundBorder(b.Min.X+1, b.Min.Y+1, b.Dx()-2, b.Dy()-2, cr-1, t.ErrorBorder)
		case isFocused:
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.FocusBorder)
		default:
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, t.BorderColor)
		}
	default:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Background)
		if hasError {
			// Ошибка валидации — красная рамка (двойная для заметности).
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.ErrorBorder)
			ctx.DrawBorder(b.Min.X+1, b.Min.Y+1, b.Dx()-2, b.Dy()-2, t.ErrorBorder)
		} else if isFocused {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.FocusBorder)
			ctx.DrawHLine(b.Min.X, b.Max.Y-2, b.Dx(), t.FocusBorder)
		} else {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.BorderColor)
		}
	}

	const textH = 13

	// В режиме пароля резервируем место под кнопку-глазик
	rightPad := t.PaddingX
	if isPwd {
		rightPad = eyeButtonWidth + 2
	}
	textAreaW := b.Dx() - t.PaddingX - rightPad
	textY := b.Min.Y + (b.Dy()-textH)/2
	if textY < b.Min.Y+2 {
		textY = b.Min.Y + 2
	}

	// Позиции символов (по отображаемому тексту): считаем ТОЛЬКО при промахе
	// кэша раскладки — см. поля layout* и заполнение ключа ниже.
	if positions == nil {
		positions = ctx.MeasureRunePositions(displayText, sizePt) // len(runes)+1
	}

	// Обновляем сохранённые позиции и scrollX
	t.mu.Lock()
	t.charPositions = positions
	t.layoutText, t.layoutSize, t.layoutRev = displayText, sizePt, metricsRev
	t.layoutMasked, t.layoutMaskRune, t.layoutRuneLen = masked, maskRune, len(t.runes)
	t.layoutValid = true

	caretPx := 0
	if caretPos < len(positions) {
		caretPx = positions[caretPos]
	} else if len(positions) > 0 {
		caretPx = positions[len(positions)-1]
	}
	// Скролл: курсор должен быть в видимой области
	if caretPx-t.scrollX > textAreaW-4 {
		t.scrollX = caretPx - textAreaW + 4
	}
	if caretPx-t.scrollX < 0 {
		t.scrollX = caretPx
	}
	if t.scrollX < 0 {
		t.scrollX = 0
	}
	scrollX := t.scrollX
	t.mu.Unlock()

	// Клиппинг по внутренней области поля (без зоны глазика)
	inner := image.Rect(b.Min.X+1, b.Min.Y+1, b.Max.X-rightPad, b.Max.Y-1)
	ctx.SetClip(inner)

	textX := b.Min.X + t.PaddingX - scrollX

	if displayText == "" {
		ctx.DrawText(t.Placeholder, b.Min.X+t.PaddingX, textY, t.PlaceColor)
	} else {
		// Подсветка выделения
		if selStart >= 0 && selStart != selEnd {
			lo, hi := selStart, selEnd
			if lo > hi {
				lo, hi = hi, lo
			}
			if lo < 0 {
				lo = 0
			}
			if hi >= len(positions) {
				hi = len(positions) - 1
			}
			selX0 := textX + positions[lo]
			selX1 := textX + positions[hi]
			ctx.FillRectAlpha(selX0, textY-1, selX1-selX0, textH+5, t.SelColor)
		}
		ctx.DrawText(displayText, textX, textY, t.TextColor)
	}

	// Мигающий курсор. Отрисованную фазу запоминаем — по ней NeedsAnimation
	// решает, нужен ли кадр (PERF-11).
	if isFocused {
		caretVisible := caretPhaseAt(time.Now().UnixMilli())
		t.mu.Lock()
		t.caretPhase, t.caretPhaseKnown = caretVisible, true
		t.mu.Unlock()
		if caretVisible {
			caretX := textX + caretPx
			ctx.DrawVLine(caretX, textY, textH, t.CaretColor)
		}
	}

	ctx.ClearClip()

	// Кнопка-глазик (показать/скрыть пароль)
	if isPwd {
		t.drawEyeButton(ctx, b, textY, textH, showPwd, eyeHov)
	}

	t.drawChildren(ctx)
	t.drawDisabledOverlay(ctx)
}

// drawEyeButton рисует кнопку показа/скрытия пароля в правой части поля.
// Иконка: упрощённый «глаз» — овал + зрачок.
// Если пароль скрыт — перечёркнутый глаз (диагональная линия).
func (t *TextInput) drawEyeButton(ctx DrawContext, b image.Rectangle, textY, textH int, showPwd, hovered bool) {
	// Область кнопки
	btnX := b.Max.X - eyeButtonWidth
	btnY := b.Min.Y
	btnW := eyeButtonWidth
	btnH := b.Dy()

	// Разделитель
	ctx.DrawVLine(btnX, btnY+4, btnH-8, t.BorderColor)

	// Подсветка при наведении
	if hovered {
		ctx.FillRectAlpha(btnX+1, btnY+1, btnW-2, btnH-2, color.RGBA{R: 255, G: 255, B: 255, A: 20})
	}

	// Центр иконки
	cx := btnX + btnW/2
	cy := btnY + btnH/2

	eyeCol := t.PlaceColor
	if hovered {
		eyeCol = t.TextColor
	}

	// Рисуем глаз: горизонтальный овал из точек
	// Верхняя и нижняя дуга
	for dx := -5; dx <= 5; dx++ {
		// Формула эллипса: dy = ±3 * sqrt(1 - (dx/5)^2)
		frac := float64(dx) / 5.0
		dyf := 3.0 * sqrt1minus(frac*frac)
		dy := int(dyf + 0.5)
		ctx.SetPixel(cx+dx, cy-dy, eyeCol)
		ctx.SetPixel(cx+dx, cy+dy, eyeCol)
	}
	// Зрачок — маленький закрашенный кружок
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			ctx.SetPixel(cx+dx, cy+dy, eyeCol)
		}
	}

	// Если пароль скрыт — рисуем перечёркивание (диагональная линия)
	if !showPwd {
		for i := -6; i <= 6; i++ {
			py := cy + i*5/6
			ctx.SetPixel(cx+i, py, eyeCol)
			ctx.SetPixel(cx+i, py+1, eyeCol)
		}
	}
}

// sqrt1minus вычисляет sqrt(1 - x) для x в [0,1].
func sqrt1minus(x float64) float64 {
	if x >= 1 {
		return 0
	}
	if x <= 0 {
		return 1
	}
	// Быстрое приближение без import math
	v := 1.0 - x
	guess := v
	for i := 0; i < 5; i++ {
		guess = (guess + v/guess) / 2
	}
	return guess
}

// ─── Bounds (расширенные при открытом контекстном меню) ─────────────────────

// Bounds возвращает расширенный прямоугольник, включая popup контекстного меню,
// чтобы hitTest и findOverlayAt (engine/events.go) находили виджет при клике
// на пункты контекстного меню.
func (t *TextInput) Bounds() image.Rectangle {
	b := t.Base.Bounds()
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		return b.Union(t.contextMenu.Bounds())
	}
	return b
}

// BaseBounds возвращает базовый прямоугольник поля (без popup).
func (t *TextInput) BaseBounds() image.Rectangle {
	return t.Base.Bounds()
}

// ─── Overlay (контекстное меню) ──────────────────────────────────────────────

// HasOverlay возвращает true когда контекстное меню открыто.
func (t *TextInput) HasOverlay() bool {
	return t.contextMenu != nil && t.contextMenu.IsOpen()
}

// DrawOverlay рисует контекстное меню поверх всего UI.
func (t *TextInput) DrawOverlay(ctx DrawContext) {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.DrawOverlay(ctx)
	}
}

// OverlayBounds возвращает прямоугольник открытого контекстного меню
// (для выноса в нативное окно). Реализует widget.OverlayBoundsProvider.
func (t *TextInput) OverlayBounds() image.Rectangle {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		return t.contextMenu.OverlayBounds()
	}
	return image.Rectangle{}
}

// Dismiss закрывает контекстное меню. Реализует Dismissable.
func (t *TextInput) Dismiss() {
	if t.contextMenu != nil && t.contextMenu.IsOpen() {
		t.contextMenu.Close()
	}
}

// ─── Themeable ───────────────────────────────────────────────────────────────

func (t *TextInput) ApplyTheme(th *Theme) {
	t.Background = th.InputBG
	t.BorderColor = th.InputBorder
	t.FocusBorder = th.InputFocus
	t.TextColor = th.InputText
	t.PlaceColor = th.InputPlaceholder
	t.CaretColor = th.InputCaret
	t.SelColor = premulAlpha(th.Accent, 110)
}
