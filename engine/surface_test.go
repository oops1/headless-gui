package engine

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// pixelFormatScene строит одну и ту же сцену (заливка + изображение +
// текст, каждое в своём углу) на переданном движке — критерий приёмки
// ЗАДАЧИ "порядок каналов буфера" требует ровно такой сцены: она проходит
// через все точки записи цвета в буфер (fillRectPx, drawAlphaMask,
// blitOver/DrawImage), которые задача просит покрыть.
func pixelFormatScene(eng *Engine) {
	const w, h = 320, 320

	root := widget.NewPanel(color.RGBA{R: 10, G: 10, B: 10, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	// Заливка.
	fill := widget.NewPanel(color.RGBA{R: 200, G: 30, B: 40, A: 255})
	fill.ShowHeader = false
	fill.SetBounds(image.Rect(0, 0, 64, 64))
	root.AddChild(fill)

	// Полупрозрачная заливка — отдельный (Over, не Src) путь fillRectRaw.
	trans := widget.NewPanel(color.RGBA{})
	trans.ShowHeader = false
	trans.SetBounds(image.Rect(80, 0, 144, 64))
	trans.UseAlpha = true
	trans.Background = premulAlphaForTest(color.RGBA{R: 90, G: 60, B: 220, A: 255}, 140)
	root.AddChild(trans)

	// Изображение —blitOver.
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = uint8(i)
	}
	pic := widget.NewImageWidget()
	pic.SetImage(img)
	pic.SetBounds(image.Rect(256, 0, 320, 64))
	root.AddChild(pic)

	// Текст — drawAlphaMask.
	label := widget.NewLabel("Текст BGRX", color.RGBA{R: 240, G: 240, B: 240, A: 255})
	label.SetBounds(image.Rect(4, 260, 220, 300))
	root.AddChild(label)

	eng.SetRoot(root)
}

// premulAlphaForTest переводит straight-alpha цвет в premultiplied
// (см. предупреждение про "premultiplied trap" в AI_AGENT_REFERENCE.md) —
// без своего маленького хелпера пришлось бы тянуть widget.premulAlpha,
// которая не экспортирована.
func premulAlphaForTest(base color.RGBA, alpha uint8) color.RGBA {
	a := uint32(alpha)
	return color.RGBA{
		R: uint8(uint32(base.R) * a / 255),
		G: uint8(uint32(base.G) * a / 255),
		B: uint8(uint32(base.B) * a / 255),
		A: alpha,
	}
}

// TestPixelFormatDefaultIsRGBA — формат по умолчанию (нулевое значение
// PixelFormat) обязан быть RGBA: SetPixelFormat никогда не вызывался,
// поведение обязано остаться прежним, байт в байт.
func TestPixelFormatDefaultIsRGBA(t *testing.T) {
	eng := New(64, 64, 30)
	if got := eng.PixelFormat(); got != FormatRGBA {
		t.Fatalf("формат по умолчанию = %v, ждали %v", got, FormatRGBA)
	}
}

// TestSetPixelFormatRejectsUnknown — неизвестное значение формата не должно
// молча приниматься (иначе enc() тихо решит, что это не BGRX, и просто
// ничего не переставит — а вызывающий будет думать, что задал формат).
func TestSetPixelFormatRejectsUnknown(t *testing.T) {
	eng := New(64, 64, 30)
	if err := eng.SetPixelFormat(PixelFormat(99)); err == nil {
		t.Fatal("SetPixelFormat(99) — ждали ошибку, получили nil")
	}
	if got := eng.PixelFormat(); got != FormatRGBA {
		t.Fatalf("формат изменился после отклонённого SetPixelFormat: %v", got)
	}
}

// TestPixelFormatBGRXMatchesRGBASwapped — критерий приёмки: кадр, снятый при
// FormatBGRX, побайтово совпадает с перестановкой каналов R/B кадра, снятого
// при FormatRGBA, на ОДНОЙ и той же сцене (заливка, включая alpha-blend,
// изображение и текст).
func TestPixelFormatBGRXMatchesRGBASwapped(t *testing.T) {
	const w, h = 320, 320

	rgba := New(w, h, 30)
	pixelFormatScene(rgba)
	rgba.renderFrame()

	bgrx := New(w, h, 30)
	if err := bgrx.SetPixelFormat(FormatBGRX); err != nil {
		t.Fatalf("SetPixelFormat(FormatBGRX): %v", err)
	}
	if got := bgrx.PixelFormat(); got != FormatBGRX {
		t.Fatalf("PixelFormat() = %v после SetPixelFormat(FormatBGRX)", got)
	}
	pixelFormatScene(bgrx)
	bgrx.renderFrame()

	want := rgba.canvas.front.Pix
	got := bgrx.canvas.front.Pix
	if len(want) != len(got) {
		t.Fatalf("разный размер буфера: rgba=%d bgrx=%d", len(want), len(got))
	}

	mismatches := 0
	for i := 0; i+3 < len(want); i += 4 {
		wr, wg, wb, wa := want[i], want[i+1], want[i+2], want[i+3]
		gr, gg, gb, ga := got[i], got[i+1], got[i+2], got[i+3]
		if gr != wb || gg != wg || gb != wr || ga != wa {
			mismatches++
			if mismatches <= 5 {
				x, y := (i/4)%w, (i/4)/w
				t.Errorf("пиксель (%d,%d): RGBA=(%d,%d,%d,%d) BGRX=(%d,%d,%d,%d) — ждали BGRX==(B,G,R,A) от RGBA",
					x, y, wr, wg, wb, wa, gr, gg, gb, ga)
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d пикселей из %d не совпали с перестановкой R/B", mismatches, len(want)/4)
	}
}

// TestPixelFormatBGRXBackgroundImage — то же самое, но с загруженным фоном
// (SetBackgroundFile идёт через отдельный путь — setBackground/blitBackground,
// закодированный ОДИН раз при установке фона, а не на каждый кадр). Меняем
// формат ПОСЛЕ загрузки фона, чтобы проверить перекодирование уже
// установленного bgImage в SetPixelFormat.
func TestPixelFormatBGRXBackgroundImage(t *testing.T) {
	const w, h = 128, 96

	bg := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			bg.SetRGBA(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 2), B: 77, A: 255})
		}
	}

	rgba := New(w, h, 30)
	rgba.mu.Lock()
	rgba.bgSrc = bg
	rgba.canvas.setBackground(bg)
	rgba.mu.Unlock()
	rgba.SetRoot(widget.NewPanel(color.RGBA{}))
	rgba.renderFrame()

	bgrx := New(w, h, 30)
	bgrx.mu.Lock()
	bgrx.bgSrc = bg
	bgrx.canvas.setBackground(bg)
	bgrx.mu.Unlock()
	if err := bgrx.SetPixelFormat(FormatBGRX); err != nil { // должен перестроить уже установленный фон
		t.Fatalf("SetPixelFormat: %v", err)
	}
	bgrx.SetRoot(widget.NewPanel(color.RGBA{}))
	bgrx.renderFrame()

	want := rgba.canvas.front.Pix
	got := bgrx.canvas.front.Pix
	for i := 0; i+3 < len(want); i += 4 {
		if got[i] != want[i+2] || got[i+1] != want[i+1] || got[i+2] != want[i] || got[i+3] != want[i+3] {
			t.Fatalf("пиксель %d: фон не переставлен корректно (rgba=%v bgrx=%v)",
				i/4, want[i:i+4], got[i:i+4])
		}
	}
}

