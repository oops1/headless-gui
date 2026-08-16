package engine

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"

	tsfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/math/fixed"
)

// pngHeaderOnly собирает PNG из сигнатуры и IHDR заданных размеров.
func pngHeaderOnly(w, h uint32) []byte {
	ihdr := []byte("IHDR")
	ihdr = binary.BigEndian.AppendUint32(ihdr, w)
	ihdr = binary.BigEndian.AppendUint32(ihdr, h)
	ihdr = append(ihdr, 8, 6, 0, 0, 0) // 8 бит, RGBA, deflate, без чересстрочности

	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	binary.Write(&b, binary.BigEndian, uint32(len(ihdr)-4))
	b.Write(ihdr)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(ihdr))
	return b.Bytes()
}

// PNG-бомба в растровом глифе отклоняется по заголовку, без аллокации пикселей.
func TestGlyphPNG_BombRejected(t *testing.T) {
	bomb := pngHeaderOnly(40000, 40000)
	if _, err := png.DecodeConfig(bytes.NewReader(bomb)); err != nil {
		t.Fatalf("тестовый заголовок PNG невалиден: %v", err)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < 100; i++ {
		if glyphPNGSane(bomb) {
			t.Fatal("PNG 40000×40000 принят — бомба не отсечена")
		}
	}
	runtime.ReadMemStats(&after)

	const limit = 1 << 20
	if got := after.TotalAlloc - before.TotalAlloc; got > limit {
		t.Errorf("100 отказов выделили %d байт, ожидалось < %d", got, limit)
	}
}

// Заголовок разумного размера принимается, слишком большие данные — нет.
func TestGlyphPNG_SaneAndOversized(t *testing.T) {
	if !glyphPNGSane(pngHeaderOnly(64, 64)) {
		t.Error("PNG 64×64 отвергнут")
	}
	if glyphPNGSane(pngHeaderOnly(maxGlyphDim+1, 8)) {
		t.Errorf("PNG шириной %d принят", maxGlyphDim+1)
	}
	if glyphPNGSane(pngHeaderOnly(3000, 3000)) {
		t.Error("PNG 3000×3000 (9 Мпикс) принят — превышен бюджет")
	}
	if glyphPNGSane(nil) {
		t.Error("пустые данные приняты")
	}
	big := make([]byte, maxGlyphBitmapBytes+1)
	copy(big, pngHeaderOnly(8, 8))
	if glyphPNGSane(big) {
		t.Error("данные больше лимита приняты")
	}
}

// Бюджет итоговой маски цветного глифа.
func TestGlyphBudget_Bounds(t *testing.T) {
	cases := []struct {
		w, h int
		want bool
	}{
		{64, 64, true},
		{maxGlyphDim, 64, true},
		{0, 64, false},
		{64, -1, false},
		{maxGlyphDim + 1, 8, false},
		{2048, 2049, false}, // > 4 Мпикс
		{maxGlyphDim, maxGlyphDim, false},
	}
	for _, c := range cases {
		if got := glyphBudgetOK(c.w, c.h); got != c.want {
			t.Errorf("glyphBudgetOK(%d,%d) = %v, want %v", c.w, c.h, got, c.want)
		}
	}
}

// Суммарный бюджет пикселей по слоям COLRv1 останавливает обход.
func TestColrWalk_LayerBudget(t *testing.T) {
	w := &colrWalk{}
	layer := v1Layer{box: glyphBox{w: 2048, h: 2048}} // 4 Мпикс на слой
	added := 0
	for i := 0; i < 32; i++ {
		if !w.addLayer(layer) {
			break
		}
		added++
	}
	if !w.over {
		t.Fatal("бюджет слоёв не сработал")
	}
	if want := maxGlyphLayerPixels / (2048 * 2048); added != want {
		t.Errorf("принято %d слоёв, ожидалось %d", added, want)
	}
	if w.pixels <= maxGlyphLayerPixels {
		t.Errorf("счётчик пикселей %d не превысил бюджет %d", w.pixels, maxGlyphLayerPixels)
	}
}

// Обычные слои укладываются в бюджет и попадают в список.
func TestColrWalk_NormalLayersFit(t *testing.T) {
	w := &colrWalk{}
	for i := 0; i < maxColrLayers; i++ {
		if !w.addLayer(v1Layer{box: glyphBox{w: 64, h: 64}}) {
			t.Fatalf("слой %d отвергнут при нормальном размере", i)
		}
	}
	if len(w.layers) != maxColrLayers || w.over {
		t.Errorf("слоёв %d (over=%v), ожидалось %d", len(w.layers), w.over, maxColrLayers)
	}
}

// Глифы, зависящие от цвета текста, берутся из кэша без ре-композита.
func TestEmoji_DynColorCacheHit(t *testing.T) {
	var ts textShaper
	col := color.RGBA{R: 10, G: 20, B: 30, A: 255}
	key := maskKey{gid: 7, sizePx: fixed.I(16)}
	want := &colorGlyph{img: image.NewRGBA(image.Rect(0, 0, 2, 2)), offX: 1, offY: -3}

	// face == nil: рендер здесь запаникует, значит попадание честное.
	ts.faceColor = map[*tsfont.Face]bool{nil: true}
	ts.dynColorGlyphs = map[dynColorKey]*colorGlyph{{maskKey: key, col: col}: want}

	if got := ts.colorGlyphFor(nil, key.gid, key.sizePx, col); got != want {
		t.Fatalf("colorGlyphFor вернул %v, ожидался кэшированный глиф", got)
	}
	allocs := testing.AllocsPerRun(50, func() {
		ts.colorGlyphFor(nil, key.gid, key.sizePx, col)
	})
	if allocs != 0 {
		t.Errorf("попадание в кэш выделило %v аллокаций, ожидалось 0", allocs)
	}
}

// Повторная отрисовка эмодзи тем же цветом не растит кэши цветных глифов.
func TestEmoji_ColorGlyphCacheStable(t *testing.T) {
	eng := New(120, 80, 20)
	c := eng.canvas
	if _, found := c.fcForRune(c.fontCache, '🙂'); !found {
		t.Skip("нет системного шрифта с цветными эмодзи — пропуск")
	}
	col := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	c.blitBackground()
	c.DrawTextSize("🙂", 8, 8, 32, col)

	c.shaper.mu.Lock()
	n1 := len(c.shaper.colorGlyphs) + len(c.shaper.dynColorGlyphs)
	c.shaper.mu.Unlock()
	if n1 == 0 {
		t.Skip("цветной глиф не построен — нечего кэшировать")
	}

	for i := 0; i < 5; i++ {
		c.DrawTextSize("🙂", 8, 8, 32, col)
	}
	c.shaper.mu.Lock()
	n2 := len(c.shaper.colorGlyphs) + len(c.shaper.dynColorGlyphs)
	c.shaper.mu.Unlock()
	if n2 != n1 {
		t.Errorf("после повторных отрисовок в кэше %d записей, было %d", n2, n1)
	}
}

// Смена DPI сразу меняет ширину сложного текста: раскладки сброшены.
func TestShaping_SetDPIDropsLayouts(t *testing.T) {
	c := newShapingCanvas(t, 'م')
	const text = "مرحبا"
	w1 := c.measureWithFallback(c.fontCache, text, 14)
	if w1 <= 0 {
		t.Fatalf("ширина при DPI %v = %d", DefaultDPI, w1)
	}
	c.setDPIAll(DefaultDPI * 2)
	w2 := c.measureWithFallback(c.fontCache, text, 14)
	if w2 <= w1 {
		t.Errorf("после удвоения DPI ширина %d, была %d — раскладка не пересчитана", w2, w1)
	}
	c.setDPIAll(DefaultDPI)
	if w3 := c.measureWithFallback(c.fontCache, text, 14); w3 != w1 {
		t.Errorf("возврат к DPI %v дал ширину %d, ожидалась %d", DefaultDPI, w3, w1)
	}
}
