package widget

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
)

// IconPosition определяет расположение иконки относительно текста.
type IconPosition int

const (
	// IconLeft — иконка слева от текста (по умолчанию).
	IconLeft IconPosition = iota
	// IconTop — иконка над текстом.
	IconTop
	// IconOnly — только иконка, текст не отображается.
	IconOnly
)

// Button — кнопка в стиле Windows 10 Dark.
// Pressed и Hovered меняются атомарно — можно вызывать из любой горутины.
type Button struct {
	Base

	Text          string
	TextColor     color.RGBA
	Background    color.RGBA
	HoverBG       color.RGBA // фон при наведении курсора
	PressedBG     color.RGBA
	BorderColor   color.RGBA
	HighlightTop  color.RGBA // 1-пиксельная акцентная линия сверху
	ShowHighlight bool

	// Icon — иконка кнопки (PNG/JPEG). Если не nil, рисуется рядом с текстом.
	Icon     image.Image
	IconPath string       // путь к файлу (заполняется из XAML атрибута Icon=)
	IconPos  IconPosition // расположение иконки (Left, Top, IconOnly)
	IconSize int          // размер иконки в пикселях (0 = авто: высота кнопки - 8)

	// FontSize — кегль подписи в пунктах. 0 означает общий размер
	// (DefaultFontSizePt).
	//
	// Своё поле, а не общий размер на всё приложение: окно с плотной
	// типографикой — заголовок раздела, названия групп, подписи, пояснения —
	// собирается из виджетов разных кеглей, и менять DefaultFontSizePt ради
	// одного окна значит переверстать все остальные.
	FontSize float64

	// Padding — внутренние отступы (WPF Padding: Left,Top,Right,Bottom).
	// (ToolTip унаследован из Base — единый для всех виджетов.)
	Padding Margin
	// CornerRadius — радиус скругления углов (0 = прямые).
	CornerRadius int

	pressed int32 // 0 | 1, атомарно
	hovered int32 // 0 | 1, атомарно
	focused int32 // 0 | 1, атомарно

	// isToggle/checked — кнопка-переключатель (togglebutton.go).
	// Атомарные, как и остальное состояние кнопки: вид её спрашивают из
	// отрисовки, а меняют из обработки ввода.
	isToggle int32
	checked  int32

	// OnCheckedChanged — обработчик смены состояния переключателя
	// (см. SetToggle). Зовётся ПОСЛЕ OnClick и только у переключателя.
	OnCheckedChanged func(checked bool)

	// OnClick — основной обработчик клика (back-compat).
	// Если нужно несколько подписчиков — используйте AddClickHandler /
	// RemoveClickHandler. OnClick всегда вызывается ДО handlers.
	OnClick func()

	// Command — команда (WPF ICommand), выполняется при клике, если CanExecute.
	Command          ICommand
	CommandParameter interface{}

	// handlersMu защищает clickHandlers; модификация подписки и
	// вызов из разных goroutine (OnMouseButton/OnKeyEvent) безопасны.
	handlersMu    sync.Mutex
	clickHandlers []clickHandler
	nextHandlerID uint64
}

// clickHandler — внутренняя обёртка над пользовательским колбэком,
// хранящая стабильный ID для последующего RemoveClickHandler.
type clickHandler struct {
	id uint64
	fn func()
}

// SetText задаёт текст кнопки (для биндингов и программного обновления).
func (b *Button) SetText(s string) {
	if b.Text == s {
		return
	}
	b.Text = s
	b.Invalidate()
}

// GetText возвращает текущий текст кнопки.
func (b *Button) GetText() string { return b.Text }

// NewButton создаёт кнопку в стиле Windows 10 Dark.
func NewButton(text string) *Button {
	return &Button{
		Text:         text,
		TextColor:    win10.BtnText,
		Background:   win10.BtnBG,
		HoverBG:      color.RGBA{R: 62, G: 62, B: 64, A: 255},
		PressedBG:    win10.BtnPressedBG,
		BorderColor:  win10.BtnBorder,
		HighlightTop: win10.Accent,
	}
}

// NewWin10AccentButton создаёт кнопку с синим акцентным фоном («primary action»).
func NewWin10AccentButton(text string) *Button {
	return &Button{
		Text:        text,
		TextColor:   color.RGBA{R: 255, G: 255, B: 255, A: 255},
		Background:  win10.Accent,
		HoverBG:     color.RGBA{R: 0, G: 99, B: 177, A: 255},
		PressedBG:   color.RGBA{R: 0, G: 84, B: 153, A: 255},
		BorderColor: color.RGBA{R: 0, G: 84, B: 153, A: 255},
	}
}

