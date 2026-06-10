// xaml_props.go — применение XAML attached-свойств к виджетам.
//
// Содержит: applyGridAttachedProps, applyMargin, applyAlignment,
// applyIsEnabled, applyDockAttachedProp, loadImageFile.
package widget

import (
	"image"
	"os"
	"strings"
)

// ─── Grid attached properties ───────────────────────────────────────────────

// applyGridAttachedProps читает Grid.Row, Grid.Column и т.д. из XAML-атрибутов
// и устанавливает их в Base виджета.
func applyGridAttachedProps(w Widget, el xElement) {
	type gridSetter interface {
		GetGridRow() int // наличие этого метода означает, что Base встроен
	}
	// Все наши виджеты встраивают Base, поэтому можно писать напрямую.
	// Используем рефлексию через интерфейс не нужно — у нас есть конкретный тип.
	// Простой подход: пишем через указатель на Base.
	type baseAccessor interface {
		Widget
		GetGridRow() int
	}
	if _, ok := w.(baseAccessor); !ok {
		return
	}

	row := xatoi(el.attr("Grid.Row"))
	col := xatoi(el.attr("Grid.Column"))
	rowSpan := xatoi(el.attr("Grid.RowSpan"))
	colSpan := xatoi(el.attr("Grid.ColumnSpan"))

	// Нужно добраться до Base. Используем сеттер-интерфейс.
	type gridPropsSetter interface {
		SetGridProps(row, col, rowSpan, colSpan int)
	}
	if gs, ok := w.(gridPropsSetter); ok {
		gs.SetGridProps(row, col, rowSpan, colSpan)
	}
}

// applyMargin читает Margin из XAML-атрибутов и устанавливает в Base.
func applyMargin(w Widget, el xElement) {
	ms := el.attr("Margin")
	if ms == "" {
		return
	}
	m := parseMargin(ms)
	type marginSetter interface {
		SetMargin(m Margin)
	}
	if setter, ok := w.(marginSetter); ok {
		setter.SetMargin(m)
	}
}

// applyAlignment читает HorizontalAlignment и VerticalAlignment из XAML-атрибутов.
func applyAlignment(w Widget, el xElement) {
	type alignSetter interface {
		SetHAlign(a HorizontalAlignment)
		SetVAlign(a VerticalAlignment)
	}
	as, ok := w.(alignSetter)
	if !ok {
		return
	}
	if ha := el.attr("HorizontalAlignment"); ha != "" {
		switch strings.ToLower(ha) {
		case "left":
			as.SetHAlign(HAlignLeft)
		case "center":
			as.SetHAlign(HAlignCenter)
		case "right":
			as.SetHAlign(HAlignRight)
		case "stretch":
			as.SetHAlign(HAlignStretch)
		}
	}
	if va := el.attr("VerticalAlignment"); va != "" {
		switch strings.ToLower(va) {
		case "top":
			as.SetVAlign(VAlignTop)
		case "center":
			as.SetVAlign(VAlignCenter)
		case "bottom":
			as.SetVAlign(VAlignBottom)
		case "stretch":
			as.SetVAlign(VAlignStretch)
		}
	}
}

// ─── ToolTip ──────────────────────────────────────────────────────────────

// applyToolTip читает ToolTip из XAML-атрибутов и устанавливает в Base.
// Поддерживает простой синтаксис ToolTip="текст".
func applyToolTip(w Widget, el xElement) {
	tip := el.attr("ToolTip", "ToolTipService.ToolTip")
	if tip == "" {
		return
	}
	type tipSetter interface {
		SetToolTip(s string)
	}
	if ts, ok := w.(tipSetter); ok {
		ts.SetToolTip(tip)
	}
}

// ─── Visibility ─────────────────────────────────────────────────────────────

// applyVisibility читает Visibility из XAML-атрибутов (WPF):
// "Visible" (default), "Collapsed", "Hidden" → скрытие виджета.
func applyVisibility(w Widget, el xElement) {
	v := el.attr("Visibility")
	if v == "" {
		return
	}
	type visSetter interface {
		SetVisible(b bool)
	}
	vs, ok := w.(visSetter)
	if !ok {
		return
	}
	switch strings.ToLower(v) {
	case "collapsed", "hidden":
		vs.SetVisible(false)
	case "visible":
		vs.SetVisible(true)
	}
}

// ─── ShowLocaleIndicator ────────────────────────────────────────────────────

