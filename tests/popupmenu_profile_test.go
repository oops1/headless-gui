package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Контекстное меню по профилю темы — то, что видно на экране: скругление,
// плашка наведения, смешивание полупрозрачных цветов, поля, шеврон, тень.
// Замечания WinLine после подключения v3.31.0, блок «Контекстное меню».

var (
	pmCanvas = color.RGBA{R: 200, G: 200, B: 200, A: 255}
	pmWhite  = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	pmBlack  = color.RGBA{A: 255}
)

// pmRender показывает меню в точке (20,20) на сером холсте и возвращает кадр.
// hover — индекс пункта, на который наведён курсор (-1 — ни на какой).
func pmRender(t *testing.T, m *widget.PopupMenu, hover int) *image.RGBA {
	t.Helper()
	eng := engine.New(400, 300, 30)
	root := widget.NewPanel(pmCanvas)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(m)
	m.SetBounds(image.Rect(0, 0, 1, 1))
	eng.SetRoot(root)
	m.Show(20, 20)
	if hover >= 0 {
		// Строки подряд от верхней рамки: берём середину нужной.
		y := 20 + padY(m) + hover*m.ItemHeight + m.ItemHeight/2
		m.OnMouseMove(30, y)
	}
	return eng.RenderOnce()
}

func padY(m *widget.PopupMenu) int {
	if m.PaddingY > 0 {
		return m.PaddingY
	}
	return 2
}

// pmMenu — меню с заданными цветами, чтобы тест не зависел от темы.
func pmMenu(items ...widget.MenuItem) *widget.PopupMenu {
	m := widget.NewPopupMenu()
	m.SetItems(items)
	m.Background, m.BorderColor, m.SeparatorColor = pmWhite, pmWhite, pmBlack
	m.TextColor, m.HoverTextColor, m.DisabledColor = pmBlack, pmBlack, color.RGBA{R: 150, G: 150, B: 150, A: 255}
	m.ShadowColor = color.RGBA{}
	return m
}

func lum(c color.RGBA) int { return (int(c.R) + int(c.G) + int(c.B)) / 3 }

// Пункт 3: полупрозрачная плашка наведения ложится плёнкой. Раньше она
// писалась без смешивания, и чёрная плёнка (0,0,0,18) выходила почти
// чёрной — текст на ней не читался.
func TestPopupMenu_TranslucentHoverBlends(t *testing.T) {
	m := pmMenu(widget.MenuItem{Text: "Копировать"}, widget.MenuItem{Text: "Вставить"})
	m.HoverBG = color.RGBA{A: 18} // чёрная плёнка, предумноженная запись
	img := pmRender(t, m, 0)
	// Плашка начинается у левого края меню (отступ 2); берём пустое место.
	got := img.RGBAAt(24, 22+15)
	if l := lum(got); l < 225 || l > 245 {
		t.Errorf("плашка из (0,0,0,18) поверх белого: яркость %d (%v), ждали около 237", l, got)
	}
	// Без наведения — чисто белый.
	if l := lum(img.RGBAAt(24, 22+30+15)); l != 255 {
		t.Errorf("пункт без наведения не белый: %d", l)
	}
}

// Пункты 1 и 2: скругление меню и плашки, отступ плашки от краёв.
func TestPopupMenu_RoundedMenuAndInsetPlate(t *testing.T) {
	m := pmMenu(widget.MenuItem{Text: "Копировать"}, widget.MenuItem{Text: "Вставить"})
	m.CornerRadius, m.ItemCorner, m.ItemInset, m.PaddingY = 8, 4, 4, 4
	m.HoverBG = color.RGBA{B: 255, A: 255}
	img := pmRender(t, m, 0)

	// Угол меню скруглён: крайняя точка холста осталась серой, а не белой.
	if l := lum(img.RGBAAt(20, 20)); l > 215 {
		t.Errorf("угол меню не скруглён: яркость %d", l)
	}
	if l := lum(img.RGBAAt(40, 20)); l < 250 {
		t.Errorf("верх меню вдали от угла не белый: %d", l)
	}
	// Плашка: первая строка начинается на 4 точки ниже рамки и на 4 правее.
	top := 20 + 4
	// Слева от плашки (отступ 4) — белое поле меню.
	if c := img.RGBAAt(20+3, top+12); c.R == 0 && c.B == 255 {
		t.Errorf("плашка вылезла в поле: %v", c)
	}
	if c := img.RGBAAt(20+4+10, top+12); !(c.B == 255 && c.R == 0) {
		t.Errorf("середина плашки не синяя: %v", c)
	}
	// Скруглённый угол плашки: самая крайняя точка плашки не залита.
	if c := img.RGBAAt(20+4, top); c.B == 255 && c.R == 0 {
		t.Errorf("угол плашки не скруглён: %v", c)
	}
	// Отступ сверху: выше плашки — поле меню (четыре точки).
	if c := img.RGBAAt(20+20, top-2); c.R == 0 && c.B == 255 {
		t.Errorf("плашка заехала в верхнее поле: %v", c)
	}
}

