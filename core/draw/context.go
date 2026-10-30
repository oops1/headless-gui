// Package draw — контекст рисования виджетов (DrawContext) и необязательные
// возможности контекста: сглаженные фигуры, тени, материалы, прозрачность
// слоя, блит пикселей, отсечение по повреждению. Реализует их холст движка
// (core/raster); в пакете widget эти имена — алиасы, и код, написанный под
// widget.DrawContext, менять не нужно (задача 4.1 плана 4.0).
//
// Пакет — нижний слой: импортирует только стандартную библиотеку и
// core/geom.
package draw

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v4/core/geom"
)

// DefaultFontSizePt — размер шрифта по умолчанию (в пунктах) для DrawText.
// Должен совпадать с engine.DefaultFontSize.
const DefaultFontSizePt = 10.0

// AAShapes — опциональные сглаженные (antialiased) примитивы рисования.
// Реализуется engine.Canvas. Кастомные реализации DrawContext могут не
// поддерживать интерфейс — код виджетов проверяет его type-assert'ом и
// откатывается на ступенчатую отрисовку (Bresenham/scanline).
type AAShapes interface {
	// FillEllipseAA — залитый эллипс с центром (cx, cy) и полуосями rx, ry.
	FillEllipseAA(cx, cy, rx, ry int, col color.RGBA)
	// StrokeEllipseAA — контур эллипса толщиной thickness.
	StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA)
	// FillPolygonAA — залитый многоугольник.
	FillPolygonAA(pts []image.Point, col color.RGBA)
	// StrokePolylineAA — ломаная толщиной thickness (closed замыкает контур).
	StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA)
	// DrawLineAA — отрезок толщиной thickness.
	DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA)
}

// Snapshotter — опциональный интерфейс DrawContext, умеющий скопировать
// прямоугольную область уже отрисованного холста в самостоятельный *image.RGBA.
// Реализуется engine.Canvas (по образцу AAShapes: код виджетов проверяет его
// type-assert'ом и откатывается, если контекст интерфейс не поддерживает).
//
// Координаты r — ЛОГИЧЕСКИЕ; результат — в физических пикселях холста
// (premultiplied RGBA), пригодных для обратного блита через DrawImage/
// DrawImageScaled. Пустое пересечение r с холстом → nil.
//
// Используется DockManager.DrawOverlay для «призрака» перетаскиваемой панели
// (снимок панели следует за курсором, как в Visual Studio).
type Snapshotter interface {
	Snapshot(r image.Rectangle) *image.RGBA
}

// OffscreenRenderer — опциональная возможность контекста отрисовать draw в
// отдельную картинку области r (логические координаты) на ПРОЗРАЧНОМ фоне,
// не трогая кадр: кэш растра элемента, который потом компонуют
// DrawImageRect (переход живой плитки, ADR-0010). Результат — физические
// пиксели от (0,0), premultiplied RGBA, размером с физическую область r.
// nil — контекст не умеет или область r не помещается в кадр целиком;
// вызывающий тогда рисует без кэша.
//
// draw получает контекст картинки — с теми же шрифтами и масштабом, что
// кадр; рисовать в нём надо в координатах кадра (r.Min — левый верхний угол
// картинки).
type OffscreenRenderer interface {
	RenderOffscreen(r image.Rectangle, draw func(ctx DrawContext)) *image.RGBA
}

// BackdropDrawer — опциональная возможность контекста размыть то, что уже
// нарисовано под слоем (acrylic и mica Windows 11, материалы macOS).
// Реализуется engine.Canvas методом BlurBehind; контекст без размытия
// компонент обходит подкраской — стекло становится плёнкой, но отрисовка
// не ломается.
type BackdropDrawer interface {
	BlurBehind(r image.Rectangle, radius int, tint color.RGBA)
}

// NoiseDrawer — опциональная возможность контекста нанести на область слабый
// зернистый шум (реализуется engine.Canvas методом DrawNoise). Это последний
// штрих acrylic Windows 10: размытая подкрашенная подложка без зерна
// выглядит гладким пластиком. amount — амплитуда в долях полной шкалы
// яркости (0.02 — ±2 %). Шум детерминирован и привязан к пикселям кадра, а не
// к области: соседние слои дают одно зерно, и оно не «плывёт» при сдвиге.
// Контекст без шума компонент пропускает — зерно украшение, не раскладка.
type NoiseDrawer interface {
	DrawNoise(r image.Rectangle, amount float64)
}

