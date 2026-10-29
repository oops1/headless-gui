# ADR-0012. `widget` → подпакеты: ядро вниз, `widget` — фасад

Статус: принято 2026-10-29, задача 8.1 (этап 8), ветка
`feature/engine-architecture`. Реализация — 8.2–8.8 по порядку из раздела
«План». Основание — план этапа 8 и обзор (`docs/ARCHITECTURE_REVIEW_RU.md`,
§4.3: `widget/xaml`, `widget/richtext`, `widget/text`, `widget/dialog`,
`widget/dock`, `widget/chrome`, `platform/clipboard`).

## Контекст

Пакет `widget` сегодня: 219 файлов на диске (214 собираются под Windows),
72 559 строк без тестов, 97 файлов
внутренних тестов (`package widget`; внешних тестов `widget_test` нет).
Экспортировано 942 имени верхнего уровня: 300 функций, 261 тип, 358
констант, 23 переменные (ошибки и кривые анимации), — и 1334 метода;
дженериков нет. Его
импортируют 22 пакета репозитория (`engine`, `window`, `desktop`,
`core/input`, `core/overlay`, `core/raster`, `output/webstream`, `tests`,
`cmd/*`, `internal/testreset`) и внешние программы (Tandem). Подпакеты
`widget/{svg,datagrid,diffview,mergeview,treeview}` уже есть и `widget` не
импортируют.

### Карта зависимостей

Карту строит `internal/depmap` по типам (`go/types`, импортёр из
исходников): обращение к методу, полю, типу или функции другого файла
находится точно. Файлы разбиты на группы — будущие подпакеты — по именам;
остальное — ядро (`core`). Тест `widget/depmap_test.go` держит карту в
репозитории: он падает на новом ребре между группами, которого нет в
целевом порядке и в базе сегодняшних нарушений, и сообщает, когда базовое
ребро исчезло.

| Группа | Файлы | Строк | Что внутри |
|---|---|---|---|
| core | 144 | 40 087 | `Base`, `Widget`, контекст, события, темы, раскладки, контролы, DiffView/MergeView/DataGrid/TreeView, доступность, навигация, JSON-загрузчик |
| xaml | 10 | 7 096 | парсер, свойства, биндинг, контейнеры, ресурсы, шаблоны, реестр тегов |
| richtext | 11 | 5 933 | `RichText`: модель, раскладка, каретка, HTML, IME, XAML-строитель |
| chrome | 9 | 4 638 | `Window`: рамка, заголовок, вкладки, материалы |
| text | 8 | 4 302 | `TextBox`, `TextInput`, IME-композиция |
| dialog | 16 | 3 941 | `Dialog`, `MessageBox`, файловый диалог, ввод, прогресс |
| dock | 6 | 3 750 | `DockManager`, `DockPane`, `DockPanel`, направляющие |
| clipboard | 10 | 2 671 | буфер обмена: текст, HTML, картинки, файлы; syscall по ОС |

Файлы и строки — те, что собираются под Windows (`go/build`); карта
строится по ним же.

Рёбра между группами (обращений / идентификаторов):

