//go:build darwin

package window

/*
macOS backend через purego — вызов Cocoa/AppKit API без CGO.

Используем purego objc runtime:
  objc.GetClass, objc.RegisterName, objc.ID.Send

Минимальный набор классов:
  NSApplication, NSWindow, NSView, NSEvent, CALayer, CATransaction

Фреймворки AppKit/QuartzCore/CoreGraphics загружаются через dlopen —
чистый Go-бинарь их не линкует, без явной загрузки objc.GetClass
возвращает 0 и бэкенд не работает.

Вывод кадра: CGImage → CALayer.contents (композитор). Старый путь
NSBitmapImageRep + NSImage + lockFocus (deprecated с macOS 10.14)
доступен через переменную окружения HEADLESS_GUI_COCOA_LEGACY=1.
*/

import (
	"fmt"
	"image"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// ─── Cocoa типы ─────────────────────────────────────────────────────────────

type nsPoint struct{ X, Y float64 }
type nsSize struct{ Width, Height float64 }
type nsRect struct {
	Origin nsPoint
	Size   nsSize
}

// ─── CocoaWindow ────────────────────────────────────────────────────────────

type CocoaWindow struct {
	nsApp    objc.ID
	nsWindow objc.ID
	nsView   objc.ID
	width    int
	height   int
	title    string

	maximized bool
	closed    bool

	// Callbacks
	onResize      func(w, h int)
	onClose       func() bool
	onMouseMove   func(x, y int)
	onMouseButton func(x, y, button int, pressed bool)
	onKeyDown     func(vk int)
	onKeyUp       func(vk int)
	onChar        func(r rune)

	// CALayer-путь: слой contentView. Пиксели кадра НЕ передаются Core
	// Animation указателем на Go-память: они копируются в CFData (владелец —
	// CoreFoundation), см. BlitRGBA. Иначе после ресайза/GC слой мог читать
	// освобождённый Go-буфер (SEC-6, use-after-free).
	nsLayer    objc.ID
	legacyBlit bool // HEADLESS_GUI_COCOA_LEGACY=1 — старый путь через lockFocus
}

// Cocoa selectors (кэшируем)
var (
	selAlloc                     objc.SEL
	selRelease                   objc.SEL
	selBitmapData                objc.SEL
	selInit                      objc.SEL
	selSharedApplication         objc.SEL
	selSetActivationPolicy       objc.SEL
	selActivateIgnoringOtherApps objc.SEL
	selRun                       objc.SEL
	selStop                      objc.SEL
	selInitWithContentRect       objc.SEL
	selSetTitle                  objc.SEL
	selMakeKeyAndOrderFront      objc.SEL
	selCenter                    objc.SEL
	selContentView               objc.SEL
	selFrame                     objc.SEL
	selMiniaturize               objc.SEL
	selZoom                      objc.SEL
	selIsZoomed                  objc.SEL
	selClose                     objc.SEL
	selSetNeedsDisplay           objc.SEL
	selNextEvent                 objc.SEL
	selSendEvent                 objc.SEL
	selType                      objc.SEL
	selLocationInWindow          objc.SEL
	selButtonNumber              objc.SEL
	selKeyCode                   objc.SEL
	selCharacters                objc.SEL
	selUTF8String                objc.SEL

	// NSString
	selInitWithUTF8String objc.SEL

	// Для BlitRGBA
	selInitWithBitmapDataPlanes objc.SEL
	selInitWithSize             objc.SEL
	selAddRepresentation        objc.SEL
	selLockFocus                objc.SEL
	selUnlockFocus              objc.SEL
	selDrawInRect               objc.SEL
	selFlushGraphics            objc.SEL
	selCurrentContext           objc.SEL

	cocoaInited bool
)

func initCocoaSelectors() {
	if cocoaInited {
		return
	}
	selAlloc = objc.RegisterName("alloc")
	selRelease = objc.RegisterName("release")
	selBitmapData = objc.RegisterName("bitmapData")
	selInit = objc.RegisterName("init")
	selSharedApplication = objc.RegisterName("sharedApplication")
	selSetActivationPolicy = objc.RegisterName("setActivationPolicy:")
	selActivateIgnoringOtherApps = objc.RegisterName("activateIgnoringOtherApps:")
	selRun = objc.RegisterName("run")
	selStop = objc.RegisterName("stop:")
	selInitWithContentRect = objc.RegisterName("initWithContentRect:styleMask:backing:defer:")
	selSetTitle = objc.RegisterName("setTitle:")
	selMakeKeyAndOrderFront = objc.RegisterName("makeKeyAndOrderFront:")
	selCenter = objc.RegisterName("center")
	selContentView = objc.RegisterName("contentView")
	selFrame = objc.RegisterName("frame")
	selMiniaturize = objc.RegisterName("miniaturize:")
	selZoom = objc.RegisterName("zoom:")
	selIsZoomed = objc.RegisterName("isZoomed")
	selClose = objc.RegisterName("close")
	selSetNeedsDisplay = objc.RegisterName("setNeedsDisplay:")
	selNextEvent = objc.RegisterName("nextEventMatchingMask:untilDate:inMode:dequeue:")
	selSendEvent = objc.RegisterName("sendEvent:")
	selType = objc.RegisterName("type")
	selLocationInWindow = objc.RegisterName("locationInWindow")
	selButtonNumber = objc.RegisterName("buttonNumber")
	selKeyCode = objc.RegisterName("keyCode")
	selCharacters = objc.RegisterName("characters")
	selUTF8String = objc.RegisterName("UTF8String")

	selInitWithUTF8String = objc.RegisterName("initWithUTF8String:")

	// BlitRGBA selectors
	selInitWithBitmapDataPlanes = objc.RegisterName("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:")
	selInitWithSize = objc.RegisterName("initWithSize:")
	selAddRepresentation = objc.RegisterName("addRepresentation:")
	selLockFocus = objc.RegisterName("lockFocus")
	selUnlockFocus = objc.RegisterName("unlockFocus")
	selDrawInRect = objc.RegisterName("drawInRect:fromRect:operation:fraction:")
	selFlushGraphics = objc.RegisterName("flushGraphics")
	selCurrentContext = objc.RegisterName("currentContext")

	// CALayer selectors
	selSetWantsLayer = objc.RegisterName("setWantsLayer:")
	selLayer = objc.RegisterName("layer")
	selSetContents = objc.RegisterName("setContents:")
	selBegin = objc.RegisterName("begin")
	selCommit = objc.RegisterName("commit")
	selSetDisableActions = objc.RegisterName("setDisableActions:")

	cocoaInited = true
}

// ─── Загрузка фреймворков и функций CoreGraphics ────────────────────────────

var (
	selSetWantsLayer     objc.SEL
	selLayer             objc.SEL
	selSetContents       objc.SEL
	selBegin             objc.SEL
	selCommit            objc.SEL
	selSetDisableActions objc.SEL

	// CoreGraphics: CGImage из сырого пиксельного буфера.
	cgColorSpaceCreateDeviceRGB    func() uintptr
	cgDataProviderCreateWithCFData func(data uintptr) uintptr
	cgDataProviderRelease          func(provider uintptr)
	// CoreFoundation: CFData делает СОБСТВЕННУЮ копию пикселей — CoreGraphics
	// и Core Animation никогда не держат указателей в Go-кучу (SEC-6).
	cfDataCreate  func(allocator, bytes, length uintptr) uintptr
	cfRelease     func(cf uintptr)
	cgImageCreate func(width, height, bitsPerComponent, bitsPerPixel, bytesPerRow,
		colorSpace uintptr, bitmapInfo uint32, provider, decode uintptr,
		shouldInterpolate uintptr, intent uintptr) uintptr
	cgImageRelease func(img uintptr)

	cgColorSpace uintptr // DeviceRGB, создаётся один раз

	frameworksOnce sync.Once
	frameworksErr  error
)

// kCGImageAlphaPremultipliedLast: порядок байт в памяти R,G,B,A
// (premultiplied; альфа канваса всегда 255, так что эквивалентно straight).
const cgBitmapRGBA8888 uint32 = 1

// loadFrameworks загружает AppKit/QuartzCore/CoreGraphics в процесс и
// биндит функции CoreGraphics. Обязательный шаг: Go-бинарь без CGO не
// линкует фреймворки, и без dlopen классы Cocoa недоступны.
func loadFrameworks() error {
	frameworksOnce.Do(func() {
		for _, lib := range []string{
			"/System/Library/Frameworks/AppKit.framework/AppKit",
			"/System/Library/Frameworks/QuartzCore.framework/QuartzCore",
		} {
			if _, err := purego.Dlopen(lib, purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
				frameworksErr = fmt.Errorf("cocoa: dlopen %s: %w", lib, err)
				return
			}
		}
		cg, err := purego.Dlopen(
			"/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics",
			purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			frameworksErr = fmt.Errorf("cocoa: dlopen CoreGraphics: %w", err)
			return
		}
		purego.RegisterLibFunc(&cgColorSpaceCreateDeviceRGB, cg, "CGColorSpaceCreateDeviceRGB")
		purego.RegisterLibFunc(&cgDataProviderCreateWithCFData, cg, "CGDataProviderCreateWithCFData")
		purego.RegisterLibFunc(&cgDataProviderRelease, cg, "CGDataProviderRelease")
		purego.RegisterLibFunc(&cgImageCreate, cg, "CGImageCreate")
		purego.RegisterLibFunc(&cgImageRelease, cg, "CGImageRelease")
		cf, err := purego.Dlopen(
			"/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation",
			purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			frameworksErr = fmt.Errorf("cocoa: dlopen CoreFoundation: %w", err)
			return
		}
		purego.RegisterLibFunc(&cfDataCreate, cf, "CFDataCreate")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		cgColorSpace = cgColorSpaceCreateDeviceRGB()
	})
	return frameworksErr
}

// nsString создаёт NSString из Go-строки.
func nsString(s string) objc.ID {
	// Используем C-строку через unsafe.Pointer
	cstr := append([]byte(s), 0) // null-terminated
	nsStringClass := objc.ID(objc.GetClass("NSString"))
	alloc := nsStringClass.Send(selAlloc)
	return alloc.Send(selInitWithUTF8String, uintptr(unsafe.Pointer(&cstr[0])))
}

// goString читает Go-строку из указателя на C-строку (null-terminated).
func goString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var buf []byte
	for {
		b := *(*byte)(unsafe.Pointer(ptr))
		if b == 0 {
			break
		}
		buf = append(buf, b)
		ptr++
	}
	return string(buf)
}

