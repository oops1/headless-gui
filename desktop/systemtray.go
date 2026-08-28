// systemtray.go — контейнер значков состояния в правой части панели задач.
//
// Значков со временем становится больше, чем места: настоящая панель задач
// решает это раскрывающейся областью — часть значков прячется, и их
// показывает шеврон. Здесь так же, и по той же причине: чем сжимать значки
// до нечитаемых точек, честнее показать столько, сколько влезает, а
// остальные убрать за одну кнопку.
package desktop

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Имена компонентов трея для стилей темы.
const (
	// ComponentTray — сам контейнер (обычно без собственной подложки).
	ComponentTray = "tray"
	// ComponentTrayChevron — кнопка «показать скрытые значки».
	ComponentTrayChevron = "tray.chevron"
	// ComponentTrayOverflow — всплывающее окно со скрытыми значками.
	ComponentTrayOverflow = "tray.overflow"
)

const (
	// KeyTrayGap — зазор между значками трея.
	KeyTrayGap theme.Key = "tray.gap"
	// KeyTrayChevronWidth — ширина кнопки раскрытия. Если тема её не задала,
	// берётся половина размера значка: кнопка узкая по своей природе.
	KeyTrayChevronWidth theme.Key = "tray.chevron.width"
	// KeyTrayOverflowColumns — сколько значков в ряду раскрывающейся области.
	KeyTrayOverflowColumns theme.Key = "tray.overflow.columns"
)

// SystemTray — ряд значков состояния с раскрывающейся областью.
//
// Сам является Item, поэтому кладётся в панель задач как обычный элемент:
// панель не знает, что внутри их несколько.
type SystemTray struct {
	widget.Base

	tm    *theme.Manager
	items []Item

	// hidden — значки, которым не хватило места в текущей раскладке.
	hidden []Item
	// chevron — прямоугольник кнопки раскрытия (пустой, если всё влезло).
	chevron image.Rectangle

	// overflow — всплывающая область со скрытыми значками.
	overflow *Flyout
}

// NewSystemTray создаёт контейнер значков, оформляемый темой tm.
func NewSystemTray(tm *theme.Manager) *SystemTray {
	t := &SystemTray{tm: tm}
	t.overflow = NewFlyout(tm, ComponentTrayOverflow)
	t.overflow.Align = AlignEnd
	t.overflow.Size = t.overflowSize
	t.overflow.Content = t.drawOverflow
	// drawOverflow выставляет скрытым значкам настоящие границы, чтобы они
	// ловили мышь внутри раскрытой области. При закрытии (клик мимо, Esc,
	// повторный щелчок по шеврону) эти границы сами не пропадают — значок
	// невидим, но Bounds() всё ещё указывают туда, где была область, и
	// следующий клик по пустому месту попал бы в него. relayout() уже прячет
	// значки так же (SetBounds(image.Rectangle{})) при пересчёте раскладки,
	// так что обнуление здесь идемпотентно с ним, а не конфликтует; при
	// следующем открытии drawOverflow снова расставит границы заново.
	t.overflow.OnClose = func() {
		for _, h := range t.hidden {
			h.SetBounds(image.Rectangle{})
		}
	}
	return t
}

// AddItem добавляет значок. Порядок добавления — порядок слева направо;
// прячутся при нехватке места последние, то есть добавленные позже.
func (t *SystemTray) AddItem(it Item) {
	if it == nil {
		return
	}
	t.items = append(t.items, it)
	t.AddChild(it)
	t.relayout()
}

// Items возвращает все значки контейнера.
func (t *SystemTray) Items() []Item { return t.items }

// Hidden возвращает значки, не поместившиеся в текущую раскладку.
func (t *SystemTray) Hidden() []Item { return t.hidden }

// Overflow возвращает всплывающую область скрытых значков — оболочке она
// нужна, чтобы задать ей границы экрана.
func (t *SystemTray) Overflow() *Flyout { return t.overflow }

