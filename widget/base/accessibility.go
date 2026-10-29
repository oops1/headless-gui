// accessibility.go — семантическое дерево UI (фундамент accessibility).
//
// Каждому виджету сопоставляется AccessInfo: роль, имя (видимая подпись),
// значение и состояния. BuildAccessTree обходит дерево виджетов и строит
// сериализуемый снапшот — источник семантики для:
//
//   - скринридеров через платформенные мосты (UI Automation / AT-SPI /
//     NSAccessibility — следующие итерации);
//   - headless-потребителей: в стриминговых сценариях (RDP/WebSocket)
//     семантика передаётся side-channel'ом рядом с пиксельными тайлами
//     и озвучивается на стороне клиента — возможность, недоступная
//     классическим GUI-тулкитам;
//   - автоматизированного тестирования UI (поиск элементов по роли/имени).
//
// Роли встроенных виджетов выводятся центральным type-switch'ем —
// виджетам не нужен собственный код. Кастомные виджеты могут
// реализовать интерфейс Accessible и переопределить семантику.
package base

import (
	"fmt"
	"image"
	"strconv"
)

// AccessRole — роль элемента в терминах accessibility (подмножество,
// достаточное для маппинга в UIA ControlType / AT-SPI role).
type AccessRole string

const (
	RoleWindow      AccessRole = "window"
	RolePanel       AccessRole = "panel"
	RoleGroup       AccessRole = "group"
	RoleButton      AccessRole = "button"
	RoleCheckBox    AccessRole = "checkbox"
	RoleRadioButton AccessRole = "radiobutton"
	RoleSwitch      AccessRole = "switch"
	RoleSlider      AccessRole = "slider"
	RoleProgressBar AccessRole = "progressbar"
	RoleTextInput   AccessRole = "textinput"
	// RoleDocument — многострочный текст (редактор, просмотр документа).
	// Отдельно от RoleTextInput: скринридер читает его иначе — по строкам и
	// абзацам, а не одной подписью, — и объявляет человеку документом, а не
	// полем ввода.
	RoleDocument   AccessRole = "document"
	RoleLabel      AccessRole = "label"
	RoleComboBox   AccessRole = "combobox"
	RoleList       AccessRole = "list"
	RoleTabControl AccessRole = "tablist"
	RoleMenuBar    AccessRole = "menubar"
	RoleSpinner    AccessRole = "spinner"
	RoleImage      AccessRole = "image"
	// RoleDialog — диалог (в том числе MessageBox): окно второго уровня со
	// своим заголовком. Имя — заголовок, описание — текст сообщения.
	RoleDialog AccessRole = "dialog"
	// RoleMenu — меню (всплывающее или раскрытое из полосы меню); дети —
	// RoleMenuItem.
	RoleMenu AccessRole = "menu"
	// RoleMenuItem — пункт меню.
	RoleMenuItem AccessRole = "menuitem"
	RoleUnknown  AccessRole = "unknown"
)

// Состояния элемента (строковые — для простой JSON-сериализации).
const (
	StateDisabled = "disabled"
	StateFocused  = "focused"
	StateChecked  = "checked"
	StateSelected = "selected"
	StateExpanded = "expanded"
	StateModal    = "modal"
	StateInactive = "inactive" // окно без фокуса ОС
	StatePassword = "password" // значение скрыто
	StateReadOnly = "readonly" // значение видно, но не правится

	// StateOffscreen — элемент целиком вне видимой области: прокручен за край
	// контейнера или закрыт (меню, которое не показано). Bounds при этом
	// остаются границами элемента в его кадре, а видимой части нет.
	StateOffscreen = "offscreen"
	// StateCheckable — пункт умеет быть отмеченным (флажок в меню): отметка
	// есть и в снятом состоянии, отличить такой пункт от обычного можно только
	// по этому признаку.
	StateCheckable = "checkable"
	// StateHasPopup — у пункта есть подменю.
	StateHasPopup = "haspopup"
	// StatePopup — элемент показан оверлеем поверх всего UI (открытое меню и
	// его пункты). Такой элемент выходит за границы своего владельца, и поиск
	// по точке обязан смотреть на него раньше, чем на то, что лежит под ним.
	StatePopup = "popup"
)

