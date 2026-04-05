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

	mu        sync.Mutex
	runes     []rune // содержимое как []rune
	caretPos  int    // позиция вставки (индекс в runes)
	selStart  int    // начало выделения (-1 = нет)
	selEnd    int    // конец выделения
	scrollX   int    // горизонтальный сдвиг, пикселей
	clipboard []rune // внутренний буфер обмена

	// Позиции символов от последнего Draw: positions[i] = X-сдвиг i-го символа от начала текста.
	// Обновляется в Draw(), используется в OnMouseButton().
	charPositions []int

	Placeholder string

	Background  color.RGBA
	BorderColor color.RGBA
	FocusBorder color.RGBA
	TextColor   color.RGBA
	PlaceColor  color.RGBA
	CaretColor  color.RGBA
	SelColor    color.RGBA

	focused bool

	// Password mode
	isPassword   bool  // true — режим пароля
	showPassword bool  // true — показать пароль (по нажатию глазика)
	eyeHovered   bool  // курсор над кнопкой-глазиком
	MaskRune     rune  // символ маски (по умолчанию '●')

	PaddingX int
	PaddingY int

	FontName string // именованный шрифт (RegisterFont); "" → default
	FontSize float64 // размер шрифта в pt (0 → DefaultFontSizePt)

	// AcceptsReturn: true — многострочный режим (WPF AcceptsReturn="True").
	// Enter вставляет перевод строки вместо вызова OnEnter.
	AcceptsReturn bool

	// OnEnter вызывается при нажатии Enter (только если AcceptsReturn=false).
	OnEnter func()
	// OnChange вызывается при каждом изменении текста.
	OnChange func(text string)
}

// NewTextInput создаёт текстовое поле в стиле Windows 10 Dark.
func NewTextInput(placeholder string) *TextInput {
	return &TextInput{
		Placeholder: placeholder,
		Background:  win10.InputBG,
		BorderColor: win10.InputBorder,
		FocusBorder: win10.InputFocus,
		TextColor:   win10.InputText,
		PlaceColor:  win10.InputPlaceholder,
		CaretColor:  win10.InputCaret,
		SelColor:    color.RGBA{R: 0, G: 120, B: 215, A: 90},
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

// SetPasswordMode включает/выключает режим пароля.
func (t *TextInput) SetPasswordMode(v bool) {
	t.mu.Lock()
	t.isPassword = v
	if !v {
		t.showPassword = false
	}
	t.mu.Unlock()
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
	t.showPassword = v
	t.mu.Unlock()
}

// ToggleShowPassword переключает видимость пароля.
func (t *TextInput) ToggleShowPassword() {
	t.mu.Lock()
	t.showPassword = !t.showPassword
	t.mu.Unlock()
}

// ─── Текст ───────────────────────────────────────────────────────────────────

// SetText устанавливает содержимое поля и сбрасывает курсор в конец.
func (t *TextInput) SetText(text string) {
	t.mu.Lock()
	t.runes = []rune(text)
	t.caretPos = len(t.runes)
	t.selStart = -1
	t.scrollX = 0
	t.mu.Unlock()
}

// GetText возвращает текущее содержимое поля.
func (t *TextInput) GetText() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.runes)
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (t *TextInput) SetFocused(focused bool) {
	t.mu.Lock()
	t.focused = focused
	t.mu.Unlock()
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

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (t *TextInput) OnKeyEvent(e KeyEvent) {
	if !t.IsEnabled() || !e.Pressed {
		return
	}
	ctrl := e.Mod&ModCtrl != 0
	shift := e.Mod&ModShift != 0

	t.mu.Lock()
	changed := false

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
		} else if t.caretPos > 0 {
			t.runes = append(t.runes[:t.caretPos-1], t.runes[t.caretPos:]...)
			t.caretPos--
			changed = true
		}

	case KeyDelete:
		if t.deleteSel() {
			changed = true
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
			go t.OnEnter()
		}

	default:
		if ctrl {
			switch e.Code {
			case KeyA:
				t.selStart = 0
				t.selEnd = len(t.runes)
				t.caretPos = len(t.runes)
			case KeyC:
				// В режиме пароля копирование запрещено
				if !t.isPassword && t.selActive() {
					lo, hi := t.normSel()
					t.clipboard = make([]rune, hi-lo)
					copy(t.clipboard, t.runes[lo:hi])
				}
			case KeyX:
				// В режиме пароля вырезание запрещено
				if !t.isPassword && t.selActive() {
					lo, hi := t.normSel()
					t.clipboard = make([]rune, hi-lo)
					copy(t.clipboard, t.runes[lo:hi])
					t.deleteSel()
					changed = true
				}
			case KeyV:
				if len(t.clipboard) > 0 {
					t.deleteSel()
					n := len(t.clipboard)
					ins := make([]rune, len(t.runes)+n)
					copy(ins, t.runes[:t.caretPos])
					copy(ins[t.caretPos:], t.clipboard)
					copy(ins[t.caretPos+n:], t.runes[t.caretPos:])
					t.runes = ins
					t.caretPos += n
					changed = true
				}
			}
		} else if e.Rune >= 32 {
			t.deleteSel()
			ins := make([]rune, len(t.runes)+1)
			copy(ins, t.runes[:t.caretPos])
			ins[t.caretPos] = e.Rune
			copy(ins[t.caretPos+1:], t.runes[t.caretPos:])
			t.runes = ins
			t.caretPos++
			changed = true
		}
	}

	t.clampCaret()
	text := string(t.runes)
	onCh := t.OnChange
	t.mu.Unlock()

	if changed && onCh != nil {
		go onCh(text)
	}
}

