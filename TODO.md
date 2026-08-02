# TODO — роадмап headless-gui

Гэп-анализ против популярных Go-тулкитов (Fyne, Gio, Wails) на 2026-07.
Рендер, текст (шейпинг), HiDPI, AA, Wayland и тестовая инфраструктура уже
на уровне или впереди; разрывы — в «обвязке приложения».

**Инвариант всех работ: headless-контракт неприкосновенен.**
`engine.Frames()` (дельта-тайлы), `SendMouse*/SendKeyEvent`, логические
координаты и zero-CGO не ломаются ни одной фичей из этого списка.

## 1. Критичные (блокируют реальные приложения)

- [x] **Полный ввод текста на Linux (xkb)** — сделано 2026-07-06:
      Wayland — парсер xkb_v1-keymap из fd + модификаторы/группы (живое
      переключение раскладок); X11 — GetKeyboardMapping + state события
      (починены баг shift-из-байта-времени и два day-one бага бэкенда:
      буфер CreateWindow и клиентские resource-ID). Ограничение среды:
      WSLg не меняет keymap на лету (переключение раскладки Windows) —
      на настоящем Linux работает через группы/новый keymap.
- [x] **Файловые диалоги** (Open/Save/SelectFolder) — сделано 2026-07-06:
      встроенный браузер на движке (работает headless/в стриминге — ФС
      процесса/сервера), темизация + локализация, фильтры, breadcrumb,
      двойной клик/Enter. Плюс MessageBox v2 (severity, Ctrl+C в
      Windows-формате, Enter/Esc), InputDialog, ProgressDialog. Нативные
      OS-диалоги — опционально позже.
- [x] **Многострочный TextBox** — сделано 2026-07-07: отдельный виджет
      TextBox (widget/textbox.go): перенос по словам (Wrap) или
      горизонтальный скролл, вертикальный скролл (колесо/PgUp/PgDn,
      тонкий индикатор), выделение мышью и Shift+навигацией, Ctrl+стрелки
      по словам, Ctrl+Home/End, Ctrl+A/C/X/V, Ctrl+Z/Y, контекстное меню,
      ReadOnly, темизация, a11y, headless-ввод. XAML: тег TextBox с
      AcceptsReturn="True"/TextWrapping="Wrap" теперь строит редактор
      (без них — прежний однострочный TextInput). Попутно: PgUp/PgDn/Y
      добавлены в маппинги клавиш всех бэкендов; починен premultiplied-
      цвет выделения (маджента на светлых темах).

## 2. Важные (ожидаемый комфорт)

- [x] **Нативные модальные окна и popup-оверлеи** — сделано 2026-07-11:
      модальные диалоги (`engine.ModalHost`/`SetModalHost`) и оверлеи —
      dropdown/меню (`engine.PopupSink`/`OverlayBoundsProvider`/
      `widget.SetPopupsHosted`) — открываются в собственных окнах ОС.
      Диалог может быть больше главного окна и тащиться за его пределы;
      меню/dropdown не обрезаются краем окна. Нативно на Win32 и X11.
      Известные ограничения:
      - Wayland/macOS/headless — фолбэк in-canvas (рисуется в холст, как
        раньше; функционально идентично, но в пределах окна).
      - X11: клик в ДРУГОЕ приложение не закрывает popup (нет надёжного
        события деактивации, как WM_ACTIVATE на Win32); закрывается кликом
        мимо в своём окне/выбором пункта/Esc.
- [x] **Анимационный фреймворк** — сделано 2026-07-07 (widget/anim.go +
      widget/easing.go): Animate/AnimateOwned (owner-replace против
      «дерущихся» анимаций), 13 канонических кривых, LerpF/Int/Rect/Color
      (premultiplied-корректно), часы у движка (StepAnimations в
      рендер-цикле, ни одной горутины на анимацию), полная интеграция с
      on-demand (кадры только пока идёт анимация, перерисовка частичная).
      Пилоты: ToggleSwitch (скольжение ручки), ProgressBar.AnimateValue,
      fade-in затемнения диалогов; Classic3D — мгновенно. Дальше по теме:
      плавный скролл с инерцией (ниже), нагрузочный тест на сотни анимаций.
