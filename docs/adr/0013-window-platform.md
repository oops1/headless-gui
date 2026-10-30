# ADR-0013. `window` → `platform/*`: бэкенды отдельными пакетами, `window` — фасад

Статус: предложено 2026-10-29, задача 9.1 (этап 9), ветка
`feature/engine-architecture`. Ревью — владелец. Основание — план этапа 9 и
обзор (`docs/ARCHITECTURE_REVIEW_RU.md`, §3.2 T1, T2, T4, T5, T8, §3.1 W9,
§4.3: `platform/{win32,x11,wayland,cocoa}`, `platform/keyboard`,
`platform/pointer`, `platform/a11y`, `platform/{tray,dnd,dbus,shell}`,
`window` — фасад, `window/minimal`).

## Контекст

Пакет `window` сегодня — 360 файлов. Без тестов:

| Часть | Файлов | Строк |
|---|---|---|
| общая (без тегов) | 70 | 16 269 |
| Windows (`windows`, `_windows.go`) | 28 | 9 418 |
| Linux (`linux && !android`, `_linux.go`) | 57 | 18 430 |
| macOS (`darwin`) | 4 | 880 |
| заглушки прочих ОС | 4 | 56 |

Тестов — 194 файла (из них под Linux 78, под Windows 35). Открытый API:
175 имён верхнего уровня (142 константы — `VK_*` и стороны, 15 функций,
14 типов, 4 переменные) и 67 методов `*Window`. Пакет импортирует
`engine`, `widget`, `output`, `internal/{clipimage,pixsimd,goid}`.

Устройство. `Window` (обвязка: `window.go`, `surface`) связывает движок
(`EngineAPI`) с нативным окном `NativeWindow` — интерфейсом из `native.go`,
плюс около семидесяти неэкспортированных необязательных интерфейсов (бэкенда
и движка), которые обвязка проверяет утверждением типа (`dirtyRectBlitter`,
`exposeNotifier`, `filesDropTarget`, `iconSetter`, `initialPlaceBackend`,
`cursorImageSetter`, `rawKeySource`, `lockKeysSettler`, `ownedWindow` и
другие). При переносе интерфейсы бэкенда становятся экспортированными в
`platform/native` (служебные — с `//facadegen:skip`). Бэкенд создаёт `NewNativeWindow()` своей ОС; на
Linux она выбирает Wayland при живом `WAYLAND_DISPLAY`, иначе X11.

### Карта зависимостей

Построена тем же инструментом, что и для `widget` (`deps8`/`internal/depmap`,
по типам, файлы по тегам сборки), отдельно под windows, linux и darwin.
Группы — по функциям: бэкенды (`win32`, `x11`, `wayland`, `cocoa`),
клавиатура, указатель, доступность, трей, DnD, D-Bus, «оболочка» (openurl,
notify, systheme, syslang), остальное — обвязка.

Что показала карта:

1. **Обвязка почти не знает бэкендов.** Под Windows рёбер «обвязка → win32»
   три: `NewNativeWindow`, `nativeDoubleClickTime`, `platformMonitors`. Под
   Linux рёбер больше (98 к X11, 75 к Wayland), но все они — из файлов,
   которые по смыслу уже части бэкенда, а по имени — нет:
   `activate_x11_linux.go`, `alwaysontop_x11_linux.go`, `icon_linux.go`,
   `initial_x11_linux.go`, `activate_wayland_linux.go`,
   `initial_wayland_linux.go`, `monitor_linux.go`.
2. **Бэкенды берут у обвязки общие типы и помощники, а не её логику:**
   `NativeWindow` и `VK_*`, `Monitor`/`matchMonitor`, `initialPlacement`,
   планы полноэкранного режима (`fsSnapshot`, `fsEnterPlan`, `fsLeavePlan`,
   `fsRefit`), `HitArea`, иконки (`pickIcon`, `iconToRGBA`), пиксельные
   преобразования (`swapRBRow`, `convRectBGRX`), `applyCornerMask`, модель
   масштаба (`xftdpi.go`), обработчики дочерних попапов Wayland, MIME-помощники
   буфера обмена Linux (`clipboard_*.go`). Это слой ниже бэкендов.
