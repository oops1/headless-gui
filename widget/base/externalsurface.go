package base

import (
	"image"
	"image/color"
	"math"
	"sync"
	"sync/atomic"
)

// externalsurface.go — вывод чужого буфера без копии (v3.39, E7).
//
// ImageWidget и DrawImage берут изображение, а движок кладёт его пиксели в
// тайлы и сравнивает с прошлым кадром; для клиента удалённого рабочего стола
// это лишняя копия: сессия 4K — 33 МБ, меняется кусками по несколько
// процентов. ExternalSurface не владеет буфером и не держит у себя его копию:
// приложение отдаёт свой *image.RGBA, говорит, какие его части изменились, а
// виджет при отрисовке копирует в холст только то, что холст сейчас
// перерисовывает (клип кадра), прямо из буфера приложения.
//
// ПИКСЕЛИ. Буфер — физические пиксели: один пиксель буфера равен одному
// физическому пикселю холста, масштаб не применяется ни при каком HiDPI (на
// масштабе 2 буфер 200×100 занимает логические 100×50). Левый верхний пиксель
// буфера лежит в левом верхнем углу границ виджета; что не поместилось в
// границы — отсекается, что не дотянулось — закрашивается Background.
//
// ДАМАГ. InvalidateRects переводит области буфера в области кадра и сообщает
// их движку точечно (InvalidateRect): движок сравнит с прошлым кадром и
// отдаст потребителю тайлы только этих областей, а не всего виджета.
// Разметку ведёт приложение: изменили пиксели — заявите область. Незаявленное
// движок не увидит (контракт InvalidateRect).
//
// ЧТО КОПИРУЕТ Draw. Не «области, заявленные приложением», а то, что стёрто в
// этом кадре: клип кадра ∩ границы виджета. Это не оплошность. Движок стирает
// и перерисовывает объединение ВСЕХ областей кадра (в том числе чужих: меню
// закрылось над поверхностью, подсказка исчезла), и пиксели поверхности под
// ним надо вернуть, хотя приложение их и не меняло. Если заявляет только
// приложение (типичный случай — кадр сессии), клип равен объединению его
// областей, то есть копируется ровно то, что заявлено. Объединение — один
// охватывающий прямоугольник: далеко разнесённые области лучше склеивать на
// стороне приложения, всё равно копируется охват.
//
// СИНХРОНИЗАЦИЯ. Буфер читается ТОЛЬКО в Draw, на горутине кадра, и только
// под замком виджета (Lock/Unlock). Приложение пишет в буфер под тем же
// замком, а заявляет области уже после записи и вне замка:
//
//	surf.Update(func(buf *image.RGBA) []image.Rectangle {
//		copy(buf.Pix[...], decoded)    // запись под замком
//		return []image.Rectangle{r}   // области — после снятия замка
//	})
//
// Update делает ровно это. Замок держится на время копирования Draw (для
// области 10% от 4K — доли миллисекунды, для полного кадра — единицы
// миллисекунд), поэтому писать под замком стоит коротко: сложить кусок,
// заявить, отпустить. Внутри замка нельзя вызывать SetBuffer, Swap, Update
// и всё, что берёт замок кадра движка (снимки, ожидание кадра): замок не
// реентерабельный.
//
// Двойная буферизация — Swap: подменяет буфер целиком и отдаёт прежний;
// после возврата движок его больше не читает (Swap ждёт конца идущего Draw),
// так что приложение вправе сразу писать в него для следующего кадра.
type ExternalSurface struct {
	Base

	// mu — замок содержимого: держат приложение (Lock/Update/Swap) и Draw на
	// время копирования. Охраняет buf.
	mu  sync.Mutex
	buf *image.RGBA

	// meta — снимок описания буфера для тех, кому замок содержимого не
	// нужен или вреден (OpaqueRegion на обходе, InvalidateRects на горутине
	// приложения): границы, признак непрозрачности, фон. Заменяется целиком.
	meta atomic.Pointer[surfaceMeta]
	// metaMu сериализует замену meta (чтение-изменение-запись); читают без него.
	metaMu sync.Mutex
	// opBuf — буфер ответа OpaqueRegion (зовётся на каждый кадр обходом).
	opBuf [1]image.Rectangle

	// scaleBits — HiDPI-масштаб, виденный последним Draw (math.Float64bits);
	// нужен переводу областей буфера в логические координаты кадра.
	scaleBits atomic.Uint64

	// input — колбэки ввода окна программы (externalsurface_input.go); nil —
	// ввод не принимается. focused — у поверхности фокус клавиатуры.
	input   atomic.Pointer[SurfaceInput]
	focused atomic.Bool

	// content — непрозрачная область, вид содержимого и частота обновлений
	// (externalsurface_content.go).
	content surfaceContent
}

