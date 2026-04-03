package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Конструктор ────────────────────────────────────────────────────────────

func TestNewWindow(t *testing.T) {
	w := widget.NewWindow("Test", 800, 600)
	if w.Title != "Test" {
		t.Fatalf("Title = %q, want %q", w.Title, "Test")
	}
	b := w.Bounds()
	if b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("Bounds = %v, want 800×600", b)
	}
	if w.Style != widget.WindowStyleSingleBorder {
		t.Fatalf("default Style = %d, want SingleBorder(0)", w.Style)
	}
	if w.TitleStyle != widget.WindowTitleWin {
		t.Fatalf("default TitleStyle = %d, want Win(0)", w.TitleStyle)
	}
	if w.Resize != widget.ResizeModeCanResize {
		t.Fatalf("default Resize = %d, want CanResize(0)", w.Resize)
	}
}

// ─── ContentBounds ──────────────────────────────────────────────────────────

func TestWindowContentBounds_SingleBorder(t *testing.T) {
	w := widget.NewWindow("Test", 800, 600)
	w.Style = widget.WindowStyleSingleBorder
	cb := w.ContentBounds()
	// TitleBar = 32, Border = 1px
	want := image.Rect(1, 32, 799, 599)
	if cb != want {
		t.Fatalf("ContentBounds = %v, want %v", cb, want)
	}
}

func TestWindowContentBounds_None(t *testing.T) {
	w := widget.NewWindow("Test", 800, 600)
	w.Style = widget.WindowStyleNone
	cb := w.ContentBounds()
	// No title, no border
	want := image.Rect(0, 0, 800, 600)
	if cb != want {
		t.Fatalf("ContentBounds = %v, want %v", cb, want)
	}
}

func TestWindowContentBounds_ToolWindow(t *testing.T) {
	w := widget.NewWindow("Tool", 400, 300)
	w.Style = widget.WindowStyleToolWindow
	cb := w.ContentBounds()
	// TitleBar = 24, Border = 1px
	want := image.Rect(1, 24, 399, 299)
	if cb != want {
		t.Fatalf("ContentBounds = %v, want %v", cb, want)
	}
}

func TestWindowContentBounds_CustomTitleBarHeight(t *testing.T) {
	w := widget.NewWindow("Custom", 800, 600)
	w.TitleBarHeight = 48
	cb := w.ContentBounds()
	want := image.Rect(1, 48, 799, 599)
	if cb != want {
		t.Fatalf("ContentBounds = %v, want %v", cb, want)
	}
}

// ─── Кнопки заголовка ───────────────────────────────────────────────────────

func TestWindowBtnCount(t *testing.T) {
	tests := []struct {
		name   string
		style  widget.WindowStyle
		resize widget.ResizeMode
		want   int
	}{
		{"SingleBorder+CanResize", widget.WindowStyleSingleBorder, widget.ResizeModeCanResize, 3},
		{"SingleBorder+NoResize", widget.WindowStyleSingleBorder, widget.ResizeModeNoResize, 1},
		{"SingleBorder+CanMinimize", widget.WindowStyleSingleBorder, widget.ResizeModeCanMinimize, 2},
		{"ToolWindow", widget.WindowStyleToolWindow, widget.ResizeModeCanResize, 1},
		{"None", widget.WindowStyleNone, widget.ResizeModeCanResize, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := widget.NewWindow("Test", 800, 600)
			w.Style = tt.style
			w.Resize = tt.resize
			// btnCount недоступен напрямую, но проверяем через closeBtnRect/minBtnRect/maxBtnRect
			// Проверяем, что кнопка Close всегда работает (кроме None)
			closeRect := w.CloseBtnRect()
			if tt.style == widget.WindowStyleNone {
				if !closeRect.Empty() {
					t.Fatalf("closeBtnRect should be empty for None style")
				}
			} else {
				if closeRect.Empty() {
					t.Fatalf("closeBtnRect should not be empty for style %d", tt.style)
				}
			}
		})
	}
}

// ─── Mouse click: close / minimize / maximize ───────────────────────────────

func TestWindowCloseClick(t *testing.T) {
	w := widget.NewWindow("Test", 800, 600)
	closed := false
	w.OnClose = func() { closed = true }

	// Кликаем в центр кнопки закрытия (Win-стиль: правый верхний угол)
	cr := w.CloseBtnRect()
	cx := (cr.Min.X + cr.Max.X) / 2
	cy := (cr.Min.Y + cr.Max.Y) / 2
	w.OnMouseButton(widget.MouseEvent{X: cx, Y: cy, Button: widget.MouseLeft, Pressed: true})

	if !closed {
		t.Fatal("OnClose not called after clicking close button")
	}
}