3. **Группы «по функциям» смешаны.** Файлы возможностей под ОС живут на
   внутренностях своего бэкенда и обратно: `tray_windows.go` → `Win32Window`
   (109 обращений: `hwnd`, буфер кадра, поля трея, `user32`/`gdi32`),
   `dnd_windows.go` → 56, указатель → Wayland 333, клавиатура → X11 90; а
   оконная процедура Win32 зовёт обработчики трея, UIA и DnD. Переносимая же
   часть возможностей (`keysym`, `keytable`, `xkbmap`, состояние lock-клавиш,
   `rawKey`, кнопки мыши, кэш картинок курсора, разбор протоколов X11 и
   Wayland, D-Bus) общая для бэкендов.
4. **Обвязка возможностей — методы `Window`** в тех же «функциональных»
   файлах (`lockkeys.go`, `keygrab.go`, `touch.go`, `pointerlock.go`,
   `ime.go`, `tray.go`, `dnd.go`, `a11y.go`): они принадлежат `Window`, а не
   бэкенду.

Тот же вывод, что на этапе 8: делить по функциям нельзя, делить надо по
бэкендам, а переносимое — вынести слоем ниже.

## Решение

### 1. Слои

```
platform/native      контракты и общие типы бэкендов: NativeWindow, VK_*,
                     необязательные возможности (экспортированные интерфейсы),
                     Monitor, HitArea, начальное положение, планы полного экрана,
                     иконки, пиксельные преобразования, скругление, масштаб,
                     MIME-помощники буфера; точки расширения (§3)
platform/keyboard    keysym, keytable, xkbmap, состояние lock-клавиш, rawKey
platform/pointer     кнопки, эмуляция касаний, состояние захвата указателя,
                     кэш картинок курсора, курсоры по ролям
platform/dbus        протокол D-Bus
platform/win32       Win32Window и всё *_windows.go бэкенда (мышь, касания, IME,
                     захват клавиатуры, lock-клавиши, полный экран, мониторы,
                     иконка, курсор, регионы попапов); детект возможностей ОС
platform/x11         X11Window, x11*.go, буфер обмена X11, активация, иконка
platform/wayland     WaylandWindow, wayland*.go, буфер обмена Wayland;
                     валидация configure (W9)
platform/cocoa       окно macOS (purego)
platform/a11y        UIA (Windows), AT-SPI (Linux)        — через точки расширения
platform/tray        Shell_NotifyIcon и iconic-превью, SNI — через точки расширения
platform/dnd         OLE, XDND, wl_data_device             — через точки расширения
platform/shell       openurl, notify, systheme, syslang, портал
window/core          Window, surface, очередь ввода и модификаторы, попапы,
                     модальные окна, док, обвязка мониторов, активации, полного
                     экрана, lock-клавиш, захвата, касаний, IME; выбор бэкенда
window               фасад над window/core и platform/native (facadegen) +
                     import _ всех возможностей
window/minimal       тот же фасад без возможностей
```

Порядок пакетов:

```
platform/native ← platform/keyboard, platform/pointer, platform/dbus
platform/native, keyboard, pointer ← platform/{win32,x11,wayland,cocoa}
бэкенды, native ← platform/{a11y,tray,dnd}; dbus ← a11y, tray, shell (Linux)
native, keyboard, pointer, бэкенды ← window/core
window/core, все возможности ← window
window/core ← window/minimal
```

Бэкенды и возможности импортируют `widget/base` (типы курсора, событий,
`ClipboardProvider`), но не `widget`, `engine` и `window/*`. `window/core`
импортирует `engine` и `widget/base`.

Почему `window/core`, а не обвязка прямо в `window`. `window/minimal` и
`window` должны давать один и тот же тип `Window`: программа, собранная на
`window/minimal`, и библиотека на `window` обмениваются окнами. Если
обвязка останется в `window`, а `window` импортирует возможности,
`window/minimal` не сможет ни повторить тип, ни импортировать `window`, не
утащив возможности. Тот же приём, что `widget/base` (ADR-0012): ядро ниже,
оба фасада — алиасы на него. Генератор фасада — `internal/facadegen`
(список `facadegen.Window`), тест полноты и снимок API — как у `widget`.

### 2. Открытый API не меняется

`window.Window`, `window.New`, `window.NativeWindow`, `window.NewNativeWindow`,
`window.VK_*`, `window.Monitor`, `window.HitArea` и прочие — алиасы и обёртки
в фасаде; методы `Window` приходят с алиасом. Программы (showcase,
desktopdemo, Tandem, WinLine) не правятся. Видимое снаружи — как на этапе 8:
имена типов в рефлексии — по пакету (`*core.Window`, `*win32.Win32Window`).

