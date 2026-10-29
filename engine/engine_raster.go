package engine

// engine_raster.go — настройки холста, которые приложение задаёт через Engine:
// формат пикселей буфера (pixelformat.go), кэш теней (shadowcache.go), сброс
// кэша картинок (imagecache.go). Сами механизмы — у холста.

import (
	"fmt"
	"image"
	"reflect"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
)

// InvalidateImageCache сбрасывает кэш масштабированных изображений
// (DrawImageScaled, DrawImageRect) для src; nil сбрасывает кэш целиком.
//
// Вызывать после того, как пиксели картинки переписаны на месте: кэш ключует
// источник по идентичности, содержимого он не сравнивает. Новая картинка
// (другой объект) сброса не требует.
//
// Безопасно с любой горутины: сброс записывается в журнал и применяется всеми
// холстами движка — основным и попапов — перед их следующим кадром. Сам по
// себе кадр сброс не заказывает; если картинка уже на экране, вызовите
// Invalidate (или InvalidateRect области картинки).
func (e *Engine) InvalidateImageCache(src image.Image) {
	if src == nil {
		e.imgInv.Add(0, true)
		return
	}
	// Обёртка непрозрачности (OpaqueImage) разворачивается, как у холста:
	// кэш масштабирования и хэш-дифф ведут поколение внутренней картинки.
	src, _ = hgdraw.IsOpaqueImage(src)
	v := reflect.ValueOf(src)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return // такие источники в кэш не попадают
	}
	e.imgInv.Add(v.Pointer(), false)
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
	e.bgGen.Add(1)
	if f != FormatRGBA && f != FormatBGRX {
		return fmt.Errorf("engine: SetPixelFormat: неизвестный формат %d", uint8(f))
	}
	e.frameMu.Lock() // буфер меняется — ждём конца текущего кадра
	e.mu.Lock()
	changed := e.canvas.SetPixelFormat(f, e.bgSrc)
	e.mu.Unlock()
	if changed {
		// Заготовка арены попапов — клон в прежнем формате (ADR-0009):
		// следующий попап получит клон в новом.
		e.ov.ReleaseArena()
	}
	e.frameMu.Unlock()
	if !changed {
		return nil
	}
	e.Invalidate() // формат сменился целиком — старый back теперь не в счёт
	return nil
}

// PixelFormat возвращает текущий порядок каналов back-буфера.
func (e *Engine) PixelFormat() PixelFormat {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.canvas.Format()
}

// SetShadowCache включает или выключает сборку теней из шаблона. Выключенный
// кэш возвращает прямое размытие на каждой отрисовке — для сравнения в
// тестах и на случай расхождения. По умолчанию включён.
func (e *Engine) SetShadowCache(on bool) {
	e.frameMu.Lock()
	e.mu.RLock()
	c := e.canvas
	e.mu.RUnlock()
	if c != nil {
		c.SetShadowCacheEnabled(on)
	}
	e.frameMu.Unlock()
	e.Invalidate()
}

// ShadowCache сообщает, включена ли сборка теней из шаблона.
func (e *Engine) ShadowCache() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.canvas == nil || e.canvas.ShadowCacheEnabled()
}
