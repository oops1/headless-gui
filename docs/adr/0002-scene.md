# ADR-0002. Сцена: движок как единственный компоновщик

Статус: принято 2026-10-26, ветка `feature/engine-architecture`, работы к 4.0
(этап 2, задача 2.1). Основание — `docs/ARCHITECTURE_REVIEW_RU.md`, §3.1 (W1,
W2) и §4.4; исследование WinLine `docs/RENDER_PIPELINE_STUDY.md`, §4.

## Контекст

Сессия WinLine (`cmd/winline-session`) показывает два вида окон: свои
(`widget.Window` в дереве движка) и чужие (буферы Wayland-клиентов в
`pkg/compositor`). Движок рисует только свои, и сессия собирает экран сама:

1. кадр движка копируется в слой `shell` попиксельно с перестановкой RGBA →
   BGRX (`blitTile`), хотя `SetPixelFormat(FormatBGRX)` есть с v3.15;
2. `recompose` копирует `shell` в `master` (`copyShell`), поверх кладёт окна
   клиентов (`compositor.CompositeAll`), поверх них снова панель задач и
   открытые панели (`restoreOverlays`), затем вторым проходом — то, что
   открылось из-под сдвинувшегося окна (`TakeStale`);
3. классификация тайлов движка под чужими окнами стирается
   (`kinds.forget`), потому что сессия не знает, что там нарисовано;
4. ввод маршрутизируется вручную: сессия сообщает компоновщику
   прямоугольники панели и открытых меню (`overlayRects` → `SetOverlays`) и
   прямоугольник своего окна с фокусом (`FocusedWindowBounds`), чтобы клик
   по ним не ушёл в программу под ними; `routePointer` держит, кто владеет
   нажатой кнопкой;
5. видео определяется эвристикой по частоте коммитов (12/с, гистерезис 5 с).

Это три полные копии кадра на кадр, второй проход при каждом движении окна
и два Z-порядка, которые сессия сводит руками. Движок при этом уже умеет
почти всё нужное: `widget.ExternalSurface` показывает чужой буфер без копии
(v3.39), `OpaqueRegioner` даёт вычитание перекрытого, `MoveRegion` —
перенос, `Regions` — классификацию, `SetSurface` — рисование в чужую память.

## Решение

### Сцена — это дерево виджетов, а не новый обход

Отдельного механизма «сцены» не будет. Дерево движка уже даёт Z-порядок
(порядок детей), hit-test сверху вниз, вычитание перекрытого по
`OpaqueRegion`, damage от перестановки и удаления узла, переносы, damage по
подложкам. Окно программы становится **узлом этого дерева**, как и окно
оболочки. Сессия WinLine строит:

```
root (Panel)
├── обои                         Engine.SetBackground
├── windows (Panel)              окна, свои и чужие, в Z-порядке детей
│   ├── Window{Content: ExternalSurface}   окно программы с рамкой темы
│   ├── Window{...}                        окно оболочки (терминал)
│   └── ExternalSurface                    полноэкранное окно, без рамки
└── chrome (Panel)               панель задач, флайауты, меню — всегда выше
```

Модальный слой движка (`ShowModal`) лежит выше всего, включая чужие окна:
защищённый рабочий стол — это модальная обёртка над `ExternalSurface`
диалога, затемнение движок рисует сам. Пакет `core/scene` в плане работ —
это место, куда на этапе 7 переедут `moveapply`, `backdropdamage`, тени и
попапы; новых обходов в нём не будет.

Для режима А (программа с окном ОС) ничего не меняется: сцена вырождается
в один слой дерева, как сейчас.

### Что добавляется

**1. Внешняя поверхность принимает ввод и подсказки клиента** (задачи 2.2,
2.3). `widget.ExternalSurface` реализует `MouseMoveHandler`,
`MouseClickHandler`, `WheelEventHandler`, `KeyHandler`, `Focusable`,
`PointerLeaveHandler`, `TouchHandler`, `CursorProvider`, переадресуя
события в колбэки:

```go
type SurfaceInput struct {
    Move   func(x, y int)                    // координаты в кадре поверхности
    Button func(e MouseEvent) bool
    Wheel  func(e WheelEvent) bool
    Key    func(e KeyEvent)
    Touch  func(e TouchEvent) bool
    Focus  func(on bool)
    Leave  func()
    Cursor func(x, y int) Cursor             // nil — CursorDefault
}
func (s *ExternalSurface) SetInput(in SurfaceInput)
func (s *ExternalSurface) SetOpaqueRegion(rects []image.Rectangle) // от wl_surface.set_opaque_region, в координатах буфера
func (s *ExternalSurface) SetContentHint(h ContentHint)            // ContentUnknown, ContentStatic, ContentVideo
```