// surfaceMeta — неизменяемое описание буфера поверхности.
type surfaceMeta struct {
	rect   image.Rectangle // границы буфера (пустой — буфера нет)
	opaque bool            // все пиксели буфера непрозрачны
	bg     color.RGBA      // чем закрашивать места без буфера
	bgra   bool            // байты буфера в порядке B,G,R,A (SetBGRA)
}

// NewExternalSurface создаёт поверхность без буфера: пока его нет, виджет
// закрашивается фоном (чёрным).
func NewExternalSurface() *ExternalSurface {
	s := &ExternalSurface{}
	s.meta.Store(&surfaceMeta{bg: color.RGBA{A: 255}})
	s.scaleBits.Store(math.Float64bits(1))
	return s
}

func (s *ExternalSurface) loadMeta() *surfaceMeta {
	if m := s.meta.Load(); m != nil {
		return m
	}
	return &surfaceMeta{bg: color.RGBA{A: 255}}
}

// SetBackground задаёт цвет мест, не закрытых буфером (буфер меньше виджета
// или отсутствует). Цвет должен быть непрозрачным: прозрачный фон просвечивает
// то, что нарисовано под поверхностью в прошлых кадрах.
func (s *ExternalSurface) SetBackground(c color.RGBA) {
	s.metaMu.Lock()
	old := s.loadMeta()
	if old.bg == c {
		s.metaMu.Unlock()
		return
	}
	n := *old
	n.bg = c
	s.meta.Store(&n)
	s.metaMu.Unlock()
	s.Invalidate()
}

// Background возвращает цвет заливки мест без буфера.
func (s *ExternalSurface) Background() color.RGBA { return s.loadMeta().bg }

// SetBGRA говорит, что байты буфера лежат в порядке B,G,R,A: так их пишут
// wl_shm (ARGB8888 и XRGB8888) и DIB Windows. На холсте FormatBGRX строки
// такого буфера копируются как есть, без перестановки каналов на каждом
// пикселе; на холсте RGBA каналы переставляются при копировании. Без вызова
// буфер — RGBA, как у image.RGBA. Premultiplied и признак opaque — как
// прежде.
func (s *ExternalSurface) SetBGRA(on bool) {
	s.metaMu.Lock()
	old := s.loadMeta()
	if old.bgra == on {
		s.metaMu.Unlock()
		return
	}
	n := *old
	n.bgra = on
	s.meta.Store(&n)
	s.metaMu.Unlock()
	s.Invalidate()
}

// SetBuffer отдаёт поверхности буфер приложения (nil снимает его). opaque —
// обещание: ВСЕ пиксели имеют альфу 255; тогда область перезаписывается
// копированием строк (композиция Src), иначе смешивается с тем, что под
// поверхностью (Over, заметно медленнее). Нарушенное обещание затирает
// прозрачными местами фон.
//
// Буфер не копируется: виджет хранит указатель. Приложение остаётся его
// владельцем и пишет в него под замком (Lock/Update). Вызов инвалидирует весь
// виджет, а берёт замок — дождитесь конца идущей отрисовки, не вызывайте его
// из-под Lock. См. Swap, если нужен прежний буфер.
func (s *ExternalSurface) SetBuffer(img *image.RGBA, opaque bool) {
	s.Swap(img, opaque, nil)
}