// AccessInfo — семантическое описание одного элемента.
type AccessInfo struct {
	Role        AccessRole      `json:"role"`
	Name        string          `json:"name,omitempty"`        // видимая подпись
	Value       string          `json:"value,omitempty"`       // текущее значение
	Description string          `json:"description,omitempty"` // ToolTip
	States      []string        `json:"states,omitempty"`
	Bounds      image.Rectangle `json:"bounds"` // логические пиксели

	// ID — устойчивый идентификатор УЗЛА внутри владельца: им владелец узлов
	// (AccessChildrenProvider) опознаёт, КОМУ адресовано действие скринридера
	// (см. AccessActionHandler). Осмыслен только у узлов из AccessChildren;
	// пустой ID строитель дерева заполняет номером узла в списке. У узлов,
	// которым соответствует настоящий виджет, не используется.
	ID string `json:"id,omitempty"`

	// Visible — видимая часть Bounds, когда элемент выступает за область
	// прокрутки: пересечение границ с окном прокрутки. Пусто — элемент виден
	// целиком (или ещё не обрезан). Целиком невидимый элемент помечается
	// StateOffscreen, а не пустым Visible: нулевое значение должно означать
	// «не обрезан». В JSON не входит.
	Visible image.Rectangle `json:"-"`
}

// AccessNode — узел семантического дерева.
type AccessNode struct {
	AccessInfo
	Children []*AccessNode `json:"children,omitempty"`

	// Widget — виджет, которому соответствует узел. Нужен платформенным
	// мостам, чтобы скринридер мог не только ЧИТАТЬ семантику, но и
	// ДЕЙСТВОВАТЬ: сфокусировать элемент (AT-SPI GrabFocus, UIA SetFocus) или
	// нажать его (AT-SPI DoAction, UIA Invoke). Из сериализации исключён —
	// снапшот остаётся чистым JSON-деревом для стриминговых сценариев,
	// где виджета на стороне клиента всё равно нет.
	Widget Widget `json:"-"`

	// ActionID — идентификатор узла у владельца (AccessInfo.ID), если узел
	// получен из AccessChildren; у узлов самих виджетов пуст. Платформенный
	// мост по нему решает, куда адресовать действие: узлу владельца
	// (AccessActionHandler) или самому виджету. В JSON не входит.
	ActionID string `json:"-"`
}

// Accessible — опциональный интерфейс: кастомный виджет сам описывает
// свою семантику (переопределяет вывод type-switch'а).
type Accessible interface {
	AccessInfo() AccessInfo
}

// AccessChildrenProvider — опциональный интерфейс: виджет, который сам рисует
// несколько логических элементов (две панели текста в DiffView), отдаёт их
// скринридеру дочерними узлами.
//
// Дерево строится по Children(), а у виджета, рисующего всё сам, детей-виджетов
// нет — и скринридер видел бы одну безымянную группу, не прочитав ни строчки.
// Узлы указывают на сам виджет-владелец (AccessNode.Widget). Чтобы действие
// скринридера (фокус, нажатие) достигло именно узла, владелец реализует ещё и
// AccessActionHandler; без него действие уходит владельцу целиком — нажатие
// синтетическим кликом в центр виджета.
type AccessChildrenProvider interface {
	AccessChildren() []AccessInfo
}

// AccessActionKind — действие, которое скринридер просит выполнить над
// узлом: UIA Invoke / SetFocus / SelectionItem.Select / ExpandCollapse,
// AT-SPI DoAction / GrabFocus.
type AccessActionKind int

const (
	// AccessInvoke — «нажать»: кнопка, пункт меню, радиокнопка (UIA Invoke,
	// AT-SPI Action click).
	AccessInvoke AccessActionKind = iota + 1
	// AccessFocus — перевести на узел фокус (UIA SetFocus, AT-SPI GrabFocus).
	AccessFocus
	// AccessSelect — выбрать узел (UIA SelectionItem.Select).
	AccessSelect
	// AccessToggle — переключить отметку узла (UIA Toggle).
	AccessToggle
	// AccessExpand и AccessCollapse — раскрыть и свернуть узел (UIA
	// ExpandCollapse, AT-SPI expand or contract).
	AccessExpand
	AccessCollapse
)

// String — имя действия для журналов и сообщений тестов.
func (k AccessActionKind) String() string {
	switch k {
	case AccessInvoke:
		return "invoke"
	case AccessFocus:
		return "focus"
	case AccessSelect:
		return "select"
	case AccessToggle:
		return "toggle"
	case AccessExpand:
		return "expand"
	case AccessCollapse:
		return "collapse"
	}
	return "unknown"
}

