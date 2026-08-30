package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Подсветка при наведении и открытый оверлей.
//
// Движение доставлялось широковещательно и без учёта перекрытия: кнопка
// панели задач под открытым меню «Пуск» продолжала получать события и
// исправно подсвечивалась. Сквозь стеклянную панель Windows 11 подсветка
// видна — владелец описал это как «подсвечивается не то, что надо».

// hoverScene — область приложений и меню «Пуск» в дереве настоящего движка:
// проверяется именно доставка событий, а не поведение одного виджета.
//
// Область кладётся ТУДА, ГДЕ ВСТАНЕТ МЕНЮ, — перекрытие здесь и есть предмет
// проверки. На настоящей панели задач под меню попадают кнопки окон, часы и
// значки трея; геометрия у каждой оболочки своя, а механизм один.
func hoverScene(t *testing.T) (*engine.Engine, *ApplicationArea, *StartMenu) {
	t.Helper()

	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	m.SetIconResolver(widget.BuiltinIcons())

	root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 600))

	cat := NewStaticAppCatalog(
		AppInfo{ID: "term", Title: "Терминал"},
		AppInfo{ID: "notes", Title: "Заметки"},
	)
	cat.Pin("term")
	cat.Pin("notes")
	wm := NewFakeWindowModel(
		WindowInfo{ID: 10, AppID: "term", Title: "Терминал", Active: true},
		WindowInfo{ID: 11, AppID: "mail", Title: "Почта"},
	)

	area := NewApplicationArea(m, cat, wm)
	root.AddChild(area)

	menu := NewStartMenu(m, cat)
	menu.Screen = image.Rect(0, 0, 800, 600)
	root.AddChild(menu)

	// Открываем меню от значка «Пуск» в углу панели, узнаём его настоящий
	// прямоугольник и кладём область приложений внутрь него, после чего
	// закрываем — сцена готова, а тесты открывают меню сами.
	menu.Toggle(image.Rect(0, 560, 40, 600))
	over := menu.Bounds()
	if over.Empty() {
		t.Fatal("у открытого меню пустой прямоугольник")
	}
	menu.Close()
	area.SetBounds(image.Rect(over.Min.X, over.Min.Y+40, over.Max.X, over.Min.Y+80))

	eng := engine.New(800, 600, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()

	t.Cleanup(func() {
		area.Close()
		menu.Close()
	})
	return eng, area, menu
}

func moveTo(eng *engine.Engine, r image.Rectangle) {
	eng.SendMouseMove(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
}

func TestHover_ButtonUnderOpenMenuIsNotHighlighted(t *testing.T) {
	eng, area, menu := hoverScene(t)

	first := cellRect(t, area, 0)
	moveTo(eng, first)
	if area.HoverIndex() != 0 {
		t.Fatalf("наведение на ячейку не сработало: HoverIndex=%d", area.HoverIndex())
	}

	menu.Toggle(image.Rect(0, 560, 40, 600))
	if !menu.IsOpen() {
		t.Fatal("меню не открылось")
	}
	overlay := menu.Bounds()

	// Точка внутри меню, под которой лежит ячейка: ровно тот случай, когда
	// «курсор над меню, а подсвечивается кнопка под ним».
	pt := image.Pt(first.Min.X+first.Dx()/2, first.Min.Y+first.Dy()/2)
	if !pt.In(overlay) {
		t.Fatalf("ячейка %v не под меню %v — тест ничего не проверяет", first, overlay)
	}
	eng.SendMouseMove(pt.X, pt.Y)

	if got := area.HoverIndex(); got != -1 {
		t.Errorf("под открытым меню подсвечена ячейка %d — сквозь стекло это видно", got)
	}
}

// Без оверлея доставка прежняя: наведение работает как работало.
func TestHover_WithoutOverlayNothingChanged(t *testing.T) {
	eng, area, _ := hoverScene(t)

	for i := 0; i < 2; i++ {
		moveTo(eng, cellRect(t, area, i))
		if got := area.HoverIndex(); got != i {
			t.Errorf("наведение на ячейку %d дало HoverIndex=%d", i, got)
		}
	}
	// Уводим курсор совсем в сторону.
	eng.SendMouseMove(700, 100)
	if got := area.HoverIndex(); got != -1 {
		t.Errorf("после увода курсора подсвечена ячейка %d", got)
	}
}

// Сам оверлей движение получает: иначе исчезла бы подсветка внутри меню.
func TestHover_OverlayItselfStillGetsTheMove(t *testing.T) {
	eng, _, menu := hoverScene(t)

	menu.Toggle(image.Rect(0, 560, 40, 600))
	overlay := menu.Bounds()
	if overlay.Empty() {
		t.Fatal("у открытого меню пустой прямоугольник")
	}

	pt := image.Pt(overlay.Min.X+overlay.Dx()/2, overlay.Min.Y+overlay.Dy()/2)
	eng.SendMouseMove(pt.X, pt.Y)

	if !menu.IsOpen() {
		t.Error("движение внутри меню его закрыло")
	}
}