// RoundClipper — опциональная возможность контекста обрезать рисование по
// скруглённому контуру, а не по охватывающему прямоугольнику.
//
// Нужна там, где слой заливается не фигурой, а областью: размытая подложка
// берёт прямоугольник, и у скруглённого слоя за дугой оставались бы
// необрезанные уголки размытия — на стекле это выглядит как оборванные углы.
// Реализуется engine.Canvas; контекст без него рисует как раньше.
//
// SetRoundClip подменяет и прямоугольный клип (равносильно SetClip(r) плюс
// скругление), а ClearRoundClip снимает только скругление. Прежний клип
// вызывающий возвращает сам — иначе всё, что рисуется дальше, останется
// обрезанным границами слоя:
//
//	prev := ctx.Clip()
//	rc.SetRoundClip(r, radius)
//	... // рисование слоя
//	rc.ClearRoundClip()
//	ctx.SetClip(prev)
type RoundClipper interface {
	SetRoundClip(r image.Rectangle, radius int)
	ClearRoundClip()
}

// ShadowDrawer — опциональная возможность контекста рисовать мягкую тень
// (реализуется engine.Canvas методом DrawSoftShadow). Контекст без неё
// компонент просто пропускает: тень — украшение, её отсутствие не должно
// менять раскладку.
type ShadowDrawer interface {
	DrawSoftShadow(r image.Rectangle, corner int, elevation float64, col color.RGBA)
}

// RotatedTextDrawer — опциональная возможность контекста рисовать строку,
// повёрнутую на прямой угол: вертикальная надпись на боковой полосе меню
// «Пуск», подпись вертикальной вкладки (реализуется engine.Canvas методом
// DrawTextRotated).
//
// Строка рисуется так, будто DrawTextFont нарисовал её у точки (x, y) и весь
// отрезок повернули вокруг этой точки, — поэтому точке не нужна высота строки:
//
//   - 90  — против часовой стрелки, читается снизу вверх. (x, y) — левый
//     нижний угол надписи: она занимает x…x+высота_строки по горизонтали и
//     y-ширина…y по вертикали, верх букв смотрит влево;
//   - 270 — по часовой, читается сверху вниз. (x, y) — правый верхний угол:
//     надпись занимает x-высота_строки…x и y…y+ширина, верх букв смотрит вправо;
//   - 180 — вверх ногами, (x, y) — правый нижний угол; 0 — обычный DrawTextFont.
//
// Ширину строки даёт MeasureTextFont, высоту — FontMetrics/MeasureUIFontMetrics.
// Глифы растрируются в физическом размере и поворачиваются без передискретизации
// — на HiDPI краски букв остаются такими же чёткими, как у горизонтального
// текста (растр, повёрнутый приложением и растянутый движком, был мягким).
// Отсечение (SetClip, скруглённое) действует как обычно.
//
// Возвращает false, если угол не кратен 90°, а также если обёртка контекста не
// может передать вызов дальше (контекст внутри неё поворота не умеет): тогда
// ничего не нарисовано и вызывающий откатывается на своё рисование.
type RotatedTextDrawer interface {
	DrawTextRotated(text string, x, y int, sizePt float64, fontName string, angle int, col color.RGBA) bool
}

// OpacityDrawer — опциональная возможность контекста: выполнить рисование как
// слой с заданной прозрачностью (реализуется engine.Canvas методом
// DrawWithOpacity).
//
// draw рисует тем же контекстом как обычно, а контекст после него смешивает
// область r с тем, что в ней лежало до draw: alpha=1 — слой как нарисован,
// alpha=0 — его нет. Это прозрачность слоя целиком, а не каждого примитива:
// подложка со стеклом, тенью и текстом проявляется как одно целое.
//
// Контекст без этой возможности компонент обходит: слой появляется сразу, без
// проявления, — раскладка от этого не меняется.
type OpacityDrawer interface {
	DrawWithOpacity(r image.Rectangle, alpha float64, draw func())
}

