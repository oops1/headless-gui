package widget

// dialog_widgets.go — внутренние виджеты стандартных диалогов:
// mutedLabel (вторичный текст), crumbBar (кликабельный путь-breadcrumb),
// placeList (панель «мест»), fileTable (список файлов с колонками и иконками).
//
// Цвета читаются из глобальной палитры активной темы (win10) в момент
// отрисовки — виджеты следуют теме без ApplyTheme.

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Семантические цвета файловых иконок (не зависят от темы, как severity).
var (
	folderIconColor = color.RGBA{R: 240, G: 170, B: 60, A: 255}  // жёлтая папка
	fileIconColor   = color.RGBA{R: 150, G: 155, B: 175, A: 255} // серый файл
)

// ─── mutedLabel ─────────────────────────────────────────────────────────────

// newMutedLabel создаёт метку вторичного текста (серый: InputPlaceholder
// темы; Label.Muted удерживает приглушённый цвет при смене темы).
func newMutedLabel(text string) *Label {
	m := NewLabel(text, win10.InputPlaceholder)
	m.Muted = true
	return m
}

// ─── crumbBar ───────────────────────────────────────────────────────────────

// crumbBar — строка пути в виде кликабельных сегментов
// «home › oops › documents › reports» (последний — акцентным цветом).
type crumbBar struct {
	Base
	mu   sync.Mutex
	path string
	segs []crumbSeg // пересчитывается при Draw (нужен MeasureText)

	// OnNavigate вызывается при клике по сегменту (передаётся его полный путь).
	OnNavigate func(dir string)
}

type crumbSeg struct {
	label string
	path  string
	hit   image.Rectangle // зона клика (абсолютные координаты)
}

func newCrumbBar() *crumbBar { return &crumbBar{} }

// SetPath задаёт отображаемый путь.
func (cb *crumbBar) SetPath(p string) {
	cb.mu.Lock()
	cb.path = p
	cb.mu.Unlock()
	cb.Invalidate()
}