// ─── SetSurface: рисование в чужую память ───────────────────────────────────

func TestSetSurfaceRejectsSmallStride(t *testing.T) {
	eng := New(64, 64, 30)
	buf := make([]byte, 64*64*4)
	if err := eng.SetSurface(buf, 64*4-1, FormatRGBA); err == nil {
		t.Fatal("SetSurface со stride меньше ширины строки — ждали ошибку")
	}
}

func TestSetSurfaceRejectsSmallBuffer(t *testing.T) {
	eng := New(64, 64, 30)
	buf := make([]byte, 64*64*4-1) // на 1 байт меньше необходимого
	if err := eng.SetSurface(buf, 64*4, FormatRGBA); err == nil {
		t.Fatal("SetSurface с буфером меньше физического размера холста — ждали ошибку")
	}
}

func TestSetSurfaceRejectsUnknownFormat(t *testing.T) {
	eng := New(64, 64, 30)
	buf := make([]byte, 64*64*4)
	if err := eng.SetSurface(buf, 64*4, PixelFormat(99)); err == nil {
		t.Fatal("SetSurface с неизвестным форматом — ждали ошибку")
	}
}

// TestSetSurfaceNilReverts — SetSurface(nil, …) обязан вернуть движок к
// собственному back-буферу (не к nil, не к последней чужой памяти).
func TestSetSurfaceNilReverts(t *testing.T) {
	eng := New(64, 64, 30)
	buf := make([]byte, 64*64*4)
	if err := eng.SetSurface(buf, 64*4, FormatRGBA); err != nil {
		t.Fatalf("SetSurface: %v", err)
	}
	if &eng.canvas.back.Pix[0] != &buf[0] {
		t.Fatal("back не переключился на переданную память")
	}
	if err := eng.SetSurface(nil, 0, 0); err != nil {
		t.Fatalf("SetSurface(nil): %v", err)
	}
	if eng.canvas.back != eng.canvas.backOwn {
		t.Fatal("после SetSurface(nil) back не вернулся к собственному буферу канваса")
	}
}

