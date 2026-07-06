package widget

import (
	"fmt"
	"image"
	"math"
	"sync/atomic"
)

// ProgressDialog — модальный диалог хода операции. Управляется из кода
// (в т.ч. из фоновой горутины: сеттеры потокобезопасны и самоинвалидируются).
//
// Компоновка — принятый дизайн-мокап: основная строка статуса, под ней
// приглушённая строка деталей (SetDetail), полоса прогресса и процент
// справа под ней; кнопка «Отмена» — в правом нижнем углу.
//
// Поддерживает определённый прогресс (0..1) и неопределённый режим
// (бегущая полоса — ProgressBar сам анимирует indeterminate).
type ProgressDialog struct {
	eng    ModalShower
	dlg    *Dialog
	status *Label
	detail *Label
	pct    *Label
	bar    *ProgressBar
	closed atomic.Bool

	onCancel func() // вызывается по кнопке Отмена / Escape / ✕ (может быть nil)
}

// ShowProgress показывает диалог прогресса.
//
//	title    — заголовок (пустой → локализованный "Выполнение операции");
//	status   — начальная строка статуса;
//	onCancel — колбэк отмены (nil → кнопки Отмена нет).
func (mb *MessageBox) ShowProgress(title, status string, onCancel func()) *ProgressDialog {
	const (
		dlgW   = 420
		padX   = dlgPad
		titleH = dlgTitleH
		barH   = 10
		btnH   = 30
		btnPad = 12
	)
	if title == "" {
		title = Tr("dlg.title.progress")
	}
	statusY := titleH + 12
	detailY := statusY + 20
	barY := detailY + 26
	pctY := barY + barH + 6
	dlgH := pctY + 18 + btnH + btnPad
	if onCancel == nil {
		dlgH = pctY + 18 + btnPad
	}

	dlg := NewDialog(title, dlgW, dlgH)
	dlg.ShowCloseButton = onCancel != nil // без отмены операция незакрываема

	lbl := NewLabel(status, win10.LabelText)
	lbl.FontSize = 11
	lbl.SetBounds(image.Rect(padX, statusY, dlgW-padX, statusY+18))
	dlg.AddChild(lbl)

	det := newMutedLabel("")
	det.FontSize = 10
	det.SetBounds(image.Rect(padX, detailY, dlgW-padX, detailY+16))
	dlg.AddChild(det)

	bar := NewProgressBar()
	bar.SetValue(0)
	bar.SetBounds(image.Rect(padX, barY, dlgW-padX, barY+barH))
	dlg.AddChild(bar)

	pct := newMutedLabel("")
	pct.FontSize = 10
	pct.SetBounds(image.Rect(dlgW-padX-44, pctY, dlgW-padX, pctY+14))
	dlg.AddChild(pct)

	pd := &ProgressDialog{eng: mb.eng, dlg: dlg, status: lbl, detail: det, pct: pct, bar: bar, onCancel: onCancel}

	if onCancel != nil {
		btnY := dlgH - btnPad - btnH
		cw := mbBtnWidth(Tr("dlg.cancel"))
		cancelBtn := trBtn("dlg.cancel", false)
		cancelBtn.SetBounds(image.Rect(dlgW-padX-cw, btnY, dlgW-padX, btnY+btnH))
		cancelBtn.OnClick = pd.cancel
		dlg.AddChild(cancelBtn)
		dlg.OnLanguageChange(func() { cancelBtn.SetText(Tr("dlg.cancel")) })
		dlg.CancelAction = pd.cancel
	}

	mb.eng.ShowModal(dlg)
	return pd
}

// SetProgress задаёт определённый прогресс 0..1 (потокобезопасно).
// Обновляет и подпись процента под полосой.
func (pd *ProgressDialog) SetProgress(v float64) {
	pd.bar.SetIndeterminate(false)
	pd.bar.SetValue(v)
	pd.pct.SetText(fmt.Sprintf("%d%%", int(math.Round(max01(v)*100))))
}

// SetIndeterminate включает/выключает неопределённый режим (бегущая полоса).
// В неопределённом режиме процент скрывается.
func (pd *ProgressDialog) SetIndeterminate(on bool) {
	pd.bar.SetIndeterminate(on)
	if on {
		pd.pct.SetText("")
	}
}

// SetStatus обновляет основную строку статуса (потокобезопасно).
func (pd *ProgressDialog) SetStatus(s string) {
	pd.status.SetText(s)
}

// SetDetail обновляет приглушённую строку деталей под статусом
// («34 из 120 файлов · 61,4 МБ/с · осталось 0:42»). Потокобезопасно.
func (pd *ProgressDialog) SetDetail(s string) {
	pd.detail.SetText(s)
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
