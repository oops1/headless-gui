// startmenu.go — меню «Пуск»: список приложений каталога, всплывающий над
// панелью задач общей основой Flyout (см. flyout.go).
//
// Список не прокручивается: если приложений больше, чем влезает по высоте
// экрана, меню показывает столько строк, сколько поместилось, и не рисует
// остальные. Это предсказуемая деградация — обрезанный список честнее
// скролла, реализовать который здесь было бы негде (Flyout не знает о
// колесе мыши), и куда безопаснее меню, вылезающего за экран.
package desktop

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentStartMenu — имя компонента для стилей темы. Части: "" — строка
// приложения, "section" — заголовок раздела («Закреплено», «Все
// приложения»).
const ComponentStartMenu = "startmenu"

// Ключи метрик темы, которыми управляется размер меню.
const (
	// KeyStartMenuWidth — ширина меню.
	KeyStartMenuWidth theme.Key = "startmenu.width"
	// KeyStartMenuRowHeight — высота одной строки (и приложения, и
	// заголовка раздела — список из строк разной высоты труднее
	// прокликивать и рассчитывать).
	KeyStartMenuRowHeight theme.Key = "startmenu.row.height"
	// KeyStartMenuIconSize — сторона квадратного значка приложения.
	KeyStartMenuIconSize theme.Key = "startmenu.icon.size"
)

// Подписи разделов. Не размер и не цвет — обычный текст интерфейса.
const (
	startMenuLabelPinned = "Закреплено"
	startMenuLabelAllApps = "Все приложения"
)

// startMenuRowKind различает строку-заголовок раздела от строки приложения:
// у заголовка нет AppID и по нему нельзя ни кликнуть, ни перейти стрелками.
type startMenuRowKind int

const (
	startMenuRowSection startMenuRowKind = iota
	startMenuRowApp
)

// startMenuRow — одна строка списка меню, ещё не привязанная к экранным
// координатам (это делает layoutRows).
type startMenuRow struct {
	kind  startMenuRowKind
	label string
	id    AppID
	icon  image.Image
}

// startMenuLaidRow — строка вместе с её прямоугольником в АБСОЛЮТНЫХ
// координатах текущего кадра. И отрисовка, и попадание мыши, и клавиатурная
// навигация используют один и тот же расчёт (layoutRows), чтобы подсветка,
// клик и Enter никогда не расходились друг с другом.
type startMenuLaidRow struct {
	rect image.Rectangle
	row  startMenuRow
}

// StartMenu — меню «Пуск»: список приложений каталога cat, оформляемый
// темой tm. Встраивает Flyout — открытие/закрытие/подложку/закрытие по Esc
// и клику мимо целиком делает он.
type StartMenu struct {
	*Flyout

	cat AppCatalog

	mu         sync.Mutex
	hoveredID  AppID
	hasHover   bool
	selectedID AppID
	hasSel     bool
}

// NewStartMenu создаёt меню «Пуск» каталога cat, оформляемое темой tm.
func NewStartMenu(tm *theme.Manager, cat AppCatalog) *StartMenu {
	m := &StartMenu{
		Flyout: NewFlyout(tm, ComponentStartMenu),
		cat:    cat,
	}
	m.Content = m.drawContent
	m.Size = m.size
	return m
}

// Open открывает меню и сбрасывает клавиатурное выделение: старое выделение
// от прошлого открытия (в другом месте списка, если каталог изменился) не
// должно пережить закрытие — иначе первая же стрелка после переоткрытия
// прыгнула бы туда, где пользователь её не ждёт.
func (m *StartMenu) Open(anchor image.Rectangle) {
	m.mu.Lock()
	m.hasSel = false
	m.mu.Unlock()
	m.Flyout.Open(anchor)
}

// Bounds расширяет обычные границы виджета до прямоугольника открытого
// меню — так движок находит его при поиске оверлея под курсором и не
// теряет клики по пунктам списка (тот же приём, что widget.Dropdown.Bounds
// и widget.MenuBar.Bounds: оба расширяют границы ровно так же, пока их
// оверлей открыт).
func (m *StartMenu) Bounds() image.Rectangle {
	base := m.Flyout.Bounds()
	if !m.IsOpen() {
		return base
	}
	return base.Union(m.rect())
}

// Dismiss закрывает меню при клике в любом другом месте интерфейса.
// Реализует widget.Dismissable — так закрывают себя все выпадающие панели
// движка (widget.Dropdown, widget.PopupMenu, widget.MenuBar): движок вызывает
// Dismiss у каждого виджета вне пути клика, независимо от того, что там
// внутри и какой у него Bounds.
func (m *StartMenu) Dismiss() {
	m.Close()
}

// Draw — у меню нет собственной раскладки в потоке виджетов: содержимое
// целиком рисуется оверлеем (Flyout.DrawOverlay, унаследованный от
// встроенной панели). Метод обязателен по интерфейсу widget.Widget — тем же
// путём идёт widget.PopupMenu.Draw.
func (m *StartMenu) Draw(widget.DrawContext) {}

