// widgetsbutton.go — кнопка виджетов у левого края панели Windows 11: значок и,
// если потребитель дал, температура с подписью («21°» и «Облачно»).
//
// Данные от потребителя: компонент ничего не знает о погоде. Потребитель задаёт
// содержимое одним вызовом SetContent (значок, две строки текста, подсказка) и
// обновляет его, когда оно меняется; ширина кнопки следует за текстом, а панель
// перекладывается сама. Без текста кнопка — один значок (виджеты по умолчанию).
//
// Кнопку кладут в слот SlotWidgets: он стоит у левого края при любом
// выравнивании группы «пуск + приложения».
package desktop

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentWidgets — имя компонента кнопки виджетов в стилях темы. Части:
// "temperature" (первая строка: цвет и шрифт) и "caption" (вторая строка).
const ComponentWidgets = "widgets"

const (
	// KeyWidgetsIconSize — сторона значка кнопки.
	KeyWidgetsIconSize theme.Key = "widgets.icon.size"
	// KeyWidgetsIcon — иконка кнопки в наборе иконок темы (без картинки от
	// потребителя); без неё рисуется встроенный глиф.
	KeyWidgetsIcon theme.Key = "widgets.icon"
	// KeyWidgetsTextGap — зазор между значком и текстом.
	KeyWidgetsTextGap theme.Key = "widgets.text.gap"
	// KeyWidgetsWidthMax — предел ширины кнопки с текстом; 0 — без предела
	// (слишком длинный текст усекается многоточием по границам кнопки).
	KeyWidgetsWidthMax theme.Key = "widgets.width.max"
)

// WidgetsContent — что показывает кнопка виджетов. Нулевое значение — один
// значок по умолчанию.
type WidgetsContent struct {
	// Icon — значок (погода); IconAt — источник по размеру в физических
	// пикселях (как AppInfo.IconAt) и побеждает Icon. Нет обоих — иконка темы
	// widgets.icon, а без неё встроенный глиф.
	Icon   image.Image
	IconAt func(size int) image.Image
	// Temperature — первая строка («21°»), Caption — вторая («Облачно»).
	// Пустые обе — кнопка из одного значка.
	Temperature string
	Caption     string
	// ToolTip — подсказка; пусто — «Виджеты» на языке интерфейса.
	ToolTip string
}

// WidgetsButton — кнопка виджетов.
//
//	wb := desktop.NewWidgetsButton(tm)
//	wb.SetContent(desktop.WidgetsContent{IconAt: sunny, Temperature: "21°", Caption: "Ясно"})
//	wb.OnClick = func() { board.Toggle(wb.Bounds()) }
//	bar.AddItem(desktop.SlotWidgets, wb)
type WidgetsButton struct {
	iconButton

	mu       sync.Mutex
	content  WidgetsContent
	relayout func()

	glyph glyphMemo
}

var (
	_ Item             = (*WidgetsButton)(nil)
	_ widget.Focusable = (*WidgetsButton)(nil)
	_ FocusNavigable   = (*WidgetsButton)(nil)
	_ FocusRinger      = (*WidgetsButton)(nil)
)

// NewWidgetsButton создаёт кнопку виджетов, оформляемую темой tm.
func NewWidgetsButton(tm *theme.Manager) *WidgetsButton {
	b := &WidgetsButton{}
	b.init(tm, ComponentWidgets)
	return b
}

// SetContent задаёт содержимое. Меняется ширина — панель перекладывается сама
// (Taskbar.AddItem передаёт кнопке функцию перекладки); тот же текст и значок
// ничего не перерисовывают.
func (b *WidgetsButton) SetContent(c WidgetsContent) {
	b.mu.Lock()
	old := b.content
	same := old.Temperature == c.Temperature && old.Caption == c.Caption && old.ToolTip == c.ToolTip &&
		sameIcon(old.Icon, c.Icon) && (old.IconAt == nil) == (c.IconAt == nil)
	b.content = c
	relayout := b.relayout
	widthChanged := old.Temperature != c.Temperature || old.Caption != c.Caption
	b.mu.Unlock()
	if same {
		return
	}
	if widthChanged && relayout != nil {
		relayout()
	}
	b.Invalidate()
}