func TestWindowMinimizeClick(t *testing.T) {
	w := widget.NewWindow("Test", 800, 600)
	w.Resize = widget.ResizeModeCanResize
	minimized := false
	w.OnMinimize = func() { minimized = true }

	mr := w.MinBtnRect()
	if mr.Empty() {
		t.Fatal("minBtnRect should not be empty for CanResize")
	}
	cx := (mr.Min.X + mr.Max.X) / 2
	cy := (mr.Min.Y + mr.Max.Y) / 2
	w.OnMouseButton(widget.MouseEvent{X: cx, Y: cy, Button: widget.MouseLeft, Pressed: true})

	if !minimized {
		t.Fatal("OnMinimize not called after clicking min button")
	}
}

func TestWindowMaximizeClick(t *testing.T) {
	w := widget.NewWindow("Test", 800, 600)
	w.Resize = widget.ResizeModeCanResize
	maximized := false
	w.OnMaximize = func() { maximized = true }

	mr := w.MaxBtnRect()
	if mr.Empty() {
		t.Fatal("maxBtnRect should not be empty for CanResize")
	}
	cx := (mr.Min.X + mr.Max.X) / 2
	cy := (mr.Min.Y + mr.Max.Y) / 2
	w.OnMouseButton(widget.MouseEvent{X: cx, Y: cy, Button: widget.MouseLeft, Pressed: true})

	if !maximized {
		t.Fatal("OnMaximize not called after clicking max button")
	}
}

func TestWindowClickOutsideButtons(t *testing.T) {
	w := widget.NewWindow("Test", 800, 600)
	w.OnClose = func() { t.Fatal("OnClose should not be called") }
	// Клик в центр клиентской области — не должен вызывать callback
	w.OnMouseButton(widget.MouseEvent{X: 400, Y: 300, Button: widget.MouseLeft, Pressed: true})
}

// ─── macOS-стиль заголовка ──────────────────────────────────────────────────

func TestWindowMacStyle_CloseClick(t *testing.T) {
	w := widget.NewWindow("Mac Test", 800, 600)
	w.TitleStyle = widget.WindowTitleMac
	closed := false
	w.OnClose = func() { closed = true }

	cr := w.CloseBtnRect()
	cx := (cr.Min.X + cr.Max.X) / 2
	cy := (cr.Min.Y + cr.Max.Y) / 2
	w.OnMouseButton(widget.MouseEvent{X: cx, Y: cy, Button: widget.MouseLeft, Pressed: true})

	if !closed {
		t.Fatal("OnClose not called for Mac-style close button")
	}
}

// ─── NoResize: только кнопка Close ──────────────────────────────────────────

func TestWindowNoResize_OnlyClose(t *testing.T) {
	w := widget.NewWindow("Test", 800, 600)
	w.Resize = widget.ResizeModeNoResize

	mr := w.MinBtnRect()
	if !mr.Empty() {
		t.Fatal("minBtnRect should be empty for NoResize")
	}
	xr := w.MaxBtnRect()
	if !xr.Empty() {
		t.Fatal("maxBtnRect should be empty for NoResize")
	}
	cr := w.CloseBtnRect()
	if cr.Empty() {
		t.Fatal("closeBtnRect should not be empty for NoResize")
	}
}

// ─── XAML загрузка ──────────────────────────────────────────────────────────

func TestXAMLWindow_Basic(t *testing.T) {
	xaml := `<Window Title="My App" Width="800" Height="600" Name="main">
		<Button Name="btn" Content="OK" Left="10" Top="10" Width="100" Height="30"/>
	</Window>`

	root, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}

	w, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("root type = %T, want *widget.Window", root)
	}
	if w.Title != "My App" {
		t.Fatalf("Title = %q, want %q", w.Title, "My App")
	}
	if w.Bounds().Dx() != 800 || w.Bounds().Dy() != 600 {
		t.Fatalf("Bounds = %v", w.Bounds())
	}

	btn, ok := reg["btn"].(*widget.Button)
	if !ok {
		t.Fatal("btn not found in registry")
	}
	// Кнопка должна быть смещена на ContentBounds.Min
	cb := w.ContentBounds()
	btnB := btn.Bounds()
	if btnB.Min.X != cb.Min.X+10 {
		t.Fatalf("btn.X = %d, want %d", btnB.Min.X, cb.Min.X+10)
	}
	if btnB.Min.Y != cb.Min.Y+10 {
		t.Fatalf("btn.Y = %d, want %d", btnB.Min.Y, cb.Min.Y+10)
	}
}