- [x] **Плавный скролл** — сделано 2026-07-12: SendMouseWheelPixels (физ.
      координаты, float64-дельты, фолбэк на тики), OnMouseWheelPixels у
      ScrollView (инерция-«маховик» через AnimateOwned, прерывание вводом,
      Classic3D мгновенно), ListView/TextBox — попиксельно с субпиксельным
      накоплением; Win32 WM_MOUSEWHEEL точные дельты, Wayland axis
      wl_fixed→пиксели. X11 — тики (кнопки 4/5), macOS колесо — позже.
- [x] **SVG-иконки** — сделано 2026-07-12: пакет widget/svg (парсер path
      M..Z/дуги/фигуры/transform/fill-rule/currentColor, растеризация через
      x/image/vector с even-odd и кэшем) + виджет SVGIcon (перекраска под
      тему/Tint, пропорции) + XAML-тег <SVGIcon Source Color Tint>.
      Ограничения: без градиентов/clipPath/text; обводка упрощённая.
- [~] **Системные уведомления** — Windows сделано 2026-07-11: balloon через
      Shell_NotifyIcon (NIF_INFO, значок по severity), `Window.ShowBalloon` +
      `SetOnBalloonClick`. **Linux сделано 2026-08-01**: свой D-Bus (window/
      dbus.go + dbus_conn_linux.go, zero deps) и org.freedesktop.Notifications
      (window/notify_linux.go) — X11 и Wayland, иконка в трее НЕ нужна, клик
      доезжает через действие "default" (когда демон объявляет "actions").
      Проверено на живой шине и на стороннем демоне (python-dbus).
      Осталось: macOS NSUserNotification.
- [x] **Tray-иконка с меню** — Windows сделано 2026-07-11 (window/tray*.go):
      `SetTrayIcon`/`RemoveTrayIcon` (Shell_NotifyIcon, image.Image→HICON,
      масштаб до SM_CXSMICON, маска из альфы), `SetOnTrayClick`, `SetTrayMenu`
      (НАШЕ widget.PopupMenu у курсора через хост popup-окон), `HideToTray`/
      `RestoreFromTray`, дефолт «двойной левый клик восстанавливает окно».
      Live-превью окна в панели задач/Aero Peek: WM_PRINTCLIENT из кэша кадра
      (+ опциональный iconic-путь DWM за `HEADLESS_GUI_ICONIC_PREVIEW=1`).
      На Linux/macOS/Wayland — no-op-заглушки. Трей на X11/macOS — позже.
- [~] **Трей + уведомления на Linux/macOS** — `trayHost` (иконка, меню,
      HideToTray) по-прежнему только у `Win32Window`; уведомления вынесены в
      узкий `balloonHost` и на Linux уже работают.
      1. [x] **Linux balloon** — сделано 2026-08-01, см. «Системные
         уведомления» выше. Свой D-Bus живёт в window/dbus*.go и переиспользован
         мостом доступности.
      2. **Linux трей** (иконка+меню) — StatusNotifierItem по D-Bus +
         `com.canonical.dbusmenu` (иконка — ARGB32-pixmap). Старый XEmbed-трей
         устарел; **GNOME без расширения AppIndicator трей не показывает**.
         Высокая цена + ненадёжно между DE — делать после balloon.
      3. **macOS трей+уведомления** — `NSStatusItem` + `UNUserNotification`
         через purego/Cocoa (purego 0.10.1 уже в go.mod). Средне, но нужен
         ЖИВОЙ Mac для проверки (Cocoa-бэкенд на железе не тестирован).
      Публичный API уже кроссплатформенный — трогать только бэкенды
      (window/native_linux.go, native_wayland.go, native_darwin.go + новый
      window/dbus*.go). Дефолт «no-op на неподдержанном» сохранить.
- [x] **Splitter** — сделано 2026-07-12: контейнер SplitPanel (доля 0..1,
      MinFirst/MinSecond, курсоры SizeWE/NS, drag через CaptureManager,
      двойной клик — коллапс/восстановление, гнездование, HasOwnLayout,
      OnPositionChanged) + XAML-тег <SplitPanel> + вкладка «Компоновка»
      в showcase. Для ячеек Grid по-прежнему GridSplitter.
