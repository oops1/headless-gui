package core

import (
	"image"
	"image/draw"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	xdraw "golang.org/x/image/draw"
)

// dnd_drag.go — перетаскивание файлов, общая часть для всех бэкендов (v3.34):
// события приёмника (вошёл / над окном / вышел) и окно как ИСТОЧНИК
// перетаскивания (BeginDragFiles).
//
// Сам сброс файлов в окно (SetOnFilesDropped) остался прежним: события
// приёмника — дополнение к нему, а не замена. Бэкенд, не умеющий
// перетаскивания (macOS), просто не реализует интерфейсы ниже, и вызовы
// становятся no-op.

// DragEffect — что можно сделать с перетаскиваемыми файлами. Битовая маска:
// источник объявляет допустимые действия, приёмник выбирает одно из них.
// Значения совпадают с DROPEFFECT_* Windows.
type DragEffect uint32

const (
	// DragEffectCopy — скопировать файлы.
	DragEffectCopy DragEffect = 1
	// DragEffectMove — переместить файлы.
	DragEffectMove DragEffect = 2
	// DragEffectLink — создать ссылку на файлы.
	DragEffectLink DragEffect = 4

	// dragEffectAll — все известные биты.
	dragEffectAll = DragEffectCopy | DragEffectMove | DragEffectLink
)

// String — читаемое имя маски для журналов и тестов: «copy|move», «none».
func (e DragEffect) String() string {
	if e&dragEffectAll == 0 {
		return "none"
	}
	s := ""
	add := func(bit DragEffect, name string) {
		if e&bit != 0 {
			if s != "" {
				s += "|"
			}
			s += name
		}
	}
	add(DragEffectCopy, "copy")
	add(DragEffectMove, "move")
	add(DragEffectLink, "link")
	return s
}

// dragTargetEvents — необязательная возможность нативного бэкенда сообщать о
// перетаскивании НАД окном (приёмник): вход, движение и выход. Реализуется
// Win32 (IDropTarget), X11 (XdndEnter/Position/Leave) и Wayland
// (wl_data_device enter/motion/leave). Координаты — ФИЗИЧЕСКИЕ пиксели
// клиентской области, как у SetOnFilesDropped.
//
// Событие enter всегда сопровождается следующим за ним over с теми же
// координатами: на X11 у XdndEnter точки нет, и бэкенд откладывает enter до
// первого XdndPosition.
type dragTargetEvents interface {
	// SetOnDragEnter: курсор с файлами вошёл в окно.
	SetOnDragEnter(fn func(x, y int))
	// SetOnDragOver: курсор с файлами движется над окном. Возврат — принимается
	// ли сброс в этой точке; false бэкенд превращает в «сброс запрещён»
	// (курсор «нельзя», у источника — отказ).
	SetOnDragOver(fn func(x, y int) bool)
	// SetOnDragLeave: курсор ушёл из окна без сброса (или сброс уже состоялся —
	// тогда leave не приходит).
	SetOnDragLeave(fn func())
}

// fileDragSource — необязательная возможность бэкенда начать перетаскивание
// файлов ИЗ окна. paths — абсолютные пути, effect — допустимые действия.
//
// Возврат true — перетаскивание начато (само оно идёт асинхронно или в
// модальном цикле ОС); done вызывается ровно один раз, когда оно кончилось:
// effect — выбранное приёмником действие, 0 — отменено или отклонено.
// false — начать нельзя (нет нажатой кнопки мыши, нет поддержки у сервера);
// done при этом не вызывается.
type fileDragSource interface {
	BeginDragFiles(paths []string, effect DragEffect, done func(effect DragEffect)) bool
}

// fileDragImageSource — необязательная возможность бэкенда показать значок
// перетаскивания (fileDragSource + картинка). img — премультиплицированный
// RGBA с началом в (0,0) и размером не больше dragImageMaxSide, hotspot —
// точка картинки под курсором. Остальное — как у BeginDragFiles.
type fileDragImageSource interface {
	BeginDragFilesImage(paths []string, effect DragEffect, img *image.RGBA, hotspot image.Point, done func(effect DragEffect)) bool
}

