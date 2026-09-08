package widget

import (
	"image"
	"image/color"
	"testing"
)

// Значок в пункте меню — запрос GG-53.
//
// Поле MenuItem.Icon было строкой с пометкой «зарезервировано» и не читалось
// ни отрисовкой, ни измерением ширины: пункт со значком рисовался так же, как
// без него. Команды тулбара и меню — одни и те же, а выглядели по-разному.

// iconCtx — контекст, запоминающий, что и куда нарисовали.
type iconCtx struct {
	DrawContext
	texts  []string
	textX  []int
	images []image.Rectangle
}

func (c *iconCtx) SetPixel(int, int, color.RGBA) {}
func (c *iconCtx) DrawText(text string, x, _ int, _ color.RGBA) {
	c.texts = append(c.texts, text)
	c.textX = append(c.textX, x)
}
func (c *iconCtx) DrawImageScaled(_ image.Image, x, y, w, h int) {
	c.images = append(c.images, image.Rect(x, y, x+w, y+h))
}
func (c *iconCtx) FillRect(int, int, int, int, color.RGBA)           {}
func (c *iconCtx) FillRectAlpha(int, int, int, int, color.RGBA)      {}
func (c *iconCtx) DrawHLine(int, int, int, color.RGBA)               {}
func (c *iconCtx) DrawVLine(int, int, int, color.RGBA)               {}
func (c *iconCtx) DrawBorder(int, int, int, int, color.RGBA)         {}
func (c *iconCtx) FillRoundRect(int, int, int, int, int, color.RGBA) {}

// menuIcon — картинка-заглушка нужного размера.
func menuIcon() image.Image { return image.NewRGBA(image.Rect(0, 0, 16, 16)) }

// textXOf возвращает X подписи с заданным текстом.
func textXOf(c *iconCtx, text string) (int, bool) {
	for i, s := range c.texts {
		if s == text {
			return c.textX[i], true
		}
	}
	return 0, false
}

func iconMenu(t *testing.T, items []MenuItem) (*PopupMenu, *iconCtx) {
	t.Helper()
	pm := NewPopupMenu()
	pm.SetItems(items)
	pm.Show(10, 10)
	ctx := &iconCtx{}
	pm.DrawOverlay(ctx)
	return pm, ctx
}

// Значок рисуется — раньше поле не читалось вовсе.
func TestPopupMenu_IconIsDrawn(t *testing.T) {
	_, ctx := iconMenu(t, []MenuItem{
		{Text: "Копировать", Icon: menuIcon()},
		{Text: "Вставить"},
	})

	if len(ctx.images) != 1 {
		t.Fatalf("нарисовано %d значков, ожидался один", len(ctx.images))
	}
	if r := ctx.images[0]; r.Dx() != r.Dy() || r.Dx() < 8 {
		t.Errorf("значок нарисован прямоугольником %v", r)
	}
}

// Зона под значок отводится ВСЕМУ меню: иначе подписи пунктов без значка
// стояли бы левее соседей.
func TestPopupMenu_IconGutterIsMenuWide(t *testing.T) {
	pm, ctx := iconMenu(t, []MenuItem{
		{Text: "Копировать", Icon: menuIcon()},
		{Text: "Вставить"}, // значка нет
	})

	if pm.iconGutter() == 0 {
		t.Fatal("у меню со значком нет зоны под значки")
	}
	withIcon, ok1 := textXOf(ctx, "Копировать")
	without, ok2 := textXOf(ctx, "Вставить")
	if !ok1 || !ok2 {
		t.Fatalf("подписи не нарисованы: %v", ctx.texts)
	}
	if withIcon != without {
		t.Errorf("подписи разъехались: %d и %d", withIcon, without)
	}
}

