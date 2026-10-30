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
platform/tray        Shell_NotifyIcon, SNI, уведомления    — через точки расширения
platform/dnd         OLE, XDND, wl_data_device             — через точки расширения
platform/shell       openurl, notify, systheme, syslang,  — через точки расширения
                     портал
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
бэкенды, native ← platform/{a11y,tray,dnd,shell}; dbus ← a11y, tray (Linux), shell
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

**9.7.** `window.Style`, `SetStyle`, `Style`, `SetOwner`, `Raise`, `Lower`,
`SetPosition`, `Position` (`window/core/style.go`); контракты бэкенда — в
`platform/native/style.go`: `Styler` (`ApplyStyle` возвращает стиль, который
действует после вызова), `OwnerSetter`, `Stacker`, `Positioner`. Отличия от
§4:

- Владелец — новая возможность `OwnerSetter.SetOwnerWindow`, а не
  `SetOwner` модального окна: тот на X11 ставит `_NET_WM_STATE_MODAL`, а на
  Win32 помечает окно вторичным. Окно с владельцем модальным не становится.
  Закрытый владелец не принимается ни до создания окна, ни после.
- Закрытие владельца освобождает окно на всех ОС одинаково: ядро до
  уничтожения окна ОС (`Close` и закрытие средствами ОС без хука) снимает
  владельца у окон, которыми оно владеет (`releaseOwned`). Иначе Windows
  уничтожила бы их вместе с ним без `WM_CLOSE` — мимо ядра и
  `SetOnCloseRequest` (а главное окно с владельцем-ребёнком завершило бы
  приложение), а на X11 их `WM_TRANSIENT_FOR` указывал бы на уничтоженное
  окно, номер которого сервер отдаёт новому соединению. Окно ОС, которое ещё
  создаётся, ядро не трогает: владельца, закрывшегося за это время, снимает
  `bringUp` сразу после `Create` (`settleOwner`); запись окна в список
  владельца перепроверяет его закрытие (гонка `SetOwner` с `Close`
  владельца). На Win32 откат опоздал бы: `DestroyWindow` владельца
  уничтожает окно сразу. Поэтому владелец ставится на его потоке, а его
  `WM_CLOSE` перед `DestroyWindow` сам снимает себя у окон, которым его
  поставил `SetOwnerWindow` (`releaseOwnedOnClose`): установка между снятием и
  уничтожением не вклинится. Окна модальных диалогов гибнут с владельцем, как
  прежде. Окно Win32, которое спрятал свёрнутый владелец (`WM_SHOWWINDOW` с
  `SW_PARENTCLOSING`), после снятия владельца показывается без активации
  на потоке окна: показать его больше некому. Окно возвращается в прежнем
  состоянии (свёрнутое — свёрнутым); его собственные окна Windows ведёт сама:
  свёрнутый владелец их не прячет (проверено на Windows 11), свёрнутое окно
  прячет и вернёт при своём восстановлении. `Close` до `Run` — no-op, как и
  обещала его документация (прежде он ставил признак закрытия навсегда).
- Wayland владельца не умеет: у каждого окна своё соединение с
  компоновщиком, а `xdg_toplevel.set_parent` принимает только объект того же
  соединения. Ответ — `false`, как и на Z-порядок и положение; из стиля —
  только `Borderless` (без `BeginResize` и маски углов). Окно Wayland теперь
  реализует `cornerRounder`: без `Borderless` — `true`, как бэкенд без
  интерфейса прежде.
- X11 `Tool` — `_NET_WM_STATE_SKIP_TASKBAR`, `_SKIP_PAGER` и, для KWin,
  `_KDE_NET_WM_STATE_SKIP_SWITCHER` (без него `Tool` всё равно выполним), без
  типа `_NET_WM_WINDOW_TYPE_UTILITY` из таблицы §4: у mutter окно UTILITY не
  получает фокус при показе и встаёт под окно с фокусом, а KWin (по
  умолчанию) прячет окна UTILITY с владельцем, пока активно чужое
  приложение. До показа состояния — в начальном `_NET_WM_STATE` одним
  значением с «развёрнуто»; из трея окно-инструмент возвращается с ними
  заново (у спрятанного окна менеджер снимает `_NET_WM_STATE`).
