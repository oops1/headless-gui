package draw

// record.go — Recorder: DrawContext, который пишет команды в списки виджетов
// с удержанием (ADR-0004). Измерения и всё, по чему виджет ветвит рисование
// (масштаб, метрики, наличие глифа, обои), отдаются холсту движка через
// Measurer — это только чтение. Пиксели рекордер не трогает (И3).

import (
	"image"
	"image/color"
	"math"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/core/geom"
)

// Measurer — то, что рекордер спрашивает у холста движка.
type Measurer interface {
	MeasureText(text string, sizePt float64) int
	MeasureTextFont(text string, sizePt float64, fontName string) int
	MeasureRunePositions(text string, sizePt float64) []int
	FontMetrics(fontName string, sizePt float64) (ascent, descent, lineGap int)
	HasGlyph(r rune) bool
	// TextBounds — верхняя оценка охвата строки (без измерения).
	TextBounds(text string, x, y int, sizePt float64, fontName string) image.Rectangle
	// LogicalClip — логический прямоугольник клипа r с округлением холста
	// (как Canvas.Clip после SetClip(r)), без базового клипа кадра.
	LogicalClip(r image.Rectangle) image.Rectangle
	LogicalSize() (w, h int)
	Scale() float64
	HasWallpaper() bool
	OcclusionEnabled() bool
	InvalidateImageCache(src image.Image)
}

// frameKind — что лежит на стеке рекордера: запись списка холдера или
// обход его удержанного списка (WalkRetained).
type frameKind uint8

const (
	frameRecord frameKind = iota
	frameWalk
)

type frame struct {
	kind   frameKind
	holder ListHolder
	list   *List
	off    image.Point // сдвиг записи списка (для ссылки на него)
	groups int         // глубина стека групп при открытии
}

// Recorder — записывающий контекст одного движка. Живёт между кадрами: его
// поколение помечает списки, записанные им. Измеритель задаётся на кадр
// (движок меняет холст при смене масштаба и размера).
type Recorder struct {
	m    Measurer
	w, h int

	// gen — поколение записи; атомарное: Invalidate зовут из любой горутины,
	// кадр читает его на своей.
	gen    atomic.Uint64
	retain bool
	force  []image.Rectangle

	root   *List
	frames []frame
	groups []*List // вложенные списки DrawWithOpacity
	// clip — текущее состояние клипа по указателю: команды под одним клипом
	// делят его; clips — пачка состояний (newClip).
	clip  *ClipState
	clips []ClipState
	exit  ClipState

	// Recorded, Retained — за кадр: команд записано и команд верхнего уровня
	// взято из удержанных списков (для статистики движка; сбрасываются в
	// BeginFrame).
	Recorded, Retained uint64
	// rerecorded — перезаписей за кадр.
	rerecorded int
	// exitChanged — в текущем WalkRetained ребёнок перезаписался и оставляет
	// другой клип на выходе: список родителя устарел.
	exitChanged bool
}

var (
	_ DrawContext           = (*Recorder)(nil)
	_ Retainer              = (*Recorder)(nil)
	_ AAShapes              = (*Recorder)(nil)
	_ PathShapes            = (*Recorder)(nil)
	_ ShadowDrawer          = (*Recorder)(nil)
	_ ShadowParamDrawer     = (*Recorder)(nil)
	_ BackdropDrawer        = (*Recorder)(nil)
	_ MicaDrawer            = (*Recorder)(nil)
	_ NoiseDrawer           = (*Recorder)(nil)
	_ RoundClipper          = (*Recorder)(nil)
	_ OpacityDrawer         = (*Recorder)(nil)
	_ RotatedTextDrawer     = (*Recorder)(nil)
	_ ImageRectDrawer       = (*Recorder)(nil)
	_ PixelBlitter          = (*Recorder)(nil)
	_ PixelBlitterBGRA      = (*Recorder)(nil)
	_ GlyphCoverage         = (*Recorder)(nil)
	_ ImageCacheInvalidator = (*Recorder)(nil)
	_ CullScope             = (*Recorder)(nil)
	_ OcclusionScope        = (*Recorder)(nil)
)