// splitCrumbs разбивает абсолютный путь на сегменты с накопленными путями.
func splitCrumbs(p string) []crumbSeg {
	if p == "" {
		return nil
	}
	vol := filepath.VolumeName(p) // "C:" на Windows, "" на Unix
	rest := strings.Trim(strings.TrimPrefix(p, vol), `/\`)
	var segs []crumbSeg
	if vol != "" {
		segs = append(segs, crumbSeg{label: vol, path: vol + string(filepath.Separator)})
	} else {
		segs = append(segs, crumbSeg{label: "/", path: "/"})
	}
	if rest == "" {
		return segs
	}
	acc := segs[0].path
	for _, part := range strings.FieldsFunc(rest, func(r rune) bool { return r == '/' || r == '\\' }) {
		acc = filepath.Join(acc, part)
		segs = append(segs, crumbSeg{label: part, path: acc})
	}
	return segs
}

func (cb *crumbBar) Draw(ctx DrawContext) {
	b := cb.bounds
	if b.Empty() {
		return
	}
	st := currentStyle()
	if st.Classic3D {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), win10.InputBG)
		drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
	} else {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, win10.InputBG)
		ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, win10.InputBorder)
	}

	cb.mu.Lock()
	segs := splitCrumbs(cb.path)
	cb.mu.Unlock()

	const fs = 11
	sepW := ctx.MeasureText("›", fs)
	avail := b.Dx() - 20

	// Ширины сегментов; при переполнении отбрасываем головные («… › tail»).
	widths := make([]int, len(segs))
	total := 0
	for i, s := range segs {
		widths[i] = ctx.MeasureText(s.label, fs)
		total += widths[i]
		if i > 0 {
			total += sepW + 12
		}
	}
	first := 0
	ellW := ctx.MeasureText("…", fs)
	for total > avail && first < len(segs)-1 {
		total -= widths[first] + sepW + 12
		first++
	}

	x := b.Min.X + 10
	ty := b.Min.Y + (b.Dy()-14)/2
	if first > 0 {
		ctx.DrawTextSize("…", x, ty, fs, win10.InputPlaceholder)
		x += ellW + 6
		ctx.DrawTextSize("›", x, ty, fs, win10.InputPlaceholder)
		x += sepW + 6
	}
	drawn := segs[first:]
	for i := range drawn {
		if i > 0 {
			ctx.DrawTextSize("›", x, ty, fs, win10.InputPlaceholder)
			x += sepW + 6
		}
		col := win10.InputText
		if i == len(drawn)-1 {
			col = win10.Accent
		}
		w := ctx.MeasureText(drawn[i].label, fs)
		drawn[i].hit = image.Rect(x-2, b.Min.Y, x+w+4, b.Max.Y)
		ctx.DrawTextSize(drawn[i].label, x, ty, fs, col)
		x += w + 6
	}
	cb.mu.Lock()
	cb.segs = drawn
	cb.mu.Unlock()
}

func (cb *crumbBar) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed || !image.Pt(e.X, e.Y).In(cb.bounds) {
		return false
	}
	cb.mu.Lock()
	segs := cb.segs
	cb.mu.Unlock()
	for _, s := range segs {
		if image.Pt(e.X, e.Y).In(s.hit) {
			if cb.OnNavigate != nil {
				cb.OnNavigate(s.path)
			}
			return true
		}
	}
	return true // клик по полосе поглощаем
}

// ─── placeList ──────────────────────────────────────────────────────────────

// placeItem — элемент панели «мест» файлового диалога.
type placeItem struct {
	label     string // готовая подпись (если localeKey пуст)
	localeKey string // ключ локализации (приоритетнее label)
	path      string
}

func (pi placeItem) title() string {
	if pi.localeKey != "" {
		return Tr(pi.localeKey)
	}
	return pi.label
}

// placeList — боковая панель быстрых переходов (Домашняя, Документы, диски…).
type placeList struct {
	Base
	mu    sync.Mutex
	items []placeItem
	cur   string // текущий каталог диалога — подсвечивает совпавшее место

	OnNavigate func(dir string)
}

func newPlaceList(items []placeItem) *placeList { return &placeList{items: items} }

func (pl *placeList) SetCurrent(dir string) {
	pl.mu.Lock()
	pl.cur = dir
	pl.mu.Unlock()
	pl.Invalidate()
}

const placeRowH = 30

func (pl *placeList) Draw(ctx DrawContext) {
	b := pl.bounds
	if b.Empty() {
		return
	}
	st := currentStyle()
	if st.Classic3D {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), win10.InputBG)
		drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
	} else {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, win10.InputBG)
	}

	pl.mu.Lock()
	items := pl.items
	cur := pl.cur
	pl.mu.Unlock()

	y := b.Min.Y + 8
	for _, it := range items {
		if y+placeRowH > b.Max.Y {
			break
		}
		if it.path == cur {
			fillSelRow(ctx, b.Min.X+6, y, b.Dx()-12, placeRowH-4)
		}
		title := ellipsizeText(ctx, it.title(), b.Dx()-26, 11)
		ctx.DrawTextSize(title, b.Min.X+16, y+(placeRowH-18)/2, 11, win10.InputText)
		y += placeRowH
	}
}

func (pl *placeList) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed || !image.Pt(e.X, e.Y).In(pl.bounds) {
		return false
	}
	idx := (e.Y - pl.bounds.Min.Y - 8) / placeRowH
	pl.mu.Lock()
	ok := idx >= 0 && idx < len(pl.items)
	var path string
	if ok {
		path = pl.items[idx].path
	}
	pl.mu.Unlock()
	if ok && pl.OnNavigate != nil {
		pl.OnNavigate(path)
	}
	return true
}

// ellipsizeText обрезает строку многоточием так, чтобы она умещалась
// в maxW пикселей (шрифт default, sizePt).
func ellipsizeText(ctx DrawContext, s string, maxW int, sizePt float64) string {
	if maxW <= 0 || ctx.MeasureText(s, sizePt) <= maxW {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r) // максимальный префикс, влезающий с «…»
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if ctx.MeasureText(string(r[:mid])+"…", sizePt) <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(r[:lo]) + "…"
}

// fillSelRow — подсветка выбранной строки (тема ListItemSelect); при
// полупрозрачном цвете темы используем честное альфа-смешивание.
func fillSelRow(ctx DrawContext, x, y, w, h int) {
	c := win10.ListItemSelect
	if currentStyle().Classic3D {
		ctx.FillRect(x, y, w, h, win10.MenuHoverBG)
		return
	}
	if c.A == 255 {
		ctx.FillRoundRect(x, y, w, h, 4, c)
	} else {
		ctx.FillRectAlpha(x, y, w, h, c)
	}
}

// ─── fileTable ──────────────────────────────────────────────────────────────

// fileTable — список файлов с заголовком колонок (Имя/Размер/Изменён),
// иконками папок/файлов, выделением, прокруткой и двойным кликом.
type fileTable struct {
	Base
	mu      sync.Mutex
	entries []fileEntry
	sel     int
	scroll  int // индекс первой видимой строки

	lastClickIdx  int
	lastClickTime time.Time

	OnSelect   func(idx int)
	OnActivate func(idx int)
}

const (
	ftHeaderH = 26
	ftRowH    = 30
)

func newFileTable() *fileTable { return &fileTable{sel: -1, lastClickIdx: -1} }

// SetEntries задаёт содержимое (сбрасывает выделение и прокрутку).
func (ft *fileTable) SetEntries(entries []fileEntry) {
	ft.mu.Lock()
	ft.entries = entries
	ft.sel = -1
	ft.scroll = 0
	ft.mu.Unlock()
	ft.Invalidate()
}

// Selected возвращает индекс выделенной строки (-1 — нет).
func (ft *fileTable) Selected() int {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.sel
}

func (ft *fileTable) visibleRows() int {
	n := (ft.bounds.Dy() - ftHeaderH - 10) / ftRowH
	if n < 1 {
		n = 1
	}
	return n
}

func (ft *fileTable) Draw(ctx DrawContext) {
	b := ft.bounds
	if b.Empty() {
		return
	}
	st := currentStyle()
	if st.Classic3D {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), win10.InputBG)
		drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
	} else {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, win10.InputBG)
	}

	sizeX := b.Max.X - 160
	dateX := b.Max.X - 92

	// Заголовок колонок.
	hy := b.Min.Y + 8
	hcol := win10.InputPlaceholder
	ctx.DrawTextSize(Tr("dlg.file.col.name"), b.Min.X+38, hy, 10, hcol)
	ctx.DrawTextSize(Tr("dlg.file.col.size"), sizeX, hy, 10, hcol)
	ctx.DrawTextSize(Tr("dlg.file.col.date"), dateX, hy, 10, hcol)
	ctx.DrawHLine(b.Min.X+8, b.Min.Y+ftHeaderH, b.Dx()-16, win10.Border)

	ft.mu.Lock()
	entries := ft.entries
	sel := ft.sel
	scroll := ft.scroll
	ft.mu.Unlock()

	vis := ft.visibleRows()
	y := b.Min.Y + ftHeaderH + 6
	for i := scroll; i < len(entries) && i < scroll+vis; i++ {
		e := entries[i]
		if i == sel {
			fillSelRow(ctx, b.Min.X+6, y, b.Dx()-12, ftRowH-2)
		}
		drawFileIcon(ctx, b.Min.X+14, y+7, e.dir)
		name := ellipsizeText(ctx, e.name, sizeX-(b.Min.X+38)-10, 11)
		ctx.DrawTextSize(name, b.Min.X+38, y+(ftRowH-16)/2, 11, win10.InputText)
		if !e.dir {
			ctx.DrawTextSize(humanSize(e.size), sizeX, y+(ftRowH-14)/2, 10, win10.InputPlaceholder)
		}
		if !e.mod.IsZero() {
			ctx.DrawTextSize(e.mod.Format("02.01.2006"), dateX, y+(ftRowH-14)/2, 10, win10.InputPlaceholder)
		}
		y += ftRowH
	}

	// Тонкий индикатор прокрутки.
	if len(entries) > vis {
		trackH := b.Dy() - ftHeaderH - 12
		thumbH := trackH * vis / len(entries)
		if thumbH < 20 {
			thumbH = 20
		}
		maxScroll := len(entries) - vis
		ty := b.Min.Y + ftHeaderH + 6 + (trackH-thumbH)*scroll/maxScroll
		ctx.FillRoundRect(b.Max.X-7, ty, 4, thumbH, 2, win10.ScrollThumbBG)
	}
}

// drawFileIcon рисует пиктограмму папки (жёлтая с «язычком») или файла
// (серый лист с линиями) в стиле принятых мокапов. (x, y) — левый верх.
func drawFileIcon(ctx DrawContext, x, y int, dir bool) {
	if dir {
		ctx.FillRect(x, y, 8, 5, folderIconColor) // язычок
		ctx.FillRoundRect(x, y+3, 16, 12, 2, folderIconColor)
		return
	}
	ctx.FillRoundRect(x+1, y, 13, 16, 2, fileIconColor)
	if aa, ok := ctx.(AAShapes); ok {
		aa.DrawLineAA(x+4, y+5, x+11, y+5, 1, win10.InputBG)
		aa.DrawLineAA(x+4, y+9, x+11, y+9, 1, win10.InputBG)
	}
}

func (ft *fileTable) rowAt(x, y int) int {
	b := ft.bounds
	if !image.Pt(x, y).In(b) || y < b.Min.Y+ftHeaderH+6 {
		return -1
	}
	idx := ft.scroll + (y-b.Min.Y-ftHeaderH-6)/ftRowH
	if idx < 0 || idx >= len(ft.entries) {
		return -1
	}
	return idx
}

func (ft *fileTable) OnMouseButton(e MouseEvent) bool {
	inside := image.Pt(e.X, e.Y).In(ft.bounds)
	// Колесо: прокрутка.
	if inside && (e.Button == MouseWheelUp || e.Button == MouseWheelDown) && e.Pressed {
		ft.mu.Lock()
		old := ft.scroll
		if e.Button == MouseWheelUp {
			ft.scroll -= 2
		} else {
			ft.scroll += 2
		}
		ft.clampScroll()
		changed := ft.scroll != old
		ft.mu.Unlock()
		if changed {
			ft.Invalidate()
		}
		return true
	}
	if e.Button != MouseLeft || !e.Pressed || !inside {
		return false
	}

	ft.mu.Lock()
	idx := ft.rowAt(e.X, e.Y)
	if idx < 0 {
		ft.mu.Unlock()
		return true
	}
	dbl := idx == ft.lastClickIdx && time.Since(ft.lastClickTime) < 400*time.Millisecond
	ft.lastClickIdx = idx
	ft.lastClickTime = time.Now()
	ft.sel = idx
	onSel, onAct := ft.OnSelect, ft.OnActivate
	ft.mu.Unlock()

	ft.Invalidate()
	if dbl {
		if onAct != nil {
			onAct(idx)
		}
	} else if onSel != nil {
		onSel(idx)
	}
	return true
}

// OnKeyEvent — навигация стрелками (Enter обрабатывает Dialog.DefaultAction).
func (ft *fileTable) OnKeyEvent(e KeyEvent) {
	if !e.Pressed {
		return
	}
	ft.mu.Lock()
	old := ft.sel
	switch e.Code {
	case KeyUp:
		if ft.sel > 0 {
			ft.sel--
		} else if ft.sel < 0 && len(ft.entries) > 0 {
			ft.sel = 0
		}
	case KeyDown:
		if ft.sel < len(ft.entries)-1 {
			ft.sel++
		}
	default:
		ft.mu.Unlock()
		return
	}
	// Держим выделение видимым.
	vis := ft.visibleRows()
	if ft.sel < ft.scroll {
		ft.scroll = ft.sel
	} else if ft.sel >= ft.scroll+vis {
		ft.scroll = ft.sel - vis + 1
	}
	changed := ft.sel != old
	sel := ft.sel
	onSel := ft.OnSelect
	ft.mu.Unlock()
	if changed {
		ft.Invalidate()
		if onSel != nil {
			onSel(sel)
		}
	}
}

func (ft *fileTable) clampScroll() {
	maxScroll := len(ft.entries) - ft.visibleRows()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if ft.scroll > maxScroll {
		ft.scroll = maxScroll
	}
	if ft.scroll < 0 {
		ft.scroll = 0
	}
}
