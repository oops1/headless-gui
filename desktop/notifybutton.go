// notifybutton.go — кнопка центра уведомлений в трее со счётчиком.
package desktop

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentTrayNotifications — имя компонента кнопки в стилях темы. Часть
// "badge" — счётчик (Text — цвет цифр, Font.Size — кегль; без неё цифры
// берут контрастный к значку цвет и кегль по умолчанию, уменьшаемый до
// размера значка).
const ComponentTrayNotifications = "tray.notifications"

const (
	// maxBadgeCount — предел счётчика: больше показывается «99+».
	maxBadgeCount = 99
	// minBadgePt — наименьший кегль цифр, до которого счётчик ужимается, чтобы
	// влезть в значок.
	minBadgePt = 6.0
)

// NotificationButton — кнопка центра уведомлений: значок «сообщение» и число
// накопленных уведомлений. Клик отдаётся колбэком OnClick — оболочка вешает
// на него открытие центра уведомлений.
//
// Значок берётся из набора иконок темы (ключи tray.notifications.icon и
// tray.notifications.icon.new — для состояния «есть новые»); без них рисуется
// фигура: контур сообщения, при ненулевом счётчике — залитый, с числом внутри.
// Подсказка собирается из строк widget.Tr и следует за языком.
type NotificationButton struct {
	widget.Base

	tm *theme.Manager
	n  Notifications

	// OnClick — колбэк клика (отпускание над кнопкой).
	OnClick func()

	hovered int32
	pressed int32
	manual  atomic.Int64 // счётчик, заданный SetCount (когда нет источника)

	glyph glyphMemo
	unsub func()
}

// NewNotificationButton создаёт кнопку, оформляемую темой tm. Счётчик — число
// уведомлений источника n; n может быть nil — тогда число задаёт SetCount.
func NewNotificationButton(tm *theme.Manager, n Notifications) *NotificationButton {
	b := &NotificationButton{tm: tm, n: n}
	if n != nil {
		b.unsub = n.Subscribe(b.Invalidate)
	}
	return b
}

// SetCount задаёт счётчик вручную (используется, если источник не передан).
func (b *NotificationButton) SetCount(n int) {
	if n < 0 {
		n = 0
	}
	if b.manual.Swap(int64(n)) != int64(n) {
		b.Invalidate()
	}
}

// Count возвращает текущее число уведомлений.
func (b *NotificationButton) Count() int {
	if b.n != nil {
		return len(b.n.List())
	}
	return int(b.manual.Load())
}

// Close снимает подписку на источник уведомлений.
func (b *NotificationButton) Close() {
	if b.unsub != nil {
		b.unsub()
		b.unsub = nil
	}
}

// GetToolTip перекрывает промоутнутый из widget.Base: текст строится из
// текущего счётчика и языка при каждом показе.
func (b *NotificationButton) GetToolTip() string {
	if n := b.Count(); n > 0 {
		return trayTrf(StrNewNotifications, n)
	}
	return widget.Tr(StrNoNewNotifications)
}

// PreferredSize — квадрат стороной из темы плюс отступы стиля по бокам.
func (b *NotificationButton) PreferredSize(image.Point) image.Point {
	size := trayIconSize(b.tm)
	return image.Point{X: size + 2*int(trayStyle(b.tm, ComponentTrayNotifications, theme.StateNormal).PadX), Y: size}
}

// OnMouseMove обновляет наведение.
func (b *NotificationButton) OnMouseMove(x, y int) {
	trayHandleMove(&b.hovered, b.Bounds(), x, y, b.Invalidate)
}

// OnMouseButton реализует клик (отпускание над кнопкой).
func (b *NotificationButton) OnMouseButton(e widget.MouseEvent) bool {
	return trayHandleClick(&b.pressed, b.Bounds(), e, b.OnClick, b.Invalidate)
}

