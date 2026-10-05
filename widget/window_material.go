package widget

// window_material.go — материал фона и мягкая тень окна по токенам темы.
//
// Окно по умолчанию не рисует ни того, ни другого: Background и без тени, как
// всегда. Профиль Windows 11 с флагами theme.FlagShadowSoft и
// theme.FlagBackdropMica (или приложение через SetShadow/SetBackdrop) включает
// их, не меняя вид остальных тем.

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
)

// SetBackdrop задаёт материал фона окна: theme.BackdropSpec с Material
// MaterialMica, MaterialMicaAlt, MaterialAcrylic или MaterialSolid. Нулевая
// спецификация возвращает заливку Background. Содержимое окна, чтобы материал
// был виден, не должно закрывать клиентскую область непрозрачным фоном.
func (w *Window) SetBackdrop(b theme.BackdropSpec) {
	if w.Backdrop == b {
		return
	}
	w.Backdrop = b
	w.Invalidate()
}

// SetShadow задаёт собственную тень окна (theme.Style.ResolveShadow даёт её из
// токенов стиля). Нулевая — тень из токенов темы, а без них окно без тени.
func (w *Window) SetShadow(s theme.ShadowSpec) {
	if w.Shadow == s {
		return
	}
	old := w.shadowExtent()
	w.Shadow = s
	w.invalidateShadowZone(old)
}

// shadowSpec — тень, которую окно рисует сейчас.
func (w *Window) shadowSpec() (theme.ShadowSpec, bool) {
	if !w.Shadow.IsZero() {
		return w.Shadow, true
	}
	// В нативном окне ОС тень за границами буфера всё равно обрежется, а у
	// классической темы её нет.
	if w.nativeHosted {
		return theme.ShadowSpec{}, false
	}
	st := w.style()
	if st.Classic3D || st.WindowShadow.IsZero() {
		return theme.ShadowSpec{}, false
	}
	return st.WindowShadow, true
}

// shadowExtent — на сколько пикселей тень выходит за границы окна.
func (w *Window) shadowExtent() int {
	sp, ok := w.shadowSpec()
	if !ok {
		return 0
	}
	return sp.Extent()
}

// DrawMargin реализует DrawMarginer: тень лежит за границами окна, и пропуск
// поддеревьев по повреждённой области не должен её отбрасывать.
func (w *Window) DrawMargin() int { return w.shadowExtent() }

// drawShadow рисует тень под окном (до фона).
func (w *Window) drawShadow(ctx DrawContext) {
	if sp, ok := w.shadowSpec(); ok {
		DrawShadowSpec(ctx, w.Bounds(), w.CornerRadius, sp)
	}
}

// drawBackdrop рисует материал фона вместо заливки Background. false —
// материал не назван, фон рисует вызывающий.
func (w *Window) drawBackdrop(ctx DrawContext) bool {
	if w.Backdrop.Material == theme.MaterialDefault {
		return false
	}
	r, cr := w.Bounds(), w.CornerRadius
	if cr > 0 {
		if rc, ok := ctx.(RoundClipper); ok {
			prev := ctx.Clip()
			rc.SetRoundClip(r, cr)
			defer func() {
				rc.ClearRoundClip()
				ctx.SetClip(prev)
			}()
		}
	}
	return PaintMaterial(ctx, r, cr, w.Backdrop)
}

// invalidateShadowZone перерисовывает кольцо тени вокруг окна — старое (оно
// могло быть шире) и нынешнее.
func (w *Window) invalidateShadowZone(oldExtent int) {
	ext := max(oldExtent, w.shadowExtent())
	if ext <= 0 {
		w.Invalidate()
		return
	}
	w.Base.invalidateRect(w.Bounds().Inset(-ext))
}

// invalidateMoved перерисовывает тень при смене положения и размера: пока
// окно ездит, кольцо тени вокруг прежних и новых границ тоже меняется.
func (w *Window) invalidateMoved(old image.Rectangle) {
	ext := w.shadowExtent()
	if ext <= 0 {
		return
	}
	w.Base.invalidateRect(old.Union(w.Bounds()).Inset(-ext))
}
