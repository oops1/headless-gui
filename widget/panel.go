package widget

import (
	"image"
	"image/color"
)

// DrawContextAlpha — расширение DrawContext для рисования с альфа-смешиванием.
// Реализуется engine.Canvas через метод FillRectAlpha.
type DrawContextAlpha interface {
	DrawContext
	FillRectAlpha(x, y, w, h int, col color.RGBA)
}

// Panel — контейнер с фоновым цветом, опциональной рамкой и скруглением углов.
type Panel struct {
	Base
	Background      color.RGBA
	BackgroundImage *image.RGBA // фоновое изображение (масштабируется под bounds)
	BorderColor     color.RGBA
	ShowBorder      bool
	CornerRadius    int  // радиус скругления углов в пикселях (0 = острые)
	// UseAlpha=true: рисовать фон через Over (смешивание), иначе Src (непрозрачно)
	UseAlpha bool

	// ── Заголовок (title bar) ───────────────────────────────────────────────
	Caption      string     // текст заголовка
	ShowHeader   bool       // показывать заголовок (по умолчанию true в конструкторах)
	MacStyle     bool       // true = стиль macOS (traffic lights + текст по центру),
	                        //        false = стиль Windows (текст слева)
	HeaderHeight int        // высота заголовка в пикселях (0 → по умолчанию 32)
	HeaderBG     color.RGBA // фон заголовка (если A=0, берётся из темы)
	CaptionColor color.RGBA // цвет текста заголовка (если A=0, берётся из темы)

	// OnClose вызывается при клике по кнопке «×» (закрыть) в заголовке.
	// Если nil — кнопка декоративная (без действия).
	OnClose func()

	// Drag — настройки перетаскивания панели мышью.
	Drag DragState
}

// NewPanel создаёт панель с явно заданным цветом фона.
func NewPanel(bg color.RGBA) *Panel {
	return &Panel{
		Background:  bg,
		BorderColor: win10.Border,
		ShowHeader:  true,
	}
}

// NewWin10Panel создаёт полупрозрачную панель в стиле Windows 10 Dark с рамкой.
func NewWin10Panel() *Panel {
	return &Panel{
		Background:  win10.PanelBG,
		BorderColor: win10.Border,
		ShowBorder:  true,
		UseAlpha:    true,
		ShowHeader:  true,
	}
}

func (p *Panel) Draw(ctx DrawContext) {
	b := p.bounds
	r := p.CornerRadius

	if r > 0 {
		if p.UseAlpha && p.Background.A < 255 {
			if p.Background.A > 0 {
				ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), r, p.Background)
			}
		} else {
			ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), r, p.Background)
		}
		if p.ShowBorder {
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), r, p.BorderColor)
		}
	} else {
		if p.UseAlpha && p.Background.A < 255 {
			if ac, ok := ctx.(DrawContextAlpha); ok {
				ac.FillRectAlpha(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.Background)
			} else {
				ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.Background)
			}
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.Background)
		}
		if p.ShowBorder {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), p.BorderColor)
		}
	}

	// ── Фоновое изображение ─────────────────────────────────────────────────
	if p.BackgroundImage != nil {
		ctx.DrawImageScaled(p.BackgroundImage, b.Min.X, b.Min.Y, b.Dx(), b.Dy())
	}

	// ── Заголовок ───────────────────────────────────────────────────────────
	if p.ShowHeader && p.Caption != "" {
		p.drawHeader(ctx)
	}

	p.drawChildren(ctx)
}

// headerH возвращает фактическую высоту заголовка.
func (p *Panel) headerH() int {
	if p.HeaderHeight > 0 {
		return p.HeaderHeight
	}
	return 32
}

// drawHeader рисует заголовочную полосу с Caption.
func (p *Panel) drawHeader(ctx DrawContext) {
	b := p.bounds
	hh := p.headerH()

	// Цвет фона заголовка
	hbg := p.HeaderBG
	if hbg.A == 0 {
		// По умолчанию — тёмный заголовок, как в Dialog (не accent blue из темы).
		hbg = color.RGBA{R: 35, G: 35, B: 38, A: 255}
		if p.UseAlpha {
			hbg = color.RGBA{R: 29, G: 29, B: 32, A: 240}
		}
	}
	// Цвет текста
	tc := p.CaptionColor
	if tc.A == 0 {
		tc = win10.TitleText
	}

	if p.MacStyle {
		p.drawMacHeader(ctx, b, hh, hbg, tc)
	} else {
		p.drawWinHeader(ctx, b, hh, hbg, tc)
	}
}

