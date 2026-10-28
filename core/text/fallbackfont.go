package text

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

	"github.com/oops1/headless-gui/v3/internal/testhook"
)

// readFallbackFile читает файл запасного шрифта. Тесты подменяют чтение через
// testhook.ReadFallbackFont («после New ни один файл не прочитан»).
func readFallbackFile(path string) ([]byte, error) {
	if h := testhook.ReadFallbackFont; h != nil {
		return h(path)
	}
	return ReadFontFile(path)
}

// FallbackFont — запись запасной цепочки: уже готовый шрифт (AddFallbackFont с
// данными) или путь к файлу, прочитываемому при первом обращении. Записи
// общие у холстов-клонов (смена размера/масштаба), поэтому шрифт, загруженный
// одним, виден всем.
type FallbackFont struct {
	path string // "" — запись создана из готовых данных

	mu   sync.Mutex
	fc   atomic.Pointer[FontCache] // nil — ещё не загружен или не удалось
	done atomic.Bool               // загрузку пытались (успешно или нет)
}

// NewReadyFallback — запись с готовым шрифтом.
func NewReadyFallback(fc *FontCache) *FallbackFont {
	f := &FallbackFont{}
	f.fc.Store(fc)
	f.done.Store(true)
	return f
}

// NewFallbackFile — запись с ленивой загрузкой файла path, если это обычный
// файл не больше MaxFontFileSize; иначе nil: такой файл в цепочку не попадает,
// как раньше, когда он не прочитывался при New (от этого зависит быстрый путь
// без запасных шрифтов).
func NewFallbackFile(path string) *FallbackFont {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > MaxFontFileSize {
		return nil
	}
	return newLazyFallback(path)
}

// newLazyFallback — запись с ленивой загрузкой файла path.
func newLazyFallback(path string) *FallbackFont { return &FallbackFont{path: path} }

// Loaded возвращает шрифт, если он уже загружен (без загрузки): для рассылки
// настроек DPI и субпикселей — незагруженный возьмёт их при загрузке.
func (f *FallbackFont) Loaded() *FontCache { return f.fc.Load() }

// Get возвращает шрифт, загружая его при первом обращении; nil — файл не
// читается или не шрифт (запись молча пропускается, как раньше при New).
// ref — шрифт холста, у которого берутся DPI и режим субпикселей.
func (f *FallbackFont) Get(ref *FontCache) *FontCache {
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
	if data, err := readFallbackFile(f.path); err == nil {
		fc = NewFontCacheLike(data, ref)
	}
	f.fc.Store(fc)
	f.done.Store(true)
	return fc
}

// Apply применяет fn к загруженному шрифту под мьютексом записи (согласованно
// с get); незагруженная запись не трогается.
func (f *FallbackFont) Apply(fn func(*FontCache)) {
	f.mu.Lock()
	if fc := f.fc.Load(); fc != nil {
		fn(fc)
	}
	f.mu.Unlock()
}

// DPI — DPI шрифта под его замком.
func (fc *FontCache) DPI() float64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.dpi
}