func NewNativeWindow() NativeWindow {
	return &CocoaWindow{}
}

func (w *CocoaWindow) Create(title string, width, height int) error {
	runtime.LockOSThread()
	if err := loadFrameworks(); err != nil {
		return err
	}
	initCocoaSelectors()

	w.title = title
	w.width = width
	w.height = height
	w.legacyBlit = os.Getenv("HEADLESS_GUI_COCOA_LEGACY") == "1"

	// NSApplication.sharedApplication
	nsAppClass := objc.GetClass("NSApplication")
	if nsAppClass == 0 {
		return fmt.Errorf("cocoa: NSApplication not found")
	}
	w.nsApp = objc.ID(nsAppClass).Send(selSharedApplication)

	// setActivationPolicy: NSApplicationActivationPolicyRegular = 0
	w.nsApp.Send(selSetActivationPolicy, 0)

	// NSWindow alloc
	nsWindowClass := objc.GetClass("NSWindow")
	nsWinAlloc := objc.ID(nsWindowClass).Send(selAlloc)

	// initWithContentRect:styleMask:backing:defer:
	contentRect := nsRect{
		Origin: nsPoint{X: 100, Y: 100},
		Size:   nsSize{Width: float64(width), Height: float64(height)},
	}

	// NSBorderlessWindowMask = 0 (borderless)
	w.nsWindow = nsWinAlloc.Send(selInitWithContentRect,
		uintptr(unsafe.Pointer(&contentRect)),
		0, // styleMask = borderless
		2, // backing = NSBackingStoreBuffered
		0, // defer = NO
	)

	if w.nsWindow == 0 {
		return fmt.Errorf("cocoa: failed to create NSWindow")
	}

	// Заголовок (для dock/taskbar)
	w.setCocoaTitle(title)

	// Центрируем
	w.nsWindow.Send(selCenter)

	// Показываем
	w.nsWindow.Send(selMakeKeyAndOrderFront, 0)

	// Активируем
	w.nsApp.Send(selActivateIgnoringOtherApps, 1)

	// Получаем contentView
	w.nsView = w.nsWindow.Send(selContentView)

	// Layer-backed view: кадры выводятся через CALayer.contents (композитор),
	// без deprecated lockFocus. Слой запрашиваем после setWantsLayer.
	if !w.legacyBlit {
		w.nsView.Send(selSetWantsLayer, 1)
		w.nsLayer = w.nsView.Send(selLayer)
		if w.nsLayer == 0 {
			w.legacyBlit = true // слой недоступен — откат на старый путь
		}
	}

	return nil
}