// dragImageMaxSide — наибольшая сторона значка перетаскивания в пикселях.
// Больше нельзя: оболочка Windows обрезает картинку, а компоновщик рисует
// значок поверх всего экрана. Большую картинку окно уменьшает само.
const dragImageMaxSide = 512

// dragAnswerTimeout — сколько бэкенд ждёт ответа приложения на «принимается ли
// сброс здесь»: ответ считается на горутине движка, а источнику (X11, Win32)
// отвечать надо сразу. Не успели — отвечаем последним известным ответом.
const dragAnswerTimeout = 25 * time.Millisecond

// dragState — подписки приложения на перетаскивание и последний ответ.
type dragState struct {
	mu       sync.Mutex
	enter    func(x, y int)
	over     func(x, y int) bool
	leave    func()
	finished func(effect DragEffect)

	// last — последний ответ приложения на over. Сбрасывается в true на входе:
	// пока приложение не ответило, считаем, что принимаем.
	last atomic.Bool
}

// SetOnDragEnter регистрирует колбэк входа перетаскиваемых файлов в окно.
// x, y — ЛОГИЧЕСКИЕ координаты точки входа в клиентской области. Вызывается на
// горутине движка. За enter сразу следует over в той же точке.
//
// Файлы приёмнику на этом этапе не видны: ОС отдаёт список только при сбросе
// (SetOnFilesDropped). Поддерживается на Windows (IDropTarget), Linux/X11
// (XDND) и Linux/Wayland (wl_data_device); на прочих платформах — no-op.
// Вызывать можно в любой момент, но до Run() надёжнее.
func (win *Window) SetOnDragEnter(fn func(x, y int)) {
	win.drag.mu.Lock()
	win.drag.enter = fn
	win.drag.mu.Unlock()
}

// SetOnDragOver регистрирует колбэк движения перетаскиваемых файлов над окном.
// x, y — ЛОГИЧЕСКИЕ координаты. Возврат — принимается ли сброс в этой точке;
// false превращается в запрет на стороне ОС (курсор «нельзя», источник
// получает отказ, SetOnFilesDropped не вызывается). Без колбэка сброс
// принимается везде.
//
// Колбэк выполняется на горутине движка, но бэкенд ждёт ответа недолго (25 мс):
// медленный колбэк работает с запозданием на одно событие. Не делайте в нём
// тяжёлой работы — только подсветите зону и верните ответ.
func (win *Window) SetOnDragOver(fn func(x, y int) bool) {
	win.drag.mu.Lock()
	win.drag.over = fn
	win.drag.mu.Unlock()
}

// SetOnDragLeave регистрирует колбэк ухода перетаскиваемых файлов из окна без
// сброса (в том числе отмены перетаскивания клавишей Esc). После сброса файлов
// leave не приходит — снимайте подсветку и в SetOnFilesDropped.
func (win *Window) SetOnDragLeave(fn func()) {
	win.drag.mu.Lock()
	win.drag.leave = fn
	win.drag.mu.Unlock()
}

// SetOnDragFinished регистрирует колбэк окончания перетаскивания, начатого
// BeginDragFiles. effect — действие, выбранное приёмником (DragEffectCopy,
// DragEffectMove или DragEffectLink); 0 — перетаскивание отменено или
// отклонено. Для Move удалять исходные файлы должно приложение (когда
// effect == DragEffectMove): ОС их не трогает. Вызывается на горутине движка.
func (win *Window) SetOnDragFinished(fn func(effect DragEffect)) {
	win.drag.mu.Lock()
	win.drag.finished = fn
	win.drag.mu.Unlock()
}