// drawWinHeader рисует заголовок в стиле Windows: текст слева, кнопки ─ □ × справа.
func (p *Panel) drawWinHeader(ctx DrawContext, b image.Rectangle, hh int, bg, tc color.RGBA) {
	x, y, w := b.Min.X, b.Min.Y, b.Dx()
	r := p.CornerRadius

	// Фон заголовка (с учётом скругления верхних углов)
	if r > 0 {
		ctx.FillRoundRect(x, y, w, hh+r, r, bg)
		// Перекрываем нижнюю часть скругления прямоугольником (заголовок прямоугольный снизу)
		ctx.FillRect(x, y+hh-r, w, r, bg)
		// FillRoundRect высотой hh+r вытекает на r пикселей ниже заголовка —
		// восстанавливаем фон тела панели под линией заголовка.
		ctx.FillRect(x, y+hh, w, r, p.Background)
	} else {
		ctx.FillRect(x, y, w, hh, bg)
	}

	// Разделительная линия
	ctx.DrawHLine(x, y+hh-1, w, p.BorderColor)

	// Текст: вертикально по центру заголовка, отступ 12px слева
	textY := y + (hh-13)/2
	ctx.DrawText(p.Caption, x+12, textY, tc)

	// Кнопки управления окном (декоративные): ─ □ ×
	btnW := 46
	btnH := hh - 1
	bx := x + w - btnW*3

	// ─ (свернуть)
	lineColor := color.RGBA{R: 180, G: 180, B: 180, A: 255}
	my := y + btnH/2
	ctx.DrawHLine(bx+16, my, 14, lineColor)

	// □ (развернуть)
	bx += btnW
	ry := y + btnH/2 - 5
	ctx.DrawBorder(bx+16, ry, 11, 11, lineColor)

	// × (закрыть)
	bx += btnW
	cx, cy := bx+btnW/2, y+btnH/2
	for i := -5; i <= 5; i++ {
		ctx.SetPixel(cx+i, cy+i, lineColor)
		ctx.SetPixel(cx+i, cy-i, lineColor)
		// Утолщение
		ctx.SetPixel(cx+i+1, cy+i, lineColor)
		ctx.SetPixel(cx+i+1, cy-i, lineColor)
	}
}

// drawMacHeader рисует заголовок в стиле macOS: traffic lights слева, текст по центру.
func (p *Panel) drawMacHeader(ctx DrawContext, b image.Rectangle, hh int, bg, tc color.RGBA) {
	x, y, w := b.Min.X, b.Min.Y, b.Dx()
	r := p.CornerRadius

	// Фон заголовка
	if r > 0 {
		ctx.FillRoundRect(x, y, w, hh+r, r, bg)
		ctx.FillRect(x, y+hh-r, w, r, bg)
		// Восстанавливаем фон тела под заголовком (FillRoundRect вытекает на r px)
		ctx.FillRect(x, y+hh, w, r, p.Background)
	} else {
		ctx.FillRect(x, y, w, hh, bg)
	}

	// Разделительная линия
	ctx.DrawHLine(x, y+hh-1, w, p.BorderColor)

	// Traffic lights: красный, жёлтый, зелёный — кружки ⌀12px
	const (
		circleR  = 6 // радиус
		startX   = 18
		spacing  = 22
	)
	cy := y + hh/2
	trafficColors := [3]color.RGBA{
		{R: 255, G: 95, B: 86, A: 255},  // close (red)
		{R: 255, G: 189, B: 46, A: 255}, // minimize (yellow)
		{R: 39, G: 201, B: 63, A: 255},  // maximize (green)
	}
	for i, c := range trafficColors {
		ccx := x + startX + i*spacing
		ctx.FillRoundRect(ccx-circleR, cy-circleR, circleR*2, circleR*2, circleR, c)
	}

	// Текст: по центру заголовка
	textW := ctx.MeasureText(p.Caption, 10)
	textX := x + (w-textW)/2
	textY := y + (hh-13)/2
	ctx.DrawText(p.Caption, textX, textY, tc)
}