// NewRecorder создаёт рекордер.
func NewRecorder() *Recorder {
	r := &Recorder{}
	r.gen.Store(1)
	return r
}

// Invalidate помечает все списки, записанные этим рекордером, устаревшими
// (полная инвалидация движка). Из любой горутины.
func (r *Recorder) Invalidate() { r.gen.Add(1) }

// BeginFrame готовит рекордер к кадру над холстом m: retain — брать ли
// удержанные списки; force — ничейное повреждение кадра (логическое):
// холдер, чей охват его задевает, перезаписывается.
func (r *Recorder) BeginFrame(m Measurer, retain bool, force []image.Rectangle) {
	r.m = m
	r.w, r.h = m.LogicalSize()
	r.retain = retain
	r.force = force
	r.root = nil
	r.frames = r.frames[:0]
	r.groups = r.groups[:0]
	r.clip = r.newClip(ClipState{NoClip: true})
	r.Recorded, r.Retained = 0, 0
	r.rerecorded = 0
}

// EndFrame завершает запись: возвращает список корня и клип на выходе из
// Draw корня (воспроизведение приводит холст к нему).
func (r *Recorder) EndFrame() (*List, ClipState) {
	r.exit = *r.clip
	r.m, r.force = nil, nil // между кадрами холст не держим
	return r.root, r.exit
}

// Abort отменяет кадр после паники в Draw: открытые холдеры снова dirty,
// их списки не публикуются (И4).
func (r *Recorder) Abort() {
	for i := len(r.frames) - 1; i >= 0; i-- {
		if r.frames[i].kind == frameRecord {
			r.frames[i].holder.MarkDirty()
		}
	}
	r.frames = r.frames[:0]
	r.groups = r.groups[:0]
	r.root = nil
	r.m, r.force = nil, nil
}

// Gen — поколение записи.
func (r *Recorder) Gen() uint64 { return r.gen.Load() }

// ─── Retainer ────────────────────────────────────────────────────────────────

// top — верхний кадр стека (nil — запись корня).
func (r *Recorder) top() *frame {
	if n := len(r.frames); n > 0 {
		return &r.frames[n-1]
	}
	return nil
}

// target — список, куда пишутся команды сейчас: группа DrawWithOpacity,
// открытая в записываемом списке, либо он сам; nil — запись не идёт (корень
// без холдера или обход удержанного списка).
func (r *Recorder) target() *List {
	t := r.top()
	if t == nil || t.kind != frameRecord {
		return nil
	}
	if n := len(r.groups); n > t.groups {
		return r.groups[n-1]
	}
	return t.list
}

// holderList — список холдера на вершине стека: тот, в который попадает
// ссылка на ребёнка (или откуда она читается при обходе); nil у корня.
func (r *Recorder) holderList() *List {
	if t := r.top(); t != nil {
		return t.list
	}
	return nil
}

func (r *Recorder) forceOverlaps(bounds image.Rectangle, off image.Point) bool {
	if len(r.force) == 0 {
		return false
	}
	b := bounds.Sub(off)
	if b.Empty() {
		return true // неизвестно, где это, — перестраховка
	}
	for _, f := range r.force {
		if b.Overlaps(f) {
			return true
		}
	}
	return false
}

// CanReuse — см. Retainer.
func (r *Recorder) CanReuse(h ListHolder, bounds image.Rectangle, off image.Point) bool {
	if !r.retain {
		return false
	}
	l := h.CmdList()
	return l != nil && l.gen == r.gen.Load() && l.rec == r &&
		l.env == (Env{Clip: *r.clip, Off: off}) && !r.forceOverlaps(bounds, off)
}

// HasForce сообщает, есть ли у кадра ничейное повреждение (тогда виджету
// нужен охват для ForceHits).
func (r *Recorder) HasForce() bool { return len(r.force) > 0 }

// ForceHits сообщает, задевает ли ничейное повреждение кадра виджет с
// охватом bounds под сдвигом off.
func (r *Recorder) ForceHits(bounds image.Rectangle, off image.Point) bool {
	return r.forceOverlaps(bounds, off)
}

