package dialog

// dialog_widgets.go — внутренние виджеты стандартных диалогов:
// mutedLabel (вторичный текст), crumbBar (кликабельный путь-breadcrumb),
// placeList (панель «мест»), fileTable (список файлов с колонками и иконками).
//
// Цвета читаются из глобальной палитры активной темы (base.ActiveTheme) в момент
// отрисовки — виджеты следуют теме без ApplyTheme.

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// Семантические цвета файловых иконок (не зависят от темы, как severity).
var (
	folderIconColor = color.RGBA{R: 240, G: 170, B: 60, A: 255}  // жёлтая папка
	fileIconColor   = color.RGBA{R: 150, G: 155, B: 175, A: 255} // серый файл
)

// ─── mutedLabel ─────────────────────────────────────────────────────────────

// newMutedLabel создаёт метку вторичного текста (серый: InputPlaceholder
// темы; Label.Muted удерживает приглушённый цвет при смене темы).
func newMutedLabel(text string) *base.Label {
	m := base.NewLabel(text, base.ActiveTheme().InputPlaceholder)
	m.Muted = true
	return m
}

// ─── crumbBar ───────────────────────────────────────────────────────────────

// crumbBar — строка пути в виде кликабельных сегментов
// «home › oops › documents › reports» (последний — акцентным цветом).
type crumbBar struct {
	base.Base
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

func (cb *crumbBar) Draw(ctx base.DrawContext) {
	b := cb.Base.Bounds()
	if b.Empty() {
		return
	}
	st := base.CurrentThemeStyle()
	if st.Classic3D {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), base.ActiveTheme().InputBG)
		base.DrawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
	} else {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, base.ActiveTheme().InputBG)
		ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, base.ActiveTheme().InputBorder)
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
		ctx.DrawTextSize("…", x, ty, fs, base.ActiveTheme().InputPlaceholder)
		x += ellW + 6
		ctx.DrawTextSize("›", x, ty, fs, base.ActiveTheme().InputPlaceholder)
		x += sepW + 6
	}
	drawn := segs[first:]
	for i := range drawn {
		if i > 0 {
			ctx.DrawTextSize("›", x, ty, fs, base.ActiveTheme().InputPlaceholder)
			x += sepW + 6
		}
		col := base.ActiveTheme().InputText
		if i == len(drawn)-1 {
			col = base.ActiveTheme().Accent
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

func (cb *crumbBar) OnMouseButton(e base.MouseEvent) bool {
	if e.Button != base.MouseLeft || !e.Pressed || !image.Pt(e.X, e.Y).In(cb.Base.Bounds()) {
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
		return base.Tr(pi.localeKey)
	}
	return pi.label
}

// placeList — боковая панель быстрых переходов (Домашняя, Документы, диски…).
type placeList struct {
	base.Base
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

func (pl *placeList) Draw(ctx base.DrawContext) {
	b := pl.Base.Bounds()
	if b.Empty() {
		return
	}
	st := base.CurrentThemeStyle()
	if st.Classic3D {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), base.ActiveTheme().InputBG)
		base.DrawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
	} else {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, base.ActiveTheme().InputBG)
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
		title := base.EllipsizeText(ctx, it.title(), b.Dx()-26, 11)
		ctx.DrawTextSize(title, b.Min.X+16, y+(placeRowH-18)/2, 11, base.ActiveTheme().InputText)
		y += placeRowH
	}
}

