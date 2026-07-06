package widget

import "image"

// InputDialog — модальный диалог ввода строки (замена OS prompt).
// Все элементы — стандартные виджеты, поэтому диалог темизируется движком
// автоматически; подписи локализованы (dlg.* ключи).
//
// Под полем зарезервирована строка подсказки (серая, см. SetHint); при
// ошибке валидации в ней показывается сообщение цветом ошибки.
//
// Enter подтверждает (если валидация проходит), Escape отменяет.
type InputDialog struct {
	eng   ModalShower
	dlg   *Dialog
	input *TextInput
	hint  *Label

	hintText string // постоянная подсказка (возвращается после ошибки)
	validate func(string) string        // "" — ок, иначе текст ошибки
	onResult func(text string, ok bool) // ok=false при отмене
}

// ShowInput показывает диалог ввода.
//
//	title    — заголовок (пустой → локализованный "Ввод");
//	label    — подпись над полем;
//	initial  — начальный текст поля;
//	validate — проверка значения (возврат "" = ок, иначе сообщение),
//	           может быть nil;
//	onResult — результат: (текст, ok). ok=false при Отмена/Escape/✕.
func (mb *MessageBox) ShowInput(title, label, initial string, validate func(string) string, onResult func(text string, ok bool)) *InputDialog {
	const (
		dlgW   = 380
		padX   = dlgPad
		titleH = dlgTitleH
		fieldH = 30
		btnH   = 30
		btnGap = 8
		btnPad = 12
	)
	if title == "" {
		title = Tr("dlg.title.input")
	}
	labelY := titleH + 12
	fieldY := labelY + 24
	hintY := fieldY + fieldH + 8
	dlgH := hintY + 22 + btnH + btnPad

	dlg := NewDialog(title, dlgW, dlgH)

	lbl := newMutedLabel(label)
	lbl.FontSize = 11
	lbl.SetBounds(image.Rect(padX, labelY, dlgW-padX, labelY+18))
	dlg.AddChild(lbl)

	field := NewTextInput("")
	field.SetText(initial)
	field.SetBounds(image.Rect(padX, fieldY, dlgW-padX, fieldY+fieldH))
	dlg.AddChild(field)

	hint := newMutedLabel("")
	hint.FontSize = 10
	hint.SetBounds(image.Rect(padX, hintY, dlgW-padX, hintY+16))
	dlg.AddChild(hint)

	id := &InputDialog{eng: mb.eng, dlg: dlg, input: field, hint: hint, validate: validate, onResult: onResult}

	// Кнопки OK / Отмена — правый нижний угол.
	btnY := dlgH - btnPad - btnH
	okW := mbBtnWidth(Tr("dlg.ok"))
	cancelW := mbBtnWidth(Tr("dlg.cancel"))
	okBtn := trBtn("dlg.ok", true)
	okBtn.SetBounds(image.Rect(dlgW-padX-cancelW-btnGap-okW, btnY, dlgW-padX-cancelW-btnGap, btnY+btnH))
	okBtn.OnClick = id.confirm
	dlg.AddChild(okBtn)

	cancelBtn := trBtn("dlg.cancel", false)
	cancelBtn.SetBounds(image.Rect(dlgW-padX-cancelW, btnY, dlgW-padX, btnY+btnH))
	cancelBtn.OnClick = func() {
		mb.eng.CloseModal(dlg)
		if onResult != nil {
			onResult("", false)
		}
	}
	dlg.AddChild(cancelBtn)

	dlg.OnLanguageChange(func() {
		okBtn.SetText(Tr("dlg.ok"))
		cancelBtn.SetText(Tr("dlg.cancel"))
	})

	// Enter подтверждает; Escape отменяет (движок закрывает модалку, а
	// CancelAction сообщает результат отмены).
	dlg.DefaultAction = id.confirm
	dlg.CancelAction = func() {
		if onResult != nil {
			onResult("", false)
		}
	}

	mb.eng.ShowModal(dlg)
	if f, ok := mb.eng.(interface{ SetFocus(Widget) }); ok {
		f.SetFocus(field)
	}
	return id
}

// Dialog возвращает базовый модальный диалог (интроспекция/автоматизация).
func (id *InputDialog) Dialog() *Dialog { return id.dlg }

// SetHint задаёт постоянную серую подсказку под полем (например, правило
// допустимых имён). При ошибке валидации подсказка временно заменяется
// сообщением об ошибке.
func (id *InputDialog) SetHint(text string) {
	id.hintText = text
	id.hint.TextColor = win10.InputPlaceholder
	id.hint.SetText(text)
}

// confirm валидирует и, если ок, закрывает диалог с результатом.
func (id *InputDialog) confirm() {
	text := id.input.GetText()
	if id.validate != nil {
		if msg := id.validate(text); msg != "" {
			id.hint.TextColor = severityColor(SeverityError)
			id.hint.SetText(msg)
			id.input.SetValidationError(msg)
			return
		}
	}
	id.eng.CloseModal(id.dlg)
	if id.onResult != nil {
		id.onResult(text, true)
	}
}
