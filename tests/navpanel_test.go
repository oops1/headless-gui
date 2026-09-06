package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Боковая навигация во всю высоту окна и её свёрнутый режим.
//
// Заливка панели обрывалась под полосой заголовка: полосу движок рисует во всю
// ширину, и левый верхний угол оставался движковым. А свёрнутая панель,
// собранная из панели и кнопок вручную, превращалась в пустое место — пункты
// исчезали вместе с подписями, хотя сворачивать нужно ШИРИНУ, а не навигацию.

func testIcon(v uint8) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v, v, v, 255
	}
	return img
}

func navDialog(t *testing.T) (*widget.Dialog, *widget.NavPanel) {
	t.Helper()
	d := widget.NewDialog("Настройки", 800, 500)
	d.SetBounds(image.Rect(0, 0, 800, 500))
	d.SetContentPadding(0)

	nav := widget.NewNavPanel()
	for i, s := range []string{"Общие", "Git", "Учётные данные", "SSH"} {
		nav.AddItem(testIcon(uint8(40+i*40)), s)
	}
	d.SetNavPanel(nav)
	return d, nav
}

// Панель занимает колонку от ВЕРХНЕГО края окна до нижнего — вместе с полосой
// заголовка. Ради этого всё и делалось: цвет панели доходит до края.
func TestNavPanel_ColumnReachesTopEdge(t *testing.T) {
	d, nav := navDialog(t)

	r := nav.Bounds()
	b := d.Bounds()
	if r.Min != b.Min {
		t.Errorf("панель начинается в %v при окне %v", r.Min, b)
	}
	if r.Max.Y != b.Max.Y {
		t.Errorf("панель не доходит до низа: %v при окне %v", r, b)
	}
	if r.Dx() != nav.Width() {
		t.Errorf("ширина колонки %d при ширине панели %d", r.Dx(), nav.Width())
	}
}

// Содержимое начинается за панелью, а не под ней.
func TestNavPanel_ContentStartsAfterColumn(t *testing.T) {
	d, nav := navDialog(t)

	if got, want := d.ContentBounds().Min.X, nav.Bounds().Max.X; got != want {
		t.Errorf("содержимое начинается на %d, панель кончается на %d", got, want)
	}
}

// Свёрнутая панель — полоска с иконками: ширина уходит, пункты остаются.
func TestNavPanel_CollapsedKeepsItems(t *testing.T) {
	d, nav := navDialog(t)
	wideContent := d.ContentBounds().Dx()

	nav.SetCollapsed(true)

	if !nav.IsCollapsed() {
		t.Fatal("панель не свернулась")
	}
	if got := nav.Bounds().Dx(); got != nav.CollapsedWidth {
		t.Errorf("ширина свёрнутой полоски %d, ожидалась %d", got, nav.CollapsedWidth)
	}
	for i := 0; i < nav.ItemCount(); i++ {
		if nav.ItemRect(i).Empty() {
			t.Errorf("пункт %d исчез в свёрнутом режиме", i)
		}
	}
	if d.ContentBounds().Dx() <= wideContent {
		t.Error("содержимое не заняло освободившееся место")
	}
}

// Свёрнутая полоска рисует иконки, а не пустоту.
func TestNavPanel_CollapsedDrawsIcons(t *testing.T) {
	scene := func(withIcons bool) *image.RGBA {
		d := widget.NewDialog("Настройки", 300, 220)
		d.SetContentPadding(0)
		nav := widget.NewNavPanel()
		for i, s := range []string{"Общие", "Git"} {
			var ic image.Image
			if withIcons {
				ic = testIcon(uint8(200 + i*20))
			}
			nav.AddItem(ic, s)
		}
		d.SetNavPanel(nav)
		nav.SetCollapsed(true)
		return widgetScene(t, d, image.Rect(20, 10, 320, 230))
	}

	a, b := scene(false), scene(true)
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return
		}
	}
	t.Error("свёрнутая полоска с иконками нарисована так же, как без них")
}

// Подпись заголовка остаётся видна поверх поднятой панели.
func TestNavPanel_TitleStaysVisible(t *testing.T) {
	scene := func(title string) *image.RGBA {
		d := widget.NewDialog(title, 300, 220)
		d.SetContentPadding(0)
		nav := widget.NewNavPanel()
		nav.AddItem(nil, "Общие")
		d.SetNavPanel(nav)
		return widgetScene(t, d, image.Rect(20, 10, 320, 230))
	}

	a, b := scene(""), scene("Настройки")
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return
		}
	}
	t.Error("подпись заголовка не нарисована: панель закрыла её собой")
}