func (pl *placeList) OnMouseButton(e base.MouseEvent) bool {
	if e.Button != base.MouseLeft || !e.Pressed || !image.Pt(e.X, e.Y).In(pl.Base.Bounds()) {
		return false
	}
	idx := (e.Y - pl.Base.Bounds().Min.Y - 8) / placeRowH
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

// fillSelRow — подсветка выбранной строки (тема ListItemSelect); при
// полупрозрачном цвете темы используем честное альфа-смешивание.
func fillSelRow(ctx base.DrawContext, x, y, w, h int) {
	c := base.ActiveTheme().ListItemSelect
	if base.CurrentThemeStyle().Classic3D {
		ctx.FillRect(x, y, w, h, base.ActiveTheme().MenuHoverBG)
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
	base.Base
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
	n := (ft.Base.Bounds().Dy() - ftHeaderH - 10) / ftRowH
	if n < 1 {
		n = 1
	}
	return n
}

func (ft *fileTable) Draw(ctx base.DrawContext) {
	b := ft.Base.Bounds()
	if b.Empty() {
		return
	}
	st := base.CurrentThemeStyle()
	if st.Classic3D {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), base.ActiveTheme().InputBG)
		base.DrawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
	} else {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 6, base.ActiveTheme().InputBG)
	}

	sizeX := b.Max.X - 160
	dateX := b.Max.X - 92

	// Заголовок колонок.
	hy := b.Min.Y + 8
	hcol := base.ActiveTheme().InputPlaceholder
	ctx.DrawTextSize(base.Tr("dlg.file.col.name"), b.Min.X+38, hy, 10, hcol)
	ctx.DrawTextSize(base.Tr("dlg.file.col.size"), sizeX, hy, 10, hcol)
	ctx.DrawTextSize(base.Tr("dlg.file.col.date"), dateX, hy, 10, hcol)
	ctx.DrawHLine(b.Min.X+8, b.Min.Y+ftHeaderH, b.Dx()-16, base.ActiveTheme().Border)

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
		name := base.EllipsizeText(ctx, e.name, sizeX-(b.Min.X+38)-10, 11)
		ctx.DrawTextSize(name, b.Min.X+38, y+(ftRowH-16)/2, 11, base.ActiveTheme().InputText)
		if !e.dir {
			ctx.DrawTextSize(humanSize(e.size), sizeX, y+(ftRowH-14)/2, 10, base.ActiveTheme().InputPlaceholder)
		}
		if !e.mod.IsZero() {
			ctx.DrawTextSize(e.mod.Format("02.01.2006"), dateX, y+(ftRowH-14)/2, 10, base.ActiveTheme().InputPlaceholder)
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
		ctx.FillRoundRect(b.Max.X-7, ty, 4, thumbH, 2, base.ActiveTheme().ScrollThumbBG)
	}
}

// drawFileIcon рисует пиктограмму папки (жёлтая с «язычком») или файла
// (серый лист с линиями) в стиле принятых мокапов. (x, y) — левый верх.
func drawFileIcon(ctx base.DrawContext, x, y int, dir bool) {
	if dir {
		ctx.FillRect(x, y, 8, 5, folderIconColor) // язычок
		ctx.FillRoundRect(x, y+3, 16, 12, 2, folderIconColor)
		return
	}
	ctx.FillRoundRect(x+1, y, 13, 16, 2, fileIconColor)
	if aa, ok := ctx.(base.AAShapes); ok {
		aa.DrawLineAA(x+4, y+5, x+11, y+5, 1, base.ActiveTheme().InputBG)
		aa.DrawLineAA(x+4, y+9, x+11, y+9, 1, base.ActiveTheme().InputBG)
	}
}

func (ft *fileTable) rowAt(x, y int) int {
	b := ft.Base.Bounds()
	if !image.Pt(x, y).In(b) || y < b.Min.Y+ftHeaderH+6 {
		return -1
	}
	idx := ft.scroll + (y-b.Min.Y-ftHeaderH-6)/ftRowH
	if idx < 0 || idx >= len(ft.entries) {
		return -1
	}
	return idx
}

func (ft *fileTable) OnMouseButton(e base.MouseEvent) bool {
	inside := image.Pt(e.X, e.Y).In(ft.Base.Bounds())
	// Колесо: прокрутка.
	if inside && (e.Button == base.MouseWheelUp || e.Button == base.MouseWheelDown) && e.Pressed {
		ft.mu.Lock()
		old := ft.scroll
		if e.Button == base.MouseWheelUp {
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
	if e.Button != base.MouseLeft || !e.Pressed || !inside {
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
func (ft *fileTable) OnKeyEvent(e base.KeyEvent) {
	if !e.Pressed {
		return
	}
	ft.mu.Lock()
	old := ft.sel
	switch e.Code {
	case base.KeyUp:
		if ft.sel > 0 {
			ft.sel--
		} else if ft.sel < 0 && len(ft.entries) > 0 {
			ft.sel = 0
		}
	case base.KeyDown:
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