Без `SetInput` поверхность, как сейчас, ввода не принимает. Фокус
клавиатуры — фокус движка: клик по поверхности делает её `Focusable` с
фокусом, клавиши идут в `Key`, уход фокуса — `Focus(false)`. Захват мыши при
нажатии — захват движка, как у любого виджета. `SetOpaqueRegion` уточняет
`OpaqueRegion()`: без вызова поверхность непрозрачна целиком при `opaque`,
с вызовом — только в объявленных прямоугольниках.

Частоту `Swap`/`Update` поверхность считает сама: скользящее окно в одну
секунду; при `ContentUnknown` и частоте выше `engine.VideoRate` (по
умолчанию 12 в секунду, как у WinLine) содержимое считается видео, ниже
`engine.VideoRate/2` в течение `engine.VideoHold` (5 с) — снова нет.
`SetContentHint` от клиента (`wp_content_type_v1`) имеет приоритет.

**2. Z-порядок без пересоздания** (2.2). `Base.RaiseChild(w)`,
`Base.LowerChild(w)`, `Base.InsertChildAt(i, w)` переставляют ребёнка и
заявляют damage его поддерева. Поднятие окна при клике — `RaiseChild`, а не
`RemoveChild` + `AddChild`.

**3. Тень — свойство узла, рисует компоновщик** (2.5, 2.6).

```go
type ShadowCaster interface {
    Shadow() theme.ShadowSpec                 // IsZero — без тени
    ShadowShape() (r image.Rectangle, corner int)
}
```

`drawChildren` перед `Draw` ребёнка-`ShadowCaster` рисует его тень через
`ShadowDrawer` контекста. Растр тени строится один раз на ключ (радиус
угла, размытие, смещение, цвет) как девять плиток (9-slice) в
`core/raster`; тень любого размера — восемь блитов и заливка; кэш ограничен
числом записей и сбрасывается при смене темы. `widget.Window` реализует
`ShadowCaster` вместо своего `drawShadow`; `ExternalSurface` в обёртке
`Window` тень получает от обёртки, полноэкранная — без тени. `DrawMargin`
узла равен `Shadow().Extent()`, и `MoveRegion` при перетаскивании включает
поля тени. Тайл с тенью над сплошным фоном честно `RegionMixed`.

**4. Вывод: несколько буферов и видео в кадре** (2.3, 2.4).

```go
func (e *Engine) SetSurfaces(bufs [][]byte, stride int, f PixelFormat) error // 1–4 буфера
func (e *Engine) ReleaseSurface(i int)        // потребитель вернул буфер (FrameAck)
// output.Frame:
Surface int                                   // в какой буфер нарисован кадр; -1 — собственный
Videos  []VideoRegion                         // поверхности с видео в этом кадре
type VideoRegion struct {
    ID    uint32            // стабильный номер поверхности
    Rect  image.Rectangle   // где она на экране, физические пиксели, обрезано холстом
    Cover []image.Rectangle // что лежит над ней: окна выше и chrome, обрезано ей
}
// output.Region: RegionVideo RegionKind; Color при RegionVideo — ID поверхности
```

Движок чередует буферы сам. Перед рисованием кадра в буфер он копирует из
`front` всё, что менялось с тех пор, как этот буфер был задним в последний
раз (накопленный damage на буфер, как `s.pending[i]` в сессии сегодня).
Буфер, не возвращённый `ReleaseSurface`, не берётся: при `PacingExternal`
кадр ждёт `RequestFrame`, при тикерном темпе — пропускается с
`Stats.FramesSkipped`. Старый `SetSurface` с одним буфером остаётся и равен
`SetSurfaces` с одним.

`Videos` движок заполняет по Z дерева: `Cover` — объединение прямоугольников
узлов выше поверхности, пересечённое с ней. Это то, что сессия сегодня
считает в `framePlayers` через компоновщик. Плоскость с картинкой плеера
(нижняя половина буфера tilebus) остаётся делом сессии: движок в основной
буфер рисует содержимое поверхности как обычно, чтобы клиент без видеокодека
получил верные тайлы.

### Ввод

Hit-test движка по дереву: клик над панелью идёт в панель, над окном
программы — в `SurfaceInput.Button`, над окном оболочки — в его виджет.
`overlayRects`, `SetOverlays`, `FocusedWindowBounds`, `routePointer` в
сессии не нужны: Z-порядок один. `DismissPopups` при клике по чужому окну —
штатное закрытие оверлеев движка по клику мимо.