// Keep — см. Retainer. После ссылки текущий клип — тот, что Draw ребёнка
// оставил бы на выходе: родитель пишет дальше под ним, как на прямом пути.
func (r *Recorder) Keep(h ListHolder, off image.Point) {
	l := h.CmdList()
	if l == nil {
		return
	}
	if r.target() != nil || len(r.frames) == 0 {
		r.Retained += uint64(len(l.Cmds))
	}
	r.addRef(h, l, off)
	r.clip = r.clipFor(l.exit)
}

// addRef добавляет ссылку на список холдера в текущий список (или делает
// его корнем), запоминает у холдера список, в котором он числится, и
// расширяет охваты объемлющих списков. При обходе удержанного родителя
// (frameWalk) ссылка уже в его списке — добавлять нечего.
//
// Клип ссылки — клип окружения списка, то есть на входе в Draw ребёнка, а не
// текущий: Draw мог оставить другой (ScrollView возвращает прочитанный
// Clip() явным прямоугольником вместо снятого), и ссылка с ним не совпала бы
// с окружением при следующей проверке.
func (r *Recorder) addRef(h ListHolder, l *List, off image.Point) {
	if t := r.target(); t != nil {
		t.Cmds = append(t.Cmds, Cmd{Op: OpChild, Holder: h, Clip: r.clipFor(l.env.Clip), X: int32(off.X), Y: int32(off.Y), BBox: l.BBox})
	} else if len(r.frames) == 0 {
		r.root = l
	}
	h.NoteRef(r.holderList())
	r.unionUp(l.BBox)
}

// unionUp расширяет охват всех открытых списков (записываемых и обходимых).
func (r *Recorder) unionUp(b image.Rectangle) {
	if b.Empty() {
		return
	}
	for i := range r.frames {
		r.frames[i].list.BBox = r.frames[i].list.BBox.Union(b)
	}
	for _, g := range r.groups {
		g.BBox = g.BBox.Union(b)
	}
}

// BeginRecord — см. Retainer. Новый список получает ёмкость по прежнему:
// виджет обычно пишет столько же команд, сколько в прошлый раз, и список
// растёт без переаллокаций.
func (r *Recorder) BeginRecord(h ListHolder, off image.Point) {
	l := &List{env: Env{Clip: *r.clip, Off: off}, gen: r.gen.Load(), rec: r}
	if old := h.CmdList(); old != nil && len(old.Cmds) > 0 {
		n := len(old.Cmds)
		l.Cmds = make([]Cmd, 0, n+n/8+2)
	}
	r.frames = append(r.frames, frame{kind: frameRecord, holder: h, list: l, off: off, groups: len(r.groups)})
}

// EndRecord — см. Retainer.
func (r *Recorder) EndRecord(h ListHolder) {
	t := r.top()
	if t == nil || t.kind != frameRecord || t.holder != h {
		panic("draw.Recorder: EndRecord без парного BeginRecord")
	}
	r.groups = r.groups[:t.groups]
	l := t.list
	l.exit = *r.clip
	r.frames = r.frames[:len(r.frames)-1]
	if len(l.Cmds) >= rowIndexMin {
		l.buildRows(r.m.Scale())
	}
	l.owner = h
	h.SetCmdList(l)
	r.Recorded += uint64(len(l.Cmds))
	r.rerecorded++
	r.addRef(h, l, t.off)
}

// Direct — см. Retainer.
func (r *Recorder) Direct(h ListHolder, bounds image.Rectangle, off image.Point) {
	b := bounds.Sub(off).Intersect(r.clipRect())
	c := Cmd{Op: OpDirect, Holder: h, Clip: r.clip, X: int32(off.X), Y: int32(off.Y), BBox: b}
	if t := r.target(); t != nil {
		t.Cmds = append(t.Cmds, c)
		h.NoteRef(r.holderList())
		r.unionUp(b)
		return
	}
	if len(r.frames) > 0 {
		h.NoteRef(r.holderList()) // обход удержанного родителя: команда уже в его списке
		return
	}
	// Корень дерева — прямой виджет: список из одной команды.
	l := &List{env: Env{Clip: *r.clip, Off: off}, gen: r.gen.Load(), rec: r, BBox: b, Cmds: []Cmd{c}, owner: h}
	r.root = l
	h.NoteRef(nil)
}