| Откуда → куда | Обращений | Главное |
|---|---|---|
| dialog → core | 590 / 289 | `Base`, `Label`, события, темы |
| dock → core | 554 / 156 | `DockSide` и стороны, `Base`, события |
| chrome → core | 475 / 202 | `Base`, темы, `TabItem`, события |
| text → core | 336 / 175 | `Base`, `PopupMenu`, темы, события |
| richtext → core | 270 / 157 | `Base`, метрики шрифта, события |
| xaml → core | 758 / 431 | строители всех базовых виджетов |
| **core → xaml** | 110 / 40 | строители фигур, градиента, растяжения, кнопки меню, `*_xaml.go` листовых виджетов; `resolveXAMLResource`, `DecodeImageBounded`, `collectionItems` из не-XAML кода |
| **core → chrome** | 84 / 37 | `mixRGBA`, `fillCircle`, `brighten`; `WindowTitleStyle` у `Panel`; `navhost.go` читает внутренности `Window`; `accessibility.go`, `canvas.go` |
| **core → text** | 83 / 43 | `accesstext.go` читает поля `TextInput`/`TextBox`; `premulAlpha`; `base.go` подбирает размеры по типу; `loader.go` |
| **core → dialog** | 26 / 12 | `navhost.go` читает внутренности `Dialog`; `ellipsizeText` у кнопок и списков; `canvas.go` |
| **core → dock** | 8 / 8 | списки контейнеров в `canvas.go`; `bgrole.go` (метод `DockPanel`); `accessibility.go` |
| **core → richtext** | 2 / 1 | `clampInt` |
| core → clipboard | 5 / 4 | `ClipboardGetText`/`SetText` из DiffView/MergeView — провайдер остаётся в ядре |
| **chrome → dialog** | 78 / 18 | `titlebar.go` и `dialog_chrome.go` держат методы и `Window`, и `Dialog`; `ellipsizeText` |
| dialog → chrome | 14 / 7 | `titleNavBtn`, методы `Dialog` из `titlebar.go`, поля `Window` из `dialog_chrome.go` |
| **chrome → dock** | 3 / 2 | `window_tabs.go` смотрит `DockPanel.Background` по типу |
| **dock → chrome** | 32 / 8 | `winEdge` и стороны, `winResizeBorder`, `mixRGBA` |
| **dock → dialog** | 1 / 1 | `ellipsizeText` |
| richtext → text | 9 / 4 | `isWordRune`, `caretPhaseAt`, `premulAlpha` |
| richtext → xaml | 45 / 8 | XAML-строитель RichText |
| dialog → text | 16 / 10 | `TextInput` в диалоге ввода |
| **xaml → chrome/dock/text/richtext/dialog** | 55 / 29 / 54 / 2 / 2 | строители `Window`, `DockPanel`/`DockManager`/`DockPane`, `TextBox`/`TextInput`, `RichText`, диалога |
| text, richtext → clipboard | 14 / 6 | текст и HTML буфера |

Жирным — рёбра, которых в целевом графе нет; этап их разрывает.

### Противоречие в плане

План 8.8 оставляет в `widget` алиасы типов подпакетов, а подпакеты, по
карте, не могут не импортировать ядро: `Base`, `Widget`, `DrawContext`,
события, темы — по 270–590 обращений у каждого. Алиас `widget.Dialog =
dialog.Dialog` при `dialog → widget` — цикл импорта, который Go запрещает.
Вторая сторона того же: методы типа объявляются только в пакете типа, а
`titlebar.go` и `dialog_chrome.go` держат методы и `Window`, и `Dialog`.

Значит, либо старые имена `widget.Dialog`, `widget.TextBox`,
`widget.Window`, `widget.RichText`, `widget.DockManager` и
`widget.LoadUIFromXAMLFile` исчезают (программы правятся — недопустимо),
либо ядро уходит ниже фасада.

## Решение

### 1. Ядро — в `widget/base`, `widget` — фасад

Весь сегодняшний код `widget` переезжает в новый пакет `widget/base` одним
шагом (8.2a), вместе с внутренними тестами и файлами с тегами сборки.
`widget` становится фасадом без логики: для каждого экспортированного имени
`base` и каждого подпакета — алиас типа (`type Button = base.Button`),
обёртка функции (`func NewButton(text string) *Button { return
base.NewButton(text) }`, с копией документации), повторное объявление
константы (`const DockLeft = base.DockLeft`) и переменной (`var
ErrXAMLTooDeep = base.ErrXAMLTooDeep`). Фасад генерирует инструмент
`internal/facadegen` (`go run ./internal/facadegen/cmd/facadegen`, файлы
`widget/<пакет>_gen.go`); он перегенерируется на каждом шаге этапа и
проверяется тестом: файлы совпадают с выводом генератора, все
экспортированные имена `base` и подпакетов присутствуют в `widget` тем же
видом, лишних нет, и по типам они тождественны (`go/types`: алиас — тот же
тип, сигнатуры `Identical`, у констант тип и значение). Генератор читает
синтаксис (`go/ast`), а не типы: все файлы пакета, кроме тестов, независимо
от тегов сборки, — вывод один на всех ОС и не требует проверки типов
зависимостей.