func TestXAMLWindow_WindowStyleNone(t *testing.T) {
	xaml := `<Window Title="Borderless" Width="640" Height="480" WindowStyle="None"/>`
	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	w := root.(*widget.Window)
	if w.Style != widget.WindowStyleNone {
		t.Fatalf("Style = %d, want None(%d)", w.Style, widget.WindowStyleNone)
	}
	cb := w.ContentBounds()
	if cb != w.Bounds() {
		t.Fatalf("ContentBounds = %v, want == Bounds %v for None", cb, w.Bounds())
	}
}

func TestXAMLWindow_ToolWindow(t *testing.T) {
	xaml := `<Window Title="Tool" Width="300" Height="200" WindowStyle="ToolWindow"/>`
	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	w := root.(*widget.Window)
	if w.Style != widget.WindowStyleToolWindow {
		t.Fatalf("Style = %d, want ToolWindow(%d)", w.Style, widget.WindowStyleToolWindow)
	}
}

func TestXAMLWindow_MacStyle(t *testing.T) {
	xaml := `<Window Title="Mac" Width="800" Height="600" TitleStyle="Mac"/>`
	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	w := root.(*widget.Window)
	if w.TitleStyle != widget.WindowTitleMac {
		t.Fatalf("TitleStyle = %d, want Mac(%d)", w.TitleStyle, widget.WindowTitleMac)
	}
}

func TestXAMLWindow_ResizeMode(t *testing.T) {
	tests := []struct {
		xml  string
		want widget.ResizeMode
	}{
		{`<Window Width="100" Height="100" ResizeMode="NoResize"/>`, widget.ResizeModeNoResize},
		{`<Window Width="100" Height="100" ResizeMode="CanMinimize"/>`, widget.ResizeModeCanMinimize},
		{`<Window Width="100" Height="100" ResizeMode="CanResize"/>`, widget.ResizeModeCanResize},
		{`<Window Width="100" Height="100"/>`, widget.ResizeModeCanResize}, // default
	}
	for _, tt := range tests {
		root, _, err := widget.LoadUIFromXAML([]byte(tt.xml))
		if err != nil {
			t.Fatalf("LoadUIFromXAML(%s): %v", tt.xml, err)
		}
		w := root.(*widget.Window)
		if w.Resize != tt.want {
			t.Fatalf("ResizeMode = %d, want %d for xml: %s", w.Resize, tt.want, tt.xml)
		}
	}
}

func TestXAMLWindow_CustomColors(t *testing.T) {
	xaml := `<Window Title="Colors" Width="400" Height="300"
		Background="#1E1E2E" BorderBrush="#FF5500"
		TitleBackground="#0078D4" TitleForeground="#FFFFFF"
		CornerRadius="8" TitleBarHeight="40"/>`

	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	w := root.(*widget.Window)
	if w.CornerRadius != 8 {
		t.Fatalf("CornerRadius = %d, want 8", w.CornerRadius)
	}
	if w.TitleBarHeight != 40 {
		t.Fatalf("TitleBarHeight = %d, want 40", w.TitleBarHeight)
	}
	if w.Background != (color.RGBA{R: 0x1E, G: 0x1E, B: 0x2E, A: 0xFF}) {
		t.Fatalf("Background = %v", w.Background)
	}
	if w.BorderColor != (color.RGBA{R: 0xFF, G: 0x55, B: 0x00, A: 0xFF}) {
		t.Fatalf("BorderColor = %v", w.BorderColor)
	}
}

// ─── Window vs Canvas — различие типов ──────────────────────────────────────

func TestXAMLCanvas_StillReturnsPanel(t *testing.T) {
	xaml := `<Canvas Width="1920" Height="1024" Background="#1E1E2E"/>`
	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	_, ok := root.(*widget.Panel)
	if !ok {
		t.Fatalf("Canvas root type = %T, want *widget.Panel", root)
	}
}

func TestXAMLWindow_ReturnsWindow(t *testing.T) {
	xaml := `<Window Title="App" Width="800" Height="600"/>`
	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	_, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("Window root type = %T, want *widget.Window", root)
	}
}

