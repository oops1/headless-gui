// xaml.go — главный диспетчер XAML-виджетов и построители простых элементов.
//
// Публичный API: LoadUIFromXAMLFile, LoadUIFromXAML, LoadUIFromXAMLWithBase.
// Внутренний диспетчер: buildXAMLWidget — маршрутизирует по тегу XAML-элемента.
// Построители простых виджетов: Label, Button, Dropdown, ProgressBar,
// Separator, CheckBox, RadioButton, Slider, ToggleSwitch, ScrollView, ListView, Image.
//
// Контейнеры (Grid, Window, Canvas, DockPanel и др.) — в containers.go.
// Парсинг XML, цветов и Margin — в parse.go.
// Применение attached-свойств — в props.go.
package xaml

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v3/widget/base"
	svgPkg "github.com/oops1/headless-gui/v3/widget/svg"
)

// ─── Публичный API ───────────────────────────────────────────────────────────

// LoadUIFromXAMLFile читает XAML-файл и строит дерево виджетов.
// Возвращает корневой виджет и map[name]Widget для именованных элементов.
func LoadUIFromXAMLFile(path string) (base.Widget, map[string]base.Widget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("xaml: read %q: %w", path, err)
	}
	baseDir := filepath.Dir(path)
	return LoadUIFromXAMLWithBase(data, baseDir)
}

// LoadUIFromXAML разбирает XAML и строит дерево виджетов.
// Ресурсы (изображения) не могут загружаться — baseDir пустой.
func LoadUIFromXAML(data []byte) (base.Widget, map[string]base.Widget, error) {
	w, reg, _, err := loadUIFromXAML(data, "", nil)
	return w, reg, err
}

// LoadUIFromXAMLWithBase разбирает XAML и строит дерево виджетов.
// baseDir используется для загрузки ресурсов (BackgroundImage и пр.).
func LoadUIFromXAMLWithBase(data []byte, baseDir string) (base.Widget, map[string]base.Widget, error) {
	w, reg, _, err := loadUIFromXAML(data, baseDir, nil)
	return w, reg, err
}

// LoadUIFromXAMLFS разбирает XAML и строит дерево виджетов, читая ресурсы
// разметки (Image Source, Button Icon/IconSource, Panel BackgroundImage,
// SVGIcon Source, Window TrayIcon) из fsys вместо каталога на диске (GG-7).
//
// Нужна для однофайловых сборок (embed.FS и т.п.): у такого приложения нет
// каталога с ресурсами на диске, поэтому LoadUIFromXAMLWithBase здесь не
// применить. Путь в разметке резолвится и удерживается внутри fsys ровно по
// той же политике SEC-8, что и baseDir на диске — абсолютный путь или выход
// за пределы корня отклоняется с ErrResourceOutsideBase (см. OpenResource).
func LoadUIFromXAMLFS(data []byte, fsys fs.FS) (base.Widget, map[string]base.Widget, error) {
	key := base.RegisterResourceFS(fsys)
	// DataContext здесь нет (у этой функции нет параметра ctx), а значит нет и
	// живых перестроений ItemsControl/VirtualizingItemsControl — единственных
	// мест, которые обращаются к baseDir уже ПОСЛЕ возврата из loadUIFromXAML
	// (см. wireItems/wireVirtuals в binding.go — оба выходят рано при
	// ctx == nil). Поэтому запись реестра снимается сразу же, без утечки.
	defer base.UnregisterResourceFS(key)
	w, reg, _, err := loadUIFromXAML(data, key, nil)
	return w, reg, err
}

// LoadUIFromXAMLWithContext разбирает XAML и привязывает {Binding Path} к
// dataContext. Привязки живые: при INotifyPropertyChanged у dataContext UI
// обновляется автоматически, TwoWay-поля пишут обратно в модель.
// Возвращаемый BindingScope удерживается подписками, поэтому привязки работают
// даже если scope не сохранён вызывающим кодом.
func LoadUIFromXAMLWithContext(data []byte, dataContext interface{}) (base.Widget, map[string]base.Widget, error) {
	w, reg, _, err := loadUIFromXAML(data, "", dataContext)
	return w, reg, err
}

// LoadUIFromXAMLBindings — как LoadUIFromXAMLWithContext, но возвращает BindingScope
// для ручного управления (SetDataContext / Refresh).
func LoadUIFromXAMLBindings(data []byte, dataContext interface{}) (base.Widget, map[string]base.Widget, *BindingScope, error) {
	return loadUIFromXAML(data, "", dataContext)
}

// LoadUIFromXAMLFileWithContext — как LoadUIFromXAMLFile, но с DataContext для {Binding}.
func LoadUIFromXAMLFileWithContext(path string, dataContext interface{}) (base.Widget, map[string]base.Widget, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("xaml: read %q: %w", path, err)
	}
	w, reg, _, err := loadUIFromXAML(data, filepath.Dir(path), dataContext)
	return w, reg, err
}

// LoadUIFromXAMLFileBindings — как LoadUIFromXAMLFileWithContext, но с BindingScope.
func LoadUIFromXAMLFileBindings(path string, dataContext interface{}) (base.Widget, map[string]base.Widget, *BindingScope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("xaml: read %q: %w", path, err)
	}
	return loadUIFromXAML(data, filepath.Dir(path), dataContext)
}

// loadUIFromXAML — общая реализация: парсинг → пред-обработка (Resources/Style/
// markup/{Binding}) → построение дерева → активация живых привязок.
func loadUIFromXAML(data []byte, baseDir string, dataContext interface{}) (base.Widget, map[string]base.Widget, *BindingScope, error) {
	root, err := parseXAML(data)
	if err != nil {
		return nil, nil, nil, err
	}
	env := preprocessXAML(root, dataContext)
	registry := make(map[string]base.Widget)
	// Переводимые строки, свёрнутые в родителя (меню, вкладки, колонки),
	// записываются за этой загрузкой: их переводит и снимает scope дерева, а не
	// общий список на весь процесс (GG-75).
	owner := &locOwner{}
	var w base.Widget
	withLocOwner(owner, func() {
		w, err = buildXAMLWidget(*root, registry, image.Point{}, baseDir)
	})
	if err != nil {
		removeOwnedLocItems(owner) // строки полусобранного дерева не нужны
		return nil, nil, nil, err
	}
	scope := newBindingScope(dataContext, env, registry, baseDir)
	scope.locOwner = owner
	scope.root = w
	scope.activate()
	resolveWindowInputCommands(w, dataContext)
	scope.track()
	return w, registry, scope, nil
}

