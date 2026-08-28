package engine

import (
	"image"
	"math"
)

// Отсечение по скруглённому контуру.
//
// Прямоугольный SetClip не умеет главного, что нужно современным темам:
// содержимое панели со скруглёнными углами должно обрезаться той же кривой,
// которой нарисован её фон. Иначе на углу вылезает угол дочернего виджета —
// заметнее всего на списке внутри панели Windows 11 и на всём, что лежит в
// окне macOS.
//
// Устройство. Скруглённый клип — не замена прямоугольному, а дополнение к
// нему: прямоугольная часть по-прежнему отсекает грубо и дёшево, а кривая
// сужает горизонтальный интервал построчно. Отступ для строки считается той
// же формулой, что и в заливке скруглённого прямоугольника
// (fillRoundRectLegacy), поэтому граница отсечения совпадает с границей
// фона — без щели в один пиксель и без наползания.
//
// Отступы предвычисляются на установку клипа: радиус редко больше двух
// десятков пикселей, а вот проверок на строку приходится по одной на каждый
// примитив.

// roundClipState — состояние скруглённого отсечения (физические пиксели).
type roundClipState struct {
	active bool
	rect   image.Rectangle
	radius int
	// insets[i] — на сколько сужается строка, отстоящая на i от края
	// угловой зоны. Длина среза равна радиусу.
	insets []int
}

// SetRoundClip включает отсечение по скруглённому прямоугольнику.
// Координаты и радиус — ЛОГИЧЕСКИЕ, как у всей отрисовки.
//
// Прямоугольная часть контура применяется тоже: вызов равносилен SetClip(r)
// плюс скругление углов. Действует один скруглённый клип за раз — вложенные
// скруглённые области пришлось бы держать стеком, а их пока никто не просит.
func (c *Canvas) SetRoundClip(r image.Rectangle, radius int) {
	c.SetClip(r)
	if radius <= 0 {
		c.round.active = false
		return
	}

	pr := c.sRect(r)
	rad := c.st(radius)
	if rad > pr.Dx()/2 {
		rad = pr.Dx() / 2
	}
	if rad > pr.Dy()/2 {
		rad = pr.Dy() / 2
	}
	if rad <= 0 {
		c.round.active = false
		return
	}

	c.round.active = true
	c.round.rect = pr
	c.round.radius = rad
	if cap(c.round.insets) >= rad {
		c.round.insets = c.round.insets[:rad]
	} else {
		c.round.insets = make([]int, rad)
	}
	// i — номер строки от края угловой зоны внутрь. Та же формула, что в
	// fillRoundRectLegacy: отступ = r - sqrt(r² - dy²).
	rf := float64(rad)
	for i := 0; i < rad; i++ {
		dy := float64(rad - i - 1)
		c.round.insets[i] = rad - int(math.Round(math.Sqrt(rf*rf-dy*dy)))
	}
}

// ClearRoundClip снимает скруглённое отсечение, оставляя прямоугольное.
func (c *Canvas) ClearRoundClip() { c.round.active = false }

// HasRoundClip сообщает, включено ли скруглённое отсечение (для тестов и
// диагностики).
func (c *Canvas) HasRoundClip() bool { return c.round.active }

// spanX сужает горизонтальный интервал [x0, x1) строки y по скруглённому
// контуру. ok=false — строка отсечена целиком.
//
// Вызывается на каждую строку каждого примитива, поэтому первым делом —
// дешёвый выход для выключенного отсечения.
func (s *roundClipState) spanX(y, x0, x1 int) (int, int, bool) {
	if !s.active {
		return x0, x1, true
	}
	if y < s.rect.Min.Y || y >= s.rect.Max.Y {
		return 0, 0, false
	}
	inset := 0
	switch {
	case y < s.rect.Min.Y+s.radius: // верхняя угловая зона
		inset = s.insets[y-s.rect.Min.Y]
	case y >= s.rect.Max.Y-s.radius: // нижняя
		inset = s.insets[s.rect.Max.Y-1-y]
	}
	lo, hi := s.rect.Min.X+inset, s.rect.Max.X-inset
	if lo > x0 {
		x0 = lo
	}
	if hi < x1 {
		x1 = hi
	}
	if x0 >= x1 {
		return 0, 0, false
	}
	return x0, x1, true
}

// clipsTile сообщает, отрезает ли скруглённый клип что-нибудь от тайла:
// тайл либо выходит за прямоугольник клипа, либо задевает дугу угла.
//
// Нужно классификации содержимого: тайл, у которого срезан угол, не сплошной,
// сколько бы заливка ни накрывала его прямоугольник.
//
// Пересечения с угловым КВАДРАТОМ мало: в него попадает и внутренняя часть,
// которую дуга не трогает. Считается расстояние от центра дуги до дальней
// точки тайла внутри этого квадрата — срезается только то, что вышло за
// радиус.
func (r *roundClipState) clipsTile(tile image.Rectangle) bool {
	if !r.active {
		return false
	}
	if !tile.In(r.rect) {
		return true // часть тайла вне клипа — там прежнее содержимое
	}
	rad := r.radius
	if rad <= 0 {
		return false
	}
	radSq := float64(rad) * float64(rad)

	// Центры четырёх дуг и их квадранты.
	type corner struct {
		cx, cy int
		zone   image.Rectangle
	}
	corners := [4]corner{
		{r.rect.Min.X + rad, r.rect.Min.Y + rad,
			image.Rect(r.rect.Min.X, r.rect.Min.Y, r.rect.Min.X+rad, r.rect.Min.Y+rad)},
		{r.rect.Max.X - rad, r.rect.Min.Y + rad,
			image.Rect(r.rect.Max.X-rad, r.rect.Min.Y, r.rect.Max.X, r.rect.Min.Y+rad)},
		{r.rect.Min.X + rad, r.rect.Max.Y - rad,
			image.Rect(r.rect.Min.X, r.rect.Max.Y-rad, r.rect.Min.X+rad, r.rect.Max.Y)},
		{r.rect.Max.X - rad, r.rect.Max.Y - rad,
			image.Rect(r.rect.Max.X-rad, r.rect.Max.Y-rad, r.rect.Max.X, r.rect.Max.Y)},
	}

	for _, c := range corners {
		part := tile.Intersect(c.zone)
		if part.Empty() {
			continue
		}
		// Дальняя точка части тайла от центра дуги.
		dx := maxAbs(part.Min.X-c.cx, part.Max.X-1-c.cx)
		dy := maxAbs(part.Min.Y-c.cy, part.Max.Y-1-c.cy)
		if float64(dx*dx+dy*dy) > radSq {
			return true
		}
	}
	return false
}

// maxAbs — большее по модулю из двух смещений.
func maxAbs(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a > b {
		return a
	}
	return b
}

// contains сообщает, лежит ли точка внутри скруглённого контура.
func (s *roundClipState) contains(x, y int) bool {
	if !s.active {
		return true
	}
	x0, x1, ok := s.spanX(y, x, x+1)
	return ok && x0 <= x && x < x1
}