- X11 `Borderless`: рамки менеджера у окна нет и так (Motif decorations=0);
  `BeginResize` отказывает, а нулевые `_GTK_FRAME_EXTENTS` говорят
  композитору, что тень окно рисует само (mutter, picom с обычным
  `shadow-exclude`). Окну с владельцем тип `_NET_WM_WINDOW_TYPE_NORMAL`
  ставится явно: окно с `WM_TRANSIENT_FOR` без типа менеджер обязан считать
  диалогом. Обычному окну и модальному диалогу ни тип, ни
  `_GTK_FRAME_EXTENTS` не ставятся — их поведение прежнее. Raise и Lower —
  `ConfigureWindow` stack-mode Above и Below: `_NET_RESTACK_WINDOW` EWMH
  отводит пейджерам. Ответ на X11 — «запрос отправлен»: mutter подъём окна
  другого приложения при активном чужом отклоняет и помечает окно «требует
  внимания» (у окна без `Tool` GNOME Shell покажет уведомление), а
  `Raise` отвечает `true`. Общий `WM_CLIENT_LEADER` окон процесса это
  смягчил бы — отдельная задача, не 9.7.
- Зоны ресайза у `Borderless` снимает и ядро: корпус виджетного окна на X11
  и Wayland заводит их по наличию `OnNativeResize`, и отказа `BeginResize`
  мало — у края остались бы курсор ресайза и съеденное нажатие.
  `syncEdgeResize` снимает и возвращает хук вместе со стилем (после запуска —
  на горутине движка, вместе с `syncSquareCorners`).
- Win32 `Borderless` выключает неклиентскую отрисовку DWM
  (`DWMWA_NCRENDERING_POLICY`): без неё нет ни тени, ни скругления, ни рамки
  в 1 px Windows 10; заказанное до `Run` окно создаётся скрытым и
  показывается уже без неё. `Tool` у видимого окна меняется через
  скрыть — сменить стиль — показать (панель задач замечает стиль только при
  показе); активное окно показывается активным, неактивное возвращается на
  своё место в Z-порядке вместе со своими окнами с владельцем (диалоги, окно
  IME): с `SWP_NOOWNERZORDER` Windows их сама не переставляет, их ставит тот
  же код (`insertGroup`). Стиль живого окна меняется
  на его потоке (как захват клавиатуры): вызов, не начатый к сроку или к
  гибели окна, отменяется — стиль позже не сменится вопреки ответу.
  Установка владельца — на потоке владельца, тем же путём (`callOnUI`) и с
  той же отменой по сроку и гибели окна; не начатый к сроку вызов отвечает
  `false`. Снятие владельца, Z-порядок и положение — прямыми вызовами, как
  `SetAlwaysOnTop`.
- Ответы Win32 — по итогу, а не по коду возврата: окно фонового процесса
  Windows молча оставляет под активным окном другого процесса, хотя
  `SetWindowPos` отвечает успехом. `Raise` сверяет, что над окном не осталось
  видимых окон его слоя (кроме принадлежащих ему; окна, скрытые DWM, не в
  счёт). Тем же путём молча не ставился «поверх всех» у окна, опущенного вниз,
  а `SetAlwaysOnTop` отвечал `true`; теперь ответ — по `WS_EX_TOPMOST` после
  вызова. `Lower` у окна «поверх всех» — `false`: `HWND_BOTTOM` молча снял бы
  режим. Окно, оказавшееся ниже нового владельца, встаёт сразу над ним
  (вместе со своими окнами с владельцем); у
  владельца «поверх всех» оно тоже становится «поверх всех» (так Windows
  поступает с окнами такого владельца); не встало — владелец прежний, ответ
  `false`.
- `SetPosition` и `Position` — `Positioner.MoveTo` и `ScreenPosition`
  (`NativeWindow.SetPosition` ответа не даёт): `false` у развёрнутого,
  свёрнутого и полноэкранного окна; у свёрнутого окна положения нет.