### 3. Возможности через `import _`: точки расширения бэкендов

Возможность под ОС живёт на внутренностях бэкенда (п. 3 карты), поэтому
отделить её можно, только если бэкенд откроет ей узкую дверь:

- **Win32.** `win32.AddMessageHook(msg uint32, h MessageHook)` — оконная
  процедура спрашивает зарегистрированные обработчики раньше своего
  `switch` (`WM_GETOBJECT` — UIA, `wmTrayCallback`, `WM_DWMSENDICONIC*`,
  `WM_PRINTCLIENT` — трей); у окна — `HWND()`, `InvokeOnUIThread`, доступ к
  последнему кадру (превью трея), место под состояние возможности
  (`Ext(key)`/`SetExt`); регистрация цели DnD — через хук создания и
  разрушения окна.
- **X11, Wayland.** Хуки событий по опкоду и расширению, хук привязки
  глобальных объектов Wayland по имени интерфейса, доступ к соединению,
  окну и `wl_surface`.
- **Связь с `Window`.** Возможность регистрирует в `platform/native`
  фабрику (`native.RegisterTray(func(NativeWindow) (Tray, bool))` и т. п.);
  методы `Window.SetTrayIcon`, `BeginDragFiles`, `SetAccessibilityEnabled`
  ищут её и без неё отвечают «не поддерживается» (`false` или
  `ErrUnsupported`) — так, как сегодня отвечают бэкенды без возможности.

`window` импортирует все возможности (`import _`) — поведение программ
прежнее. `window/minimal` — без них: нет AT-SPI-соединения, SNI, OLE,
портала; меньше горутин при старте и меньше бинарь.

Это самая неопределённая часть решения. Она проверяется на 9.8 первой же
возможностью (UIA — у неё одна точка входа, `WM_GETOBJECT`). Запасной
вариант, если точки расширения окажутся толще самих возможностей: файлы
возможностей под ОС остаются в бэкенде, а `window/minimal` отключает их
флагом бэкенда, без выигрыша в размере.

### 4. `window.Style` для E8 (Tandem RemoteApp)

Окна RemoteApp — без рамки, принадлежат главному окну, сами не появляются
на панели задач, управляются извне по Z и положению (T5). Сегодня у
`Window` нет ни позиции после показа, ни Z-порядка, ни владельца.

```go
// Style — вид и поведение нативного окна.
type Style struct {
	// Borderless — окно без рамки ОС и без её украшений: нет зон изменения
	// размера, скругления углов, тени ОС; содержимое целиком принадлежит
	// приложению (окно RemoteApp рисует свою рамку).
	Borderless bool
	// Owned — окно принадлежит владельцу (SetOwner): всегда над ним по Z,
	// сворачивается и восстанавливается вместе с ним.
	Owned bool
	// Tool — окно-инструмент: без кнопки на панели задач и вне Alt+Tab.
	Tool bool
}

func (w *Window) SetStyle(s Style) bool        // false — часть стиля бэкенд не умеет
func (w *Window) Style() Style
func (w *Window) SetOwner(owner *Window) bool
func (w *Window) Raise() bool                  // наверх Z без активации
func (w *Window) Lower() bool                  // вниз Z
func (w *Window) SetPosition(x, y int) bool    // после показа
func (w *Window) Position() (x, y int, ok bool)
```

Стиль задаётся до `Run` (применяется при создании) или после (бэкенд
меняет стиль живого окна, где умеет). Каждая операция честно отвечает,
выполнена ли она:

| | Win32 | X11 | Wayland | macOS |
|---|---|---|---|---|
| Borderless | `WS_POPUP` без зон `WM_NCHITTEST`, `DWMWCP_DONOTROUND`, без тени | `_MOTIF_WM_HINTS`, без формы скругления | да (декор у клиента) | `NSWindowStyleMaskBorderless` |
| Owned / SetOwner | `GWLP_HWNDPARENT` | `WM_TRANSIENT_FOR` | `xdg_toplevel.set_parent` | `addChildWindow` |
| Tool | `WS_EX_TOOLWINDOW` | `_NET_WM_WINDOW_TYPE_UTILITY`, `SKIP_TASKBAR`, `SKIP_PAGER` | `false` | `NSWindowStyleMaskUtilityWindow` |
| Raise / Lower | `SetWindowPos(HWND_TOP/BOTTOM, SWP_NOACTIVATE)` | `_NET_RESTACK_WINDOW` (иначе `ConfigureWindow`) | `false` | `orderFront`/`orderBack` |
| SetPosition после показа | да | да (с учётом рамки WM) | `false` | да |