// resolveWindowInputCommands связывает CommandPath из Window/Canvas.InputBindings
// с объектами-командами из DataContext.
func resolveWindowInputCommands(root base.Widget, ctx interface{}) {
	if ctx == nil {
		return
	}
	var bindings []base.InputBinding
	switch t := root.(type) {
	case *base.Window:
		bindings = t.InputBindings
	case *base.Canvas:
		bindings = t.InputBindings
	default:
		return
	}
	for i := range bindings {
		p := bindings[i].CommandPath
		if p == "" {
			continue
		}
		if v, ok := dgridGetProperty(ctx, p); ok {
			if cmd, ok := v.(base.ICommand); ok {
				bindings[i].Command = cmd
			}
		}
	}
}

// ─── Ресурсы разметки: диск или fs.FS (GG-7) ───────────────────────────────
//
// baseDir остаётся string во ВСЕХ билдерах — ни один из ~25 buildXAML*
// (buildXAMLWidget включительно) не меняет сигнатуру. Причина не только в
// формулировке задачи: binding.go хранит baseDir как string-поле
// (pendingItems.baseDir, pendingVirtual.baseDir) и передаёт его обратно в
// buildXAMLWidget при живом перестроении ItemsControl/VirtualizingItemsControl
// — менять тип значило бы менять и binding.go, а трогать этот файл здесь
// не входит в задачу.
//
// Поэтому fs.FS протаскивается тем же каналом, что и baseDir: LoadUIFromXAMLFS
// заводит СИНТЕТИЧЕСКИЙ baseDir — строку-маркер, недостижимую для реального
// пути каталога, — и кладёт fsys в реестр под этим ключом. Единая точка
// чтения ресурса (OpenResource) сначала смотрит в реестр: нашли — это
// fs.FS; не нашли — обычный путь на диске (или пустой baseDir, как раньше).

