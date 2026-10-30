package chrome

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

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
	classic := base.Win2000Theme()
	modern := base.DarkTheme()
	if classic.Style.Classic3D == modern.Style.Classic3D {
		t.Fatal("темы для теста неразличимы по форме")
	}

	win := NewWindow("Окно", 300, 200)
	win.SetBounds(image.Rect(0, 0, 300, 200))
	win.ApplyTheme(classic)

	// Общая тема — современная: именно её окно и читало бы из общей
	// переменной.
	popModern := base.PushThemeStyle(modern.Style)
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
	pop := base.PushThemeStyle(classic.Style)
	got := plain.frameW()
	pop()
	if got != classicFrameW {
		t.Errorf("окно без своей темы не послушалось общей: рамка %d вместо %d",
			got, classicFrameW)
	}
}
