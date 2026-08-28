package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// StartButton — кнопка «Пуск» в начале панели задач.
//
// Своей иконки у движка нет: значок рисуется фигурами — решётка 2×2 из
// квадратов, цветом текста стиля. Тема, которая захочет заменить его на
// растровую иконку через tm.GetIcon, вольна переопределить Draw в своей
// сборке; движок обязан работать и без единой картинки на диске.
//
// Поведение нажатия — как у системных кнопок окна (см. widget/window.go,
// поле armedBtn): press «взводит» кнопку, а колбэк срабатывает на release,
// только если курсор всё ещё над кнопкой. Так пользователь может отменить
// клик, уведя курсор в сторону перед отпусканием.
type StartButton struct {
	widget.Base

	tm *theme.Manager

	// Icon — своя картинка вместо значка темы. Задаётся оболочкой, когда
	// нужен конкретный логотип, а не тот, что предлагает тема; побеждает
	// и иконку темы, и встроенный значок.
	Icon image.Image

	hovered bool
	armed   bool

	// OnClick вызывается при успешном клике (press+release над кнопкой).
	// Оболочка вешает сюда открытие меню «Пуск».
	OnClick func()
}

// ComponentStartButton — имя компонента для стилей темы.
const ComponentStartButton = "startbutton"

// Ключи токенов, которыми тема управляет кнопкой «Пуск».
const (
	// KeyStartButtonIconSize — сторона квадрата, в который вписан значок
	// (решётка 2×2).
	KeyStartButtonIconSize theme.Key = "startbutton.icon.size"
	// KeyStartButtonIconGap — зазор между четырьмя квадратами значка.
	KeyStartButtonIconGap theme.Key = "startbutton.icon.gap"
	// KeyStartButtonLabelGap — зазор между значком и подписью.
	KeyStartButtonLabelGap theme.Key = "startbutton.label.gap"
	// KeyStartButtonLabelWidth — ширина, которую PreferredSize резервирует
	// под подпись. PreferredSize не получает DrawContext и не может
	// измерить текст по-настоящему — тема заявляет ожидаемую ширину сама.
	// Фактическая отрисовка (Draw) меряет подпись по-настоящему и прячет
	// её, если места не хватило, — так что неточная оценка здесь не ломает
	// раскладку, только делает панель чуть менее оптимальной.
	KeyStartButtonLabelWidth theme.Key = "startbutton.label.width"
	// KeyStartButtonIcon — имя иконки кнопки «Пуск» в наборе иконок темы.
	// Тема вправе подменить значок целиком: логотип заказчика, эмблема
	// дистрибутива, что угодно — не трогая ни строчки кода компонента.
	KeyStartButtonIcon theme.Key = "startbutton.icon"
	// KeyStartButtonLabel — признак темы: показывать ли подпись
	// рядом со значком, когда место позволяет. Большинство профилей Windows
	// его не заявляют — тогда кнопка всегда показывает только значок.
	KeyStartButtonLabel theme.Key = "startbutton.label"
)

// startButtonLabel — текст подписи кнопки. Не размер и не цвет — обычная
// строка интерфейса, наравне с любым другим текстом виджета.
const startButtonLabel = "Пуск"

// NewStartButton создаёт кнопку «Пуск», оформляемую темами из tm.
func NewStartButton(tm *theme.Manager) *StartButton {
	return &StartButton{tm: tm}
}

// PreferredSize возвращает желаемый размер кнопки: значок, отступы стиля и,
// если тема просит и оценочная ширина подписи не превышает avail, — место
// под подпись. Высоту решает панель (0 — не ограничиваем).
func (s *StartButton) PreferredSize(avail image.Point) image.Point {
	st := s.style(theme.StateNormal)
	width := int(s.metric(KeyStartButtonIconSize)) + 2*int(st.PadX)
	if s.wantLabel() {
		width += int(s.metric(KeyStartButtonLabelGap)) + int(s.metric(KeyStartButtonLabelWidth))
	}
	if width < 0 {
		width = 0
	}
	if avail.X > 0 && width > avail.X {
		width = avail.X
	}
	return image.Pt(width, 0)
}

// OnMouseMove реализует widget.MouseMoveHandler — обновляет наведение.
func (s *StartButton) OnMouseMove(x, y int) {
	hovered := image.Pt(x, y).In(s.Bounds())
	if hovered != s.hovered {
		s.hovered = hovered
		s.Invalidate()
	}
}

