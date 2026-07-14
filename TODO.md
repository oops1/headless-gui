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
      `SetOnBalloonClick`. Осталось: D-Bus org.freedesktop.Notifications
      (Linux), macOS NSUserNotification.
- [x] **Tray-иконка с меню** — Windows сделано 2026-07-11 (window/tray*.go):
      `SetTrayIcon`/`RemoveTrayIcon` (Shell_NotifyIcon, image.Image→HICON,
      масштаб до SM_CXSMICON, маска из альфы), `SetOnTrayClick`, `SetTrayMenu`
      (НАШЕ widget.PopupMenu у курсора через хост popup-окон), `HideToTray`/
      `RestoreFromTray`, дефолт «двойной левый клик восстанавливает окно».
      Live-превью окна в панели задач/Aero Peek: WM_PRINTCLIENT из кэша кадра
      (+ опциональный iconic-путь DWM за `HEADLESS_GUI_ICONIC_PREVIEW=1`).
      На Linux/macOS/Wayland — no-op-заглушки. Трей на X11/macOS — позже.
- [x] **Splitter** — сделано 2026-07-12: контейнер SplitPanel (доля 0..1,
      MinFirst/MinSecond, курсоры SizeWE/NS, drag через CaptureManager,
      двойной клик — коллапс/восстановление, гнездование, HasOwnLayout,
      OnPositionChanged) + XAML-тег <SplitPanel> + вкладка «Компоновка»
      в showcase. Для ячеек Grid по-прежнему GridSplitter.
- [ ] **Toolbox / докинг-панели** — плавающие инструментальные панели в духе
      Visual Studio: притягиваются (док) к любой стороне окна с направляющими,
      сворачиваются в заголовок (auto-hide/pin), отрываются в ОТДЕЛЬНЫЕ
      нативные окна (инфраструктура нативных окон v3.10 уже готова: owned
      окно + свой surface, как у диалогов) и возвращаются обратно доком.
      Состав: DockManager-контейнер, DockPanelWindow (титлбар с pin/✕),
      сериализация раскладки. Зависит от Splitter (ресайз доков).
- [ ] **Drag&Drop файлов из ОС** в окно (WM_DROPFILES, xdg dnd, wl_data_device).
- [ ] **Цветные эмодзи** — COLR/CBDT-глифы (сейчас честно пропускаются).
- [ ] **Курсор ввода/IME-позиция** в нативных окнах (candidate window рядом
      с кареткой).

## 3. Стратегические (большие ставки)

- [~] **Браузерный вьювер тайлов** — база готова 2026-07-07
      (output/webstream): zero-dep WebSocket-сервер (RFC 6455), бинарный
      протокол (PNG на тайл + keyframe новому клиенту из композита),
      обратные события мыши/клавиатуры/колеса, несколько одновременных
      вьюверов, встроенная HTML/JS-страница (canvas + drawImage,
      упорядоченная очередь кадров), демо cmd/webdemo, e2e-тест
      (рукопожатие → keyframe → клик из клиента → дельта). Дальше:
      сжатие лучше PNG (WebP/zstd), троттлинг/коалесценция для медленных
      клиентов, буфер обмена браузера, опциональный Go-WASM клиент.
- [ ] **Платформенный accessibility-мост** — семантическое дерево уже есть
      (`eng.AccessibilityTree()`); мосты: UI Automation (Windows, COM через
      syscall), AT-SPI (Linux, D-Bus), NSAccessibility (macOS, purego).
- [ ] **IME (CJK)** — text-input-v3 (Wayland), TSF (Windows), NSTextInputClient.
- [ ] **Mobile (Android/iOS)** — самый дорогой разрыв с Fyne/Gio; браться
      только при реальном запросе (WASM-вьювер частично закрывает кейс).

## 4. Техдолг / мелочи

- [ ] macOS-бэкенд не проверен на живой машине (CALayer-путь; фолбэк
      `HEADLESS_GUI_COCOA_LEGACY=1`); активация окна на macOS (delegate).
- [ ] Wayland: минимизация/максимизация (set_minimized/set_maximized),
      точный скролл (axis_discrete/value120), wl_output scale (авто-HiDPI),
      server-side decoration protocol (opt-in).
- [ ] gradient.go рисует по логическим строкам — на дробных HiDPI-масштабах
      возможен лёгкий бандинг.
- [ ] Бейдж локали «EN» налезает на длинный заголовок в узком окне
      (обрезать title перед бейджем).
- [ ] DataGrid/TreeView: внутренние пакеты не сообщают «changed» — обёртки
      инвалидируют грубо (весь виджет вместо строки).
- [ ] Кэш кернинг-пар (fc.Kern ходит в sfnt на каждую пару).
- [ ] X11: MIT-SHM для блита (сейчас PutImage через сокет).

## Рекомендуемый порядок

xkb-ввод → файловые диалоги → multiline TextBox → WASM-вьювер → анимации/SVG.
Первые три делают фреймворк пригодным для продуктовых приложений,
WASM-вьювер — фича, которой нет ни у одного Go-тулкита.