// TestSetSurfaceMatchesInternalBuffer — критерий приёмки: кадр, нарисованный
// в чужую память, обязан совпадать с кадром на внутреннем буфере (та же
// сцена, что и в TestPixelFormatBGRXMatchesRGBASwapped: заливка, включая
// alpha-blend, изображение и текст).
func TestSetSurfaceMatchesInternalBuffer(t *testing.T) {
	const w, h = 320, 320

	internal := New(w, h, 30)
	pixelFormatScene(internal)
	internal.renderFrame()

	external := New(w, h, 30)
	stride := w * 4
	buf := make([]byte, stride*h)
	if err := external.SetSurface(buf, stride, FormatRGBA); err != nil {
		t.Fatalf("SetSurface: %v", err)
	}
	pixelFormatScene(external)
	external.renderFrame()

	want := internal.canvas.front.Pix
	// front канваса с чужим back — собственный внутренний буфер (см.
	// SetSurface: front не переключается), после diff обязан совпасть.
	if got := external.canvas.front.Pix; !bytes.Equal(want, got) {
		t.Fatal("рендер в чужую память разошёлся с рендером на внутреннем буфере (front)")
	}
	// А сама чужая память — то, ради чего SetSurface вообще есть — обязана
	// содержать итоговый кадр напрямую, без промежуточной копии.
	if !bytes.Equal(want, buf) {
		t.Fatal("чужая память не содержит итоговый кадр напрямую")
	}
}

// TestSetSurfacePaddedStride — буфер потребителя со шагом строки БОЛЬШЕ
// физической ширины×4 (например, построчное выравнивание DIB). Проверяет,
// что запись идёт по PixOffset (учитывает stride), а не плоским циклом по
// w×4 — иначе со второй строки данные бы поехали в паддинг.
func TestSetSurfacePaddedStride(t *testing.T) {
	const w, h = 65, 40 // нечётная ширина — паддинг не выравнивается сам собой
	stride := w*4 + 32
	buf := make([]byte, stride*h)

	eng := New(w, h, 30)
	if err := eng.SetSurface(buf, stride, FormatRGBA); err != nil {
		t.Fatalf("SetSurface: %v", err)
	}
	root := widget.NewPanel(color.RGBA{R: 11, G: 22, B: 33, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	eng.SetRoot(root)
	eng.renderFrame()

	for y := 0; y < h; y++ {
		off := y * stride
		px := buf[off : off+4]
		if px[0] != 11 || px[1] != 22 || px[2] != 33 || px[3] != 255 {
			t.Fatalf("строка %d, смещение %d: пиксель = %v, ждали [11 22 33 255]", y, off, px)
		}
	}
}

// Возврат к собственному буферу возвращает и его формат.
//
// Формат — свойство буфера: чужая память вправе иметь свой порядок каналов,
// но собственный буфер обязан вернуться к тому, который просил вызывающий
// через SetPixelFormat. Иначе после круга «BGRX → внешний RGBA → свой» все
// кадры кодировались бы не тем форматом, а PixelFormat() сообщал бы неправду.
func TestSetSurfaceRestoresOwnFormatOnNil(t *testing.T) {
	eng := New(64, 48, 30)
	if err := eng.SetPixelFormat(FormatBGRX); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 64*4*48)
	if err := eng.SetSurface(buf, 64*4, FormatRGBA); err != nil {
		t.Fatal(err)
	}
	if got := eng.PixelFormat(); got != FormatRGBA {
		t.Errorf("на внешнем буфере формат %v, ждали RGBA", got)
	}

	if err := eng.SetSurface(nil, 0, FormatRGBA); err != nil {
		t.Fatal(err)
	}
	if got := eng.PixelFormat(); got != FormatBGRX {
		t.Errorf("после возврата к своему буферу формат %v, ждали BGRX — тот, что задавали", got)
	}
}
