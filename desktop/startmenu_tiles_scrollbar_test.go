package desktop

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Перетаскивание бегунка тонкой полосы прокрутки и щелчок по дорожке.

// manyTileGroups — восемь групп по шесть средних плиток: плитки не умещаются в
// окно и у них есть собственная прокрутка.
func manyTileGroups() []TileGroup {
	var out []TileGroup
	for g := 0; g < 8; g++ {
		grp := TileGroup{ID: fmt.Sprintf("g%d", g), Title: fmt.Sprintf("Группа %d", g)}
		for i := 0; i < 6; i++ {
			id := fmt.Sprintf("g%dt%d", g, i)
			grp.Tiles = append(grp.Tiles, Tile{ID: TileID(id), App: AppID(id), Size: TileMedium, Content: TileContent{Title: id}})
		}
		out = append(out, grp)
	}
	return out
}

func scrollMenu(t *testing.T) (*StartMenu, *StaticAppCatalog) {
	t.Helper()
	m, cat := tiledMenu(t)
	m.SetTileGroups(manyTileGroups())
	return m, cat
}

func barOfMenu(t *testing.T, m *StartMenu, a startArea) barGeom {
	t.Helper()
	b, ok := m.barOf(m.startGeometry(m.contentRect()), a)
	if !ok {
		t.Fatalf("у области %v нет полосы прокрутки", a)
	}
	return b
}

func pressMenu(m *StartMenu, x, y int) {
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
}
func release(m *StartMenu, x, y int) {
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
}

// Бегунок списка: нажатие захватывает мышь, смещение бегунка пропорционально
// прокрутке, верх бегунка остаётся там, где его схватили.
func TestScrollbar_DragThumbScrollsList(t *testing.T) {
	m, cat := tiledMenu(t)
	b := barOfMenu(t, m, areaList)
	span, room := b.content-b.viewH, b.viewH-b.thumb.Dy()

	x, y := b.thumb.Min.X+2, b.thumb.Min.Y+10 // схвачен на 10 px ниже верха
	m.OnMouseMove(x, y)
	if !m.WantsCapture(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true}) {
		t.Error("нажатие на бегунок не запросило захват мыши")
	}
	pressMenu(m, x, y)
	if !m.v.bar.active || m.v.bar.area != areaList {
		t.Fatalf("перетаскивание не началось: %+v", m.v.bar)
	}
	if m.v.listScroll != 0 {
		t.Errorf("нажатие на бегунок сдвинуло список: %d", m.v.listScroll)
	}

	m.OnMouseMove(x, y+100)
	want := 100 * span / room
	if got := m.v.listScroll; got < want-1 || got > want+1 {
		t.Errorf("после сдвига мыши на 100 px прокрутка %d, ждали ~%d", got, want)
	}
	nb := barOfMenu(t, m, areaList)
	if d := nb.thumb.Min.Y - b.thumb.Min.Y; d < 99 || d > 101 {
		t.Errorf("бегунок сдвинулся на %d px при сдвиге мыши на 100: он ушёл от курсора", d)
	}

	// Мышь ушла далеко за нижний и верхний край (и за пределы меню): прокрутка
	// не выходит из диапазона.
	m.OnMouseMove(x+400, y+5000)
	if m.v.listScroll != span {
		t.Errorf("у нижнего предела прокрутка %d, ждали %d", m.v.listScroll, span)
	}
	m.OnMouseMove(x, y-5000)
	if m.v.listScroll != 0 {
		t.Errorf("у верхнего предела прокрутка %d, ждали 0", m.v.listScroll)
	}
	if m.v.bar.active == false {
		t.Error("перетаскивание оборвалось, пока кнопка зажата")
	}

	// Отпускание за пределами меню заканчивает перетаскивание.
	m.OnMouseMove(x, y+150)
	release(m, x+900, y+150)
	if m.v.bar.active {
		t.Error("отпускание за пределами меню не закончило перетаскивание")
	}
	if len(cat.Launched) != 0 {
		t.Errorf("перетаскивание бегунка запустило приложение: %v", cat.Launched)
	}
	// После отпускания мышь больше не водит список.
	before := m.v.listScroll
	m.OnMouseMove(x, y+300)
	if m.v.listScroll != before {
		t.Error("после отпускания движение мыши прокручивает список")
	}
}

