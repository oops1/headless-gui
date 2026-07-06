package widget

import (
	"image"
	"strings"
)

// ─── Типы кнопок и результатов ──────────────────────────────────────────────

// MessageBoxButtons определяет набор кнопок в MessageBox.
type MessageBoxButtons int

const (
	MBOk          MessageBoxButtons = iota // только «OK»
	MBOkCancel                             // «OK» + «Отмена»
	MBYesNo                                // «Да» + «Нет»
	MBYesNoCancel                          // «Да» + «Нет» + «Отмена»
)

// MessageBoxResult — результат закрытия MessageBox.
type MessageBoxResult int

const (
	MBResultOK     MessageBoxResult = iota
	MBResultCancel
	MBResultYes
	MBResultNo
)

// ─── ModalShower ────────────────────────────────────────────────────────────

// ModalShower — интерфейс для показа/закрытия модальных виджетов.
// Реализуется engine.Engine, определён здесь чтобы избежать циклического импорта.
type ModalShower interface {
	ShowModal(m ModalWidget)
	CloseModal(m ModalWidget)
}

// ─── MessageBox ─────────────────────────────────────────────────────────────

// MessageBox предоставляет API для показа модальных диалогов в стиле WinForms.
//
// Использование:
//
//	mb := widget.NewMessageBox(eng)
//
//	// Простое сообщение с одной кнопкой OK:
//	mb.Show("Ошибка", "Файл не найден")
//
//	// С выбором:
//	mb.ShowDialog("Подтверждение", "Удалить файл?", widget.MBYesNo, func(r widget.MessageBoxResult) {
//	    if r == widget.MBResultYes { ... }
//	})
type MessageBox struct {
	eng ModalShower
}

// NewMessageBox создаёт MessageBox привязанный к движку.
func NewMessageBox(eng ModalShower) *MessageBox {
	return &MessageBox{eng: eng}
}

// Show показывает MessageBox с одной кнопкой «OK».
// Возвращает диалог (можно закрыть программно через eng.CloseModal).
func (mb *MessageBox) Show(caption, message string) *Dialog {
	return mb.ShowDialog(caption, message, MBOk, nil)
}

// ShowOKCancel показывает MessageBox с кнопками «OK» / «Отмена».
func (mb *MessageBox) ShowOKCancel(caption, message string, onResult func(MessageBoxResult)) *Dialog {
	return mb.ShowDialog(caption, message, MBOkCancel, onResult)
}

// ShowYesNo показывает MessageBox с кнопками «Да» / «Нет».
func (mb *MessageBox) ShowYesNo(caption, message string, onResult func(MessageBoxResult)) *Dialog {
	return mb.ShowDialog(caption, message, MBYesNo, onResult)
}

// ShowYesNoCancel показывает MessageBox с кнопками «Да» / «Нет» / «Отмена».
func (mb *MessageBox) ShowYesNoCancel(caption, message string, onResult func(MessageBoxResult)) *Dialog {
	return mb.ShowDialog(caption, message, MBYesNoCancel, onResult)
}

// ─── Пресеты severity (иконка + локализованный заголовок по умолчанию) ──────

// ShowInfo — информационное сообщение (синий значок «i», кнопка OK).
// Пустой caption заменяется локализованным «Информация».
func (mb *MessageBox) ShowInfo(caption, message string) *Dialog {
	return mb.ShowSeverity(caption, message, SeverityInfo, MBOk, nil)
}

// ShowWarning — предупреждение (оранжевый треугольник «!»).
func (mb *MessageBox) ShowWarning(caption, message string) *Dialog {
	return mb.ShowSeverity(caption, message, SeverityWarning, MBOk, nil)
}

// ShowError — ошибка (красный значок «✕»).
func (mb *MessageBox) ShowError(caption, message string) *Dialog {
	return mb.ShowSeverity(caption, message, SeverityError, MBOk, nil)
}

// ShowQuestion — вопрос Да/Нет (зелёный значок «?»).
func (mb *MessageBox) ShowQuestion(caption, message string, onResult func(MessageBoxResult)) *Dialog {
	return mb.ShowSeverity(caption, message, SeverityQuestion, MBYesNo, onResult)
}

// ShowDialog — полная версия без значка: caption, message, набор кнопок, callback.
func (mb *MessageBox) ShowDialog(caption, message string, buttons MessageBoxButtons, onResult func(MessageBoxResult)) *Dialog {
	return mb.ShowSeverity(caption, message, SeverityNone, buttons, onResult)
}

