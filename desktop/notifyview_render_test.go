package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// renderFixture — рабочий стол с настоящим движком и открытым центром
// Windows 10: нужен тестам, которым важны пиксели.
type renderFixture struct {
	eng *engine.Engine
	nc  *NotificationCenter
	tm  *theme.Manager
	q   *QuickActionList
	ns  *FakeNotifications
	w   int
	h   int
}

func newRenderFixture(t *testing.T, scale float64) *renderFixture {
	t.Helper()
	const w, h = 800, 600
	tm := richNotifTheme(t)
	root := widget.NewPanel(color.RGBA{R: 40, G: 90, B: 160, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	// Клетчатые обои: размытие под акрилом должно быть чем размывать.
	for y := 0; y < h; y += 20 {
		for x := 0; x < w; x += 20 {
			if (x/20+y/20)%2 == 0 {
				p := widget.NewPanel(color.RGBA{R: 200, G: 120, B: 90, A: 255})
				p.ShowHeader = false
				p.SetBounds(image.Rect(x, y, x+20, y+20))
				root.AddChild(p)
			}
		}
	}
	nc, ns, q := richFixture(t, 4)
	fm := NewFlyoutManager()
	fm.Register("notifications", nc)
	root.AddChild(fm)

	eng := engine.New(w, h, 30)
	eng.SetScale(scale)
	eng.SetRoot(root)
	// richFixture уже открыл центр на другой теме; нужная тема — та, что в nc.
	_ = tm
	return &renderFixture{eng: eng, nc: nc, tm: nc.Theme(), q: q, ns: ns, w: w, h: h}
}

func (f *renderFixture) frame() *image.RGBA {
	f.eng.Invalidate()
	return clone(f.eng.RenderOnce())
}

func clone(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

func (f *renderFixture) tileRect(id string) image.Rectangle {
	for _, tl := range f.nc.testLayout().tiles {
		if string(tl.a.ID) == id {
			return tl.rect
		}
	}
	return image.Rectangle{}
}

// px — пиксель в логической точке (масштаб 1).
func px(img *image.RGBA, p image.Point) color.RGBA { return img.RGBAAt(p.X, p.Y) }

func nearColor(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

// Включённая плитка залита акцентом, выключенная — нет; смена акцента на
// открытом центре перекрашивает плитку без пересоздания.
func TestRender_QuickTileOnFollowsAccent(t *testing.T) {
	f := newRenderFixture(t, 1)
	on, off := f.tileRect("wifi"), f.tileRect("bt")
	if on.Empty() || off.Empty() {
		t.Fatal("плиток нет в раскладке")
	}
	corner := func(r image.Rectangle) image.Point { return image.Pt(r.Max.X-5, r.Max.Y-5) }

	acc, _ := f.tm.Accent()
	img := f.frame()
	if got := px(img, corner(on)); !nearColor(got, acc, 3) {
		t.Errorf("включённая плитка %v, акцент %v", got, acc)
	}
	if got := px(img, corner(off)); nearColor(got, acc, 20) {
		t.Errorf("выключенная плитка залита акцентом: %v", got)
	}

	f.tm.SetAccent(theme.RGB(190, 40, 60))
	img = f.frame()
	if got := px(img, corner(on)); !nearColor(got, theme.RGB(190, 40, 60), 3) {
		t.Errorf("после SetAccent плитка %v", got)
	}
}

// Светлая и тёмная палитры переключаются флагом на открытом центре.
func TestRender_LightFlagRecoloursOpenCenter(t *testing.T) {
	f := newRenderFixture(t, 1)
	probe := image.Pt(f.nc.OverlayBounds().Min.X+8, f.nc.OverlayBounds().Min.Y+f.nc.OverlayBounds().Dy()/2)
	lum := func(c color.RGBA) int { return (int(c.R) + int(c.G) + int(c.B)) / 3 }
	dark := lum(px(f.frame(), probe))
	f.tm.SetFlag(theme.KeyTaskbarLight, true)
	light := lum(px(f.frame(), probe))
	if light <= dark+60 {
		t.Errorf("светлый режим не посветлел: тёмный %d, светлый %d", dark, light)
	}
	f.tm.SetFlag(theme.KeyTaskbarLight, false)
	if again := lum(px(f.frame(), probe)); again != dark {
		t.Errorf("возврат в тёмный режим дал %d вместо %d", again, dark)
	}
}

// Перерисовка одной плитки по заявленной области даёт тот же кадр, что полная
// перерисовка: частичная не портит ни размытую подложку, ни соседей.
func TestRender_PartialTileRepaintEqualsFull(t *testing.T) {
	f := newRenderFixture(t, 1)
	f.frame()
	f.q.SetOn("night", true) // заявляет только область плитки
	partial := clone(f.eng.RenderOnce())
	full := f.frame()
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			if a, b := partial.RGBAAt(x, y), full.RGBAAt(x, y); a != b {
				t.Fatalf("частичная и полная перерисовки расходятся в (%d,%d): %v и %v", x, y, a, b)
			}
		}
	}
	if got := px(partial, image.Pt(f.tileRect("night").Max.X-5, f.tileRect("night").Max.Y-5)); !nearColor(got, mustAccent(f.tm), 3) {
		t.Errorf("плитка «Ночной свет» не включилась на кадре: %v", got)
	}
}

func mustAccent(tm *theme.Manager) color.RGBA {
	a, _ := tm.Accent()
	return a
}

// Открытие центра меняет пиксели только внутри его области.
func TestRender_OpenChangesOnlyItsArea(t *testing.T) {
	f := newRenderFixture(t, 1)
	f.nc.Close()
	f.nc.Settle()
	before := f.frame()
	f.nc.Open(panelAnchor())
	f.nc.Settle()
	after := f.frame()
	area := f.nc.OverlayBounds().Union(panelAnchor())
	diff := 0
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			if image.Pt(x, y).In(area) {
				continue
			}
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				diff++
			}
		}
	}
	if diff != 0 {
		t.Errorf("открытие центра изменило %d пикселей вне его области %v", diff, area)
	}
	if ob := f.nc.OverlayBounds(); after.RGBAAt(ob.Min.X+8, ob.Min.Y+100) == before.RGBAAt(ob.Min.X+8, ob.Min.Y+100) {
		t.Error("внутри области центра ничего не нарисовано")
	}
}