// Draw рисует подложку, значок и счётчик.
func (b *NotificationButton) Draw(ctx widget.DrawContext) {
	r := b.Bounds()
	if r.Empty() {
		return
	}
	st := trayState(&b.hovered, &b.pressed)
	s := trayStyle(b.tm, ComponentTrayNotifications, st)
	PaintStyle(ctx, r, s)

	inner := shrinkByPad(r, s)
	box := glyphSquare(inner)
	if box.Empty() {
		return
	}
	count := b.Count()

	keys := []theme.Key{KeyTrayNotificationsIcon}
	if count > 0 {
		keys = []theme.Key{KeyTrayNotificationsIcon + ".new", KeyTrayNotificationsIcon}
	}
	filled := count > 0
	if !drawThemeGlyph(ctx, b.tm, keys, inner, s, &b.glyph) {
		drawBubble(ctx, box, ink(s), filled)
	}
	if count > 0 {
		b.drawBadge(ctx, box, s, count)
	}
}

// drawBadge рисует число по центру квадрата значка.
func (b *NotificationButton) drawBadge(ctx widget.DrawContext, box image.Rectangle, s *theme.Style, count int) {
	// Часть "badge" без собственного объявления откатывается к стилю компонента
	// целиком — тогда её Text совпадает с цветом значка и не годится для цифр.
	bs := &theme.Style{}
	if b.tm != nil {
		bs = b.tm.GetStyle(ComponentTrayNotifications, "badge", theme.StateNormal)
	}
	text := strconv.Itoa(count)
	if count > maxBadgeCount {
		text = strconv.Itoa(maxBadgeCount) + "+"
	}

	col := bs.Text
	if col.A == 0 || col == ink(s) {
		col = contrastOn(ink(s))
	}
	// Кегль ужимается, пока число не влезет в значок (с запасом в пиксель по
	// краям). Начало — кегль части "badge", иначе общий.
	size := fontSizeOf(bs)
	for size > minBadgePt &&
		(ctx.MeasureText(text, size) > box.Dx()-2 || lineHeight(size) > box.Dy()) {
		size--
	}
	w := ctx.MeasureText(text, size)
	x := box.Min.X + (box.Dx()-w)/2
	y := box.Min.Y + (box.Dy()-lineHeight(size))/2
	ctx.DrawTextSize(text, x, y, size, col)
}

// drawBubble рисует запасной значок — «облачко сообщения»: прямоугольник с
// хвостиком снизу слева. filled — залить (есть новые уведомления).
func drawBubble(ctx widget.DrawContext, box image.Rectangle, col color.RGBA, filled bool) {
	if box.Empty() || col.A == 0 {
		return
	}
	// Хвостик занимает нижнюю четверть, тело — остальное.
	tail := box.Dy() / 4
	body := image.Rect(box.Min.X, box.Min.Y, box.Max.X, box.Max.Y-tail)
	if body.Empty() {
		return
	}
	if filled {
		ctx.FillRect(body.Min.X, body.Min.Y, body.Dx(), body.Dy(), col)
	} else {
		ctx.DrawBorder(body.Min.X, body.Min.Y, body.Dx(), body.Dy(), col)
	}
	// Хвостик — сужающийся вниз треугольник от левой четверти тела.
	x0 := body.Min.X + body.Dx()/4
	for i := 0; i < tail; i++ {
		ctx.DrawHLine(x0, body.Max.Y+i, tail-i, col)
	}
}

// contrastOn выбирает чёрный или белый цвет, читаемый на фоне bg.
func contrastOn(bg color.RGBA) color.RGBA {
	// Яркость по весам Rec. 601; ниже середины фон тёмный — нужен светлый текст.
	lum := (299*uint32(bg.R) + 587*uint32(bg.G) + 114*uint32(bg.B)) / 1000
	if lum < 128 {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	return color.RGBA{A: 255}
}

// trayTrf форматирует перевод ключа: в строке из каталога — verbs fmt.
func trayTrf(key string, args ...any) string {
	return fmt.Sprintf(widget.Tr(key), args...)
}
