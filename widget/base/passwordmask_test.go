package base

import "testing"

// glyphCtx — контекст, который знает про глифы шрифта по умолчанию.
type glyphCtx struct {
	DrawContext
	has map[rune]bool
}

func (c glyphCtx) HasGlyph(r rune) bool { return c.has[r] }

// Маску пароля выбирает шрифт (widget/text, textinput_mask_test.go): '●'
// берётся, только если он есть в шрифте. Вопрос про глиф доходит до шрифта и
// сквозь обёртку прокрутки.
func TestPasswordMask_ScrollViewForwardsGlyph(t *testing.T) {
	sv := &svOffsetCtx{inner: glyphCtx{has: map[rune]bool{'●': true}}}
	if has, known := ContextHasGlyph(sv, '●'); !known || !has {
		t.Errorf("svOffsetCtx не пробросил HasGlyph: has=%v known=%v", has, known)
	}
}