func (w *CocoaWindow) RunEventLoop() error {
	// Ручной event loop (не NSApplication.run) для полного контроля
	selDistantFuture := objc.RegisterName("distantFuture")
	nsDateClass := objc.GetClass("NSDate")
	distantFuture := objc.ID(nsDateClass).Send(selDistantFuture)

	// NSDefaultRunLoopMode — создаём как NSString
	defaultMode := nsString("kCFRunLoopDefaultMode")

	for !w.closed {
		// nextEventMatchingMask:untilDate:inMode:dequeue:
		event := w.nsApp.Send(selNextEvent,
			uintptr(0xFFFFFFFF), // NSAnyEventMask
			uintptr(distantFuture),
			uintptr(defaultMode),
			uintptr(1), // dequeue = YES
		)

		if event == 0 {
			continue
		}

		// Определяем тип события
		evType := int(objc.Send[uintptr](event, selType))

		w.handleCocoaEvent(event, evType)

		// Передаём событие дальше
		w.nsApp.Send(selSendEvent, uintptr(event))
	}
	return nil
}

func (w *CocoaWindow) handleCocoaEvent(event objc.ID, evType int) {
	switch evType {
	case 1: // NSLeftMouseDown
		w.handleMouseButton(event, 0, true)
	case 2: // NSLeftMouseUp
		w.handleMouseButton(event, 0, false)
	case 3: // NSRightMouseDown
		w.handleMouseButton(event, 1, true)
	case 4: // NSRightMouseUp
		w.handleMouseButton(event, 1, false)
	case 5, 6: // NSMouseMoved, NSLeftMouseDragged
		w.handleMouseMove(event)
	case 25, 26: // NSOtherMouseDown/Up
		pressed := evType == 25
		w.handleMouseButton(event, 2, pressed)
	case 10: // NSKeyDown
		w.handleKeyDown(event)
	case 11: // NSKeyUp
		w.handleKeyUp(event)
	}
}