### Что остаётся у сессии WinLine

- `pkg/compositor` как сервер Wayland: буферы, поверхности, протокол,
  декорации как `Insets` для обёртки, курсор клиента, DnD, RAIL, захват
  указателя. Композиция пикселей из него уходит.
- Привязка `Toplevel` ↔ `Window{Content: ExternalSurface}`: создать при
  `map`, `SetBounds` при `configure`, `Swap` при `commit` с damage клиента,
  `RaiseChild` при активации, `RemoveChild` при `unmap`.
- `kinds.cut` по `Regions` (как сейчас) и плоскости плееров по
  `Frame.Videos`.
- RAIL (окна как отдельные картинки): остаётся на нынешнем пути через
  `compositor.PaintRailWindow`; в сцену не входит. Отдельное решение, когда
  Tandem дойдёт до E8.

### Чего решение не делает

- Не переносит композицию чужих окон в GPU: кадр нужен в системной памяти.
- Не даёт поверхности рисоваться без копии в `back`: `ExternalSurface`
  копирует изменившиеся области из буфера клиента, как в v3.39. Копия одна,
  вместо трёх.
- Не решает RAIL и локальный вывод через DRM/KMS: первый — отдельное
  решение, второй — дело стока (`FrameSink` и `SetSurfaces` достаточно).
- Не меняет размер холста: стол больше `MaxCanvasSide` (16384) не
  поддерживается, как и сейчас.

## Последствия

- Для сессии WinLine 1080p: одна запись кадра вместо трёх копий, нет
  попиксельной перестановки, нет второго прохода, классификация под окнами
  не теряется. Приёмка 2.11: счётчики `composedPx`/`againPx` исчезают, кадр
  сценария «перетаскивание окна» быстрее не менее чем на 30 % по
  `painter`.
- Для режима А: ноль изменений. `ExternalSurface` без `SetInput` ведёт себя
  как раньше; `SetSurface` с одним буфером работает как раньше.
- Риск: golden-тесты `widget.Window` при переносе тени из `drawShadow` в
  `ShadowCaster` — тень обязана совпасть в пределах 1/255 (2.5).
- Риск: Mica и размытие под чужим окном читают `front`, где лежит прошлый
  кадр поверхности, — это и сейчас так для `ExternalSurface`; новых
  ограничений нет.

## Брифы

| № | Задача | Исполнитель | Приёмка |
|---|---|---|---|
| 2.2 | `SurfaceInput`, интерфейсы ввода у `ExternalSurface`, `Base.RaiseChild/LowerChild/InsertChildAt` | Opus, ревью Fable | тесты: клик над панелью не доходит до поверхности под ней; клик по поверхности даёт ей фокус и `Button`; перетаскивание из поверхности на панель держит захват; `RaiseChild` заявляет damage и меняет hit-test |
| 2.3 | `SetOpaqueRegion`, `SetContentHint`, счётчик частоты и порог видео, `RegionVideo`, `Frame.Videos` с `Cover` по Z | Opus, ревью Fable | тесты: 30 `Swap`/с → `RegionVideo` и запись в `Videos`; 1/с → нет; окно над видео попадает в `Cover`; opaque-прямоугольник вычитает перекрытое |
| 2.4 | `SetSurfaces`, `ReleaseSurface`, `Frame.Surface`, damage на буфер, поведение без свободного буфера | Fable | тесты с двумя буферами и подтверждением по образцу tilebus; существующий `SetSurface` не меняет поведения |
| 2.5 | `ShadowCaster`, 9-slice-кэш в `core/raster`, `Window` через `ShadowCaster`, лимит кэша, сброс при смене темы | Opus, ревью Fable | golden `Window` в пределах 1/255; бенчмарк: без размытия на кадр |
| 2.6 | `DrawMargin` = `Extent`, `MoveRegion` с полями тени | Opus, ревью Fable | тест перетаскивания: один `MoveRegion`, ноль периметровых тайлов |
| 2.7 | Модальный слой над `ExternalSurface`: затемнение и перехват ввода | Opus | тест |
| 2.8 | Golden-сценарии: окно над поверхностью, поверхность над окном, chrome над обоими, тени на стыках | Opus | |
| 2.9 | Бенчмарк 0.1 до и после | Haiku | |
| 2.10 | Документация | Haiku | |
| 2.11 | Пилот в WinLine: сессия на сцене движка, `compose.go` удаляется | Fable | см. «Последствия» |