// ContentBounds возвращает прямоугольник для размещения дочерних виджетов
// (под заголовком, если он показан).
func (p *Panel) ContentBounds() image.Rectangle {
	b := p.bounds
	if p.ShowHeader && p.Caption != "" {
		b.Min.Y += p.headerH()
	}
	return b
}

// ApplyTheme применяет тему к панели (Win10-стиль — обновляет цвета панели и рамки).
func (p *Panel) ApplyTheme(t *Theme) {
	if p.UseAlpha {
		p.Background = t.PanelBG
	}
	p.BorderColor = t.Border
	// Если пользователь не задал явно — будет использоваться win10.TitleBG / win10.TitleText
}

// ─── Drag support ───────────────────────────────────────────────────────────

// SetCaptureManager инжектит менеджер захвата мыши (вызывается движком).
func (p *Panel) SetCaptureManager(cm CaptureManager) {
	p.Drag.capMgr = cm
}

// WantsCapture возвращает true, если панель хочет захватить мышь (drag handle).
func (p *Panel) WantsCapture(e MouseEvent) bool {
	if !p.Drag.Enabled {
		return false
	}
	return p.Drag.inDragHandle(e.X, e.Y, p.bounds)
}

// isCloseButtonHit проверяет, попал ли клик (x, y) в область кнопки «×» заголовка.
func (p *Panel) isCloseButtonHit(x, y int) bool {
	if !p.ShowHeader || p.Caption == "" {
		return false
	}
	b := p.bounds
	hh := p.headerH()
	// Клик должен быть в области заголовка по вертикали
	if y < b.Min.Y || y >= b.Min.Y+hh {
		return false
	}
	if p.MacStyle {
		// Mac: кнопка закрытия — красный кружок (первый traffic light)
		// Центр: (b.Min.X + 18, b.Min.Y + hh/2). Зона попадания ±10px.
		cx := b.Min.X + 18
		cy := b.Min.Y + hh/2
		return x >= cx-10 && x <= cx+10 && y >= cy-10 && y <= cy+10
	}
	// Windows: кнопка «×» — крайняя правая, ширина 46px
	btnW := 46
	closeX := b.Min.X + b.Dx() - btnW
	return x >= closeX && x < b.Min.X+b.Dx()
}

// OnMouseButton обрабатывает нажатие/отпускание мыши (close button + drag).
func (p *Panel) OnMouseButton(e MouseEvent) bool {
	// Кнопка закрытия в заголовке — работает даже без Drag.Enabled
	if e.Button == MouseLeft && e.Pressed && p.OnClose != nil {
		if p.isCloseButtonHit(e.X, e.Y) {
			p.OnClose()
			return true
		}
	}

	if !p.Drag.Enabled {
		return false
	}
	if e.Button == MouseLeft && e.Pressed {
		// Закрываем все открытые dropdown/popup внутри панели перед drag'ом,
		// чтобы overlay не рендерился некорректно при перемещении.
		DismissAll(p)
		p.Drag.initDrag(e, p.bounds)
		return true
	}
	if e.Button == MouseLeft && !e.Pressed && p.Drag.dragging {
		p.Drag.dragging = false
		if p.Drag.capMgr != nil {
			p.Drag.capMgr.ReleaseCapture()
		}
		return true
	}
	return false
}

// OnMouseMove обрабатывает перемещение мыши (drag move).
func (p *Panel) OnMouseMove(x, y int) {
	if !p.Drag.dragging {
		return
	}
	dx := x - p.Drag.startX
	dy := y - p.Drag.startY
	if dx == 0 && dy == 0 {
		return
	}
	// Целевая позиция панели
	newX := p.Drag.panelX + dx
	newY := p.Drag.panelY + dy
	// Текущее смещение относительно текущей позиции
	shiftX := newX - p.bounds.Min.X
	shiftY := newY - p.bounds.Min.Y
	if shiftX == 0 && shiftY == 0 {
		return
	}
	ShiftWidget(p, shiftX, shiftY)
}

// AddChild добавляет дочерний виджет и инжектит CaptureManager, если доступен.
func (p *Panel) AddChild(w Widget) {
	p.Base.AddChild(w)
	if p.Drag.capMgr != nil {
		if ca, ok := w.(CaptureAware); ok {
			ca.SetCaptureManager(p.Drag.capMgr)
		}
	}
}