// PreferredSize — сумма ширин значков с зазорами, но не больше доступного.
// Если всё не влезает, к запросу добавляется место под кнопку раскрытия.
func (t *SystemTray) PreferredSize(avail image.Point) image.Point {
	if len(t.items) == 0 {
		return image.Point{}
	}
	gap := t.metric(KeyTrayGap)
	want := 0
	for i, it := range t.items {
		if i > 0 {
			want += gap
		}
		want += it.PreferredSize(avail).X
	}
	if avail.X > 0 && want > avail.X {
		want = avail.X
	}
	return image.Pt(want, avail.Y)
}

// SetBounds задаёт границы контейнера и раскладывает значки.
func (t *SystemTray) SetBounds(r image.Rectangle) {
	t.Base.SetBounds(r)
	t.relayout()
}

// relayout раскладывает значки слева направо, пряча не поместившиеся.
//
// Кнопка раскрытия занимает место ПЕРВОЙ, а не последней: иначе решение
// «прятать ли значки» зависело бы от того, поместилась ли кнопка, а место
// под неё — от того, прячем ли мы значки. Поэтому сначала считается, влезают
// ли все значки без кнопки, и только если нет — резервируется место под неё.
func (t *SystemTray) relayout() {
	b := t.Bounds()
	t.hidden = nil
	t.chevron = image.Rectangle{}
	if b.Empty() || len(t.items) == 0 {
		return
	}

	gap := t.metric(KeyTrayGap)
	avail := image.Pt(b.Dx(), b.Dy())

	widths := make([]int, len(t.items))
	total := 0
	for i, it := range t.items {
		widths[i] = it.PreferredSize(avail).X
		total += widths[i]
		if i > 0 {
			total += gap
		}
	}

	room := b.Dx()
	if total > room {
		room -= t.chevronWidth() + gap
	}

	x := b.Min.X
	for i, it := range t.items {
		w := widths[i]
		if x+w > b.Min.X+room {
			t.hidden = append(t.hidden, t.items[i:]...)
			for _, h := range t.items[i:] {
				h.SetBounds(image.Rectangle{}) // спрятанный не рисуется и не ловит мышь
			}
			break
		}
		place(it, image.Rect(x, b.Min.Y, x+w, b.Min.Y+it.PreferredSize(avail).Y), b)
		x += w + gap
	}

	if len(t.hidden) > 0 {
		t.chevron = image.Rect(b.Max.X-t.chevronWidth(), b.Min.Y, b.Max.X, b.Max.Y)
	}
}

// Draw рисует подложку контейнера, значки и кнопку раскрытия.
func (t *SystemTray) Draw(ctx widget.DrawContext) {
	b := t.Bounds()
	if b.Empty() {
		return
	}
	// Своей подложки у трея нет: ни в одной настоящей панели он не выделен
	// плашкой, а стиль по умолчанию дал бы ему заливку темы — светлый
	// прямоугольник поверх тёмной панели.
	t.DrawChildren(ctx)

	if t.chevron.Empty() {
		return
	}
	s := t.style(ComponentTrayChevron, theme.StateNormal)
	PaintStyle(ctx, t.chevron, s)
	drawChevron(ctx, t.chevron, t.overflow.IsOpen(), ink(s))
}

// OnMouseButton раскрывает область скрытых значков щелчком по шеврону.
func (t *SystemTray) OnMouseButton(e widget.MouseEvent) bool {
	if t.chevron.Empty() || e.Button != widget.MouseLeft || !e.Pressed {
		return false
	}
	if !image.Pt(e.X, e.Y).In(t.chevron) {
		return false
	}
	t.overflow.Toggle(t.chevron)
	return true
}

// Close закрывает раскрывающуюся область и освобождает значки, которые этого
// требуют (часы, значки состояния держат подписки).
func (t *SystemTray) Close() {
	t.overflow.Close()
	for _, it := range t.items {
		if c, ok := it.(interface{ Close() }); ok {
			c.Close()
		}
	}
}