Исключение — служебные имена, которые `base` экспортирует только для
соседних подпакетов (с 8.2b: `NotifyUIChanged`, `DesiredOf`, `ParseKeyName`,
`OpenResource` и др.): в документации у них директива `//facadegen:skip`,
и в фасад они не идут — открытый API `widget` от разбиения не растёт. Их
тестам неэкспортированное состояние `base` отдаёт `internal/testhook`, как
уже отдавал сброс языка.

Почему это сохраняет программы без правок. Алиас — тот же тип: методы,
встраивание (`struct{ widget.Base }`), утверждения типов
(`w.(*widget.Button)`), удовлетворение интерфейсов, `reflect.Type.Name()`
— всё прежнее. Константы с алиасным типом остаются типизированными,
переменные-ошибки — теми же значениями (`errors.Is` работает). Обёртки
функций встраиваются компилятором.

Что меняется и что это значит. `reflect.Type.String()` отдаёт `base.Button`
вместо `widget.Button` — в пакете по имени пакета ничего не ищется
(проверено: ни `PkgPath`, ни `String()` типов в XAML, темах и
доступности). Трассы стека и `go doc` показывают `widget/base`; у фасада
документация скопирована в обёртки. Внутренние тесты `widget` не могут
импортировать `internal/testreset` (он импортирует `widget`) — так и
сегодня, помощники у них свои.

Отвергнуто. (а) Ядро остаётся в `widget`, подпакеты импортируют его:
старые имена исчезают. (б) Хук, который `engine` ставит через `init`
подпакета: годится только для функций (XAML), не для типов, и пришивает
XAML к каждому приложению. (в) `xaml` без зависимости от ядра, через
интерфейсы: 758 обращений к ядру, строители всех базовых виджетов, —
интерфейсный слой был бы вторым пакетом `widget`.

### 2. Порядок пакетов

```
widget/base      ядро: Widget, Base, DrawContext, события, темы, раскладки,
                 контролы, DiffView/MergeView/DataGrid/TreeView, доступность,
                 ClipboardProvider
widget/text      → base                      TextBox, TextInput, IME-композиция
widget/richtext  → base, text, xaml          RichText
widget/chrome    → base, xaml                Window
widget/dialog    → base, chrome, text, xaml  Dialog, MessageBox, файловые диалоги
widget/dock      → base, xaml                DockManager, DockPane, DockPanel
widget/xaml      → base                      парсер, свойства, биндинг, реестр, загрузчик,
                                             строители базовых виджетов
platform/clipboard → base                    буфер обмена по ОС (8.7)
widget (фасад)   → все выше                  алиасы, обёртки, LoadUIFromXAML*,
                                             JSON-загрузчик, NavHost
```

Подпакеты импортируют `xaml` только ради регистрации своих строителей;
`xaml` не знает ни одного подпакета. `engine`, `window`, `desktop`,
`core/*` и программы продолжают импортировать `widget`; новые программы,
которым нужна тонкая сборка, импортируют `widget/base` и нужные подпакеты
напрямую (8.9 меряет именно это).

### 3. Разрыв циклов

