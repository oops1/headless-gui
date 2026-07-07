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
package widget

import (
	"fmt"
	"image"
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
	RoleLabel       AccessRole = "label"
	RoleComboBox    AccessRole = "combobox"
	RoleList        AccessRole = "list"
	RoleTabControl  AccessRole = "tablist"
	RoleMenuBar     AccessRole = "menubar"
	RoleSpinner     AccessRole = "spinner"
	RoleImage       AccessRole = "image"
	RoleUnknown     AccessRole = "unknown"
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
)

// AccessInfo — семантическое описание одного элемента.
type AccessInfo struct {
	Role        AccessRole      `json:"role"`
	Name        string          `json:"name,omitempty"`        // видимая подпись
	Value       string          `json:"value,omitempty"`       // текущее значение
	Description string          `json:"description,omitempty"` // ToolTip
	States      []string        `json:"states,omitempty"`
	Bounds      image.Rectangle `json:"bounds"` // логические пиксели
}

// AccessNode — узел семантического дерева.
type AccessNode struct {
	AccessInfo
	Children []*AccessNode `json:"children,omitempty"`
}

// Accessible — опциональный интерфейс: кастомный виджет сам описывает
// свою семантику (переопределяет вывод type-switch'а).
type Accessible interface {
	AccessInfo() AccessInfo
}

// BuildAccessTree строит семантический снапшот дерева виджетов.
// focused — виджет с фокусом ввода (или nil); невидимые виджеты
// пропускаются вместе с поддеревьями.
func BuildAccessTree(root Widget, focused Widget) *AccessNode {
	if root == nil || !IsWidgetVisible(root) {
		return nil
	}
	node := &AccessNode{AccessInfo: accessInfoFor(root)}
	if root == focused {
		node.States = append(node.States, StateFocused)
	}
	for _, child := range root.Children() {
		if cn := BuildAccessTree(child, focused); cn != nil {
			node.Children = append(node.Children, cn)
		}
	}
	return node
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
	case *Window:
		info.Role = RoleWindow
		info.Name = t.Title
		if !t.IsActive() {
			info.States = append(info.States, StateInactive)
		}
	case *Button:
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
	case *TextInput:
		info.Role = RoleTextInput
		info.Value = t.GetText()
	case *TextBox:
		info.Role = RoleTextInput
		info.Value = t.GetText()
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
