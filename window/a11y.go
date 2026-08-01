// a11y.go — платформенно-независимая часть моста доступности.
//
// Движок уже отдаёт семантику UI деревом (engine.AccessibilityTree →
// widget.AccessNode). Платформенным мостам (AT-SPI на Linux, UI Automation на
// Windows, NSAccessibility на macOS) нужно другое представление: ПЛОСКИЙ список
// узлов с числовыми идентификаторами, по которым клиент ходит вверх/вниз,
// спрашивает границы и попадание точкой. Этот файл строит такой снимок и
// содержит общую логику, не зависящую от платформы (поиск фокуса, hit-test,
// диффы для событий). Сам транспорт — в a11y_linux.go / a11y_windows.go.
package window

import (
	"image"
	"strconv"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// a11yRootID — идентификатор корня снимка (окно приложения).
const a11yRootID int32 = 0

// a11yRefreshEvery — как часто мост пересобирает снимок семантики при
// активности UI. Реже, чем кадры: бурная анимация не должна топить клиента
// доступности в событиях.
const a11yRefreshEvery = 150 * time.Millisecond

// a11yNode — узел плоского снимка.
type a11yNode struct {
	Info     widget.AccessInfo
	Parent   int32 // -1 у корня
	Children []int32
	Index    int32 // порядковый номер среди детей родителя
}

// a11ySnapshot — плоский снимок семантического дерева.
type a11ySnapshot struct {
	Nodes []a11yNode
	Focus int32 // id узла с фокусом, -1 — фокуса нет
}

// a11yFlatten разворачивает дерево семантики в плоский снимок. Порядок обхода —
// в глубину: id ребёнка всегда больше id родителя, поэтому снимок устойчив,
// пока не меняется СТРУКТУРА дерева (клиенты доступности перезапрашивают узлы
// по id, и стабильность важнее компактности).
func a11yFlatten(root *widget.AccessNode) *a11ySnapshot {
	s := &a11ySnapshot{Focus: -1}
	if root == nil {
		return s
	}
	var walk func(n *widget.AccessNode, parent, index int32) int32
	walk = func(n *widget.AccessNode, parent, index int32) int32 {
		id := int32(len(s.Nodes))
		s.Nodes = append(s.Nodes, a11yNode{Info: n.AccessInfo, Parent: parent, Index: index})
		if a11yHasState(n.States, widget.StateFocused) {
			s.Focus = id
		}
		for i, c := range n.Children {
			cid := walk(c, id, int32(i))
			s.Nodes[id].Children = append(s.Nodes[id].Children, cid)
		}
		return id
	}
	walk(root, -1, 0)
	return s
}

// a11yHasState — есть ли состояние в списке.
func a11yHasState(states []string, want string) bool {
	for _, s := range states {
		if s == want {
			return true
		}
	}
	return false
}

// node возвращает узел по id (nil при выходе за границы).
func (s *a11ySnapshot) node(id int32) *a11yNode {
	if s == nil || id < 0 || int(id) >= len(s.Nodes) {
		return nil
	}
	return &s.Nodes[id]
}

// valid проверяет, что id указывает на существующий узел.
func (s *a11ySnapshot) valid(id int32) bool { return s.node(id) != nil }

// hitTest возвращает id самого глубокого узла, содержащего точку (логические
// координаты окна). Среди перекрывающихся детей выигрывает ПОСЛЕДНИЙ — он
// нарисован поверх. -1, если точка вне корня.
func (s *a11ySnapshot) hitTest(x, y int) int32 {
	if s == nil || len(s.Nodes) == 0 {
		return -1
	}
	p := image.Pt(x, y)
	if !p.In(s.Nodes[a11yRootID].Info.Bounds) {
		return -1
	}
	cur := a11yRootID
	for {
		next := int32(-1)
		for _, cid := range s.Nodes[cur].Children {
			if p.In(s.Nodes[cid].Info.Bounds) {
				next = cid // без break: поверх лежит последний подходящий
			}
		}
		if next < 0 {
			return cur
		}
		cur = next
	}
}

// a11yChanges — что изменилось между снимками (для событий доступности).
type a11yChanges struct {
	FocusLost    int32   // узел, потерявший фокус (-1 — нечего сообщать)
	FocusGained  int32   // узел, получивший фокус (-1 — нечего сообщать)
	NameChanged  []int32 // узлы с изменившимся именем
	ValueChanged []int32 // узлы с изменившимся значением
	StateChanged []int32 // узлы с изменившимся набором состояний
	Structural   bool    // изменилась структура дерева — нужен полный перезапрос
}

// a11yDiff сравнивает старый и новый снимок. При изменении структуры (другое
// число узлов, другая роль/родитель у узла) выставляется Structural и точечные
// списки не заполняются — клиент всё равно перечитывает дерево.
func a11yDiff(old, cur *a11ySnapshot) a11yChanges {
	ch := a11yChanges{FocusLost: -1, FocusGained: -1}
	if old == nil || cur == nil {
		ch.Structural = old != cur
		if cur != nil && cur.Focus >= 0 {
			ch.FocusGained = cur.Focus
		}
		return ch
	}
	if old.Focus != cur.Focus {
		ch.FocusLost, ch.FocusGained = old.Focus, cur.Focus
		if !cur.valid(ch.FocusLost) {
			ch.FocusLost = -1
		}
	}
	if len(old.Nodes) != len(cur.Nodes) {
		ch.Structural = true
		return ch
	}
	for i := range cur.Nodes {
		a, b := &old.Nodes[i], &cur.Nodes[i]
		if a.Info.Role != b.Info.Role || a.Parent != b.Parent || len(a.Children) != len(b.Children) {
			ch.Structural = true
			return ch
		}
		if a.Info.Name != b.Info.Name {
			ch.NameChanged = append(ch.NameChanged, int32(i))
		}
		if a.Info.Value != b.Info.Value {
			ch.ValueChanged = append(ch.ValueChanged, int32(i))
		}
		if !a11ySameStates(a.Info.States, b.Info.States) {
			ch.StateChanged = append(ch.StateChanged, int32(i))
		}
	}
	return ch
}

// a11ySameStates сравнивает наборы состояний (порядок сохраняется сборщиком
// дерева, поэтому достаточно поэлементного сравнения).
func a11ySameStates(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ─── Устойчивые идентификаторы ───────────────────────────────────────────────

// a11yView — снимок вместе с УСТОЙЧИВЫМИ идентификаторами объектов.
//
// Индексы в снимке — позиции в обходе дерева: после перестройки UI один и тот
// же индекс может достаться другому виджету. Клиенты доступности (libatspi на
// Linux, UI Automation на Windows) КЭШИРУЮТ элементы, поэтому такое
// переиспользование показало бы скринридеру чужие имя и роль. Отсюда отдельный
// слой: каждому узлу выдаётся id по структурному ключу (путь индексов + роль),
// живущий столько же, сколько сам элемент.
type a11yView struct {
	Snap  *a11ySnapshot
	IDs   []int32         // индекс снимка → устойчивый id
	Index map[int32]int32 // устойчивый id → индекс снимка
}

// node возвращает узел по устойчивому id.
func (v *a11yView) node(id int32) *a11yNode {
	if v == nil {
		return nil
	}
	idx, ok := v.Index[id]
	if !ok {
		return nil
	}
	return v.Snap.node(idx)
}

// id переводит индекс снимка в устойчивый id (-1 — индекс вне снимка).
func (v *a11yView) id(idx int32) int32 {
	if v == nil || idx < 0 || int(idx) >= len(v.IDs) {
		return -1
	}
	return v.IDs[idx]
}

// a11yIDPool раздаёт устойчивые идентификаторы по структурным ключам.
type a11yIDPool struct {
	keys map[string]int32
	next int32
}

// nodeKeys строит структурные ключи узлов снимка: путь индексов от корня плюс
// роль. Обход в глубину гарантирует, что родитель обработан раньше ребёнка.
func a11yNodeKeys(s *a11ySnapshot) []string {
	keys := make([]string, len(s.Nodes))
	for i := range s.Nodes {
		n := &s.Nodes[i]
		if n.Parent < 0 {
			keys[i] = "w:" + string(n.Info.Role)
			continue
		}
		keys[i] = keys[n.Parent] + "/" + strconv.Itoa(int(n.Index)) + ":" + string(n.Info.Role)
	}
	return keys
}

// assign раздаёт узлам снимка устойчивые идентификаторы. Ключи запоминаются на
// всё время жизни пула: элемент, вернувшийся на прежнее место (переключение
// вкладок, повторное открытие панели), получает СВОЙ прежний id.
func (p *a11yIDPool) assign(s *a11ySnapshot) *a11yView {
	if p.keys == nil {
		p.keys = map[string]int32{}
	}
	v := &a11yView{Snap: s, IDs: make([]int32, len(s.Nodes)), Index: make(map[int32]int32, len(s.Nodes))}
	for i, key := range a11yNodeKeys(s) {
		id, ok := p.keys[key]
		if !ok {
			id = p.next
			p.next++
			p.keys[key] = id
		}
		v.IDs[i] = id
		v.Index[id] = int32(i)
	}
	return v
}

// ─── Подключение платформенного моста ────────────────────────────────────────

// a11yBridge — платформенный мост доступности. Реализации: AT-SPI (Linux).
type a11yBridge interface {
	// start поднимает мост (подключение к сервису доступности, регистрация).
	start() error
	// stop освобождает ресурсы.
	stop()
}

// newA11yBridge создаёт мост для текущей платформы. Задаётся в init()
// платформенного файла; nil — мостов нет (окно работает как раньше).
var newA11yBridge func(win *Window) a11yBridge

// SetAccessibilityEnabled принудительно включает (true) или выключает (false)
// платформенный мост доступности.
//
// По умолчанию (метод не вызывался): на Linux мост AT-SPI поднимается сам,
// когда система сообщает о включённой доступности (org.a11y.Status); на
// Windows мост UI Automation ВЫКЛЮЧЕН — он экспериментальный (см. шапку
// a11y_windows.go), и без него окно отдаёт клиентам штатный HWND-провайдер.
//
// Вызывать до Run(). Переменная окружения HEADLESS_GUI_A11Y=1/0 имеет
// наивысший приоритет (удобно для отладки со скринридером).
func (win *Window) SetAccessibilityEnabled(v bool) {
	win.a11yForce = &v
}

// startAccessibility поднимает мост доступности (вызывается из Run).
// Ошибки не фатальны: без моста приложение работает как прежде.
func (win *Window) startAccessibility() {
	if newA11yBridge == nil || win.a11y != nil {
		return
	}
	b := newA11yBridge(win)
	if b == nil {
		return
	}
	if err := b.start(); err != nil {
		return
	}
	win.a11y = b
}

// stopAccessibility снимает мост (вызывается после цикла событий).
func (win *Window) stopAccessibility() {
	if win.a11y != nil {
		win.a11y.stop()
		win.a11y = nil
	}
}

// accessibilityTreeProvider — опциональная возможность движка отдать
// семантическое дерево (реализует *engine.Engine). Через интерфейс, чтобы не
// расширять обязательный EngineAPI ради моста доступности.
type accessibilityTreeProvider interface {
	AccessibilityTree() *widget.AccessNode
}

// accessibilitySnapshot строит текущий снимок семантики окна.
func (win *Window) accessibilitySnapshot() *a11ySnapshot {
	p, ok := win.eng.(accessibilityTreeProvider)
	if !ok {
		return &a11ySnapshot{Focus: -1}
	}
	return a11yFlatten(p.AccessibilityTree())
}