// AccessActionHandler — необязательный интерфейс владельца узлов
// (AccessChildrenProvider): доставляет действие скринридера КОНКРЕТНОМУ узлу.
//
// Без него Invoke и SetFocus на узле действовали бы на виджет-владельца: узел
// «правая панель сравнения» нажимался бы кликом в центр всего DiffView, а пункт
// меню — кликом в центр меню. id — AccessInfo.ID узла, action — что сделать.
//
// Вызывается на горутине движка (дерево виджетов принадлежит ей), поэтому
// обработчик может менять состояние виджета без дополнительной синхронизации с
// событиями ввода. Возвращает true, если действие выполнено; false — узел не
// принимает такое действие или его уже нет (дерево перестроилось между
// снимком и действием). Ответ окончательный: запасного клика в центр
// владельца при отказе нет — он попал бы не в тот узел.
//
// Для самого владельца (его узел в дереве, не из AccessChildren) интерфейс не
// зовётся: там прежнее поведение. Объявлять поддержку действий отдельным узлам
// не нужно: у владельца без этого интерфейса узлы просто остаются «читаемыми».
type AccessActionHandler interface {
	AccessAction(id string, action AccessActionKind) bool
}

// BuildAccessTree строит семантический снапшот дерева виджетов.
// focused — виджет с фокусом ввода (или nil); невидимые виджеты
// пропускаются вместе с поддеревьями.
//
// Bounds узлов — экранные: элемент внутри прокрутки описан там, где его видно, а
// не в координатах содержимого. Скринридер и тесты наводят курсор и кликают по
// этим границам; с координатами содержимого они промахивались бы на величину
// прокрутки (см. contentoffset.go).
func BuildAccessTree(root Widget, focused Widget) *AccessNode {
	return buildAccessTree(root, focused, image.Point{}, accessClip{})
}

// accessClip — область, за которую элемент не виден: окно прокрутки над ним
// (в экранных координатах). Пустое значение (set=false) — обрезки нет.
type accessClip struct {
	r   image.Rectangle
	set bool
}

// with сужает область окном контейнера screen (его границы на экране).
func (c accessClip) with(screen image.Rectangle) accessClip {
	if !c.set {
		return accessClip{r: screen, set: true}
	}
	return accessClip{r: c.r.Intersect(screen), set: true}
}

// applyAccessClip считает видимую часть элемента под обрезкой c: целиком
// вне области — StateOffscreen, частично — Visible = пересечение. Элемент без
// границ (закрытое меню) обрезке не подлежит. Уже заданные самим виджетом
// Visible и StateOffscreen уважаются: виджет, у которого есть своя область
// просмотра (длинное меню), знает о ней больше контейнера.
func applyAccessClip(info *AccessInfo, c accessClip) {
	if !c.set || info.Bounds.Empty() || accessHasState(info.States, StateOffscreen) {
		return
	}
	base := info.Bounds
	if !info.Visible.Empty() {
		base = info.Visible
	}
	vis := base.Intersect(c.r)
	if vis.Empty() {
		info.Visible = image.Rectangle{}
		info.States = append(append([]string(nil), info.States...), StateOffscreen)
		return
	}
	if vis != info.Bounds {
		info.Visible = vis
	}
}

// accessHasState — есть ли состояние в списке.
func accessHasState(states []string, want string) bool {
	for _, s := range states {
		if s == want {
			return true
		}
	}
	return false
}

