package widget

import (
	"image"
	"image/color"
)

// ─── Modal-интерфейс ────────────────────────────────────────────────────────

// ModalWidget — виджет, который блокирует весь ввод под собой.
// Движок проверяет наличие модальных виджетов перед dispatch'ем событий.
type ModalWidget interface {
	Widget
	// IsModal возвращает true, пока модальный виджет активен.
	IsModal() bool
	// DimColor возвращает цвет затемнения фона (обычно полупрозрачный чёрный).
	DimColor() color.RGBA
}

// ─── Dialog ─────────────────────────────────────────────────────────────────

// Геометрия современного диалога (не-Classic3D темы).
const (
	dlgCorner    = 8  // радиус скругления корпуса
	dlgTitleH    = 34 // высота заголовка
	dlgPad       = 14 // горизонтальный отступ контента
	dlgShadowW   = 5  // ширина мягкой тени справа/снизу
	dlgCloseSize = 24 // зона кнопки ✕
)

// Dialog — модальный диалог в стиле активной темы.
//
// Отрисовывается поверх всех виджетов с затемнением фона.
// Весь ввод ограничен содержимым диалога — клики вне него игнорируются.
//
// Использование:
//
//	dlg := widget.NewDialog("Подтверждение", 400, 200)
//	dlg.AddChild(label)
//	dlg.AddChild(okBtn)
//	eng.ShowModal(dlg)
//	// ...
//	eng.CloseModal(dlg)
type Dialog struct {
	Base

	Title       string
	Background  color.RGBA
	BorderColor color.RGBA
	TitleColor  color.RGBA
	TitleBG     color.RGBA
	Dim         color.RGBA // затемнение фона
	Shadow      color.RGBA // тень под диалогом (A=0 — без тени)
	TitleHeight int

	// ShowLocaleIndicator — показывать индикатор текущей локали (напр. «EN»)
	// в заголовке диалога. По умолчанию выключен (принятый дизайн — ✕).
	ShowLocaleIndicator bool

	// ShowCloseButton — рисовать кнопку ✕ в заголовке (по умолчанию true).
	// Клик по ✕ эквивалентен Escape: CancelAction + закрытие модалки.
	ShowCloseButton bool

	// DefaultAction вызывается по Enter (кнопка по умолчанию диалога).
	DefaultAction func()
	// CancelAction вызывается по Escape/✕ в дополнение к закрытию (может быть
	// nil; само закрытие модалки выполняет движок).
	CancelAction func()
	// CopyText, если задан, вызывается по Ctrl+C и его результат кладётся
	// в буфер обмена (MessageBox формирует Windows-подобный дамп).
	CopyText func() string

	modal      bool   // управляется движком: true пока диалог показан
	closer     func() // закрытие модалки движком (устанавливает ShowModal)
	closeBtn   *dialogCloseBtn
	localeSubs []int // id подписчиков на смену языка (снимаются при закрытии)
}

// OnLanguageChange регистрирует применение перевода при смене языка
// интерфейса, пока диалог открыт. apply вызывается сразу (для текущего
// языка) и далее при каждом SetLanguage; подписка снимается в SetModal(false).
func (d *Dialog) OnLanguageChange(apply func()) {
	apply()
	id := AddLanguageListener(func(string) {
		apply()
		d.Invalidate()
	})
	d.localeSubs = append(d.localeSubs, id)
}

// OnCancel вызывается движком при закрытии диалога по Escape (и кнопкой ✕
// через RequestClose) — сообщает результат отмены.
func (d *Dialog) OnCancel() {
	if d.CancelAction != nil {
		d.CancelAction()
	}
}

// SetCloser задаёт функцию закрытия модалки. Вызывается движком в ShowModal;
// функция должна выполнить OnCancel-семантику и CloseModal.
func (d *Dialog) SetCloser(close func()) { d.closer = close }

// RequestClose закрывает диалог путём отмены (кнопка ✕): как Escape.
func (d *Dialog) RequestClose() {
	if d.closer != nil {
		d.closer()
	}
}

// HandleInputBinding обрабатывает клавиши диалога до фокус-диспатча:
// Enter → DefaultAction, Ctrl+C → копирование содержимого (см. CopyText).
// Движок вызывает его у верхнего модального виджета (см. SendKeyEvent).
func (d *Dialog) HandleInputBinding(code KeyCode, mod KeyMod) bool {
	switch {
	case code == KeyEnter && mod == 0 && d.DefaultAction != nil:
		d.DefaultAction()
		return true
	case code == KeyC && mod&ModCtrl != 0 && d.CopyText != nil:
		ClipboardSetText(d.CopyText())
		return true
	}
	return false
}

