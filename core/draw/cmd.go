package draw

// cmd.go — команды рисования и списки команд (ADR-0004, задача 4а.1).
//
// Draw виджета пишет не пиксели, а команды: Recorder (record.go) принимает
// вызовы DrawContext и складывает их в List виджета, Replay (replay.go)
// воспроизводит дерево списков в любой DrawContext. Команда хранит
// аргументы, действовавший клип и логический охват, по которому
// воспроизведение пропускает то, что не задевает повреждение кадра, а
// тайловый растеризатор (4а.2) раскладывает команды по тайлам.

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v4/core/geom"
)

// Op — код операции команды.
type Op uint8

const (
	OpFillRect Op = iota
	OpFillRectAlpha
	OpFillRoundRect
	OpDrawBorder
	OpDrawRoundBorder
	OpSetPixel
	OpHLine
	OpVLine
	OpDrawImage
	OpDrawImageScaled
	OpDrawImageRect
	OpText
	OpTextRotated
	OpFillEllipseAA
	OpStrokeEllipseAA
	OpFillPolygonAA
	OpStrokePolylineAA
	OpDrawLineAA
	OpStrokeCubicAA
	OpStrokePathAA
	OpFillPathAA
	OpSoftShadow
	OpShadow
	OpBlurBehind
	OpMica
	OpNoise
	OpBlitPixels
	OpBlitBGRA
	// OpOpacity — DrawWithOpacity: вложенный список Ext.Sub, область X/Y/W/H, доля F[0].
	OpOpacity
	// OpChild — ссылка на список ребёнка (Holder.CmdList() в момент
	// воспроизведения): так контейнер, записанный один раз, видит свежие
	// списки перезаписанных детей.
	OpChild
	// OpDirect — виджет, рисующий прямо в холст при воспроизведении
	// (DirectDrawer): его аргументы нельзя пережить Draw.
	OpDirect
)

// ClipState — клип в момент команды, как его задал виджет: прямоугольный
// (NoClip — снят) и скруглённый (Round пуст — снят).
type ClipState struct {
	NoClip bool
	Clip   image.Rectangle
	Round  image.Rectangle
	RoundR int
}

// Cmd — одна команда рисования. Поля общие на все операции: что из них
// значимо, определяет Op (см. Recorder). Частые аргументы лежат в самой
// команде (прямоугольник, цвет, строка), редкие — в Ext: команда короткая,
// а список в сотни команд копируется и обходится дёшево. Строки и картинки —
// по ссылке (картинка читается в момент воспроизведения), точки — копией.
type Cmd struct {
	Op   Op
	Flag bool
	// X, Y, W, H — прямоугольник или точка (у эллипсов — центр и полуоси, у
	// OpChild/OpDirect — сдвиг контекста записи); R — радиус (скругление,
	// размытие); I1, I2 — вторая точка линии, угол поворота.
	X, Y, W, H, R int32
	I1, I2        int32
	// F — кегль, толщина, доля прозрачности, высота тени, сила шума.
	F       [2]float64
	Col     color.RGBA
	S, Font string
	Holder  ListHolder // OpChild, OpDirect
	// Clip — клип в момент команды; команды под одним клипом делят одно
	// состояние (рекордер выдаёт новое только при смене).
	Clip *ClipState
	// BBox — логический охват отрисовки с учётом клипа (пустой — команда
	// ничего не рисует и не записывается).
	BBox image.Rectangle
	// Ext — редкие аргументы (картинки, точки, кривая, тень, вложенный
	// список); nil у большинства команд.
	Ext *CmdExt
	// sum — хэш команды, посчитанный при записи (Sum).
	sum uint64
}

// CmdExt — редкие аргументы команды.
type CmdExt struct {
	Img  image.Image
	RGBA *image.RGBA
	// Rect — второй прямоугольник: приёмник у DrawImageRect, область
	// источника у блита.
	Rect   image.Rectangle
	Pts    []image.Point
	PtsF   []geom.Point2F
	F      [9]float64 // кривая (x0 y0 … x3 y3, толщина); тень (размытие, сдвиг x, y)
	Filter geom.ImageFilter
	Sub    *List // OpOpacity
}

// Rect — прямоугольник команды из X, Y, W, H (логический).
func (c *Cmd) Rect() image.Rectangle {
	return image.Rect(int(c.X), int(c.Y), int(c.X+c.W), int(c.Y+c.H))
}

func (c *Cmd) rect() image.Rectangle { return c.Rect() }