Бэкенд сообщает возможности через необязательный интерфейс
`native.Styler` (и `native.Stacker`, `native.Positioner`): обвязка без
него отвечает `false`. `SetOwner` опирается на уже существующую
возможность бэкенда `ownedWindow.SetOwner` (владелец модальных окон,
`modal_host.go`; сегодня её реализуют Win32 и X11). macOS — в той же таблице, но реализуется по мере
E15 (отложено у Tandem); на 9.7 — Win32 и X11, Wayland — свои `false` и
`set_parent`.

### 5. Возможности версии ОС (T1, T2) — в `platform/win32`

`win32.Caps` определяется один раз при загрузке (номер сборки через
`RtlGetVersion`, проверки DWM): скругление DWM (Windows 11), тень окна с
регионом (её нет на Windows 10 — T2), предел `StretchDIBits` (окно
больше 1024² пустое на Windows 10 — T1). Блит большого кадра на Windows 10
идёт через DIB-секцию (`CreateDIBSection` + `BitBlt`). Живые тесты T1 и
T2 — только на интерактивном рабочем столе (`HGUI_LIVE=1`), в CI
пропускаются.

### 6. Валидация `configure` (W9) — в `platform/wayland`

У окна с закреплённым размером (`pinSize`: не растягиваемое) `configure`
с другим размером не принимается как есть: окно отвечает своим размером
(`ack_configure` + буфер своего размера). Тест W9 — на поддельном
композиторе уровня протокола (по образцу поддельных серверов X11 в
сегодняшних тестах на `net.Pipe`).

### 7. Контрактные тесты бэкендов (T4)

Один набор сценариев — `platform/native/contracttest`: размер и
минимальный размер, `SetSize` до и после показа, полный экран и выход,
фокус и активация, lock-клавиши, стиль (§4). Набор принимает фабрику
`NativeWindow` и окружение; Win32 гоняет его живьём (`HGUI_LIVE=1`), X11
и Wayland — на поддельных серверах протокола, macOS — только сборка.
Пишется на Win32 (9.9), тиражируется по образцу (9.10).

### 8. Проверка порядка

`window/core/depmap_test.go` — карта групп файлов по целевым пакетам с
базой сегодняшних обратных рёбер (как `widget/base/depmap_test.go`);
каждый шаг убирает свои строки. Тест идёт под ОС, на которой запущен
(файлы по тегам), поэтому база ведётся отдельно для windows и linux (в CI
Linux есть).

### 9. План (замена 9.2–9.12)

| Шаг | Что | Приёмка |
|---|---|---|
| 9.2a | `window/core`: перенос всего кода и тестов (как 8.2a), фасад `window` (`facadegen.Window`), тест полноты и снимок API, `depmap` по целевым пакетам | `go test ./...`, showcase, desktopdemo и Tandem без правок; сборки и vet на windows, linux, darwin |
| 9.2b | `platform/native`, `platform/keyboard`, `platform/pointer`, `platform/dbus`: контракты, общие типы, переносимые таблицы и протоколы (поглощает 9.6) | то же; `depmap`: бэкенды не зависят от обвязки |
| 9.2 | `platform/win32` (с файлами, по имени не похожими на бэкенд); `win32.Caps`; T1, T2 | то же; живые T1, T2 |
| 9.3 | `platform/x11` (с `activate_x11`, `alwaysontop_x11`, `icon_linux`, `initial_x11`, буфером X11) | то же; тесты X11 на поддельном сервере |
| 9.4 | `platform/wayland`; W9 | то же; тест W9 |
| 9.5 | `platform/cocoa` | кросс-сборка darwin |
| 9.7 | `Style`, `SetOwner`, `Raise`/`Lower`, `SetPosition`/`Position`: Win32, X11; Wayland — `set_parent` и честные `false` | пилот Tandem RemoteApp |
| 9.8 | точки расширения бэкендов; `platform/a11y` (первой — проверка решения §3), `platform/tray`, `platform/dnd`, `platform/shell`; `window/minimal` (поглощает 9.11) | `window` — поведение прежнее; `window/minimal` — без возможностей; замер |
| 9.9 | контрактные тесты на Win32 | живой прогон |
| 9.10 | тиражирование на X11, Wayland, Cocoa | |
| 9.12 | документация, матрица возможностей бэкендов | |