// WalkRetained — см. Retainer.
func (r *Recorder) WalkRetained(h ListHolder, visit func(child ListHolder, off image.Point)) bool {
	l := h.CmdList()
	if l == nil {
		return false
	}
	saveClip, saveChanged := r.clip, r.exitChanged
	r.exitChanged = false
	r.frames = append(r.frames, frame{kind: frameWalk, holder: h, list: l, groups: len(r.groups)})
	r.walkList(l, visit)
	r.frames = r.frames[:len(r.frames)-1]
	r.clip = saveClip
	stale := r.exitChanged
	r.exitChanged = saveChanged
	return stale
}

// walkList зовёт visit для каждой ссылки списка (и вложенных групп
// DrawWithOpacity). Перезаписанный ребёнок мог вырасти: его охват
// объединяется в ссылку, группу и список — иначе воспроизведение пропустило
// бы их по старому охвату.
func (r *Recorder) walkList(l *List, visit func(child ListHolder, off image.Point)) {
	for i := range l.Cmds {
		c := &l.Cmds[i]
		switch c.Op {
		case OpChild:
			r.clip = c.Clip
			var wasExit ClipState
			if cl := c.Holder.CmdList(); cl != nil {
				wasExit = cl.exit
			}
			visit(c.Holder, image.Pt(int(c.X), int(c.Y)))
			if cl := c.Holder.CmdList(); cl != nil {
				c.BBox = c.BBox.Union(cl.BBox)
				l.BBox = l.BBox.Union(cl.BBox)
				if cl.exit != wasExit {
					r.exitChanged = true
				}
			}
		case OpOpacity:
			if sub := c.Ext.Sub; sub != nil {
				r.walkList(sub, visit)
				c.BBox = c.BBox.Union(sub.BBox)
				l.BBox = l.BBox.Union(sub.BBox)
			}
		}
	}
}

// NoteSkip — см. Retainer.
func (r *Recorder) NoteSkip(h ListHolder, occlusion bool) {
	t := r.top()
	if t == nil || t.kind != frameRecord {
		return
	}
	h.NoteRef(t.list)
	if occlusion {
		t.list.occlusionSkipped = true
	}
}

// ParentOcclusionSkipped сообщает, пропускает ли записываемый сейчас список
// родителя детей по occlusion: тогда ребёнку стоит запомнить свою
// непрозрачную область (widget/retain.go), чтобы при следующем визите
// родитель узнал, изменилась ли она.
func (r *Recorder) ParentOcclusionSkipped() bool {
	t := r.top()
	return t != nil && t.kind == frameRecord && t.list.occlusionSkipped
}

// Parent — см. Retainer.
func (r *Recorder) Parent() ListHolder {
	if t := r.top(); t != nil {
		return t.holder
	}
	return nil
}

// ─── Запись команд ───────────────────────────────────────────────────────────

// newClip выдаёт состояние клипа s по указателю. Состояния живут в списках
// дольше кадра, поэтому выделяются пачками и не переиспользуются.
func (r *Recorder) newClip(s ClipState) *ClipState {
	if len(r.clips) == cap(r.clips) {
		r.clips = make([]ClipState, 0, 64)
	}
	r.clips = append(r.clips, s)
	return &r.clips[len(r.clips)-1]
}

// setClip делает s текущим состоянием: новый указатель только при смене.
func (r *Recorder) setClip(s ClipState) {
	if *r.clip != s {
		r.clip = r.newClip(s)
	}
}

// clipFor — указатель на состояние s: текущий, если совпадает.
func (r *Recorder) clipFor(s ClipState) *ClipState {
	if *r.clip == s {
		return r.clip
	}
	return r.newClip(s)
}

// clipRect — действующий прямоугольный клип (логический, как у холста).
func (r *Recorder) clipRect() image.Rectangle {
	if r.clip.NoClip {
		return image.Rect(0, 0, r.w, r.h)
	}
	return r.clip.Clip.Intersect(image.Rect(0, 0, r.w, r.h))
}