func (w *CocoaWindow) handleMouseMove(event objc.ID) {
	if w.onMouseMove == nil {
		return
	}
	// locationInWindow возвращает NSPoint (Cocoa: Y снизу вверх)
	pt := objc.Send[nsPoint](event, selLocationInWindow)
	x := int(pt.X)
	y := w.height - int(pt.Y) // переворачиваем Y
	w.onMouseMove(x, y)
}

func (w *CocoaWindow) handleMouseButton(event objc.ID, button int, pressed bool) {
	if w.onMouseButton == nil {
		return
	}
	pt := objc.Send[nsPoint](event, selLocationInWindow)
	x := int(pt.X)
	y := w.height - int(pt.Y)
	w.onMouseButton(x, y, button, pressed)
}

func (w *CocoaWindow) handleKeyDown(event objc.ID) {
	keyCode := int(objc.Send[uintptr](event, selKeyCode))

	vk := cocoaKeyCodeToVK(keyCode)
	if w.onKeyDown != nil && vk != 0 {
		w.onKeyDown(vk)
	}

	// Символьный ввод
	if w.onChar != nil {
		chars := event.Send(selCharacters)
		if chars != 0 {
			cstr := uintptr(objc.Send[uintptr](chars, selUTF8String))
			if cstr != 0 {
				s := goString(cstr)
				for _, r := range s {
					if r >= 32 {
						w.onChar(r)
					}
				}
			}
		}
	}
}

