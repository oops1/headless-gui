package widget

// docksplit_test.go — запрос GG-10: несколько панелей на одной стороне должны
// уметь стоять в столбик, а не только вкладками.
//
// «Репозитории» над «Ветками» на макете заказчика собрать было нечем: две
// панели одной стороны показывались корешками, и разделить их по длине
// стороны не позволял ни один вызов.

import (
	"image"
	"testing"
)

// splitMgr — менеджер с двумя панелями на левой стороне и заданными границами.
func splitMgr(t *testing.T) (*DockManager, *DockPane, *DockPane) {
	t.Helper()
	m, _ := newDockSizeMgr()
	a := newDockSizePane("a", "Репозитории")
	b := newDockSizePane("b", "Ветки")
	m.AddPane(a, DockLeft)
	m.AddPane(b, DockLeft)
	m.SetSideSize(DockLeft, 260)
	m.SetBounds(image.Rect(0, 0, 900, 600))
	return m, a, b
}

// По умолчанию — вкладки: прежнее поведение не меняется.
func TestDockSplit_TabsByDefault(t *testing.T) {
	m, a, b := splitMgr(t)

	if got := m.SideStack(DockLeft); got != DockStackTabs {
		t.Errorf("режим стороны %v, ждали вкладки", got)
	}
	// У вкладок видна одна панель: у второй границы пусты либо совпадают с
	// первой — но обе они точно не делят сторону пополам.
	if !a.Bounds().Empty() && !b.Bounds().Empty() && !a.Bounds().Overlaps(b.Bounds()) {
		t.Errorf("панели поделили сторону без просьбы: %v и %v", a.Bounds(), b.Bounds())
	}
}

// Столбик: панели делят сторону по длинной оси и не налезают друг на друга.
func TestDockSplit_PanesShareTheSide(t *testing.T) {
	m, a, b := splitMgr(t)
	m.SetSideStack(DockLeft, DockStackSplit)

	ra, rb := a.Bounds(), b.Bounds()
	if ra.Empty() || rb.Empty() {
		t.Fatalf("в столбике панель без границ: %v и %v", ra, rb)
	}
	if ra.Overlaps(rb) {
		t.Errorf("панели столбика налезли друг на друга: %v и %v", ra, rb)
	}
	// Левая сторона делится по ВЫСОТЕ: иначе столбик вышел бы поперёк и
	// панели сплющило бы в вертикальные полоски.
	if ra.Dx() != rb.Dx() {
		t.Errorf("ширины панелей столбика разошлись: %d и %d", ra.Dx(), rb.Dx())
	}
	if ra.Min.Y >= rb.Min.Y {
		t.Errorf("порядок панелей нарушен: первая %v ниже второй %v", ra, rb)
	}
	// Между ними — кромка, за которую тянут.
	if got := ra.Max.Y; rb.Min.Y <= got {
		t.Errorf("между панелями нет кромки: %v и %v", ra, rb)
	}
}

// Нижняя сторона делится по ШИРИНЕ: ось столбика зависит от стороны.
func TestDockSplit_BottomSideSplitsByWidth(t *testing.T) {
	m, _ := newDockSizeMgr()
	a := newDockSizePane("a", "Журнал")
	b := newDockSizePane("b", "Вывод")
	m.AddPane(a, DockBottom)
	m.AddPane(b, DockBottom)
	m.SetBounds(image.Rect(0, 0, 900, 600))
	m.SetSideStack(DockBottom, DockStackSplit)

	ra, rb := a.Bounds(), b.Bounds()
	if ra.Empty() || rb.Empty() {
		t.Fatalf("в столбике панель без границ: %v и %v", ra, rb)
	}
	if ra.Dy() != rb.Dy() {
		t.Errorf("высоты панелей нижнего столбика разошлись: %d и %d", ra.Dy(), rb.Dy())
	}
	if ra.Min.X >= rb.Min.X {
		t.Errorf("порядок панелей нарушен: %v и %v", ra, rb)
	}
}

// SplitSide задаёт долю первой панели.
func TestDockSplit_SplitSideSetsTheRatio(t *testing.T) {
	m, a, b := splitMgr(t)
	m.SplitSide(DockLeft, 0.75)

	ra, rb := a.Bounds(), b.Bounds()
	if ra.Dy() <= rb.Dy() {
		t.Errorf("доля 0.75 не сделала первую панель больше: %d против %d", ra.Dy(), rb.Dy())
	}
	// Примерно три четверти: точного равенства нет — между панелями кромка.
	total := ra.Dy() + rb.Dy()
	if got := float64(ra.Dy()) / float64(total); got < 0.6 || got > 0.9 {
		t.Errorf("доля первой панели %.2f, просили около 0.75", got)
	}
}

