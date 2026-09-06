package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Своя начинка штатной полосы заголовка и отступ содержимого — запрос GG-52 и
// замечания к GG-51.
//
// SetChromeless убирал полосу целиком, а вместе с ней подпись и ✕: приложению
// оставалось нарисовать шапку заново. При этом область содержимого всё равно
// сжималась константой dlgPad, и боковая панель во всю левую сторону не
// доходила до края окна — вокруг неё оставалась рамка фона диалога.

// ─── Отступ содержимого (GG-52) ─────────────────────────────────────────────

// Без штатной полосы содержимое занимает диалог целиком.
func TestContentPadding_ChromelessReachesEdges(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetBounds(image.Rect(100, 100, 500, 400))
	d.SetChromeless(true)

	if got, want := d.ContentBounds(), d.Bounds(); got != want {
		t.Errorf("область содержимого %v при диалоге %v", got, want)
	}
}

// Обычному диалогу отступы оставлены прежними: MessageBox расставляет кнопки
// в расчёте на них.
func TestContentPadding_DefaultUnchanged(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetBounds(image.Rect(0, 0, 400, 300))

	h, v := d.ContentPadding()
	if h != 14 || v != 12 {
		t.Errorf("умолчание стало %d/%d", h, v)
	}
	cb := d.ContentBounds()
	if cb.Min.X != 14 || cb.Max.X != 386 || cb.Max.Y != 288 {
		t.Errorf("область содержимого %v", cb)
	}
}

// Отступ задаётся явно и снимается отрицательным значением.
func TestContentPadding_Explicit(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetBounds(image.Rect(0, 0, 400, 300))
	th := d.TitleHeight

	d.SetContentPadding(0)
	cb := d.ContentBounds()
	if cb.Min.X != 0 || cb.Max.X != 400 || cb.Max.Y != 300 || cb.Min.Y != th {
		t.Errorf("при нулевом отступе область содержимого %v (полоса %d)", cb, th)
	}

	d.SetContentPadding(-1) // вернуть умолчание
	if h, v := d.ContentPadding(); h != 14 || v != 12 {
		t.Errorf("умолчание не вернулось: %d/%d", h, v)
	}
}

// Виджет содержимого переезжает вместе с изменением отступа: сам он о нём не
// знает, а без перекладки рамка осталась бы на экране до ближайшего ресайза.
func TestContentPadding_RelaysContent(t *testing.T) {
	d := widget.NewDialog("Настройки", 400, 300)
	d.SetBounds(image.Rect(0, 0, 400, 300))
	panel := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 40, A: 255})
	panel.ShowHeader = false
	d.SetContent(panel)

	d.SetContentPadding(0)
	if got := panel.Bounds(); got != d.ContentBounds() {
		t.Errorf("содержимое %v при области %v", got, d.ContentBounds())
	}
}

// ─── Начинка полосы заголовка ───────────────────────────────────────────────

func titleBarDialog(t *testing.T) (*widget.Dialog, *widget.TextInput) {
	t.Helper()
	d := widget.NewDialog("Настройки", 600, 400)
	d.SetBounds(image.Rect(0, 0, 600, 400))
	search := widget.NewTextInput("Поиск настроек...")
	d.SetTitleBarContent(search)
	return d, search
}

// Виджет приложения стоит В полосе: правее подписи и левее ✕.
func TestTitleBarContent_PlacedInTheBar(t *testing.T) {
	d, search := titleBarDialog(t)

	r := search.Bounds()
	if r.Empty() {
		t.Fatal("виджету не отвели места в полосе заголовка")
	}
	if r.Max.Y > d.Bounds().Min.Y+d.TitleHeight {
		t.Errorf("виджет вылез из полосы: %v при полосе высотой %d", r, d.TitleHeight)
	}
	if r.Min.X <= d.Bounds().Min.X+14 {
		t.Errorf("виджет наехал на подпись заголовка: %v", r)
	}
	if r.Max.X > d.Bounds().Max.X-24 {
		t.Errorf("виджет наехал на ✕: %v", r)
	}
	if got := d.TitleBarContent(); got != widget.Widget(search) {
		t.Errorf("TitleBarContent вернул %T", got)
	}
}

