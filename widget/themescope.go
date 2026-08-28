// themescope.go — тема на поддерево вместо одной темы на всё приложение.
//
// Тема в этом движке была ровно одна: `ApplyGlobalTheme` пишет цвета в общие
// переменные, `Engine.SetTheme` обходит всё дерево и раздаёт их каждому
// виджету. Ни на окно, ни на часть интерфейса свою тему назначить было
// нельзя, а нужно это в двух случаях сразу: оболочке удалённого стола —
// показать окно гостя в его собственной теме рядом со своим интерфейсом, и
// демонстрации — показать четыре облика одновременно, а не по очереди.
//
// ThemeScope решает это, ничего не ломая: он назначает тему своему поддереву
// и защищает его от глобальной смены. Цвета попадают в виджеты обычным
// путём (`ApplyThemeTree`), а форма — фаски, скругления, признак классики —
// подменяется на время отрисовки поддерева, потому что читается она из общей
// переменной и по-другому до неё не добраться.
package widget

import (
	"image"
)

// ThemeScope — контейнер, чьё поддерево живёт под своей темой.
//
// Кладётся в дерево как обычная панель: всё, что внутри, оформляется темой
// области, всё, что снаружи, — глобальной.
type ThemeScope struct {
	Base

	// theme — тема области. nil означает «как у всех» — тогда область ведёт
	// себя как обычный контейнер.
	theme *Theme
}

// NewThemeScope создаёт область с темой t (nil — глобальная тема).
func NewThemeScope(t *Theme) *ThemeScope {
	s := &ThemeScope{}
	s.SetTheme(t)
	return s
}

// SetTheme назначает тему области и раздаёт её поддереву.
func (s *ThemeScope) SetTheme(t *Theme) {
	s.theme = t
	if t == nil {
		return
	}
	for _, child := range s.Children() {
		ApplyThemeTree(child, t)
	}
	s.Invalidate()
}

// Theme возвращает тему области (nil — глобальная).
func (s *ThemeScope) Theme() *Theme { return s.theme }

// HasOwnTheme реализует themeScoped: обход глобальной темы сюда не заходит.
func (s *ThemeScope) HasOwnTheme() bool { return s.theme != nil }

// AddChild добавляет виджет и сразу оформляет его темой области — иначе он
// остался бы в глобальной теме до ближайшей её смены.
func (s *ThemeScope) AddChild(w Widget) {
	s.Base.AddChild(w)
	if s.theme != nil {
		ApplyThemeTree(w, s.theme)
	}
}

// Draw рисует поддерево, подменив на это время общий стиль темы.
//
// Цвета виджеты уже носят в своих полях, а вот ФОРМА (фаски Windows 2000,
// скругления, признак классического вида) читается из общей переменной прямо
// в Draw — см. currentStyle(). Без подмены поддерево получило бы чужую форму:
// цвета Windows 2000 на скруглённых кнопках Windows 11.
//
// Подмена безопасна, потому что весь кадр рисуется в одной горутине: движок
// вызывает Draw дерева, оверлеев и модалок последовательно. Возврат прежнего
// стиля — через defer, иначе паника внутри поддерева оставила бы чужую форму
// всему остальному интерфейсу.
func (s *ThemeScope) Draw(ctx DrawContext) {
	if s.theme == nil {
		s.DrawChildren(ctx)
		return
	}
	defer pushThemeStyle(s.theme.Style)()
	s.DrawChildren(ctx)
}

// Bounds области — объединение границ детей, если свои не заданы: область
// сама ничего не рисует и обычно служит просто рамкой темы.
func (s *ThemeScope) Bounds() image.Rectangle {
	b := s.Base.Bounds()
	if !b.Empty() {
		return b
	}
	var u image.Rectangle
	for _, child := range s.Children() {
		u = u.Union(child.Bounds())
	}
	return u
}