// decodeXAMLImage читает и декодирует растровое изображение (PNG/JPEG) через
// OpenResource — общая точка для Image.Source, Button.Icon и TrayIcon.
// Декодирование ограничено (DecodeImageBounded, SEC-9) так же, как раньше
// у loadImageFile/ImageWidget.SetSource.
func decodeXAMLImage(baseDir, src string) (image.Image, error) {
	f, err := base.OpenResource(baseDir, src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return base.DecodeImageBounded(f)
}

// decodeXAMLImageRGBA — как decodeXAMLImage, но приводит результат к
// *image.RGBA. Нужна только для Panel.BackgroundImage: это поле типизировано
// именно как *image.RGBA (в отличие от Image.Source/Button.Icon/TrayIcon,
// которым достаточно интерфейса image.Image). Конвертация — копия финального
// шага loadImageFile (props.go); отдельно, потому что читать оттуда
// нельзя без правки того файла, а сам он остаётся диск-only.
func decodeXAMLImageRGBA(baseDir, src string) (*image.RGBA, error) {
	img, err := decodeXAMLImage(baseDir, src)
	if err != nil {
		return nil, err
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba, nil
}

// readXAMLResourceBytes читает ресурс целиком, но не больше maxBytes —
// используется там, где нет потокового декодера с собственным лимитом (SVG:
// svg.Parse проверяет len(data) уже ПОСЛЕ чтения, поэтому чтение неограниченного
// файла целиком в память само по себе было бы дырой — как раньше делал
// svg.ParseFile через io.LimitReader).
func readXAMLResourceBytes(baseDir, src string, maxBytes int64) ([]byte, error) {
	f, err := base.OpenResource(baseDir, src)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("xaml: ресурс больше предела %d байт", maxBytes)
	}
	return data, nil
}

// ─── Построитель виджетов ───────────────────────────────────────────────────

// buildXAMLWidget строит виджет из XAML-элемента.
// parentOff — абсолютное смещение родительского контейнера; координаты
// потомков (Canvas.Left, Canvas.Top, Left, Top, …) трактуются как
// относительные и сдвигаются на parentOff, что соответствует поведению
// WPF Canvas и позволяет открывать XAML-файлы в Blend.
// Для корневого элемента parentOff = image.Point{}.
//
// Это точка входа с глубины 0: рекурсия идёт через buildXAMLWidgetAt.
func buildXAMLWidget(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string) (base.Widget, error) {
	return buildXAMLWidgetAt(el, reg, parentOff, baseDir, 0)
}

// buildXAMLWidgetAt — рекурсивное тело buildXAMLWidget. depth — глубина
// текущего элемента в дереве; превышение maxXAMLDepth прерывает построение
// ошибкой, не давая рекурсии исчерпать стек (SEC-7). Парсер держит ту же
// границу, так что здесь это вторая линия обороны — на случай деревьев,
// собранных в обход parseXAML (клоны DataTemplate/ItemsControl).
func buildXAMLWidgetAt(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	if depth > maxXAMLDepth {
		return nil, fmt.Errorf("%w (%d): <%s>", ErrXAMLTooDeep, maxXAMLDepth, el.Tag)
	}
	tag := strings.ToLower(el.Tag)

	// Игнорируем теги-свойства WPF (Panel.Children, Grid.RowDefinitions, …)
	if strings.Contains(tag, ".") {
		return nil, nil // пропускаем как дочерний виджет
	}

	// Ресурсы/стили обрабатываются в preprocessXAML — как виджеты не строим.
	switch tag {
	case "style", "setter", "resourcedictionary", "solidcolorbrush",
		"lineargradientbrush", "radialgradientbrush", "gradientstop", "color",
		"keybinding", "mousebinding", "controltemplate", "contentpresenter":
		return nil, nil
	}

	var w base.Widget

	switch tag {
	// ── Grid ────────────────────────────────────────────────────────────────
	case "grid":
		return buildXAMLGrid(el, reg, parentOff, baseDir, depth)

	// ── Window — корневой элемент нативного окна ────────────────────────────
	case "window":
		return buildXAMLWindow(el, reg, parentOff, baseDir, depth)

	// ── StackPanel — контейнер с автораскладкой ─────────────────────────────
	case "stackpanel":
		return buildXAMLStackPanel(el, reg, parentOff, baseDir, depth)

	// ── ItemsControl — список объектов по DataTemplate ──────────────────────
	// preprocessXAML разворачивает его в StackPanel (по элементам ItemsSource);
	// неразвёрнутый (нет шаблона/контекста) строится как пустой StackPanel.
	case "itemscontrol":
		return buildXAMLStackPanel(el, reg, parentOff, baseDir, depth)

	// ── WrapPanel — контейнер с переносом ───────────────────────────────────
	case "wrappanel":
		return buildXAMLWrapPanel(el, reg, parentOff, baseDir, depth)

	// ── UniformGrid — равномерная сетка ─────────────────────────────────────
	case "uniformgrid":
		return buildXAMLUniformGrid(el, reg, parentOff, baseDir, depth)

	// ── GroupBox — рамка с заголовком ───────────────────────────────────────
	case "groupbox":
		return buildXAMLGroupBox(el, reg, parentOff, baseDir, depth)

	// ── Expander — раскрывающаяся панель ────────────────────────────────────
	case "expander":
		return buildXAMLExpander(el, reg, parentOff, baseDir, depth)

	// ── TreeView — иерархический список ─────────────────────────────────────
	case "treeview":
		return buildXAMLTreeView(el, reg, parentOff)

	// ── DiffView — сравнение и правка двух текстов ──────────────────────────
	case "diffview":
		w = buildXAMLDiffView(el, baseDir)

	// ── MergeView — трёхстороннее слияние ───────────────────────────────────
	case "mergeview":
		w = buildXAMLMergeView(el, baseDir)

	// ── RichText — форматированный текст (абзацы из ранов) ──────────────────
	case "richtext":
		w = buildXAMLRichText(el)

	// ── TreeViewItem вне TreeView — игнорируем ──────────────────────────────
	case "treeviewitem":
		return nil, nil

	// ── DataGrid column definitions — игнорируем вне DataGrid ───────────────
	case "datagridtextcolumn", "datagridtemplatecolumn",
		"datagridcheckboxcolumn", "datagridcomboboxcolumn":
		return nil, nil

	// ── DockPanel — контейнер с dock-layout ────────────────────────────────
	case "dockpanel":
		return buildXAMLDockPanel(el, reg, parentOff, baseDir, depth)

	// ── DockManager — зона докинга (инструментальные панели VS-стиля) ──────
	case "dockmanager":
		return buildXAMLDockManager(el, reg, parentOff, baseDir, depth)

	// ── DockPane вне DockManager — игнорируем (валиден только как child) ───
	case "dockpane":
		return nil, nil

	// ── DockContent вне DockManager — маркер, не виджет ─────────────────────
	case "dockcontent":
		return nil, nil

	// ── GridSplitter — перетаскиваемый разделитель ячеек Grid ───────────────
	case "gridsplitter":
		s := base.NewGridSplitter()
		ApplyColor(&s.Background, el, "Background")
		w = s

	// ── SplitPanel — две панели с перетаскиваемым разделителем ──────────────
	case "splitpanel":
		return buildXAMLSplitPanel(el, reg, parentOff, baseDir, depth)

	// ── ToolBarTray / ToolBar → горизонтальный StackPanel (WPF ToolBar) ────
	case "toolbartray":
		return buildXAMLToolBarTray(el, reg, parentOff, baseDir, depth)
	case "toolbar":
		return buildXAMLToolBar(el, reg, parentOff, baseDir, depth)

	// ── StatusBar → StackPanel (горизонтальный) ────────────────────────────
	case "statusbar":
		return buildXAMLStatusBar(el, reg, parentOff, baseDir, depth)

	// ── DataGrid — полноценный табличный виджет ─────────────────────────────
	case "datagrid":
		w = buildXAMLDataGrid(el)

	// ── Border — контейнер с фоном и одним потомком ─────────────────────────
	case "border":
		return buildXAMLBorder(el, reg, parentOff, baseDir, depth)

	// ── Canvas — контейнер с абсолютным позиционированием (WPF Canvas) ──────
	case "canvas":
		return buildXAMLCanvas(el, reg, parentOff, baseDir, depth)

	// ── Контейнеры ──────────────────────────────────────────────────────────
	case "usercontrol",
		"panel", "viewbox", "contentcontrol", "headeredcontentcontrol":
		w = buildXAMLPanel(el, baseDir)

	// ── Текст ────────────────────────────────────────────────────────────────
	case "label", "textblock", "text", "run":
		w = buildXAMLLabel(el)

	// ── Кнопки ───────────────────────────────────────────────────────────────
	case "button", "togglebutton", "repeatbutton":
		w = buildXAMLButton(el, baseDir)
		// ToggleButton — та же кнопка, но с устойчивым состоянием «нажата»
		// (см. widget/togglebutton.go). Прежде тег разбирался как обычный
		// Button, и переключатель из разметки было не получить.
		if tag == "togglebutton" {
			if btn, ok := w.(*base.Button); ok {
				btn.SetToggle(true)
			}
		}

	// ── Кнопка с меню: Pull▾, Git-Flow▾ (GG-78) ─────────────────────────────
	case "menubutton", "splitbutton", "dropdownbutton":
		w = buildXAMLMenuButton(el, tag, baseDir)

	// ── Распорка: забирает остаток места в StackPanel и ToolBar (GG-84) ─────
	case "stretch", "toolbarstretch", "spacer":
		w = buildXAMLStretch(el)

	// ── Выпадающий список ────────────────────────────────────────────────────
	case "combobox", "dropdown":
		w = buildXAMLDropdown(el)

	// ── Поле даты с календарём ──────────────────────────────────────────────
	case "datepicker":
		w = buildXAMLDatePicker(el)

	// ── Поле цвета и образец ────────────────────────────────────────────────
	case "colorpicker":
		w = buildXAMLColorPicker(el)
	case "swatch", "colorbox":
		w = buildXAMLSwatch(el)

	// ── Прогресс ─────────────────────────────────────────────────────────────
	case "progressbar":
		w = buildXAMLProgressBar(el)

	// ── CheckBox ─────────────────────────────────────────────────────────────
	case "checkbox":
		w = buildXAMLCheckBox(el)

	// ── RadioButton ──────────────────────────────────────────────────────────
	case "radiobutton":
		w = buildXAMLRadioButton(el)

	// ── TabControl ───────────────────────────────────────────────────────────
	case "tabcontrol":
		return buildXAMLTabControl(el, reg, parentOff, baseDir, depth)

	// ── Slider ───────────────────────────────────────────────────────────────
	case "slider":
		w = buildXAMLSlider(el)

	// ── NumericUpDown (Extended Toolkit IntegerUpDown / DoubleUpDown) ─────────
	case "numericupdown", "integerupdown", "doubleupdown":
		w = buildXAMLNumericUpDown(el)

	// ── VirtualizingItemsControl (UI-виртуализация длинных списков) ───────────
	case "virtualizingitemscontrol":
		w = buildXAMLVirtualizingItemsControl(el)

	// ── ToggleSwitch ─────────────────────────────────────────────────────────
	case "toggleswitch":
		w = buildXAMLToggleSwitch(el)

	// ── ScrollViewer ─────────────────────────────────────────────────────────
	case "scrollviewer", "scrollview":
		w = buildXAMLScrollView(el)

	// ── ListView ─────────────────────────────────────────────────────────────
	case "listview", "listbox":
		w = buildXAMLListView(el)

	// ── Изображение ───────────────────────────────────────────────────────────
	case "image":
		w = buildXAMLImage(el, baseDir)

	// ── SVG-иконка (темизируемая векторная) ─────────────────────────────────
	case "svgicon":
		w = buildXAMLSVGIcon(el, baseDir)

	// ── Разделитель ──────────────────────────────────────────────────────────
	case "separator":
		w = buildXAMLSeparator(el)

	// ── Векторные фигуры (WPF Shapes) ────────────────────────────────────────
	case "rectangle":
		w = buildXAMLRectangleShape(el)
	case "ellipse":
		w = buildXAMLEllipse(el)
	case "line":
		return buildXAMLLine(el, reg, parentOff), nil
	case "polygon":
		return buildXAMLPolygon(el, reg, parentOff), nil
	case "polyline":
		return buildXAMLPolyline(el, reg, parentOff), nil

	// ── PopupMenu ────────────────────────────────────────────────────────────
	case "popupmenu", "contextmenu":
		return buildXAMLPopupMenu(el, reg, parentOff, baseDir)

	// ── MenuBar (горизонтальное меню) ────────────────────────────────────────
	case "menu", "menubar", "mainmenu":
		return buildXAMLMenuBar(el, reg, parentOff, baseDir)

	default:
		// Теги пакетов виджетов вне xaml (ввод текста — widget/text):
		// реестр Default(), заполненный их RegisterXAML.
		if b, ok := Default().Lookup(tag); ok {
			cw, err := b(&el, &Loader{baseDir: baseDir})
			if err != nil {
				return nil, fmt.Errorf("xaml: <%s>: %w", el.Tag, err)
			}
			if cw == nil {
				return nil, nil
			}
			w = cw
			break
		}
		// Проверяем реестр пользовательских виджетов.
		if builder, ok := lookupCustomXAML(tag); ok {
			cw, err := builder(newXAMLAttrs(&el))
			if err != nil {
				return nil, fmt.Errorf("xaml: custom <%s>: %w", el.Tag, err)
			}
			if cw != nil {
				w = cw
				break
			}
		}
		return nil, fmt.Errorf("xaml: неизвестный элемент <%s>", el.Tag)
	}

	// Координаты в XAML относительны родительского Canvas/Panel (стандарт WPF).
	// Прибавляем parentOff чтобы получить абсолютные экранные координаты.
	// Если el.bounds() пуст (нет координат в XAML) — не затираем bounds,
	// которые виджет мог установить сам (напр. Separator с дефолтным размером).
	absBounds := el.Bounds().Add(parentOff)
	if !absBounds.Empty() {
		w.SetBounds(absBounds)
	}

	// Сохраняем явные XAML Width/Height для alignment (даже если bounds пуст).
	type xamlSizeSetter interface {
		SetXAMLSize(w, h int)
	}
	if xss, ok := w.(xamlSizeSetter); ok {
		xw := ParseInt(el.Attr("Width"))
		xh := ParseInt(el.Attr("Height"))
		if xw > 0 || xh > 0 {
			xss.SetXAMLSize(xw, xh)
		}
	}

	// Attached properties: Grid.*, DockPanel.Dock, Margin, Alignment, IsEnabled,
	// ToolTip, Visibility, ShowLocaleIndicator.
	applyCommonProps(w, el)

	// Регистрация по имени
	if id := el.Name(); id != "" {
		reg[id] = w
	}

	// Смещение для дочерних виджетов — Min текущего элемента.
	// Если absBounds пуст — берём из реальных bounds виджета.
	childOff := absBounds.Min
	if absBounds.Empty() {
		childOff = w.Bounds().Min
	}

	// Дочерние виджеты (пропускаем <Item>, <TabItem> — уже обработаны)
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		// Абзацы RichText разобраны в buildXAMLRichText: это не виджеты. Теги-
		// свойства (RichText.ContextMenu) идут обычным путём.
		if _, ok := w.(*base.RichText); ok && !strings.Contains(childTag, ".") {
			continue
		}
		if childTag == "item" || childTag == "comboboxitem" || childTag == "listboxitem" ||
			childTag == "tabitem" || childTag == "listviewitem" {
			continue
		}
		// Пункты кнопки с меню уже разобраны в buildXAMLMenuButton.
		if _, ok := w.(*base.MenuButton); ok && (childTag == "menuitem" || childTag == "separator") {
			continue
		}
		// Attached ContextMenu: <X.ContextMenu><ContextMenu>…</ContextMenu></X.ContextMenu>
		if strings.HasSuffix(childTag, ".contextmenu") {
			for _, inner := range child.Children {
				cw, err := buildXAMLWidgetAt(inner, reg, childOff, baseDir, depth+1)
				if err != nil {
					return nil, err
				}
				if pm, ok := cw.(*base.PopupMenu); ok {
					if h, ok := w.(interface{ SetContextMenu(*base.PopupMenu) }); ok {
						h.SetContextMenu(pm)
					}
					w.AddChild(pm) // в дереве — чтобы движок рисовал overlay и ловил клики
				}
			}
			continue
		}
		if strings.Contains(childTag, ".") {
			// Ресурсы уже собраны в preprocessXAML — не строим как виджеты.
			if strings.HasSuffix(childTag, ".resources") {
				continue
			}
			// WPF property element — пропускаем сам тег, но обрабатываем его потомков
			for _, inner := range child.Children {
				cw, err := buildXAMLWidgetAt(inner, reg, childOff, baseDir, depth+1)
				if err != nil {
					return nil, err
				}
				if cw != nil {
					w.AddChild(cw)
				}
			}
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, childOff, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			w.AddChild(cw)
		}
	}

	return w, nil
}

