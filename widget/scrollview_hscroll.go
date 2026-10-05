package widget

import (
	"image"
	"math"
)

// scrollview_hscroll.go — горизонтальная прокрутка ScrollView.
//
// Содержимое шире области раньше просто обрезалось: добраться до его правой
// части было нечем. Полоса и её геометрия уже были в сравнении/слиянии
// (hscrollbar.go), поэтому здесь используются те же hbarThumb, hbarScrollAt,
// hbarScrollForThumbX и drawHBar — вторая копия арифметики разошлась бы с
// первой при первой же правке.
//
// Всё включается полем ContentWidth. Пока оно нулевое, ни один метод этого
// файла ничего не меняет: полоса не нужна, смещение всегда 0, область
// содержимого не уменьшается.

// bars говорит, какие полосы показаны. Вызывать под sv.mu или в потоке, который
// заведомо не конкурирует с изменением bounds/ContentWidth/ContentHeight.
//
// Полосы зависят друг от друга: вертикальная отнимает ширину, горизонтальная —
// высоту. Если решать каждую отдельно, содержимое, которое «чуть-чуть не
// влезает», получало бы одну полосу, а появившаяся полоса тут же отнимала бы
// место и делала нужной вторую — на следующем кадре картинка прыгала бы.
// Поэтому считаем по порядку: сначала вертикальную по полной высоте, затем
// горизонтальную с учётом ширины, съеденной вертикальной, и, если горизонтальная
// появилась, перепроверяем вертикальную по сокращённой высоте. Обратной
// перепроверки не нужно: вертикальная уже учтена в ширине, а горизонтальная
// только добавляется, поэтому ответ монотонен и дальше не меняется.
func (sv *ScrollView) bars() (vert, horiz bool) {
	w, h := sv.bounds.Dx(), sv.bounds.Dy()
	vert = sv.ContentHeight > h
	if sv.ContentWidth <= 0 {
		return vert, false // горизонтальная прокрутка не включена
	}
	availW := w
	if vert {
		availW -= sv.reserve()
	}
	horiz = sv.ContentWidth > availW
	if horiz && !vert {
		vert = sv.ContentHeight > h-sv.reserve()
	}
	return vert, horiz
}

// needsHScrollbar возвращает true, если ContentWidth задан и не влезает в ширину.
func (sv *ScrollView) needsHScrollbar() bool {
	_, horiz := sv.bars()
	return horiz
}

// hscrollAvailable — needsHScrollbar для вызова без замка.
func (sv *ScrollView) hscrollAvailable() bool {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.needsHScrollbar()
}

// contentHeight возвращает высоту контентной области (без горизонтальной
// полосы) — зеркало contentWidth().
func (sv *ScrollView) contentHeight() int {
	h := sv.bounds.Dy()
	if sv.needsHScrollbar() {
		h -= sv.reserve()
	}
	return h
}

// maxScrollX возвращает максимальное значение scrollX.
func (sv *ScrollView) maxScrollX() int {
	if sv.ContentWidth <= 0 {
		return 0
	}
	viewW := sv.contentWidth()
	if sv.ContentWidth <= viewW {
		return 0
	}
	return sv.ContentWidth - viewW
}

// scrollXLocked — текущее смещение вбок, зажатое в допустимые пределы.
//
// Хранимое scrollX может устареть: ContentWidth уменьшили или обнулили, а
// смещение осталось прежним. С полосой это было бы просто неудобно, а без неё
// (ContentWidth=0) — катастрофа: содержимое осталось бы сдвинутым, а вернуть
// его на место нечем. Поэтому наружу и в отрисовку смещение отдаётся зажатым.
func (sv *ScrollView) scrollXLocked() int {
	x := sv.scrollX
	if m := sv.maxScrollX(); x > m {
		x = m
	}
	return x
}

// ScrollX возвращает текущее смещение вбок.
func (sv *ScrollView) ScrollX() int {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.scrollXLocked()
}

// SetScrollX задаёт смещение вбок с ограничением [0, ContentWidth - ширина].
// Пока ContentWidth не задан или bounds ещё не выставлены, предел нулевой и
// смещение остаётся 0 — как у SetScrollY.
func (sv *ScrollView) SetScrollX(x int) {
	sv.mu.Lock()
	changed := sv.setScrollXLocked(x)
	sv.mu.Unlock()
	if changed {
		sv.Invalidate()
		sv.thinPoke()
	}
}