// add записывает команду с текущим клипом; охват пересекается с клипом,
// пустой охват — команда ничего не нарисует и отбрасывается.
func (r *Recorder) add(c Cmd, bbox image.Rectangle) {
	t := r.target()
	if t == nil {
		return // вне записи (Draw вне DrawTraced): нечего хранить
	}
	bbox = bbox.Intersect(r.clipRect())
	if r.clip.Round != (image.Rectangle{}) {
		bbox = bbox.Intersect(r.clip.Round)
	}
	if bbox.Empty() {
		return
	}
	c.Clip = r.clip
	c.BBox = bbox
	c.sum = c.Hash(HashSeed)
	t.Cmds = append(t.Cmds, c)
	t.BBox = t.BBox.Union(bbox)
}

// ─── DrawContext ─────────────────────────────────────────────────────────────

func rectXYWH(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

// xywh — команда с прямоугольником (или точкой и размерами) и цветом.
func xywh(op Op, x, y, w, h int, col color.RGBA) Cmd {
	return Cmd{Op: op, X: int32(x), Y: int32(y), W: int32(w), H: int32(h), Col: col}
}

// rectCmd — команда с прямоугольником rc.
func rectCmd(op Op, rc image.Rectangle, col color.RGBA) Cmd {
	return xywh(op, rc.Min.X, rc.Min.Y, rc.Dx(), rc.Dy(), col)
}

func (r *Recorder) FillRect(x, y, w, h int, col color.RGBA) {
	if col.A == 0 {
		return
	}
	r.add(xywh(OpFillRect, x, y, w, h, col), rectXYWH(x, y, w, h))
}

func (r *Recorder) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	r.add(xywh(OpFillRectAlpha, x, y, w, h, col), rectXYWH(x, y, w, h))
}

func (r *Recorder) FillRoundRect(x, y, w, h, rad int, col color.RGBA) {
	c := xywh(OpFillRoundRect, x, y, w, h, col)
	c.R = int32(rad)
	r.add(c, rectXYWH(x, y, w, h))
}

func (r *Recorder) DrawBorder(x, y, w, h int, col color.RGBA) {
	r.add(xywh(OpDrawBorder, x, y, w, h, col), rectXYWH(x, y, w, h))
}

func (r *Recorder) DrawRoundBorder(x, y, w, h, rad int, col color.RGBA) {
	c := xywh(OpDrawRoundBorder, x, y, w, h, col)
	c.R = int32(rad)
	r.add(c, rectXYWH(x, y, w, h))
}

func (r *Recorder) SetPixel(x, y int, col color.RGBA) {
	r.add(xywh(OpSetPixel, x, y, 1, 1, col), rectXYWH(x, y, 1, 1))
}

func (r *Recorder) DrawHLine(x, y, length int, col color.RGBA) {
	r.add(xywh(OpHLine, x, y, length, 1, col), rectXYWH(x, y, length, 1))
}

func (r *Recorder) DrawVLine(x, y, length int, col color.RGBA) {
	r.add(xywh(OpVLine, x, y, 1, length, col), rectXYWH(x, y, 1, length))
}

func (r *Recorder) DrawImage(src image.Image, x, y int) {
	if src == nil {
		return
	}
	b := src.Bounds()
	c := xywh(OpDrawImage, x, y, b.Dx(), b.Dy(), color.RGBA{})
	c.Ext = &CmdExt{Img: src}
	r.add(c, rectXYWH(x, y, b.Dx(), b.Dy()))
}

func (r *Recorder) DrawImageScaled(src image.Image, x, y, w, h int) {
	if src == nil {
		return
	}
	c := xywh(OpDrawImageScaled, x, y, w, h, color.RGBA{})
	c.Ext = &CmdExt{Img: src}
	r.add(c, rectXYWH(x, y, w, h))
}

// DrawImageRect: область источника — в команде, приёмник — в Ext.Rect.
func (r *Recorder) DrawImageRect(src image.Image, sr, dr image.Rectangle, f geom.ImageFilter) {
	if src == nil {
		return
	}
	c := rectCmd(OpDrawImageRect, sr, color.RGBA{})
	c.Ext = &CmdExt{Img: src, Rect: dr, Filter: f}
	r.add(c, dr)
}