// ─── Построители простых виджетов ──────────────────────────────────────────

// buildXAMLImage строит <Image Source="…">.
//
// Раньше путь уходил в SetSource как есть: относительный резолвился от
// текущего каталога процесса (а не от XAML-файла — несовпадение с иконками
// кнопок и SVGIcon), абсолютный читал что угодно. Теперь путь проходит через
// decodeXAMLImage — тот же резолв и та же граница, что у всех ресурсов
// разметки (SEC-8), плюс умеет читать из fs.FS (LoadUIFromXAMLFS, GG-7).
func buildXAMLImage(el Element, baseDir string) base.Widget {
	iw := base.NewImageWidget()
	if src := el.Attr("Source"); src != "" {
		if img, err := decodeXAMLImage(baseDir, src); err == nil {
			iw.SetImage(img)
		} else {
			log.Printf("xaml: Image Source=%q: %v", src, err)
		}
		iw.Source = src
	}
	switch strings.ToLower(el.Attr("Stretch")) {
	case "uniform", "uniformtofill":
		iw.Stretch = base.ImageStretchUniform
	case "none":
		iw.Stretch = base.ImageStretchNone
	default:
		iw.Stretch = base.ImageStretchFill
	}
	return iw
}

func buildXAMLLabel(el Element) base.Widget {
	text := el.Attr("Text", "Content")
	if text == "" {
		text = el.Text
	}

	fg := el.Attr("Foreground", "TextColor", "Fill")
	bg := el.Attr("Background")

	var lbl *base.Label
	if fg != "" {
		if c, err := parseXAMLColor(fg); err == nil {
			lbl = base.NewLabel(text, c)
		}
	}
	if lbl == nil {
		lbl = base.NewWin10Label(text)
	}

	if bg != "" {
		if c, err := parseXAMLColor(bg); err == nil && c.A > 0 {
			lbl.HasBG = true
			lbl.Background = c
		}
	}

	// TextWrapping="Wrap" или TextWrapping="WrapWithOverflow"
	wrap := strings.ToLower(el.Attr("TextWrapping"))
	if wrap == "wrap" || wrap == "wrapwithoverflow" {
		lbl.WrapText = true
	}

	// FontSize
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			lbl.FontSize = v
		}
	}

	// FontFamily — именованный шрифт (зарегистрированный через RegisterFont)
	if ff := el.Attr("FontFamily"); ff != "" {
		lbl.FontName = ff
	}

	// FontWeight / FontStyle / TextDecorations (P1)
	if fw := strings.ToLower(el.Attr("FontWeight")); fw == "bold" || fw == "semibold" || fw == "black" || fw == "heavy" {
		lbl.Bold = true
	}
	if fs := strings.ToLower(el.Attr("FontStyle")); fs == "italic" || fs == "oblique" {
		lbl.Italic = true
	}
	if td := strings.ToLower(el.Attr("TextDecorations")); strings.Contains(td, "underline") {
		lbl.Underline = true
	}

	// Padding
	if pad := el.Attr("Padding"); pad != "" {
		parts := strings.Split(pad, ",")
		switch len(parts) {
		case 1:
			v := ParseInt(strings.TrimSpace(parts[0]))
			lbl.PaddingX = v
			lbl.PaddingY = v
		case 2:
			lbl.PaddingX = ParseInt(strings.TrimSpace(parts[0]))
			lbl.PaddingY = ParseInt(strings.TrimSpace(parts[1]))
		case 4:
			lbl.PaddingX = ParseInt(strings.TrimSpace(parts[0]))
			lbl.PaddingY = ParseInt(strings.TrimSpace(parts[1]))
		}
	}

	return lbl
}