Порядок: сначала ядро и слой ниже бэкендов (9.2a, 9.2b), затем бэкенды по
одному, затем `Style` (нужен Tandem и опирается на бэкенды), затем
возможности (опираются на точки расширения бэкендов).

### 10. Уточнения по ходу работ

**9.2b.** Слой ниже бэкендов вынесен, но не весь «контракт» сразу:

- В `platform/native` переехали `NativeWindow` и `VK_*` и только те
  необязательные интерфейсы, на которые бэкенды ссылаются по имени
  (`IconSetter`, `ActivateBackend`, `PointerLocker`). Остальные
  (`dirtyRectBlitter`, `filesDropTarget`, `ownedWindow` и другие) остаются в
  `window/core`, пока бэкенды там же: у них неэкспортированные методы, и
  переезжают они вместе с первым бэкендом, которому нужно реализовать их из
  другого пакета (9.2), — тогда же их методы экспортируются.
- Ключ монитора в бэкенде (`Monitor.id`) остаётся скрытым полем: бэкенды и
  ядро обращаются к нему через служебные `native.MonitorWithKey` и
  `native.MonitorKey`. Открытый `window.Monitor` не меняется.
- Колесо в единицах оборудования (`Win32WheelRaw`, `X11WheelRaw`,
  `WaylandAxisRaw`) — в `platform/pointer`, вместе с кнопками, уходом
  указателя, кэшем и картинками курсоров; интерфейсы ядра `rawKeySource`,
  `wheelRawSource`, `wheelRawEngine` — в `window/core`.
- Выбор бэкенда Linux (`NewNativeWindow`: Wayland или X11) — в ядре
  (`backend_linux.go`), а не в файле X11.
- Фасад `window` собирается из трёх пакетов: `window/core`,
  `platform/native`, `platform/keyboard` (`LockKeys`); `platform/pointer` и
  `platform/dbus` открытых имён `window` не дают.
- У типов бэкендов (`Win32Window`, `X11Window`, `WaylandWindow`) появился
  экспортированный метод `SetIcon` — метод интерфейса `native.IconSetter`.
  Это неизбежно для любого интерфейса, который бэкенд реализует из другого
  пакета; на 9.2–9.5 так же откроются методы остальных необязательных
  интерфейсов. Параметры `SetOnRawKey` и `SetChildPopupHandlers` у тех же
  типов (раньше — неэкспортированные типы) теперь `keyboard.RawKey` и
  `native.ChildPopupHandlers`: служебные типы пакетов слоя ниже.
- Эмуляция мыши касанием и состояние захвата указателя, которые §1 относит к
  `platform/pointer`, — обвязка `surface` (`touch.go`, `pointerlock.go`) и
  остаются в `window/core`; в `platform/pointer` — только переносимое
  (кнопки, уход указателя, колесо, RAWINPUT, курсоры).
- В `depmap` возможности под ОС (`a11y_*`, `tray_*`, `dnd_*`, `openurl_*`,
  `systheme_*`, `syslang_*`, `notify_*`) — своя группа: рёбра «бэкенд →
  возможность» (оконная процедура Win32 зовёт обработчики трея, UIA и DnD;
  X11 — XDND; бэкенды Linux держат трей SNI и уведомления) остаются в базе
  до точек расширения 9.8.

**9.2.** Бэкенд Win32 — пакет `platform/win32`:

- Вместе с окном переехали файлы, живущие на его внутренностях: трей,
  перетаскивание (OLE), системная тема и акцент, а также основа COM
  (`GUID`, HRESULT, `NewVTable`), общая с UI Automation. В `window/core`
  под Windows остались возможности, которым нужно ядро: UI Automation
  (снимок доступности `Window`), открытие ссылок, языки системы.
- Первая точка расширения §3 — `win32.AddMessageHook(msg, h)`: оконная
  процедура спрашивает обработчики раньше своих; UI Automation отвечает так на
  `WM_GETOBJECT`. Дескриптор окна — `Win32Window.HWND`.