// Меню без значков остаётся прежним: ни зоны, ни сдвига подписи.
func TestPopupMenu_NoIconsNoGutter(t *testing.T) {
	plain, plainCtx := iconMenu(t, []MenuItem{{Text: "Вставить"}})
	if plain.iconGutter() != 0 {
		t.Errorf("зона под значки %d у меню без значков", plain.iconGutter())
	}

	_, iconedCtx := iconMenu(t, []MenuItem{{Text: "Вставить", Icon: menuIcon()}})
	x1, _ := textXOf(plainCtx, "Вставить")
	x2, _ := textXOf(iconedCtx, "Вставить")
	if x2 <= x1 {
		t.Errorf("подпись со значком начинается на %d, без значка — на %d", x2, x1)
	}
}

// Значок делает меню шире ровно на свою зону: подпись не должна упираться в
// правый край.
func TestPopupMenu_IconWidensMenu(t *testing.T) {
	const long = "Показать в файловом менеджере"

	plain := NewPopupMenu()
	plain.MinWidth = 0 // иначе разницу съедает минимум, а не зона значка
	plain.SetItems([]MenuItem{{Text: long}})
	pw, _ := plain.calcSize()

	iconed := NewPopupMenu()
	iconed.MinWidth = 0
	iconed.SetItems([]MenuItem{{Text: long, Icon: menuIcon()}})
	iw, _ := iconed.calcSize()

	if iw-pw != iconed.iconGutter() {
		t.Errorf("меню со значком шире на %d при зоне %d", iw-pw, iconed.iconGutter())
	}
}

// Значок и отметка уживаются: значок правее зоны отметки, подпись правее
// значка, ничто ни на что не налезает.
func TestPopupMenu_IconAndCheckMarkCoexist(t *testing.T) {
	pm, ctx := iconMenu(t, []MenuItem{
		{Text: "Показывать скрытые", Icon: menuIcon(), Checkable: true, Checked: true},
	})

	if len(ctx.images) != 1 {
		t.Fatalf("нарисовано %d значков", len(ctx.images))
	}
	icon := ctx.images[0]
	x, _, _, _ := pm.geo()
	checkRight := x + pm.PaddingX + checkMarkSize

	if icon.Min.X < checkRight {
		t.Errorf("значок на %d налезает на зону отметки (до %d)", icon.Min.X, checkRight)
	}
	textX, ok := textXOf(ctx, "Показывать скрытые")
	if !ok {
		t.Fatal("подпись не нарисована")
	}
	if textX < icon.Max.X {
		t.Errorf("подпись на %d налезает на значок (до %d)", textX, icon.Max.X)
	}
}

// Свой размер значка соблюдается, но не выше пункта: значок крупнее строки
// разрезал бы соседние пункты.
func TestPopupMenu_IconSizeRespectedAndClamped(t *testing.T) {
	_, ctx := iconMenu(t, []MenuItem{{Text: "Открыть", Icon: menuIcon(), IconSize: 12}})
	if got := ctx.images[0].Dx(); got != 12 {
		t.Errorf("свой размер значка не соблюдён: %d вместо 12", got)
	}

	pm, big := iconMenu(t, []MenuItem{{Text: "Открыть", Icon: menuIcon(), IconSize: 500}})
	if got := big.images[0].Dx(); got > pm.ItemHeight {
		t.Errorf("значок %d выше пункта %d", got, pm.ItemHeight)
	}
}

// Подменю рисует значки так же: у MenuBar подменю — тот же PopupMenu.
func TestPopupMenu_SubMenuIconsDrawn(t *testing.T) {
	pm := NewPopupMenu()
	pm.SetItems([]MenuItem{{
		Text: "Удалённые",
		SubItems: []MenuItem{
			{Text: "Получить", Icon: menuIcon()},
			{Text: "Отправить", Icon: menuIcon()},
		},
	}})
	pm.Show(10, 10)
	pm.openChild(0)

	ctx := &iconCtx{}
	pm.DrawOverlay(ctx) // рисует и раскрытое подменю

	if len(ctx.images) != 2 {
		t.Errorf("в подменю нарисовано %d значков, ожидалось 2", len(ctx.images))
	}
}