// DrawText — кеглем по умолчанию темы (DefaultFontSize), как холст.
func (r *Recorder) DrawText(text string, x, y int, col color.RGBA) {
	r.DrawTextFont(text, x, y, DefaultFontSize(), "", col)
}

func (r *Recorder) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	r.DrawTextFont(text, x, y, sizePt, "", col)
}

func (r *Recorder) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	if text == "" || col.A == 0 {
		return
	}
	c := Cmd{Op: OpText, X: int32(x), Y: int32(y), S: text, Font: fontName, Col: col}
	c.F[0] = sizePt
	r.add(c, r.m.TextBounds(text, x, y, sizePt, fontName))
}

func (r *Recorder) DrawTextRotated(text string, x, y int, sizePt float64, fontName string, angle int, col color.RGBA) bool {
	if turns := ((angle % 360) + 360) % 360; turns%90 != 0 {
		return false // как холст: виджет рисует сам
	}
	if text == "" || col.A == 0 {
		return true
	}
	tb := r.m.TextBounds(text, 0, 0, sizePt, fontName)
	s := tb.Dx() + tb.Dy()
	c := Cmd{Op: OpTextRotated, X: int32(x), Y: int32(y), S: text, Font: fontName, I1: int32(angle), Col: col}
	c.F[0] = sizePt
	r.add(c, image.Rect(x-s, y-s, x+s, y+s))
	return true
}

func (r *Recorder) MeasureText(text string, sizePt float64) int { return r.m.MeasureText(text, sizePt) }
func (r *Recorder) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return r.m.MeasureTextFont(text, sizePt, fontName)
}
func (r *Recorder) MeasureRunePositions(text string, sizePt float64) []int {
	return r.m.MeasureRunePositions(text, sizePt)
}
func (r *Recorder) FontMetrics(fontName string, sizePt float64) (ascent, descent, lineGap int) {
	return r.m.FontMetrics(fontName, sizePt)
}

func (r *Recorder) SetClip(rc image.Rectangle) {
	s := *r.clip
	s.NoClip, s.Clip = false, rc
	r.setClip(s)
}

func (r *Recorder) ClearClip() {
	s := *r.clip
	s.NoClip = true
	r.setClip(s)
}

// Clip — как у холста: при снятом клипе — логические границы, иначе клип с
// округлением холста на дробном масштабе.
func (r *Recorder) Clip() image.Rectangle {
	if r.clip.NoClip {
		return image.Rect(0, 0, r.w, r.h)
	}
	return r.m.LogicalClip(r.clip.Clip)
}

// SetRoundClip — как у холста: прямоугольная часть ставится тоже.
func (r *Recorder) SetRoundClip(rc image.Rectangle, radius int) {
	s := *r.clip
	s.NoClip, s.Clip, s.Round, s.RoundR = false, rc, rc, radius
	r.setClip(s)
}

func (r *Recorder) ClearRoundClip() {
	s := *r.clip
	s.Round, s.RoundR = image.Rectangle{}, 0
	r.setClip(s)
}

// Scale — масштаб холста движка (widget.ContextScale).
func (r *Recorder) Scale() float64 { return r.m.Scale() }

// ─── Необязательные возможности ──────────────────────────────────────────────

func ptsBounds(pts []image.Point, pad int) image.Rectangle {
	if len(pts) == 0 {
		return image.Rectangle{}
	}
	b := image.Rectangle{Min: pts[0], Max: pts[0].Add(image.Pt(1, 1))}
	for _, q := range pts[1:] {
		b = b.Union(image.Rectangle{Min: q, Max: q.Add(image.Pt(1, 1))})
	}
	return b.Inset(-pad)
}

