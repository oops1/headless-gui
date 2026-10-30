// props.go — применение XAML attached-свойств к виджетам.
//
// Содержит: applyGridAttachedProps, applyMargin, applyAlignment,
// ApplyIsEnabled, applyDockAttachedProp, loadImageFile.
package xaml

import (
	"strings"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// ─── Grid attached properties ───────────────────────────────────────────────

// applyGridAttachedProps читает Grid.Row, Grid.Column и т.д. из XAML-атрибутов
// и устанавливает их в Base виджета.
func applyGridAttachedProps(w base.Widget, el Element) {
	type gridSetter interface {
		GetGridRow() int // наличие этого метода означает, что Base встроен
	}
	// Все наши виджеты встраивают Base, поэтому можно писать напрямую.
	// Используем рефлексию через интерфейс не нужно — у нас есть конкретный тип.
	// Простой подход: пишем через указатель на Base.
	type baseAccessor interface {
		base.Widget
		GetGridRow() int
	}
	if _, ok := w.(baseAccessor); !ok {
		return
	}

	row := ParseInt(el.Attr("Grid.Row"))
	col := ParseInt(el.Attr("Grid.Column"))
	rowSpan := ParseInt(el.Attr("Grid.RowSpan"))
	colSpan := ParseInt(el.Attr("Grid.ColumnSpan"))

	// Нужно добраться до Base. Используем сеттер-интерфейс.
	type gridPropsSetter interface {
		SetGridProps(row, col, rowSpan, colSpan int)
	}
	if gs, ok := w.(gridPropsSetter); ok {
		gs.SetGridProps(row, col, rowSpan, colSpan)
	}
}

// applyMargin читает Margin из XAML-атрибутов и устанавливает в Base.
func applyMargin(w base.Widget, el Element) {
	ms := el.Attr("Margin")
	if ms == "" {
		return
	}
	m := ParseMargin(ms)
	type marginSetter interface {
		SetMargin(m base.Margin)
	}
	if setter, ok := w.(marginSetter); ok {
		setter.SetMargin(m)
	}
}

// applyAlignment читает HorizontalAlignment и VerticalAlignment из XAML-атрибутов.
func applyAlignment(w base.Widget, el Element) {
	type alignSetter interface {
		SetHAlign(a base.HorizontalAlignment)
		SetVAlign(a base.VerticalAlignment)
	}
	as, ok := w.(alignSetter)
	if !ok {
		return
	}
	if ha := el.Attr("HorizontalAlignment"); ha != "" {
		switch strings.ToLower(ha) {
		case "left":
			as.SetHAlign(base.HAlignLeft)
		case "center":
			as.SetHAlign(base.HAlignCenter)
		case "right":
			as.SetHAlign(base.HAlignRight)
		case "stretch":
			as.SetHAlign(base.HAlignStretch)
		}
	}
	if va := el.Attr("VerticalAlignment"); va != "" {
		switch strings.ToLower(va) {
		case "top":
			as.SetVAlign(base.VAlignTop)
		case "center":
			as.SetVAlign(base.VAlignCenter)
		case "bottom":
			as.SetVAlign(base.VAlignBottom)
		case "stretch":
			as.SetVAlign(base.VAlignStretch)
		}
	}
}

// ─── ToolTip ──────────────────────────────────────────────────────────────

// ApplyToolTip читает ToolTip из XAML-атрибутов и устанавливает в Base.
// Поддерживает простой синтаксис ToolTip="текст".
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ApplyToolTip(w base.Widget, el Element) {
	tip := el.Attr("ToolTip", "ToolTipService.ToolTip")
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

// ApplyVisibility читает Visibility из XAML-атрибутов (WPF):
// "Visible" (default), "Collapsed", "Hidden" → скрытие виджета.
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ApplyVisibility(w base.Widget, el Element) {
	v := el.Attr("Visibility")
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
// устанавливает соответствующее поле у Panel и Dialog (у Window — его строитель,
// widget/chrome).
func applyLocaleIndicator(w base.Widget, el Element) {
	v := el.Attr("ShowLocaleIndicator", "ShowLocale")
	if v == "" {
		return
	}
	on := strings.EqualFold(v, "true") || v == "1"
	switch t := w.(type) {
	case *base.Panel:
		t.ShowLocaleIndicator = on
	case interface{ SetShowLocaleIndicator(bool) }:
		// Dialog (widget/dialog).
		t.SetShowLocaleIndicator(on)
	}
}

// ─── TabIndex ───────────────────────────────────────────────────────────────

// applyTabIndex читает TabIndex из XAML и задаёт порядок Tab-навигации.
func applyTabIndex(w base.Widget, el Element) {
	v := el.Attr("TabIndex")
	if v == "" {
		return
	}
	type tabSetter interface{ SetTabIndex(int) }
	if ts, ok := w.(tabSetter); ok {
		ts.SetTabIndex(ParseInt(v))
	}
}

// ─── Общий набор attached-свойств ──────────────────────────────────────────

// ApplyCommonProps применяет полный набор общих XAML-свойств к виджету:
// Grid.*, DockPanel.Dock, Margin, Alignment, IsEnabled, ToolTip, Visibility,
// ShowLocaleIndicator, Cursor. Используется как контейнерами, так и листовыми
// виджетами, чтобы поведение было единообразным (см. BUG-1).
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ApplyCommonProps(w base.Widget, el Element) {
	applyGridAttachedProps(w, el)
	applyDockAttachedProp(w, el)
	applyMargin(w, el)
	applyAlignment(w, el)
	ApplyIsEnabled(w, el)
	ApplyToolTip(w, el)
	ApplyVisibility(w, el)
	applyLocaleIndicator(w, el)
	applyTabIndex(w, el)
	applyCursor(w, el)
}

// applyCursor читает XAML-атрибут Cursor= и задаёт принудительный курсор виджету.
func applyCursor(w base.Widget, el Element) {
	cs := strings.ToLower(strings.TrimSpace(el.Attr("Cursor")))
	if cs == "" {
		return
	}
	sc, ok := w.(interface{ SetCursor(base.Cursor) })
	if !ok {
		return
	}
	switch cs {
	case "ibeam":
		sc.SetCursor(base.CursorIBeam)
	case "hand":
		sc.SetCursor(base.CursorHand)
	case "sizewe":
		sc.SetCursor(base.CursorSizeWE)
	case "sizens":
		sc.SetCursor(base.CursorSizeNS)
	case "arrow":
		sc.SetCursor(base.CursorArrow)
	case "none":
		sc.SetCursor(base.CursorNone)
	case "grab":
		sc.SetCursor(base.CursorGrab)
	case "grabbing":
		sc.SetCursor(base.CursorGrabbing)
	case "sizeall":
		sc.SetCursor(base.CursorSizeAll)
	case "cross", "crosshair":
		sc.SetCursor(base.CursorCrosshair)
	case "wait":
		sc.SetCursor(base.CursorWait)
	}
}

// ─── IsEnabled ──────────────────────────────────────────────────────────────

// ApplyIsEnabled читает IsEnabled из XAML-атрибутов и устанавливает в Base.
// WPF по умолчанию IsEnabled=True, поэтому false нужно задавать явно.
//
// Служебное: для пакетов виджетов (их XAML-строителей), в фасад не входит.
//
//facadegen:skip
func ApplyIsEnabled(w base.Widget, el Element) {
	type enabledSetter interface {
		SetEnabled(v bool)
	}
	es, ok := w.(enabledSetter)
	if !ok {
		return
	}
	if v := el.Attr("IsEnabled"); strings.EqualFold(v, "False") {
		es.SetEnabled(false)
	}
}

// ─── DockPanel.Dock attached property ───────────────────────────────────────

// applyDockAttachedProp читает DockPanel.Dock из XAML-атрибутов и устанавливает в Base.
func applyDockAttachedProp(w base.Widget, el Element) {
	dock := el.Attr("DockPanel.Dock")
	if dock == "" {
		return
	}
	type dockSetter interface {
		SetDock(d base.DockSide)
	}
	if ds, ok := w.(dockSetter); ok {
		switch strings.ToLower(dock) {
		case "top":
			ds.SetDock(base.DockTop)
		case "bottom":
			ds.SetDock(base.DockBottom)
		case "left":
			ds.SetDock(base.DockLeft)
		case "right":
			ds.SetDock(base.DockRight)
		}
	}
}

// ─── Резолв путей к ресурсам из XAML (SEC-8) ────────────────────────────────

// ─── Ограничения на декодирование изображений (SEC-9) ───────────────────────