// SetPressed потокобезопасно меняет состояние нажатия.
// При фактическом изменении инвалидирует область кнопки (авто-damage).
func (btn *Button) SetPressed(v bool) {
	if atomic.SwapInt32(&btn.pressed, b2i(v)) != b2i(v) {
		btn.Invalidate()
	}
}

// IsPressed возвращает текущее состояние нажатия.
func (btn *Button) IsPressed() bool {
	return atomic.LoadInt32(&btn.pressed) == 1
}

// SetHovered потокобезопасно меняет состояние наведения.
// При фактическом изменении инвалидирует область кнопки (авто-damage).
func (btn *Button) SetHovered(v bool) {
	if atomic.SwapInt32(&btn.hovered, b2i(v)) != b2i(v) {
		btn.Invalidate()
	}
}

// IsHovered возвращает true если курсор над кнопкой.
func (btn *Button) IsHovered() bool {
	return atomic.LoadInt32(&btn.hovered) == 1
}

// OnMouseMove реализует MouseMoveHandler — обновляет hover-состояние.
func (btn *Button) OnMouseMove(x, y int) {
	if !btn.IsEnabled() {
		btn.SetHovered(false)
		return
	}
	btn.SetHovered(image.Pt(x, y).In(btn.bounds))
}

func (btn *Button) Draw(ctx DrawContext) {
	b := btn.bounds
	if b.Empty() {
		return
	}

	st := currentStyle()

	bg, txt, border := btn.Background, btn.TextColor, btn.BorderColor
	switch {
	case btn.IsPressed():
		bg = btn.PressedBG
	case btn.IsChecked():
		// Включённый переключатель выглядит нажатым постоянно. Наведение на
		// него всё равно должно быть заметно — иначе непонятно, на что
		// нажимаешь, — поэтому фон слегка ведётся в сторону hover, а не
		// подменяется им: подмена стёрла бы само состояние.
		bg = btn.PressedBG
		if btn.IsHovered() && btn.HoverBG.A > 0 && !st.Classic3D {
			bg = mixRGBA(btn.PressedBG, btn.HoverBG, 0.35)
		}
	case btn.IsHovered() && btn.HoverBG.A > 0 && !st.Classic3D:
		bg = btn.HoverBG // классика Win2000 не подсвечивает hover
	}
	if !btn.IsEnabled() {
		bg, txt, border = disabledLook(bg, txt, border)
	}

	switch {
	case st.Classic3D:
		// Классическая объёмная кнопка: прямые углы, bevel-рамка;
		// нажатие показывается инверсией граней (sunken).
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), bg)
		// Классика: включённый переключатель — вдавленная грань, как у
		// нажатой кнопки. Иначе в теме Win2000 состояние не видно вовсе:
		// hover она не подсвечивает, и фон у обоих состояний один.
		if btn.IsPressed() || btn.IsChecked() {
			drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
		} else {
			drawBevelRaised(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
		}
		if btn.IsFocused() {
			// Пунктирная рамка фокуса (классика Win2000).
			drawDottedRect(ctx, b.Min.X+3, b.Min.Y+3, b.Dx()-6, b.Dy()-6, st.BevelDark)
		}
	default:
		// Скругление: приоритет у явного CornerRadius (XAML), иначе — тема.
		cr := btn.CornerRadius
		if cr == 0 {
			cr = st.ControlCorner
		}
		if cr > 0 {
			ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, bg)
			if btn.IsFocused() {
				ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, btn.HighlightTop)
			} else if border.A > 0 {
				ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, border)
			}
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), bg)
			if btn.IsFocused() {
				ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), btn.HighlightTop)
			} else {
				ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), border)
			}
		}
	}

	if btn.ShowHighlight && !btn.IsPressed() && !st.Classic3D {
		ctx.DrawHLine(b.Min.X+1, b.Min.Y, b.Dx()-2, btn.HighlightTop)
	}

	// ── Размер иконки ──────────────────────────────────────────────────────
	iconSz := btn.IconSize
	if iconSz <= 0 {
		iconSz = b.Dy() - 8 // авто: высота кнопки − 8px padding
		if iconSz < 12 {
			iconSz = 12
		}
	}
	hasIcon := btn.Icon != nil
	hasText := btn.Text != "" && btn.IconPos != IconOnly

	const textH = 13
	sizePt := fontSizeOrDefault(btn.FontSize)
	iconGap := 4 // зазор между иконкой и текстом

	// Подпись усекается многоточием по доступной ширине.
	//
	// Без этого длинный текст просто вылезал за край кнопки и наезжал на
	// соседний виджет: клиппинга у кнопки нет, а ширину подписи задаёт
	// таблица строк — то есть язык, который выбрал пользователь, а не
	// разметка. Ровно этим приёмом пользуются DockPane и Dropdown.
	label := btn.Text
	textW := 0
	if hasText {
		avail := b.Dx() - 8
		if hasIcon && btn.IconPos == IconLeft {
			avail -= iconSz + iconGap
		}
		label = ellipsizeText(ctx, label, avail, sizePt)
		textW = ctx.MeasureText(label, sizePt)
	}

	// ── Расположение контента ───────────────────────────────────────────
	switch {
	case hasIcon && hasText && btn.IconPos == IconLeft:
		// Иконка слева, текст справа — оба центрированы по вертикали
		totalW := iconSz + iconGap + textW
		startX := b.Min.X + (b.Dx()-totalW)/2
		if startX < b.Min.X+4 {
			startX = b.Min.X + 4
		}
		iconY := b.Min.Y + (b.Dy()-iconSz)/2
		ctx.DrawImageScaled(btn.Icon, startX, iconY, iconSz, iconSz)
		textX := startX + iconSz + iconGap
		textY := b.Min.Y + (b.Dy()-textH)/2
		ctx.DrawTextSize(label, textX, textY, sizePt, txt)

	case hasIcon && hasText && btn.IconPos == IconTop:
		// Иконка сверху, текст снизу — оба центрированы по горизонтали
		totalH := iconSz + iconGap + textH
		startY := b.Min.Y + (b.Dy()-totalH)/2
		if startY < b.Min.Y+2 {
			startY = b.Min.Y + 2
		}
		iconX := b.Min.X + (b.Dx()-iconSz)/2
		ctx.DrawImageScaled(btn.Icon, iconX, startY, iconSz, iconSz)
		textX := b.Min.X + (b.Dx()-textW)/2
		textY := startY + iconSz + iconGap
		ctx.DrawTextSize(label, textX, textY, sizePt, txt)

	case hasIcon && !hasText:
		// Только иконка — центрирована
		iconX := b.Min.X + (b.Dx()-iconSz)/2
		iconY := b.Min.Y + (b.Dy()-iconSz)/2
		ctx.DrawImageScaled(btn.Icon, iconX, iconY, iconSz, iconSz)

	default:
		// Только текст (или нет ничего)
		textX := b.Min.X + (b.Dx()-textW)/2
		textY := b.Min.Y + (b.Dy()-textH)/2
		if textX < b.Min.X+4 {
			textX = b.Min.X + 4
		}
		if textY < b.Min.Y+2 {
			textY = b.Min.Y + 2
		}
		if hasText {
			ctx.DrawTextSize(label, textX, textY, sizePt, txt)
		}
	}

	btn.drawChildren(ctx)
}