// ─── MouseClickHandler ───────────────────────────────────────────────────────

func (t *TextInput) OnMouseButton(e MouseEvent) bool {
	if !t.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	b := t.bounds

	// Клик по кнопке-глазику (правая часть поля)
	if t.isPassword && e.X >= b.Max.X-eyeButtonWidth && e.X <= b.Max.X {
		t.showPassword = !t.showPassword
		return true
	}

	textX := b.Min.X + t.PaddingX - t.scrollX
	relX := e.X - textX

	pos := t.charPositions
	if len(pos) == 0 {
		return true
	}

	// Ищем ближайший символ к позиции клика
	best := len(pos) - 1
	for i := 0; i < len(pos)-1; i++ {
		mid := (pos[i] + pos[i+1]) / 2
		if relX <= mid {
			best = i
			break
		}
	}
	t.caretPos = best
	t.selStart = -1
	return true
}

// OnMouseMove обрабатывает hover для кнопки-глазика.
func (t *TextInput) OnMouseMove(x, y int) {
	t.mu.Lock()
	defer t.mu.Unlock()
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
	t.mu.Lock()
	runes := make([]rune, len(t.runes))
	copy(runes, t.runes)
	isFocused := t.focused
	caretPos := t.caretPos
	selStart := t.selStart
	selEnd := t.selEnd
	isPwd := t.isPassword
	showPwd := t.showPassword
	maskRune := t.MaskRune
	eyeHov := t.eyeHovered
	t.mu.Unlock()

	if maskRune == 0 {
		maskRune = '●'
	}

	b := t.bounds
	if b.Empty() {
		return
	}

	// Фон
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.Background)

	// Рамка
	if isFocused {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.FocusBorder)
		ctx.DrawHLine(b.Min.X, b.Max.Y-2, b.Dx(), t.FocusBorder)
	} else {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), t.BorderColor)
	}

	const sizePt = DefaultFontSizePt
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

	// Текст для отображения: маскированный или реальный
	displayText := string(runes)
	if isPwd && !showPwd && len(runes) > 0 {
		masked := make([]rune, len(runes))
		for i := range masked {
			masked[i] = maskRune
		}
		displayText = string(masked)
	}

	// Позиции символов (по отображаемому тексту)
	positions := ctx.MeasureRunePositions(displayText, sizePt) // len(runes)+1

	// Обновляем сохранённые позиции и scrollX
	t.mu.Lock()
	t.charPositions = positions

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
			ctx.FillRectAlpha(selX0, textY-1, selX1-selX0, textH+2, t.SelColor)
		}
		ctx.DrawText(displayText, textX, textY, t.TextColor)
	}

	// Мигающий курсор
	if isFocused {
		caretVisible := (time.Now().UnixMilli()/530)%2 == 0
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

// ─── Themeable ───────────────────────────────────────────────────────────────

func (t *TextInput) ApplyTheme(th *Theme) {
	t.Background = th.InputBG
	t.BorderColor = th.InputBorder
	t.FocusBorder = th.InputFocus
	t.TextColor = th.InputText
	t.PlaceColor = th.InputPlaceholder
	t.CaretColor = th.InputCaret
}