// Swap подменяет буфер и возвращает прежний (двойная буферизация).
//
// Вызов ждёт конца идущей отрисовки: после возврата движок прежний буфер не
// читает и не прочтёт, приложение вправе писать в него без замка. Новый буфер
// обязан содержать ВЕСЬ кадр. damage — что изменилось по сравнению с тем, что
// сейчас на экране; nil (или смена размера буфера и признака opaque) означает
// «всё», и перерисовывается весь виджет. Области — в координатах буфера (см.
// InvalidateRects).
func (s *ExternalSurface) Swap(next *image.RGBA, opaque bool, damage []image.Rectangle) (prev *image.RGBA) {
	m := &surfaceMeta{opaque: opaque && next != nil}
	if next != nil {
		m.rect = next.Rect
	}
	s.mu.Lock()
	prev = s.buf
	s.buf = next
	s.metaMu.Lock()
	old := s.loadMeta()
	m.bg = old.bg
	m.bgra = old.bgra
	s.meta.Store(m)
	s.metaMu.Unlock()
	s.mu.Unlock()

	// Области годятся только при том же устройстве буфера: другой размер или
	// порядок смешивания меняет всю картинку.
	same := damage != nil && old.rect == m.rect && old.opaque == m.opaque && !m.rect.Empty()
	if !same {
		s.noteUpdate()
		s.Invalidate()
		return prev
	}
	s.InvalidateRects(damage)
	return prev
}

// Buffer возвращает текущий буфер (его же, не копию). Читать и писать его
// можно только под замком.
func (s *ExternalSurface) Buffer() *image.RGBA {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf
}

// Lock берёт замок содержимого буфера: пока он взят, Draw не читает буфер.
// Приложение держит его на время записи. Вместе с Unlock делает
// поверхность sync.Locker.
func (s *ExternalSurface) Lock() { s.mu.Lock() }

// Unlock снимает замок содержимого буфера.
func (s *ExternalSurface) Unlock() { s.mu.Unlock() }

// Update вызывает fn с текущим буфером под замком и после его снятия
// заявляет возвращённые области (в координатах буфера). Без буфера fn не
// вызывается. Не вызывайте из fn методы поверхности, берущие замок.
func (s *ExternalSurface) Update(fn func(buf *image.RGBA) (damage []image.Rectangle)) {
	s.mu.Lock()
	buf := s.buf
	if buf == nil {
		s.mu.Unlock()
		return
	}
	damage := fn(buf)
	s.mu.Unlock()
	s.InvalidateRects(damage)
}

// InvalidateRect заявляет одну изменившуюся область буфера (см.
// InvalidateRects).
func (s *ExternalSurface) InvalidateRect(r image.Rectangle) {
	s.InvalidateRects([]image.Rectangle{r})
}

// InvalidateRects заявляет движку области буфера, изменившиеся после
// последнего кадра. Координаты — буфера (относительно его Rect); части за
// буфером и за границами виджета отбрасываются, пустые игнорируются. Движок
// сравнит с прошлым кадром и вернёт потребителю тайлы только этих областей.
//
// Зовите ПОСЛЕ записи в буфер, вне замка (иначе кадр, начатый между
// вызовом и записью, не увидит записанного, а следующий ничего не изменит).
// Безопасно из любой горутины; срез не запоминается. Масштаб берётся тот,
// что виден последней отрисовкой (до первой — 1); при смене масштаба окна
// движок перерисовывает кадр целиком.
func (s *ExternalSurface) InvalidateRects(rects []image.Rectangle) {
	if len(rects) > 0 {
		s.noteUpdate()
	}
	m := s.loadMeta()
	if m.rect.Empty() || len(rects) == 0 {
		return
	}
	b := s.Bounds()
	if b.Empty() {
		return
	}
	k := math.Float64frombits(s.scaleBits.Load())
	if k <= 0 {
		k = 1
	}
	// Физическая позиция левого верхнего угла виджета — та же формула, что у
	// холста (округление по краям), так что область ложится на свои пиксели.
	ox, oy := physEdge(b.Min.X, k), physEdge(b.Min.Y, k)
	for _, r := range rects {
		r = r.Intersect(m.rect)
		if r.Empty() {
			continue
		}
		r = r.Sub(m.rect.Min).Add(image.Pt(ox, oy))
		l := image.Rect(
			logFloor(r.Min.X, k), logFloor(r.Min.Y, k),
			logCeil(r.Max.X, k), logCeil(r.Max.Y, k),
		).Intersect(b)
		if !l.Empty() {
			s.invalidateRect(l)
		}
	}
}