| Ребро | Причина | Приём | Куда |
|---|---|---|---|
| core → chrome, dock, dialog, richtext, text | помощники `mixRGBA`, `fillCircle`, `brighten`, `ellipsizeText`, `clampInt`, `premulAlpha`, `isWordRune`, `caretPhaseAt` объявлены в файлах будущих подпакетов | перенос файлов-помощников | `base/colorutil.go`, `base/textutil.go` |
| core → chrome | `Panel` и платформенное умолчание читают `WindowTitleStyle`, его константы, `captionGlyphColor`, `titleBarGap` | перенос типа и констант | `base/titlestyle.go` |
| core → chrome, dialog | `navhost.go` читает внутренности `Window` и `Dialog` (`navPanel`, `titleH`, `titleBarRect`, `titleColors`, `frameW`, `macTitleBar`, `titleContentLeft/Right`, `titleTabsActive`) | интерфейс с экспортированными методами в `base`: `NavTitleHost` (геометрия заголовка, цвета, установка панели); `Window` и `Dialog` реализуют его в своих пакетах; `NavHost` живёт в фасаде | `base`, `chrome`, `dialog`, `widget` |
| core → chrome, dock, dialog | `canvas.go` (и копия списка в `dockmanager.go`) перечисляет контейнеры, чей `SetBounds` сам двигает детей | интерфейс-возможность в `base`: `ChildLayouter` (`LaysOutChildren() bool`); контейнеры реализуют; `Canvas` спрашивает интерфейс | `base` + каждый контейнер |
| core → text | `accesstext.go`: методы `TextInput`/`TextBox` читают их неэкспортированные поля | интерфейс `AccessTextProvider` и помощники (`AccessTextOf`, `AccessRuneLen`, `AccessSubstring`) остаются в `base`; методы уезжают с типами | `text` |
| core → chrome, text, dock | `accessibility.go` выбирает роль и значение по конкретному типу (`Window`, `TextBox`, `TextInput`, `DockPanel`) | интерфейс-возможность `AccessDescriber` в `base` (роль, имя, значение, состояния); виджеты подпакетов реализуют его, как уже делают `dialog_access.go`, `menubar_access.go`, `popupmenu_access.go` | `base` + подпакеты |
| core → text | `base.go` подбирает размер по умолчанию по типу (`case *TextInput`) | интерфейс `DefaultSizer` (`DefaultSize() (w, h int)`) в `base`, `TextInput` его реализует; не `DesiredSize`: `DesiredOf` ставит его выше размера из разметки, и поле с `Width` в XAML сжалось бы до 120 (с 8.3) | `base`, `text` |
| core → text | `loader.go` (JSON-загрузчик) строит `TextInput` | загрузчик — верхний уровень, остаётся в фасаде | `widget` |
| core → dock | `bgrole.go` объявляет метод `DockPanel.SetBackgroundRole` | метод уезжает с типом; `BackgroundRole` и `ThemeBackground` (служебная с 8.6) остаются | `dock` |
| chrome → dock | `window_tabs.go` смотрит `Background` у `Panel`, `Canvas`, `Grid`, `DockPanel` по типу | интерфейс `BackgroundColorer` (`BackgroundColor() (color.RGBA, bool)`; имя не совпадает с полем `Background`) в `base`; четыре типа реализуют | `base`, `dock` |
| dock → chrome | `winEdge`, его стороны и `winResizeBorder` нужны и окну, и панели дока | экспортированный тип `ResizeEdge` с константами и `ResizeBorder` в `base` | `base/resizeedge.go` |
| chrome ⇄ dialog | `titlebar.go` и `dialog_chrome.go` держат методы обоих типов; `titleNavBtn` общий | файлы делятся по типу приёмника: методы `Dialog` — в `dialog`, методы `Window` — в `chrome`; `titleNavBtn` — экспортированный `chrome.TitleNavButton`; `dlgPad` остаётся в `dialog` | `chrome`, `dialog` |
| core → xaml | строители XAML базовых виджетов лежат в их файлах (`shapes.go`, `gradient.go`, `stretch.go`, `menubutton.go`, `*_xaml.go` цветового поля, даты, DiffView, MergeView) | строители переезжают в `xaml` (`xaml` → `base`), виджеты остаются | `xaml` |
| core → xaml | не-XAML код пользуется `resolveXAMLResource`/`FS`, `DecodeImageBounded`, `collectionItems`, `ErrResourceOutsideBase` (`themeicons.go`, `imagewidget.go`, `collectionview.go`) | реестр файловых систем ресурсов и ограниченный декодер картинок — не про XAML: в `base/resources.go` | `base` |
| xaml → chrome, dock, text, richtext, dialog | строители `Window`, `DockPanel`/`DockManager`/`DockPane`, `TextBox`/`TextInput`, `RichText`, диалога внутри `xaml` | строители переезжают в пакеты своих виджетов и регистрируются в реестре `xaml` (§4) | подпакеты |
| core → clipboard; text, richtext → clipboard | `ClipboardGetText`/`SetText`/`ClipboardHTML` и провайдеры по ОС в одном пакете | `ClipboardProvider`, `SetClipboardProvider` и функции-обёртки — в `base`; syscall-реализации — `platform/clipboard` с `init`; его импортирует фасад `widget` (`window` — через фасад; как сегодня, буфер есть у всех программ) | `base`, `platform/clipboard` |