// disabledLook приглушает цвета выключенной кнопки.
//
// Заменяет собой общий полупрозрачный оверлей (Base.drawDisabledOverlay). Тот
// красит виджет ЧЁРНЫМ на 55% поверх всего и прямоугольником: в тёмной теме
// это читается как «погашено», а в светлой выключенная кнопка выходила темнее
// и контрастнее соседней рабочей — притягивала взгляд и читалась нажатой, — да
// ещё и с прямыми углами поверх скруглённых.
//
// Гасить — значит уводить К СОБСТВЕННОМУ фону, а не к чёрному: тёмная кнопка
// темнеет, светлая светлеет, и в обеих темах выключенная отступает назад.
// Главный признак при этом даёт текст: он наполовину растворяется в фоне
// кнопки — так выключенное показывают и WPF, и сама Windows.
func disabledLook(bg, text, border color.RGBA) (color.RGBA, color.RGBA, color.RGBA) {
	ground := color.RGBA{A: 255} // чёрный — для тёмного фона
	if luminance(bg) >= 128 {
		ground = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	bg = mixRGBA(bg, ground, 0.18)
	text = mixRGBA(text, bg, 0.55)
	if border.A > 0 {
		border = mixRGBA(border, bg, 0.5)
	}
	return bg, text, border
}

// luminance — воспринимаемая яркость цвета (ITU-R BT.601).
func luminance(c color.RGBA) int {
	return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
}

// OnMouseButton реализует widget.MouseClickHandler — вызывает OnClick при отпускании.
//
// WPF-совместимое поведение: OnClick срабатывает только если press был
// на этой же кнопке. Без этой проверки release от закрытого окна/диалога
// может «пролететь» к кнопке, оказавшейся под курсором.
func (btn *Button) OnMouseButton(e MouseEvent) bool {
	if !btn.IsEnabled() {
		return false
	}
	if e.Button == MouseLeft {
		if e.Pressed {
			btn.SetPressed(true)
			return true
		}
		// Release: стреляем OnClick только если press был на нас.
		wasPressed := btn.IsPressed()
		btn.SetPressed(false)
		if wasPressed {
			btn.fireClick()
		}
		// Поглощаем release только если мы «владели» этим кликом.
		// Иначе — пропускаем дальше (bubbling).
		return wasPressed
	}
	return false
}


// AddClickHandler подписывает дополнительный обработчик на клик кнопки.
// Возвращает идентификатор, по которому подписку можно снять
// через RemoveClickHandler. OnClick (поле) сохраняется для
// обратной совместимости и вызывается всегда первым.
//
// Все обработчики вызываются синхронно в порядке регистрации.
// Безопасно вызывать из любой goroutine.
func (btn *Button) AddClickHandler(fn func()) uint64 {
	if fn == nil {
		return 0
	}
	btn.handlersMu.Lock()
	defer btn.handlersMu.Unlock()
	btn.nextHandlerID++
	id := btn.nextHandlerID
	btn.clickHandlers = append(btn.clickHandlers, clickHandler{id: id, fn: fn})
	return id
}

// RemoveClickHandler удаляет ранее зарегистрированный обработчик по id.
// Возвращает true, если обработчик был найден и удалён.
func (btn *Button) RemoveClickHandler(id uint64) bool {
	if id == 0 {
		return false
	}
	btn.handlersMu.Lock()
	defer btn.handlersMu.Unlock()
	for i, h := range btn.clickHandlers {
		if h.id == id {
			btn.clickHandlers = append(btn.clickHandlers[:i], btn.clickHandlers[i+1:]...)
			return true
		}
	}
	return false
}

// ClearClickHandlers удаляет всех подписчиков, добавленных через AddClickHandler.
// Поле OnClick не трогается.
func (btn *Button) ClearClickHandlers() {
	btn.handlersMu.Lock()
	defer btn.handlersMu.Unlock()
	btn.clickHandlers = nil
}

// fireClick вызывает OnClick и всех зарегистрированных подписчиков.
// Снапшот списка снимается под Mutex, чтобы безопасно итерировать
// при возможной отписке внутри обработчика.
func (btn *Button) fireClick() {
	if btn.OnClick != nil {
		btn.OnClick()
	}
	btn.handlersMu.Lock()
	handlers := make([]clickHandler, len(btn.clickHandlers))
	copy(handlers, btn.clickHandlers)
	btn.handlersMu.Unlock()
	for _, h := range handlers {
		if h.fn != nil {
			h.fn()
		}
	}
	// WPF Command: выполняется при клике, если доступна.
	if btn.Command != nil && btn.Command.CanExecute(btn.CommandParameter) {
		btn.Command.Execute(btn.CommandParameter)
	}
	// Переключатель меняет состояние ЗДЕСЬ, а не в обработке мыши: клавиша
	// Enter/Space активирует кнопку тем же путём, и состояние обязано
	// меняться одинаково — иначе переключатель слушался бы только мыши.
	if btn.IsToggle() {
		btn.Toggle()
	}
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (btn *Button) SetFocused(v bool) {
	if atomic.SwapInt32(&btn.focused, b2i(v)) != b2i(v) {
		btn.Invalidate()
	}
}

func (btn *Button) IsFocused() bool {
	return atomic.LoadInt32(&btn.focused) == 1
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

// OnKeyEvent — Enter или Space активируют кнопку.
//
// Поведение совпадает с mouse-path (см. OnMouseButton): обработчики вызываются
// синхронно. Раньше keyboard-путь стартовал OnClick в новой goroutine
// (`go btn.OnClick()`), что давало разную модель исполнения для одинакового
// API-события — пользовательский код OnClick был обязан быть thread-safe
// только из-за одного нестандартного кодового пути. Теперь — единая модель.
func (btn *Button) OnKeyEvent(e KeyEvent) {
	if !btn.IsEnabled() || !e.Pressed {
		return
	}
	if e.Code == KeyEnter || e.Code == KeySpace {
		btn.fireClick()
	}
}

// ApplyTheme обновляет цвета кнопки в соответствии с темой.
func (btn *Button) ApplyTheme(t *Theme) {
	btn.TextColor = t.BtnText
	btn.Background = t.BtnBG
	btn.HoverBG = t.BtnHoverBG
	btn.PressedBG = t.BtnPressedBG
	btn.BorderColor = t.BtnBorder
	btn.HighlightTop = t.Accent
}