// Полоса тянется вместе с диалогом, и начинка тянется с ней.
func TestTitleBarContent_FollowsResize(t *testing.T) {
	d, search := titleBarDialog(t)
	before := search.Bounds()

	d.SetResizable(true)
	d.Resize(900, 500)

	after := search.Bounds()
	if after.Max.X <= before.Max.X {
		t.Errorf("после расширения диалога начинка осталась прежней: %v → %v", before, after)
	}
	if after.Max.X > d.Bounds().Max.X-24 {
		t.Errorf("начинка наехала на ✕: %v", after)
	}
}

// Нажатие на виджет в полосе не тащит диалог: иначе поле поиска нельзя было бы
// ни выделить, ни прокрутить.
func TestTitleBarContent_DoesNotDragDialog(t *testing.T) {
	d, search := titleBarDialog(t)
	before := d.Bounds()

	pt := image.Pt(search.Bounds().Min.X+20, search.Bounds().Min.Y+8)
	d.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
	d.OnMouseMove(pt.X+60, pt.Y+40)
	d.OnMouseButton(widget.MouseEvent{X: pt.X + 60, Y: pt.Y + 40, Button: widget.MouseLeft, Pressed: false})

	if d.Bounds() != before {
		t.Errorf("диалог уехал за нажатием на поле поиска: %v → %v", before, d.Bounds())
	}
}

// Свободная часть полосы рядом с начинкой всё ещё тащит диалог: полоса
// осталась полосой заголовка.
func TestTitleBarContent_BarStillDrags(t *testing.T) {
	d, _ := titleBarDialog(t)
	before := d.Bounds()

	pt := image.Pt(before.Min.X+20, before.Min.Y+8) // подпись — не виджет
	d.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
	d.OnMouseMove(pt.X+30, pt.Y+20)
	d.OnMouseButton(widget.MouseEvent{X: pt.X + 30, Y: pt.Y + 20, Button: widget.MouseLeft, Pressed: false})

	if d.Bounds().Min.X != before.Min.X+30 || d.Bounds().Min.Y != before.Min.Y+20 {
		t.Errorf("диалог не сдвинулся за полосу заголовка: %v → %v", before, d.Bounds())
	}
}

// Без полосы заголовка начинке места нет.
func TestTitleBarContent_NoneWhenChromeless(t *testing.T) {
	d, _ := titleBarDialog(t)
	d.SetChromeless(true)

	if r := d.TitleBarContentBounds(); !r.Empty() {
		t.Errorf("у диалога без полосы нашлось место для начинки: %v", r)
	}
}

// ─── Кнопка сворачивания ────────────────────────────────────────────────────

// Кнопка переключает состояние и уведомляет приложение.
func TestNavButton_TogglesAndNotifies(t *testing.T) {
	d := widget.NewDialog("Настройки", 600, 400)
	d.SetBounds(image.Rect(0, 0, 600, 400))
	d.SetNavButton(true)

	var got []bool
	d.OnNavToggle = func(collapsed bool) { got = append(got, collapsed) }

	press := func() {
		pt := image.Pt(d.Bounds().Min.X+16, d.Bounds().Min.Y+d.TitleHeight/2)
		for _, c := range d.Children() {
			if !pt.In(c.Bounds()) {
				continue
			}
			if mb, ok := c.(interface{ OnMouseButton(widget.MouseEvent) bool }); ok {
				mb.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
				mb.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: false})
				return
			}
		}
		t.Fatalf("кнопки сворачивания нет под точкой %v", pt)
	}

	press()
	if !d.IsNavCollapsed() {
		t.Error("после нажатия область не помечена свёрнутой")
	}
	press()
	if d.IsNavCollapsed() {
		t.Error("повторное нажатие не развернуло область")
	}
	if len(got) != 2 || !got[0] || got[1] {
		t.Errorf("уведомления: %v", got)
	}
}

// Кнопка сдвигает подпись и начинку вправо: иначе она легла бы на заголовок.
func TestNavButton_ShiftsTitleAndContent(t *testing.T) {
	d, search := titleBarDialog(t)
	before := search.Bounds()

	d.SetNavButton(true)

	after := search.Bounds()
	if after.Min.X <= before.Min.X {
		t.Errorf("начинка не сдвинулась вправо: %v → %v", before, after)
	}
}

// Своя иконка рисуется вместо встроенной.
func TestNavButton_CustomIcon(t *testing.T) {
	scene := func(icon image.Image) *image.RGBA {
		d := widget.NewDialog("Настройки", 300, 200)
		d.SetNavButton(true)
		if icon != nil {
			d.SetNavIcons(icon, nil)
		}
		return widgetScene(t, d, image.Rect(20, 10, 320, 210))
	}

	icon := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range icon.Pix {
		icon.Pix[i] = 255
	}

	a, b := scene(nil), scene(icon)
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return
		}
	}
	t.Error("своя иконка нарисована так же, как встроенная")
}