func buildXAMLButton(el Element, baseDir string) base.Widget {
	text := el.Attr("Content", "Text")
	if text == "" {
		text = el.Text
	}

	// ── WPF Content Model: парсим вложенные элементы ───────────────────────
	// <Button> может содержать <StackPanel> с <Image> + <TextBlock>,
	// или напрямую <Image> / <TextBlock> как дочерние элементы.
	bc := extractButtonContent(el)
	if text == "" {
		text = bc.Text
	}
	// Иконка из атрибута имеет приоритет
	iconSrc := el.Attr("Icon", "IconSource")
	if iconSrc == "" {
		iconSrc = bc.IconSrc
	}
	style := strings.ToLower(el.Attr("Tag", "Style"))
	var btn *base.Button
	if style == "accent" || style == "primary" {
		btn = base.NewWin10AccentButton(text)
	} else {
		btn = base.NewButton(text)
	}

	ApplyColor(&btn.Background, el, "Background")
	ApplyColor(&btn.TextColor, el, "Foreground")
	ApplyColor(&btn.HoverBG, el, "HoverBG", "HoverBackground")
	ApplyColor(&btn.PressedBG, el, "PressedBG", "PressedBackground")
	ApplyColor(&btn.BorderColor, el, "BorderBrush")

	// FontSize — свой кегль виджета (0 = общий размер).
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			btn.FontSize = v
		}
	}

	// ── ToolTip ────────────────────────────────────────────────────────────
	if tip := el.Attr("ToolTip"); tip != "" {
		btn.ToolTip = tip
	}

	// ── Начальное состояние переключателя ──────────────────────────────────
	// Читается и у обычной кнопки: <Button IsChecked="True"> означает
	// «нарисуй выбранным», а поведение переключателя даёт сам тег
	// <ToggleButton>.
	if strings.EqualFold(el.Attr("IsChecked", "Checked"), "true") {
		btn.SetChecked(true)
	}

	// ── Padding ────────────────────────────────────────────────────────────
	if pad := el.Attr("Padding"); pad != "" {
		btn.Padding = parseMargin(pad)
	}

	// ── CornerRadius ───────────────────────────────────────────────────────
	if cr := ParseInt(el.Attr("CornerRadius")); cr > 0 {
		btn.CornerRadius = cr
	}

	// ── Иконка ─────────────────────────────────────────────────────────────
	// Путь ограничен каталогом XAML-файла или fs.FS (SEC-8, GG-7) —
	// decodeXAMLImage сама решает, откуда читать (см. OpenResource).
	if iconSrc != "" {
		if img, err := decodeXAMLImage(baseDir, iconSrc); err == nil {
			btn.Icon = img
			btn.IconPath = iconSrc
		}
	}
	// Размер иконки из вложенного <Image Width="..." Height="...">
	if btn.IconSize <= 0 && bc.IconW > 0 {
		btn.IconSize = bc.IconW
	}
	if btn.IconSize <= 0 && bc.IconH > 0 {
		btn.IconSize = bc.IconH
	}
	// Foreground из вложенного TextBlock (если не задан на самой кнопке)
	if bc.Foreground != "" && el.Attr("Foreground") == "" {
		if c, err := parseXAMLColor(bc.Foreground); err == nil {
			btn.TextColor = c
		}
	}

	switch strings.ToLower(el.Attr("IconPosition", "IconPos")) {
	case "top":
		btn.IconPos = base.IconTop
	case "only", "icononly":
		btn.IconPos = base.IconOnly
	}
	if sz := ParseInt(el.Attr("IconSize")); sz > 0 {
		btn.IconSize = sz
	}

	return btn
}