// NewDialog создаёт модальный диалог заданного размера.
// Диалог центрируется на экране при показе через Engine.ShowModal.
// Цвета берутся из активной темы (глобальная палитра win10).
func NewDialog(title string, width, height int) *Dialog {
	d := &Dialog{
		Title:           title,
		Background:      win10.DialogBG,
		BorderColor:     win10.Border,
		TitleColor:      win10.TitleText,
		TitleBG:         win10.DialogTitleBG,
		Dim:             win10.DialogDim,
		Shadow:          win10.ShadowColor,
		TitleHeight:     dlgTitleH,
		ShowCloseButton: true,
		modal:           true,
		Base: Base{
			bounds: image.Rect(0, 0, width, height),
		},
	}
	d.closeBtn = &dialogCloseBtn{owner: d}
	d.closeBtn.SetBounds(image.Rect(
		width-dlgCloseSize-6, (dlgTitleH-dlgCloseSize)/2,
		width-6, (dlgTitleH-dlgCloseSize)/2+dlgCloseSize))
	d.AddChild(d.closeBtn)
	return d
}

// IsModal реализует ModalWidget.
func (d *Dialog) IsModal() bool { return d.modal }

// DimColor реализует ModalWidget.
func (d *Dialog) DimColor() color.RGBA { return d.Dim }

// SetModal управляет модальным состоянием (вызывается движком).
// При закрытии снимает подписки на смену языка.
func (d *Dialog) SetModal(v bool) {
	d.modal = v
	if !v {
		for _, id := range d.localeSubs {
			RemoveLanguageListener(id)
		}
		d.localeSubs = nil
	}
}

// ContentBounds возвращает прямоугольник для размещения дочерних виджетов
// (под заголовком, с отступами).
func (d *Dialog) ContentBounds() image.Rectangle {
	b := d.bounds
	return image.Rect(
		b.Min.X+dlgPad,
		b.Min.Y+d.TitleHeight+12,
		b.Max.X-dlgPad,
		b.Max.Y-12,
	)
}

// Draw рисует диалог (без затемнения — затемнение рисует движок).
func (d *Dialog) Draw(ctx DrawContext) {
	b := d.bounds
	st := currentStyle()
	d.closeBtn.SetVisible(d.ShowCloseButton)

	if st.Classic3D {
		// Классика Win2000: квадрат, градиентный заголовок, рамка.
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), d.Background)
		if d.TitleHeight > 0 {
			fillTitleBar(ctx, image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+d.TitleHeight), d.TitleBG)
			textY := b.Min.Y + (d.TitleHeight-13)/2
			drawTitleText(ctx, d.Title, b.Min.X+10, textY, d.TitleColor)
			if d.ShowLocaleIndicator {
				drawLocaleBadge(ctx, b.Max.X-8, b.Min.Y, d.TitleHeight, d.TitleColor)
			}
			ctx.DrawHLine(b.Min.X, b.Min.Y+d.TitleHeight, b.Dx(), d.BorderColor)
		}
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), d.BorderColor)
		d.drawChildren(ctx)
		return
	}

	// Современный вид (принятый дизайн): скругление, мягкая тень, ✕.
	cr := dlgCorner

	// Тень: полосы с честным альфа-смешиванием справа и снизу.
	if d.Shadow.A > 0 {
		sc := d.Shadow
		half := color.RGBA{R: sc.R, G: sc.G, B: sc.B, A: sc.A / 2}
		ctx.FillRectAlpha(b.Min.X+cr, b.Max.Y, b.Dx()-cr+3, 3, sc)
		ctx.FillRectAlpha(b.Min.X+cr+2, b.Max.Y+3, b.Dx()-cr, 2, half)
		ctx.FillRectAlpha(b.Max.X, b.Min.Y+cr, 3, b.Dy()-cr, sc)
		ctx.FillRectAlpha(b.Max.X+3, b.Min.Y+cr+2, 2, b.Dy()-cr-2, half)
	}

	// Корпус и заголовок.
	ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, d.Background)
	if d.TitleHeight > 0 {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), d.TitleHeight, cr, d.TitleBG)
		ctx.FillRect(b.Min.X, b.Min.Y+d.TitleHeight-cr, b.Dx(), cr, d.TitleBG)
		textY := b.Min.Y + (d.TitleHeight-14)/2
		ctx.DrawTextFont(d.Title, b.Min.X+dlgPad, textY, 11, BuiltinFontBold, d.TitleColor)
		if d.ShowLocaleIndicator {
			drawLocaleBadge(ctx, b.Max.X-dlgCloseSize-12, b.Min.Y, d.TitleHeight, d.TitleColor)
		}
	}
	ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, d.BorderColor)

	d.drawChildren(ctx)
}