// Кнопка «≡» в полосе сворачивает именно эту панель — без единой строки в
// приложении.
func TestNavPanel_NavButtonCollapses(t *testing.T) {
	d, nav := navDialog(t)
	d.SetNavButton(true)

	d.SetNavCollapsed(true)
	if !nav.IsCollapsed() {
		t.Error("кнопка не свернула панель")
	}
	d.SetNavCollapsed(false)
	if nav.IsCollapsed() {
		t.Error("кнопка не развернула панель обратно")
	}
}

// Клик выбирает пункт и уведомляет приложение.
func TestNavPanel_SelectOnClick(t *testing.T) {
	_, nav := navDialog(t)

	var got []int
	nav.OnSelect = func(i int) { got = append(got, i) }

	r := nav.ItemRect(2)
	nav.OnMouseButton(widget.MouseEvent{
		X: r.Min.X + 10, Y: r.Min.Y + r.Dy()/2, Button: widget.MouseLeft, Pressed: true})

	if nav.Selected() != 2 {
		t.Errorf("выбран пункт %d", nav.Selected())
	}
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("уведомления: %v", got)
	}
}

// SetSelected обработчик НЕ зовёт: он сообщает о выборе пользователя, а не о
// состоянии, выставленном программой.
func TestNavPanel_SetSelectedIsSilent(t *testing.T) {
	_, nav := navDialog(t)
	called := 0
	nav.OnSelect = func(int) { called++ }

	nav.SetSelected(3)
	if nav.Selected() != 3 {
		t.Errorf("выбран пункт %d", nav.Selected())
	}
	if called != 0 {
		t.Errorf("обработчик вызван %d раз", called)
	}
}

// Пункты начинаются НИЖЕ подписи заголовка: панель поднята под верх, и первый
// пункт иначе оказался бы под ней.
func TestNavPanel_ItemsStartBelowCaption(t *testing.T) {
	d, nav := navDialog(t)

	if got, want := nav.ItemRect(0).Min.Y, d.Bounds().Min.Y+d.TitleHeight; got < want {
		t.Errorf("первый пункт на %d, полоса заголовка кончается на %d", got, want)
	}
}

// Панель шире половины окна ужимается: занявшая всё окно, она оставила бы
// содержимое без места.
func TestNavPanel_WidthClampedToHalf(t *testing.T) {
	d := widget.NewDialog("Настройки", 300, 200)
	d.SetBounds(image.Rect(0, 0, 300, 200))
	nav := widget.NewNavPanel()
	nav.ExpandedWidth = 400
	nav.AddItem(nil, "Общие")
	d.SetNavPanel(nav)

	if got := nav.Bounds().Dx(); got != 150 {
		t.Errorf("ширина колонки %d при окне 300", got)
	}
}

// Окно: клиентская область отступает вправо от панели, остальные дети
// раскладываются уже по ней.
func TestWindowNavPanel_ClientAreaShifts(t *testing.T) {
	w := widget.NewWindow("Настройки", 800, 600)
	w.SetBounds(image.Rect(0, 0, 800, 600))
	body := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	body.ShowHeader = false
	w.AddChild(body)

	nav := widget.NewNavPanel()
	nav.AddItem(nil, "Общие")
	w.SetNavPanel(nav)

	if got, want := w.ContentBounds().Min.X, nav.Bounds().Max.X; got != want {
		t.Errorf("клиентская область начинается на %d, панель кончается на %d", got, want)
	}
	if body.Bounds() != w.ContentBounds() {
		t.Errorf("содержимое %v при клиентской области %v", body.Bounds(), w.ContentBounds())
	}
	if nav.Bounds() == w.ContentBounds() {
		t.Error("панель растянута на клиентскую область")
	}
}

// Начинка полосы (поиск) не заезжает на колонку панели: у панели свой цвет и
// свои пункты, и поле поверх неё выглядит чужим.
func TestNavPanel_TitleBarContentStartsAfterColumn(t *testing.T) {
	d, nav := navDialog(t)
	search := widget.NewTextInput("Поиск...")
	d.SetTitleBarContent(search)

	if got, want := search.Bounds().Min.X, nav.Bounds().Max.X; got < want {
		t.Errorf("поиск начинается на %d, колонка кончается на %d", got, want)
	}

	// Свёрнутая полоска освобождает место — поиск переезжает левее.
	before := search.Bounds().Min.X
	nav.SetCollapsed(true)
	if after := search.Bounds().Min.X; after >= before {
		t.Errorf("после сворачивания поиск остался на %d (было %d)", after, before)
	}
	if got, want := search.Bounds().Min.X, nav.Bounds().Max.X; got < want {
		t.Errorf("поиск заехал на свёрнутую полоску: %d при её крае %d", got, want)
	}
}