- До `Run` (и до `OpenWindow` для дочернего окна) стиль и владелец
  запоминаются (`true`) и уходят бэкенду до создания окна ОС: Win32 — в
  `CreateWindowExW` (`WS_EX_TOOLWINDOW`, `hWndParent`), X11 — до `MapWindow`.
  `Style` после запуска — ответ бэкенда. На macOS заказ не запоминается
  (`false`, `Style` его не показывает). У закрытого окна все операции
  отвечают `false`.
- Открытый API: тип `window.Style` (снимок API — 176 имён), методы окна; у
  `Win32Window` и `X11Window` — методы возможностей (`ApplyStyle`,
  `SetOwnerWindow`, `RaiseWindow`, `LowerWindow`, `MoveTo`, `ScreenPosition`),
  у `WaylandWindow` — `ApplyStyle` и `RoundsCorners`. Контракты: Win32 45, X11
  41, Wayland 38, Cocoa 3 и отрицательный тест (`TestBackendContract_CocoaNoStyle`).

**9.8.** Возможности окна выносятся из ядра и бэкендов подшагами, каждый —
своим коммитом: 9.8.0 каркас, 9.8a доступность (первой — проверка решения
§3 на UIA), 9.8b оболочка, 9.8c трей, 9.8d перетаскивание, 9.8e замер и
документы.

- **9.8.0, каркас (поведение не меняется).** Реестр возможностей —
  `platform/native/capability.go`: ячейка на возможность (неэкспортированный
  обобщённый тип: экспортированный `internal/facadegen` не пропускает и с
  `//facadegen:skip`), последняя регистрация выигрывает, `restore` возвращает
  прежнюю (тестам — через `t.Cleanup`); `native.Capabilities()` —
  подключённые возможности, для диагностики и тестов. Реестр читается только
  во время работы, не из `init`. Фасад `window/minimal` (пакет `minimal`,
  `facadegen.WindowMinimal`) — те же источники и тот же снимок API, что у
  `window`: тип `Window` один, окна взаимозаменяемы; отличие фасадов —
  рукописный `window/capabilities.go` с пустыми импортами возможностей
  (появляется с первой возможностью). Тест слоёв `internal/layers` — по
  `go/build` под windows, linux и darwin, только рабочие импорты: ядро и
  `window/minimal` не тянут ни возможностей, ни `platform/dbus`; бэкенды не
  тянут возможностей; `platform/*` не тянет ни `engine`, ни фасада
  `widget`, ни `window`; `window` подключает каждую вынесенную возможность.
  Нарушения сейчас — в базе теста по ОС (ядро → `platform/dbus` на всех ОС,
  на Linux бэкенды X11 и Wayland → `platform/tray`); база убывает по
  подшагам, а ребро базы, которого больше нет, — ошибка теста. Живой тест
  `TestUIA_LiveWMGetObject`: `WM_GETOBJECT` на настоящем окне Win32 доходит
  до моста через `win32.AddMessageHook` (без моста — `DefWindowProc`); до
  9.8 этот путь тестами не покрывался, при переносе он сторожит регистрацию
  обработчика.
- Ядро импортирует фасад `widget`, а не только `widget/base`, как сказано в
  §1 (`widget.Window`, док, диалоги): ядру это разрешено, правило «только
  `widget/base`» — для бэкендов и возможностей.