- Ядро зовёт возможности ОС одними именами на всех платформах
  (`platformMonitors`, `detectSystemTheme`, `nativeDoubleClickTime`…); под
  Windows их передаёт бэкенду прослойка `window/core/backend_windows.go`. Там
  же открытые имена фасада: псевдоним `Win32Window = win32.Win32Window`,
  `NewNativeWindow`, `AllowForeground`. Тип в `win32` назван `Win32Window`, а
  не `Window`, — имя открытого типа `window.Win32Window` сохраняется без
  переименования в генераторе фасада.
- Интерфейсы ядра с неэкспортированными методами, которым удовлетворяли
  бэкенды (`trayHost`, `balloonHost`, `monitorSource`, `monitorChangeSource`,
  `initialPlaceBackend`, `clientSizer`, `popupRegionSetter`,
  `secondaryMarker`) и анонимный интерфейс `DragImageSupported` в
  `Window.DragImageSupported`, получили экспортированные методы у всех
  бэкендов сразу:
  тип из другого пакета иначе молча перестал бы им удовлетворять. Интерфейсы
  остались в ядре: бэкенды на них по имени не ссылаются.
- В `platform/native` — общее для бэкендов, что понадобилось Win32 вне
  ядра: `SystemTheme` с константами (открытое имя, в фасаде), служебные
  `ErrTrayUnsupported`, `IconColorBuffer`. `window.SystemTheme` теперь
  псевдоним `native.SystemTheme`.
- Интеграционные тесты ядра с живым окном Win32 (хост попапов, курсоры,
  смена размера, тема) берут внутренности бэкенда через `internal/testhook`
  (их ставит `platform/win32` при загрузке), а не через открытый API.
- T1 и T2 исправлены ещё в v3.38 и v3.39 (буфер кадра одной резервацией
  `VirtualAlloc`; прямые углы и рамка DWM в 1 px вместо скруглённого
  региона). `win32.Caps` (версия по `RtlGetVersion`, `RoundCorners`,
  `SplitDIBSource`) сводит различия версий в одно место: скругление DWM
  больше не пробуется вызовом на Windows 10; `SplitDIBSource` справочное —
  буфер одной резервацией и так выводится на любой версии. Блит через DIB-секцию не
  понадобился: буфер одной резервацией выводится и на Windows 10.

**9.3.** Бэкенд X11 — пакет `platform/x11`:

- Вместе с окном переехали файлы, по имени не похожие на бэкенд:
  `activate_x11_linux.go`, `alwaysontop_x11_linux.go`, `icon_linux.go`,
  `initial_x11_linux.go`, XDND (`dnd_linux.go` — методы `X11Window`),
  раскладки (`locale_linux.go` → `x11_locale_linux.go`), переносимые разбор
  протокола и помощники (`x11argb.go`, `x11shape.go`, `x11cursorbitmap.go`,
  `x11touch.go` и другие).
- Трей StatusNotifierItem и уведомления `org.freedesktop.Notifications`
  встраиваются и в X11, и в Wayland, поэтому вынесены слоем ниже бэкендов —
  пакет `platform/tray` (`tray.SNI`, `tray.Notifier`). Окна встраивают их
  через неэкспортированные псевдонимы: методы трея продвигаются, а полей в
  открытом API `window.X11Window` и `window.WaylandWindow` нет. `HideToTray`
  и `RestoreFromTray` — методы бэкендов. На 9.8 трей подключится через точки
  расширения, и направление зависимости развернётся (бэкенд не будет
  импортировать трей).
- Биты краёв окна `NativeEdge*` (рамка → `OnNativeResize` → бэкенд) — в
  `widget/base` вместо `widget/chrome`: бэкенды импортируют только
  `widget/base`. Открытые имена `widget.NativeEdge*` прежние.
- Стандартные интерфейсы D-Bus — `dbus.IfaceProperties`,
  `dbus.IfaceIntrospectable`; порядок мониторов — `native.SortMonitors`;
  `SolidBands` бэкенд берёт из `core/raster`, а не из `engine`.
- В ядре — псевдоним `X11Window = x11.X11Window` (открытое имя фасада) и
  выбор бэкенда; методы трея `SetTrayMenuNative`, `SetTrayMenuRunner`
  экспортированы (интерфейсы ядра `nativeTrayMenu` и анонимный).