Список интерфейсов, которые появляются в `base`: `NavTitleHost`,
`ChildLayouter`, `AccessDescriber`, `BackgroundColorer`,
`ClipboardProvider` (уже есть), `AccessTextProvider` (уже есть), плюс
экспортированный `ResizeEdge`. Имена уточняются при реализации; правило
одно — у интерфейса, который реализуют из другого пакета, все методы
экспортированы. С 8.3 есть ещё `TextEditor` (поле ввода для привязки XAML,
диалогов и загрузчиков: текст, подсказка, цвета, обработчик изменения),
`DefaultSizer` и `AccessDescriber`, а `IMEComposer` переехал в `base`
(его реализует и `RichText`). Ветки по интерфейсу стоят на месте прежних веток по
типу; разница одна — тип приложения, встраивающий поле (`struct{
*widget.TextInput }`), теперь тоже их проходит: авторазмер 120×26, роль
доступности, двусторонняя привязка XAML. В репозитории таких типов нет.

С 8.5 у `Panel`, `Canvas`, `Grid`, `DockPanel` есть `BackgroundColor()`
(`BackgroundColorer`), у окна — `LaysOutChildren()` (`ChildLayouter`), у
`Canvas` и окна — `InputBindingList()`. Как и с полями ввода: тип
приложения, встраивающий такой контейнер, теперь проходит ветки по
интерфейсу (фон корешка вкладки окна, «свой SetBounds»).

Переходное (8.3–8.6, снято на 8.6): диалоги ядра строили своё поле через
`base.SetTextInputFactory`, которую задавал `widget/text` при загрузке; с
8.6 `dialog` импортирует `text` напрямую, фабрика и заглушка поля в тестах
ядра удалены.

### 4. Реестр XAML и регистрация

`xaml` экспортирует разобранный узел `Element` (тег, атрибуты, дети,
текст), атрибуты `Attrs` (сегодняшний `XAMLAttrs`), загрузчик `Loader`
(построить ребёнка, зарегистрировать имя, каталог ресурсов, область
биндинга) и строитель `Builder func(el *Element, l *Loader) (base.Widget,
error)`. Логика контейнера живёт в строителе, в пакете своего виджета:
меню окна, панели дока, вкладки — не в `xaml_containers.go`. Реестр тегов —
значение `xaml.Registry` с умолчанием `xaml.Default()`; базовые теги
(кнопка, подпись, панель, сетка, список, фигуры…) `xaml` регистрирует сам.

Регистрация из подпакетов — явной функцией, не `init`: `text.RegisterXAML(r
*xaml.Registry)`, `richtext.RegisterXAML`, `dialog.RegisterXAML`,
`dock.RegisterXAML`, `chrome.RegisterXAML`. Фасад `widget` зовёт их все при
первом `LoadUIFromXAML*` (`sync.Once`): программа, которая XAML не грузит,
не удерживает строители в бинаре (компоновщик выбрасывает недостижимое,
`init` он выбросить не может), а программа на `widget/base` + `widget/xaml`
регистрирует ровно те пакеты, что импортировала. План формулировал
«регистрация через `init`»; это ADR её меняет на явную по названной
причине. Исключение — `platform/clipboard`: у него нет иного назначения,
кроме регистрации провайдера, и его импортируют ради неё.

Пользовательские теги: `widget.RegisterXAMLWidget(tag, func(XAMLAttrs)
(Widget, error))` остаётся обёрткой над реестром с алиасами типов.

### 5. План реализации (замена 8.2–8.8)

