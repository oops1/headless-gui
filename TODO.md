# TODO — роадмап headless-gui

Гэп-анализ против популярных Go-тулкитов (Fyne, Gio, Wails) на 2026-07.
Рендер, текст (шейпинг), HiDPI, AA, Wayland и тестовая инфраструктура уже
на уровне или впереди; разрывы — в «обвязке приложения».

**Инвариант всех работ: headless-контракт неприкосновенен.**
`engine.Frames()` (дельта-тайлы), `SendMouse*/SendKeyEvent`, логические
координаты и zero-CGO не ломаются ни одной фичей из этого списка.

## 1. Критичные (блокируют реальные приложения)

- [ ] **Полный ввод текста на Linux (xkb)** — сейчас упрощённый маппинг:
      кириллица в TextBox под X11/Wayland не набирается.
      Wayland: парсить keymap из `wl_keyboard.keymap` (fd, формат xkb_v1),
      модификаторы/группа из события `modifiers` (живое переключение RU/EN).
      X11: `GetKeyboardMapping` + state из события (заодно баг: shift сейчас
      читается из байта времени). Windows уже полноценен (WM_CHAR).
- [ ] **Файловые диалоги** (Open/Save/SelectFolder) — топ-запрос любого
      тулкита. Без CGO: Win32 `GetOpenFileNameW`/IFileDialog; macOS
      NSOpenPanel через purego; Linux — xdg-desktop-portal (D-Bus) с
      fallback на zenity/kdialog.
- [ ] **Многострочный TextBox** (TextWrapping, скролл, выделение по строкам,
      Ctrl+стрелки) — без него не сделать формы с комментариями/редакторы.

## 2. Важные (ожидаемый комфорт)

- [ ] **Анимационный фреймворк** — easing-кривые, `Animate(from, to, dur,
      fn)`, интеграция с on-demand циклом (кадры только пока идёт анимация).
- [ ] **Плавный скролл** — пиксельные дельты тачпада (Wayland axis уже
      дискретизируем — отдавать точное значение), инерция в ScrollView.
- [ ] **SVG-иконки** — темизируемые иконки (перекраска под тему), парсер
      подмножества SVG (path/fill) поверх существующего AA-растеризатора.
- [ ] **Системные уведомления** — Win32 toast/Shell_NotifyIcon, D-Bus
      org.freedesktop.Notifications, macOS NSUserNotification.
- [ ] **Tray-иконка** с меню.
- [ ] **Drag&Drop файлов из ОС** в окно (WM_DROPFILES, xdg dnd, wl_data_device).
- [ ] **Цветные эмодзи** — COLR/CBDT-глифы (сейчас честно пропускаются).
- [ ] **Курсор ввода/IME-позиция** в нативных окнах (candidate window рядом
      с кареткой).

## 3. Стратегические (большие ставки)

- [ ] **WASM-вьювер тайлов** — тонкий браузерный клиент: WebSocket →
      дельта-тайлы → canvas `putImageData`, события мыши/клавиатуры обратно.
      Одно Go-приложение на сервере — UI в любом браузере без пересборки.
      Идеально ложится в headless-ДНК; «WebSocket streaming» из README
      становится живой демкой. Сюда же: кодирование тайлов (PNG/WebP/zstd)
      для экономии трафика.
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