// buttonContent содержит информацию, извлечённую из дочерних элементов Button.
type buttonContent struct {
	Text       string // текст из TextBlock/Label
	IconSrc    string // Source из Image
	IconW      int    // Width из Image
	IconH      int    // Height из Image
	Foreground string // Foreground из TextBlock (для цвета текста кнопки)
}

// extractButtonContent рекурсивно обходит дочерние элементы Button,
// извлекая текст из TextBlock и путь к иконке из Image.
// Поддерживает WPF Content Model: <Button> → <StackPanel> → <Image> + <TextBlock>.
func extractButtonContent(el Element) buttonContent {
	var bc buttonContent
	extractButtonContentRec(el, &bc)
	return bc
}

func extractButtonContentRec(el Element, bc *buttonContent) {
	for _, child := range el.Children {
		tag := strings.ToLower(child.Tag)
		if strings.Contains(tag, ".") {
			continue
		}
		switch tag {
		case "textblock", "label", "run":
			if bc.Text == "" {
				if t := child.Attr("Text", "Content"); t != "" {
					bc.Text = t
				} else if child.Text != "" {
					bc.Text = child.Text
				}
			}
			if bc.Foreground == "" {
				bc.Foreground = child.Attr("Foreground")
			}
		case "image":
			if bc.IconSrc == "" {
				if src := child.Attr("Source", "Src"); src != "" {
					bc.IconSrc = src
				}
			}
			if bc.IconW == 0 {
				bc.IconW = ParseInt(child.Attr("Width"))
			}
			if bc.IconH == 0 {
				bc.IconH = ParseInt(child.Attr("Height"))
			}
		case "stackpanel", "wrappanel", "dockpanel", "grid":
			extractButtonContentRec(child, bc)
		}
	}
}

func buildXAMLDropdown(el Element) base.Widget {
	var items []string
	var keys []string // ключи {Loc …} по индексам items (пустая строка — нет)
	if raw := el.Attr("Items", "ItemsSource"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if s := strings.TrimSpace(item); s != "" {
				text, key := locItemText(s)
				items = append(items, text)
				keys = append(keys, key)
			}
		}
	}
	for _, child := range el.Children {
		t := strings.ToLower(child.Tag)
		if t == "item" || t == "comboboxitem" || t == "listboxitem" {
			v := child.Attr("Content", "Value")
			if v == "" {
				v = strings.TrimSpace(child.Text)
			}
			if v != "" {
				text, key := locItemText(v)
				items = append(items, text)
				keys = append(keys, key)
			}
		}
	}

	dd := base.NewDropdown(items...)
	// Элементы списка — не виджеты, поэтому обновляем их сами: при смене
	// языка перечитываем переводы и переустанавливаем весь список.
	registerLocItemList(keys, func(i int, s string) {
		cur := dd.Items()
		if i < len(cur) {
			cur[i] = s
			dd.SetItems(cur)
		}
	})
	if sel := el.Attr("SelectedIndex", "Selected"); sel != "" {
		if idx, err := strconv.Atoi(sel); err == nil {
			dd.SetSelected(idx)
		}
	}
	// FontSize — свой кегль виджета (0 = общий размер).
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			dd.FontSize = v
		}
	}
	// ArrowStyle="Chevron" | "Triangle" | "Auto" — вид стрелки раскрытия.
	switch strings.ToLower(el.Attr("ArrowStyle")) {
	case "chevron":
		dd.ArrowStyle = base.ArrowChevron
	case "triangle":
		dd.ArrowStyle = base.ArrowTriangle
	}

	return dd
}