- Тесты ядра `backend_contract_*_test.go`: каждый бэкенд удовлетворяет всем
  интерфейсам-возможностям ядра, которым удовлетворял на 9.3 (проверка при
  сборке) — тихая потеря возможности при переносе бэкенда (как
  `dragImageSupported` на 9.2) теперь ошибка сборки тестов.
- Тесты Linux прогоняются в WSL (WSLg — живые X11-тесты тоже): тестовые
  бинари собираются с `GOOS=linux` и запускаются в каталогах пакетов.

**9.4.** Бэкенд Wayland — пакет `platform/wayland`:

- Окно и всё на его внутренностях; общие Linux-тесты ядра, которые после
  9.3 остались только про Wayland (`clipboard_*_linux_test.go`,
  `rawkey_linux_test.go` и другие), переехали целиком.
- Ядру нужны только конструктор и мониторы: `wayland.NewWindow` (выбор
  бэкенда в `backend_linux.go`) и `wayland.EnumMonitors` (мониторы без
  окна); опрос мониторов окна остаётся неэкспортированным. В ядре —
  псевдоним `WaylandWindow = wayland.WaylandWindow`.
- `SolidBands`, `OpaqueBands` бэкенд берёт из `core/raster`, а не из
  `engine`: `platform/wayland` не тянет ни `engine`, ни `widget`.
- Тесты lock-клавиш Wayland проверяют бэкенд, а не окно ядра:
  `Window.SetLockKeys` и `Window.LockKeys` — маска и вызов бэкенда, логику
  «активация вместе с состоянием» ядро проверяет фейком
  (`TestLockKeys_ActivateLocks*`, в том числе срок ожидания); тесты Wayland —
  что окно сообщает активность (`SetOnActivate`) и первое состояние фокуса
  (`LockKeysSettled`, `SetOnLockKeysSettled`) при любом порядке
  configure(activated), enter и modifiers.
- W9 исправлен ещё в v3.43.3 по отчёту на v3.43.2 (`floatingSize`: плавающее
  окно с запретом ресайза держит размер фиксации `pinSize`, что бы ни назвал
  configure; развёрнутому, полноэкранному и прилепленному размер назначает
  компоновщик). Добавлен тест на
  поддельном композиторе уровня протокола
  (`TestWayland_ConfigureSize_FixedWindowAnswersWithOwnSize`): на устаревший
  configure окно отвечает `ack_configure` тем же serial, пределов не
  переотправляет, нового буфера не заводит, прикрепляет и коммитит уже
  заведённый кадр своего размера;
  при снятой проверке тест падает.

**9.5.** Бэкенд macOS — пакет `platform/cocoa` (`CocoaWindow`, purego):

- В пакет ушло окно (`native_darwin.go`); заглушки мониторов, темы и
  открытие ссылок под macOS — функции ядра под ОС, остались в `window/core`.
  В ядре — прослойка `backend_darwin.go`: псевдоним
  `CocoaWindow = cocoa.CocoaWindow` и `NewNativeWindow`.
- Бэкендов в `window/core` больше нет ни под одной ОС; `depmap` из карты
  переноса стал сторожем. Файлы ядра под ОС — только прослойки
  (`backend_*.go`, `monitor_*.go`) и возможности (feature); любой другой файл
  под ОС или с именем бэкенда попадает в группу `backend`, и тест требует,
  чтобы она была пуста. Пустота проверяется явно, а не через рёбра: бэкенд с
  9.2b к ядру не обращается, и вернувшийся файл бэкенда рёбер бы не дал.
- Проверка — кросс-сборка и vet под darwin/arm64 и darwin/amd64: живого
  macOS у проекта нет.

## Последствия

- Открытый API `window` сохраняется целиком; новый открытый API —
  `window.Style` и методы окна (§4), пакеты `platform/*`, `window/core`,
  `window/minimal`. Релиз — 4.0.0 после всех этапов.
- Два больших механических шага (9.2a, 9.2b) теми же инструментами, что на
  этапе 8 (перенос объявлений, переквалификация по типам, генератор
  фасада), но с проверкой под каждую ОС: чистка импортов и переквалификация
  идут для windows, linux и darwin отдельно (урок 8.6).
- Живые тесты Linux и macOS здесь не запускаются: Linux проверяется
  тестами на поддельных серверах и в CI, macOS — сборкой.
- Риск — точки расширения (§3): если они выйдут толще самих возможностей,
  `window/minimal` теряет выигрыш в размере, но сохраняет отключение
  служб при старте.