- [x] **Toolbox / докинг-панели** — сделано 2026-07-15 (widget/dockmanager.go,
      dockpane.go, window/dock_host.go): DockManager (центр + 4 стороны,
      VS-порядок, стопки табами, ресайз желобами с MinSize), DockPane
      (титлбар pin/float/✕ release-семантики), drag&dock с направляющими и
      призраком, floating в холсте (drag+edge-resize), auto-hide с выездом
      (анимация часов движка), SaveLayout/RestoreLayout (JSON по id),
      XAML <DockManager>/<DockPane>/<DockContent>, вкладка «Докинг» в
      showcase. НАТИВНЫЙ ОТРЫВ: window.EnableDockFloating(dm) — панель в
      собственном немодальном окне ОС (Win32/X11; фундамент — broadcast-
      реестр нотификаторов, несколько живых движков). Ограничения:
      drag-возврат на направляющие не реализован (возврат кнопкой dock),
      оторванное окно без ресайза, guides — 4 стрелки без центр-креста.
- [ ] **Курсор ввода/IME-позиция** в нативных окнах (candidate window рядом
      с кареткой).

## 3. Стратегические (большие ставки)

- [~] **Браузерный вьювер тайлов** — витрина в браузере готова 2026-08-01
      (cmd/webshowcase): та же разметка showcase.xaml, те же вкладки, темы и
      локализация, но ни одного окна ОС; диалоги и файловые диалоги работают
      поверх стрима и показывают ФС сервера. Страница вьювера переписана
      (масштабирование «вписать/1:1», зрители, кадры, тайлы, трафик, задержка),
      сервер отдаёт `/stats` (JSON) и `/snapshot.png` (текущий кадр), а первый
      подключившийся зритель больше не видит чёрный экран (движок просят
      перерисоваться, если кадров ещё не было). База готова 2026-07-07
      (output/webstream): zero-dep WebSocket-сервер (RFC 6455), бинарный
      протокол (PNG на тайл + keyframe новому клиенту из композита),
      обратные события мыши/клавиатуры/колеса, несколько одновременных
      вьюверов, встроенная HTML/JS-страница (canvas + drawImage,
      упорядоченная очередь кадров), демо cmd/webdemo, e2e-тест
      (рукопожатие → keyframe → клик из клиента → дельта). Дальше:
      сжатие лучше PNG (WebP/zstd), троттлинг/коалесценция для медленных
      клиентов, буфер обмена браузера, опциональный Go-WASM клиент.
- [~] **Платформенный accessibility-мост** — семантическое дерево уже есть
      (`eng.AccessibilityTree()`).
      **AT-SPI (Linux) сделано 2026-08-01** (window/a11y.go — плоский снимок с
      УСТОЙЧИВЫМИ id, hit-test, диффы; window/a11y_linux.go — мост):
      регистрация через `org.a11y.atspi.Socket.Embed`, объекты Accessible/
      Component/Application/Value/Action, кэш `org.a11y.atspi.Cache.GetItems`
      (одним вызовом всё дерево — так его читает Orca), события фокуса, имени,
      значения, состояний и перестройки дерева. Поднимается сам при включённой
      доступности (`org.a11y.Status`), принудительно — `SetAccessibilityEnabled`
      или `HEADLESS_GUI_A11Y=1`. Проверено настоящим клиентом libatspi.
      Не сделано: `GrabFocus`/`DoAction` из скринридера (нужен путь «активировать
      узел по семантическому id» в движке) и интерфейс Text (карет, выделение).
      **UI Automation (Windows) сделано 2026-08-01**
      (window/a11y_uia_windows.go — COM-обвязка без CGO: vtable из
      windows.NewCallback, VARIANT/BSTR/SAFEARRAY, журнал вызовов провайдера
      по `HEADLESS_GUI_UIA_LOG`; window/a11y_windows.go — провайдеры
      Simple/Fragment/FragmentRoot, WM_GETOBJECT, события).
      Живой клиент .NET UIAutomationClient обходит дерево от окна до листьев с
      верными типами, именами, границами и фокусом. Мост включён по умолчанию
      и пассивен: горутина и снимки появляются только после первого
      WM_GETOBJECT. Выключить — `SetAccessibilityEnabled(false)` /
      `HEADLESS_GUI_A11Y=0`.
      Грабли, стоившие полдня и закреплённые тестом: корень фрагмента НЕ должен
      отдавать `NativeWindowHandle` — UIA идёт по этому дескриптору за
      провайдером и делает корень собственным ребёнком.
      Не сделано: паттерны управления (Invoke/Toggle/Value) и SetFocus — нужен
      путь «активировать узел по семантическому id» в движке; поиск по точке
      берёт позицию курсора (windows.NewCallback не принимает double-аргументы).
      Осталось: NSAccessibility (macOS, purego).
