package widget

import (
	"image"
	"sync/atomic"
	"testing"
)

// GG-78: кнопка с выпадающим меню — разделённая (действие + стрелка) и
// целиком открывающая меню.

func withScreen(t *testing.T) {
	t.Helper()
	w, h := getScreenBounds()
	SetScreenBounds(2000, 2000)
	t.Cleanup(func() { SetScreenBounds(w, h) })
}

type splitFixture struct {
	mb                      *MenuButton
	clicks, opened, fetched int
}

// Кнопка (10,10)-(110,40): основная часть до x=94, стрелка 94..110.
func newSplitFixture(t *testing.T) *splitFixture {
	t.Helper()
	withScreen(t)
	f := &splitFixture{}
	f.mb = NewSplitButton("Pull", func() { f.clicks++ })
	f.mb.OnOpening = func() {
		f.opened++
		f.mb.Items = []MenuItem{
			{Text: "Fetch", OnClick: func() { f.fetched++ }},
			{Text: "Rebase", Disabled: true},
		}
	}
	f.mb.SetBounds(image.Rect(10, 10, 110, 40))
	return f
}

func press(w interface{ OnMouseButton(MouseEvent) bool }, x, y int) bool {
	return w.OnMouseButton(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: true})
}

func release(w interface{ OnMouseButton(MouseEvent) bool }, x, y int) bool {
	return w.OnMouseButton(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: false})
}

func click(w interface{ OnMouseButton(MouseEvent) bool }, x, y int) {
	press(w, x, y)
	release(w, x, y)
}

// firstItemPoint — точка на первом пункте открытого меню.
func firstItemPoint(m *PopupMenu) (int, int) {
	r := m.OverlayBounds()
	return r.Min.X + 20, r.Min.Y + 2 + m.ItemHeight/2
}

// Основная часть выполняет действие, меню не открывает.
func TestSplitButton_MainPartClicks(t *testing.T) {
	f := newSplitFixture(t)
	click(f.mb, 30, 25)
	if f.clicks != 1 || f.opened != 0 || f.mb.IsMenuOpen() {
		t.Fatalf("щелчок по основной части: действий %d, открытий %d, меню открыто %v", f.clicks, f.opened, f.mb.IsMenuOpen())
	}
	// Нажали на основной части, отпустили на стрелке — не щелчок.
	press(f.mb, 30, 25)
	release(f.mb, 100, 25)
	if f.clicks != 1 {
		t.Fatal("отпускание мимо основной части выполнило действие")
	}
}

// Стрелка открывает меню под кнопкой, пункт выполняется, меню закрывается.
func TestSplitButton_ArrowOpensMenu(t *testing.T) {
	f := newSplitFixture(t)
	press(f.mb, 100, 25)
	if !f.mb.IsMenuOpen() || f.opened != 1 || f.clicks != 0 {
		t.Fatalf("нажатие на стрелку: открыто %v, OnOpening %d, действий %d", f.mb.IsMenuOpen(), f.opened, f.clicks)
	}
	release(f.mb, 100, 25)
	if !f.mb.IsMenuOpen() {
		t.Fatal("отпускание той же кнопки погасило меню")
	}
	if got := f.mb.OverlayBounds().Min; got != image.Pt(10, 40) {
		t.Fatalf("меню открыто в %v, ждал под кнопкой (10,40)", got)
	}
	m := f.mb.menu.menu
	if items := m.Items(); len(items) != 2 || items[0].Text != "Fetch" {
		t.Fatalf("пункты меню не из OnOpening: %+v", items)
	}

	x, y := firstItemPoint(m)
	click(f.mb, x, y)
	if f.fetched != 1 || f.mb.IsMenuOpen() || f.clicks != 0 {
		t.Fatalf("выбор пункта: выполнен %d раз, меню открыто %v, действий %d", f.fetched, f.mb.IsMenuOpen(), f.clicks)
	}
}

// Нажатие по стрелке при открытом меню закрывает его и заново не открывает.
func TestSplitButton_ArrowTogglesMenu(t *testing.T) {
	f := newSplitFixture(t)
	click(f.mb, 100, 25)
	click(f.mb, 100, 25)
	if f.mb.IsMenuOpen() || f.opened != 1 {
		t.Fatalf("второе нажатие по стрелке: открыто %v, открытий %d", f.mb.IsMenuOpen(), f.opened)
	}
}

// Простая кнопка открывает меню щелчком в любом месте и действия не выполняет.
func TestMenuButton_WholeButtonOpensMenu(t *testing.T) {
	withScreen(t)
	clicks := 0
	mb := NewMenuButton("Git-Flow")
	mb.OnClick = func() { clicks++ }
	mb.Items = []MenuItem{{Text: "Start Feature…"}}
	mb.SetBounds(image.Rect(0, 0, 100, 30))

	click(mb, 20, 15)
	if !mb.IsMenuOpen() || clicks != 0 {
		t.Fatalf("щелчок по простой кнопке: открыто %v, действий %d", mb.IsMenuOpen(), clicks)
	}
}

// Доступность основной части и меню задаётся отдельно.
func TestSplitButton_PartsEnabledSeparately(t *testing.T) {
	f := newSplitFixture(t)
	f.mb.SetActionEnabled(false)
	click(f.mb, 30, 25)
	if f.clicks != 0 {
		t.Fatal("выключенная основная часть выполнила действие")
	}
	click(f.mb, 100, 25)
	if !f.mb.IsMenuOpen() {
		t.Fatal("при выключенном действии не открылось меню")
	}
	f.mb.CloseMenu()

	f.mb.SetActionEnabled(true)
	f.mb.SetMenuEnabled(false)
	click(f.mb, 100, 25)
	if f.mb.IsMenuOpen() || f.opened != 1 {
		t.Fatal("выключенная стрелка открыла меню")
	}
	click(f.mb, 30, 25)
	if f.clicks != 1 {
		t.Fatal("при выключенном меню не работает действие")
	}
}