// buildAccessTree — BuildAccessTree с накопленным смещением off: сумма
// ContentOffset контейнеров над root, то есть на сколько границы root лежат
// правее и ниже его места на экране, и областью обрезки clip: окнами
// прокрутки над root.
func buildAccessTree(root Widget, focused Widget, off image.Point, clip accessClip) *AccessNode {
	if root == nil || !IsWidgetVisible(root) {
		return nil
	}
	node := &AccessNode{AccessInfo: accessInfoFor(root), Widget: root}
	node.Bounds = node.Bounds.Sub(off)
	if !node.Visible.Empty() {
		node.Visible = node.Visible.Sub(off)
	}
	// Меню рисуется оверлеем поверх всего и окном прокрутки не обрезается.
	if node.Role == RoleMenu {
		clip = accessClip{}
	}
	applyAccessClip(&node.AccessInfo, clip)
	if root == focused {
		node.States = append(node.States, StateFocused)
	}
	if p, ok := root.(AccessChildrenProvider); ok {
		for i, info := range p.AccessChildren() {
			info.Bounds = info.Bounds.Sub(off)
			if !info.Visible.Empty() {
				info.Visible = info.Visible.Sub(off)
			}
			if info.ID == "" {
				info.ID = strconv.Itoa(i)
			}
			applyAccessClip(&info, clip)
			node.Children = append(node.Children, &AccessNode{AccessInfo: info, Widget: root, ActionID: info.ID})
		}
	}
	// Дети контейнера со сдвигом живут в кадре, сдвинутом ещё и на его смещение,
	// и видны только в его окне.
	childOff := off
	childClip := clip
	if oc, ok := root.(ContentOffsetter); ok {
		childOff = off.Add(oc.ContentOffset())
		childClip = clip.with(root.Bounds().Sub(off))
	}
	for _, child := range root.Children() {
		if cn := buildAccessTree(child, focused, childOff, childClip); cn != nil {
			node.Children = append(node.Children, cn)
		}
	}
	return node
}

// AccessDescriber — виджет вне ядра сам дописывает к общей семантике
// (границы, доступность, фокус, подсказка) роль, имя, значение и состояния —
// то, что для виджетов ядра делают ветки accessInfoFor. Полную семантику
// вместо общей задаёт Accessible.
type AccessDescriber interface {
	DescribeAccess(info *AccessInfo)
}

// accessInfoFor выводит семантику встроенного виджета.
func accessInfoFor(w Widget) AccessInfo {
	// Кастомная семантика имеет приоритет.
	if a, ok := w.(Accessible); ok {
		return a.AccessInfo()
	}

	info := AccessInfo{Role: RoleUnknown, Bounds: w.Bounds()}

	// Общие свойства Base (доступны через интерфейсы).
	if e, ok := w.(interface{ IsEnabled() bool }); ok && !e.IsEnabled() {
		info.States = append(info.States, StateDisabled)
	}
	if tt, ok := w.(interface{ GetToolTip() string }); ok {
		info.Description = tt.GetToolTip()
	}

	switch t := w.(type) {
	case *Button:
		info.Role = RoleButton
		info.Name = t.Text
	case *MenuButton:
		info.Role = RoleButton
		info.Name = t.Text
	case *CheckBox:
		info.Role = RoleCheckBox
		info.Name = t.Text
		if t.IsChecked() {
			info.States = append(info.States, StateChecked)
		}
	case *RadioButton:
		info.Role = RoleRadioButton
		info.Name = t.Text
		if t.IsSelected() {
			info.States = append(info.States, StateSelected)
		}
	case *ToggleSwitch:
		info.Role = RoleSwitch
		info.Name = t.Text
		if t.IsOn() {
			info.States = append(info.States, StateChecked)
		}
	case *Slider:
		info.Role = RoleSlider
		info.Value = fmt.Sprintf("%g", t.Value())
	case *ProgressBar:
		info.Role = RoleProgressBar
		info.Value = fmt.Sprintf("%g", t.Value())
	case *NumericUpDown:
		info.Role = RoleSpinner
		info.Value = fmt.Sprintf("%g", t.Value())
	case AccessDescriber:
		// Виджеты вне ядра: поля ввода (widget/text), окно (widget/chrome).
		t.DescribeAccess(&info)
	case *Label:
		info.Role = RoleLabel
		info.Name = t.Text()
	case *Dropdown:
		info.Role = RoleComboBox
		info.Value = t.SelectedText()
	case *ListView:
		info.Role = RoleList
		info.Value = t.SelectedText()
	case *TabControl:
		info.Role = RoleTabControl
		info.Value = t.TabHeader(t.Active())
	case *MenuBar:
		info.Role = RoleMenuBar
	case *GroupBox:
		info.Role = RoleGroup
		info.Name = t.Header
	case *Expander:
		info.Role = RoleGroup
		info.Name = t.Header
		if t.IsExpanded {
			info.States = append(info.States, StateExpanded)
		}
	case *ImageWidget:
		info.Role = RoleImage
	case *Panel, *Canvas, *Grid, *StackPanel, *DockPanel:
		info.Role = RolePanel
	}
	return info
}