| Шаг | Что | Приёмка |
|---|---|---|
| 8.2a | `widget/base`: перенос всего кода и тестов; `internal/facadegen`; фасад `widget`; тест полноты фасада | `go test ./...`; showcase, webshowcase, desktopdemo и Tandem без правок; `go vet`; матрица |
| 8.2b | `widget/xaml`: парсер, реестр, загрузчик, биндинг, строители всех виджетов (и тех, что пока в `base`); не-XAML помощники в `base/resources.go` | то же; тест: программа без XAML не содержит парсера (`go tool nm`) |
| 8.3 | `widget/text`: `TextBox`, `TextInput`, `ime.go`; `accesstext` методы с типами; `DefaultSizer` вместо ветки по типу; помощники в `base/textutil.go`; реестр §4 (`xaml.Registry`, `Element`, `Loader`, `Builder`) — первый строитель вне `xaml`; `text.RegisterXAML`; `LoadUIFromXAML*` в фасаде с регистрацией по первому вызову (генератор: `Config.Prelude`); JSON-загрузчик — в фасаде | то же; `depmap`: `core → text` и `xaml → text` исчезли |
| 8.4 | `widget/richtext` (после `text`); `richtext.RegisterXAML`; метод `RichText.OwnsMarkupChildren` — загрузчик не строит детей-абзацев у любого `RichText` (и из пользовательского тега), вместо ветки по типу в загрузчике | `core → richtext`, `xaml → richtext` исчезли |
| 8.5 | `widget/chrome`: `Window`; `ResizeEdge`, `WindowTitleStyle`, цветовые помощники в `base`; `TitleNavButton` — в `base` (общая с диалогом до 8.6); методы `Window` из `navhost.go`, `dialog_chrome.go`, `titlebar.go` — по файлам типа (вместо `NavTitleHost`: каждый тип держит свои методы, `NavHost` не нужен); `BackgroundColorer`, `ChildLayouter`, `AccessDescriber` у `Window`; `InputBindingList` у `Window` и `Canvas`; строитель-контейнер `xaml.Registry.RegisterComplete` и `Loader` (`Offset`, `BuildChild`, `BuildPopupMenu`, `SetName`); `chrome.RegisterXAML`; тесты `xaml` с корнем `<Window>` — во внешнем пакете `xaml_test` (`export_test.go`) | `core → chrome`, `xaml → chrome`, `chrome → dialog`, `chrome → dock` исчезли (и `dock → chrome`) |
| 8.6 | `widget/dialog` (→ `text` напрямую; фабрика `base.SetTextInputFactory` и заглушка поля в тестах ядра сняты) и `widget/dock` (с `Border` — он устроен через `DockPanel`); методы `Dialog` из `navhost.go` — в `dialog`; `EllipsizeText` и `SetWidgetVisible` — в `base`; `ChildLayouter` у `Dialog`, `DockPanel`, `DockManager`, `DockPane`; `AccessDescriber` у `DockPanel`; `Dialog.SetShowLocaleIndicator`; `dock.RegisterXAML` (строители-контейнеры); XAML-тегов у диалога нет — `dialog.RegisterXAML` не нужна | оставшиеся обратные рёбра исчезли; база `depmap` пуста |
| 8.7 | `platform/clipboard`: реализации по ОС (Windows — текст, HTML, картинки, файлы, виртуальные файлы через COM; macOS; Linux), регистрация `base.SetClipboardProvider` в `init`; переносимая часть (`ClipboardProvider` и расширения, буфер в памяти, CF_HTML, списки файлов) — в `base`, служебные для реализаций — с `//facadegen:skip`; импорт — из фасада `widget/clipboard.go` (`window` получает его через фасад) | буфер обмена в showcase и tests без правок |
| 8.8 | не отдельная задача: фасад генерируется на каждом шаге; остаётся проверка `go vet` и `Deprecated`-пометок нет — старые имена не устаревают, они фасад | |

`widget/base/depmap_test.go` (с 8.2a — в `base`) меняется на каждом шаге: исчезнувшие строки базы
убираются; новые рёбра тест не пропустит. 8.9 и 8.10 — как в плане.

## Замер размеров (8.9)

