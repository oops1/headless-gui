// theme_bindings.go — таблица привязок 73 цветовых полей widget.Theme к
// адресам в модели токенов theme.Profile/theme.Theme.
//
// Ровно ОДНА таблица обслуживает оба направления моста (theme_bridge.go):
// ProfileFromTheme читает поля Theme по этим адресам и складывает в профиль,
// Materialize делает обратное. Если бы у каждого направления была своя
// таблица, они могли бы разойтись — одно поле уехало бы не туда, откуда его
// же читает другое, и round-trip (тема → профиль → тема) перестал бы быть
// тождеством. Тест на round-trip по всем шести пресетам держит именно этот
// инвариант, и держит его только потому, что источник для обеих сторон один.
//
// Правила, по которым читается таблица:
//
//   - большинство полей адресуют Style конкретного (Component, Part, State) —
//     они и есть основной случай, ради которого заведена модель токенов;
//   - несколько полей сквозные (Accent, Disabled, Border, ...) — один
//     семантический выбор темы, который сегодня читают от трёх до полутора
//     десятков разных виджетов напрямую. Разложить их по компонентам значило
//     бы продублировать одно значение много раз и потерять смысл «акцент у
//     темы один» — такие уходят плоским ключом (поле key), а не в Styles;
//   - несколько полей — вторые точки двухцветного градиента (TitleBG2,
//     ProgressGlow*) или обозначают состояние КОНТЕЙНЕРА, а не самого
//     виджета (Title*Inactive — окно активно/неактивно в ОС, а не в фокусе
//     клавиатуры). У Style нет отдельного поля под «второй цвет градиента»
//     (Style.Fill — одна точка, Gradient — это отдельная сущность, которую
//     мост пока не собирает по крупицам из плоских полей), поэтому такие
//     тоже уходят плоским ключом.
package widget

import (
	"image/color"

	"github.com/oops1/headless-gui/v3/theme"
)

