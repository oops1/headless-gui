package widget

import (
	"image"
	"sync/atomic"
)

// ProgressDialog — модальный диалог хода операции. Управляется из кода
// (в т.ч. из фоновой горутины: сеттеры потокобезопасны и самоинвалидируются).
//
// Поддерживает определённый прогресс (0..1) и неопределённый режим
// (бегущая полоса — ProgressBar сам анимирует indeterminate).
type ProgressDialog struct {
	eng    ModalShower
	dlg    *Dialog
	status *Label
	bar    *ProgressBar
	closed atomic.Bool

	onCancel func() // вызывается по кнопке Отмена / Escape (может быть nil)
}

// ShowProgress показывает диалог прогресса.
//
//	title    — заголовок (пустой → локализованный "Выполнение операции");
//	status   — начальная строка статуса;
//	onCancel — колбэк отмены (nil → кнопки Отмена нет).
func (mb *MessageBox) ShowProgress(title, status string, onCancel func()) *ProgressDialog {
	const (
		dlgW   = 420
		padX   = 18
		titleH = 32
		barH   = 12
		btnW   = 90
		btnH   = 30
		btnPad = 14
	)
	if title == "" {
		title = Tr("dlg.title.progress")
	}
	statusY := titleH + 16
	barY := statusY + 24
	dlgH := barY + barH + 20 + btnH + btnPad
	if onCancel == nil {
		dlgH = barY + barH + btnPad + 6
	}

	dlg := NewDialog(title, dlgW, dlgH)

	lbl := NewLabel(status, dlg.TitleColor)
	lbl.SetBounds(image.Rect(padX, statusY, dlgW-padX, statusY+18))
	dlg.AddChild(lbl)

	bar := NewProgressBar()
	bar.SetValue(0)
	bar.SetBounds(image.Rect(padX, barY, dlgW-padX, barY+barH))
	dlg.AddChild(bar)

	pd := &ProgressDialog{eng: mb.eng, dlg: dlg, status: lbl, bar: bar, onCancel: onCancel}

	if onCancel != nil {
		btnY := dlgH - btnPad - btnH
		cancelBtn := trBtn("dlg.cancel", false)
		cancelBtn.SetBounds(image.Rect(dlgW-padX-btnW, btnY, dlgW-padX, btnY+btnH))
		cancelBtn.OnClick = pd.cancel
		dlg.AddChild(cancelBtn)
		dlg.OnLanguageChange(func() { cancelBtn.SetText(Tr("dlg.cancel")) })
		dlg.CancelAction = pd.cancel
	}

	mb.eng.ShowModal(dlg)
	return pd
}

// SetProgress задаёт определённый прогресс 0..1 (потокобезопасно).
func (pd *ProgressDialog) SetProgress(v float64) {
	pd.bar.SetIndeterminate(false)
	pd.bar.SetValue(v)
}

// SetIndeterminate включает/выключает неопределённый режим (бегущая полоса).
func (pd *ProgressDialog) SetIndeterminate(on bool) {
	pd.bar.SetIndeterminate(on)
}

// SetStatus обновляет строку статуса (потокобезопасно).
func (pd *ProgressDialog) SetStatus(s string) {
	pd.status.SetText(s)
}

// Close закрывает диалог (идемпотентно; безопасно из любой горутины).
func (pd *ProgressDialog) Close() {
	if pd.closed.Swap(true) {
		return
	}
	pd.eng.CloseModal(pd.dlg)
}

// cancel — обработчик кнопки Отмена / Escape: закрывает и уведомляет.
func (pd *ProgressDialog) cancel() {
	if pd.closed.Swap(true) {
		return
	}
	pd.eng.CloseModal(pd.dlg)
	if pd.onCancel != nil {
		pd.onCancel()
	}
}