func buildXAMLProgressBar(el Element) base.Widget {
	pb := base.NewProgressBar()
	ApplyColor(&pb.FillColor, el, "Foreground", "Fill")
	if val := el.Attr("Value"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			pb.SetValue(v)
		}
	}
	// Style: Bar (по умолчанию) | Glow — светящаяся голова со следом.
	if strings.EqualFold(el.Attr("Style"), "glow") {
		pb.Style = base.ProgressStyleGlow
	}
	if ind := el.Attr("Indeterminate"); strings.EqualFold(ind, "true") || ind == "1" {
		pb.SetIndeterminate(true)
	}
	ApplyColor(&pb.GlowTail, el, "GlowTail")
	ApplyColor(&pb.GlowHead, el, "GlowHead")
	return pb
}

// buildXAMLSeparator строит разделитель (widget.Separator).
//
// Раньше здесь собиралась панель с намертво зашитым цветом #4C4C4C — тот самый
// обход «панель в один пиксель», только внутри движка: при смене темы линия
// оставалась прежней. Теперь это настоящий виджет, и цвет берётся из темы.
//
// Ориентация выводится из размеров, как это делает WPF: заданная ширина без
// высоты означает горизонтальную линию, высота без ширины — вертикальную.
// Явный атрибут Orientation сильнее.
func buildXAMLSeparator(el Element) base.Widget {
	s := base.NewSeparator()

	w := ParseInt(el.Attr("Width"))
	h := ParseInt(el.Attr("Height"))
	switch {
	case strings.EqualFold(el.Attr("Orientation"), "vertical"):
		s.Orientation = base.SeparatorVertical
	case strings.EqualFold(el.Attr("Orientation"), "horizontal"):
		s.Orientation = base.SeparatorHorizontal
	case h > 0 && w <= 0:
		s.Orientation = base.SeparatorVertical
	}

	if bg := el.Attr("Background", "Fill", "Stroke"); bg != "" {
		if parsed, err := parseXAMLColor(bg); err == nil {
			s.Color = parsed
		}
	}
	if t := ParseInt(el.Attr("Thickness")); t > 0 {
		s.Thickness = t
	}
	if m := ParseInt(el.Attr("Margin")); m > 0 {
		s.Margin = m
	}

	// Размеры по умолчанию: линия занимает всю доступную длину, а толщину
	// берёт свою. Нули оставляем контейнеру — он сам растянет.
	if w <= 0 && h <= 0 {
		if s.Orientation == base.SeparatorVertical {
			h = 24
		} else {
			w = 24
		}
	}
	th := s.Thickness // 0 — один пиксель (Separator.Thickness)
	if th <= 0 {
		th = 1
	}
	if w <= 0 {
		w = th
	}
	if h <= 0 {
		h = th
	}
	s.SetBounds(image.Rect(0, 0, w, h))
	return s
}

func buildXAMLCheckBox(el Element) base.Widget {
	text := el.Attr("Content", "Text")
	if text == "" {
		text = el.Text
	}
	cb := base.NewCheckBox(text)
	if strings.EqualFold(el.Attr("IsChecked", "Checked"), "true") {
		cb.SetChecked(true)
	}
	// FontSize — свой кегль виджета (0 = общий размер).
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			cb.FontSize = v
		}
	}

	return cb
}

func buildXAMLRadioButton(el Element) base.Widget {
	text := el.Attr("Content", "Text")
	if text == "" {
		text = el.Text
	}
	group := el.Attr("GroupName", "Group")
	rb := base.NewRadioButton(text, group)
	if strings.EqualFold(el.Attr("IsChecked", "Checked", "Selected"), "true") {
		rb.SetSelected(true)
	}
	return rb
}

func buildXAMLSlider(el Element) base.Widget {
	s := base.NewSlider()
	if min := el.Attr("Minimum", "Min"); min != "" {
		if v, err := strconv.ParseFloat(min, 64); err == nil {
			s.Min = v
		}
	}
	if max := el.Attr("Maximum", "Max"); max != "" {
		if v, err := strconv.ParseFloat(max, 64); err == nil {
			s.Max = v
		}
	}
	if val := el.Attr("Value"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			s.SetValue(v)
		}
	}
	return s
}

func buildXAMLVirtualizingItemsControl(el Element) base.Widget {
	v := base.NewVirtualizingItemsControl()
	if ih := el.Attr("ItemHeight"); ih != "" {
		if n := ParseInt(ih); n > 0 {
			v.ItemHeight = n
		}
	}
	if bf := el.Attr("Buffer"); bf != "" {
		if n := ParseInt(bf); n >= 0 {
			v.Buffer = n
		}
	}
	if strings.EqualFold(el.Attr("ShowBorder"), "false") {
		v.ShowBorder = false
	}
	ApplyColor(&v.Background, el, "Background")
	return v
}

func buildXAMLNumericUpDown(el Element) base.Widget {
	n := base.NewNumericUpDown()
	if min := el.Attr("Minimum", "Min"); min != "" {
		if v, err := strconv.ParseFloat(min, 64); err == nil {
			n.Min = v
		}
	}
	if max := el.Attr("Maximum", "Max"); max != "" {
		if v, err := strconv.ParseFloat(max, 64); err == nil {
			n.Max = v
		}
	}
	if inc := el.Attr("Increment", "Step"); inc != "" {
		if v, err := strconv.ParseFloat(inc, 64); err == nil {
			n.Step = v
		}
	}
	// Decimals напрямую, либо вывод из FormatString вида "F2".
	if d := el.Attr("Decimals", "DecimalPlaces"); d != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(d)); err == nil && v >= 0 {
			n.Decimals = v
		}
	} else if fs := el.Attr("FormatString", "StringFormat"); len(fs) >= 2 {
		if c := fs[0]; c == 'F' || c == 'f' || c == 'N' || c == 'n' {
			if v, err := strconv.Atoi(fs[1:]); err == nil && v >= 0 {
				n.Decimals = v
			}
		}
	}
	if val := el.Attr("Value"); val != "" {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			n.SetValue(v)
		}
	}
	return n
}

