package base

import "testing"

// glyphCtx — контекст, который знает про глифы шрифта по умолчанию.
type glyphCtx struct {
	DrawContext
	has map[rune]bool
}

func (c glyphCtx) HasGlyph(r rune) bool { return c.has[r] }

// Маска пароля по умолчанию — '●', только если он есть в шрифте по
// умолчанию; иначе '•'. На ALT '●' брался из шрифта-замены, и точки выходили
// невидимыми (замечание WinLine после v3.38.0).
func TestPasswordMask_DefaultFollowsFont(t *testing.T) {
	if got := passwordMask(glyphCtx{has: map[rune]bool{'●': true}}); got != '●' {
		t.Errorf("шрифт с '●': маска %q", got)
	}
	if got := passwordMask(glyphCtx{has: map[rune]bool{'•': true}}); got != '•' {
		t.Errorf("шрифт без '●': маска %q, ждали '•'", got)
	}
	if got := passwordMask(nil); got != '•' {
		t.Errorf("контекст без ответа про глифы: маска %q, ждали '•'", got)
	}
	// Поле пароля больше не прибивает '●': маску выбирает шрифт.
	if p := NewPasswordInput(""); p.MaskRune != 0 {
		t.Errorf("NewPasswordInput задал MaskRune %q, ждали 0 (по шрифту)", p.MaskRune)
	}
	// Обёртка прокрутки пробрасывает вопрос внутрь.
	sv := &svOffsetCtx{inner: glyphCtx{has: map[rune]bool{'●': true}}}
	if has, known := ContextHasGlyph(sv, '●'); !known || !has {
		t.Errorf("svOffsetCtx не пробросил HasGlyph: has=%v known=%v", has, known)
	}
}
