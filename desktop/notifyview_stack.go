// notifyview_stack.go — стопки уведомлений Windows 11: несколько уведомлений
// одного приложения сворачиваются в самое свежее с кнопкой «+N уведомл.», щелчок
// по ней разворачивает группу.
//
// Вид включает флаг профиля theme.KeyNotifyStack (у Windows 11 он поднят). Без
// флага остаётся прежняя группа: заголовок со счётчиком и все карточки, которые
// сворачиваются шевроном. Строка «+N уведомл.» — ключ с формами по числу (см.
// plural.go), как счётчик на колокольчике трея.
package desktop

import (
	"fmt"
	"image"

	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// Размеры кнопки стопки, которые не метрики темы, а устройство фигуры: высота,
// поля по бокам надписи и зазор над ней от текста карточки.
const (
	ncW11StackH   = 28
	ncW11StackPad = 12
	ncW11StackGap = 8
)

// StrNotifStackMore — основа строки «+N уведомл.» на кнопке свёрнутой стопки. Это
// ключ с формами по числу: «.one», «.few», «.many», «.other» (см. PluralForm);
// в строке один %d — сколько уведомлений спрятано за самым свежим.
const StrNotifStackMore = "desktop.notif.stackMore"

func init() {
	widget.RegisterStrings("RU", map[string]string{
		StrNotifStackMore + ".one":   "+%d уведомл.",
		StrNotifStackMore + ".few":   "+%d уведомл.",
		StrNotifStackMore + ".many":  "+%d уведомл.",
		StrNotifStackMore + ".other": "+%d уведомл.",
	})
	widget.RegisterStrings("EN", map[string]string{
		StrNotifStackMore + ".one":   "+%d notification",
		StrNotifStackMore + ".other": "+%d notifications",
	})
}

// stackLabel возвращает надпись кнопки стопки для n спрятанных уведомлений: форма
// по числу и языку интерфейса.
func stackLabel(n int) string {
	lang := uiLanguage()
	key := pluralKey(lang, StrNotifStackMore, n, func(k string) bool { return widgetTranslated(lang, k) })
	if key == "" {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprintf(tr(key), n)
}

// stackMode сообщает, что центр рисует группы стопками: презентер Windows 11 и
// поднятый флаг профиля.
func (v *richView) stackMode() bool {
	return v.mode == ncModeCenter && v.w11() && v.tm != nil && v.tm.GetFlag(theme.KeyNotifyStack, false)
}

// stackCard достраивает свёрнутую стопку: под текстом самой свежей карточки
// встаёт кнопка «+N уведомл.» (more — сколько уведомлений за ней).
func (v *richView) stackCard(c *richCard, m ncMetrics, f ncFonts, more int) {
	c.stackText = stackLabel(more)
	pw := f.body.width(c.stackText) + 2*ncW11StackPad
	p := m.cardPad
	top := c.rect.Max.Y - p + ncW11StackGap
	c.stackRect = image.Rect(c.rect.Min.X+p, top, c.rect.Min.X+p+pw, top+ncW11StackH)
	c.rect.Max.Y = c.stackRect.Max.Y + p
}

// stackSpan — высоты первой карточки стопки в двух видах: раскрытом (без кнопки
// «+N») и свёрнутом (с кнопкой) и высота, до которой она тянется на этом шаге.
type stackSpan struct {
	open, closed, cur int
}

// stackSpanCard готовит первую карточку стопки на шаге движения: выкладывается
// как свёрнутая (с кнопкой «+N уведомл.»), а её высота — между раскрытой (sk=1) и
// свёрнутой (sk=0); кнопка уходит вместе с нижним краем карточки, потому что
// карточка в движении обрезает рисунок по своей рамке. Возвращает высоты для
// видимой части группы.
func (v *richView) stackSpanCard(c *richCard, m ncMetrics, f ncFonts, more int, sk float64) stackSpan {
	open := c.rect.Dy()
	v.stackCard(c, m, f, more)
	closed := c.rect.Dy()
	cur := open + int(float64(closed-open)*(1-sk)+0.5)
	c.rect.Max.Y = c.rect.Min.Y + cur
	c.anim = true
	return stackSpan{open: open, closed: closed, cur: cur}
}

// visible возвращает высоту видимой части группы на шаге sk, если вид
// раскрытой стопки выше свёрнутого (total — высота уже выложенных карточек с
// зазором gap после каждой): от свёрнутого вида к раскрытому линейно.
func (s stackSpan) visible(total, gap int, sk float64) (int, bool) {
	hc := s.closed + gap
	he := total - s.cur + s.open
	if he <= hc {
		return 0, false
	}
	return hc + int(float64(he-hc)*sk+0.5), true
}

// setStackOpen разворачивает или сворачивает стопку группы app. Клавиатурный
// фокус переходит на заголовок группы: кнопка стопки после раскрытия исчезает.
func (v *richView) setStackOpen(app AppID, open bool) {
	v.mu.Lock()
	if v.stackOpen[app] == open {
		v.mu.Unlock()
		return
	}
	if v.stackOpen == nil {
		v.stackOpen = map[AppID]bool{}
	}
	if open {
		v.stackOpen[app] = true
	} else {
		delete(v.stackOpen, app)
	}
	if v.focus.kind == zoneStack && v.focus.app == app {
		v.focus = zoneKey{kind: zoneGroup, app: app}
	}
	v.mu.Unlock()
	v.invalidateAll()
}

// drawStackButton рисует кнопку «+N уведомл.» внутри карточки.
func (v *richView) drawStackButton(ctx widget.DrawContext, l *richLayout, u ncUI, c richCard) {
	k := zoneKey{kind: zoneStack, app: c.n.AppID}
	st := v.style(k, c.stackRect, ncPartStack, u.state(k, false))
	PaintStyle(ctx, c.stackRect, st)
	f := l.fonts.body
	f.draw(ctx, c.stackText, c.stackRect.Min.X+(c.stackRect.Dx()-f.width(c.stackText))/2,
		c.stackRect.Min.Y+(c.stackRect.Dy()-f.lineH())/2, st.Text)
}