// applyLocaleIndicator читает ShowLocaleIndicator из XAML-атрибутов и
// устанавливает соответствующее поле у Window/Dialog/Panel.
func applyLocaleIndicator(w Widget, el xElement) {
	v := el.attr("ShowLocaleIndicator", "ShowLocale")
	if v == "" {
		return
	}
	on := strings.EqualFold(v, "true") || v == "1"
	switch t := w.(type) {
	case *Window:
		t.ShowLocaleIndicator = on
	case *Dialog:
		t.ShowLocaleIndicator = on
	case *Panel:
		t.ShowLocaleIndicator = on
	}
}

// ─── TabIndex ───────────────────────────────────────────────────────────────

// applyTabIndex читает TabIndex из XAML и задаёт порядок Tab-навигации.
func applyTabIndex(w Widget, el xElement) {
	v := el.attr("TabIndex")
	if v == "" {
		return
	}
	type tabSetter interface{ SetTabIndex(int) }
	if ts, ok := w.(tabSetter); ok {
		ts.SetTabIndex(xatoi(v))
	}
}

// ─── Общий набор attached-свойств ──────────────────────────────────────────

// applyCommonProps применяет полный набор общих XAML-свойств к виджету:
// Grid.*, DockPanel.Dock, Margin, Alignment, IsEnabled, ToolTip, Visibility,
// ShowLocaleIndicator, Cursor. Используется как контейнерами, так и листовыми
// виджетами, чтобы поведение было единообразным (см. BUG-1).
func applyCommonProps(w Widget, el xElement) {
	applyGridAttachedProps(w, el)
	applyDockAttachedProp(w, el)
	applyMargin(w, el)
	applyAlignment(w, el)
	applyIsEnabled(w, el)
	applyToolTip(w, el)
	applyVisibility(w, el)
	applyLocaleIndicator(w, el)
	applyTabIndex(w, el)
	applyCursor(w, el)
}

// applyCursor читает XAML-атрибут Cursor= и задаёт принудительный курсор виджету.
func applyCursor(w Widget, el xElement) {
	cs := strings.ToLower(strings.TrimSpace(el.attr("Cursor")))
	if cs == "" {
		return
	}
	sc, ok := w.(interface{ SetCursor(Cursor) })
	if !ok {
		return
	}
	switch cs {
	case "ibeam":
		sc.SetCursor(CursorIBeam)
	case "hand":
		sc.SetCursor(CursorHand)
	case "sizewe":
		sc.SetCursor(CursorSizeWE)
	case "sizens":
		sc.SetCursor(CursorSizeNS)
	case "arrow":
		sc.SetCursor(CursorArrow)
	}
}

// ─── IsEnabled ──────────────────────────────────────────────────────────────

// applyIsEnabled читает IsEnabled из XAML-атрибутов и устанавливает в Base.
// WPF по умолчанию IsEnabled=True, поэтому false нужно задавать явно.
func applyIsEnabled(w Widget, el xElement) {
	type enabledSetter interface {
		SetEnabled(v bool)
	}
	es, ok := w.(enabledSetter)
	if !ok {
		return
	}
	if v := el.attr("IsEnabled"); strings.EqualFold(v, "False") {
		es.SetEnabled(false)
	}
}

// ─── DockPanel.Dock attached property ───────────────────────────────────────

// applyDockAttachedProp читает DockPanel.Dock из XAML-атрибутов и устанавливает в Base.
func applyDockAttachedProp(w Widget, el xElement) {
	dock := el.attr("DockPanel.Dock")
	if dock == "" {
		return
	}
	type dockSetter interface {
		SetDock(d DockSide)
	}
	if ds, ok := w.(dockSetter); ok {
		switch strings.ToLower(dock) {
		case "top":
			ds.SetDock(DockTop)
		case "bottom":
			ds.SetDock(DockBottom)
		case "left":
			ds.SetDock(DockLeft)
		case "right":
			ds.SetDock(DockRight)
		}
	}
}

// LoadImageFile загружает PNG или JPEG файл и возвращает *image.RGBA.
// Экспортирован для использования в приложениях (иконки TreeView, и т.д.).
func LoadImageFile(path string) (*image.RGBA, error) {
	return loadImageFile(path)
}

// loadImageFile загружает PNG или JPEG файл и возвращает *image.RGBA.
func loadImageFile(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// image.Decode использует зарегистрированные декодеры (png, jpeg).
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	// Конвертируем в RGBA
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba, nil
}