- **9.8a, доступность → `platform/a11y`.** Решение §3 на UIA подтвердилось:
  от бэкенда мосту нужно только то, что есть с 9.2, — `win32.AddMessageHook`
  (`WM_GETOBJECT`) и `Win32Window.HWND()`; новых точек расширения нет. Мосты
  UI Automation и AT-SPI и общая часть (плоский снимок семантики, устойчивые
  id, диффы, диапазоны текста UIA, куски текста AT-SPI, маршрутизация действий
  скринридера) перенесены почти побайтно: `git mv`, пакет, `widget.` →
  `base.` в коде (все имена — из `widget/base`; в комментариях ссылки на
  настоящие виджеты остались `widget.`), обращения к окну `b.win.*` →
  `b.host.*()`, `start`/`stop` → `Start`/`Stop`, регистрация — из `init`
  пакета. Контракт — `platform/native/access.go`: `AccessHost` (дерево
  семантики и действия движка, `PostToEngine`, окно ОС, масштаб, заголовок,
  `AccessibilityForced` — решение `SetAccessibilityEnabled`), `AccessBridge
  {Start, Stop}`, `RegisterAccessibility`/`Accessibility` (всё со
  `//facadegen:skip`). В ядре остались `SetAccessibilityEnabled`, подъём моста
  в `bringUp` и снятие в `tearDown` через реестр и неэкспортированный адаптер
  `a11yHost` (методы на `Window` попали бы в открытый API). Фасад `window`
  подключает возможность в `window/capabilities.go`; в `window/minimal`
  моста нет, и `SetAccessibilityEnabled` — тихий no-op (метод значения не
  возвращает, API по §2 не меняется). Тесты перенесены все — 115 функций, как
  до переноса; окно для них — тестовый хост поверх настоящего движка с теми же
  именами, что у окна ядра (`New`, `newTestWindow`, `scale`, `title`,
  `native`). Тестам `platform/a11y` разрешено импортировать `engine` и
  `widget` (им нужны настоящие кнопки и поля): правило слоёв — только для
  рабочих импортов. Исходы тестов Linux в WSL совпадают с HEAD; живой
  `TestUIA_LiveWMGetObject` переехал вместе с мостом и проверяет, что `init`
  пакета зарегистрировал и фабрику, и обработчик `WM_GETOBJECT`. Новые тесты
  ядра: без возможности — no-op, фабрика (Start в `bringUp`, Stop в
  `tearDown`, не поднявшийся мост окну не достаётся), адаптер (в том числе
  движок без доступности), `window` — возможность подключена, `window/minimal`
  — ни одной. Найденные при переносе старые ошибки (`parentRef` AT-SPI
  отдаёт индекс снимка вместо устойчивого id; `GetAll` и `GetChildAtIndex` без
  проверки длины тела вызова, а у `platform/dbus` нет `recover`;
  `UiaDisconnectAllProviders` на весь процесс при закрытии любого окна; гонки
  чтения заголовка и масштаба с потоков клиента) перенесены как есть — это
  отдельные задачи.
