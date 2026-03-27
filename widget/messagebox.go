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

// ShowDialog — полная версия: caption, message, набор кнопок, callback.
func (mb *MessageBox) ShowDialog(caption, message string, buttons MessageBoxButtons, onResult func(MessageBoxResult)) *Dialog {
	// ── Определяем размеры ──────────────────────────────────────────────
	const (
		padX        = 20  // горизонтальный отступ текста
		padTop      = 16  // отступ сообщения от заголовка
		lineH       = 18  // высота строки текста
		btnW        = 90  // ширина кнопки
		btnH        = 32  // высота кнопки
		btnGap      = 10  // зазор между кнопками
		btnPadBot   = 14  // отступ кнопок от нижнего края
		minW        = 300
		maxW        = 500
		titleH      = 32
		maxLineLen  = 60 // символов на строку для переноса
	)

	// Переносим длинные строки
	lines := wrapText(message, maxLineLen)
	msgH := len(lines) * lineH
	if msgH < lineH {
		msgH = lineH
	}

	// Ширина: максимальная строка * ~7px или минимум
	maxLine := 0
	for _, l := range lines {
		if len([]rune(l)) > maxLine {
			maxLine = len([]rune(l))
		}
	}
	dlgW := maxLine*7 + padX*2
	if dlgW < minW {
		dlgW = minW
	}
	if dlgW > maxW {
		dlgW = maxW
	}

	dlgH := titleH + padTop + msgH + 16 + btnH + btnPadBot

	dlg := NewDialog(caption, dlgW, dlgH)

	// ── Метки для каждой строки сообщения ────────────────────────────────
	for i, line := range lines {
		lbl := NewLabel(line, dlg.TitleColor)
		y := titleH + padTop + i*lineH
		lbl.SetBounds(image.Rect(padX, y, dlgW-padX, y+lineH))
		dlg.AddChild(lbl)
	}

	// ── Кнопки ──────────────────────────────────────────────────────────
	btnDefs := mbButtonDefs(buttons)
	totalBtnW := len(btnDefs)*btnW + (len(btnDefs)-1)*btnGap
	startX := (dlgW - totalBtnW) / 2
	btnY := dlgH - btnPadBot - btnH

	for i, def := range btnDefs {
		bx := startX + i*(btnW+btnGap)
		var btn *Button
		if def.accent {
			btn = NewWin10AccentButton(def.text)
		} else {
			btn = NewButton(def.text)
		}
		btn.SetBounds(image.Rect(bx, btnY, bx+btnW, btnY+btnH))

		result := def.result // capture для замыкания
		btn.OnClick = func() {
			mb.eng.CloseModal(dlg)
			if onResult != nil {
				onResult(result)
			}
		}
		dlg.AddChild(btn)
	}

	mb.eng.ShowModal(dlg)
	return dlg
}

// ─── Внутренние типы ────────────────────────────────────────────────────────

type mbBtnDef struct {
	text   string
	result MessageBoxResult
	accent bool // синяя кнопка (primary)
}

func mbButtonDefs(buttons MessageBoxButtons) []mbBtnDef {
	switch buttons {
	case MBOkCancel:
		return []mbBtnDef{
			{text: "OK", result: MBResultOK, accent: true},
			{text: "Отмена", result: MBResultCancel},
		}
	case MBYesNo:
		return []mbBtnDef{
			{text: "Да", result: MBResultYes, accent: true},
			{text: "Нет", result: MBResultNo},
		}
	case MBYesNoCancel:
		return []mbBtnDef{
			{text: "Да", result: MBResultYes, accent: true},
			{text: "Нет", result: MBResultNo},
			{text: "Отмена", result: MBResultCancel},
		}
	default: // MBOk
		return []mbBtnDef{
			{text: "OK", result: MBResultOK, accent: true},
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