// Env — окружение записи списка: клип и сдвиг контекста в точке, где
// контейнер позвал Draw ребёнка. Список воспроизводится без Draw только в
// том же окружении (ADR-0004, И1).
type Env struct {
	Clip ClipState
	Off  image.Point
}

// List — команды одного виджета: собственные и ссылки на списки детей в
// порядке рисования.
type List struct {
	Cmds []Cmd
	// BBox — объединение охватов команд и списков детей на момент записи;
	// воспроизведение пропускает список целиком, если он не задевает
	// повреждение.
	BBox image.Rectangle
	// env — окружение записи; gen и rec — поколение и рекордер, записавшие
	// список: другой движок или полная инвалидация делают его устаревшим.
	env Env
	gen uint64
	rec *Recorder
	// occlusionSkipped — контейнер не записал ссылку на кого-то из детей,
	// потому что того закрыли непрозрачные соседи: решение зависит от соседей
	// и пересматривается, когда они перезаписываются.
	occlusionSkipped bool
	// exit — клип, который Draw виджета оставил на выходе: родитель пишет
	// команды после ссылки под ним (виджет вправе снять отсечение — GUIDE,
	// «Вложенное отсечение»), и удержанный список обязан оставлять тот же.
	exit ClipState
	// rows — индекс команд большого списка по полосам тайлов (rowIndex):
	// тайл обходит только свою полосу, а не все команды корня.
	rows *rowIndex
	// owner — холдер, чей это список (идентичность слоя размытия в кэше:
	// переживает перезапись списка).
	owner ListHolder
}

// Owner — холдер списка (nil у списка без холдера).
func (l *List) Owner() ListHolder {
	if l == nil {
		return nil
	}
	return l.owner
}

// rowIndexMin — с какого числа команд список получает индекс по полосам.
const rowIndexMin = 256

// rowIndex — команды списка по полосам высотой в тайл (физические 64 px):
// bands[i] — возрастающие индексы команд, чей охват задевает полосу i.
type rowIndex struct {
	scale float64
	bands [][]int32
}

// buildRows строит индекс по полосам для масштаба scale (логический охват
// команды переводится в физические полосы с округлением наружу).
func (l *List) buildRows(scale float64) {
	if len(l.Cmds) < rowIndexMin {
		l.rows = nil
		return
	}
	const ts = 64
	band := func(y int) int {
		py := float64(y) * scale
		if py < 0 {
			py = 0
		}
		return int(py) / ts
	}
	n := band(l.BBox.Max.Y-1) + 1
	if n < 1 || n > 4096 {
		l.rows = nil
		return
	}
	bands := make([][]int32, n)
	for i := range l.Cmds {
		b := l.Cmds[i].BBox
		if b.Empty() {
			continue
		}
		r0, r1 := band(b.Min.Y), band(b.Max.Y-1)
		if r1 >= n {
			r1 = n - 1
		}
		for r := r0; r <= r1; r++ {
			bands[r] = append(bands[r], int32(i))
		}
	}
	l.rows = &rowIndex{scale: scale, bands: bands}
}

// eachInBands зовёт fn для индексов двух соседних полос по возрастанию:
// порядок рисования сохраняется, повторов нет; false от fn обрывает обход.
func eachInBands(a, b []int32, fn func(i int32) bool) bool {
	for len(a) > 0 || len(b) > 0 {
		var i int32
		switch {
		case len(b) == 0 || len(a) > 0 && a[0] < b[0]:
			i, a = a[0], a[1:]
		case len(a) == 0 || b[0] < a[0]:
			i, b = b[0], b[1:]
		default: // одна и та же команда в обеих полосах
			i, a, b = a[0], a[1:], b[1:]
		}
		if !fn(i) {
			return false
		}
	}
	return true
}

// bandsFor возвращает команды списка, задевающие полосу (или две соседние —
// логическое повреждение тайла на дробном масштабе округлено наружу и
// задевает соседку), которым принадлежит логический прямоугольник r;
// ok=false — индекса нет или r шире двух полос (тогда обходится весь список).
func (l *List) bandsFor(r image.Rectangle) (a, b []int32, ok bool) {
	ri := l.rows
	if ri == nil {
		return nil, nil, false
	}
	const ts = 64
	y0 := int(float64(r.Min.Y)*ri.scale) / ts
	y1 := int(float64(r.Max.Y-1)*ri.scale) / ts
	if y0 < 0 || y1 < y0 || y1-y0 > 1 || y0 >= len(ri.bands) {
		return nil, nil, false
	}
	a = ri.bands[y0]
	if y1 > y0 && y1 < len(ri.bands) {
		b = ri.bands[y1]
	}
	return a, b, true
}