// ShowSeverity — самая полная версия: со значком severity.
// Пустой caption берётся из локализованного заголовка по severity.
//
// Компоновка — принятый дизайн-мокап: значок слева, первый абзац сообщения
// основным цветом, последующие абзацы (после «\n») — приглушённым, кнопки
// прижаты к правому нижнему углу.
func (mb *MessageBox) ShowSeverity(caption, message string, severity DialogSeverity, buttons MessageBoxButtons, onResult func(MessageBoxResult)) *Dialog {
	// ── Определяем размеры ──────────────────────────────────────────────
	const (
		padX       = dlgPad // горизонтальный отступ контента
		padTop     = 14     // отступ сообщения от заголовка
		lineH      = 19     // высота строки текста
		btnH       = 30     // высота кнопки
		btnGap     = 8      // зазор между кнопками
		btnPadBot  = 12     // отступ кнопок от нижнего края
		minW     = 280
		maxW     = 500
		titleH   = dlgTitleH
		msgFS    = 11 // размер шрифта сообщения (pt)
		iconSize = 32 // диаметр значка severity
		iconGap  = 14 // зазор между значком и текстом
	)

	// Заголовок: пустой → локализованный по severity.
	titleKey := ""
	if caption == "" {
		titleKey = severityTitleKey(severity)
		caption = Tr(titleKey)
	}

	// Отступ текста слева: со значком — уступаем ему место.
	textLeft := padX + 2
	if severity != SeverityNone {
		textLeft = padX + 2 + iconSize + iconGap
	}

	// Абзацы: первый — основной тон, последующие — приглушённый.
	// Перенос и ширина считаются точным замером текста (MeasureUIText),
	// поэтому строки не выходят за границы диалога.
	type msgLine struct {
		text  string
		muted bool
	}
	maxTextW := maxW - textLeft - padX
	var lines []msgLine
	for pi, para := range strings.Split(message, "\n") {
		for _, l := range wrapTextPx(para, maxTextW, msgFS) {
			lines = append(lines, msgLine{text: l, muted: pi > 0})
		}
	}
	if len(lines) == 0 {
		lines = []msgLine{{}}
	}
	msgH := len(lines) * lineH
	if msgH < iconSize {
		msgH = iconSize // не ниже значка
	}

	// Ширина — по самой длинной строке (замер), в пределах [minW, maxW].
	maxLineW := 0
	for _, l := range lines {
		if w := MeasureUIText(l.text, msgFS); w > maxLineW {
			maxLineW = w
		}
	}
	dlgW := textLeft + maxLineW + padX
	if dlgW < minW {
		dlgW = minW
	}
	if dlgW > maxW {
		dlgW = maxW
	}

	dlgH := titleH + padTop + msgH + 18 + btnH + btnPadBot

	dlg := NewDialog(caption, dlgW, dlgH)

	// ── Значок severity ──────────────────────────────────────────────────
	if severity != SeverityNone {
		icon := NewDialogIcon(severity)
		iy := titleH + padTop + 4
		icon.SetBounds(image.Rect(padX+2, iy, padX+2+iconSize, iy+iconSize))
		dlg.AddChild(icon)
	}

	// ── Метки для строк сообщения (вторичные абзацы — приглушённые) ──────
	var msgLabels []*Label
	for i, line := range lines {
		y := titleH + padTop + i*lineH
		r := image.Rect(textLeft, y, dlgW-padX, y+lineH)
		var lbl *Label
		if line.muted {
			lbl = newMutedLabel(line.text)
		} else {
			// Основной цвет текста темы (НЕ цвет заголовка: в классике
			// заголовок белый на синем, а тело диалога — светлое).
			lbl = NewLabel(line.text, win10.LabelText)
		}
		lbl.FontSize = msgFS
		lbl.SetBounds(r)
		dlg.AddChild(lbl)
		msgLabels = append(msgLabels, lbl)
	}

	// ── Кнопки (локализованные), прижаты к правому нижнему углу ──────────
	btnDefs := mbButtonDefs(buttons)
	widths := make([]int, len(btnDefs))
	totalBtnW := 0
	for i, def := range btnDefs {
		widths[i] = mbBtnWidth(Tr(def.key))
		totalBtnW += widths[i]
	}
	totalBtnW += (len(btnDefs) - 1) * btnGap
	bx := dlgW - padX - totalBtnW
	btnY := dlgH - btnPadBot - btnH

	type btnBind struct {
		btn *Button
		key string
	}
	var binds []btnBind
	for i, def := range btnDefs {
		btn := trBtn(def.key, def.accent)
		btn.SetBounds(image.Rect(bx, btnY, bx+widths[i], btnY+btnH))
		bx += widths[i] + btnGap

		result := def.result // capture для замыкания
		btn.OnClick = func() {
			mb.eng.CloseModal(dlg)
			if onResult != nil {
				onResult(result)
			}
		}
		dlg.AddChild(btn)
		binds = append(binds, btnBind{btn, def.key})
	}

	// Живая локализация: заголовок (если брался по severity) и подписи кнопок.
	dlg.OnLanguageChange(func() {
		if titleKey != "" {
			dlg.Title = Tr(titleKey)
		}
		for _, b := range binds {
			b.btn.SetText(Tr(b.key))
		}
	})

	// Enter → кнопка по умолчанию (первая accent); Ctrl+C → дамп в буфер.
	defResult, hasDefault := mbDefaultResult(btnDefs)
	if hasDefault {
		dlg.DefaultAction = func() {
			mb.eng.CloseModal(dlg)
			if onResult != nil {
				onResult(defResult)
			}
		}
	}
	dlg.CopyText = func() string {
		return messageBoxClipboard(dlg.Title, msgLabels, btnDefs)
	}

	mb.eng.ShowModal(dlg)
	return dlg
}