// physEdge — физическая граница логической координаты (округление, как у
// холста).
func physEdge(v int, k float64) int {
	if k == 1 {
		return v
	}
	return int(math.Round(float64(v) * k))
}

// logFloor/logCeil — логическая координата, не правее/не левее физической.
func logFloor(p int, k float64) int {
	if k == 1 {
		return p
	}
	return int(math.Floor(float64(p) / k))
}

func logCeil(p int, k float64) int {
	if k == 1 {
		return p
	}
	return int(math.Ceil(float64(p) / k))
}

// OpaqueRegion объявляет границы виджета закрытыми, если буфер непрозрачен и
// фон непрозрачен: тогда обход не рисует то, что целиком лежит под
// поверхностью.
func (s *ExternalSurface) OpaqueRegion() []image.Rectangle {
	// Клиент объявил, что непрозрачно (SetOpaqueRegion): только это, даже у
	// буфера с альфой — пиксели с A = 255 закрывают и при смешивании.
	if rects, ok := s.opaqueRegionLocal(); ok {
		return rects
	}
	m := s.loadMeta()
	if !m.opaque || m.bg.A != 255 {
		return nil
	}
	return oneRegion(&s.opBuf, s.Bounds())
}

// Draw копирует в холст то, что холст перерисовывает в этом кадре (клип ∩
// границы), прямо из буфера приложения: без промежуточной копии и без
// масштабирования. Места, не закрытые буфером, закрашиваются фоном.
func (s *ExternalSurface) Draw(ctx DrawContext) {
	b := s.Bounds()
	k := ContextScale(ctx)
	s.scaleBits.Store(math.Float64bits(k))
	if !b.Empty() {
		s.drawBuffer(ctx, b, k)
	}
	s.drawChildren(ctx)
}

// DrawDirect реализует DirectDrawer: буфер блитится под замком из текущего
// содержимого, и по контракту Swap/Lock движок не читает его после возврата
// Draw — записать блит командой и воспроизвести позже нельзя. Поверхность
// рисуется прямо в холст при воспроизведении списка родителя.
func (s *ExternalSurface) DrawDirect() bool { return true }

func (s *ExternalSurface) drawBuffer(ctx DrawContext, b image.Rectangle, k float64) {
	// Часть кадра, которую вообще стоит трогать: за ней холст всё равно отсечёт.
	if ctx.Clip().Intersect(b).Empty() {
		return
	}
	phys := PhysicalRect(ctx, b)
	pw, ph := phys.X, phys.Y

	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.loadMeta()
	buf := s.buf

	// Часть буфера, что попадает в границы виджета.
	var sr image.Rectangle
	if buf != nil {
		sr = buf.Rect
		sr.Max.X = min(sr.Max.X, sr.Min.X+pw)
		sr.Max.Y = min(sr.Max.Y, sr.Min.Y+ph)
	}
	sw, sh := sr.Dx(), sr.Dy()
	if sr.Empty() {
		sw, sh = 0, 0
	}

	// Места без буфера — фоном, ДО блита: края (на дробном масштабе
	// логическая граница полосы заходит на пиксель буфера) потом перекроет
	// сам буфер.
	switch {
	case sw == 0 || sh == 0:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), m.bg)
	default:
		ox, oy := physEdge(b.Min.X, k), physEdge(b.Min.Y, k)
		if sw < pw {
			x := max(b.Min.X, logFloor(ox+sw, k))
			ctx.FillRect(x, b.Min.Y, b.Max.X-x, b.Dy(), m.bg)
		}
		if sh < ph {
			y := max(b.Min.Y, logFloor(oy+sh, k))
			ctx.FillRect(b.Min.X, y, b.Dx(), b.Max.Y-y, m.bg)
		}
	}
	if sw > 0 && sh > 0 {
		if m.bgra {
			BlitPixelsBGRA(ctx, buf, sr, b.Min.X, b.Min.Y, m.opaque)
		} else {
			BlitPixels(ctx, buf, sr, b.Min.X, b.Min.Y, m.opaque)
		}
	}
}