// ScrollXBy прокручивает вбок на delta пикселей (положительное — вправо).
func (sv *ScrollView) ScrollXBy(delta int) {
	sv.mu.Lock()
	changed := sv.setScrollXLocked(sv.scrollXLocked() + delta)
	sv.mu.Unlock()
	if changed {
		sv.Invalidate()
		sv.thinPoke()
	}
}

// setScrollXLocked зажимает и применяет scrollX; возвращает true, если
// смещение фактически изменилось (для авто-инвалидации).
func (sv *ScrollView) setScrollXLocked(x int) bool {
	if maxX := sv.maxScrollX(); x > maxX {
		x = maxX
	}
	if x < 0 {
		x = 0
	}
	if sv.scrollX == x {
		return false
	}
	sv.scrollX = x
	return true
}

// ─── Геометрия полос ─────────────────────────────────────────────────────────

// vbarRect — трек вертикальной полосы: у правого края и (если есть
// горизонтальная полоса) только до неё. Угол между полосами остаётся пустым:
// иначе обе полосы накрывали бы его, и рисовался он дважды.
func (sv *ScrollView) vbarRect() image.Rectangle {
	b := sv.bounds
	bottom := b.Max.Y
	if sv.needsHScrollbar() {
		bottom -= sv.sbW()
	}
	return image.Rect(b.Max.X-sv.sbW(), b.Min.Y, b.Max.X, bottom)
}

// hbarStrip — вся полоса у нижнего края: до вертикальной полосы, а не до
// правого края виджета. Пустой прямоугольник — полосы нет.
func (sv *ScrollView) hbarStrip() image.Rectangle {
	if !sv.needsHScrollbar() {
		return image.Rectangle{}
	}
	b := sv.bounds
	right := b.Max.X
	if sv.needsScrollbar() {
		right -= sv.sbW()
	}
	return image.Rect(b.Min.X, b.Max.Y-sv.sbW(), right, b.Max.Y)
}

// hbarTrack — трек, по которому ходит ползунок: полоса с небольшими полями,
// чтобы скруглённые концы drawHBar не липли к краям виджета и вертикальной
// полосе. Хит-тест при этом идёт по всей hbarStrip: тонкая (6 px) линия трека
// слишком мелка, чтобы по ней попадать мышью.
func (sv *ScrollView) hbarTrack() image.Rectangle {
	strip := sv.hbarStrip()
	if strip.Empty() {
		return image.Rectangle{}
	}
	if sv.isThin() {
		return strip // тонкая полоса рисуется на всю свою колонку, без полей
	}
	return strip.Inset(sv.scrollbarWidth / 5)
}

// hbarThumbLocked — ползунок горизонтальной полосы (пустой, если полосы нет).
func (sv *ScrollView) hbarThumbLocked() image.Rectangle {
	track := sv.hbarTrack()
	if track.Empty() {
		return image.Rectangle{}
	}
	return hbarThumb(track, float64(sv.scrollXLocked()), float64(sv.maxScrollX()),
		float64(sv.contentWidth()), float64(sv.ContentWidth))
}

// hbarThumbHitLocked — курсор над ползунком. По вертикали годится вся полоса,
// а не только 6-пиксельный ползунок: иначе в него надо было бы целиться.
func (sv *ScrollView) hbarThumbHitLocked(x, y int) bool {
	if !image.Pt(x, y).In(sv.hbarStrip()) || !sv.interactive() {
		return false
	}
	th := sv.hbarThumbLocked()
	return !th.Empty() && x >= th.Min.X && x < th.Max.X
}

// ─── Мышь ────────────────────────────────────────────────────────────────────

