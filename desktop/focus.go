// focus.go — клавиатурный фокус элементов панели задач.
//
// До этого файла в desktop ничего не реализовывало widget.Focusable: панель
// нельзя было обойти с клавиатуры, а «Пуск», часы и значки трея — нажать
// Enter'ом. Теперь все элементы панели — Focusable, и обход устроен так, как
// его ждут от панели задач Windows:
//
//   - Tab и Shift+Tab ходят по элементам в порядке областей панели: «Пуск»,
//     (поиск и прочее в SlotStart), приложения, трей, часы. Порядок обхода —
//     порядок областей (Taskbar.Children), а не порядок добавления элементов;
//   - внутри области стрелки Влево/Вправо (и Вверх/Вниз), Home и End переносят
//     фокус к соседу, не покидая области; в области приложений они
//     переходят по ячейкам одного элемента;
//   - Enter и Space нажимают то, на чём стоит фокус;
//   - рамка фокуса — двухцветный контур (цвет текста стиля и контрастный
//     внутренний), видимый на любом фоне, а не смена цвета заливки. Рисует её
//     панель поверх элементов.
//
// Рамка показывается, только если фокус пришёл с клавиатуры: щелчок мышью по
// «Пуску» не оставляет вокруг него контур (так ведёт себя Windows).
package desktop

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/internal/focusreq"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Ключи токенов рамки фокуса. Необязательны: без них рамка рисуется цветом
// текста стиля элемента толщиной defaultFocusRingWidth.
const (
	// KeyFocusRingWidth — толщина наружного контура рамки в логических
	// пикселях (внутренний контрастный контур всегда в один пиксель).
	KeyFocusRingWidth theme.Key = "focus.ring.width"
	// KeyFocusRingColor — цвет наружного контура. Нет — цвет текста стиля.
	KeyFocusRingColor theme.Key = "focus.ring"
)

// defaultFocusRingWidth — толщина наружного контура, пока тема её не задала.
// Единица: значки трея невелики, и контур потолще закрыл бы сам значок.
const defaultFocusRingWidth = 1

// FocusMove — куда перенести фокус внутри области панели.
type FocusMove int

const (
	// FocusPrev — к предыдущему элементу области.
	FocusPrev FocusMove = iota
	// FocusNext — к следующему.
	FocusNext
	// FocusFirst и FocusLast — к первому и последнему (Home, End).
	FocusFirst
	FocusLast
)

// FocusNavigator переносит фокус между элементами одной области панели.
// Реализует Taskbar; элементы получают его через FocusNavigable.
type FocusNavigator interface {
	// MoveFocus переносит фокус от from к соседу по области. Возвращает
	// false, если соседа нет (край области) и фокус остался на месте.
	MoveFocus(from widget.Widget, move FocusMove) bool
}

// FocusNavigable — элемент, принимающий навигатор. Панель вызывает
// SetFocusNavigator при добавлении элемента; встроенный FocusState уже его
// реализует, поэтому собственному элементу достаточно встроить FocusState.
type FocusNavigable interface {
	SetFocusNavigator(FocusNavigator)
}

// FocusRinger — элемент, у которого рамка фокуса не совпадает с его границами
// (в области приложений она обводит одну ячейку из многих). Без него панель
// обводит границы элемента.
type FocusRinger interface {
	// FocusRing возвращает прямоугольник рамки в координатах панели и стиль,
	// по которому выбирается цвет и скругление (nil — стиль панели).
	FocusRing() (image.Rectangle, *theme.Style)
}

// FocusState — состояние фокуса элемента панели. Встраивается в элемент:
// даёт ему IsFocused, FocusVisible, SetFocusNavigator и обработку клавиш
// (HandleKey, HandleCellKey). SetFocused элемент пишет сам — он должен
// перерисовать себя:
//
//	func (b *MyButton) SetFocused(v bool) {
//	    if b.FocusState.Set(v) { b.Invalidate() }
//	}
//
// Нулевое значение готово к работе. Безопасно для вызова из разных горутин:
// ввод идёт из горутины кадра, а подписки потребителя — из своей.
type FocusState struct {
	focused  int32 // 1 — элемент держит фокус
	pointer  int32 // 1 — фокус пришёл от мыши: рамку не показываем
	cellPlus int32 // выбранная ячейка + 1 (0 — не выбиралась)

	navMu sync.Mutex
	nav   FocusNavigator
}

// Set меняет признак фокуса и сообщает, изменилось ли что-то, что видно:
// элементу тогда нужно перерисоваться. Получение фокуса сбрасывает признак
// «от мыши» — фокус, пришедший по Tab, рамку показывает.
func (f *FocusState) Set(focused bool) bool {
	want := b2i32(focused)
	prevFocused := atomic.SwapInt32(&f.focused, want)
	prevPointer := atomic.SwapInt32(&f.pointer, 0)
	return prevFocused != want || (focused && prevPointer != 0)
}

// IsFocused реализует widget.Focusable.
func (f *FocusState) IsFocused() bool { return atomic.LoadInt32(&f.focused) == 1 }

