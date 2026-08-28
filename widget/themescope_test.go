package widget

import (
	"image"
	"testing"
)

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

// Геометрия окна помнит свою тему и вне отрисовки.
//
// Высота заголовка и толщина рамки зависят от стиля: в классике Windows 2000
// рамка 5px и заголовок обрезан до 24px, в остальных темах — 1px и полная
// высота. Спрашивают эту геометрию не только из Draw: хит-тест заголовка,
// ContentBounds и перекладка детей зовутся из обработки ввода, когда подмена
// стиля области темы уже снята.
//
// Без собственного стиля у окна классическое окно внутри ThemeScope мерило
// себя по общей теме — заголовок «кончался» на восемь точек ниже, чем
// выглядел, и клик у его нижнего края уходил в содержимое вместо
// перетаскивания.
func TestWindow_GeometryUsesOwnThemeOutsideDraw(t *testing.T) {
	classic := Win2000Theme()
	modern := DarkTheme()
	if classic.Style.Classic3D == modern.Style.Classic3D {
		t.Fatal("темы для теста неразличимы по форме")
	}

	win := NewWindow("Окно", 300, 200)
	win.SetBounds(image.Rect(0, 0, 300, 200))
	win.ApplyTheme(classic)

	// Общая тема — современная: именно её окно и читало бы из общей
	// переменной.
	popModern := pushThemeStyle(modern.Style)
	inDraw := struct {
		title   int
		frame   int
		content image.Rectangle
	}{win.effTitleH(), win.frameW(), win.ContentBounds()}
	popModern()

	// А теперь то же самое ВНЕ отрисовки — как при обработке мыши.
	if got := win.effTitleH(); got != inDraw.title {
		t.Errorf("высота заголовка вне отрисовки %d, при отрисовке %d", got, inDraw.title)
	}
	if got := win.frameW(); got != inDraw.frame {
		t.Errorf("толщина рамки вне отрисовки %d, при отрисовке %d", got, inDraw.frame)
	}
	if got := win.ContentBounds(); got != inDraw.content {
		t.Errorf("клиентская область вне отрисовки %v, при отрисовке %v", got, inDraw.content)
	}

	// И это именно классическая геометрия, а не «одинаково неправильная».
	if win.frameW() != classicFrameW {
		t.Errorf("рамка %d, ждали классическую %d", win.frameW(), classicFrameW)
	}

	// Окно без назначенной темы по-прежнему следует общей.
	plain := NewWindow("Обычное", 300, 200)
	plain.SetBounds(image.Rect(0, 0, 300, 200))
	pop := pushThemeStyle(classic.Style)
	got := plain.frameW()
	pop()
	if got != classicFrameW {
		t.Errorf("окно без своей темы не послушалось общей: рамка %d вместо %d",
			got, classicFrameW)
	}
}
