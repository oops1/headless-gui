// dialog_busy.go — диалог ожидания: «идёт обработка, подождите».
//
// В отличие от ProgressDialog (строка статуса, детали, процент, «Отмена»)
// здесь всё по центру и без цифр: заголовок, подпись, полоса и нижняя
// строка-предупреждение. Такой диалог показывают, когда пользователю нечего
// решать — только дождаться, — и лишние подробности только мешают.
//
// Полоса — ProgressStyleGlow: в современных темах светящаяся голова со
// следом, в классике Win2000 и в Mac-теме сама полоса остаётся канонической
// для темы (см. progressbar_glow.go).
package widget

import "image"

// BusyDialog — открытый диалог ожидания. Все сеттеры потокобезопасны и
// вызываются в том числе из фоновой горутины, которая делает работу.
type BusyDialog struct {
	*ProgressDialog

	titleLbl *Label
	hint     *Label
}

// Геометрия диалога ожидания.
const (
	busyDlgW     = 520
	busyDlgH     = 226
	busyTitleY   = 46 // от верха корпуса
	busySubtitle = 82
	busyBarY     = 122
	busyBarH     = 16
	busyHintY    = 166
	busyBarPadX  = 44
	busyCorner   = 14 // радиус скругления корпуса (в классике не применяется)
)

// ShowBusy показывает диалог ожидания и возвращает управление им.
//
//	title    — крупная строка по центру («Обработка данных»);
//	subtitle — подпись под ней («Пожалуйста, подождите…»), может быть пустой;
//	hint     — нижняя строка-предупреждение («Не закрывайте это окно»),
//	           пустая — строки не будет;
//	onCancel — колбэк отмены. nil — операция непрерываемая: диалог без ✕,
//	           Escape его тоже не закроет.
//
// По умолчанию полоса в неопределённом режиме (голова ходит сама). Как
// только известен прогресс — SetProgress переключит её на значение.
func (mb *MessageBox) ShowBusy(title, subtitle, hint string, onCancel func()) *BusyDialog {
	dlg := NewDialog("", busyDlgW, busyDlgH)
	dlg.ShowCloseButton = onCancel != nil
	dlg.CornerRadius = busyCorner // мягче обычного диалога — так в дизайне

	// Полосы заголовка нет: ✕ висит в пустом верхнем поле, как в дизайне.
	// TitleHeight оставляем — по нему Dialog кладёт кнопку ✕, — но красим
	// её в цвет корпуса, чтобы она не читалась как заголовок.
	dlg.TitleBG = dlg.Background

	mkLabel := func(text string, size float64, muted, bold bool, y, h int) *Label {
		l := NewLabel(text, win10.LabelText)
		if muted {
			l = newMutedLabel(text)
		}
		l.FontSize = size
		l.Bold = bold
		l.TextAlign = TextAlignCenter
		l.SetBounds(image.Rect(dlgPad, y, busyDlgW-dlgPad, y+h))
		dlg.AddChild(l)
		return l
	}

	titleLbl := mkLabel(title, 15, false, true, busyTitleY, 22)
	sub := mkLabel(subtitle, 11, true, false, busySubtitle, 18)

	bar := NewProgressBar()
	bar.Style = ProgressStyleGlow
	bar.SetIndeterminate(true)
	bar.SetBounds(image.Rect(busyBarPadX, busyBarY, busyDlgW-busyBarPadX, busyBarY+busyBarH))
	dlg.AddChild(bar)

	hintLbl := mkLabel(hint, 10, true, false, busyHintY, 16)

	pd := &ProgressDialog{eng: mb.eng, dlg: dlg, status: sub, bar: bar, onCancel: onCancel}
	if onCancel != nil {
		dlg.CancelAction = pd.cancel
	}
	bd := &BusyDialog{ProgressDialog: pd, titleLbl: titleLbl, hint: hintLbl}

	mb.eng.ShowModal(dlg)
	return bd
}

// SetTitle меняет крупную строку по центру.
func (bd *BusyDialog) SetTitle(s string) { bd.titleLbl.SetText(s) }

// SetSubtitle меняет подпись под заголовком (та же строка, что у ShowBusy).
func (bd *BusyDialog) SetSubtitle(s string) { bd.status.SetText(s) }

// SetHint меняет нижнюю строку-предупреждение.
func (bd *BusyDialog) SetHint(s string) { bd.hint.SetText(s) }