func (w *CocoaWindow) handleKeyUp(event objc.ID) {
	keyCode := int(objc.Send[uintptr](event, selKeyCode))
	vk := cocoaKeyCodeToVK(keyCode)
	if w.onKeyUp != nil && vk != 0 {
		w.onKeyUp(vk)
	}
}

func (w *CocoaWindow) Close() {
	w.closed = true
	if w.nsWindow != 0 {
		w.nsWindow.Send(selClose)
	}
}

func (w *CocoaWindow) SetTitle(title string) {
	w.title = title
	w.setCocoaTitle(title)
}

func (w *CocoaWindow) setCocoaTitle(title string) {
	if w.nsWindow != 0 {
		nsStr := nsString(title)
		w.nsWindow.Send(selSetTitle, uintptr(nsStr))
	}
}

func (w *CocoaWindow) SetSize(width, height int) {
	w.width = width
	w.height = height
	// TODO: setFrame:display:
}

func (w *CocoaWindow) GetSize() (int, int) {
	return w.width, w.height
}

func (w *CocoaWindow) SetPosition(x, y int) {
	// TODO: setFrameOrigin:
}

func (w *CocoaWindow) GetPosition() (int, int) {
	return 0, 0
}

func (w *CocoaWindow) Minimize() {
	if w.nsWindow != 0 {
		w.nsWindow.Send(selMiniaturize, 0)
	}
}

func (w *CocoaWindow) Maximize() {
	if w.nsWindow != 0 && !w.IsMaximized() {
		w.nsWindow.Send(selZoom, 0)
		w.maximized = true
	}
}

func (w *CocoaWindow) Restore() {
	if w.nsWindow != 0 && w.IsMaximized() {
		w.nsWindow.Send(selZoom, 0)
		w.maximized = false
	}
}

func (w *CocoaWindow) IsMaximized() bool {
	if w.nsWindow != 0 {
		ret := objc.Send[bool](w.nsWindow, selIsZoomed)
		w.maximized = ret
	}
	return w.maximized
}

// SetCornerRadius — скругление окна на macOS (пока no-op; Cocoa скругляет
// стандартные окна сама).
func (w *CocoaWindow) SetCornerRadius(int) {}