var legacyBindings = []legacyBinding{
	// ─── Окно (Window) и панели (Panel) ─────────────────────────────────────

	{name: "WindowBG", field: func(t *Theme) *color.RGBA { return &t.WindowBG },
		style: theme.StyleKey{Component: "window"}, role: roleFill},
	{name: "PanelBG", field: func(t *Theme) *color.RGBA { return &t.PanelBG },
		style: theme.StyleKey{Component: "panel"}, role: roleFill},

	// TitleBG/TitleText описывают заголовок ОКНА, ПОЛУЧИВШЕГО ФОКУС В ОС —
	// то есть ту же ось, что и StateFocused у обычных виджетов, только для
	// контейнера, а не для самого заголовка. TitleBGInactive/TitleTextInactive —
	// зеркальная пара для окна БЕЗ фокуса ОС — естественно ложится в Normal
	// (это состояние покоя окна, когда оно не на переднем плане). Смысл
	// StateFocused здесь шире обычного: не «виджет держит клавиатурный
	// фокус», а «окно активно в ОС» — см. также §3.5 карты соответствия.
	{name: "TitleBG", field: func(t *Theme) *color.RGBA { return &t.TitleBG },
		style: theme.StyleKey{Component: "window", Part: "titlebar", State: theme.StateFocused}, role: roleFill},
	{name: "TitleText", field: func(t *Theme) *color.RGBA { return &t.TitleText },
		style: theme.StyleKey{Component: "window", Part: "titlebar", State: theme.StateFocused}, role: roleText},
	{name: "TitleBGInactive", field: func(t *Theme) *color.RGBA { return &t.TitleBGInactive },
		style: theme.StyleKey{Component: "window", Part: "titlebar"}, role: roleFill},
	{name: "TitleTextInactive", field: func(t *Theme) *color.RGBA { return &t.TitleTextInactive },
		style: theme.StyleKey{Component: "window", Part: "titlebar"}, role: roleText},

	// TitleBG2/TitleBG2Inactive — вторая точка градиента заголовка. У Style
	// нет отдельного поля под «второй цвет» (Gradient — не сборка из плоских
	// полей), поэтому обе идут плоским ключом.
	{name: "TitleBG2", field: func(t *Theme) *color.RGBA { return &t.TitleBG2 },
		key: "window.titlebar.gradient2"},
	{name: "TitleBG2Inactive", field: func(t *Theme) *color.RGBA { return &t.TitleBG2Inactive },
		key: "window.titlebar.gradient2.inactive"},

	// Border используют ~15 не связанных друг с другом виджетов (Window,
	// Panel, ListView, Dialog, ScrollView, MenuBar и другие) с одним и тем же
	// значением — это ровно тот сквозной случай, под который в resolve.go уже
	// заведён запасной путь: defaultStyle() берёт цвет рамки из
	// t.ColorOr("border", ...) как встроенное значение по умолчанию для
	// любого необъявленного стиля. Ключ здесь выбран тем же именем "border"
	// не случайно — поле Theme.Border и есть источник этого умолчания.
	{name: "Border", field: func(t *Theme) *color.RGBA { return &t.Border },
		key: "border"},

	// ShadowColor одинаково используют Dialog и PopupMenu, но поле в Theme
	// одно — адресуем его туда, где у Style есть специально отведённое поле
	// (Style.Shadow), т.е. в стиль диалога; у PopupMenu своего цветового поля
	// в Theme нет, он в legacy-коде просто читает то же ShadowColor напрямую.
	{name: "ShadowColor", field: func(t *Theme) *color.RGBA { return &t.ShadowColor },
		style: theme.StyleKey{Component: "dialog"}, role: roleShadow},

	{name: "OutlineDragFill", field: func(t *Theme) *color.RGBA { return &t.OutlineDragFill },
		style: theme.StyleKey{Component: "window", Part: "dragoutline"}, role: roleFill},

	// ─── Кнопка (Button) ─────────────────────────────────────────────────────

	{name: "BtnBG", field: func(t *Theme) *color.RGBA { return &t.BtnBG },
		style: theme.StyleKey{Component: "button"}, role: roleFill},
	{name: "BtnBorder", field: func(t *Theme) *color.RGBA { return &t.BtnBorder },
		style: theme.StyleKey{Component: "button"}, role: roleBorder},
	{name: "BtnHoverBG", field: func(t *Theme) *color.RGBA { return &t.BtnHoverBG },
		style: theme.StyleKey{Component: "button", State: theme.StateHover}, role: roleFill},
	{name: "BtnPressedBG", field: func(t *Theme) *color.RGBA { return &t.BtnPressedBG },
		style: theme.StyleKey{Component: "button", State: theme.StatePressed}, role: roleFill},
	{name: "BtnText", field: func(t *Theme) *color.RGBA { return &t.BtnText },
		style: theme.StyleKey{Component: "button"}, role: roleText},

	// ─── Текстовое поле (TextInput / TextBox) ───────────────────────────────

	{name: "InputBG", field: func(t *Theme) *color.RGBA { return &t.InputBG },
		style: theme.StyleKey{Component: "textinput"}, role: roleFill},
	{name: "InputBorder", field: func(t *Theme) *color.RGBA { return &t.InputBorder },
		style: theme.StyleKey{Component: "textinput"}, role: roleBorder},
	{name: "InputFocus", field: func(t *Theme) *color.RGBA { return &t.InputFocus },
		style: theme.StyleKey{Component: "textinput", State: theme.StateFocused}, role: roleBorder},
	{name: "InputText", field: func(t *Theme) *color.RGBA { return &t.InputText },
		style: theme.StyleKey{Component: "textinput"}, role: roleText},
	{name: "InputCaret", field: func(t *Theme) *color.RGBA { return &t.InputCaret },
		style: theme.StyleKey{Component: "textinput", Part: "caret"}, role: roleFill},
	{name: "InputPlaceholder", field: func(t *Theme) *color.RGBA { return &t.InputPlaceholder },
		style: theme.StyleKey{Component: "textinput", Part: "placeholder"}, role: roleText},

	// ─── Метка (Label) ───────────────────────────────────────────────────────

	{name: "LabelText", field: func(t *Theme) *color.RGBA { return &t.LabelText },
		style: theme.StyleKey{Component: "label"}, role: roleText},
	// LabelBG сегодня ни один виджет не читает (Label.Draw берёт фон из
	// PanelBG) — но поле объявлено в Theme и обязано пережить round-trip,
	// поэтому у него есть законный адрес, как и у всех остальных.
	{name: "LabelBG", field: func(t *Theme) *color.RGBA { return &t.LabelBG },
		style: theme.StyleKey{Component: "label"}, role: roleFill},
	// Пояснительный текст — часть «label», но со своим состоянием: это не
	// выключенная метка (StateDisabled), а вторичная по смыслу.
	{name: "SecondaryText", field: func(t *Theme) *color.RGBA { return &t.SecondaryText },
		style: theme.StyleKey{Component: "label", Part: "secondary"}, role: roleText},

	// ─── Прогресс-бар (ProgressBar) ─────────────────────────────────────────

	{name: "ProgressBG", field: func(t *Theme) *color.RGBA { return &t.ProgressBG },
		style: theme.StyleKey{Component: "progressbar", Part: "track"}, role: roleFill},
	{name: "ProgressFill", field: func(t *Theme) *color.RGBA { return &t.ProgressFill },
		style: theme.StyleKey{Component: "progressbar", Part: "fill"}, role: roleFill},
	// Концы градиента светящейся полосы — как и TitleBG2, вторая/третья точка
	// градиента, для которой в Style нет отдельного слота.
	{name: "ProgressGlowTail", field: func(t *Theme) *color.RGBA { return &t.ProgressGlowTail },
		key: "progressbar.glow.tail"},
	{name: "ProgressGlowHead", field: func(t *Theme) *color.RGBA { return &t.ProgressGlowHead },
		key: "progressbar.glow.head"},

	// ─── Выпадающий список (Dropdown) и PopupMenu ───────────────────────────

	{name: "DropBG", field: func(t *Theme) *color.RGBA { return &t.DropBG },
		style: theme.StyleKey{Component: "dropdown"}, role: roleFill},
	{name: "DropBorder", field: func(t *Theme) *color.RGBA { return &t.DropBorder },
		style: theme.StyleKey{Component: "dropdown"}, role: roleBorder},
	{name: "DropText", field: func(t *Theme) *color.RGBA { return &t.DropText },
		style: theme.StyleKey{Component: "dropdown"}, role: roleText},
	{name: "DropArrow", field: func(t *Theme) *color.RGBA { return &t.DropArrow },
		style: theme.StyleKey{Component: "dropdown", Part: "arrow"}, role: roleFill},
	{name: "DropItemBG", field: func(t *Theme) *color.RGBA { return &t.DropItemBG },
		style: theme.StyleKey{Component: "dropdown", Part: "item", State: theme.StateActive}, role: roleFill},

	// MenuHoverBG/MenuHoverText/MenuBG обслуживают PopupMenu, MenuBar и
	// Dropdown одновременно (все три читают одни и те же поля Theme) —
	// адресуем их под общим компонентом "menu", как самостоятельным от
	// dropdown/menubar понятием "выпадающее меню".
	{name: "MenuHoverBG", field: func(t *Theme) *color.RGBA { return &t.MenuHoverBG },
		style: theme.StyleKey{Component: "menu", Part: "item", State: theme.StateHover}, role: roleFill},
	{name: "MenuHoverText", field: func(t *Theme) *color.RGBA { return &t.MenuHoverText },
		style: theme.StyleKey{Component: "menu", Part: "item", State: theme.StateHover}, role: roleText},
	{name: "MenuBG", field: func(t *Theme) *color.RGBA { return &t.MenuBG },
		style: theme.StyleKey{Component: "menu"}, role: roleFill},

	// ─── Чекбокс (CheckBox) и радиокнопка (RadioButton) ─────────────────────

	{name: "CheckBG", field: func(t *Theme) *color.RGBA { return &t.CheckBG },
		style: theme.StyleKey{Component: "checkbox", Part: "box"}, role: roleFill},
	{name: "CheckBorder", field: func(t *Theme) *color.RGBA { return &t.CheckBorder },
		style: theme.StyleKey{Component: "checkbox", Part: "box"}, role: roleBorder},
	{name: "CheckMark", field: func(t *Theme) *color.RGBA { return &t.CheckMark },
		style: theme.StyleKey{Component: "checkbox", Part: "mark", State: theme.StateActive}, role: roleFill},
	{name: "CheckHoverBG", field: func(t *Theme) *color.RGBA { return &t.CheckHoverBG },
		style: theme.StyleKey{Component: "checkbox", Part: "box", State: theme.StateHover}, role: roleFill},
	{name: "CheckText", field: func(t *Theme) *color.RGBA { return &t.CheckText },
		style: theme.StyleKey{Component: "checkbox", Part: "label"}, role: roleText},

	// ─── Вкладки (TabControl) ────────────────────────────────────────────────

	{name: "TabBG", field: func(t *Theme) *color.RGBA { return &t.TabBG },
		style: theme.StyleKey{Component: "tab", Part: "item"}, role: roleFill},
	{name: "TabActiveBG", field: func(t *Theme) *color.RGBA { return &t.TabActiveBG },
		style: theme.StyleKey{Component: "tab", Part: "item", State: theme.StateActive}, role: roleFill},
	{name: "TabBorder", field: func(t *Theme) *color.RGBA { return &t.TabBorder },
		style: theme.StyleKey{Component: "tab"}, role: roleBorder},
	{name: "TabText", field: func(t *Theme) *color.RGBA { return &t.TabText },
		style: theme.StyleKey{Component: "tab", Part: "item"}, role: roleText},
	{name: "TabActiveText", field: func(t *Theme) *color.RGBA { return &t.TabActiveText },
		style: theme.StyleKey{Component: "tab", Part: "item", State: theme.StateActive}, role: roleText},
	{name: "TabContentBG", field: func(t *Theme) *color.RGBA { return &t.TabContentBG },
		style: theme.StyleKey{Component: "tab", Part: "content"}, role: roleFill},

	// ─── Ползунок (Slider) ───────────────────────────────────────────────────

	{name: "SliderTrackBG", field: func(t *Theme) *color.RGBA { return &t.SliderTrackBG },
		style: theme.StyleKey{Component: "slider", Part: "track"}, role: roleFill},
	{name: "SliderFill", field: func(t *Theme) *color.RGBA { return &t.SliderFill },
		style: theme.StyleKey{Component: "slider", Part: "fill"}, role: roleFill},
	{name: "SliderThumb", field: func(t *Theme) *color.RGBA { return &t.SliderThumb },
		style: theme.StyleKey{Component: "slider", Part: "thumb"}, role: roleFill},
	{name: "SliderBorder", field: func(t *Theme) *color.RGBA { return &t.SliderBorder },
		style: theme.StyleKey{Component: "slider", Part: "track"}, role: roleBorder},

	// ─── Переключатель (ToggleSwitch) ────────────────────────────────────────

	{name: "ToggleBG", field: func(t *Theme) *color.RGBA { return &t.ToggleBG },
		style: theme.StyleKey{Component: "toggle", Part: "track"}, role: roleFill},
	{name: "ToggleOnBG", field: func(t *Theme) *color.RGBA { return &t.ToggleOnBG },
		style: theme.StyleKey{Component: "toggle", Part: "track", State: theme.StateActive}, role: roleFill},
	{name: "ToggleThumb", field: func(t *Theme) *color.RGBA { return &t.ToggleThumb },
		style: theme.StyleKey{Component: "toggle", Part: "thumb"}, role: roleFill},
	{name: "ToggleBorder", field: func(t *Theme) *color.RGBA { return &t.ToggleBorder },
		style: theme.StyleKey{Component: "toggle", Part: "track"}, role: roleBorder},

	// ─── Скроллбар (ScrollView) и списки (ListView) ─────────────────────────

	{name: "ScrollTrackBG", field: func(t *Theme) *color.RGBA { return &t.ScrollTrackBG },
		style: theme.StyleKey{Component: "scrollbar", Part: "track"}, role: roleFill},
	{name: "ScrollThumbBG", field: func(t *Theme) *color.RGBA { return &t.ScrollThumbBG },
		style: theme.StyleKey{Component: "scrollbar", Part: "thumb"}, role: roleFill},
	{name: "ListItemHover", field: func(t *Theme) *color.RGBA { return &t.ListItemHover },
		style: theme.StyleKey{Component: "list", Part: "item", State: theme.StateHover}, role: roleFill},
	{name: "ListItemSelect", field: func(t *Theme) *color.RGBA { return &t.ListItemSelect },
		style: theme.StyleKey{Component: "list", Part: "item", State: theme.StateActive}, role: roleFill},

	// ─── Дерево (TreeView) ───────────────────────────────────────────────────

	{name: "TreeText", field: func(t *Theme) *color.RGBA { return &t.TreeText },
		style: theme.StyleKey{Component: "tree", Part: "item"}, role: roleText},
	{name: "TreeArrow", field: func(t *Theme) *color.RGBA { return &t.TreeArrow },
		style: theme.StyleKey{Component: "tree", Part: "indicator"}, role: roleFill},

	// ─── Диалог (Dialog) ─────────────────────────────────────────────────────

	{name: "DialogBG", field: func(t *Theme) *color.RGBA { return &t.DialogBG },
		style: theme.StyleKey{Component: "dialog"}, role: roleFill},
	{name: "DialogTitleBG", field: func(t *Theme) *color.RGBA { return &t.DialogTitleBG },
		style: theme.StyleKey{Component: "dialog", Part: "titlebar"}, role: roleFill},
	{name: "DialogDim", field: func(t *Theme) *color.RGBA { return &t.DialogDim },
		style: theme.StyleKey{Component: "dialog", Part: "scrim"}, role: roleFill},

	// ─── Разделитель (GridSplitter / SplitPanel) ────────────────────────────

	{name: "SplitterBG", field: func(t *Theme) *color.RGBA { return &t.SplitterBG },
		style: theme.StyleKey{Component: "splitter"}, role: roleFill},
	{name: "SplitterHoverBG", field: func(t *Theme) *color.RGBA { return &t.SplitterHoverBG },
		style: theme.StyleKey{Component: "splitter", State: theme.StateHover}, role: roleFill},

	// ─── Строка состояния (StatusBar) ───────────────────────────────────────

	{name: "StatusBarBG", field: func(t *Theme) *color.RGBA { return &t.StatusBarBG },
		style: theme.StyleKey{Component: "statusbar"}, role: roleFill},
	{name: "StatusBarText", field: func(t *Theme) *color.RGBA { return &t.StatusBarText },
		style: theme.StyleKey{Component: "statusbar"}, role: roleText},

	// ─── Заголовок колонок (DataGrid / ListView header) ─────────────────────

	{name: "HeaderBG", field: func(t *Theme) *color.RGBA { return &t.HeaderBG },
		style: theme.StyleKey{Component: "datagrid", Part: "header"}, role: roleFill},
	{name: "HeaderText", field: func(t *Theme) *color.RGBA { return &t.HeaderText },
		style: theme.StyleKey{Component: "datagrid", Part: "header"}, role: roleText},

	// ─── Системные / сквозные ────────────────────────────────────────────────

	// Accent — акцентный цвет, который напрямую (в обход своих полей Theme)
	// читают ~15 виджетов: Button.HighlightTop, CheckBox, Dropdown.FocusBorder,
	// TreeView.FocusBorderColor, TextBox/TextInput.SelColor, Slider.ThumbHover,
	// ScrollView/ListView.ThumbHoverBG, GridSplitter.HoverColor,
	// DockManager/DockPane.AccentColor и другие. Раскладывать по компонентам
	// значило бы продублировать одно значение полтора десятка раз.
	{name: "Accent", field: func(t *Theme) *color.RGBA { return &t.Accent },
		key: "accent"},
	// Scrollbar — глобальный fallback-цвет, объявлен во всех пресетах, но не
	// читается ни одним настоящим скроллбаром (все используют ScrollTrackBG/
	// ScrollThumbBG) — чистый дубль без потребителей, тем не менее поле есть
	// в Theme и обязано сохраниться при round-trip.
	{name: "Scrollbar", field: func(t *Theme) *color.RGBA { return &t.Scrollbar },
		key: "scrollbar.fallback"},
	// Disabled — комментарий в themes.go обещает «все виджеты», но реально
	// читает только PopupMenu.DisabledColor. Плоский ключ соответствует
	// прицелу плана: встроенное значение по умолчанию для StateDisabled,
	// когда конкретный компонент своего не объявил.
	{name: "Disabled", field: func(t *Theme) *color.RGBA { return &t.Disabled },
		key: "disabled.default"},
}
