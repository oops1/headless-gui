package widget

// backdrop_material.go — материалы подложки (Solid, Acrylic, Mica, MicaAlt),
// тени по токенам стиля и режим «меньше движения» для виджетов.
//
// Это общий слой для desktop.PaintStyle, widget.Window, диалогов и меню:
// каждый из них спрашивает у стиля темы тень и подложку и рисует их одной и
// той же функцией, а не своими литералами.

import (
	"image"
	"image/color"
	"math"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// ShadowParamDrawer — опциональная возможность контекста рисовать мягкую тень
// с явными радиусом размытия и смещением (токены тени профиля). Реализуется
// engine.Canvas методом DrawShadow. Контекст с одним ShadowDrawer получает
// близкое приближение через DrawSoftShadow.
type ShadowParamDrawer interface {
	DrawShadow(r image.Rectangle, corner int, blur, offsetX, offsetY float64, col color.RGBA)
}

// MicaDrawer — опциональная возможность контекста положить в область
// размытые обои (материал Mica). Реализуется engine.Canvas методом MicaBehind.
// Вернул false — обоев нет (фон движка не задан); слой тогда рисуют сплошным
// цветом темы.
type MicaDrawer interface {
	MicaBehind(r image.Rectangle, radius int, tint color.RGBA) bool
}

// DrawShadowSpec рисует тень sp под слоем r со скруглением corner. Единая
// точка для всех, кто рисует тень по токенам стиля (theme.Style.ResolveShadow):
// PaintStyle, окно, диалог, меню. Пустая тень или контекст без тени — ничего.
func DrawShadowSpec(ctx DrawContext, r image.Rectangle, corner int, sp theme.ShadowSpec) {
	if sp.IsZero() || r.Empty() {
		return
	}
	if pd, ok := ctx.(ShadowParamDrawer); ok {
		pd.DrawShadow(r, corner, sp.Blur, sp.OffsetX, sp.OffsetY, sp.Color)
		return
	}
	if sd, ok := ctx.(ShadowDrawer); ok {
		// Старый контекст сдвигает тень вниз на половину высоты сам: добавляем
		// разницу между нужным смещением и этим.
		dx := int(math.Round(sp.OffsetX))
		dy := int(math.Round(sp.OffsetY - sp.Blur/2))
		sd.DrawSoftShadow(r.Add(image.Pt(dx, dy)), corner, sp.Blur, sp.Color)
	}
}

// MenuShadow — тень всплывающего меню по токенам активной темы (стиль "menu":
// ShadowBlur, ShadowOffsetX/Y, ShadowOpacity). false — профиль токенов не
// объявил, и PopupMenu остаётся на своей прежней тени.
//
// PopupMenu вызывает это в Draw вместо собственных литералов:
//
//	if sp, ok := MenuShadow(); ok {
//		DrawShadowSpec(ctx, menuRect, corner, sp)
//	} else {
//		// прежняя тень
//	}
//
// и добавляет sp.Extent() к области, которую меню инвалидирует и которую
// занимает его оверлей.
func MenuShadow() (theme.ShadowSpec, bool) {
	sp := currentStyle().MenuShadow
	return sp, !sp.IsZero()
}

// WindowShadow — тень окна по токенам активной темы (стиль "window").
func WindowShadow() (theme.ShadowSpec, bool) {
	sp := currentStyle().WindowShadow
	return sp, !sp.IsZero()
}

// DialogShadow — тень диалога по токенам активной темы (стиль "dialog").
func DialogShadow() (theme.ShadowSpec, bool) {
	sp := currentStyle().DialogShadow
	return sp, !sp.IsZero()
}

// PaintMaterial рисует подложку b, если у неё назван материал (Solid, Acrylic,
// Mica, MicaAlt), и сообщает, что нарисовала. false — материал не назван:
// слой ведёт себя по Mode, как до материалов, и рисует его вызывающий.
//
// Клип слоя (скруглённый или прямоугольный) ставит вызывающий; здесь только
// содержимое. Solid — сплошной Fallback (а без него подкраска Tint), без
// размытия и обоев. Acrylic — размытие нарисованного под слоем, подкраска,
// шум; без размытия в контексте — Fallback. Mica и MicaAlt — размытые обои
// (MicaDrawer), подкраска, без шума; нет обоев или контекст не умеет — Fallback.
func PaintMaterial(ctx DrawContext, r image.Rectangle, corner int, b theme.BackdropSpec) bool {
	if r.Empty() {
		return b.Material != theme.MaterialDefault
	}
	switch b.Material {
	case theme.MaterialSolid:
		paintSolidBackdrop(ctx, r, corner, b)
	case theme.MaterialAcrylic:
		if bd, ok := ctx.(BackdropDrawer); ok {
			bd.BlurBehind(r, int(b.Radius), b.Tint)
			if b.Noise > 0 {
				if nd, ok := ctx.(NoiseDrawer); ok {
					nd.DrawNoise(r, b.Noise)
				}
			}
		} else {
			paintSolidBackdrop(ctx, r, corner, b)
		}
	case theme.MaterialMica, theme.MaterialMicaAlt:
		if md, ok := ctx.(MicaDrawer); ok && md.MicaBehind(r, int(b.Radius), b.Tint) {
			return true
		}
		paintSolidBackdrop(ctx, r, corner, b)
	default:
		return false
	}
	return true
}

// paintSolidBackdrop — сплошной цвет темы вместо стекла.
func paintSolidBackdrop(ctx DrawContext, r image.Rectangle, corner int, b theme.BackdropSpec) {
	switch {
	case b.Fallback.A > 0 && corner > 0 && b.Fallback.A == 255:
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), corner, b.Fallback)
	case b.Fallback.A == 255:
		ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), b.Fallback)
	case b.Fallback.A > 0:
		ctx.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), b.Fallback)
	case b.Tint.A > 0:
		ctx.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), b.Tint)
	}
}

// ─── Меньше движения ────────────────────────────────────────────────────────

var reduceMotion atomic.Bool

// SetReduceMotion включает режим «меньше движения» для виджетов пакета: их
// декоративные анимации (переключатель, затухание диалога, тонкая полоса
// прокрутки, инерция, плавное значение индикатора) становятся мгновенными.
// Таймеры (задержки подсказок, мигание каретки, часы) режим не трогает.
//
// Компоненты оболочки (пакет desktop) читают тот же режим из темы через
// theme.Manager.GetAnimation; Engine.ApplyThemeProfile и Engine.SetMotionReduce
// держат оба в согласии с флагом theme.FlagMotionReduce.
func SetReduceMotion(on bool) { reduceMotion.Store(on) }

// ReduceMotion сообщает, включён ли режим «меньше движения» для виджетов.
func ReduceMotion() bool { return reduceMotion.Load() }

// MotionDur пропускает длительность декоративной анимации через режим «меньше
// движения»: при включённом режиме — нуль (мгновенно), иначе d без изменений.
// Своим анимациям виджета, не привязанным к токену темы, достаточно обернуть
// в неё длительность.
func MotionDur(d time.Duration) time.Duration {
	if reduceMotion.Load() {
		return 0
	}
	return d
}