- **9.8b, оболочка → `platform/shell`.** Открытие ссылок и файлов
  (ShellExecute, портал OpenURI и FileManager1 с запасным xdg-open, open),
  языки ОС (`GetUserPreferredUILanguages` на Windows, переменные среды по
  gettext), системная тема и акцент (реестр и `WM_SETTINGCHANGE` на Windows,
  портал настроек и `GTK_THEME` на Linux) перенесены из ядра и из
  `platform/win32` (`systheme*` бэкенда — целиком) в `platform/shell`.
  Контракт — `native.Shell` (структура функций, поле nil — ОС этого не
  умеет) и `RegisterShell`/`ShellServices`. Открытый API прежний и у обоих
  фасадов один: `OpenURL`, `OpenFile`, `RevealFile`, `ErrOpen*`,
  `SystemLanguages`, `DetectSystem*`, `SystemAppearance` и подписки окна
  остаются в ядре тонкими диспетчерами. Проверка входа осталась в ядре и
  действует перед любой реализацией (политика схем и запрет UNC — граница
  безопасности, запрет файлов-программ — защита от ошибки приложения): в
  `window/minimal` плохая ссылка по-прежнему даёт `ErrOpenScheme`,
  проверенная — `ErrOpenUnsupported`; язык не задан, тема неизвестна,
  наблюдения нет. Win32: обработчик `WM_SETTINGCHANGE` ставит `init` пакета
  через `win32.AddMessageHook` и всегда отвечает `handled=false` (мониторы
  бэкенда по `SPI_SETWORKAREA` и `DefWindowProc` работают как прежде);
  подписки — в пакете, по окну (`*win32.Win32Window`), а не по `hwnd`: их
  просят и до `Create`. Новая точка расширения бэкенда — `win32.WindowOf(hwnd)`
  (та же маршрутизация, что у оконной процедуры: дескриптор, а до него —
  создаваемое окно). Из бэкенда ушли поля `onSettingChange` и
  `onAccentChange`, метод `handleSettingChange`, экспортированные
  `ReadSystemTheme`, `ReadSystemAccent`, `WatchSystemTheme`,
  `WatchSystemAccent` (пакет ещё не выпускался) и точки
  `testhook.Win32SettingWatched`/`Win32SettingChange`. Исправлено попутно:
  подписка Windows была одной ячейкой окна, и при двух одновременных
  подписках (ядро поднимает её из `bringUp` и из `SetOn*` приложения, лишнюю
  снимает) `stop` лишней стирал нужную — тема и акцент переставали приходить
  до конца жизни окна; теперь у каждой подписки свой номер, как у хаба
  портала на Linux. Сквозной тест `TestWindowAppearanceWiredToWin32` заменён
  тремя: ядро с поддельной оболочкой (окно ОС доходит до обеих подписок,
  смена — до колбэка, снятие колбэка снимает подписку); тест регистрации в
  `platform/shell` (набор служб по ОС, и в каждом поле своя реализация —
  сверка указателей функций), через зарегистрированные службы идут и тесты
  открытия Windows; живое окно (`SendMessage WM_SETTINGCHANGE` доходит до
  подписок через обработчик, значения — из реестра, а `SPI_SETWORKAREA` того
  же сообщения — до мониторов бэкенда: обработчик его не забирает).
  Регистрация: Windows и Linux — всё; macOS — открытие и языки (тема пока не
  реализована, см. `platform/shell/register_darwin.go`); прочие ОС и android —
  только языки. Ядро больше не импортирует `platform/dbus` на Windows и macOS
  (на Linux его пока приносит трей бэкендов — до 9.8c); база теста слоёв
  сократилась. Фасад `window` линкует `platform/dbus` на всех ОС через
  `platform/shell`: разбор ответов портала собирается без тегов и
  проверяется на любой машине, а к шине обращается только Linux. Горутин при
  старте оболочка не заводит ни в одном фасаде (сессионная шина — лениво, при
  первом вызове). Сторож depmap ядра: группа `feature` снята — файлов
  возможностей под ОС в ядре больше нет, вернувшийся файл с суффиксом ОС
  попадает в группу `backend` и роняет тест; файл, выбранный только тегом
  `//go:build`, ловит новый `TestDepMap_NoTaggedOSFiles` (в ядре такие —
  только прослойки, поимённо). Подписка на тему не переживает окно:
  `tearDown` и неудачное создание окна ОС её снимают и до следующего
  `bringUp` новую не допускают, а поднятая в гонке с закрытием снимается
  сразу — иначе на Windows (карта окон пакета) и на Linux (хаб портала на
  процесс) обработчик держал бы окно с движком до конца процесса.