// BeginDragFiles начинает перетаскивание файлов из окна в другие программы
// (или в это же окно): paths — файлы, effect — допустимые действия (маска
// DragEffectCopy|DragEffectMove|DragEffectLink). Вызывать из обработчика
// мыши, пока кнопка ЗАЖАТА — обычно из движения мыши после нажатия, когда
// курсор ушёл от точки нажатия на несколько пикселей.
//
// Возвращает true, если перетаскивание начато: дальше его ведёт ОС, а о
// конце сообщит SetOnDragFinished. false — начать нельзя: бэкенд не умеет
// (macOS), пустой список или маска, нет зажатой кнопки / serial'а нажатия,
// сервер не даёт wl_data_device. Относительные пути приводятся к абсолютным.
//
// Win32: DoDragDrop (модальный цикл ОС на потоке окна, данные — CF_HDROP);
// X11: XDND-источник (захват указателя, XdndEnter/Position/Drop);
// Wayland: wl_data_device.start_drag с источником text/uri-list.
func (win *Window) BeginDragFiles(paths []string, effect DragEffect) bool {
	return win.beginDragFiles(paths, effect, nil, image.Point{})
}

// BeginDragFilesImage — BeginDragFiles со значком: img показывается рядом с
// курсором, пока идёт перетаскивание, hotspot — точка img под курсором
// (относительно её левого верхнего угла; за границы картинки не выходит).
// Размер img — в пикселях экрана (масштаб окна не применяется); картинка с
// стороной больше 512 пикселей уменьшается, прозрачность сохраняется. nil —
// без значка (как BeginDragFiles).
//
// Win32: IDragSourceHelper.InitializeFromBitmap, значок рисует оболочка —
// и над чужими окнами, и над этим же (приёмник окна подключён к
// IDropTargetHelper). Wayland: поверхность-значок в wl_data_device.start_drag
// (ARGB8888 из wl_shm). X11: XDND значка не определяет, а рисовать
// окно-призрак под курсором без композитора нечем — перетаскивание
// начинается без значка (DragImageSupported() == false), курсор остаётся
// курсором окна.
//
// Возврат и SetOnDragFinished — как у BeginDragFiles.
func (win *Window) BeginDragFilesImage(paths []string, effect DragEffect, img image.Image, hotspot image.Point) bool {
	return win.beginDragFiles(paths, effect, img, hotspot)
}

// DragImageSupported сообщает, нарисует ли бэкенд окна значок, заданный в
// BeginDragFilesImage. false — перетаскивание начнётся, но без значка (X11,
// macOS); приложение может показать свою подсказку. До Run — false.
func (win *Window) DragImageSupported() bool {
	if ds, ok := win.native.(interface{ dragImageSupported() bool }); ok {
		return ds.dragImageSupported()
	}
	_, ok := win.native.(fileDragImageSource)
	return ok
}

func (win *Window) beginDragFiles(paths []string, effect DragEffect, img image.Image, hotspot image.Point) bool {
	src, ok := win.native.(fileDragSource)
	if !ok {
		return false
	}
	paths = normalizeDragPaths(paths)
	effect &= dragEffectAll
	if len(paths) == 0 || effect == 0 {
		return false
	}
	done := func(res DragEffect) {
		win.post(func() {
			win.drag.mu.Lock()
			fn := win.drag.finished
			win.drag.mu.Unlock()
			if fn != nil {
				fn(res & dragEffectAll)
			}
		})
	}
	if is, ok := src.(fileDragImageSource); ok && img != nil {
		if rgba, hot, ok := prepareDragImage(img, hotspot); ok {
			return is.BeginDragFilesImage(paths, effect, rgba, hot, done)
		}
	}
	return src.BeginDragFiles(paths, effect, done)
}

// prepareDragImage приводит значок к виду, который ждут бэкенды: RGBA
// (премультиплицированный) с началом в (0,0), сторона не больше
// dragImageMaxSide (большая уменьшается с сохранением пропорций, точка
// захвата пересчитывается), hotspot внутри картинки. ok=false — картинки нет
// (пустая).
func prepareDragImage(img image.Image, hotspot image.Point) (*image.RGBA, image.Point, bool) {
	if img == nil {
		return nil, image.Point{}, false
	}
	b := img.Bounds()
	if b.Empty() {
		return nil, image.Point{}, false
	}
	w, h := b.Dx(), b.Dy()
	tw, th := w, h
	if m := max(w, h); m > dragImageMaxSide {
		tw = max(w*dragImageMaxSide/m, 1)
		th = max(h*dragImageMaxSide/m, 1)
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	if tw == w && th == h {
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
		hotspot = image.Pt(hotspot.X*tw/w, hotspot.Y*th/h)
	}
	hotspot.X = min(max(hotspot.X, 0), tw-1)
	hotspot.Y = min(max(hotspot.Y, 0), th-1)
	return dst, hotspot, true
}

// normalizeDragPaths отбрасывает пустые пути и приводит остальные к
// абсолютным: источник отдаёт приёмнику file://-URI / HDROP, а относительный
// путь там бессмыслен.
func normalizeDragPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		out = append(out, p)
	}
	return out
}