// OnMouseMove подсвечивает строку приложения под курсором.
func (m *StartMenu) OnMouseMove(x, y int) {
	if !m.IsOpen() {
		return
	}
	pt := image.Pt(x, y)
	var id AppID
	hit := false
	for _, lr := range m.layoutRows() {
		if lr.row.kind == startMenuRowApp && pt.In(lr.rect) {
			id, hit = lr.row.id, true
			break
		}
	}
	m.mu.Lock()
	changed := m.hoveredID != id || m.hasHover != hit
	m.hoveredID, m.hasHover = id, hit
	m.mu.Unlock()
	if changed {
		m.Invalidate()
	}
}

// OnMouseButton запускает приложение, по строке которого кликнули, и
// закрывает меню. Клик мимо строки (по заголовку раздела, по пустому месту
// или вовсе мимо меню) отдаётся встроенной панели — она либо ничего не
// делает (клик внутри), либо закрывает меню (клик снаружи).
func (m *StartMenu) OnMouseButton(e widget.MouseEvent) bool {
	if m.IsOpen() && e.Button == widget.MouseLeft && e.Pressed {
		pt := image.Pt(e.X, e.Y)
		for _, lr := range m.layoutRows() {
			if lr.row.kind == startMenuRowApp && pt.In(lr.rect) {
				m.launch(lr.row.id)
				return true
			}
		}
	}
	return m.Flyout.OnMouseButton(e)
}

// OnKeyEvent двигает выделение стрелками и запускает выделенное приложение
// по Enter. Всё остальное (в первую очередь Esc) отдаётся встроенной
// панели — она уже умеет закрываться сама.
func (m *StartMenu) OnKeyEvent(e widget.KeyEvent) {
	if m.IsOpen() && e.Pressed {
		switch e.Code {
		case widget.KeyDown:
			m.moveSelection(1)
			return
		case widget.KeyUp:
			m.moveSelection(-1)
			return
		case widget.KeyEnter:
			m.launchSelected()
			return
		}
	}
	m.Flyout.OnKeyEvent(e)
}

// moveSelection переносит клавиатурное выделение на delta строк приложений
// (заголовки разделов при обходе пропускаются — на них нечего запускать).
// Если выделения ещё не было, стрелка вниз ставит его на первое приложение,
// стрелка вверх — на последнее: так первое же нажатие уже что-то выделяет,
// а не требует лишнего повтора.
func (m *StartMenu) moveSelection(delta int) {
	rows := m.layoutRows()
	var appIdx []int
	for i, lr := range rows {
		if lr.row.kind == startMenuRowApp {
			appIdx = append(appIdx, i)
		}
	}
	if len(appIdx) == 0 {
		return
	}

	cur := m.selectedRowIndex(rows)
	pos := -1
	for i, ri := range appIdx {
		if ri == cur {
			pos = i
			break
		}
	}
	if pos < 0 {
		if delta > 0 {
			pos = 0
		} else {
			pos = len(appIdx) - 1
		}
	} else {
		pos += delta
		if pos < 0 {
			pos = 0
		}
		if pos >= len(appIdx) {
			pos = len(appIdx) - 1
		}
	}

	m.mu.Lock()
	m.selectedID = rows[appIdx[pos]].row.id
	m.hasSel = true
	m.mu.Unlock()
	m.Invalidate()
}

// selectedRowIndex ищет текущее клавиатурное выделение в rows (-1, если
// выделения нет или выделенное приложение пропало из каталога).
func (m *StartMenu) selectedRowIndex(rows []startMenuLaidRow) int {
	m.mu.Lock()
	id, has := m.selectedID, m.hasSel
	m.mu.Unlock()
	if !has {
		return -1
	}
	for i, lr := range rows {
		if lr.row.kind == startMenuRowApp && lr.row.id == id {
			return i
		}
	}
	return -1
}

// launchSelected запускает приложение под клавиатурным выделением (Enter
// без выделения — не событие: нечего запускать).
func (m *StartMenu) launchSelected() {
	rows := m.layoutRows()
	idx := m.selectedRowIndex(rows)
	if idx < 0 {
		return
	}
	m.launch(rows[idx].row.id)
}

// launch запускает приложение через каталог и закрывает меню — успешный
// запуск, как и в настоящем Windows, убирает меню с экрана; ошибку запуска
// (несуществующее приложение, отказ каталога) меню само не показывает —
// каталог решает, как её сообщить пользователю.
func (m *StartMenu) launch(id AppID) {
	if m.cat != nil {
		_ = m.cat.Launch(id)
	}
	m.Close()
}