// Пункт 5: сочетание берёт цвет из стиля, а не смешивается с фоном.
func TestPopupMenu_ShortcutColourFromStyle(t *testing.T) {
	item := widget.MenuItem{Text: "Отменить", Shortcut: "CTRL+Z"}
	plain := pmMenu(item)
	styled := pmMenu(item)
	styled.ShortcutColor = color.RGBA{R: 255, A: 255} // красный — легко узнать
	a := pmRender(t, plain, -1)
	b := pmRender(t, styled, -1)

	reds := func(img *image.RGBA) (n int) {
		for y := 20; y < 60; y++ {
			for x := 20; x < 230; x++ {
				c := img.RGBAAt(x, y)
				if int(c.R)-int(c.G) > 60 && int(c.R)-int(c.B) > 60 {
					n++
				}
			}
		}
		return
	}
	if n := reds(a); n != 0 {
		t.Errorf("без цвета профиля в меню красного быть не должно: %d точек", n)
	}
	if n := reds(b); n < 15 {
		best := color.RGBA{R: 255, G: 255, B: 255, A: 255}
		for y := 25; y < 50; y++ {
			for x := 120; x < 200; x++ {
				if c := b.RGBAAt(x, y); lum(c) < lum(best) {
					best = c
				}
			}
		}
		t.Errorf("сочетание не окрашено цветом профиля: красных точек %d, самая тёмная точка справа %v", n, best)
	}
}

// Пункт 4: тонкий шеврон у правого края вместо глифа «▸» у поля.
func TestPopupMenu_ThinChevron(t *testing.T) {
	sub := []widget.MenuItem{{Text: "Папку"}}
	m := pmMenu(widget.MenuItem{Text: "Создать", SubItems: sub})
	m.ChevronRight = 18
	img := pmRender(t, m, -1)

	r := m.Bounds()
	cx := r.Max.X - 18
	dark := func(x0, x1 int) (n int) {
		for y := 22; y < 22+m.ItemHeight; y++ {
			for x := x0; x < x1; x++ {
				if lum(img.RGBAAt(x, y)) < 160 {
					n++
				}
			}
		}
		return
	}
	if n := dark(cx-5, cx+5); n < 6 {
		t.Errorf("шеврона у правого края нет: тёмных точек %d", n)
	}
	// Правее — пусто: глифа у поля (PaddingX от края) больше нет.
	if n := dark(r.Max.X-11, r.Max.X-2); n != 0 {
		t.Errorf("правее шеврона остались тёмные точки: %d", n)
	}
}

