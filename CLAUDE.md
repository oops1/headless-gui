# CLAUDE.md — руководство для ИИ-агентов

Инструкции для ИИ-ассистентов (Claude Code и аналогичных), работающих с этой
кодовой базой. Человеческая документация: [README](README.md) ·
[README_RU](README_RU.md) · [GUIDE](GUIDE.md) · [GUIDE_EN](GUIDE_EN.md) ·
роадмап: [TODO.md](TODO.md).

## Что это

Headless GUI-движок на чистом Go: рендерит виджетный UI off-screen в
RGBA-буфер и выдаёт изменившиеся тайлы 64×64 через канал
(`engine.Frames() <-chan output.Frame`). Вывод подключается отдельно:
браузер (`output/webstream`), RDP, нативное окно (`window/`).

## Железные правила

1. **Headless-контракт неприкосновенен.** Ни одна фича не должна ломать:
   `engine.Frames()` (дельта-тайлы 64×64, физические пиксели),
   `SendMouseMove/SendMouseButton/SendKeyEvent`, логические координаты
   виджетов, **zero CGO** во всех модулях. Всё, что требует окна ОС,
   живёт только в `window/` за build-тегами.
2. **Ветки:** рабочая ветка — `develop`. Релизы через git-flow оставляют
   HEAD на `master` — **всегда** проверяй `git branch --show-current`
   перед коммитом и при необходимости `git checkout develop`.
3. **Не форматируй массово.** Репозиторий не gofmt-чистый (CRLF).
   `gofmt -l` покажет почти все файлы — это норма. Форматируй только
   строки, которые меняешь.
4. **Полный прогон перед коммитом:**
   ```bash
   go build ./...
   go test ./...
   go test -race ./tests/ ./engine/
   go vet -unsafeptr=false ./...          # -unsafeptr=false обязателен (purego)
   GOOS=linux  CGO_ENABLED=0 go build ./...
   GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...
   ```
5. **Визуальные изменения проверяй рендером**: собери мини-приложение,
   `eng.SaveFrames(dir)` пишет PNG-кадры — смотри их. Golden-тесты
   (`tests/golden_*`) ловят регрессии попиксельно; при осознанном
   изменении внешнего вида перегенерируй эталоны.

## Карта пакетов

```
engine/     рендер-цикл, Canvas (Draw*-примитивы, AA, HiDPI), шрифты
            (кэш глифов + шейпинг go-text), события, тултипы, damage
widget/     все виджеты + темы + XAML + биндинги + локализация + a11y
  treeview/, datagrid/   ядра без зависимости от widget
output/     Frame/DirtyTile
  webstream/  WebSocket-вьювер (zero-dep RFC 6455) + встроенный viewer.html
window/     нативные бэкенды Win32/X11/Wayland/Cocoa (отдельный go.mod)
cmd/        showcase (витрина), webdemo (браузер), smartgit, guiview
tests/      интеграционные и golden-тесты
```

## Ключевые паттерны (соблюдай при правках)

- **Инвалидация:** сеттеры виджетов сравнивают состояние и зовут
  `Invalidate()`/`InvalidateRect` только при фактическом изменении —
  движок по умолчанию рендерит по запросу. Забытая инвалидация =
  «зависший» UI; лишняя = сожжённый CPU.
- **Мьютексы:** виджет держит `mu` только вокруг состояния; колбэки
  (`OnChange`, `OnClick`) и `Invalidate()` вызываются **после** Unlock.
  `Draw` копирует состояние под mu, рисует без него.
- **Замер текста вне Draw:** `widget.MeasureUIText(text, sizePt)` —
  точный измеритель, который регистрирует движок (`SetTextMeasurer`).
  Использовать для компоновки до отрисовки (диалоги, TextBox). Внутри
  Draw — `ctx.MeasureText`.
- **Темизация:** конструкторы читают глобальную палитру `win10.*`
  (обновляется `ApplyGlobalTheme`), `ApplyTheme(t *Theme)` перекрашивает
  живые виджеты. Служебные виджеты могут читать `win10.*` прямо в Draw.
  Вторичный текст — `Label.Muted = true` (тема красит в InputPlaceholder).
  Классика Win2000 — отдельная ветка отрисовки (`currentStyle().Classic3D`,
  bevel-хелперы), не забывай её.
- **Полупрозрачные цвета — ловушка premultiplied:** `color.RGBA` в Go —
  альфа-премультиплицированный. Цвет с каналами больше альфы
  (например `{0,120,215,90}`) при Over-блендинге переполняется и даёт
  мадженту. Строй такие цвета через `premulAlpha(base, alpha)`
  (widget/textbox.go). Также: `FillRoundRect` при A<255 идёт по
  legacy-Src-пути (НЕ смешивает) — для честного бленда используй
  `FillRectAlpha`.
- **Модальные диалоги:** `engine.ShowModal` центрирует и сдвигает детей;
  Enter/Escape/Ctrl+C идут через `Dialog.HandleInputBinding`/`OnCancel`
  до фокус-диспатча; ✕ подключается движком через `SetCloser`.
  Локализация — ключи `dlg.*`, живое переключение через
  `Dialog.OnLanguageChange` (отписка в `SetModal(false)`).
- **Клавиши:** коды `widget.KeyCode` совпадают с VK-кодами Windows и
  `e.keyCode` браузера. Новую клавишу добавляй во ВСЕ маппинги:
  `window/native.go` (VK_*), `window/window.go` (vkToKeyCode),
  `window/native_linux.go` (X11-keycode; Wayland использует его же
  через +8), `window/native_darwin.go`.
- **Headless-тесты ввода:** юнит — прямые `w.OnKeyEvent/OnMouseButton`;
  сквозные — `eng.SetFocus(w)` + `eng.SendKeyEvent` (см. tests/*_test.go).

## Известные особенности среды

- `go vet` без `-unsafeptr=false` ругается на purego — это ожидаемо.
- Тесты создают движки без окон — работают в CI и WSL.
- X11/Wayland-бэкенды покрыты юнит-тестами парсеров (xkbmap) — живой
  Linux нужен только для ручной проверки окон.
- В Windows-шелле многострочные строки в heredoc/Edit могут ломаться на
  кодировке — питоновские патчи запускать как `py -3 -X utf8`.

## Документация при изменениях

Добавил фичу — обнови: README.md + README_RU.md (симметрично!),
GUIDE.md + GUIDE_EN.md (раздел «Новые возможности»), TODO.md (отметь
пункт с датой), этот файл — если появились новые правила/паттерны.