func buildXAMLToggleSwitch(el Element) base.Widget {
	text := el.Attr("Content", "Text", "Header")
	if text == "" {
		text = el.Text
	}
	ts := base.NewToggleSwitch(text)
	if strings.EqualFold(el.Attr("IsOn", "IsChecked", "Checked"), "true") {
		ts.SetOn(true)
	}
	return ts
}

func buildXAMLScrollView(el Element) base.Widget {
	sv := base.NewScrollView()
	if ch := el.Attr("ContentHeight"); ch != "" {
		sv.ContentHeight = ParseInt(ch)
	}
	ApplyColor(&sv.Background, el, "Background")
	return sv
}

func buildXAMLListView(el Element) base.Widget {
	var items []string
	// keys[i] — ключ {Loc …} для i-й строки (пустой, если строка не
	// локализуемая): строки списка не виджеты, поэтому перевод и подписку на
	// смену языка делает сборщик (см. loc_items.go).
	var keys []string
	add := func(raw string) {
		text, key := locItemText(raw)
		items = append(items, text)
		keys = append(keys, key)
	}
	if raw := el.Attr("Items", "ItemsSource"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if s := strings.TrimSpace(item); s != "" {
				add(s)
			}
		}
	}
	for _, child := range el.Children {
		t := strings.ToLower(child.Tag)
		switch {
		case t == "item" || t == "listviewitem" || t == "listboxitem":
			v := child.Attr("Content", "Value")
			if v == "" {
				v = strings.TrimSpace(child.Text)
			}
			if v != "" {
				add(v)
			}
		case t == "textblock" || t == "label" || t == "run":
			// WPF: <ListView><TextBlock>text</TextBlock></ListView>
			v := child.Attr("Text")
			if v == "" {
				v = strings.TrimSpace(child.Text)
			}
			if v != "" {
				add(v)
			}
		}
	}
	lv := base.NewListView(items...)
	registerLocItemList(keys, func(i int, s string) {
		cur := lv.Items()
		if i < len(cur) {
			cur[i] = s
			lv.SetItems(cur)
		}
	})

	ApplyColor(&lv.Background, el, "Background")

	if sel := el.Attr("SelectedIndex", "Selected"); sel != "" {
		if idx, err := strconv.Atoi(sel); err == nil {
			lv.SetSelectedQuiet(idx) // начальное выделение — не выбор пользователя
		}
	}
	if strings.EqualFold(el.Attr("Reorderable"), "true") {
		lv.Reorderable = true // строки переставляются перетаскиванием (GG-85)
	}
	if ih := el.Attr("ItemHeight"); ih != "" {
		if v := ParseInt(ih); v > 0 {
			lv.ItemHeight = v
		}
	}
	return lv
}

// buildXAMLSplitPanel строит SplitPanel: две панели с перетаскиваемым
// разделителем. Первый дочерний элемент — First, второй — Second.
func buildXAMLSplitPanel(el Element, reg map[string]base.Widget, parentOff image.Point, baseDir string, depth int) (base.Widget, error) {
	orient := base.OrientationHorizontal
	if strings.EqualFold(el.Attr("Orientation"), "vertical") {
		orient = base.OrientationVertical
	}
	sp := base.NewSplitPanel(orient)

	if fs := el.Attr("Position"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v >= 0 && v <= 1 {
			sp.Position = v
		}
	}
	if v := ParseInt(el.Attr("SplitterSize")); v > 0 {
		sp.SplitterSize = v
	}
	if v := ParseInt(el.Attr("MinFirst")); v > 0 {
		sp.MinFirst = v
	}
	if v := ParseInt(el.Attr("MinSecond")); v > 0 {
		sp.MinSecond = v
	}
	ApplyColor(&sp.Background, el, "Background")
	ApplyColor(&sp.HoverColor, el, "HoverColor")

	absBounds := el.Bounds().Add(parentOff)
	sp.SetBounds(absBounds)
	applyCommonProps(sp, el)
	if id := el.Name(); id != "" {
		reg[id] = sp
	}

	// Дочерние виджеты: layout SplitPanel сам расставит первых двух.
	for _, child := range el.Children {
		childTag := strings.ToLower(child.Tag)
		if strings.Contains(childTag, ".") {
			continue
		}
		cw, err := buildXAMLWidgetAt(child, reg, image.Point{}, baseDir, depth+1)
		if err != nil {
			return nil, err
		}
		if cw != nil {
			sp.AddChild(cw)
		}
	}
	return sp, nil
}

// buildXAMLSVGIcon строит темизируемую векторную иконку из .svg-файла.
// Без явного Color иконка перекрашивается под цвет текста темы.
func buildXAMLSVGIcon(el Element, baseDir string) base.Widget {
	ic := base.NewSVGIcon()
	if src := el.Attr("Source"); src != "" {
		// Граница по baseDir/fs.FS — SEC-8, GG-7 (см. readXAMLResourceBytes).
		// Читаем байты сами (а не ic.SetSVGFile) — SetSVGFile умеет только диск.
		data, err := readXAMLResourceBytes(baseDir, src, svgPkg.MaxFileBytes)
		if err == nil {
			err = ic.SetSVG(data)
		}
		if err != nil {
			log.Printf("xaml: SVGIcon Source=%q: %v", src, err)
		}
	}
	var c color.RGBA
	ApplyColor(&c, el, "Color", "Foreground", "Fill")
	if c.A > 0 {
		ic.SetColor(c)
	}
	if strings.EqualFold(el.Attr("Tint"), "true") {
		ic.SetTint(true)
	}
	return ic
}
