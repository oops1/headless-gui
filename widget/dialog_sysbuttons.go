// dialog_sysbuttons.go — штатные кнопки окна у диалога: свернуть и развернуть.
//
// В полосе заголовка диалога была одна кнопка — ✕. Пока диалог был вопросом с
// двумя ответами, этого хватало. Диалог, показанный в СОБСТВЕННОМ окне ОС и
// растянутый на пол-экрана (окно настроек), ведёт себя как окно, и человек
// ищет в его полосе привычные три кнопки: свернуть, развернуть, закрыть.
//
// Сворачивает и разворачивает НЕ виджет: он рисуется в холсте своего окна и о
// нём ничего не знает. Кнопки зовут хуки, которые ставит нативный хост
// (window/modal_host.go) — там есть и Minimize, и Maximize, и Restore.
package widget

import "image"

// sysBtnKind — какая из штатных кнопок.
type sysBtnKind int

const (
	sysBtnMinimize sysBtnKind = iota
	sysBtnMaximize
)

// dialogSysBtn — кнопка «свернуть»/«развернуть» в полосе заголовка диалога.
type dialogSysBtn struct {
	Base
	owner *Dialog
	kind  sysBtnKind
	hover bool
	// armed — кнопка взведена нажатием: срабатывание на отпускании и только
	// если курсор всё ещё над ней (та же семантика, что у ✕ и у кнопок
	// заголовка окна).
	armed  bool
	capMgr CaptureManager
}

func (sb *dialogSysBtn) Draw(ctx DrawContext) {
	b := sb.bounds
	if b.Empty() {
		return
	}
	fg := sb.owner.TitleColor
	if sb.hover {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, win10.BtnHoverBG)
	} else {
		// Ненавязчивый серый, как у ✕: кнопки заголовка не должны спорить с
		// содержимым за внимание.
		fg = win10.InputPlaceholder
	}

	cx, cy := b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2
	switch sb.kind {
	case sysBtnMinimize:
		ctx.FillRect(cx-5, cy, 10, 1, fg)
	case sysBtnMaximize:
		if sb.owner.maximized {
			// Развёрнутое окно: две рамки — «восстановить», как в Windows.
			ctx.DrawBorder(cx-6, cy-3, 9, 9, fg)
			ctx.DrawBorder(cx-3, cy-6, 9, 9, fg)
			return
		}
		ctx.DrawBorder(cx-5, cy-5, 10, 10, fg)
	}
}

func (sb *dialogSysBtn) OnMouseMove(x, y int) {
	h := image.Pt(x, y).In(sb.bounds)
	if h != sb.hover {
		sb.hover = h
		sb.Invalidate()
	}
}

// SetCaptureManager инжектится движком: захват нужен, чтобы отпускание пришло
// кнопке, даже если курсор ушёл с неё.
func (sb *dialogSysBtn) SetCaptureManager(cm CaptureManager) { sb.capMgr = cm }

func (sb *dialogSysBtn) WantsCapture(e MouseEvent) bool {
	return e.Button == MouseLeft && e.Pressed && image.Pt(e.X, e.Y).In(sb.bounds)
}

func (sb *dialogSysBtn) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	over := image.Pt(e.X, e.Y).In(sb.bounds)
	if e.Pressed {
		if !over {
			return false
		}
		sb.armed = true
		return true
	}
	if !sb.armed {
		return false
	}
	sb.armed = false
	if sb.capMgr != nil {
		sb.capMgr.ReleaseCapture()
	}
	if !over {
		return true
	}
	switch sb.kind {
	case sysBtnMinimize:
		if sb.owner.OnMinimize != nil {
			sb.owner.OnMinimize()
		}
	case sysBtnMaximize:
		if sb.owner.OnMaximizeRestore != nil {
			sb.owner.OnMaximizeRestore()
		}
	}
	return true
}

// ─── API диалога ────────────────────────────────────────────────────────────

// SetWindowButtons показывает в полосе заголовка все три штатные кнопки:
// свернуть, развернуть и закрыть (вместо одной ✕).
//
// Нужно диалогу, который ведёт себя как окно: показан в собственном окне ОС,
// растянут на пол-экрана и живёт долго. Диалогу-вопросу это ни к чему —
// поэтому по умолчанию выключено.
//
// Сворачивание и разворачивание выполняет нативный хост: он ставит OnMinimize
// и OnMaximizeRestore при показе диалога в своём окне. В headless (диалог
// нарисован внутри главного окна) сворачивать нечего — кнопки останутся без
// действия, и показывать их там незачем.
func (d *Dialog) SetWindowButtons(v bool) {
	if v == (d.minBtn != nil) {
		return
	}
	if !v {
		d.RemoveChild(d.minBtn)
		d.RemoveChild(d.maxBtn)
		d.minBtn, d.maxBtn = nil, nil
	} else {
		d.minBtn = &dialogSysBtn{owner: d, kind: sysBtnMinimize}
		d.maxBtn = &dialogSysBtn{owner: d, kind: sysBtnMaximize}
		d.AddChild(d.minBtn)
		d.AddChild(d.maxBtn)
	}
	// Кнопки занимают место в полосе: и ✕ переезжает левее, и начинка
	// приложения ужимается.
	d.SetBounds(d.bounds)
	d.Invalidate()
}

// HasWindowButtons сообщает, показаны ли кнопки «свернуть»/«развернуть».
func (d *Dialog) HasWindowButtons() bool { return d.minBtn != nil }

// SetMaximized сообщает диалогу, что его окно развёрнуто: кнопка меняет глиф
// на «восстановить». Состояние знает хост окна, а не виджет.
func (d *Dialog) SetMaximized(v bool) {
	if d.maximized == v {
		return
	}
	d.maximized = v
	if d.maxBtn != nil {
		d.maxBtn.Invalidate()
	}
}

// IsMaximized возвращает последнее сообщённое состояние окна диалога.
func (d *Dialog) IsMaximized() bool { return d.maximized }

// sysButtonsLeft — левый край блока кнопок в полосе заголовка.
//
// Одна точка правды: по ней раскладываются сами кнопки и от неё считается
// правая граница начинки приложения. Без полосы (SetChromeless) кнопок нет —
// возвращается правый край диалога.
func (d *Dialog) sysButtonsLeft() int {
	b := d.bounds
	if d.titleH() == 0 {
		return b.Max.X
	}
	n := 0
	if d.ShowCloseButton {
		n++
	}
	if d.minBtn != nil {
		n += 2
	}
	if n == 0 {
		return b.Max.X - dlgPad
	}
	return b.Max.X - 6 - n*dlgCloseSize - (n-1)*sysBtnGap
}

// sysBtnGap — зазор между штатными кнопками.
const sysBtnGap = 2

// layoutSysButtons расставляет ✕ и кнопки «свернуть»/«развернуть» справа
// налево: ✕ всегда крайняя.
func (d *Dialog) layoutSysButtons(r image.Rectangle) {
	top := r.Min.Y + (d.TitleHeight-dlgCloseSize)/2
	right := r.Max.X - 6
	place := func(w Widget) {
		w.SetBounds(image.Rect(right-dlgCloseSize, top, right, top+dlgCloseSize))
		right -= dlgCloseSize + sysBtnGap
	}
	if d.closeBtn != nil {
		place(d.closeBtn)
	}
	if d.maxBtn != nil {
		place(d.maxBtn)
		place(d.minBtn)
	}
}
