// previewlist.go — режим списка окон в предпросмотре.
//
// Когда у приложения несколько окон, кнопка панели задач одна (taskbutton.group),
// а предпросмотр показывает все окна рядом: у каждого значок, заголовок,
// миниатюра и крестик закрытия. Щелчок по окну поднимает его, по крестику —
// закрывает. Окон больше, чем влезает в ряд (до семи, как в Windows, и не шире
// экрана), показываются столбцом из одних заголовков без миниатюр.
//
// Список открывают две вещи: наведение на кнопку (после задержки, как
// предпросмотр одного окна) и щелчок по ней (сразу).
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// previewMaxListThumbs — сколько миниатюр с заголовками встаёт в ряд, прежде
// чем список превращается в столбец строк.
const previewMaxListThumbs = 7

// hoverFilmAlpha — доля цвета текста, которой подсвечивается окно списка под
// курсором: подсветка берётся из цвета темы и годится для светлой панели и
// для тёмной.
const hoverFilmAlpha = 0.16

// ListedWindows возвращает окна, которые показывает панель-список (пусто — это
// предпросмотр одного окна или панель закрыта).
func (p *WindowPreview) ListedWindows() []WindowInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]WindowInfo(nil), p.wins...)
}

// listLen — сколько окон в списке (0 — режим одного окна).
func (p *WindowPreview) listLen() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.wins)
}

// listMaxCols — сколько окон помещается в ряд.
func (p *WindowPreview) listMaxCols() int {
	max := previewMaxListThumbs
	pad := p.metric(KeyPreviewPad, previewDefPad)
	if sw := p.Screen.Dx(); sw > 0 {
		if c := (sw - pad) / (p.thumbMax().X + pad); c < max {
			max = c
		}
	}
	if max < 1 {
		max = 1
	}
	return max
}

// listVertical — n окон не помещаются в ряд: показываем столбец заголовков.
func (p *WindowPreview) listVertical(n int) bool { return n > p.listMaxCols() }

// listRows — сколько строк столбца умещается на экране.
func (p *WindowPreview) listRows(n int) int {
	pad := p.metric(KeyPreviewPad, previewDefPad)
	row := p.metric(KeyPreviewHeader, previewDefHeader) + pad
	if sh := p.Screen.Dy(); sh > 0 {
		if m := (sh - 2*pad) / row; m >= 1 && m < n {
			return m
		}
	}
	return n
}

// listItems раскладывает n окон по прямоугольнику содержимого r: рядом или
// столбцом. Второе значение — столбец ли это.
func (p *WindowPreview) listItems(r image.Rectangle, n int) ([]image.Rectangle, bool) {
	max := p.thumbMax()
	pad := p.metric(KeyPreviewPad, previewDefPad)
	header := p.metric(KeyPreviewHeader, previewDefHeader)
	vertical := p.listVertical(n)
	if vertical {
		n = p.listRows(n)
	}
	items := make([]image.Rectangle, 0, n)
	for i := 0; i < n; i++ {
		if vertical {
			row := header + pad
			y := r.Min.Y + pad + i*row
			items = append(items, image.Rect(r.Min.X+pad, y, r.Max.X-pad, y+row))
			continue
		}
		x := r.Min.X + pad + i*(max.X+pad)
		items = append(items, image.Rect(x, r.Min.Y+pad, x+max.X, r.Min.Y+pad+header+max.Y))
	}
	return items, vertical
}

// closeRect — крестик закрытия в окне списка it.
func (p *WindowPreview) closeRect(it image.Rectangle, vertical bool) image.Rectangle {
	header := p.metric(KeyPreviewHeader, previewDefHeader)
	y := it.Min.Y
	if vertical {
		y = it.Min.Y + (it.Dy()-header)/2
	}
	return image.Rect(it.Max.X-header, y, it.Max.X, y+header)
}

// contentRect — прямоугольник содержимого панели: тот, что получает draw.
func (p *WindowPreview) contentRect() image.Rectangle {
	return p.rect().Inset(int(p.style(theme.StateNormal).PadX))
}

// itemAt находит окно списка под точкой и сообщает, на крестике ли она.
func (p *WindowPreview) itemAt(pt image.Point) (idx int, onClose bool) {
	n := p.listLen()
	if n < 2 {
		return -1, false
	}
	items, vertical := p.listItems(p.contentRect(), n)
	for i, it := range items {
		if pt.In(it) {
			return i, pt.In(p.closeRect(it, vertical))
		}
	}
	return -1, false
}

// showList показывает список окон wins, прижав панель к кнопке anchor.
func (p *WindowPreview) showList(wins []WindowInfo, anchor image.Rectangle) {
	if len(wins) < 2 {
		return
	}
	p.mu.Lock()
	p.wins = append([]WindowInfo(nil), wins...)
	p.listApp = wins[0].AppID
	p.listHover = -1
	p.win, p.hasWin = wins[0], true
	p.thumb, p.thumbs = nil, nil
	p.mu.Unlock()

	p.grabThumb()
	p.Align = AlignCenter
	p.Open(anchor)
	p.startRefresh()
}

// syncList сверяет открытый список с моделью окон: окно закрыли или открыли
// ещё одно. Возвращает true, если список опустел и панель закрылась.
func (p *WindowPreview) syncList() bool {
	p.mu.Lock()
	app, cur := p.listApp, p.wins
	p.mu.Unlock()
	if len(cur) < 2 || app == "" || p.wm == nil {
		return false
	}
	var next []WindowInfo
	for _, w := range p.wm.Windows() {
		if w.AppID == app {
			next = append(next, w)
		}
	}
	if len(next) < 2 {
		p.Close()
		return true
	}
	if sameWindows(cur, next) {
		return false
	}
	p.mu.Lock()
	p.wins = next
	if p.listHover >= len(next) {
		p.listHover = -1
	}
	p.mu.Unlock()
	p.Open(p.Anchor) // размер панели изменился: перекладываем и перерисовываем
	return false
}

