// startmenu_tiles_letters.go — переход по буквам в списке приложений меню «Пуск»
// с плитками.
//
// Щелчок (или Enter и Пробел) по заголовку буквы открывает на месте списка
// сетку всех букв: «#», латиница, кириллица, если в списке есть кириллические
// группы, и прочие буквы источника. Буквы без приложений приглушены и не
// нажимаются. Выбор буквы прокручивает список к её группе и закрывает сетку;
// Esc закрывает сетку, не закрывая меню. Стрелки в сетке ходят по её геометрии
// (как в плитках), Home и End — к первой и последней доступной букве.
//
// Сетка не отдельная область: это состояние области списка (startView.grid).
// Выбор в ней хранится там же, где выбор строки списка, ключом "g:"+буква, а
// появление идёт по токену темы menu.open (прозрачность). Размер ячеек
// подбирается так, чтобы алфавит поместился в окно списка без прокрутки:
// от четырёх столбцов (как в Windows) до восьми на низком меню.
package desktop

import (
	"image"
	"strings"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// prefGrid — префикс ключа ячейки сетки букв.
const prefGrid = "g:"

// Алфавиты сетки. Ё стоит на своём месте (после Е), как в русском алфавите;
// GroupByLetter выделяет её в отдельную группу.
const (
	gridLatin    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	gridCyrillic = "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ"
)

// gridCell — ячейка сетки букв.
type gridCell struct {
	letter string
	active bool // в списке есть группа с этой буквой
	rect   image.Rectangle
}

// startAlphabet составляет буквы сетки: «#», латиница, кириллица (если в
// списке есть кириллические группы), затем остальные буквы источника в порядке
// их показа.
func startAlphabet(present []string) []string {
	out := []string{"#"}
	seen := map[string]bool{"#": true}
	add := func(s string) {
		for _, r := range s {
			if l := string(r); !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	add(gridLatin)
	for _, l := range present {
		if letterRank(l) == 2 {
			add(gridCyrillic)
			break
		}
	}
	for _, l := range present {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// ─── Состояние ───────────────────────────────────────────────────────────────

// LetterGridOpen сообщает, открыта ли сетка перехода по буквам.
func (m *StartMenu) LetterGridOpen() bool { return m.gridOpen() }

func (m *StartMenu) gridOpen() bool {
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	return m.v.grid
}

// holding — мышь «зажата» на чём-то, что ведут за курсором: плитка или бегунок.
func (m *StartMenu) holding() bool {
	m.v.mu.Lock()
	defer m.v.mu.Unlock()
	return m.v.bar.active || m.v.drag.pending || m.v.drag.active
}

// OpenLetterGrid открывает сетку букв над списком приложений. Сетка выходит из
// группы, видимой наверху списка. false — открыть нечего: меню закрыто, идёт
// поиск или в списке нет групп с буквами.
func (m *StartMenu) OpenLetterGrid() bool {
	rows, _ := m.listRows()
	m.v.mu.Lock()
	scroll := m.v.listScroll
	m.v.mu.Unlock()
	best := -1
	for i, r := range rows {
		if r.kind != rowLetter {
			continue
		}
		if best < 0 || r.y <= scroll {
			best = i
		}
	}
	if best < 0 {
		return false
	}
	return m.openLetterGridFrom(rows[best].key, rows[best].label)
}

// openLetterGridFrom открывает сетку с заголовка буквы letter (строка списка с
// ключом rowKey): на ней стоит выбор, к ней вернётся выбор по Esc.
func (m *StartMenu) openLetterGridFrom(rowKey, letter string) bool {
	if !m.IsOpen() || !m.modern() || m.Query() != "" {
		return false
	}
	v := m.v
	v.mu.Lock()
	if v.grid {
		v.mu.Unlock()
		return true
	}
	v.grid = true
	v.gridFrom = prefRow + rowKey
	v.sel[areaList] = prefGrid + letter
	v.mu.Unlock()
	v.gridFade.Set(0)
	v.gridFade.To(1)
	m.gridChanged()
	return true
}

// CloseLetterGrid закрывает сетку букв, оставляя список там, где он был. Выбор
// возвращается на заголовок, с которого сетку открыли.
func (m *StartMenu) CloseLetterGrid() {
	v := m.v
	v.mu.Lock()
	if !v.grid {
		v.mu.Unlock()
		return
	}
	v.grid = false
	if strings.HasPrefix(v.sel[areaList], prefGrid) {
		v.sel[areaList] = v.gridFrom
	}
	v.gridFrom = ""
	v.mu.Unlock()
	v.gridFade.Set(0)
	m.gridChanged()
}

// JumpToLetter прокручивает список к группе буквы letter (её заголовок встаёт
// наверх, насколько позволяет конец списка), ставит на заголовок выбор и
// закрывает сетку. false — в списке нет такой группы.
func (m *StartMenu) JumpToLetter(letter string) bool {
	if !m.IsOpen() || !m.modern() {
		return false
	}
	inner := m.contentRect()
	if inner.Empty() {
		return false
	}
	rows, contentH := m.listRows()
	at := -1
	for i, r := range rows {
		if r.kind == rowLetter && r.label == letter {
			at = i
			break
		}
	}
	if at < 0 {
		return false
	}
	g := m.startGeometry(inner)
	viewH := m.listViewport(g).Dy() // до замка: listViewport берёт его сам
	v := m.v
	v.mu.Lock()
	v.listScroll = clampScroll(rows[at].y, contentH, viewH)
	v.grid, v.gridFrom = false, ""
	v.sel[areaList] = prefRow + rows[at].key
	v.mu.Unlock()
	v.gridFade.Set(0)
	v.listBar.poke(m.invalListBar)
	widget.InvalidateRect(g.list)
	m.refreshHover()
	return true
}

// gridChanged перерисовывает окно списка: появление сетки идёт по анимации, а
// открытие и закрытие меняют его содержимое.
func (m *StartMenu) gridChanged() {
	if !m.IsOpen() {
		return
	}
	inner := m.contentRect()
	if inner.Empty() {
		return
	}
	widget.InvalidateRect(m.listViewport(m.startGeometry(inner)))
}

// closeGridOnPress закрывает сетку нажатием вне её ячеек: на плитках, боковой
// панели, пустом месте списка. Нажатие на ячейку (и на неактивную тоже)
// сетку не закрывает.
func (m *StartMenu) closeGridOnPress(h startHit, pt image.Point) {
	if !m.gridOpen() {
		return
	}
	if h.area == areaList {
		if inner := m.contentRect(); !inner.Empty() {
			if _, ok := m.gridCellAt(m.listViewport(m.startGeometry(inner)), pt); ok {
				return
			}
		}
	}
	m.CloseLetterGrid()
}

// ─── Раскладка ───────────────────────────────────────────────────────────────

// gridCells раскладывает сетку букв в окне списка vp: ячейки квадратные, число
// столбцов — наименьшее от четырёх до восьми, при котором алфавит помещается
// по высоте; сетка стоит по центру с полем строки слева и справа.
func (m *StartMenu) gridCells(vp image.Rectangle) []gridCell {
	rows, _ := m.listRows()
	var present []string
	have := map[string]bool{}
	for _, r := range rows {
		if r.kind == rowLetter {
			present = append(present, r.label)
			have[r.label] = true
		}
	}
	letters := startAlphabet(present)

	pad := m.metricInt(KeyStartRowPad)
	area := image.Rect(vp.Min.X+pad, vp.Min.Y+pad/2, vp.Max.X-pad, vp.Max.Y)
	if area.Dx() < 8 || area.Dy() < 8 {
		return nil
	}
	n := len(letters)
	cols := 4
	for ; cols < 8; cols++ {
		side := area.Dx() / cols
		if ((n+cols-1)/cols)*side <= area.Dy() {
			break
		}
	}
	side := area.Dx() / cols
	x0 := area.Min.X + (area.Dx()-cols*side)/2
	cells := make([]gridCell, n)
	for i, l := range letters {
		x := x0 + (i%cols)*side
		y := area.Min.Y + (i/cols)*side
		cells[i] = gridCell{letter: l, active: have[l], rect: image.Rect(x, y, x+side, y+side)}
	}
	return cells
}

// gridCellAt возвращает ячейку под точкой.
func (m *StartMenu) gridCellAt(vp image.Rectangle, pt image.Point) (gridCell, bool) {
	for _, c := range m.gridCells(vp) {
		if pt.In(c.rect) {
			return c, true
		}
	}
	return gridCell{}, false
}

// gridCellByLetter возвращает ячейку буквы.
func (m *StartMenu) gridCellByLetter(vp image.Rectangle, letter string) (gridCell, bool) {
	for _, c := range m.gridCells(vp) {
		if c.letter == letter {
			return c, true
		}
	}
	return gridCell{}, false
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

// keyGrid: стрелки ходят по доступным буквам по геометрии сетки, Home и End — к
// первой и последней, Enter и Пробел переходят к выбранной букве.
func (m *StartMenu) keyGrid(e widget.KeyEvent) {
	inner := m.contentRect()
	if inner.Empty() {
		return
	}
	vp := m.listViewport(m.startGeometry(inner))
	var cells []gridCell
	for _, c := range m.gridCells(vp) {
		if c.active {
			cells = append(cells, c)
		}
	}
	if len(cells) == 0 {
		m.CloseLetterGrid()
		return
	}
	v := m.v
	v.mu.Lock()
	cur := v.sel[areaList]
	v.mu.Unlock()
	idx := -1
	for i, c := range cells {
		if prefGrid+c.letter == cur {
			idx = i
		}
	}
	switch e.Code {
	case widget.KeyEnter, widget.KeySpace:
		if idx >= 0 && !e.Repeat {
			m.JumpToLetter(cells[idx].letter)
		}
		return
	case widget.KeyHome:
		idx = 0
	case widget.KeyEnd:
		idx = len(cells) - 1
	case widget.KeyLeft, widget.KeyRight, widget.KeyUp, widget.KeyDown:
		if idx < 0 {
			idx = 0
		} else {
			geoms := make([]tileGeom, len(cells))
			for i, c := range cells {
				geoms[i] = tileGeom{rect: c.rect}
			}
			if n := nearestTile(geoms, idx, e.Code); n >= 0 {
				idx = n
			}
		}
	default:
		return
	}
	v.mu.Lock()
	v.sel[areaList] = prefGrid + cells[idx].letter
	v.mu.Unlock()
	m.invalidateKeys(cur, prefGrid+cells[idx].letter)
}

// ─── Отрисовка ───────────────────────────────────────────────────────────────

// drawLetterRow рисует заголовок буквы в списке: как строку, с наведением,
// нажатием и рамкой клавиатурного фокуса (стиль letter), буква по центру столбца
// значков cx.
func (m *StartMenu) drawLetterRow(ctx widget.DrawContext, rect image.Rectangle, row listRow, sn startSnap, cx int) {
	key := prefRow + row.key
	st := theme.StateNormal
	selected := sn.kbd && sn.area == areaList && sn.sel[areaList] == key
	switch {
	case sn.press == key:
		st = theme.StatePressed
	case sn.hover == key:
		st = theme.StateHover
	case selected:
		st = theme.StateActive
	}
	s := m.fade.Style(m.tm, key, rect, st, func(st theme.State) *theme.Style { return m.tpart("letter", st) })
	PaintStyle(ctx, rect, s)
	w := MeasureText(ctx, row.label, s)
	drawTextAt(ctx, rect, cx-w/2, row.label, s)
	if selected {
		PaintFocusRing(ctx, rect, m.tm, s)
	}
}

// drawLetterGrid рисует сетку букв в окне списка vp. Доступная буква — стилем
// letter (наведение, нажатие, выбор клавишами) на едва заметной подложке,
// недоступная — приглушённо и без реакции. Появление — прозрачностью по
// переходу gridFade.
func (m *StartMenu) drawLetterGrid(ctx widget.DrawContext, vp image.Rectangle, sn startSnap) {
	alpha := m.v.gridFade.Value()
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	a := uint8(alpha*255 + 0.5)
	fadeStyle := func(s *theme.Style) *theme.Style {
		if a == 255 {
			return s
		}
		c := *s
		c.Fill, c.Text, c.Border = withAlpha(s.Fill, a), withAlpha(s.Text, a), withAlpha(s.Border, a)
		return &c
	}
	tint := withAlpha(m.tpart("row", theme.StateHover).Fill, uint8(int(a)*3/5))
	clip := ctx.Clip()
	for _, c := range m.gridCells(vp) {
		rect := c.rect.Inset(1)
		if !rect.Overlaps(clip) {
			continue
		}
		key := prefGrid + c.letter
		var s *theme.Style
		selected := false
		if c.active {
			ctx.FillRectAlpha(rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy(), tint)
			st := theme.StateNormal
			selected = sn.kbd && sn.area == areaList && sn.sel[areaList] == key
			switch {
			case sn.press == key:
				st = theme.StatePressed
			case sn.hover == key:
				st = theme.StateHover
			case selected:
				st = theme.StateActive
			}
			s = fadeStyle(m.fade.Style(m.tm, key, rect, st, func(st theme.State) *theme.Style { return m.tpart("letter", st) }))
			PaintStyle(ctx, rect, s)
		} else {
			s = fadeStyle(m.tpart("letter", theme.StateDisabled))
		}
		w := MeasureText(ctx, c.letter, s)
		drawTextAt(ctx, rect, rect.Min.X+(rect.Dx()-w)/2, c.letter, s)
		if selected {
			PaintFocusRing(ctx, rect, m.tm, s)
		}
	}
}