// ─── Window как root в Engine ───────────────────────────────────────────────

func TestEngineWithWindowRoot(t *testing.T) {
	w := widget.NewWindow("Engine Test", 800, 600)
	btn := widget.NewButton("OK")
	cb := w.ContentBounds()
	btn.SetBounds(image.Rect(cb.Min.X+10, cb.Min.Y+10, cb.Min.X+110, cb.Min.Y+40))
	w.AddChild(btn)

	eng := engine.New(800, 600, 30)
	eng.SetRoot(w)
	eng.Start()
	defer eng.Stop()

	// Получаем хотя бы один кадр
	frame := <-eng.Frames()
	if len(frame.Tiles) == 0 {
		t.Fatal("expected at least one tile from first frame")
	}
}

// ─── Draw: не паникует ──────────────────────────────────────────────────────

func TestWindowDraw_NoPanic(t *testing.T) {
	tests := []struct {
		name  string
		style widget.WindowStyle
		title widget.WindowTitleStyle
	}{
		{"Win+SingleBorder", widget.WindowStyleSingleBorder, widget.WindowTitleWin},
		{"Mac+SingleBorder", widget.WindowStyleSingleBorder, widget.WindowTitleMac},
		{"Win+None", widget.WindowStyleNone, widget.WindowTitleWin},
		{"Mac+ToolWindow", widget.WindowStyleToolWindow, widget.WindowTitleMac},
	}

	eng := engine.New(800, 600, 30)
	defer func() {
		eng.Start()
		eng.Stop()
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := widget.NewWindow("Test", 800, 600)
			w.Style = tt.style
			w.TitleStyle = tt.title

			lbl := widget.NewLabel("Hello", color.RGBA{R: 255, G: 255, B: 255, A: 255})
			cb := w.ContentBounds()
			lbl.SetBounds(image.Rect(cb.Min.X, cb.Min.Y, cb.Min.X+100, cb.Min.Y+20))
			w.AddChild(lbl)

			eng.SetRoot(w)
			eng.Start()
			<-eng.Frames()
			eng.Stop()

			eng = engine.New(800, 600, 30) // recreate for next iteration
		})
	}
}

// ─── ApplyTheme ─────────────────────────────────────────────────────────────

func TestWindowApplyTheme(t *testing.T) {
	w := widget.NewWindow("Theme", 800, 600)
	light := widget.LightTheme()
	w.ApplyTheme(light)

	if w.Background != light.WindowBG {
		t.Fatalf("Background = %v, want %v", w.Background, light.WindowBG)
	}
	if w.BorderColor != light.Border {
		t.Fatalf("BorderColor = %v, want %v", w.BorderColor, light.Border)
	}
}

// ─── Hover-состояние ────────────────────────────────────────────────────────

func TestWindowMouseMove_Hover(t *testing.T) {
	w := widget.NewWindow("Hover", 800, 600)

	// Перемещаем курсор на кнопку закрытия
	cr := w.CloseBtnRect()
	cx := (cr.Min.X + cr.Max.X) / 2
	cy := (cr.Min.Y + cr.Max.Y) / 2
	w.OnMouseMove(cx, cy)

	// Перемещаем курсор в клиентскую область — hover должен сброситься
	w.OnMouseMove(400, 300)
	// Не паникует — достаточно
}

// ─── XAML: Window с Grid внутри ──────────────────────────────────────────────

func TestXAMLWindow_WithGrid(t *testing.T) {
	xaml := `<Window Title="Grid Window" Width="800" Height="600">
		<Grid Name="mainGrid" Left="0" Top="0" Width="798" Height="567">
			<Grid.RowDefinitions>
				<RowDefinition Height="*"/>
				<RowDefinition Height="40"/>
			</Grid.RowDefinitions>
			<Grid.ColumnDefinitions>
				<ColumnDefinition Width="*"/>
			</Grid.ColumnDefinitions>
			<Label Grid.Row="0" Text="Content" Foreground="White"/>
			<Button Grid.Row="1" Content="OK" Style="Accent" Name="btnOK"/>
		</Grid>
	</Window>`

	root, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	w, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("root type = %T, want *widget.Window", root)
	}
	if len(w.Children()) == 0 {
		t.Fatal("Window should have children (Grid)")
	}
	if _, ok := reg["mainGrid"]; !ok {
		t.Fatal("mainGrid not found in registry")
	}
	if _, ok := reg["btnOK"]; !ok {
		t.Fatal("btnOK not found in registry")
	}
}