// Пункт 6: поля слева и справа и отступ разделителя — раздельно.
func TestPopupMenu_SeparatePadsAndSeparatorInset(t *testing.T) {
	m := pmMenu(
		widget.MenuItem{Text: "Копировать", Shortcut: "Ctrl+C"},
		widget.MenuItem{Separator: true},
		widget.MenuItem{Text: "Вставить"},
	)
	m.PadLeft, m.PadRight, m.SeparatorInset = 36, 20, 24
	img := pmRender(t, m, -1)
	r := m.Bounds()

	firstInk := func(x0, x1, y0, y1 int) int {
		for x := x0; x < x1; x++ {
			for y := y0; y < y1; y++ {
				if lum(img.RGBAAt(x, y)) < 140 {
					return x
				}
			}
		}
		return -1
	}
	if x := firstInk(21, r.Max.X, 22, 22+30); x < 20+36 || x > 20+36+4 {
		t.Errorf("подпись начинается в %d, ждали около %d (левое поле 36)", x, 20+36)
	}
	lastInk := -1
	for x := r.Max.X - 2; x > r.Min.X+60; x-- {
		for y := 22; y < 22+30; y++ {
			if lum(img.RGBAAt(x, y)) < 140 {
				lastInk = x
				break
			}
		}
		if lastInk >= 0 {
			break
		}
	}
	if lastInk < 0 || lastInk > r.Max.X-20 || lastInk < r.Max.X-24 {
		t.Errorf("сочетание кончается в %d, ждали около %d (правое поле 20)", lastInk, r.Max.X-20)
	}
	sepY := 22 + 30 + m.SeparatorH/2
	if lum(img.RGBAAt(20+10, sepY)) != 255 {
		t.Error("разделитель дотянулся до края: отступ 24 не соблюдён")
	}
	if lum(img.RGBAAt(20+30, sepY)) != 0 {
		t.Errorf("линия разделителя не нарисована: %v", img.RGBAAt(20+30, sepY))
	}
}

// Пункт 7: мягкая тень по Elevation вместо прямоугольника со смещением 2.
func TestPopupMenu_SoftShadowByElevation(t *testing.T) {
	probe := func(elev float64) int {
		m := pmMenu(widget.MenuItem{Text: "Копировать"})
		m.ShadowColor = color.RGBA{A: 140}
		m.Elevation = elev
		img := pmRender(t, m, -1)
		r := m.Bounds()
		return lum(img.RGBAAt(r.Max.X+6, r.Min.Y+r.Dy()/2))
	}
	if l := probe(0); l != 200 {
		t.Errorf("прежняя тень со смещением 2 не должна доставать на 6 точек: %d", l)
	}
	if l := probe(12); l >= 198 {
		t.Errorf("мягкая тень не легла за краем меню: яркость %d", l)
	}
	// И она мягкая: дальше от меню — светлее.
	m := pmMenu(widget.MenuItem{Text: "Копировать"})
	m.ShadowColor, m.Elevation = color.RGBA{A: 140}, 12
	img := pmRender(t, m, -1)
	r := m.Bounds()
	y := r.Min.Y + r.Dy()/2
	if near, far := lum(img.RGBAAt(r.Max.X+3, y)), lum(img.RGBAAt(r.Max.X+13, y)); near >= far {
		t.Errorf("тень не затухает от меню: у края %d, дальше %d", near, far)
	}
}

// Пункт 9: одноцветный значок берёт цвет текста в каждом состоянии, значок
// для плашки наведения подменяется.
func TestPopupMenu_IconFollowsItemState(t *testing.T) {
	icon := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			icon.SetRGBA(x, y, pmBlack) // сплошной чёрный квадрат
		}
	}
	m := pmMenu(widget.MenuItem{Text: "Копировать", Icon: icon, IconTint: true})
	m.HoverBG = color.RGBA{B: 128, A: 255}
	m.HoverTextColor = pmWhite
	m.PadLeft = 10
	img := pmRender(t, m, 0)
	// Значок: левое поле 10, высота 30 (значок 16 по центру), центр квадрата.
	c := img.RGBAAt(20+10+8, 22+15)
	if c.R < 240 || c.G < 240 {
		t.Errorf("значок на синей плашке остался тёмным: %v", c)
	}
	// Без наведения — цвет текста (чёрный), тот же рисунок.
	img = pmRender(t, pmMenuWithIcon(icon), -1)
	if c := img.RGBAAt(20+10+8, 22+15); lum(c) > 40 {
		t.Errorf("значок без наведения не цвета текста: %v", c)
	}
}

func pmMenuWithIcon(icon image.Image) *widget.PopupMenu {
	m := pmMenu(widget.MenuItem{Text: "Копировать", Icon: icon, IconTint: true})
	m.PadLeft = 10
	return m
}
