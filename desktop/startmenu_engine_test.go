package desktop_test

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/widget"
)

// Меню «Пуск» Windows 10 через настоящий путь событий движка: клики и клавиши
// идут тем же путём, что от окна, — с фокусом по просьбе, захватом мыши при
// перетаскивании и закрытием по клику мимо.

func (s *win10Scene) mouse(x, y int, down, up bool) {
	s.eng.SendMouseMove(x, y)
	if down {
		s.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	}
	if up {
		s.eng.SendMouseButton(x, y, widget.MouseLeft, false)
	}
}

func (s *win10Scene) clickCentre(r image.Rectangle) {
	x, y := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	s.mouse(x, y, true, true)
}

func (s *win10Scene) key(code widget.KeyCode, r rune, mod widget.KeyMod) {
	s.eng.SendKeyEvent(widget.KeyEvent{Code: code, Rune: r, Mod: mod, Pressed: true})
	s.eng.SendKeyEvent(widget.KeyEvent{Code: code, Rune: r, Mod: mod})
}

// firstTile — центр первой плитки меню по стартовым метрикам: рамка 1 + боковая
// панель 48 + список 260 + поле 21 по X; рамка 1 + поле 18 + заголовок 32 +
// зазор 5 по Y; плитка 100×100.
func (s *win10Scene) tileCentre(col, row int) image.Point {
	pm := s.menu.OverlayBounds()
	return image.Pt(pm.Min.X+1+48+260+21+50+col*104, pm.Min.Y+1+18+32+5+50+row*104)
}

func TestStartMenu10_EngineStartButtonOpensAndKeyboardLaunches(t *testing.T) {
	s := newWin10Scene(t, 1280, 720, false, 1)
	s.clickCentre(s.start.Bounds())
	s.menu.Settle()
	if !s.menu.IsOpen() {
		t.Fatal("нажатие на «Пуск» не открыло меню")
	}
	// Меню попросило фокус при открытии: клавиши идут ему без SetFocus.
	s.key(widget.KeyDown, 0, 0)
	s.key(widget.KeyEnter, 0, 0)
	if len(s.cat.Launched) != 1 || s.cat.Launched[0] != "music" {
		t.Fatalf("Enter запустил %v, ждали первое приложение «music» (Яндекс.Музыка)", s.cat.Launched)
	}
	if s.menu.IsOpen() {
		t.Error("меню осталось открытым после запуска")
	}

	// Повторное открытие: Tab уходит в плитки (а не в обход фокуса движка),
	// Enter запускает первую.
	s.clickCentre(s.start.Bounds())
	s.menu.Settle()
	s.key(widget.KeyTab, 9, 0)
	s.key(widget.KeyEnter, 0, 0)
	if n := len(s.cat.Launched); n != 2 || s.cat.Launched[1] != "m365" {
		t.Errorf("запущено %v, ждали вторым приложение первой плитки m365", s.cat.Launched)
	}
}

func TestStartMenu10_EngineEscapeAndClickAwayClose(t *testing.T) {
	s := newWin10Scene(t, 1280, 720, false, 1)
	s.clickCentre(s.start.Bounds())
	s.menu.Settle()
	s.key(widget.KeyEscape, 27, 0)
	if s.menu.IsOpen() {
		t.Error("Esc не закрыл меню")
	}
	s.clickCentre(s.start.Bounds())
	s.menu.Settle()
	s.mouse(1100, 100, true, true)
	if s.menu.IsOpen() {
		t.Error("клик мимо не закрыл меню")
	}
	// Повторный клик по «Пуску» при открытом меню закрывает его и не открывает
	// заново.
	s.clickCentre(s.start.Bounds())
	s.menu.Settle()
	s.clickCentre(s.start.Bounds())
	if s.menu.IsOpen() {
		t.Error("повторный клик по «Пуску» не закрыл меню")
	}
}

func TestStartMenu10_EngineTileClickAndDrag(t *testing.T) {
	s := newWin10Scene(t, 1280, 720, false, 1)
	s.open()
	var order []desktop.TileGroup
	s.menu.OnTilesChanged = func(g []desktop.TileGroup) { order = g }

	// Перетаскивание через захват мыши: нажатие на первой плитке, шаги мыши,
	// отпускание правее второй.
	a, b := s.tileCentre(0, 0), s.tileCentre(1, 0)
	s.eng.SendMouseMove(a.X, a.Y)
	s.eng.SendMouseButton(a.X, a.Y, widget.MouseLeft, true)
	s.eng.SendMouseMove(a.X+30, a.Y)
	s.eng.SendMouseMove(b.X+30, b.Y)
	s.eng.SendMouseButton(b.X+30, b.Y, widget.MouseLeft, false)
	if order == nil {
		t.Fatal("OnTilesChanged не вызван после перетаскивания")
	}
	if order[0].Tiles[0].ID == "m365" {
		t.Errorf("порядок не изменился: %v", order[0].Tiles[0].ID)
	}
	if !s.menu.IsOpen() {
		t.Error("перетаскивание закрыло меню")
	}
	if len(s.cat.Launched) != 0 {
		t.Errorf("перетаскивание запустило приложение: %v", s.cat.Launched)
	}

	// Простой щелчок по плитке запускает приложение.
	c := s.tileCentre(2, 0)
	s.mouse(c.X, c.Y, true, true)
	if len(s.cat.Launched) != 1 {
		t.Errorf("щелчок по плитке запустил %v", s.cat.Launched)
	}
}

func TestStartMenu10_EngineWheelScrollsAndTypingSearches(t *testing.T) {
	s := newWin10Scene(t, 1280, 720, false, 1)
	manyApps(s, 100)
	s.clickCentre(s.start.Bounds())
	s.menu.Settle()

	pm := s.menu.OverlayBounds()
	listPt := image.Pt(pm.Min.X+1+48+100, pm.Min.Y+200)
	s.eng.SendMouseMove(listPt.X, listPt.Y)
	img0 := s.frame()
	row := image.Rect(listPt.X, listPt.Y-5, listPt.X+60, listPt.Y+5)
	before := sampleHash(img0, row)
	s.eng.SendMouseWheelPixels(listPt.X, listPt.Y, 0, 200)
	img1 := s.frame()
	if sampleHash(img1, row) == before {
		t.Error("колесо над списком не прокрутило его")
	}

	// Набор буквы на меню: ввод уходит в строку поиска на панели.
	s.key(widget.KeyE, 'e', 0)
	if s.search.Text() != "e" {
		t.Errorf("текст строки %q после набора на меню, ждали «e»", s.search.Text())
	}
	s.key(widget.KeyD, 'd', 0)
	if s.search.Text() != "ed" {
		t.Errorf("текст строки %q, ждали «ed» — фокус не перешёл к строке", s.search.Text())
	}
	s.key(widget.KeyEnter, 13, 0)
	if len(s.prov.Activated) != 1 || s.prov.Activated[0] != "edge" {
		t.Errorf("Enter в строке открыл %v, ждали edge", s.prov.Activated)
	}
}

func sampleHash(img *image.RGBA, r image.Rectangle) uint64 {
	var h uint64 = 1469598103934665603
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			h = (h ^ uint64(c.R)) * 1099511628211
			h = (h ^ uint64(c.G)) * 1099511628211
			h = (h ^ uint64(c.B)) * 1099511628211
		}
	}
	return h
}