// Exit — клип на выходе из Draw виджета (для тестов).
func (l *List) Exit() ClipState {
	if l == nil {
		return ClipState{}
	}
	return l.exit
}

// Len — число команд верхнего уровня (включая ссылки).
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.Cmds)
}

// HasRef сообщает, есть ли в списке ссылка на холдер h (для тестов).
func (l *List) HasRef(h ListHolder) bool {
	if l == nil {
		return false
	}
	for i := range l.Cmds {
		if (l.Cmds[i].Op == OpChild || l.Cmds[i].Op == OpDirect) && l.Cmds[i].Holder == h {
			return true
		}
	}
	return false
}

// Gen — поколение записи списка (для тестов).
func (l *List) Gen() uint64 {
	if l == nil {
		return 0
	}
	return l.gen
}

// OcclusionSkipped — см. поле.
func (l *List) OcclusionSkipped() bool { return l != nil && l.occlusionSkipped }

// Drawer — то, что умеет рисовать в контекст (widget.Widget).
type Drawer interface {
	Draw(ctx DrawContext)
}

// ListHolder — виджет, хранящий свой список команд (widget.Base).
// Рекордер хранит список у холдера и ссылается на холдер из списка
// родителя: при воспроизведении берётся текущий список.
type ListHolder interface {
	Drawer
	// CmdList — удержанный список (nil — ещё не записывался).
	CmdList() *List
	// SetCmdList публикует новый список (после нормального возврата Draw).
	SetCmdList(*List)
	// MarkDirty помечает виджет устаревшим (Recorder.Abort после паники).
	MarkDirty()
	// NoteRef запоминает список родителя, в который попала ссылка на виджет
	// (или решение его пропустить): пока родитель держит этот список, пометка
	// виджета поднимается к родителю как subtreeDirty, иначе родитель должен
	// перезаписаться сам — в его текущем списке виджета нет.
	NoteRef(parent *List)
}

// DirectDrawer — виджет, которого нельзя записать: его Draw читает данные,
// живущие только во время вызова (буфер ExternalSurface под замком). При
// воспроизведении он рисуется прямо в холст, удержания у него нет.
type DirectDrawer interface {
	DrawDirect() bool
}

// Retainer — контекст записи с удержанием списков; его реализует Recorder,
// widget.DrawTraced идёт через него. Контекст со сдвигом (OffsetContext)
// Retainer-ом не становится: DrawTraced находит под обёртками рекордер и
// передаёт ему накопленный сдвиг off — команды ребёнка уже записаны
// сдвинутыми, а ссылка на его список хранит сдвиг как часть окружения.
type Retainer interface {
	// CanReuse сообщает, верен ли удержанный список h в текущем окружении
	// (И1): записан этим рекордером на текущем поколении, под тем же клипом и
	// сдвигом off, и ничейное повреждение кадра не задевает bounds — охват
	// виджета с поддеревом в его системе координат.
	CanReuse(h ListHolder, bounds image.Rectangle, off image.Point) bool
	// Keep добавляет в текущий список ссылку на удержанный список h.
	Keep(h ListHolder, off image.Point)
	// BeginRecord открывает новый список для h под сдвигом off; за ним
	// следует Draw виджета в этот контекст и EndRecord. Паника между ними
	// отменяется Recorder.Abort (движок зовёт его на границе кадра).
	BeginRecord(h ListHolder, off image.Point)
	// EndRecord публикует список h и добавляет ссылку на него в список
	// родителя.
	EndRecord(h ListHolder)
	// Direct записывает команду прямого рисования h (DirectDrawer) с охватом
	// bounds под сдвигом off.
	Direct(h ListHolder, bounds image.Rectangle, off image.Point)
	// WalkRetained обходит ссылки удержанного списка h: для каждого ребёнка
	// зовёт visit в окружении его ссылки — с клипом записи и её сдвигом
	// (внутри visit — DrawTraced ребёнка в контексте с тем же сдвигом).
	// Возвращает true, если список h устарел: перезаписанный ребёнок теперь
	// оставляет на выходе другой клип, и всё, что записано после него, лежало
	// бы под другим отсечением.
	WalkRetained(h ListHolder, visit func(child ListHolder, off image.Point)) bool
	// NoteSkip отмечает в текущем списке, что ребёнок h не рисуется: по
	// occlusion (решение зависит от соседей — список пересматривается, когда
	// кто-то из детей перезаписывается) или по собственному состоянию
	// контейнера (прокручен за край).
	NoteSkip(h ListHolder, occlusion bool)
	// Parent — холдер, чей список записывается или обходится сейчас (nil —
	// корень).
	Parent() ListHolder
}