// severityTitleKey возвращает КЛЮЧ локализации заголовка по умолчанию.
func severityTitleKey(s DialogSeverity) string {
	switch s {
	case SeverityInfo:
		return "dlg.title.info"
	case SeverityQuestion:
		return "dlg.title.question"
	case SeverityWarning:
		return "dlg.title.warning"
	case SeverityError:
		return "dlg.title.error"
	}
	return ""
}

// mbBtnWidth — ширина кнопки под подпись (мин. 80, точный замер + поля).
func mbBtnWidth(label string) int {
	w := MeasureUIText(label, 11) + 28
	if w < 80 {
		w = 80
	}
	return w
}

// mbDefaultResult возвращает результат кнопки по умолчанию (первая accent).
func mbDefaultResult(defs []mbBtnDef) (MessageBoxResult, bool) {
	for _, d := range defs {
		if d.accent {
			return d.result, true
		}
	}
	return 0, false
}

// messageBoxClipboard формирует текст для Ctrl+C в формате Windows MessageBox:
// заголовок, разделители, текст, кнопки.
func messageBoxClipboard(title string, msg []*Label, btns []mbBtnDef) string {
	const sep = "---------------------------"
	var b strings.Builder
	b.WriteString(sep + "\r\n")
	b.WriteString(title + "\r\n")
	b.WriteString(sep + "\r\n")
	for i, l := range msg {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(l.Text())
	}
	b.WriteString("\r\n" + sep + "\r\n")
	for i, d := range btns {
		if i > 0 {
			b.WriteString("   ")
		}
		b.WriteString(Tr(d.key))
	}
	b.WriteString("\r\n" + sep + "\r\n")
	return b.String()
}

// ─── Внутренние типы ────────────────────────────────────────────────────────

type mbBtnDef struct {
	key    string // ключ локализации (напр. "dlg.ok")
	result MessageBoxResult
	accent bool // синяя кнопка (primary)
}

func mbButtonDefs(buttons MessageBoxButtons) []mbBtnDef {
	switch buttons {
	case MBOkCancel:
		return []mbBtnDef{
			{key: "dlg.ok", result: MBResultOK, accent: true},
			{key: "dlg.cancel", result: MBResultCancel},
		}
	case MBYesNo:
		return []mbBtnDef{
			{key: "dlg.yes", result: MBResultYes, accent: true},
			{key: "dlg.no", result: MBResultNo},
		}
	case MBYesNoCancel:
		return []mbBtnDef{
			{key: "dlg.yes", result: MBResultYes, accent: true},
			{key: "dlg.no", result: MBResultNo},
			{key: "dlg.cancel", result: MBResultCancel},
		}
	default: // MBOk
		return []mbBtnDef{
			{key: "dlg.ok", result: MBResultOK, accent: true},
		}
	}
}