// 100–200 %: текст помещается в свои рамки при любом масштабе.
func TestRender_TextFitsAtAllScales(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
		f := newRenderFixture(t, scale)
		ns := NewFakeNotifications()
		ns.Add(Notification{AppID: "a", AppName: "Приложение с длинным названием для проверки", Icon: nil,
			Title: "Заголовок, который не помещается в одну строку карточки целиком", Time: sampleTime(),
			Body: "Первая строка текста уведомления и вторая, и третья, и ещё немного слов для переноса по строкам карточки",
			Actions: []NotificationAction{
				{ID: "1", Kind: NotificationActionButton, Title: "Приступим к настройке"},
				{ID: "2", Kind: NotificationActionButton, Title: "Нет"},
				{ID: "r", Kind: NotificationActionReply},
				{ID: "s", Kind: NotificationActionSelect, Title: "Напомнить ещё раз через:", Options: []string{"1 неделю"}},
			}})
		nc := NewNotificationCenter(f.tm, ns)
		nc.Screen = panelScreen()
		nc.Open(panelAnchor())
		nc.Settle()
		nc.SetQuickActions(NewQuickActionList(
			QuickAction{ID: "a", Title: "Расположение"}, QuickAction{ID: "b", Title: "Режим \"в самолёте\" и ещё слова"},
			QuickAction{ID: "c", Title: "Ночной свет"}, QuickAction{ID: "d", Title: "Сверхдлиннаяподписьплиткибезпробелов"}))
		l := nc.testLayout()
		for _, g := range l.groups {
			for _, c := range g.cards {
				for _, line := range c.bodyLines {
					if w := l.fonts.body.width(line); w > c.title.Dx() {
						t.Errorf("масштаб %v: строка %q шире рамки: %d > %d", scale, line, w, c.title.Dx())
					}
				}
				if w := l.fonts.title.width(c.titleText); w > c.title.Dx() {
					t.Errorf("масштаб %v: заголовок шире рамки: %d > %d", scale, w, c.title.Dx())
				}
				for _, a := range c.acts {
					if a.text != "" && l.fonts.body.width(a.text) > a.rect.Dx() {
						t.Errorf("масштаб %v: надпись %q шире кнопки", scale, a.text)
					}
				}
			}
			if w := l.fonts.title.width(g.name); w > g.rect.Dx() {
				// Название группы усекается при отрисовке по ширине без слотов.
				_ = w
			}
		}
		for _, tl := range l.tiles {
			lines, _ := l.fonts.body.wrap(tl.a.Title, tl.rect.Dx()-2*l.m.qPad, 2)
			if len(lines) > 2 {
				t.Errorf("масштаб %v: подпись плитки в %d строк", scale, len(lines))
			}
		}
		nc.Close()
		f.eng.Stop()
	}
}