// Кнопки нет, пока её не попросили: у прежних диалогов заголовок не меняется.
func TestNavButton_AbsentByDefault(t *testing.T) {
	d := widget.NewDialog("Настройки", 300, 200)
	if d.HasNavButton() {
		t.Fatal("кнопка сворачивания появилась сама")
	}
	a := widgetScene(t, d, image.Rect(20, 10, 320, 210))

	same := widget.NewDialog("Настройки", 300, 200)
	b := widgetScene(t, same, image.Rect(20, 10, 320, 210))
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatal("два одинаковых диалога нарисованы по-разному")
		}
	}
}

// ─── Окно ───────────────────────────────────────────────────────────────────

// Начинку полосы окно не растягивает на клиентскую область, в отличие от
// остальных детей.
func TestWindowTitleBar_ContentNotStretched(t *testing.T) {
	w := widget.NewWindow("Настройки", 800, 600)
	w.SetBounds(image.Rect(0, 0, 800, 600))
	search := widget.NewTextInput("Поиск...")
	w.SetTitleBarContent(search)

	r := search.Bounds()
	if r.Empty() {
		t.Fatal("виджету не отвели места в полосе заголовка окна")
	}
	if r == w.ContentBounds() {
		t.Error("начинка растянута на клиентскую область")
	}
	if r.Max.Y > w.ContentBounds().Min.Y {
		t.Errorf("начинка вылезла под полосу: %v при клиентской области %v", r, w.ContentBounds())
	}
}

// Нажатие на начинку не тащит окно.
func TestWindowTitleBar_ContentDoesNotDrag(t *testing.T) {
	w := widget.NewWindow("Настройки", 800, 600)
	w.SetBounds(image.Rect(0, 0, 800, 600))
	search := widget.NewTextInput("Поиск...")
	w.SetTitleBarContent(search)

	pt := image.Pt(search.Bounds().Min.X+20, search.Bounds().Min.Y+6)
	press := widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true}
	if w.WantsCapture(press) {
		t.Error("окно просит захват при нажатии на поле поиска в заголовке")
	}

	before := w.Bounds()
	w.OnMouseButton(press)
	w.OnMouseMove(pt.X+50, pt.Y+30)
	w.OnMouseButton(widget.MouseEvent{X: pt.X + 50, Y: pt.Y + 30, Button: widget.MouseLeft, Pressed: false})
	if w.Bounds() != before {
		t.Errorf("окно уехало за нажатием на поле поиска: %v → %v", before, w.Bounds())
	}
}

// Полоса заголовка окна по-прежнему тащит его.
func TestWindowTitleBar_StillDrags(t *testing.T) {
	w := widget.NewWindow("Настройки", 800, 600)
	// Стиль явно: в mac-раскладке точка нажатия попала бы в «светофор», и тест
	// проверял бы захват кнопкой, а не перетаскивание за полосу.
	w.TitleStyle = widget.WindowTitleWin
	w.SetBounds(image.Rect(0, 0, 800, 600))
	w.SetTitleBarContent(widget.NewTextInput("Поиск..."))

	// Левее начинки, ниже полосы ресайза: там полоса осталась полосой.
	y := w.ContentBounds().Min.Y / 2
	press := widget.MouseEvent{X: 30, Y: y, Button: widget.MouseLeft, Pressed: true}
	if !w.WantsCapture(press) {
		t.Error("окно не просит захват при нажатии на свободную часть полосы")
	}
}

// Кнопка сворачивания у окна: состояние и уведомление.
func TestWindowNavButton_Toggles(t *testing.T) {
	w := widget.NewWindow("Настройки", 800, 600)
	w.SetBounds(image.Rect(0, 0, 800, 600))
	w.SetNavButton(true)

	var got []bool
	w.OnNavToggle = func(collapsed bool) { got = append(got, collapsed) }

	w.SetNavCollapsed(true)
	w.SetNavCollapsed(false)
	if len(got) != 2 || !got[0] || got[1] {
		t.Errorf("уведомления: %v", got)
	}
	if w.IsNavCollapsed() {
		t.Error("состояние не вернулось в развёрнутое")
	}
}
