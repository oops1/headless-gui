// Package engine — кэш TrueType шрифтов.
//
// Использует golang.org/x/image/font/opentype для рендеринга TTF/OTF.
// Встроенный шрифт: Go Regular (поддерживает Cyrillic, Latin, Greek и другие наборы).
// Если файл assets/fonts/Go-Regular.ttf присутствует, используется он;
// иначе — встроенный бинарный TTF из пакета gofont/goregular.
package engine

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// DefaultDPI — DPI по умолчанию для рендеринга шрифтов.
const DefaultDPI = 96.0

// DefaultFontSize — размер шрифта по умолчанию (в пунктах) для DrawText.
// При DefaultDPI (96) соответствует ~13 px высоты — как basicfont.Face7x13.
const DefaultFontSize = 10.0

// FontCache кэширует font.Face для разных размеров одного TTF-файла.
// Потокобезопасен.
type FontCache struct {
	mu    sync.RWMutex
	ttf   *opentype.Font
	cache map[float64]font.Face
	dpi   float64
}

// newFontCache создаёт кэш, загружая шрифт из assetsDir или используя встроенный.
func newFontCache(assetsDir string) *FontCache {
	data := loadFontData(assetsDir)
	return newFontCacheFromData(data, DefaultDPI)
}

// newFontCacheFromData создаёт FontCache из TTF-байт и заданного DPI.
// Возвращает nil, если данные невалидны.
func newFontCacheFromData(data []byte, dpi float64) *FontCache {
	parsed, err := opentype.Parse(data)
	if err != nil {
		parsed, err = opentype.Parse(goregular.TTF)
		if err != nil {
			return nil
		}
	}
	return &FontCache{
		ttf:   parsed,
		cache: make(map[float64]font.Face),
		dpi:   dpi,
	}
}

// Face возвращает font.Face для заданного размера (в пунктах).
// Кэшируется. Потокобезопасно.
func (fc *FontCache) Face(sizePt float64) font.Face {
	fc.mu.RLock()
	if f, ok := fc.cache[sizePt]; ok {
		fc.mu.RUnlock()
		return f
	}
	fc.mu.RUnlock()

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if f, ok := fc.cache[sizePt]; ok {
		return f
	}
	face, err := opentype.NewFace(fc.ttf, &opentype.FaceOptions{
		Size:    sizePt,
		DPI:     fc.dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return basicfont.Face7x13
	}
	fc.cache[sizePt] = face
	return face
}

// Measure возвращает ширину строки text в пикселях для шрифта размером sizePt.
func (fc *FontCache) Measure(text string, sizePt float64) int {
	face := fc.Face(sizePt)
	var w fixed.Int26_6
	prev := rune(-1)
	for _, r := range text {
		if prev >= 0 {
			w += face.Kern(prev, r)
		}
		a, ok := face.GlyphAdvance(r)
		if !ok {
			a, _ = face.GlyphAdvance('?')
		}
		w += a
		prev = r
	}
	return w.Round()
}

// MeasureRunes возвращает массив накопленных ширин: positions[0]==0,
// positions[i] — ширина первых i символов строки (в пикселях).
// Длина результата: len([]rune(text))+1.
func (fc *FontCache) MeasureRunes(text string, sizePt float64) []int {
	face := fc.Face(sizePt)
	runes := []rune(text)
	pos := make([]int, len(runes)+1)
	var w fixed.Int26_6
	for i, r := range runes {
		if i > 0 {
			w += face.Kern(runes[i-1], r)
		}
		a, ok := face.GlyphAdvance(r)
		if !ok {
			a, _ = face.GlyphAdvance('?')
		}
		w += a
		pos[i+1] = w.Round()
	}
	return pos
}

// SetDPI обновляет DPI и сбрасывает кэш face (чтобы шрифты перерендерились).
func (fc *FontCache) SetDPI(dpi float64) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.dpi = dpi
	fc.cache = make(map[float64]font.Face) // очищаем кэш
}

// loadFontData читает TTF из файла; если не удаётся — возвращает встроенный Go Regular.
func loadFontData(assetsDir string) []byte {
	candidates := []string{
		filepath.Join(assetsDir, "fonts", "Go-Regular.ttf"),
		"assets/fonts/Go-Regular.ttf",
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			return data
		}
	}
	return goregular.TTF
}
