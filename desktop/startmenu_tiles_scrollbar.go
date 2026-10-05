// startmenu_tiles_scrollbar.go — управление тонкой полосой прокрутки списка и
// плиток меню «Пуск» мышью: перетаскивание бегунка и щелчок по дорожке.
//
// Рисует полосу startmenu_tiles_draw.go (ползунок тонкий, гаснет через паузу);
// здесь — геометрия дорожки, захват мыши и перевод положения бегунка в
// прокрутку. Дорожка — колонка у правого края области шириной
// scrollbar.thin.hover.width (+2 пикселя): та же ширина, до которой ползунок
// расширяется под курсором, так что нажимается ровно то, что видно. Она уже
// отступа строки от края (startmenu.row.pad / 2), поэтому стрелка раскрытия
// папки остаётся нажимаемой.
package desktop

import (
	"image"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"
)

// Ключи наведения на дорожки полос; префикс не пересекается с другими
// областями (s:, r:, t:, g:).
const (
	prefBar     = "b:"
	keyBarList  = prefBar + "list"
	keyBarTiles = prefBar + "tiles"
)

// barDrag — состояние перетаскивания бегунка.
type barDrag struct {
	active bool
	area   startArea
	// grab — на сколько пикселей ниже верха бегунка схвачена мышь: бегунок не
	// прыгает под курсор в момент нажатия.
	grab int
}

// barGeom — дорожка и бегунок полосы одной области.
type barGeom struct {
	track, thumb                image.Rectangle // бегунок — на всю ширину дорожки
	content, viewH, top, scroll int
}

// barKey — ключ дорожки области a.
func barKey(a startArea) string {
	if a == areaTiles {
		return keyBarTiles
	}
	return keyBarList
}

// barArea — область, которой принадлежит дорожка с ключом key (areaNone — это
// не дорожка).
func barArea(key string) startArea {
	switch key {
	case keyBarList:
		return areaList
	case keyBarTiles:
		return areaTiles
	}
	return areaNone
}

// barOf считает дорожку и бегунок области a. false — прокручивать нечего.
func (m *StartMenu) barOf(g startGeo, a startArea) (b barGeom, ok bool) {
	v := m.v
	v.mu.Lock()
	ls, ts := v.listScroll, v.tileScroll
	v.mu.Unlock()
	var view image.Rectangle
	switch a {
	case areaList:
		vp := m.listViewport(g)
		_, contentH := m.listRows()
		view, b.content, b.viewH, b.top = vp, contentH, vp.Dy(), vp.Min.Y
		b.scroll = clampScroll(ls, b.content, b.viewH)
	case areaTiles:
		if g.cols == 0 {
			return b, false
		}
		view, b.content, b.viewH, b.top = g.tiles, m.layoutTiles(g).height, tilesViewHeight(g), g.tinner.Min.Y
		b.scroll = clampScroll(ts, b.content, b.viewH)
	default:
		return b, false
	}
	if b.content <= b.viewH || b.viewH <= 0 {
		return b, false
	}
	b.track = image.Rect(view.Max.X-m.barTrackWidth(), b.top, view.Max.X, b.top+b.viewH)
	th := m.thinBarRect(view, b.content, b.viewH, b.scroll, b.top)
	b.thumb = image.Rect(b.track.Min.X, th.Min.Y, b.track.Max.X, th.Max.Y)
	return b, true
}

// barTrackWidth — ширина дорожки: широкий ползунок и по пикселю с краёв.
func (m *StartMenu) barTrackWidth() int {
	w := m.metricInt(KeyScrollThinHoverWidth)
	if w <= 0 {
		w = 8
	}
	return w + 2
}

// barHit определяет, на какую дорожку пришлась точка pt (пусто — ни на какую).
func (m *StartMenu) barHit(g startGeo, a startArea, pt image.Point) string {
	if b, ok := m.barOf(g, a); ok && pt.In(b.track) {
		return barKey(a)
	}
	return ""
}

// barRectAbs — прямоугольник дорожки для перерисовки наведения.
func (m *StartMenu) barRectAbs(key string) image.Rectangle {
	inner := m.contentRect()
	if inner.Empty() {
		return image.Rectangle{}
	}
	if b, ok := m.barOf(m.startGeometry(inner), barArea(key)); ok {
		return b.track
	}
	return image.Rectangle{}
}