Сборка `go build -trimpath`, `CGO_ENABLED=0`, Go из `toolchain` go.mod;
«до» — коммит `8ddb95b` (8.1, `widget` одним пакетом), «после» — `b7a3f50`
(8.7). Минимальная программа без XAML — `widget/testdata/noxaml` (панель и
кнопка), с XAML — `widget/testdata/withxaml` (одна загрузка разметки),
«только base» — та же программа на `widget/base` без фасада. МиБ.

| Программа | ОС | до | после | разница | `-s -w` до | после | разница |
|---|---|---|---|---|---|---|---|
| showcase | windows | 23,76 | 23,93 | +169 КБ (+0,69 %) | 16,61 | 16,69 | +79 КБ (+0,46 %) |
| desktopdemo | windows | 16,41 | 16,44 | +33 КБ (+0,20 %) | 11,71 | 11,72 | +6 КБ (+0,05 %) |
| webshowcase | js/wasm | 33,96 | 34,12 | +163 КБ (+0,47 %) | 32,90 | 33,02 | +122 КБ (+0,36 %) |
| без XAML | windows | 4,30 | 4,34 | +41 КБ (+0,93 %) | 2,98 | 3,00 | +19 КБ (+0,61 %) |
| без XAML | linux | 4,24 | 4,28 | +43 КБ (+0,98 %) | 2,88 | 2,90 | +20 КБ (+0,68 %) |
| с XAML | windows | 12,94 | 13,05 | +116 КБ (+0,87 %) | 8,88 | 8,93 | +53 КБ (+0,58 %) |
| только base | windows | — | 4,22 | −2,8 % к «без XAML» | — | 2,90 | −3,1 % к «без XAML» |

Что видно. Разбиение почти не меняет размер: код пакетов виджетов в
минимальной программе тот же (символы `widget` до — 24,9 КБ, `base` и
фасад после — 26,9 КБ, из них ~2 КБ — init новых пакетов), прирост файла
(0,2–1 %) — в основном длинные пути пакетов в именах функций и типов
таблицы строк (`…/widget/base.(*Button)…`), которые остаются и после
`-s -w`. Программа на `widget/base` без фасада на 3 % меньше программы на
фасаде: фасад тянет init всех подпакетов и `platform/clipboard`. Парсер
XAML в программу без XAML не попадает ни до, ни после (тест
`widget/noxaml_test.go`).

Главный вклад в размер — XAML: +8,7 МиБ (+5,9 без отладочных таблиц) к
минимальной программе, как и до этапа. Загрузка разметки достигает
строителей всех тегов, а через привязку `ItemsSource` —
`widget/datagrid/binding.go`, где `reflect.Value.MethodByName`: такой вызов
заставляет компоновщик сохранить все экспортированные методы всех
достижимых типов. Это не следствие разбиения, а возможная отдельная работа
(привязка без `MethodByName`, по реестру или кодогенерации) — вне этапа 8.

## Последствия

- Открытый API `widget` сохраняется целиком; новый открытый API — пакеты
  `widget/base` и подпакеты (минорный номер по правилу версий; релиз —
  4.0.0 после всех этапов по решению владельца).
- Один большой механический коммит (8.2a) вместо нескольких средних;
  дальше — обычные вырезания пакетов, каждое с измеримым исчезновением
  рёбер в `depmap`.
- Фасад — сгенерированный код (~950 объявлений: 261 тип, 300 функций,
  358 констант, 23 переменные; методы приходят с алиасами); правится только
  генератор. Коллизии имён между подпакетами (одинаковые конструкторы)
  запрещены тестом полноты фасада: подпакеты сохраняют прежние имена
  (`NewTextBox`, не `text.New`).
- Документация: `go doc widget` показывает обёртки с прежними
  комментариями; подробности — в `widget/base` и подпакетах. GUIDE
  описывает оба способа импорта.
- Риск: внешние программы, пользующиеся `reflect.Type.String()` виджетов
  или путями `widget/*.go` в своих инструментах, увидят `base`. В
  репозитории таких мест нет; Tandem собирает владелец.
