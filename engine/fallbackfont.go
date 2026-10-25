package engine

// fallbackfont.go — ленивая загрузка шрифтов запасной цепочки (v3.38, E16).
//
// engine.New читал все системные шрифты цепочки сразу: на Windows это Segoe UI
// Symbol, Nirmala, Segoe UI Emoji и другие — около 22 МБ, 21 мс на старте и 53 МБ
// выделений (читалка росла вдвое, пока не вмещала файл). Запасной шрифт нужен
// только рунам, которых нет в основном, — обычный текст в цепочку не заходит
// вовсе.
//
// Теперь путь регистрируется сразу (порядок цепочки сохраняется), а файл
// читается и разбирается при первом обращении к этому месту цепочки: когда
// руна не нашлась в основном шрифте и в запасных перед ним. Цепочка
// просматривается по порядку и останавливается на первом шрифте с глифом, так
// что загружаются только шрифты до него включительно — те, что стоят после, не
// трогаются. Руну, которой нет нигде, проверяют все шрифты цепочки — каждый
// загружается один раз.
//
// Потокобезопасность: цепочку читает горутина кадра (fcForRune, шейпер), а
// измеритель (MeasureText и родня) — горутина приложения. Загрузка записи
// выполняется под её мьютексом один раз; готовый шрифт читается атомарно.

import (
	"os"
	"sync"
	"sync/atomic"
)

// fallbackFileReader читает файл запасного шрифта. Переменная — точка подмены
// для тестов («после New ни один файл не прочитан»).
var fallbackFileReader = readFontFile

// fallbackFont — запись запасной цепочки: уже готовый шрифт (AddFallbackFont с
// данными) или путь к файлу, прочитываемому при первом обращении. Записи
// общие у холстов-клонов (смена размера/масштаба), поэтому шрифт, загруженный
// одним, виден всем.
type fallbackFont struct {
	path string // "" — запись создана из готовых данных

	mu   sync.Mutex
	fc   atomic.Pointer[FontCache] // nil — ещё не загружен или не удалось
	done atomic.Bool               // загрузку пытались (успешно или нет)
}

// newReadyFallback — запись с готовым шрифтом.
func newReadyFallback(fc *FontCache) *fallbackFont {
	f := &fallbackFont{}
	f.fc.Store(fc)
	f.done.Store(true)
	return f
}

// newLazyFallback — запись с ленивой загрузкой файла path.
func newLazyFallback(path string) *fallbackFont { return &fallbackFont{path: path} }

// loaded возвращает шрифт, если он уже загружен (без загрузки): для рассылки
// настроек DPI и субпикселей — незагруженный возьмёт их при загрузке.
func (f *fallbackFont) loaded() *FontCache { return f.fc.Load() }

// get возвращает шрифт, загружая его при первом обращении; nil — файл не
// читается или не шрифт (запись молча пропускается, как раньше при New).
// ref — шрифт холста, у которого берутся DPI и режим субпикселей.
func (f *fallbackFont) get(ref *FontCache) *FontCache {
	if fc := f.fc.Load(); fc != nil || f.done.Load() {
		return f.fc.Load()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done.Load() {
		return f.fc.Load()
	}
	// DPI и субпиксели читаются и применяются под мьютексом записи: рассылка
	// настроек (setDPIAll, setTextSubpixel) берёт тот же мьютекс после того,
	// как поменяла основной шрифт, — загрузка либо увидит уже новые значения,
	// либо будет догнана рассылкой.
	var fc *FontCache
	if data, err := fallbackFileReader(f.path); err == nil {
		if fc = newFontCacheFromData(data, ref.currentDPI()); fc != nil {
			fc.subpixel = ref.Subpixel()
			fc.quality = ref.Quality()
		}
	}
	f.fc.Store(fc)
	f.done.Store(true)
	return fc
}

// apply применяет fn к загруженному шрифту под мьютексом записи (согласованно
// с get); незагруженная запись не трогается.
func (f *fallbackFont) apply(fn func(*FontCache)) {
	f.mu.Lock()
	if fc := f.fc.Load(); fc != nil {
		fn(fc)
	}
	f.mu.Unlock()
}

// currentDPI — DPI шрифта под его замком.
func (fc *FontCache) currentDPI() float64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.dpi
}

// eachFallback обходит цепочку по порядку, загружая записи по мере надобности,
// пока fn не вернёт true (найдено). ref — шрифт холста для настроек
// загружаемых записей.
func (c *Canvas) eachFallback(ref *FontCache, fn func(*FontCache) bool) {
	for _, fb := range c.fallbacks {
		if fc := fb.get(ref); fc != nil && fn(fc) {
			return
		}
	}
}

// addFallbackFontFile добавляет в цепочку шрифт-файл, не читая его: файл
// прочитается при первом обращении. Несуществующий, слишком большой файл или
// не файл в цепочку не попадает — как раньше, когда такой файл не
// прочитывался при New (от этого зависит быстрый путь без запасных шрифтов).
func (c *Canvas) addFallbackFontFile(path string) bool {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxFontFileSize {
		return false
	}
	c.fallbacks = append(c.fallbacks, newLazyFallback(path))
	c.fontRev.Add(1)
	return true
}