// FocusVisible — нужно ли рисовать рамку: элемент в фокусе, и фокус пришёл
// с клавиатуры, а не от щелчка мыши.
func (f *FocusState) FocusVisible() bool {
	return atomic.LoadInt32(&f.focused) == 1 && atomic.LoadInt32(&f.pointer) == 0
}

// NotePointer сообщает, что элемент нажали мышью: если фокус пришёл вместе с
// нажатием, рамка не нужна. Возвращает true, если рамка при этом погасла.
// Зовётся из OnMouseButton элемента.
func (f *FocusState) NotePointer(e widget.MouseEvent) bool {
	if e.Button != widget.MouseLeft || !e.Pressed {
		return false
	}
	wasVisible := f.FocusVisible()
	if f.IsFocused() {
		atomic.StoreInt32(&f.pointer, 1)
	}
	return wasVisible
}

// noteKey: клавиатура снова в деле — рамка возвращается. Истина, если она
// появилась.
func (f *FocusState) noteKey() bool {
	return atomic.SwapInt32(&f.pointer, 0) != 0 && f.IsFocused()
}

// SetFocusNavigator реализует FocusNavigable.
func (f *FocusState) SetFocusNavigator(n FocusNavigator) {
	f.navMu.Lock()
	f.nav = n
	f.navMu.Unlock()
}

func (f *FocusState) navigator() FocusNavigator {
	f.navMu.Lock()
	defer f.navMu.Unlock()
	return f.nav
}

// Cell возвращает выбранную ячейку многоячеечного элемента при n ячейках:
// число в [0, n) или -1, если ячеек нет. Не выбиралась — первая.
func (f *FocusState) Cell(n int) int {
	if n <= 0 {
		return -1
	}
	c := int(atomic.LoadInt32(&f.cellPlus)) - 1
	if c < 0 {
		return 0
	}
	if c >= n {
		return n - 1
	}
	return c
}

// SetCell запоминает выбранную ячейку.
func (f *FocusState) SetCell(i int) {
	if i < 0 {
		i = 0
	}
	atomic.StoreInt32(&f.cellPlus, int32(i)+1)
}

// plainKey — клавиша без модификаторов, которые делают её сочетанием.
func plainKey(e widget.KeyEvent) bool {
	return e.Pressed && e.Mod&(widget.ModCtrl|widget.ModAlt|widget.ModMeta) == 0
}

// HandleKey разбирает клавишу у элемента с одним местом фокуса: Enter и Space
// вызывают activate, стрелки, Home и End переносят фокус к соседям по
// области (через навигатор). Возвращает true, если клавиша обработана.
// invalidate зовётся, когда изменился видимый вид (вернулась рамка).
func (f *FocusState) HandleKey(self widget.Widget, e widget.KeyEvent, activate func(), invalidate func()) bool {
	if !plainKey(e) {
		return false
	}
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace:
		if f.noteKey() && invalidate != nil {
			invalidate()
		}
		if !e.Repeat && activate != nil {
			activate()
		}
		return true
	}
	if mv, ok := moveForKey(e.Code); ok {
		if f.noteKey() && invalidate != nil {
			invalidate()
		}
		if n := f.navigator(); n != nil {
			n.MoveFocus(self, mv)
		}
		return true
	}
	return false
}

// HandleCellKey — HandleKey для элемента из n ячеек (область приложений):
// стрелки, Home и End двигают выбранную ячейку, а не фокус между элементами;
// Enter и Space вызывают activate для выбранной. Возвращает true, если
// клавиша обработана.
func (f *FocusState) HandleCellKey(e widget.KeyEvent, n int, activate func(cell int), invalidate func()) bool {
	if !plainKey(e) {
		return false
	}
	cur := f.Cell(n)
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace:
		if f.noteKey() && invalidate != nil {
			invalidate()
		}
		if !e.Repeat && cur >= 0 && activate != nil {
			activate(cur)
		}
		return true
	}
	mv, ok := moveForKey(e.Code)
	if !ok {
		return false
	}
	changed := f.noteKey()
	if cur >= 0 {
		next := cur
		switch mv {
		case FocusPrev:
			next = cur - 1
		case FocusNext:
			next = cur + 1
		case FocusFirst:
			next = 0
		case FocusLast:
			next = n - 1
		}
		if next >= 0 && next < n && next != cur {
			f.SetCell(next)
			changed = true
		}
	}
	if changed && invalidate != nil {
		invalidate()
	}
	return true
}

// moveForKey — какое перемещение значит клавиша.
func moveForKey(c widget.KeyCode) (FocusMove, bool) {
	switch c {
	case widget.KeyLeft, widget.KeyUp:
		return FocusPrev, true
	case widget.KeyRight, widget.KeyDown:
		return FocusNext, true
	case widget.KeyHome:
		return FocusFirst, true
	case widget.KeyEnd:
		return FocusLast, true
	}
	return 0, false
}