// buildRows собирает плоский список строк: (если есть закреплённые) —
// заголовок «Закреплено» и закреплённые приложения, затем заголовок «Все
// приложения» и полный каталог. Закреплённые приложения не убираются из
// общего списка — как и в Windows 11, «Все приложения» показывает всё, вне
// зависимости от закрепления.
func (m *StartMenu) buildRows() []startMenuRow {
	if m.cat == nil {
		return nil
	}
	apps := m.cat.Apps()
	byID := make(map[AppID]AppInfo, len(apps))
	for _, a := range apps {
		byID[a.ID] = a
	}

	var rows []startMenuRow
	if pinned := m.cat.Pinned(); len(pinned) > 0 {
		rows = append(rows, startMenuRow{kind: startMenuRowSection, label: startMenuLabelPinned})
		for _, id := range pinned {
			info, ok := byID[id]
			if !ok {
				continue // закреплено, но уже удалено из каталога — строки не строим
			}
			rows = append(rows, startMenuRow{kind: startMenuRowApp, label: info.Title, id: info.ID, icon: info.Icon})
		}
	}

	rows = append(rows, startMenuRow{kind: startMenuRowSection, label: startMenuLabelAllApps})
	for _, a := range apps {
		rows = append(rows, startMenuRow{kind: startMenuRowApp, label: a.Title, id: a.ID, icon: a.Icon})
	}
	return rows
}

// layoutRows раскладывает buildRows по видимым строкам меню: столько,
// сколько действительно поместится в contentRect (та же арифметика, что и
// в size — иначе подсказанная Size высота и фактически нарисованные строки
// разойдутся).
func (m *StartMenu) layoutRows() []startMenuLaidRow {
	rowH := m.metric(KeyStartMenuRowHeight)
	r := m.contentRect()
	if rowH <= 0 || r.Empty() {
		return nil
	}
	rows := m.buildRows()
	out := make([]startMenuLaidRow, 0, len(rows))
	y := r.Min.Y
	for _, row := range rows {
		if y+rowH > r.Max.Y {
			break // не влезло — предсказуемая деградация, дальше не рисуем
		}
		out = append(out, startMenuLaidRow{rect: image.Rect(r.Min.X, y, r.Max.X, y+rowH), row: row})
		y += rowH
	}
	return out
}

// contentRect — прямоугольник, который получит Content при отрисовке:
// панель минус отступ PadX (Flyout.DrawOverlay инсетит содержимое ровно на
// него — image.Rectangle.Inset одним числом ужимает сразу оба измерения).
// Используется и Draw'ом, и попаданием мыши/клавиатурой, чтобы координаты
// строки в обработчике совпадали с тем, что фактически нарисовано.
func (m *StartMenu) contentRect() image.Rectangle {
	if !m.IsOpen() {
		return image.Rectangle{}
	}
	r := m.rect()
	if r.Empty() {
		return r
	}
	return r.Inset(int(m.style(theme.StateNormal).PadX))
}

// size — Flyout.Size: желаемая высота меню ограничена числом строк,
// умещающихся в высоту экрана (m.Screen из встроенного Flyout), если экран
// задан. Не задан — меню показывает список целиком, границам его ужать
// нечем.
func (m *StartMenu) size() image.Point {
	width := m.metric(KeyStartMenuWidth)
	rowH := m.metric(KeyStartMenuRowHeight)
	if width <= 0 || rowH <= 0 {
		return image.Point{} // без метрик темы рисовать нечего — Flyout не откроется
	}
	pad := int(m.style(theme.StateNormal).PadX)

	total := len(m.buildRows())
	visible := total
	if !m.Screen.Empty() {
		avail := m.Screen.Dy() - 2*m.Margin - 2*pad
		max := avail / rowH
		if max < 0 {
			max = 0
		}
		if max < visible {
			visible = max
		}
	}
	return image.Pt(width, visible*rowH+2*pad)
}

// drawContent — Flyout.Content: рисует видимые строки меню.
func (m *StartMenu) drawContent(ctx widget.DrawContext, _ image.Rectangle) {
	m.mu.Lock()
	hoveredID, hasHover := m.hoveredID, m.hasHover
	selectedID, hasSel := m.selectedID, m.hasSel
	m.mu.Unlock()

	iconSize := m.metric(KeyStartMenuIconSize)
	for _, lr := range m.layoutRows() {
		if lr.row.kind == startMenuRowSection {
			s := m.partStyle("section", theme.StateNormal)
			DrawTextLeftElided(ctx, lr.rect, lr.row.label, s)
			continue
		}

		highlighted := (hasHover && hoveredID == lr.row.id) || (hasSel && selectedID == lr.row.id)
		st := theme.StateNormal
		if highlighted {
			st = theme.StateHover
		}
		s := m.partStyle("", st)
		PaintStyle(ctx, lr.rect, s)

		textLeft := lr.rect.Min.X
		if lr.row.icon != nil && iconSize > 0 {
			iconX := lr.rect.Min.X + int(s.PadX)
			iconY := lr.rect.Min.Y + (lr.rect.Dy()-iconSize)/2
			ctx.DrawImageScaled(lr.row.icon, iconX, iconY, iconSize, iconSize)
			textLeft = iconX + iconSize + int(s.PadX)
		}
		textRect := image.Rect(textLeft, lr.rect.Min.Y, lr.rect.Max.X, lr.rect.Max.Y)
		DrawTextLeftElided(ctx, textRect, lr.row.label, s)
	}
}

// partStyle читает стиль части part меню из активной темы (пустой стиль,
// если темы нет).
func (m *StartMenu) partStyle(part string, st theme.State) *theme.Style {
	tm := m.Theme()
	if tm == nil {
		return &theme.Style{}
	}
	return tm.GetStyle(ComponentStartMenu, part, st)
}