func ptsFBounds(pts []geom.Point2F, pad int) image.Rectangle {
	if len(pts) == 0 {
		return image.Rectangle{}
	}
	x0, y0, x1, y1 := pts[0].X, pts[0].Y, pts[0].X, pts[0].Y
	for _, q := range pts[1:] {
		x0, y0 = math.Min(x0, q.X), math.Min(y0, q.Y)
		x1, y1 = math.Max(x1, q.X), math.Max(y1, q.Y)
	}
	return image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1))+1, int(math.Ceil(y1))+1).Inset(-pad)
}

func copyPts(pts []image.Point) []image.Point {
	out := make([]image.Point, len(pts))
	copy(out, pts)
	return out
}

func copyPtsF(pts []geom.Point2F) []geom.Point2F {
	out := make([]geom.Point2F, len(pts))
	copy(out, pts)
	return out
}

func thickPad(t float64) int { return int(math.Ceil(t)) + 2 }

func (r *Recorder) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	r.add(xywh(OpFillEllipseAA, cx, cy, rx, ry, col), image.Rect(cx-rx-1, cy-ry-1, cx+rx+2, cy+ry+2))
}

func (r *Recorder) StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA) {
	t := thickPad(thickness)
	c := xywh(OpStrokeEllipseAA, cx, cy, rx, ry, col)
	c.F[0] = thickness
	r.add(c, image.Rect(cx-rx-t, cy-ry-t, cx+rx+t+1, cy+ry+t+1))
}

func (r *Recorder) FillPolygonAA(pts []image.Point, col color.RGBA) {
	r.add(Cmd{Op: OpFillPolygonAA, Col: col, Ext: &CmdExt{Pts: copyPts(pts)}}, ptsBounds(pts, 1))
}

func (r *Recorder) StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA) {
	c := Cmd{Op: OpStrokePolylineAA, Flag: closed, Col: col, Ext: &CmdExt{Pts: copyPts(pts)}}
	c.F[0] = thickness
	r.add(c, ptsBounds(pts, thickPad(thickness)))
}

func (r *Recorder) DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA) {
	c := Cmd{Op: OpDrawLineAA, X: int32(x1), Y: int32(y1), I1: int32(x2), I2: int32(y2), Col: col}
	c.F[0] = thickness
	r.add(c, ptsBounds([]image.Point{{X: x1, Y: y1}, {X: x2, Y: y2}}, thickPad(thickness)))
}

func (r *Recorder) StrokeCubicAA(x0, y0, x1, y1, x2, y2, x3, y3, thickness float64, col color.RGBA) {
	c := Cmd{Op: OpStrokeCubicAA, Col: col, Ext: &CmdExt{F: [9]float64{x0, y0, x1, y1, x2, y2, x3, y3, thickness}}}
	r.add(c, ptsFBounds([]geom.Point2F{{X: x0, Y: y0}, {X: x1, Y: y1}, {X: x2, Y: y2}, {X: x3, Y: y3}}, thickPad(thickness)))
}

func (r *Recorder) StrokePathAA(pts []geom.Point2F, thickness float64, closed bool, col color.RGBA) {
	c := Cmd{Op: OpStrokePathAA, Flag: closed, Col: col, Ext: &CmdExt{PtsF: copyPtsF(pts)}}
	c.F[0] = thickness
	r.add(c, ptsFBounds(pts, thickPad(thickness)))
}

func (r *Recorder) FillPathAA(pts []geom.Point2F, col color.RGBA) {
	r.add(Cmd{Op: OpFillPathAA, Col: col, Ext: &CmdExt{PtsF: copyPtsF(pts)}}, ptsFBounds(pts, 1))
}

func (r *Recorder) DrawSoftShadow(rc image.Rectangle, corner int, elevation float64, col color.RGBA) {
	c := rectCmd(OpSoftShadow, rc, col)
	c.R = int32(corner)
	c.F[0] = elevation
	r.add(c, rc.Inset(-(int(math.Ceil(elevation*3)) + 8)))
}

func (r *Recorder) DrawShadow(rc image.Rectangle, corner int, blur, offsetX, offsetY float64, col color.RGBA) {
	c := rectCmd(OpShadow, rc, col)
	c.R = int32(corner)
	c.Ext = &CmdExt{F: [9]float64{blur, offsetX, offsetY}}
	moved := rc.Add(image.Pt(int(math.Round(offsetX)), int(math.Round(offsetY))))
	r.add(c, rc.Union(moved).Inset(-(int(math.Ceil(blur))*2 + 4)))
}