- **9.8c, трей и уведомления → `platform/tray`.** Контракт —
  `native.Tray` (иконка, клики, меню системы и его очередь, курсор для нашего
  меню, уведомления, `Close`) и `TrayFactory` по окну ОС (`nil` — у окна трея
  нет), `RegisterTray`/`TrayService`; имена методов прежние. Ядро создаёт трей
  окну в `bringUp` после `Create`, прямо перед `applyPendingTray` (порядок
  применения прежний), и снимает в `tearDown`; после `Close` трей отвечает
  отказом, так что вызов, разминувшийся с закрытием, иконку не вернёт.
  Интерфейсы бэкенда `trayHost`, `balloonHost`, `nativeTrayMenu` и анонимный
  `SetTrayMenuRunner` сняты; у бэкенда остался узкий `trayWindow`
  (`HideToTray`, `RestoreFromTray` — операции окна, работают и в
  `window/minimal`), передний план для меню — прежний `foregrounder`. Без
  возможности: `SetTrayIcon` до Run — `nil`, после — `ErrTrayUnsupported`,
  `ShowBalloon` — `ErrTrayUnsupported`, меню и клики — no-op. Win32:
  Shell_NotifyIcon и состояние иконки — в пакете, по окну
  (`*win32.Win32Window`, маршрутизация через `WindowOf`), под замками (поля
  окна раньше писались с горутины движка и читались в `wndProc`; вызов
  Shell_NotifyIcon — не под замком колбэков, чтобы присланное окну во время
  ожидания сообщение не встало на нём); обработчики `win32.AddMessageHook` —
  `WM_APP+1` (события иконки, отвечает за сообщение целиком, как прежний
  `case`; бэкенд держит код за треем) и `WM_DESTROY` (`handled=false`).
  Новая точка расширения бэкенда — одна: `win32.NewIcon` (HICON из картинки,
  тот же, что у значка окна). Превью окна в панели задач (`WM_PRINTCLIENT`,
  iconic DWM) — функция окна, а не трея: осталось в бэкенде
  (`taskbar_windows.go`, вместе с «спрятать в трей»), отклонение от §1 и §3 —
  доступа к последнему кадру не понадобилось, и превью в `window/minimal` не
  чернеет. Помощники значков (`rgbaToHandle`, процедуры GDI) — в
  `icon_windows.go`, маска — `iconmask.go`. Linux: трей — `SNI` и `Notifier`
  без окна бэкенда (оба — разговор с панелью и демоном по D-Bus), фабрика —
  для любого окна; бэкенды X11 и Wayland больше не импортируют
  `platform/tray`. Из открытых наборов методов `Win32Window`, `X11Window`,
  `WaylandWindow` ушли методы иконки и уведомлений (пакеты `platform/*` ещё не
  выпускались, в v3.44.0 эти методы были неэкспортированными; снимок API —
  176 имён — прежний). Исправлено попутно: иконку при закрытии окна не снимал
  никто (Win32 — призрак до наведения мыши и утечка HICON, Linux — живое
  соединение и клики в мёртвое окно) — теперь `Close` в `tearDown` и, на
  Win32, `WM_DESTROY` (дочерние окна, чей `tearDown` приходит только с
  закрытием главного, и окна, гибнущие с владельцем); подписка уведомлений
  Linux была своим вечным обработчиком на каждое окно (у `dbus.Conn` снятия
  нет), державшим окно с движком, — теперь хаб на процесс (один `AddMatch` и
  один обработчик, id уведомления → окно), `Close` снимает окно из хаба;
  два первых `SetTrayIcon` SNI поднимали два соединения — регистрацию и
  снятие упорядочивает свой замок. Слои: ядро и `window/minimal` больше не
  тянут `platform/dbus` и `platform/tray` ни на одной ОС — база теста слоёв
  пуста везде; `window` подключает трей. Контракты: Win32 44, X11 39,
  Wayland 36, Cocoa 3 и отрицательные `TestBackendContract_Win32NoTray`,
  `TestBackendContract_LinuxNoTray`. Тесты: ядро с поддельным треем (без
  возможности, порядок применения и `Close` ровно раз, свой трей и диспетчер
  у нового окна, клики по умолчанию и меню), регистрация по ОС, живое окно
  Win32 (события иконки через обработчик, `WM_DESTROY` снимает иконку и
  пропускает сообщение — бэкенд окно убирает; после — отказ), `Close` у SNI
  (имя на шине освобождается) и у `Notifier` (клик закрытого окна никуда не
  идёт). Не сделано, отдельные задачи: перезапуск Проводника (`TaskbarCreated`
  — иконка пропадает); `SetTrayMenu` не убирает прежнее меню из дерева; у
  дочерних окон Linux иконка живёт до закрытия главного (их `tearDown`);
  GUIDE и README о трее — на 9.12.

