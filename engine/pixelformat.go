// pixelformat.go — порядок цветовых каналов back-буфера (ЗАДАЧА: буфер сам
// знает свой порядок байт, а не потребитель кадра, переставляющий их
// попиксельным циклом после копирования).
package engine

import (
	"fmt"
	"image/color"
)

// PixelFormat — порядок цветовых каналов в back-буфере канваса.
//
// Раньше единственный порядок был RGBA (как у Go image.RGBA), а потребитель
// с другими ожиданиями (например RDP-оболочка, которой нужен BGRX для
// DIB/GDI-пути) переставлял каналы сам, попиксельно, уже после копирования
// кадра к себе — лишний проход по каждому кадру на стороне, которая ничего
// не растеризует. Формат — параметр буфера: растеризатор кладёт байты сразу
// в нужном порядке (см. Canvas.enc и Engine.SetPixelFormat).
type PixelFormat uint8

const (
	// FormatRGBA — R,G,B,A по возрастанию адреса: как сегодня, как Go
	// image.RGBA. Значение по умолчанию (нулевое) — не задавая формат,
	// потребитель получает байт-в-байт прежнее поведение.
	FormatRGBA PixelFormat = iota
	// FormatBGRX — R и B переставлены местами (B,G,R,X); четвёртый байт (X)
	// несёт ту же альфу, что и в FormatRGBA (просто другое имя канала —
	// многие потребители DIB считают его "не используется", но мы кладём
	// туда настоящую альфу, как и раньше).
	FormatBGRX
)

// String — для логов и отладочных сообщений об ошибках.
func (f PixelFormat) String() string {
	switch f {
	case FormatRGBA:
		return "RGBA"
	case FormatBGRX:
		return "BGRX"
	default:
		return fmt.Sprintf("PixelFormat(%d)", uint8(f))
	}
}

// enc переводит "логический" цвет col (как его строит виджет — обычные
// R,G,B,A) в порядок байт ТЕКУЩЕГО back-буфера канваса.
//
// Единая точка преобразования: примитивы записи (fillRectPx, setPixelPx,
// drawAlphaMask) складывают байты col.R,col.G,col.B,col.A в буфер как есть,
// без всякого понятия о формате. Если приёмнику нужен другой порядок —
// его меняет цвет ДО того, как дойдёт до записи, а не сама запись раз за
// разом на каждый пиксель. Ровно это раньше делал потребитель кадра постфактум.
//
// ИСКЛЮЧЕНИЕ: код, который копирует байты уже ИЗ буфера того же формата
// (backdrop.go — снял пиксели с back, размыл, положил обратно; blitBackground
// — копирует заранее закодированный фон) через enc не идёт: там нечего
// перекодировать, байты и так в целевом порядке.
func (c *Canvas) enc(col color.RGBA) color.RGBA {
	if c.format == FormatBGRX {
		col.R, col.B = col.B, col.R
	}
	return col
}

// swapRB меняет местами R и B во всех пикселях плотно упакованного
// RGBA-буфера (Stride == 4×ширина — верно для свежего image.NewRGBA,
// поэтому проход по Pix напрямую, без PixOffset).
func swapRB(pix []byte) {
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+2] = pix[i+2], pix[i]
	}
}

// SetPixelFormat задаёт порядок каналов back-буфера. По умолчанию —
// FormatRGBA (прежнее поведение, байт в байт).
//
// Меняет ТОЛЬКО порядок байт при записи — геометрия, тайлы, damage не
// затрагиваются. Уже загруженный фон (SetBackgroundFile) перестраивается
// под новый формат сразу здесь, а не при следующем кадре: blitBackground
// каждый кадр делает чистый memcpy заранее закодированного фона, и держать
// его в устаревшем порядке байт до первой перерисовки было бы врать кадру.
//
// Вызывать в любой момент; следующий кадр перерисуется целиком (Invalidate),
// так как весь back-буфер теперь в другом порядке байт.
func (e *Engine) SetPixelFormat(f PixelFormat) error {
	if f != FormatRGBA && f != FormatBGRX {
		return fmt.Errorf("engine: SetPixelFormat: неизвестный формат %d", uint8(f))
	}
	e.frameMu.Lock() // буфер меняется — ждём конца текущего кадра
	e.mu.Lock()
	if e.canvas.format == f {
		e.mu.Unlock()
		e.frameMu.Unlock()
		return nil
	}
	e.canvas.setOwnFormat(f)
	if e.bgSrc != nil {
		e.canvas.setBackground(e.bgSrc) // перестроить фон в новом порядке байт
	}
	e.mu.Unlock()
	e.frameMu.Unlock()
	e.Invalidate() // формат сменился целиком — старый back теперь не в счёт
	return nil
}

// PixelFormat возвращает текущий порядок каналов back-буфера.
func (e *Engine) PixelFormat() PixelFormat {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.canvas.format
}