func (w *CocoaWindow) BlitRGBA(img *image.RGBA) {
	if w.nsView == 0 || img == nil {
		return
	}

	b := img.Bounds()
	width := b.Dx()
	height := b.Dy()
	if width <= 0 || height <= 0 {
		return
	}

	if w.legacyBlit || w.nsLayer == 0 {
		w.blitRGBALegacy(img, width, height)
		return
	}

	// CALayer-путь: пиксели → CFData (копия) → CGImage → layer.contents.
	// Без переворота Y: CGImage отображается в contents в естественной
	// ориентации (строка 0 — верх).
	//
	// SEC-6: CFDataCreate КОПИРУЕТ пиксели синхронно, здесь и сейчас — в память,
	// которой владеет CoreFoundation. Прежний вариант (CGDataProviderCreateWithData
	// поверх Go-слайса без release-колбэка) оставлял Core Animation указатель в
	// Go-кучу: после ресайза буфер пересоздавался, старый уходил в GC, а слой мог
	// прочитать освобождённую память. Одна копия кадра (~1 мс на 1080p) — та же
	// цена, что была у прежнего copy в двойной буфер, но без use-after-free.
	pixLen := height * img.Stride
	if pixLen <= 0 || len(img.Pix) < pixLen {
		return
	}
	cfData := cfDataCreate(0, uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(pixLen))
	runtime.KeepAlive(img) // img.Pix должен жить до возврата CFDataCreate
	if cfData == 0 {
		return
	}
	provider := cgDataProviderCreateWithCFData(cfData)
	cfRelease(cfData) // провайдер удерживает собственную ссылку
	if provider == 0 {
		return
	}
	cgImg := cgImageCreate(
		uintptr(width), uintptr(height),
		8, 32, uintptr(img.Stride),
		cgColorSpace, cgBitmapRGBA8888,
		provider, 0,
		0, // shouldInterpolate = false
		0, // kCGRenderingIntentDefault
	)
	if cgImg == 0 {
		cgDataProviderRelease(provider)
		return
	}

	// Подмена contents в транзакции без implicit-анимаций.
	caTransaction := objc.ID(objc.GetClass("CATransaction"))
	caTransaction.Send(selBegin)
	caTransaction.Send(selSetDisableActions, 1)
	w.nsLayer.Send(selSetContents, uintptr(cgImg))
	caTransaction.Send(selCommit)

	// Слой удерживает contents; наши ссылки больше не нужны.
	cgImageRelease(cgImg)
	cgDataProviderRelease(provider)
}

// blitRGBALegacy — прежний путь вывода через NSBitmapImageRep + NSImage +
// lockFocus (deprecated с macOS 10.14). Оставлен как аварийный фолбэк:
// HEADLESS_GUI_COCOA_LEGACY=1.
func (w *CocoaWindow) blitRGBALegacy(img *image.RGBA, width, height int) {
	// SEC-6: NSBitmapImageRep с planes=NULL выделяет СОБСТВЕННЫЙ буфер; пиксели
	// копируем в него через -bitmapData (Cocoa Y=0 внизу → переворачиваем).
	// Прежний вариант передавал указатель на Go-слайс, который репрезентация
	// не копирует и который переживает возврат из функции (autorelease pool,
	// NSImage-кэш) — при ресайзе старый слайс уходил в GC под ногами у Cocoa.
	rowLen := width * 4

	// Создаём NSBitmapImageRep с внутренним буфером
	nsBitmapClass := objc.ID(objc.GetClass("NSBitmapImageRep"))
	bitmapAlloc := nsBitmapClass.Send(selAlloc)

	// NSDeviceRGBColorSpace
	colorSpace := nsString("NSDeviceRGBColorSpace")

	// initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:
	//   hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:
	bitmapRep := bitmapAlloc.Send(selInitWithBitmapDataPlanes,
		uintptr(0),          // planes = NULL → буфер выделяет сама репрезентация
		uintptr(width),      // pixelsWide
		uintptr(height),     // pixelsHigh
		uintptr(8),          // bitsPerSample
		uintptr(4),          // samplesPerPixel (RGBA)
		uintptr(1),          // hasAlpha = YES
		uintptr(0),          // isPlanar = NO
		uintptr(colorSpace), // colorSpaceName
		uintptr(rowLen),     // bytesPerRow
		uintptr(32),         // bitsPerPixel
	)
	colorSpace.Send(selRelease)
	if bitmapRep == 0 {
		return
	}
	dataPtr := bitmapRep.Send(selBitmapData)
	if dataPtr == 0 {
		bitmapRep.Send(selRelease)
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(dataPtr)), rowLen*height)
	stride := img.Stride
	for y := 0; y < height; y++ {
		srcOff := y * stride
		dstOff := (height - 1 - y) * rowLen
		copy(dst[dstOff:dstOff+rowLen], img.Pix[srcOff:srcOff+rowLen])
	}
	runtime.KeepAlive(img)

	// Создаём NSImage и добавляем представление
	nsImageClass := objc.ID(objc.GetClass("NSImage"))
	imgAlloc := nsImageClass.Send(selAlloc)

	imgSize := nsSize{Width: float64(width), Height: float64(height)}
	nsImage := imgAlloc.Send(selInitWithSize, uintptr(unsafe.Pointer(&imgSize)))

	nsImage.Send(selAddRepresentation, uintptr(bitmapRep))

	// lockFocus на view и рисуем
	w.nsView.Send(selLockFocus)

	// Рисуем NSImage в view
	dstRect := nsRect{
		Origin: nsPoint{X: 0, Y: 0},
		Size:   nsSize{Width: float64(width), Height: float64(height)},
	}
	srcRect := nsRect{} // NSZeroRect = вся картинка

	// drawInRect:fromRect:operation:fraction:
	// operation=2 (NSCompositingOperationCopy), fraction=1.0
	nsImage.Send(selDrawInRect,
		uintptr(unsafe.Pointer(&dstRect)),
		uintptr(unsafe.Pointer(&srcRect)),
		uintptr(2), // NSCompositingOperationCopy
		uintptr(1), // fraction=1.0 (полная непрозрачность)
	)

	// Flush
	nsGCtxClass := objc.ID(objc.GetClass("NSGraphicsContext"))
	ctx := nsGCtxClass.Send(selCurrentContext)
	if ctx != 0 {
		ctx.Send(selFlushGraphics)
	}

	w.nsView.Send(selUnlockFocus)

	// alloc/init дают retain=1: отпускаем обе наши ссылки (NSImage удерживает
	// репрезентацию сам) — иначе утечка двух объектов на каждый кадр.
	nsImage.Send(selRelease)
	bitmapRep.Send(selRelease)
}