// Content возвращает текущее содержимое.
func (b *WidgetsButton) Content() WidgetsContent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.content
}

// SetRelayout принимает от панели функцию перекладки (Taskbar.AddItem зовёт её
// для каждого элемента, который её просит).
func (b *WidgetsButton) SetRelayout(fn func()) {
	b.mu.Lock()
	b.relayout = fn
	b.mu.Unlock()
}

// GetToolTip — подсказка потребителя, а без неё «Виджеты».
func (b *WidgetsButton) GetToolTip() string {
	if tt := b.Content().ToolTip; tt != "" {
		return tt
	}
	return tr(StrWidgets)
}

// textStyles — стили первой и второй строки.
func (b *WidgetsButton) textStyles() (temp, caption *theme.Style) {
	return styleOf(b.tm, ComponentWidgets, "temperature", theme.StateNormal),
		styleOf(b.tm, ComponentWidgets, "caption", theme.StateNormal)
}

// PreferredSize — значок с отступами, а с текстом ещё и его ширина (но не больше
// widgets.width.max). Измеряет текст вне отрисовки измерителем движка.
func (b *WidgetsButton) PreferredSize(avail image.Point) image.Point {
	c := b.Content()
	st := styleOf(b.tm, ComponentWidgets, "", theme.StateNormal)
	pad := int(st.PadX)
	w := b.metric(KeyWidgetsIconSize) + 2*pad
	if c.Temperature != "" || c.Caption != "" {
		ts, cs := b.textStyles()
		tw := 0
		if c.Temperature != "" {
			tw = widget.MeasureUIText(c.Temperature, fontSizeOf(ts))
		}
		if c.Caption != "" {
			if cw := widget.MeasureUIText(c.Caption, fontSizeOf(cs)); cw > tw {
				tw = cw
			}
		}
		w += b.metric(KeyWidgetsTextGap) + tw
	}
	if max := b.metric(KeyWidgetsWidthMax); max > 0 && w > max {
		w = max
	}
	if avail.X > 0 && w > avail.X {
		w = avail.X
	}
	if w < 0 {
		w = 0
	}
	return image.Pt(w, 0)
}

// OnKeyEvent: Enter и Space — как щелчок, стрелки — к соседу по области.
func (b *WidgetsButton) OnKeyEvent(e widget.KeyEvent) {
	b.HandleKey(b, e, click(b.OnClick), b.Invalidate)
}

// Draw рисует плашку, значок и две строки текста.
func (b *WidgetsButton) Draw(ctx widget.DrawContext) {
	r, s := b.paintPlate(ctx)
	if s == nil {
		return
	}
	c := b.Content()
	size := b.metric(KeyWidgetsIconSize)
	keys := []theme.Key{KeyWidgetsIcon}
	if c.Temperature == "" && c.Caption == "" {
		drawIconIn(ctx, b.tm, r, size, s, c.Icon, c.IconAt, keys, glyphWidgets, &b.glyph)
		return
	}
	pad := int(s.PadX)
	iconBox := image.Rect(r.Min.X+pad, r.Min.Y, r.Min.X+pad+size, r.Max.Y)
	drawIconIn(ctx, b.tm, iconBox, size, s, c.Icon, c.IconAt, keys, glyphWidgets, &b.glyph)

	ts, cs := b.textStyles()
	x := iconBox.Max.X + b.metric(KeyWidgetsTextGap)
	room := r.Max.X - pad - x
	if room <= 0 {
		return
	}
	tsz, csz := fontSizeOf(ts), fontSizeOf(cs)
	th, ch := 0, 0
	if c.Temperature != "" {
		th = lineHeight(tsz)
	}
	if c.Caption != "" {
		ch = lineHeight(csz)
	}
	y := r.Min.Y + (r.Dy()-th-ch)/2
	if c.Temperature != "" {
		drawText(ctx, Elide(ctx, c.Temperature, ts, room), x, y, tsz, ts)
		y += th
	}
	if c.Caption != "" {
		// Вторая строка тише первой: тот же цвет вполсилы, а не отдельный токен.
		muted := *cs
		muted.Text = fadeColor(cs.Text, 0.72)
		drawText(ctx, Elide(ctx, c.Caption, cs, room), x, y, csz, &muted)
	}
}