// pickDragEffect выбирает действие из допустимых источником (allowed) с учётом
// пожеланий приёмника (prefer, может быть 0). Предпочтение выигрывает, если оно
// допустимо; иначе по убыванию «безопасности»: copy, move, link. 0 — выбрать
// нечего.
func pickDragEffect(allowed, prefer DragEffect) DragEffect {
	allowed &= dragEffectAll
	if p := prefer & allowed; p != 0 {
		for _, e := range []DragEffect{DragEffectCopy, DragEffectMove, DragEffectLink} {
			if p&e != 0 {
				return e
			}
		}
	}
	for _, e := range []DragEffect{DragEffectCopy, DragEffectMove, DragEffectLink} {
		if allowed&e != 0 {
			return e
		}
	}
	return 0
}

// dragEffectForKeys — действие по зажатым модификаторам, как в Проводнике
// Windows: Ctrl — копировать, Shift — переместить, Ctrl+Shift (или Alt) —
// ссылка; без клавиш — копировать, а если копирование недопустимо, то
// первое допустимое. Чистая функция: проверяется без ОС.
func dragEffectForKeys(allowed DragEffect, ctrl, shift, alt bool) DragEffect {
	var prefer DragEffect
	switch {
	case ctrl && shift, alt:
		prefer = DragEffectLink
	case ctrl:
		prefer = DragEffectCopy
	case shift:
		prefer = DragEffectMove
	}
	return pickDragEffect(allowed, prefer)
}

// dragLogical переводит физические координаты клиентской области (так их
// отдают бэкенды) в логические координаты контента для колбэка приложения.
func (win *Window) dragLogical(x, y int) (int, int) {
	x, y = win.toContent(x, y)
	if win.scale != 1 && win.scale > 0 {
		x = int(float64(x) / win.scale)
		y = int(float64(y) / win.scale)
	}
	return x, y
}

// setupDragEvents подключает события приёмника бэкенда к колбэкам приложения.
// Бэкенд вызывает свои колбэки со своего потока: события ставятся в очередь
// движка (win.post), как и у остального ввода. Единственное исключение — over:
// ответ нужен ОС сразу, поэтому бэкенд коротко ждёт его (dragAnswerTimeout).
func (win *Window) setupDragEvents() {
	de, ok := win.native.(dragTargetEvents)
	if !ok {
		return
	}
	de.SetOnDragEnter(func(x, y int) {
		win.drag.last.Store(true)
		win.drag.mu.Lock()
		fn := win.drag.enter
		win.drag.mu.Unlock()
		if fn == nil {
			return
		}
		win.post(func() {
			lx, ly := win.dragLogical(x, y)
			fn(lx, ly)
		})
	})
	de.SetOnDragOver(func(x, y int) bool {
		win.drag.mu.Lock()
		fn := win.drag.over
		win.drag.mu.Unlock()
		if fn == nil {
			return true
		}
		res := make(chan bool, 1)
		win.post(func() {
			lx, ly := win.dragLogical(x, y)
			ok := fn(lx, ly)
			win.drag.last.Store(ok)
			res <- ok
		})
		select {
		case ok := <-res:
			return ok
		case <-time.After(dragAnswerTimeout):
			return win.drag.last.Load()
		}
	})
	de.SetOnDragLeave(func() {
		win.drag.mu.Lock()
		fn := win.drag.leave
		win.drag.mu.Unlock()
		if fn == nil {
			return
		}
		win.post(fn)
	})
}