func (r *Recorder) BlurBehind(rc image.Rectangle, radius int, tint color.RGBA) {
	c := rectCmd(OpBlurBehind, rc, tint)
	c.R = int32(radius)
	r.add(c, rc)
}

// MicaBehind записывает слой и отвечает, как холст: есть ли обои.
func (r *Recorder) MicaBehind(rc image.Rectangle, radius int, tint color.RGBA) bool {
	c := rectCmd(OpMica, rc, tint)
	c.R = int32(radius)
	r.add(c, rc)
	return r.m.HasWallpaper()
}

func (r *Recorder) DrawNoise(rc image.Rectangle, amount float64) {
	c := rectCmd(OpNoise, rc, color.RGBA{})
	c.F[0] = amount
	r.add(c, rc)
}

// blitBounds — логический охват блита: пиксель src.Rect.Min ложится в (x, y),
// область sr — со сдвигом sr.Min−src.Rect.Min физических пикселей
// (PixelBlitter); на дробном масштабе границы округляются наружу.
func (r *Recorder) blitBounds(src *image.RGBA, sr image.Rectangle, x, y int) image.Rectangle {
	k := r.m.Scale()
	dx, dy := float64(sr.Min.X-src.Rect.Min.X), float64(sr.Min.Y-src.Rect.Min.Y)
	return image.Rect(
		x+int(math.Floor(dx/k)), y+int(math.Floor(dy/k)),
		x+int(math.Ceil((dx+float64(sr.Dx()))/k))+1, y+int(math.Ceil((dy+float64(sr.Dy()))/k))+1)
}

func (r *Recorder) BlitPixels(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool) {
	if src == nil {
		return
	}
	if sr = sr.Intersect(src.Rect); sr.Empty() {
		return
	}
	c := Cmd{Op: OpBlitPixels, X: int32(x), Y: int32(y), Flag: opaque, Ext: &CmdExt{RGBA: src, Rect: sr}}
	r.add(c, r.blitBounds(src, sr, x, y))
}

func (r *Recorder) BlitPixelsBGRA(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool) {
	if src == nil {
		return
	}
	if sr = sr.Intersect(src.Rect); sr.Empty() {
		return
	}
	c := Cmd{Op: OpBlitBGRA, X: int32(x), Y: int32(y), Flag: opaque, Ext: &CmdExt{RGBA: src, Rect: sr}}
	r.add(c, r.blitBounds(src, sr, x, y))
}

// DrawWithOpacity записывает вложенный список: команды внутри draw и ссылки
// на детей попадают в него, группа воспроизводится в колбэке холста.
func (r *Recorder) DrawWithOpacity(rc image.Rectangle, alpha float64, draw func()) {
	if draw == nil || alpha <= 0 {
		return // как холст: нечего рисовать
	}
	if alpha >= 1 || r.target() == nil {
		draw() // как холст: без группы
		return
	}
	sub := &List{gen: r.gen.Load(), rec: r}
	r.groups = append(r.groups, sub)
	draw()
	r.groups = r.groups[:len(r.groups)-1]
	if len(sub.Cmds) == 0 {
		return // холст тоже ничего не нарисовал бы
	}
	c := rectCmd(OpOpacity, rc, color.RGBA{})
	c.F[0] = alpha
	c.Ext = &CmdExt{Sub: sub}
	r.add(c, rc.Union(sub.BBox))
}

func (r *Recorder) HasGlyph(ru rune) bool { return r.m.HasGlyph(ru) }

func (r *Recorder) InvalidateImageCache(src image.Image) { r.m.InvalidateImageCache(src) }

// DrawDamage — у рекордера повреждения нет: пропуск поддеревьев заменяет
// удержание.
func (r *Recorder) DrawDamage() []image.Rectangle { return nil }

func (r *Recorder) SubtreeCullingEnabled() bool { return false }

func (r *Recorder) OcclusionEnabled() bool { return r.m.OcclusionEnabled() }
