package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// popupPair — две сцены с одинаковым содержимым: большой холст и холст попапа,
// вырезанный из него по rect (шахматка с клеткой 10, rect выровнен по клетке).
func popupPair(rect image.Rectangle) (big *Canvas, tc *translatingContext, small *Canvas) {
	big = checkerCanvas(200, 200, 10)
	small = checkerCanvas(rect.Dx(), rect.Dy(), 10)
	tc = &translatingContext{inner: small, dx: rect.Min.X, dy: rect.Min.Y}
	return
}

// samePixels сравнивает область inner большого холста с холстом попапа.
func samePixels(t *testing.T, big, small *Canvas, rect, inner image.Rectangle) {
	t.Helper()
	for y := inner.Min.Y; y < inner.Max.Y; y++ {
		for x := inner.Min.X; x < inner.Max.X; x++ {
			a := big.back.RGBAAt(x, y)
			b := small.back.RGBAAt(x-rect.Min.X, y-rect.Min.Y)
			if a != b {
				t.Fatalf("пиксель (%d,%d): холст=%v попап=%v", x, y, a, b)
			}
		}
	}
}

// TestTranslatingContext_ImplementsOptionalCapabilities — контекст попапа
// обязан отзываться на те же приведения типа, что и обычный холст: иначе
// PaintStyle не найдёт ни тени, ни размытия, ни скругления.
func TestTranslatingContext_ImplementsOptionalCapabilities(t *testing.T) {
	var ctx widget.DrawContext = &translatingContext{inner: newCanvas(10, 10, newFontCache("assets"))}
	if _, ok := ctx.(widget.BackdropDrawer); !ok {
		t.Error("контекст попапа не реализует BackdropDrawer")
	}
	if _, ok := ctx.(widget.ShadowDrawer); !ok {
		t.Error("контекст попапа не реализует ShadowDrawer")
	}
	if _, ok := ctx.(widget.RoundClipper); !ok {
		t.Error("контекст попапа не реализует RoundClipper")
	}
}

// TestTranslatingContext_BlurBehindMatchesCanvas — размытие через попап
// совпадает с размытием на обычном холсте (вдали от краёв буфера, где захват
// у холстов разный).
func TestTranslatingContext_BlurBehindMatchesCanvas(t *testing.T) {
	rect := image.Rect(40, 20, 140, 120)
	big, tc, small := popupPair(rect)
	area := rect.Inset(10)

	big.BlurBehind(area, 6, color.RGBA{})
	tc.BlurBehind(area, 6, color.RGBA{})

	samePixels(t, big, small, rect, area.Inset(12))
	// И размытие реально произошло, а не «ничего не сделано в обоих»:
	// исходная шахматка в центре клетки чисто чёрная или белая.
	mixed := 0
	for y := area.Min.Y + 12; y < area.Max.Y-12; y += 4 {
		for x := area.Min.X + 12; x < area.Max.X-12; x += 4 {
			if p := small.back.RGBAAt(x-rect.Min.X, y-rect.Min.Y); p.R > 20 && p.R < 235 {
				mixed++
			}
		}
	}
	if mixed == 0 {
		t.Error("в буфере попапа нет полутонов — размытие не дошло до нижележащего холста")
	}
}

// TestTranslatingContext_DrawSoftShadowMatchesCanvas — тень в буфере попапа
// ложится туда же, куда и на холсте, с учётом смещения.
func TestTranslatingContext_DrawSoftShadowMatchesCanvas(t *testing.T) {
	rect := image.Rect(40, 20, 140, 120)
	big, tc, small := popupPair(rect)
	body := rect.Inset(20)
	shadow := color.RGBA{A: 120}

	big.DrawSoftShadow(body, 6, 8, shadow)
	tc.DrawSoftShadow(body, 6, 8, shadow)

	samePixels(t, big, small, rect, rect)
	// Тень заметна: под нижней кромкой тела (белая клетка шахматки) пиксель
	// потемнел.
	before := checkerCanvas(rect.Dx(), rect.Dy(), 10)
	if before.back.RGBAAt(45, 85) == small.back.RGBAAt(45, 85) {
		t.Error("тень не нарисована в буфере попапа")
	}
}

// TestTranslatingContext_RoundClipMatchesCanvas — скруглённый клип обрезает
// углы заливки в попапе так же, как на холсте, и снимается по ClearRoundClip.
func TestTranslatingContext_RoundClipMatchesCanvas(t *testing.T) {
	rect := image.Rect(40, 20, 140, 120)
	big, tc, small := popupPair(rect)
	red := color.RGBA{R: 255, A: 255}

	big.SetRoundClip(rect, 20)
	big.FillRect(rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy(), red)
	big.ClearRoundClip()

	tc.SetRoundClip(rect, 20)
	if !small.HasRoundClip() {
		t.Fatal("SetRoundClip не дошёл до нижележащего холста")
	}
	tc.FillRect(rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy(), red)
	tc.ClearRoundClip()
	if small.HasRoundClip() {
		t.Fatal("ClearRoundClip не снял скругление")
	}

	samePixels(t, big, small, rect, rect)
	if small.back.RGBAAt(0, 0) == red {
		t.Error("угол попапа залит: скругление не применилось")
	}
	if small.back.RGBAAt(rect.Dx()/2, rect.Dy()/2) != red {
		t.Error("центр попапа не залит")
	}
}
