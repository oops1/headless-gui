// Тесты светящейся полосы прогресса (ProgressStyleGlow): свечение рисуется
// только в современных темах, в классике Win2000 и в Mac-теме остаётся
// штатный для темы вид.
package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// alphaCtx — recCtx, считающий полупрозрачные заливки (из них состоит
// свечение) и запоминающий их крайнюю правую точку.
type alphaCtx struct {
	recCtx
	alphaFills int
	maxAlphaX  int
	solidFills int
}

func (c *alphaCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	c.alphaFills++
	if x+w > c.maxAlphaX {
		c.maxAlphaX = x + w
	}
}
func (c *alphaCtx) FillRect(x, y, w, h int, col color.RGBA) { c.solidFills++ }

// Mac/Win11 рисуют дорожку скруглённой — считаем и её.
func (c *alphaCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) { c.solidFills++ }

// drawGlowBar рисует полосу заданного стиля в теме themeName и возвращает
// счётчики отрисовки.
func drawGlowBar(t *testing.T, themeName string, style widget.ProgressBarStyle, v float64) *alphaCtx {
	t.Helper()
	th := widget.ThemeByName(themeName)
	if th == nil {
		t.Fatalf("тема %q не найдена", themeName)
	}
	widget.ApplyGlobalTheme(th)
	t.Cleanup(func() {
		widget.ApplyGlobalTheme(widget.ThemeByName("Win10 Dark"))
		widget.StopAllAnimations()
	})

	pb := widget.NewProgressBar()
	pb.Style = style
	widget.ApplyThemeTree(pb, th)
	pb.SetValue(v)
	pb.SetBounds(image.Rect(20, 20, 420, 36))

	ctx := &alphaCtx{}
	pb.Draw(ctx)
	return ctx
}

// TestProgressGlow_ModernThemeGlows — в Win11 свечение рисуется, а штатный
// стиль полосы обходится без полупрозрачных заливок.
func TestProgressGlow_ModernThemeGlows(t *testing.T) {
	glow := drawGlowBar(t, "Win11 Dark", widget.ProgressStyleGlow, 0.5)
	if glow.alphaFills == 0 {
		t.Error("ProgressStyleGlow в Win11: свечение не нарисовано")
	}

	plain := drawGlowBar(t, "Win11 Dark", widget.ProgressStyleBar, 0.5)
	if plain.alphaFills != 0 {
		t.Errorf("ProgressStyleBar: %d полупрозрачных заливок, ждали 0", plain.alphaFills)
	}
}

// TestProgressGlow_ClassicAndMacKeepTheirBar — Win2000 и Mac рисуют свою
// каноническую полосу даже при запрошенном свечении.
func TestProgressGlow_ClassicAndMacKeepTheirBar(t *testing.T) {
	for _, name := range []string{"Win2000", "Mac"} {
		ctx := drawGlowBar(t, name, widget.ProgressStyleGlow, 0.5)
		if ctx.alphaFills != 0 {
			t.Errorf("%s: %d полупрозрачных заливок — свечение просочилось в тему",
				name, ctx.alphaFills)
		}
		if ctx.solidFills == 0 {
			t.Errorf("%s: полоса не нарисована вовсе", name)
		}
	}
}

// TestProgressGlow_HeadFollowsValue — голова свечения едет вслед за значением.
func TestProgressGlow_HeadFollowsValue(t *testing.T) {
	near := drawGlowBar(t, "Win11 Dark", widget.ProgressStyleGlow, 0.2)
	far := drawGlowBar(t, "Win11 Dark", widget.ProgressStyleGlow, 0.8)
	if far.maxAlphaX <= near.maxAlphaX {
		t.Errorf("правый край свечения: при 0.8 = %d, при 0.2 = %d — должен быть правее",
			far.maxAlphaX, near.maxAlphaX)
	}
}

// TestProgressGlow_KeepsFramesComing — светящаяся полоса просит кадры:
// в диалоге она не в фокусе, и без зациклённой анимации движок бы уснул.
func TestProgressGlow_KeepsFramesComing(t *testing.T) {
	widget.StopAllAnimations()
	ctx := drawGlowBar(t, "Win11 Dark", widget.ProgressStyleGlow, 0.4)
	_ = ctx
	if !widget.AnimationsActive() {
		t.Error("после отрисовки свечения нет активной анимации — кадры остановятся")
	}
}