- [ ] **IME (CJK)** — text-input-v3 (Wayland), TSF (Windows), NSTextInputClient.
- [ ] **Mobile (Android/iOS)** — самый дорогой разрыв с Fyne/Gio; браться
      только при реальном запросе (WASM-вьювер частично закрывает кейс).

## 4. Техдолг / мелочи

- [ ] macOS-бэкенд не проверен на живой машине (CALayer-путь; фолбэк
      `HEADLESS_GUI_COCOA_LEGACY=1`); активация окна на macOS (delegate).
- [ ] Wayland: минимизация/максимизация (set_minimized/set_maximized),
      точный скролл (axis_discrete/value120), wl_output scale (авто-HiDPI),
      server-side decoration protocol (opt-in).
- [x] gradient.go рисует по логическим строкам — на дробных HiDPI-масштабах
      возможен лёгкий бандинг. Сделано 2026-07-15: ramp строится 1×h / w×1, а
      движок интерполирует в физическое разрешение через DrawImageScaled;
      при scale==1 байт-в-байт идентично (goldens не тронуты).
- [x] Бейдж локали «EN» налезает на длинный заголовок в узком окне
      (обрезать title перед бейджем). Сделано 2026-07-15: заголовок эллипсируется
      у левого края бейджа (или блока кнопок) во всех стилях титлбара.
- [x] DataGrid/TreeView: внутренние пакеты не сообщают «changed» — обёртки
      инвалидируют грубо (весь виджет вместо строки). Сделано 2026-07-15:
      ядра накапливают построчный dirty-диапазон (TakeDirty) — selection/hover
      перерисовывают только затронутые строки; скролл/сорт/expand — вьюпорт.
- [x] Кэш кернинг-пар (fc.Kern ходит в sfnt на каждую пару). Сделано 2026-07-15:
      FontCache.Kern кэширует пары (сброс на SetDPI) — ~1.7× на шрифтах с реальной
      kern-таблицей.
- [x] X11: MIT-SHM для блита. Сделано 2026-08-02 (window/x11shm.go): кадр
      уходит через SysV-сегмент разделяемой памяти (ShmPutImage + событие
      ShmCompletion как обратное давление; занят — кадр идёт PutImage), при
      недоступности расширения/чужом IPC-namespace (WSLg XWayland) — чистый
      фолбэк на PutImage, отловленный синхронным round-trip'ом на старте.
      Проверено e2e против настоящего Xvfb (активация, серия кадров,
      пересоздание сегмента при ресайзе — внешнем и программном SetSize).
- [ ] VirtualizingItemsControl: при ПЕРВОМ показе вкладки строки могут
      отрисоваться со сдвигом (частичная инвалидация первой материализации);
      следующая перерисовка (скролл, hover) чинит. В окне незаметно — кадры
      идут постоянно, а в webstream артефакт видим до следующего изменения.
      Репро: webshowcase → вкладка 3.2.5 → снимок /snapshot.png сразу после
      переключения.

## Рекомендуемый порядок

xkb-ввод → файловые диалоги → multiline TextBox → WASM-вьювер → анимации/SVG.
Первые три делают фреймворк пригодным для продуктовых приложений,
WASM-вьювер — фича, которой нет ни у одного Go-тулкита.
