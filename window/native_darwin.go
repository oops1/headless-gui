//go:build darwin

package window

/*
macOS backend через purego — вызов Cocoa/AppKit API без CGO.

Используем purego objc runtime:
  objc.GetClass, objc.RegisterName, objc.ID.Send

Минимальный набор классов:
  NSApplication, NSWindow, NSView, NSEvent, NSImage, NSBitmapImageRep
*/

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"unsafe"

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
	mu        sync.Mutex

	// Callbacks
	onResize      func(w, h int)
	onClose       func() bool
	onMouseMove   func(x, y int)
	onMouseButton func(x, y, button int, pressed bool)
	onKeyDown     func(vk int)
	onKeyUp       func(vk int)
	onChar        func(r rune)

	// Для BlitRGBA
	frameBuf []byte
}

// Cocoa selectors (кэшируем)
var (
	selAlloc                     objc.SEL
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
	selCurrentContext            objc.SEL

	cocoaInited bool
)

func initCocoaSelectors() {
	if cocoaInited {
		return
	}
	selAlloc = objc.RegisterName("alloc")
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

	cocoaInited = true
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
	initCocoaSelectors()

	w.title = title
	w.width = width
	w.height = height

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
			uintptr(0xFFFFFFFF),      // NSAnyEventMask
			uintptr(distantFuture),
			uintptr(defaultMode),
			uintptr(1),               // dequeue = YES
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

	// Подготавливаем RGBA данные (Cocoa Y=0 внизу → переворачиваем)
	pixLen := width * height * 4
	w.mu.Lock()
	if len(w.frameBuf) < pixLen {
		w.frameBuf = make([]byte, pixLen)
	}
	stride := img.Stride
	for y := 0; y < height; y++ {
		srcOff := y * stride
		dstOff := (height - 1 - y) * width * 4
		copy(w.frameBuf[dstOff:dstOff+width*4], img.Pix[srcOff:srcOff+width*4])
	}

	dataPtr := uintptr(unsafe.Pointer(&w.frameBuf[0]))
	w.mu.Unlock()

	// Создаём NSBitmapImageRep из пиксельных данных
	nsBitmapClass := objc.ID(objc.GetClass("NSBitmapImageRep"))
	bitmapAlloc := nsBitmapClass.Send(selAlloc)

	// NSDeviceRGBColorSpace
	colorSpace := nsString("NSDeviceRGBColorSpace")

	// initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:
	//   hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:
	bitmapRep := bitmapAlloc.Send(selInitWithBitmapDataPlanes,
		uintptr(unsafe.Pointer(&dataPtr)), // planes (pointer to pointer to pixel data)
		uintptr(width),                     // pixelsWide
		uintptr(height),                    // pixelsHigh
		uintptr(8),                         // bitsPerSample
		uintptr(4),                         // samplesPerPixel (RGBA)
		uintptr(1),                         // hasAlpha = YES
		uintptr(0),                         // isPlanar = NO
		uintptr(colorSpace),                // colorSpaceName
		uintptr(width*4),                   // bytesPerRow
		uintptr(32),                        // bitsPerPixel
	)

	if bitmapRep == 0 {
		return
	}

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
		uintptr(2),   // NSCompositingOperationCopy
		uintptr(1),   // fraction=1.0 (полная непрозрачность)
	)

	// Flush
	nsGCtxClass := objc.ID(objc.GetClass("NSGraphicsContext"))
	ctx := nsGCtxClass.Send(selCurrentContext)
	if ctx != 0 {
		ctx.Send(selFlushGraphics)
	}

	w.nsView.Send(selUnlockFocus)
}

// Callbacks
func (w *CocoaWindow) SetOnResize(fn func(w, h int))                              { w.onResize = fn }
func (w *CocoaWindow) SetOnClose(fn func() bool)                                   { w.onClose = fn }
func (w *CocoaWindow) SetOnMouseMove(fn func(x, y int))                            { w.onMouseMove = fn }
func (w *CocoaWindow) SetOnMouseButton(fn func(x, y, button int, pressed bool))    { w.onMouseButton = fn }
func (w *CocoaWindow) SetOnKeyDown(fn func(vk int))                                { w.onKeyDown = fn }
func (w *CocoaWindow) SetOnKeyUp(fn func(vk int))                                  { w.onKeyUp = fn }
func (w *CocoaWindow) SetOnChar(fn func(r rune))                                   { w.onChar = fn }

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
	case 115:
		return VK_HOME
	case 119:
		return VK_END
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
