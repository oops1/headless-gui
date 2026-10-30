// Package base содержит интерфейсы и реализации UI-виджетов.
//
// Координатная система: все Bounds задаются в абсолютных пикселях
// пространства холста. Движок не применяет дополнительных трансформаций.
package base

import (
	hgdraw "github.com/oops1/headless-gui/v4/core/draw"
	"image"
)

// DefaultFontSizePt — см. draw.DefaultFontSizePt (core/draw).
const DefaultFontSizePt = hgdraw.DefaultFontSizePt

// AAShapes — см. draw.AAShapes (core/draw); алиас: тот же тип.
type AAShapes = hgdraw.AAShapes

// Snapshotter — см. draw.Snapshotter (core/draw); алиас: тот же тип.
type Snapshotter = hgdraw.Snapshotter

// OffscreenRenderer — см. draw.OffscreenRenderer (core/draw); алиас: тот же
// тип.
type OffscreenRenderer = hgdraw.OffscreenRenderer

// OverlayDrawer реализуется виджетами, которым нужно рисовать поверх всего дерева.
// Например, раскрытый выпадающий список должен перекрывать соседние виджеты.
//
// Движок вызывает DrawOverlay после отрисовки всего дерева виджетов.
// HasOverlay возвращает true, если overlay нужно рисовать в данный момент.
type OverlayDrawer interface {
	HasOverlay() bool
	DrawOverlay(ctx DrawContext)
}

// OverlayBoundsProvider — оверлей-виджет, умеющий сообщить прямоугольник
// своего оверлея в абсолютных ЛОГИЧЕСКИХ координатах холста. Пустой Rect
// означает, что активного оверлея нет.
//
// Реализуется popup-подобными оверлеями (Dropdown, PopupMenu, MenuBar,
// контекстные меню TextBox/TextInput). Движок использует это, чтобы вынести
// оверлей в отдельное нативное окно-попап ОС (см. engine.SetPopupSink) —
// тогда оверлей может выходить за границы родительского окна, как системное
// меню. Оверлеи БЕЗ этого интерфейса рисуются по-старому в основной холст.
type OverlayBoundsProvider interface {
	OverlayBounds() image.Rectangle
}

// OverlayRectsProvider — оверлей, который умеет назвать, где именно он
// непрозрачен: по прямоугольнику на каждую свою часть (у меню — на каждый
// открытый уровень), без тени и без пикселей за скруглением углов.
//
// Нужен потребителю, который кладёт картинку оверлея поверх ЧУЖИХ окон
// непрозрачно (оболочка WinLine над окнами Wayland-клиентов): охватывающий
// OverlayBounds у меню с подменю захватывает пустое место между уровнями, а
// у скруглённого меню — углы, и там проступает то, что нарисовано под меню.
// Координаты — те же абсолютные логические, что у OverlayBounds; пустой
// срез — оверлей закрыт. Вызывать можно из любой горутины.
type OverlayRectsProvider interface {
	OverlayRects() []image.Rectangle
}

// BackdropDrawer — см. draw.BackdropDrawer (core/draw); алиас: тот же тип.
type BackdropDrawer = hgdraw.BackdropDrawer

// NoiseDrawer — см. draw.NoiseDrawer (core/draw); алиас: тот же тип.
type NoiseDrawer = hgdraw.NoiseDrawer

// RoundClipper — см. draw.RoundClipper (core/draw); алиас: тот же тип.
type RoundClipper = hgdraw.RoundClipper

// ShadowDrawer — см. draw.ShadowDrawer (core/draw); алиас: тот же тип.
type ShadowDrawer = hgdraw.ShadowDrawer

// RotatedTextDrawer — см. draw.RotatedTextDrawer (core/draw); алиас: тот же тип.
type RotatedTextDrawer = hgdraw.RotatedTextDrawer

// OpacityDrawer — см. draw.OpacityDrawer (core/draw); алиас: тот же тип.
type OpacityDrawer = hgdraw.OpacityDrawer

// DrawContext — см. draw.DrawContext (core/draw); алиас: тот же тип.
type DrawContext = hgdraw.DrawContext