// barPress разбирает нажатие на дорожке. На бегунке — захват мыши и
// перетаскивание, на пустой дорожке — сдвиг на страницу в сторону щелчка (с
// перекрытием в одну строку или ряд плиток, чтобы не терять место чтения).
func (m *StartMenu) barPress(a startArea, pt image.Point) {
	inner := m.contentRect()
	if inner.Empty() {
		return
	}
	g := m.startGeometry(inner)
	b, ok := m.barOf(g, a)
	if !ok {
		return
	}
	v := m.v
	if pt.In(b.thumb) {
		v.mu.Lock()
		v.bar = barDrag{active: true, area: a, grab: pt.Y - b.thumb.Min.Y}
		v.mu.Unlock()
		m.syncBarPins()
		m.invalBar(a)
		return
	}
	dir := 1
	if pt.Y < b.thumb.Min.Y {
		dir = -1
	}
	step := m.metricInt(KeyStartMenuRowHeight)
	if a == areaTiles {
		step = m.metricInt(KeyTileUnit) + m.metricInt(KeyTileGap)
	}
	page := b.viewH - step
	if page < step {
		page = b.viewH
	}
	if a == areaTiles {
		m.scrollTiles(dir * page)
	} else {
		m.scrollList(dir * page)
	}
}

// barMove ведёт бегунок за мышью: положение бегунка внутри дорожки
// пропорционально прокрутке, поэтому его верх всегда там, где схвачен, а
// нижний край хода соответствует концу содержимого. Положение считается от
// мыши, а не накапливается шагами, — рывки мыши не копят ошибку.
func (m *StartMenu) barMove(pt image.Point) {
	v := m.v
	v.mu.Lock()
	bd := v.bar
	v.mu.Unlock()
	if !bd.active {
		return
	}
	inner := m.contentRect()
	if inner.Empty() {
		return
	}
	g := m.startGeometry(inner)
	b, ok := m.barOf(g, bd.area)
	if !ok {
		return
	}
	room := b.viewH - b.thumb.Dy()
	span := b.content - b.viewH
	if room <= 0 || span <= 0 {
		return
	}
	pos := pt.Y - bd.grab - b.top
	if pos < 0 {
		pos = 0
	}
	if pos > room {
		pos = room
	}
	next := (pos*span + room/2) / room
	v.mu.Lock()
	var changed bool
	if bd.area == areaTiles {
		changed = v.tileScroll != next
		v.tileScroll = next
	} else {
		changed = v.listScroll != next
		v.listScroll = next
	}
	v.mu.Unlock()
	if !changed {
		return
	}
	if bd.area == areaTiles {
		widget.InvalidateRect(g.tiles)
	} else {
		widget.InvalidateRect(g.list)
	}
}

// barRelease заканчивает перетаскивание бегунка. Возвращает true, если оно шло.
func (m *StartMenu) barRelease() bool {
	v := m.v
	v.mu.Lock()
	bd := v.bar
	v.bar = barDrag{}
	cm := v.capture
	v.mu.Unlock()
	if !bd.active {
		return false
	}
	if cm != nil {
		cm.ReleaseCapture()
	}
	m.syncBarPins()
	m.invalBar(bd.area)
	return true
}

// syncBarPins удерживает на виду полосы, которые тянут или над дорожкой которых
// курсор, и отпускает остальные (они гаснут через обычную паузу).
func (m *StartMenu) syncBarPins() {
	v := m.v
	v.mu.Lock()
	bd, hover := v.bar, v.hover
	v.mu.Unlock()
	v.listBar.pin((bd.active && bd.area == areaList) || hover == keyBarList, m.invalListBar)
	v.tileBar.pin((bd.active && bd.area == areaTiles) || hover == keyBarTiles, m.invalTileBar)
}

// isBarKey — ключ принадлежит дорожке полосы.
func isBarKey(key string) bool { return strings.HasPrefix(key, prefBar) }

// thumbHot — ползунок области a надо рисовать широким и ярким: над его
// дорожкой курсор или его тянут.
func thumbHot(sn startSnap, a startArea) bool {
	return sn.hover == barKey(a) || (sn.bar.active && sn.bar.area == a)
}