// Пока бегунок тянут, полоса не гаснет; после отпускания гаснет через обычную
// паузу.
func TestScrollbar_ThumbDoesNotFadeWhileDragged(t *testing.T) {
	old1, old2 := thinBarHold, thinBarFade
	thinBarHold, thinBarFade = 30*time.Millisecond, 20*time.Millisecond
	defer func() { thinBarHold, thinBarFade = old1, old2 }()

	m, _ := tiledMenu(t)
	b := barOfMenu(t, m, areaList)
	x, y := b.thumb.Min.X+2, b.thumb.Min.Y+5
	m.OnMouseMove(x, y)
	pressMenu(m, x, y)
	m.OnMouseMove(x, y+20)

	time.Sleep(90 * time.Millisecond) // втрое дольше паузы
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(time.Second))
	if a := m.v.listBar.Alpha(); a != 1 {
		t.Fatalf("бегунок потух во время перетаскивания: alpha %v", a)
	}
	// Он нарисован широким (ширина под курсором).
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	wide := false
	for _, f := range ctx.fills {
		if f.w == m.metricInt(KeyScrollThinHoverWidth) && f.h > 20 {
			wide = true
		}
	}
	if !wide {
		t.Error("бегунок при перетаскивании не расширен")
	}

	release(m, x, y+20)
	m.OnMouseMove(m.contentRect().Min.X+10, m.contentRect().Min.Y+10) // курсор ушёл с дорожки
	time.Sleep(90 * time.Millisecond)
	t1 := time.Now()
	widget.StepAnimations(t1)
	widget.StepAnimations(t1.Add(time.Second))
	if a := m.v.listBar.Alpha(); a != 0 {
		t.Errorf("после отпускания полоса не погасла: alpha %v", a)
	}
}

// Курсор над дорожкой тоже удерживает полосу на виду.
func TestScrollbar_HoverKeepsBarVisible(t *testing.T) {
	old1, old2 := thinBarHold, thinBarFade
	thinBarHold, thinBarFade = 30*time.Millisecond, 20*time.Millisecond
	defer func() { thinBarHold, thinBarFade = old1, old2 }()

	m, _ := tiledMenu(t)
	b := barOfMenu(t, m, areaList)
	m.OnMouseMove(b.thumb.Min.X+2, b.thumb.Min.Y+5)
	time.Sleep(90 * time.Millisecond)
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(time.Second))
	if a := m.v.listBar.Alpha(); a != 1 {
		t.Errorf("над дорожкой полоса потухла: alpha %v", a)
	}
	m.OnMouseMove(m.contentRect().Min.X+100, m.contentRect().Min.Y+100)
	time.Sleep(90 * time.Millisecond)
	t1 := time.Now()
	widget.StepAnimations(t1)
	widget.StepAnimations(t1.Add(time.Second))
	if a := m.v.listBar.Alpha(); a != 0 {
		t.Errorf("курсор ушёл, а полоса не гаснет: alpha %v", a)
	}
}

// Щелчок по дорожке вне бегунка сдвигает список на страницу в его сторону.
func TestScrollbar_TrackClickPagesList(t *testing.T) {
	m, cat := tiledMenu(t)
	b := barOfMenu(t, m, areaList)
	page := b.viewH - m.metricInt(KeyStartMenuRowHeight)

	x, y := b.track.Min.X+3, b.track.Max.Y-10 // ниже бегунка
	m.OnMouseMove(x, y)
	pressMenu(m, x, y)
	release(m, x, y)
	if m.v.listScroll != page {
		t.Errorf("щелчок под бегунком: прокрутка %d, ждали страницу %d", m.v.listScroll, page)
	}
	if m.v.bar.active {
		t.Error("щелчок по дорожке начал перетаскивание")
	}
	pressMenu(m, x, y)
	release(m, x, y)
	if m.v.listScroll != 2*page {
		t.Errorf("второй щелчок: %d, ждали %d", m.v.listScroll, 2*page)
	}

	nb := barOfMenu(t, m, areaList)
	x, y = nb.track.Min.X+3, nb.track.Min.Y+2 // выше бегунка
	m.OnMouseMove(x, y)
	pressMenu(m, x, y)
	release(m, x, y)
	if m.v.listScroll != page {
		t.Errorf("щелчок над бегунком: прокрутка %d, ждали %d", m.v.listScroll, page)
	}
	if len(cat.Launched) != 0 {
		t.Errorf("щелчок по дорожке запустил приложение: %v", cat.Launched)
	}
	// Дальше конца не уходит.
	for i := 0; i < 50; i++ {
		b = barOfMenu(t, m, areaList)
		pressMenu(m, b.track.Min.X+3, b.track.Max.Y-2)
		release(m, b.track.Min.X+3, b.track.Max.Y-2)
	}
	if want := b.content - b.viewH; m.v.listScroll != want {
		t.Errorf("предел прокрутки %d, ждали %d", m.v.listScroll, want)
	}
}