// Callbacks
func (w *CocoaWindow) SetOnResize(fn func(w, h int))                            { w.onResize = fn }
func (w *CocoaWindow) SetOnClose(fn func() bool)                                { w.onClose = fn }
func (w *CocoaWindow) SetOnMouseMove(fn func(x, y int))                         { w.onMouseMove = fn }
func (w *CocoaWindow) SetOnMouseButton(fn func(x, y, button int, pressed bool)) { w.onMouseButton = fn }
func (w *CocoaWindow) SetOnKeyDown(fn func(vk int))                             { w.onKeyDown = fn }
func (w *CocoaWindow) SetOnKeyUp(fn func(vk int))                               { w.onKeyUp = fn }
func (w *CocoaWindow) SetOnChar(fn func(r rune))                                { w.onChar = fn }

// ─── Маппинг клавиш macOS → VK ─────────────────────────────────────────────

func cocoaKeyCodeToVK(keyCode int) int {
	switch keyCode {
	case 51:
		return VK_BACKSPACE
	case 48:
		return VK_TAB
	case 36:
		return VK_ENTER
	case 53:
		return VK_ESCAPE
	case 49:
		return VK_SPACE
	case 123:
		return VK_LEFT
	case 126:
		return VK_UP
	case 124:
		return VK_RIGHT
	case 125:
		return VK_DOWN
	case 117:
		return VK_DELETE
	case 114: // Mac-клавиатуры не имеют Insert; клавиша Help — best-effort маппинг
		return VK_INSERT
	case 115:
		return VK_HOME
	case 119:
		return VK_END
	case 116:
		return VK_PRIOR // Page Up
	case 121:
		return VK_NEXT // Page Down
	case 0:
		return VK_A
	case 8:
		return VK_C
	case 9:
		return VK_V
	case 7:
		return VK_X
	case 6:
		return VK_Z
	case 56, 60:
		return VK_SHIFT
	case 59, 62:
		return VK_CONTROL
	case 58, 61:
		return VK_ALT
	}
	return 0
}

// SetResizable — no-op: пользовательский resize за края borderless-окна
// на этой платформе пока не реализован.
func (w *CocoaWindow) SetResizable(v bool) {}

// SetMinSize — no-op: минимальный размер окна на macOS пока не ограничиваем.
func (w *CocoaWindow) SetMinSize(width, height int) {}