// Панель нельзя схлопнуть в ноль: иначе её потом не найти.
func TestDockSplit_RatioIsClamped(t *testing.T) {
	m, a, b := splitMgr(t)
	for _, ratio := range []float64{0, -1, 1, 5} {
		m.SplitSide(DockLeft, ratio)
		if a.Bounds().Dy() <= 0 || b.Bounds().Dy() <= 0 {
			t.Errorf("доля %v схлопнула панель: %v и %v", ratio, a.Bounds(), b.Bounds())
		}
	}
}

// Кромку между панелями можно поймать мышью и перетащить.
func TestDockSplit_GutterDragsTheRatio(t *testing.T) {
	m, a, b := splitMgr(t)
	m.SplitSide(DockLeft, 0.5)

	before := a.Bounds().Dy()
	gut := m.splitGutters[int(DockLeft)]
	if len(gut) != 1 {
		t.Fatalf("кромок между двумя панелями %d, ждали одну", len(gut))
	}
	g := gut[0]

	// Кромка обязана ловиться хит-тестом — иначе за неё не потянуть.
	s, i, ok := m.SplitGutterAt(g.Min.X+g.Dx()/2, g.Min.Y+g.Dy()/2)
	if !ok || s != DockLeft || i != 0 {
		t.Fatalf("кромка не найдена под собственным центром: %v %d %v", s, i, ok)
	}

	// Тянем вниз — первая панель растёт, вторая уменьшается.
	region := m.regions[int(DockLeft)]
	m.DragSplitGutter(DockLeft, 0, g.Min.X, region.Min.Y+region.Dy()*3/4)

	if a.Bounds().Dy() <= before {
		t.Errorf("после перетаскивания вниз первая панель %d, была %d", a.Bounds().Dy(), before)
	}
	if b.Bounds().Dy() <= 0 {
		t.Errorf("вторая панель схлопнулась: %v", b.Bounds())
	}
}

// Раскладка переживает сохранение и восстановление — вместе с режимом и долями.
func TestDockSplit_SurvivesSaveRestore(t *testing.T) {
	m, a, _ := splitMgr(t)
	m.SplitSide(DockLeft, 0.7)
	wantH := a.Bounds().Dy()
	data := m.SaveLayout()

	// Другой менеджер с теми же панелями — как перезапуск приложения.
	m2, _ := newDockSizeMgr()
	a2 := newDockSizePane("a", "Репозитории")
	b2 := newDockSizePane("b", "Ветки")
	m2.AddPane(a2, DockLeft)
	m2.AddPane(b2, DockLeft)
	m2.SetBounds(image.Rect(0, 0, 900, 600))

	if err := m2.RestoreLayout(data); err != nil {
		t.Fatalf("восстановление: %v", err)
	}
	m2.SetBounds(image.Rect(0, 0, 900, 600))

	if got := m2.SideStack(DockLeft); got != DockStackSplit {
		t.Errorf("после восстановления сторона в режиме %v, ждали столбик", got)
	}
	if got := a2.Bounds().Dy(); got != wantH {
		t.Errorf("высота первой панели после восстановления %d, была %d", got, wantH)
	}
}

// Раскладка, сохранённая ПРЕЖНЕЙ версией (без полей режима), читается и
// означает вкладки: обновление движка не должно ломать сохранённые раскладки.
func TestDockSplit_OldLayoutMeansTabs(t *testing.T) {
	m, _, _ := splitMgr(t)
	m.SetSideStack(DockLeft, DockStackSplit)

	old := []byte(`{"sizes":[260,0,0,0],"panes":[` +
		`{"id":"a","state":0,"side":0,"active":true},` +
		`{"id":"b","state":0,"side":0}]}`)
	if err := m.RestoreLayout(old); err != nil {
		t.Fatalf("восстановление старой раскладки: %v", err)
	}
	if got := m.SideStack(DockLeft); got != DockStackTabs {
		t.Errorf("старая раскладка дала режим %v, ждали вкладки", got)
	}
}

// Одна панель на стороне занимает её целиком — кромке взяться неоткуда.
func TestDockSplit_SinglePaneTakesTheWholeSide(t *testing.T) {
	m, _ := newDockSizeMgr()
	a := newDockSizePane("a", "Репозитории")
	m.AddPane(a, DockLeft)
	m.SetSideSize(DockLeft, 260)
	m.SetBounds(image.Rect(0, 0, 900, 600))
	m.SetSideStack(DockLeft, DockStackSplit)

	if got := len(m.splitGutters[int(DockLeft)]); got != 0 {
		t.Errorf("у одной панели %d кромок", got)
	}
	region := m.regions[int(DockLeft)]
	if got := a.Bounds(); got != region {
		t.Errorf("одна панель заняла %v вместо всей стороны %v", got, region)
	}
}