// sameWindows — одинаковые ли списки окон по тому, что видно на экране.
func sameWindows(a, b []WindowInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Title != b[i].Title ||
			a[i].Active != b[i].Active || a[i].Minimized != b[i].Minimized {
			return false
		}
	}
	return true
}

// hoverItem подсвечивает окно списка под точкой.
func (p *WindowPreview) hoverItem(pt image.Point) {
	if p.listLen() < 2 {
		return
	}
	idx, _ := p.itemAt(pt)
	p.mu.Lock()
	changed := p.listHover != idx
	p.listHover = idx
	p.mu.Unlock()
	if changed {
		if r := p.rect(); !r.Empty() {
			widget.InvalidateRect(r)
		}
	}
}

// listPressed разбирает нажатие внутри списка: окно — поднять, крестик —
// закрыть окно.
func (p *WindowPreview) listPressed(pt image.Point) bool {
	idx, onClose := p.itemAt(pt)
	p.mu.Lock()
	wins := p.wins
	p.mu.Unlock()
	if idx < 0 || idx >= len(wins) || p.wm == nil {
		return true
	}
	w := wins[idx]
	if !onClose {
		p.wm.Activate(w.ID)
		p.Close()
		return true
	}

	p.wm.Close(w.ID)
	rest := make([]WindowInfo, 0, len(wins)-1)
	for _, o := range wins {
		if o.ID != w.ID {
			rest = append(rest, o)
		}
	}
	if len(rest) < 2 {
		p.Close()
		return true
	}
	p.mu.Lock()
	p.wins = rest
	p.listHover = -1
	p.mu.Unlock()
	p.Open(p.Anchor)
	return true
}

// OnKeyEvent: в списке стрелки выбирают окно, Enter и Space его поднимают,
// Delete закрывает; Esc закрывает панель.
func (p *WindowPreview) OnKeyEvent(e widget.KeyEvent) {
	n := p.listLen()
	if !p.IsOpen() || !e.Pressed || n < 2 {
		p.Flyout.OnKeyEvent(e)
		return
	}
	p.mu.Lock()
	cur := p.listHover
	p.mu.Unlock()
	move := func(d int) {
		next := cur + d
		if cur < 0 {
			next = 0
		}
		if next < 0 {
			next = n - 1
		}
		if next >= n {
			next = 0
		}
		p.mu.Lock()
		p.listHover = next
		p.mu.Unlock()
		if r := p.rect(); !r.Empty() {
			widget.InvalidateRect(r)
		}
	}
	switch e.Code {
	case widget.KeyLeft, widget.KeyUp:
		move(-1)
	case widget.KeyRight, widget.KeyDown:
		move(1)
	case widget.KeyEnter, widget.KeySpace, widget.KeyDelete:
		if cur < 0 {
			return
		}
		items, vertical := p.listItems(p.contentRect(), n)
		if cur >= len(items) {
			return
		}
		pt := items[cur].Min.Add(image.Pt(1, 1))
		if e.Code == widget.KeyDelete {
			pt = p.closeRect(items[cur], vertical).Min.Add(image.Pt(1, 1))
		}
		p.listPressed(pt)
	default:
		p.Flyout.OnKeyEvent(e)
	}
}

// hoverFilm — стиль подсветки окна списка под курсором: плёнка цвета текста
// строки без рамки.
func hoverFilm(s *theme.Style) *theme.Style {
	f := *s
	f.Fill = fadeColor(s.Text, hoverFilmAlpha)
	f.BorderWidth = 0
	f.Elevation = 0
	return &f
}

// drawList рисует окна списка: у каждого значок, заголовок и крестик (у окна
// под курсором), в ряду — ещё и миниатюра.
func (p *WindowPreview) drawList(ctx widget.DrawContext, r image.Rectangle, wins []WindowInfo, thumbs map[WindowID]image.Image, hover int) {
	header := p.metric(KeyPreviewHeader, previewDefHeader)
	items, vertical := p.listItems(r, len(wins))
	headStyle := styleOf(p.tm, ComponentPreview, previewPartHeader, theme.StateNormal)

	for i, it := range items {
		if i >= len(wins) {
			break
		}
		w := wins[i]
		if i == hover {
			PaintStyle(ctx, it, hoverFilm(headStyle))
		}

		head := image.Rect(it.Min.X, it.Min.Y, it.Max.X, it.Min.Y+header)
		if vertical {
			head = image.Rect(it.Min.X, it.Min.Y+(it.Dy()-header)/2, it.Max.X, it.Min.Y+(it.Dy()-header)/2+header)
		}
		text := head
		if w.Icon != nil && header > 0 {
			icon := image.Rect(head.Min.X, head.Min.Y, head.Min.X+header, head.Max.Y)
			ctx.DrawImageScaled(w.Icon, icon.Min.X, icon.Min.Y, icon.Dx(), icon.Dy())
			text.Min.X = icon.Max.X + header/4
		}
		text.Max.X -= header // место под крестик
		DrawTextLeftElided(ctx, text, w.Title, headStyle)
		if i == hover {
			DrawTextCentered(ctx, p.closeRect(it, vertical), "×", headStyle)
		}

		if !vertical {
			p.drawThumb(ctx, image.Rect(it.Min.X, it.Min.Y+header, it.Max.X, it.Max.Y), thumbs[w.ID])
		}
	}
}