// OnMouseButton реализует widget.MouseClickHandler.
//
// Нажатие над кнопкой «взводит» её; отпускание вызывает OnClick только
// если курсор к этому моменту всё ещё над кнопкой — отпускание в стороне
// отменяет клик, не вызывая колбэк (поведение системных кнопок Windows).
func (s *StartButton) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft {
		return false
	}
	over := image.Pt(e.X, e.Y).In(s.Bounds())
	if e.Pressed {
		if !over {
			return false
		}
		s.armed = true
		s.Invalidate()
		return true
	}
	wasArmed := s.armed
	s.armed = false
	s.Invalidate()
	if wasArmed && over {
		if s.OnClick != nil {
			s.OnClick()
		}
	}
	return wasArmed
}

// Draw рисует подложку кнопки по стилю темы, значок фигурами и, если место
// позволяет и тема просит, — подпись.
func (s *StartButton) Draw(ctx widget.DrawContext) {
	b := s.Bounds()
	if b.Empty() {
		return
	}
	st := StateOf(s.hovered, s.armed, false, false, false)
	style := s.style(st)
	PaintStyle(ctx, b, style)

	iconSize := int(s.metric(KeyStartButtonIconSize))
	iconGap := int(s.metric(KeyStartButtonIconGap))
	padX := int(style.PadX)

	wantLabel := s.wantLabel()
	labelGap := int(s.metric(KeyStartButtonLabelGap))
	labelW := 0
	if wantLabel {
		labelW = MeasureText(ctx, startButtonLabel, style)
	}
	showLabel := wantLabel && labelW > 0 &&
		iconSize+2*padX+labelGap+labelW <= b.Dx()

	iconX := b.Min.X + padX
	if !showLabel {
		iconX = b.Min.X + (b.Dx()-iconSize)/2
	}
	iconY := b.Min.Y + (b.Dy()-iconSize)/2

	s.drawIcon(ctx, iconX, iconY, iconSize, iconGap, style)

	if showLabel {
		textLeft := iconX + iconSize + labelGap
		labelRect := image.Rect(textLeft-int(style.PadX), b.Min.Y, b.Max.X, b.Max.Y)
		DrawTextLeft(ctx, labelRect, startButtonLabel, style)
	}

	s.DrawChildren(ctx)
}

// drawIcon рисует значок кнопки: свою картинку, если её задала оболочка,
// иначе иконку темы, иначе встроенную решётку 2×2.
//
// Три уровня, а не один, потому что заказчики у них разные: оболочке нужен
// свой логотип, теме — свой значок в наборе иконок, а движку — что-то, что
// нарисуется даже когда нет ни того, ни другого.
func (s *StartButton) drawIcon(ctx widget.DrawContext, x, y, size, gap int, style *theme.Style) {
	if size <= 0 {
		return
	}
	if img := s.iconImage(size); img != nil {
		ctx.DrawImageScaled(img, x, y, size, size)
		return
	}
	drawStartGlyph(ctx, x, y, size, gap, style)
}

// iconImage — картинка значка или nil, если ни оболочка, ни тема её не дали.
func (s *StartButton) iconImage(size int) image.Image {
	if s.Icon != nil {
		return s.Icon
	}
	if s.tm == nil {
		return nil
	}
	return s.tm.GetIcon(string(KeyStartButtonIcon), size)
}

// drawStartGlyph рисует значок «Пуск» — решётку 2×2 из квадратов — цветом
// текста стиля. Сторона квадрата и зазор целиком выводятся из iconSize и
// iconGap: ни одного самостоятельного размера здесь нет.
func drawStartGlyph(ctx widget.DrawContext, x, y, iconSize, gap int, style *theme.Style) {
	sq := (iconSize - gap) / 2
	if sq <= 0 {
		return
	}
	col := style.Text
	ctx.FillRect(x, y, sq, sq, col)
	ctx.FillRect(x+sq+gap, y, sq, sq, col)
	ctx.FillRect(x, y+sq+gap, sq, sq, col)
	ctx.FillRect(x+sq+gap, y+sq+gap, sq, sq, col)
}

// metric читает метрику темы (0, если темы нет).
// wantLabel — просит ли тема подпись у кнопки.
//
// Читается ПРИЗНАКОМ, а не метрикой: SetFlag кладёт значение в таблицу
// признаков, и чтение метрикой всегда возвращало ноль — подпись не
// появлялась ни в одной теме, включая те, где она обязана быть.
// По умолчанию подпись есть: так выглядела кнопка «Пуск» до Windows 8.
func (s *StartButton) wantLabel() bool {
	if s.tm == nil {
		return false
	}
	return s.tm.GetFlag(KeyStartButtonLabel, true)
}

func (s *StartButton) metric(k theme.Key) float64 {
	if s.tm == nil {
		return 0
	}
	return s.tm.GetMetric(k)
}

// style возвращает стиль кнопки из активной темы для состояния st.
func (s *StartButton) style(st theme.State) *theme.Style {
	if s.tm == nil {
		return &theme.Style{}
	}
	return s.tm.GetStyle(ComponentStartButton, "", st)
}
