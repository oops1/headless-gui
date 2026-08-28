package widget

import "testing"

// Подмена стиля на время отрисовки поддерева.
//
// Проверяется механика, а не картинка: областей темы может быть вложено
// сколько угодно, и внутренняя обязана вернуть стиль ВНЕШНЕЙ, а не сбросить
// его в общий. По кадру такую ошибку видно не всегда — она проявится только
// на виджете, нарисованном после вложенной области.
func TestPushThemeStyle_NestedRestoresOuter(t *testing.T) {
	if drawStyle != nil {
		t.Fatal("перед тестом стиль уже подменён")
	}

	outer := Win2000Theme().Style
	inner := DarkTheme().Style
	if outer.Classic3D == inner.Classic3D {
		t.Fatal("темы для теста неразличимы по форме")
	}

	popOuter := pushThemeStyle(outer)
	if got := currentStyle(); got.Classic3D != outer.Classic3D {
		t.Fatalf("внешняя область не подменила стиль: Classic3D=%v", got.Classic3D)
	}

	popInner := pushThemeStyle(inner)
	if got := currentStyle(); got.Classic3D != inner.Classic3D {
		t.Fatalf("вложенная область не подменила стиль: Classic3D=%v", got.Classic3D)
	}

	popInner()
	if got := currentStyle(); got.Classic3D != outer.Classic3D {
		t.Errorf("после вложенной области стиль стал %v, ждали внешний %v",
			got.Classic3D, outer.Classic3D)
	}

	popOuter()
	if drawStyle != nil {
		t.Error("после последней области подмена не снята — она осталась бы на весь кадр")
	}
	if got, want := currentStyle().Classic3D, win10.Style.Classic3D; got != want {
		t.Errorf("вернулся стиль %v вместо общего %v", got, want)
	}
}

// Область без темы ничего не подменяет: её дети рисуются общим стилем.
func TestThemeScope_NilThemeDoesNotSwap(t *testing.T) {
	s := NewThemeScope(nil)
	if s.HasOwnTheme() {
		t.Error("область без темы объявила себя со своей темой — глобальный обход её пропустит")
	}
	s.Draw(nil) // детей нет, подмены быть не должно
	if drawStyle != nil {
		t.Error("область без темы всё же подменила стиль")
	}
}
