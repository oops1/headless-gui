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
func (mb *MessageBox) ShowSeverity(caption, message string, severity DialogSeverity, buttons MessageBoxButtons, onResult func(MessageBoxResult)) *Dialog {
	// ── Определяем размеры ──────────────────────────────────────────────
	const (
		padX       = 20 // горизонтальный отступ текста
		padTop     = 16 // отступ сообщения от заголовка
		lineH      = 18 // высота строки текста
		btnW       = 90 // ширина кнопки
		btnH       = 32 // высота кнопки
		btnGap     = 10 // зазор между кнопками
		btnPadBot  = 14 // отступ кнопок от нижнего края
		minW       = 300
		maxW       = 500
		titleH     = 32
		maxLineLen = 60 // символов на строку для переноса
		iconSize   = 32 // диаметр значка severity
		iconGap    = 16 // зазор между значком и текстом
	)

	// Заголовок: пустой → локализованный по severity.
	titleKey := ""
	if caption == "" {
		titleKey = severityTitleKey(severity)
		caption = Tr(titleKey)
	}

	// Отступ текста слева: со значком — уступаем ему место.
	textLeft := padX
	if severity != SeverityNone {
		textLeft = padX + iconSize + iconGap
	}

	// Переносим длинные строки
	lines := wrapText(message, maxLineLen)
	msgH := len(lines) * lineH
	if msgH < iconSize {
		msgH = iconSize // не ниже значка
	}

	// Ширина: максимальная строка * ~7px или минимум
	maxLine := 0
	for _, l := range lines {
		if len([]rune(l)) > maxLine {
			maxLine = len([]rune(l))
		}
	}
	dlgW := textLeft + maxLine*7 + padX
	if dlgW < minW {
		dlgW = minW
	}
	if dlgW > maxW {
		dlgW = maxW
	}

	dlgH := titleH + padTop + msgH + 16 + btnH + btnPadBot

	dlg := NewDialog(caption, dlgW, dlgH)

	// ── Значок severity ──────────────────────────────────────────────────
	if severity != SeverityNone {
		icon := NewDialogIcon(severity)
		iy := titleH + padTop
		icon.SetBounds(image.Rect(padX, iy, padX+iconSize, iy+iconSize))
		dlg.AddChild(icon)
	}

	// ── Метки для каждой строки сообщения ────────────────────────────────
	var msgLabels []*Label
	for i, line := range lines {
		lbl := NewLabel(line, dlg.TitleColor)
		y := titleH + padTop + i*lineH
		lbl.SetBounds(image.Rect(textLeft, y, dlgW-padX, y+lineH))
		dlg.AddChild(lbl)
		msgLabels = append(msgLabels, lbl)
	}

	// ── Кнопки (локализованные, живо переобновляются при смене языка) ────
	btnDefs := mbButtonDefs(buttons)
	totalBtnW := len(btnDefs)*btnW + (len(btnDefs)-1)*btnGap
	startX := (dlgW - totalBtnW) / 2
	btnY := dlgH - btnPadBot - btnH

	type btnBind struct {
		btn *Button
		key string
	}
	var binds []btnBind
	for i, def := range btnDefs {
		bx := startX + i*(btnW+btnGap)
		btn := trBtn(def.key, def.accent)
		btn.SetBounds(image.Rect(bx, btnY, bx+btnW, btnY+btnH))

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

// ─── Перенос текста ─────────────────────────────────────────────────────────

// wrapText разбивает текст на строки длиной не более maxRunes символов.
// Переносит по пробелам. Явные \n тоже учитываются.
func wrapText(text string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = 60
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len([]rune(line))+1+len([]rune(w)) > maxRunes {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, line)
	}
	if len(result) == 0 {
		result = []string{""}
	}
	return result
}