// Дорожка не отнимает у строки стрелку раскрытия папки: узкая колонка у самого
// края, шире отступа строки она не бывает.
func TestScrollbar_TrackNarrowerThanRowPad(t *testing.T) {
	m, _ := tiledMenu(t)
	b := barOfMenu(t, m, areaList)
	if w := b.track.Dx(); w > m.metricInt(KeyStartRowPad)/2 {
		t.Errorf("дорожка %d px шире половины отступа строки (%d): перекроет стрелку папки", w, m.metricInt(KeyStartRowPad)/2)
	}
}

// У плиток своя прокрутка, и бегунок ведёт её так же.
func TestScrollbar_DragThumbScrollsTiles(t *testing.T) {
	m, _ := scrollMenu(t)
	var changed bool
	m.OnTilesChanged = func([]TileGroup) { changed = true }
	b := barOfMenu(t, m, areaTiles)
	span, room := b.content-b.viewH, b.viewH-b.thumb.Dy()

	x, y := b.thumb.Min.X+2, b.thumb.Min.Y+4
	m.OnMouseMove(x, y)
	if !m.WantsCapture(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true}) {
		t.Error("нажатие на бегунок плиток не запросило захват")
	}
	pressMenu(m, x, y)
	if m.v.drag.pending || m.v.drag.active {
		t.Fatal("нажатие на бегунок плиток начало перенос плитки")
	}
	m.OnMouseMove(x, y+120)
	want := 120 * span / room
	if got := m.v.tileScroll; got < want-1 || got > want+1 {
		t.Errorf("прокрутка плиток %d, ждали ~%d", got, want)
	}
	if m.v.listScroll != 0 {
		t.Error("бегунок плиток сдвинул список")
	}
	m.OnMouseMove(x, y+9000)
	if m.v.tileScroll != span {
		t.Errorf("предел прокрутки плиток %d, ждали %d", m.v.tileScroll, span)
	}
	release(m, x, y+9000)
	if changed {
		t.Error("бегунок плиток вызвал OnTilesChanged")
	}
	if m.v.bar.active {
		t.Error("перетаскивание не закончилось")
	}

	// Щелчок по дорожке: на страницу.
	nb := barOfMenu(t, m, areaTiles)
	step := m.metricInt(KeyTileUnit) + m.metricInt(KeyTileGap)
	x, y = nb.track.Min.X+3, nb.track.Min.Y+2
	m.OnMouseMove(x, y)
	pressMenu(m, x, y)
	release(m, x, y)
	if got, want := m.v.tileScroll, span-(nb.viewH-step); got != want {
		t.Errorf("страница вверх по плиткам: прокрутка %d, ждали %d", got, want)
	}
}

// Перетаскивание бегунка перерисовывает область прокручиваемого, а не меню
// целиком, и не будит полных кадров.
func TestScrollbar_DragInvalidatesOnlyItsArea(t *testing.T) {
	m, _ := tiledMenu(t)
	b := barOfMenu(t, m, areaList)
	x, y := b.thumb.Min.X+2, b.thumb.Min.Y+5
	m.OnMouseMove(x, y)
	pressMenu(m, x, y)

	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)
	m.OnMouseMove(x, y+40)
	m.OnMouseMove(x, y+80)
	if fulls != 0 {
		t.Errorf("перетаскивание вызвало %d полных перерисовок", fulls)
	}
	g := m.startGeometry(m.contentRect())
	if len(rects) == 0 {
		t.Fatal("перетаскивание ничего не заявило")
	}
	for _, r := range rects {
		if !r.In(g.list) {
			t.Errorf("заявлена область %v вне столбца списка %v", r, g.list)
		}
	}
	release(m, x, y+80)
}