// DrawContext — API рисования, предоставляемый движком каждому виджету.
// Реализуется типом engine.Canvas.
//
// Все координаты — абсолютные пиксели холста.
type DrawContext interface {
	// ── Примитивы ────────────────────────────────────────────────────────────

	// FillRect заливает прямоугольник (x, y, x+w, y+h) сплошным цветом.
	FillRect(x, y, w, h int, col color.RGBA)

	// FillRectAlpha заливает прямоугольник с альфа-смешиванием (Over).
	FillRectAlpha(x, y, w, h int, col color.RGBA)

	// FillRoundRect заливает прямоугольник со скруглёнными углами радиуса r.
	FillRoundRect(x, y, w, h, r int, col color.RGBA)

	// DrawBorder рисует 1-пиксельный контур прямоугольника.
	DrawBorder(x, y, w, h int, col color.RGBA)

	// DrawRoundBorder рисует 1-пиксельный контур со скруглёнными углами.
	DrawRoundBorder(x, y, w, h, r int, col color.RGBA)

	// SetPixel устанавливает цвет одного пикселя.
	SetPixel(x, y int, col color.RGBA)

	// DrawHLine рисует горизонтальную линию.
	DrawHLine(x, y, length int, col color.RGBA)

	// DrawVLine рисует вертикальную линию.
	DrawVLine(x, y, length int, col color.RGBA)

	// ── Изображения ──────────────────────────────────────────────────────────

	// DrawImage рисует изображение в (x, y) в оригинальном размере.
	DrawImage(src image.Image, x, y int)

	// DrawImageScaled рисует изображение масштабированным до (w × h) в (x, y).
	DrawImageScaled(src image.Image, x, y, w, h int)

	// ── Текст ────────────────────────────────────────────────────────────────

	// DrawText выводит строку TTF-шрифтом «default» кеглем по умолчанию
	// (DefaultFontSize — его задаёт тема или приложение).
	DrawText(text string, x, y int, col color.RGBA)

	// DrawTextSize выводит строку шрифтом «default» произвольного размера.
	DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA)

	// DrawTextFont выводит строку именованным шрифтом (RegisterFont).
	// fontName="" → шрифт по умолчанию.
	DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA)

	// MeasureText возвращает ширину строки в пикселях (шрифт default, sizePt).
	MeasureText(text string, sizePt float64) int

	// MeasureTextFont возвращает ширину строки именованным шрифтом.
	MeasureTextFont(text string, sizePt float64, fontName string) int

	// MeasureRunePositions возвращает накопленную ширину после каждого символа.
	// Результат: len(text)+1 элементов; positions[0]==0, positions[n] — ширина text[:n].
	MeasureRunePositions(text string, sizePt float64) []int

	// ── Clip ─────────────────────────────────────────────────────────────────

	// SetClip ограничивает все последующие операции рисования прямоугольником r.
	SetClip(r image.Rectangle)

	// ClearClip снимает ограничение области рисования.
	ClearClip()

	// Clip возвращает текущую область отсечения (или полные границы холста,
	// если отсечение не задано). Позволяет виджету сохранить и восстановить
	// внешнее отсечение при вложенном клиппинге (контейнеры в контейнерах).
	Clip() image.Rectangle
}

// ShadowParamDrawer — опциональная возможность контекста рисовать мягкую тень
// с явными радиусом размытия и смещением (токены тени профиля). Реализуется
// engine.Canvas методом DrawShadow. Контекст с одним ShadowDrawer получает
// близкое приближение через DrawSoftShadow.
type ShadowParamDrawer interface {
	DrawShadow(r image.Rectangle, corner int, blur, offsetX, offsetY float64, col color.RGBA)
}

// MicaDrawer — опциональная возможность контекста положить в область
// размытые обои (материал Mica). Реализуется engine.Canvas методом MicaBehind.
// Вернул false — обоев нет (фон движка не задан); слой тогда рисуют сплошным
// цветом темы.
type MicaDrawer interface {
	MicaBehind(r image.Rectangle, radius int, tint color.RGBA) bool
}

// ImageRectDrawer — опциональная возможность контекста рисовать часть
// изображения в прямоугольник с выбором фильтра, масштабируя только ту часть,
// что попадает под текущий клип.
//
// В отличие от DrawImageScaled, которому нужен весь источник и весь целевой
// размер (и кэш ради этого), DrawImageRect работает за время, пропорциональное
// ВИДИМОЙ площади: картинка 12000×8000 на зуме 400 % в окне 1200×800 стоит
// как 1200×800 пикселей, а не как 48000×32000.
//
//   - sr — прямоугольник в координатах источника (пиксели src); часть за
//     src.Bounds() отбрасывается вместе с пропорциональной частью dr;
//   - dr — прямоугольник назначения в ЛОГИЧЕСКИХ координатах контекста (как
//     x, y, w, h у DrawImageScaled); он может быть сколь угодно больше окна
//     и уходить за его края;
//   - f — фильтр.
//
// Реализуется engine.Canvas. Результат не кэшируется. Вызывающему удобнее
// помощник DrawImageRect: контексту без интерфейса он подставит
// DrawImageScaled от вырезанной части (с билинейным ядром и без выигрыша в
// скорости).
type ImageRectDrawer interface {
	DrawImageRect(src image.Image, sr, dr image.Rectangle, f geom.ImageFilter)
}

// GlyphCoverage — опциональная возможность контекста сказать, есть ли глиф
// руны в ШРИФТЕ ПО УМОЛЧАНИЮ (без шрифтов-замен). Реализуется engine.Canvas.
// Нужна виджету, который выбирает символ (маска пароля): символ из
// шрифта-замены может прийти цветным глифом и не следовать цвету текста.
type GlyphCoverage interface {
	HasGlyph(r rune) bool
}

