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

// Dialog — модальный диалог в стиле Windows 10 Dark.
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
	TitleHeight int

	modal bool // управляется движком: true пока диалог показан
}

// NewDialog создаёт модальный диалог заданного размера.
// Диалог центрируется на экране при показе через Engine.ShowModal.
func NewDialog(title string, width, height int) *Dialog {
	return &Dialog{
		Title:       title,
		Background:  color.RGBA{R: 45, G: 45, B: 48, A: 255},
		BorderColor: color.RGBA{R: 100, G: 100, B: 110, A: 255},
		TitleColor:  color.RGBA{R: 255, G: 255, B: 255, A: 255},
		TitleBG:     color.RGBA{R: 35, G: 35, B: 38, A: 255},
		Dim:         color.RGBA{R: 0, G: 0, B: 0, A: 120},
		TitleHeight: 32,
		modal:       true,
		Base: Base{
			bounds: image.Rect(0, 0, width, height),
		},
	}
}

// IsModal реализует ModalWidget.
func (d *Dialog) IsModal() bool { return d.modal }

// DimColor реализует ModalWidget.
func (d *Dialog) DimColor() color.RGBA { return d.Dim }

// SetModal управляет модальным состоянием (вызывается движком).
func (d *Dialog) SetModal(v bool) { d.modal = v }

// ContentBounds возвращает прямоугольник для размещения дочерних виджетов
// (под заголовком, с отступами).
func (d *Dialog) ContentBounds() image.Rectangle {
	b := d.bounds
	return image.Rect(
		b.Min.X+8,
		b.Min.Y+d.TitleHeight+4,
		b.Max.X-8,
		b.Max.Y-8,
	)
}

// Draw рисует диалог (без затемнения — затемнение рисует движок).
func (d *Dialog) Draw(ctx DrawContext) {
	b := d.bounds

	// Фон диалога
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), d.Background)

	// Заголовок
	if d.TitleHeight > 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), d.TitleHeight, d.TitleBG)
		textY := b.Min.Y + (d.TitleHeight-13)/2
		ctx.DrawText(d.Title, b.Min.X+10, textY, d.TitleColor)
		// Разделитель
		ctx.DrawHLine(b.Min.X, b.Min.Y+d.TitleHeight, b.Dx(), d.BorderColor)
	}

	// Рамка
	ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), d.BorderColor)

	d.drawChildren(ctx)
}

// ApplyTheme обновляет цвета Dialog.
func (d *Dialog) ApplyTheme(t *Theme) {
	d.BorderColor = t.Border
	d.TitleColor = t.TitleText
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
	lbl := NewLabel(message, dlg.TitleColor)
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