func b2i32(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

// focusTabIndex — TabIndex элемента панели: -1 (исключён из обхода), пока у
// элемента нет места на панели. Скрытый значок без границ иначе оставался бы
// невидимой остановкой Tab.
func focusTabIndex(w widget.Widget) int {
	if w.Bounds().Empty() {
		return -1
	}
	return 0
}

// moveFocusTo переносит фокус на w: через движок, если сейчас доставляется
// его событие (так фокус получает и инвалидацию рамки), иначе напрямую — для
// интерфейса, собранного без движка.
func moveFocusTo(from, to widget.Widget) {
	if from == to || to == nil {
		return
	}
	if focusreq.Request(to) {
		return
	}
	if f, ok := from.(widget.Focusable); ok {
		f.SetFocused(false)
	}
	if f, ok := to.(widget.Focusable); ok {
		f.SetFocused(true)
	}
}

// ─── Рамка ───────────────────────────────────────────────────────────────────

// PaintFocusRing рисует рамку фокуса по границам r: наружный контур цветом
// текста стиля s (или токена KeyFocusRingColor) и внутренний в один пиксель
// контрастного цвета. Два цвета — не прихоть: контур одного цвета теряется на
// фоне, который с ним совпал (белая рамка на светлой кнопке), а пара «цвет и
// его противоположность» видна на любом.
//
// Скругление берётся из s.Corner. s == nil допустим — тогда контур белый.
func PaintFocusRing(ctx widget.DrawContext, r image.Rectangle, tm *theme.Manager, s *theme.Style) {
	if r.Empty() {
		return
	}
	outer := focusRingColor(tm, s)
	inner := contrastInk(outer)
	w := defaultFocusRingWidth
	if tm != nil {
		if v := int(tm.GetMetric(KeyFocusRingWidth)); v > 0 {
			w = v
		}
	}
	corner := 0
	if s != nil {
		corner = int(s.Corner)
	}

	// Контур строится от внешнего края внутрь, чтобы не вылезти за границы
	// элемента и не быть обрезанным клипом соседа.
	rr := r
	for i := 0; i < w; i++ {
		strokeRect(ctx, rr, corner, outer)
		rr = rr.Inset(1)
		if corner > 0 {
			corner--
		}
	}
	strokeRect(ctx, rr, corner, inner)
}

// strokeRect рисует однопиксельный контур, скруглённый при corner > 0.
func strokeRect(ctx widget.DrawContext, r image.Rectangle, corner int, col color.RGBA) {
	if r.Empty() {
		return
	}
	if corner > 0 {
		ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), corner, col)
		return
	}
	ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), col)
}

// focusRingColor — цвет наружного контура: токен темы, иначе цвет текста
// стиля, иначе белый. Всегда непрозрачный.
func focusRingColor(tm *theme.Manager, s *theme.Style) color.RGBA {
	if tm != nil {
		if t := tm.Active(); t != nil {
			if c, ok := t.Color(KeyFocusRingColor); ok && c.A > 0 {
				return opaque(c)
			}
		}
	}
	if s != nil {
		for _, c := range []color.RGBA{s.Text, s.Border, s.Fill} {
			if c.A > 0 {
				return opaque(c)
			}
		}
	}
	return color.RGBA{R: 255, G: 255, B: 255, A: 255}
}

// opaque делает цвет непрозрачным, сохраняя оттенок (цвета темы хранятся с
// предумноженной альфой).
func opaque(c color.RGBA) color.RGBA {
	if c.A == 255 || c.A == 0 {
		c.A = 255
		return c
	}
	un := func(v uint8) uint8 {
		x := int(v) * 255 / int(c.A)
		if x > 255 {
			x = 255
		}
		return uint8(x)
	}
	return color.RGBA{R: un(c.R), G: un(c.G), B: un(c.B), A: 255}
}

// contrastInk — чёрный или белый, смотря что виднее на фоне c.
func contrastInk(c color.RGBA) color.RGBA {
	lum := (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000
	if lum > 128 {
		return color.RGBA{A: 255}
	}
	return color.RGBA{R: 255, G: 255, B: 255, A: 255}
}

// paintFocusOf рисует рамку для w, если он в фокусе, и фокус виден. Границы
// по умолчанию — Bounds, растянутые по высоте до bar (значок трея ниже
// панели, а рамка обводит всю кнопку, как в Windows).
func paintFocusOf(ctx widget.DrawContext, w widget.Widget, tm *theme.Manager, base *theme.Style, bar image.Rectangle) {
	f, ok := w.(widget.Focusable)
	if !ok || !f.IsFocused() {
		return
	}
	if v, ok := w.(interface{ FocusVisible() bool }); ok && !v.FocusVisible() {
		return
	}
	r := w.Bounds()
	style := base
	if fr, ok := w.(FocusRinger); ok {
		var s *theme.Style
		r, s = fr.FocusRing()
		if s != nil {
			style = s
		}
	} else if !r.Empty() && !bar.Empty() {
		r.Min.Y, r.Max.Y = bar.Min.Y, bar.Max.Y
	}
	PaintFocusRing(ctx, r, tm, style)
}
