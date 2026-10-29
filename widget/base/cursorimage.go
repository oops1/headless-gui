// cursorimage.go — курсор мыши из картинки (CursorImage) и событие
// относительного движения (RelativeMotion).
package base

import (
	"crypto/sha256"
	"encoding/binary"
	"image"
)

// MaxCursorImageSide — наибольшая сторона картинки курсора. Курсор указателя
// удалённого рабочего стола — до 384×384 (RDP), с запасом: некоторые
// компоновщики и системы допускают и больше, но дальше полуметра картинка уже
// не курсор, а ошибка.
const MaxCursorImageSide = 512

// CursorImage — готовая к показу картинка курсора: пиксели NRGBA (прямая
// альфа) без отступов между строками, горячая точка и ключ содержимого.
//
// Ключ — хеш пикселей, размера и горячей точки: по нему окно узнаёт ту же
// картинку, пришедшую повторно (сервер удалённого рабочего стола шлёт один и
// тот же указатель на каждое движение), и берёт готовый системный курсор из
// кэша, а не строит его заново. Значение неизменяемое: после создания его не
// правят, оно ходит между горутинами как есть.
type CursorImage struct {
	W, H       int
	Pix        []byte // NRGBA, W*H*4 байт, строка идёт сразу за строкой
	HotX, HotY int    // горячая точка в пикселях картинки, внутри неё
	Key        [32]byte
}

// cursorImageGeometry проверяет картинку и возвращает размер и горячую точку,
// прижатую к картинке. ok=false — картинка пуста или недопустима.
func cursorImageGeometry(img *image.NRGBA, hotX, hotY int) (w, h, hx, hy int, ok bool) {
	if img == nil {
		return 0, 0, 0, 0, false
	}
	b := img.Bounds()
	w, h = b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || w > MaxCursorImageSide || h > MaxCursorImageSide {
		return 0, 0, 0, 0, false
	}
	return w, h, clampInt(hotX, 0, w-1), clampInt(hotY, 0, h-1), true
}

// cursorKey — хеш картинки: размер, горячая точка и пиксели построчно.
func cursorKey(w, h, hotX, hotY int, row func(y int) []byte) (key [32]byte) {
	hs := sha256.New()
	var hdr [16]byte
	binary.LittleEndian.PutUint32(hdr[0:], uint32(w))
	binary.LittleEndian.PutUint32(hdr[4:], uint32(h))
	binary.LittleEndian.PutUint32(hdr[8:], uint32(hotX))
	binary.LittleEndian.PutUint32(hdr[12:], uint32(hotY))
	hs.Write(hdr[:])
	for y := 0; y < h; y++ {
		hs.Write(row(y))
	}
	hs.Sum(key[:0])
	return key
}

// CursorImageKey возвращает ключ картинки, не копируя её: тот же, что получит
// NewCursorImage. ok=false — картинка недопустима. Нужен, чтобы повторную
// установку той же картинки распознавать без выделения памяти под её копию.
func CursorImageKey(img *image.NRGBA, hotX, hotY int) (key [32]byte, ok bool) {
	w, h, hx, hy, ok := cursorImageGeometry(img, hotX, hotY)
	if !ok {
		return key, false
	}
	bounds := img.Bounds()
	return cursorKey(w, h, hx, hy, func(y int) []byte {
		si := img.PixOffset(bounds.Min.X, bounds.Min.Y+y)
		return img.Pix[si : si+w*4]
	}), true
}

// NewCursorImage копирует картинку в CursorImage. nil — картинка пуста или
// недопустима (пустые границы, сторона больше MaxCursorImageSide). Горячая
// точка, вылезшая за картинку, прижимается к её краю.
//
// Копия нужна, чтобы приложение могло менять свой буфер дальше: указатель
// удалённого рабочего стола обычно собирается в одной и той же картинке.
func NewCursorImage(img *image.NRGBA, hotX, hotY int) *CursorImage {
	w, h, hx, hy, ok := cursorImageGeometry(img, hotX, hotY)
	if !ok {
		return nil
	}
	b := img.Bounds()
	ci := &CursorImage{W: w, H: h, HotX: hx, HotY: hy, Pix: make([]byte, w*h*4)}
	for y := 0; y < h; y++ {
		si := img.PixOffset(b.Min.X, b.Min.Y+y)
		copy(ci.Pix[y*w*4:(y+1)*w*4], img.Pix[si:si+w*4])
	}
	ci.Key = cursorKey(w, h, hx, hy, func(y int) []byte { return ci.Pix[y*w*4 : (y+1)*w*4] })
	return ci
}

// RelativeMotion — относительное движение мыши в режиме захвата указателя
// (window.Window.SetPointerLock): сдвиг в единицах устройства, без ускорения
// ОС и без привязки к границам окна. DX вправо, DY вниз.
//
// Значения дробные: Wayland и X11 присылают доли пикселя, Win32 — целые.
type RelativeMotion struct {
	DX, DY float64
}