// overflowSize — размер раскрывающейся области: сетка скрытых значков.
func (t *SystemTray) overflowSize() image.Point {
	if len(t.hidden) == 0 {
		return image.Point{}
	}
	cols := int(t.metricF(KeyTrayOverflowColumns))
	if cols <= 0 {
		cols = 3 // столько же, сколько в раскрывающейся области Windows
	}
	cell := t.cellSize()
	gap := t.metric(KeyTrayGap)
	pad := int(t.style(ComponentTrayOverflow, theme.StateNormal).PadX)

	n := len(t.hidden)
	if n < cols {
		cols = n
	}
	rows := (n + cols - 1) / cols
	return image.Pt(
		cols*cell.X+(cols-1)*gap+2*pad,
		rows*cell.Y+(rows-1)*gap+2*pad,
	)
}

// drawOverflow раскладывает и рисует скрытые значки сеткой.
//
// Значки получают НАСТОЯЩИЕ границы внутри всплывающей области — иначе они
// рисовались бы, но не ловили мышь, и щелчок по значку в раскрытой области
// не работал бы.
func (t *SystemTray) drawOverflow(ctx widget.DrawContext, r image.Rectangle) {
	cols := int(t.metricF(KeyTrayOverflowColumns))
	if cols <= 0 {
		cols = 3
	}
	cell := t.cellSize()
	gap := t.metric(KeyTrayGap)

	for i, it := range t.hidden {
		col, row := i%cols, i/cols
		x := r.Min.X + col*(cell.X+gap)
		y := r.Min.Y + row*(cell.Y+gap)
		cellRect := image.Rect(x, y, x+cell.X, y+cell.Y)
		if !cellRect.In(r) {
			break // не влезло — лучше не показать, чем нарисовать поверх края
		}
		it.SetBounds(cellRect)
		it.Draw(ctx)
	}
}

// cellSize — размер ячейки сетки: самый широкий из скрытых значков, чтобы
// сетка была ровной.
func (t *SystemTray) cellSize() image.Point {
	avail := image.Pt(t.Bounds().Dx(), t.Bounds().Dy())
	cell := image.Pt(0, 0)
	for _, it := range t.hidden {
		sz := it.PreferredSize(avail)
		if sz.X > cell.X {
			cell.X = sz.X
		}
		if sz.Y > cell.Y {
			cell.Y = sz.Y
		}
	}
	if cell.Y <= 0 {
		cell.Y = trayIconSize(t.tm)
	}
	return cell
}

// chevronWidth — ширина кнопки раскрытия.
func (t *SystemTray) chevronWidth() int {
	if w := t.metric(KeyTrayChevronWidth); w > 0 {
		return w
	}
	if s := trayIconSize(t.tm); s > 0 {
		return s / 2
	}
	return 0
}

func (t *SystemTray) metric(k theme.Key) int { return int(t.metricF(k)) }

func (t *SystemTray) metricF(k theme.Key) float64 {
	if t.tm == nil {
		return 0
	}
	return t.tm.GetMetric(k)
}

func (t *SystemTray) style(component string, st theme.State) *theme.Style {
	return styleOf(t.tm, component, "", st)
}

// drawChevron рисует уголок: вверх — «показать скрытые», вниз — «свернуть».
//
// Фигурой, а не глифом шрифта: значок должен быть одинаковым при любом
// шрифте темы, включая тот, в котором нужного символа нет вовсе.
func drawChevron(ctx widget.DrawContext, r image.Rectangle, open bool, col color.RGBA) {
	if col.A == 0 || r.Empty() {
		return
	}
	// Сторона уголка — треть меньшей стороны кнопки: так он остаётся уголком
	// и на мелкой панели Windows 2000, и на крупной macOS.
	size := r.Dx()
	if r.Dy() < size {
		size = r.Dy()
	}
	size /= 3
	if size < 2 {
		size = 2
	}
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	for i := 0; i <= size; i++ {
		dy := i
		if open {
			dy = -i
		}
		ctx.SetPixel(cx-i, cy+dy/2, col)
		ctx.SetPixel(cx+i, cy+dy/2, col)
		// Вторая строка пикселей — иначе уголок теряется на светлом фоне.
		ctx.SetPixel(cx-i, cy+dy/2+1, col)
		ctx.SetPixel(cx+i, cy+dy/2+1, col)
	}
}