// hbarPressLocked — нажатие ЛКМ на горизонтальную полосу (под sv.mu). Возвращает
// true, если нажатие пришлось на полосу и поглощено.
//
// Нажатие на ползунок начинает перетаскивание без скачка (запоминаем, за какое
// место его взяли), мимо — содержимое прыгает туда, куда ткнули, и перетаскивание
// тоже начинается: так привычнее, чем требовать второго нажатия. После прыжка
// ползунок стоит серединой под курсором, поэтому и «хват» — его середина:
// иначе при первом же сдвиге ползунок дёрнулся бы левым краем под курсор.
func (sv *ScrollView) hbarPressLocked(e MouseEvent) bool {
	if !image.Pt(e.X, e.Y).In(sv.hbarStrip()) || !sv.interactive() {
		return false
	}
	sv.hdragging = true
	sv.thinHold(true)
	if th := sv.hbarThumbLocked(); !th.Empty() && e.X >= th.Min.X && e.X < th.Max.X {
		sv.hdragGrab = e.X - th.Min.X
	} else {
		v := hbarScrollAt(sv.hbarTrack(), e.X, float64(sv.maxScrollX()),
			float64(sv.contentWidth()), float64(sv.ContentWidth))
		sv.setScrollXLocked(int(math.Round(v)))
		sv.hdragGrab = sv.hbarThumbLocked().Dx() / 2
	}
	sv.Invalidate() // ползунок подсвечивается при drag
	return true
}

// hbarDragToLocked ведёт ползунок за курсором (под sv.mu); true — смещение
// изменилось.
func (sv *ScrollView) hbarDragToLocked(x int) bool {
	v := hbarScrollForThumbX(sv.hbarTrack(), x-sv.hdragGrab, float64(sv.maxScrollX()),
		float64(sv.contentWidth()), float64(sv.ContentWidth))
	return sv.setScrollXLocked(int(math.Round(v)))
}

// ─── Колесо ──────────────────────────────────────────────────────────────────

// OnMouseWheelPixels — колесо без модификаторов; см. OnMouseWheelPixelsMod.
func (sv *ScrollView) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	return sv.OnMouseWheelPixelsMod(x, y, dx, dy, 0)
}

// OnMouseWheelPixelsMod — плавная прокрутка точной пиксельной дельтой с
// модификаторами, зажатыми в момент прокрутки. dy>0 — вниз, dx>0 — вправо.
// Движок предпочитает этот метод OnMouseWheelPixels (см. wheelPixelModHandler).
//
// Горизонтальная составляющая dx двигает содержимое вбок; вертикальное колесо
// с Shift — тоже: на мыши без горизонтального колеса это единственный способ
// добраться до правой части. Но только если горизонтальная полоса есть: в
// ScrollView без неё Shift по-прежнему ничего не значит и колесо листает
// вертикально, как раньше.
//
// Возвращает true, если дельта поглощена; false — если прокручивать нечего или
// упёрлись в край в сторону жеста (тогда она всплывёт к родителю).
func (sv *ScrollView) OnMouseWheelPixelsMod(x, y int, dx, dy float64, mod KeyMod) bool {
	if !sv.hscrollAvailable() {
		// Прежнее поведение, байт в байт: dx игнорируется.
		return sv.wheelPixelsY(dy)
	}
	hx, vy := dx, dy
	if mod&ModShift != 0 {
		hx, vy = dx+dy, 0
	}
	handled := false
	if hx != 0 && sv.wheelPixelsX(hx) {
		handled = true
	}
	// Вертикаль обрабатываем, если по ней есть дельта, либо если по горизонтали
	// дельты нет вовсе (нулевое событие прежнее поведение принимало как есть).
	if (vy != 0 || hx == 0) && sv.wheelPixelsY(vy) {
		handled = true
	}
	return handled
}

// wheelPixelsX — горизонтальная часть прокрутки. Применяется сразу, без
// инерции (см. ensureInertia), с субпиксельным накоплением: тачпад отдаёт
// дробные дельты, и без накопления они бы терялись при округлении.
func (sv *ScrollView) wheelPixelsX(dx float64) bool {
	if !sv.IsEnabled() {
		return false
	}
	sv.mu.Lock()
	cur, maxX := sv.scrollXLocked(), sv.maxScrollX()
	// Упёрлись в край в сторону жеста — отдаём событие родителю.
	if maxX <= 0 || (dx < 0 && cur <= 0) || (dx > 0 && cur >= maxX) {
		sv.scrollFracX = 0
		sv.mu.Unlock()
		return false
	}
	sv.scrollFracX += dx
	whole := math.Trunc(sv.scrollFracX)
	sv.scrollFracX -= whole
	changed := sv.setScrollXLocked(cur + int(whole))
	if x := sv.scrollXLocked(); (dx < 0 && x <= 0) || (dx > 0 && x >= maxX) {
		sv.scrollFracX = 0 // в крае остаток не копим: он бы «пружинил» в обратную сторону
	}
	sv.mu.Unlock()
	if changed {
		sv.Invalidate()
	}
	sv.thinPoke()
	return true
}