// Клавиатура: ↓ открывает меню с подсвеченным первым пунктом, Enter выбирает;
// Enter на закрытом меню выполняет действие разделённой кнопки.
func TestSplitButton_Keyboard(t *testing.T) {
	f := newSplitFixture(t)
	f.mb.OnKeyEvent(KeyEvent{Code: KeyDown, Pressed: true})
	if !f.mb.IsMenuOpen() {
		t.Fatal("↓ не открыла меню")
	}
	if got := atomic.LoadInt32(&f.mb.menu.menu.hoverIdx); got != 0 {
		t.Fatalf("подсвечен пункт %d, ждал первый", got)
	}
	f.mb.OnKeyEvent(KeyEvent{Code: KeyEnter, Pressed: true})
	if f.fetched != 1 || f.mb.IsMenuOpen() {
		t.Fatalf("Enter в меню: выполнен %d раз, меню открыто %v", f.fetched, f.mb.IsMenuOpen())
	}
	f.mb.OnKeyEvent(KeyEvent{Code: KeyEnter, Pressed: true})
	if f.clicks != 1 {
		t.Fatalf("Enter на кнопке: действий %d, ждал 1", f.clicks)
	}
}

// Пустое меню не открывается — рамка без пунктов не меню.
func TestMenuButton_EmptyMenuNotShown(t *testing.T) {
	withScreen(t)
	mb := NewMenuButton("Log")
	mb.SetBounds(image.Rect(0, 0, 80, 30))
	if mb.OpenMenu() || mb.IsMenuOpen() {
		t.Fatal("открылось пустое меню")
	}
}

// В панели инструментов кнопка с меню шире обычной на зону стрелки, в режиме
// «только иконки» теряет подпись, а при переполнении становится подменю.
func TestToolBar_MenuButton(t *testing.T) {
	withScreen(t)
	plain := NewButton("Pull")
	split := NewSplitButton("Pull", nil)
	split.Items = []MenuItem{{Text: "Fetch"}}
	tb := NewToolBar()
	tb.AddChild(plain)
	tb.AddChild(split)
	tb.SetBounds(image.Rect(0, 0, 800, 40))

	if got, want := split.Bounds().Dx(), plain.Bounds().Dx()+menuButtonArrowW; got != want {
		t.Fatalf("ширина разделённой кнопки %d, ждал %d", got, want)
	}

	icon := image.NewRGBA(image.Rect(0, 0, 16, 16))
	split.Icon = icon
	tb.SetIconsOnly(true)
	if split.IconPos != IconOnly {
		t.Fatal("режим «только иконки» не тронул кнопку с меню")
	}
	tb.SetIconsOnly(false)

	narrow := NewToolBar()
	wide := NewButton("очень длинная кнопка, занимающая всё место")
	narrow.AddChild(wide)
	narrow.AddChild(split)
	narrow.SetBounds(image.Rect(0, 0, 200, 40))
	if narrow.OverflowCount() != 1 {
		t.Fatalf("переполнилось %d элементов, ждал 1", narrow.OverflowCount())
	}
	items := narrow.overflowItems()
	if len(items) != 1 || len(items[0].SubItems) != 3 {
		t.Fatalf("пункт переполнения: %+v", items)
	}
	sub := items[0].SubItems
	if sub[0].Text != "Pull" || !sub[1].Separator || sub[2].Text != "Fetch" {
		t.Fatalf("подменю переполнения: %+v", sub)
	}
}

// Разметка: <SplitButton> и <MenuButton> с пунктами.
func TestMenuButton_XAML(t *testing.T) {
	_, reg, err := LoadUIFromXAML([]byte(`<Window Title="T" Width="600" Height="100">
  <ToolBar Name="tb" Left="0" Top="0" Width="600" Height="40">
    <SplitButton Name="pull" Content="Pull">
      <MenuItem Header="Fetch"/>
      <MenuItem Header="Pull (rebase)"/>
    </SplitButton>
    <MenuButton Name="flow" Content="Git-Flow" IsMenuEnabled="False">
      <MenuItem Header="Start Feature…"/>
    </MenuButton>
  </ToolBar>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	pull, ok := reg["pull"].(*MenuButton)
	if !ok {
		t.Fatalf("pull — %T, ждал *MenuButton", reg["pull"])
	}
	flow, ok := reg["flow"].(*MenuButton)
	if !ok {
		t.Fatalf("flow — %T, ждал *MenuButton", reg["flow"])
	}
	if !pull.Split || len(pull.Items) != 2 || pull.Items[1].Text != "Pull (rebase)" {
		t.Fatalf("pull: Split=%v Items=%+v", pull.Split, pull.Items)
	}
	if flow.Split || flow.MenuEnabled() || len(flow.Items) != 1 {
		t.Fatalf("flow: Split=%v MenuEnabled=%v Items=%+v", flow.Split, flow.MenuEnabled(), flow.Items)
	}
	if pull.CornerRadius != 4 {
		t.Fatalf("скругление кнопки панели %d, ждал 4", pull.CornerRadius)
	}
}