- **9.8d, перетаскивание → флаг `platform/dnd` (запасной вариант §3).**
  Приёмник и источник остались в бэкендах: они живут на внутренностях окна
  (OLE `IDropTarget` и `DoDragDrop`, XDND, `wl_data_device`), и точки
  расширения вышли бы толще самой возможности. Пакет `platform/dnd` — флаг:
  `init` зовёт `native.RegisterDragDrop` (ячейка `dnd` реестра 9.8.0, на
  Windows и Linux), бэкенды и ядро читают `native.DragDropEnabled`. Без него
  (`window/minimal`): Win32 не объявляет окно приёмником (ни
  `OleInitialize`/`RegisterDragDrop`, ни `DragAcceptFiles`), X11 не ставит
  `XdndAware`, Wayland отклоняет offer (`wl_data_device` нужен буферу обмена,
  его не снять), `BeginDragFiles` и `DragImageSupported` в ядре — `false`;
  выигрыша в размере нет, код остаётся в бинаре. Тестовые бинари ядра и
  бэкендов включают флаг своим `init` (как фасад `window`) и снимают его в
  проверках «без возможности»: `TestBeginDragFiles_NoCapability`,
  `TestEnableFileDrop_NoCapability`, `TestWayland_DragTargetNoCapability`;
  у X11 отдельной проверки нет (`XdndAware` ставится в `Create` на живом
  соединении). Проверено заодно: UI Automation не зависит от OLE окна — в
  тестовом бинаре `platform/a11y` флага нет, `OleInitialize` на потоке окна не
  зовётся, а живой `TestUIA_LiveWMGetObject` получает корневой провайдер.
  Слои: `window` подключает `platform/dnd` (тянет только `platform/native`).

- **9.8e, замер и итог 9.8.** Проба — одна и та же программа (окно на
  фасаде, `CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`) с `window` и с
  `window/minimal`:

  | ОС | `window` | `window/minimal` | разница |
  |---|---|---|---|
  | Windows amd64 | 9 510 912 | 9 112 064 | −390 КиБ (−4,2 %) |
  | Linux amd64 | 10 252 448 | 9 867 424 | −376 КиБ (−3,8 %) |
  | macOS arm64 | 8 574 818 | 8 558 034 | −16 КиБ (−0,2 %) |

  Пакетов модуля в программе: 43 и 38 (Windows, macOS), 44 и 39 (Linux) — у
  `window/minimal` нет `platform/a11y`, `dnd`, `shell`, `tray` и `dbus`.
  Windows с открытым окном: модулей процесса 38 и 33 — `window/minimal` не
  грузит `ole32.dll` (OLE приёмника перетаскивания) и `UIAutomationCore.dll`;
  горутин в обоих фасадах 1 после `init`, 2 после `New`, 25 с окном. Linux
  (WSLg, Wayland и X11): горутины те же (1, 2 и 23–26 с окном — разброс
  момента замера); ни одна возможность горутин при старте не заводит (шина
  D-Bus — лениво), мост AT-SPI в WSL не поднимается (нет шины доступности).
  macOS: выигрыша почти нет — там из возможностей только оболочка.

  Итог 9.8. Решение §3 подтвердилось для доступности, оболочки и трея: у
  Win32 им хватило `AddMessageHook`, `HWND`, `WindowOf` и `NewIcon`, у X11 и
  Wayland — ничего (Linux-возможности от окна бэкенда не зависят). Для
  перетаскивания взят запасной вариант §3 — флаг. Отклонения от §1 и §3:
  превью окна в панели задач осталось в бэкенде (функция окна, не трея);
  уведомления — в `platform/tray`, а не в `platform/shell`; хуки
  создания и разрушения окна для цели перетаскивания, хуки X11 и Wayland по
  опкоду, `Ext`/`SetExt` и доступ к последнему кадру не понадобились —
  состояние возможностей лежит в картах пакетов по окну, а их снятие —
  `tearDown` ядра и, на Win32, хук `WM_DESTROY`. Тест слоёв: база пуста на
  всех ОС; в ядре из файлов под ОС — только прослойки (`depmap`). Попутно
  исправлено (подробности — в 9.8b и 9.8c): подписки темы и трея, которые
  переживали окно; гонка двух подписок темы на Windows; иконка трея, которую
  при закрытии окна не снимал никто; вечные обработчики уведомлений Linux.
  Отдельные задачи: перезапуск Проводника (`TaskbarCreated`), иконка
  дочерних окон Linux до закрытия главного, старые ошибки AT-SPI и UIA из
  9.8a; документация `window/minimal` в GUIDE и README — на 9.12.