// ImageCacheInvalidator — опциональная возможность контекста сбросить кэш
// масштабированных изображений (реализуется engine.Canvas). Нужна после
// того, как пиксели картинки переписаны на месте: кэш ключует источник по
// идентичности и содержимого не сравнивает.
type ImageCacheInvalidator interface {
	InvalidateImageCache(src image.Image)
}

// PixelBlitter — опциональная возможность контекста положить часть чужого
// *image.RGBA на холст КАК ЕСТЬ: пиксель в пиксель, без масштабирования и
// без кэша (v3.39, E7). Реализует engine.Canvas; обёртки пробрасывают.
//
// В отличие от DrawImage (логический размер = размер картинки в пикселях, при
// HiDPI растягивается на масштаб) здесь единица измерения буфера — ФИЗИЧЕСКИЙ
// пиксель холста. Сессия удалённого рабочего стола идёт в физическом
// разрешении, и растягивать её на 125–200 % значит мылить картинку.
//
//   - src — буфер приложения; sr — часть буфера (в координатах src, то есть
//     относительно src.Rect), которую надо положить; часть за src.Rect
//     отбрасывается;
//   - (x, y) — ЛОГИЧЕСКАЯ точка, в физическую позицию которой попадает пиксель
//     src.Rect.Min (округление по краям, как у остальных операций холста,
//     поэтому буфер стыкуется с соседними виджетами без щелей);
//   - opaque — все пиксели буфера имеют альфу 255 (см. OpaqueImage): тогда
//     область перезаписывается композицией Src, параллельно по полосам;
//     иначе — Over.
//
// Рисование подчиняется текущему клипу (в том числе скруглённому) и тратит
// время пропорционально ВИДИМОЙ части sr. Буфер не копируется и не
// запоминается: после возврата вызывающий снова им владеет.
type PixelBlitter interface {
	BlitPixels(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool)
}

// PixelBlitterBGRA — PixelBlitter для буфера, чьи байты лежат в порядке
// B,G,R,A (см. ExternalSurface.SetBGRA). Реализует engine.Canvas.
type PixelBlitterBGRA interface {
	BlitPixelsBGRA(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool)
}

// PathShapes — опциональные сглаженные пути с дробными координатами.
// Реализуется engine.Canvas (масштаб HiDPI холст учитывает сам).
//
// Ломаная обводится ОДНИМ контуром с круглыми сопряжениями во внутренних
// вершинах — на изгибе нет ни щели, ни зубца. Концы у незамкнутого пути
// прямые, как у DrawLineAA.
type PathShapes interface {
	// StrokeCubicAA — кубическая кривая Безье (x0,y0)…(x3,y3) толщиной thickness.
	StrokeCubicAA(x0, y0, x1, y1, x2, y2, x3, y3, thickness float64, col color.RGBA)
	// StrokePathAA — ломаная толщиной thickness (closed замыкает контур).
	StrokePathAA(pts []geom.Point2F, thickness float64, closed bool, col color.RGBA)
	// FillPathAA — залитый многоугольник.
	FillPathAA(pts []geom.Point2F, col color.RGBA)
}

// CullScope — то, что контекст отрисовки знает о текущем кадре: какие области
// перерисовываются и разрешён ли пропуск.
//
// Спрашивается у КОНТЕКСТА, а не у пакета. Раньше это были переменные на
// процесс, и два движка в одном процессе (окно и вынесенный в своё окно
// попап — обычное дело) перетирали их друг другу: движок мог пропустить
// поддерево по чужому damage и отдать недорисованный кадр.
//
// Реализуется engine.Canvas. Контекст без этих методов ограничений не
// ставит — обход идёт целиком, как до появления пропуска.
type CullScope interface {
	// DrawDamage — области кадра в ЛОГИЧЕСКИХ координатах. Пустой список
	// означает «рисуем всё».
	DrawDamage() []image.Rectangle
	// SubtreeCullingEnabled — разрешён ли пропуск поддеревьев.
	SubtreeCullingEnabled() bool
}

// OcclusionScope — то, что контекст отрисовки знает о вычитании перекрытого
// (occlusion.go). Отдельно от CullScope: у пропуска по damage и у вычитания
// перекрытого разные условия и разные способы ошибиться, и выключать их
// приходится порознь.
//
// Контекст без этого метода вычитание разрешает — так же, как это было до
// появления выключателя.
type OcclusionScope interface {
	OcclusionEnabled() bool
}

// DrawContextAlpha — расширение DrawContext для рисования с альфа-смешиванием.
// Реализуется engine.Canvas через метод FillRectAlpha.
type DrawContextAlpha interface {
	DrawContext
	FillRectAlpha(x, y, w, h int, col color.RGBA)
}