// ApplyTheme обновляет цвета Dialog.
func (d *Dialog) ApplyTheme(t *Theme) {
	d.Background = t.DialogBG
	d.TitleBG = t.DialogTitleBG
	d.TitleColor = t.TitleText
	d.BorderColor = t.Border
	d.Dim = t.DialogDim
	d.Shadow = t.ShadowColor
}

// ─── Кнопка ✕ в заголовке ───────────────────────────────────────────────────

// dialogCloseBtn — кнопка закрытия в заголовке диалога. Клик — отмена
// (эквивалент Escape). В классике — выпуклая bevel-кнопка.
type dialogCloseBtn struct {
	Base
	owner *Dialog
	hover bool
}

func (cb *dialogCloseBtn) Draw(ctx DrawContext) {
	b := cb.bounds
	if b.Empty() {
		return
	}
	st := currentStyle()
	fg := win10.TitleText
	if st.Classic3D {
		// Классика: маленькая выпуклая кнопка «лица» с чёрным ✕.
		ctx.FillRect(b.Min.X, b.Min.Y+2, b.Dx()-2, b.Dy()-4, win10.BtnBG)
		drawBevelRaised(ctx, b.Min.X, b.Min.Y+2, b.Dx()-2, b.Dy()-4, st)
		fg = win10.BtnText
		tw := ctx.MeasureText("x", 10)
		ctx.DrawTextFont("x", b.Min.X+(b.Dx()-2-tw)/2, b.Min.Y+(b.Dy()-13)/2, 10, BuiltinFontBold, fg)
		return
	}
	if cb.hover {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, win10.BtnHoverBG)
	} else {
		fg = win10.InputPlaceholder // ненавязчивый серый ✕ (как в мокапе)
	}
	tw := ctx.MeasureText("✕", 11)
	ctx.DrawTextSize("✕", b.Min.X+(b.Dx()-tw)/2, b.Min.Y+(b.Dy()-14)/2, 11, fg)
}

func (cb *dialogCloseBtn) OnMouseMove(x, y int) {
	h := image.Pt(x, y).In(cb.bounds)
	if h != cb.hover {
		cb.hover = h
		cb.Invalidate()
	}
}

func (cb *dialogCloseBtn) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed || !image.Pt(e.X, e.Y).In(cb.bounds) {
		return false
	}
	cb.owner.RequestClose()
	return true
}

// ─── Хелперы для быстрого создания диалогов ─────────────────────────────────

// NewConfirmDialog создаёт диалог подтверждения с кнопками «OK» и «Отмена».
// Кнопки позиционируются автоматически. onResult(true) — OK, onResult(false) — Отмена.
func NewConfirmDialog(title, message string, onResult func(ok bool)) *Dialog {
	const (
		dlgW = 400
		dlgH = 160
	)
	dlg := NewDialog(title, dlgW, dlgH)

	// Все координаты дочерних виджетов — относительно (0,0) диалога.
	// ShowModal сдвинет их при центрировании.
	lbl := NewLabel(message, win10.LabelText)
	lbl.SetBounds(image.Rect(16, dlg.TitleHeight+12, dlgW-16, dlg.TitleHeight+52))

	okBtn := NewWin10AccentButton("  OK  ")
	okBtn.SetBounds(image.Rect(dlgW-200, dlgH-48, dlgW-110, dlgH-12))

	cancelBtn := NewButton("  Отмена  ")
	cancelBtn.SetBounds(image.Rect(dlgW-100, dlgH-48, dlgW-10, dlgH-12))

	okBtn.OnClick = func() {
		if onResult != nil {
			onResult(true)
		}
	}
	cancelBtn.OnClick = func() {
		if onResult != nil {
			onResult(false)
		}
	}

	dlg.AddChild(lbl)
	dlg.AddChild(okBtn)
	dlg.AddChild(cancelBtn)

	return dlg
}