- **9.9, контрактные тесты на Win32 (§7, T4).** Набор сценариев —
  `platform/native/contracttest` (`Run(t, Env)`): окно создаётся заказанного
  размера; до `Create` `GetSize` отвечает заказанным `SetSize`, а размер
  окна задаёт `Create`; `SetSize` после показа меняет размер и доходит до
  `SetOnResize`; минимальный размер держится (у бэкенда, который держит его и
  при программной смене размера, — `Env.MinSizeEnforced`); развернуть и
  восстановить; полный экран и выход с размером, заказанным в полном экране;
  активация; переключатели клавиатуры (то же состояние — успех без перемен,
  Scroll Lock — на миг и обратно, `Env.ToggleLockKeys`); стиль §4 (умеемое —
  `Env.Style`, остальное честно `false`, сброс — нулевой стиль). Сценарии
  обращаются к окну только через открытые методы и необязательные
  интерфейсы, бэкенд даёт окружение (`New`, `Pump`, `Destroy`). Win32 —
  `TestWin32Contract` на живых окнах, на закреплённом потоке сценария; идёт
  по умолчанию, как остальные живые тесты Win32 (без рабочего стола —
  пропуск), а не по `HGUI_LIVE=1`: самое заметное — Scroll Lock на миг.
  Активация в свежем фоновом процессе честно пропускается (Windows не даёт
  передний план), в последующих прогонах — проверяется. Найдено при
  написании: Win32 сообщает об активации ещё при создании окна, а успешный
  `Activate` уже активного окна второго уведомления не шлёт — колбэк сценарий
  ставит до `Create`, как ядро. Мутационная проверка набора: 6 поломок
  бэкенда (размер, восстановление, минимум, размер в полном экране, стиль Tool,
  переключатели) — все ловятся. Тиражирование на X11, Wayland и Cocoa — 9.10.

- **9.10, контрактные тесты на X11, Wayland и Cocoa.** Тот же набор:
  `TestX11Contract` и `TestWaylandContract` — на настоящих серверах (`DISPLAY`,
  `WAYLAND_DISPLAY`; без них — пропуск), прогон — в WSLg (Xwayland и Weston);
  `TestCocoaContract` собирается под darwin, гоняется только на Mac по
  `HEADLESS_GUI_COCOA_LIVE=1`. Отклонение от §7: вместо поддельных серверов
  протокола — живые; полный `Create` на подделке (рукопожатие, атомы,
  реестр, xdg-shell) — отдельная задача ради CI без дисплея. Окружению
  добавлены `Started` (насос событий на соединение окна), `contracttest.Poll`,
  `Requests` (Activate, Maximize, SetFullscreen у X11 и Wayland — просьбы:
  не изменившееся состояние — пропуск, изменившееся не так — ошибка) и
  `LockKeysSetOptional` (Wayland переключает переключатели только при
  `zwp_virtual_keyboard_manager_v1`). Найдено и исправлено: насос событий
  вторичного окна X11 (`StartEventPump`: окна `OpenWindow`, попапы) не
  обслуживал вопросов о переключателях — `LockKeys`/`SetLockKeys` у таких
  окон всегда отвечали «не знаю»; окно Wayland после «развернуть →
  восстановить» оставалось размером с экран, когда компоновщик (Weston)
  отвечал на восстановление `0×0`, — теперь окно запоминает обычный размер при
  развороте, как уже делало окно, начавшее жизнь развёрнутым
  (`TestWayland_ConfigureStates` закреплял прежнее поведение — исправлен);
  `configure`, сменивший только активность окна и пришедший раньше ответа на
  `set_fullscreen`, Wayland принимал за отказ (лишнее «не в полном экране» и
  возврат размера) — теперь отказом не считается
  (`TestWayland_Fullscreen_ActivationConfigureIsNotRefusal`). Итог прогонов в
  WSLg: X11 — все сценарии (6 прогонов подряд), Wayland — все, кроме
  активации (нет токена xdg-activation) и переключения переключателей (нет
  виртуальной клавиатуры), — пропуск (8 прогонов подряд). Компоновщик WSLg
  изредка не отвечает на `unset_maximized`/`unset_fullscreen`; так же
  изредка падал и прежний `TestWaylandFullscreenE2E`. Отдельно: вторичный
  насос X11 не обслуживает и касания, захват указателя, подписку буфера
  обмена — проверить, нужны ли они окнам `OpenWindow`.

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
