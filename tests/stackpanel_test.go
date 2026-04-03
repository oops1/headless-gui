// Package tests — тесты StackPanel и XAML-загрузки StackPanel.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── StackPanel unit ────────────────────────────────────────────────────────

func TestStackPanel_NewDefaults(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationVertical)
	if sp.Orientation != widget.OrientationVertical {
		t.Fatalf("expected Vertical, got %d", sp.Orientation)
	}
	if sp.Spacing != 0 {
		t.Fatalf("expected Spacing=0, got %d", sp.Spacing)
	}
}

func TestStackPanel_HorizontalLayout(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationHorizontal)
	sp.Spacing = 4
	sp.Padding = 5
	sp.SetBounds(image.Rect(0, 0, 400, 40))

	b1 := widget.NewButton("A")
	b1.SetBounds(image.Rect(0, 0, 80, 28))
	b2 := widget.NewButton("B")
	b2.SetBounds(image.Rect(0, 0, 60, 28))

	sp.AddChild(b1)
	sp.AddChild(b2)

	// b1 should start at padding offset
	r1 := b1.Bounds()
	if r1.Min.X != 5 {
		t.Fatalf("b1.Min.X = %d, want 5 (padding)", r1.Min.X)
	}
	if r1.Dx() != 80 {
		t.Fatalf("b1.Dx() = %d, want 80", r1.Dx())
	}

	// b2 should follow b1 with spacing
	r2 := b2.Bounds()
	expectedX := 5 + 80 + 4 // padding + b1.width + spacing
	if r2.Min.X != expectedX {
		t.Fatalf("b2.Min.X = %d, want %d", r2.Min.X, expectedX)
	}
}

func TestStackPanel_VerticalLayout(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationVertical)
	sp.Spacing = 2
	sp.SetBounds(image.Rect(10, 10, 200, 300))

	b1 := widget.NewButton("A")
	b1.SetBounds(image.Rect(0, 0, 0, 30)) // width=0 → fill parent
	b2 := widget.NewButton("B")
	b2.SetBounds(image.Rect(0, 0, 0, 30))

	sp.AddChild(b1)
	sp.AddChild(b2)

	r1 := b1.Bounds()
	if r1.Min.Y != 10 {
		t.Fatalf("b1.Min.Y = %d, want 10", r1.Min.Y)
	}
	if r1.Dy() != 30 {
		t.Fatalf("b1.Dy() = %d, want 30", r1.Dy())
	}

	r2 := b2.Bounds()
	expectedY := 10 + 30 + 2 // parent.Min.Y + b1.height + spacing
	if r2.Min.Y != expectedY {
		t.Fatalf("b2.Min.Y = %d, want %d", r2.Min.Y, expectedY)
	}
}

func TestStackPanel_DrawNoPanic(t *testing.T) {
	sp := widget.NewStackPanel(widget.OrientationHorizontal)
	sp.SetBounds(image.Rect(0, 0, 200, 40))
	b := widget.NewButton("OK")
	b.SetBounds(image.Rect(0, 0, 80, 28))
	sp.AddChild(b)

	// Используем Engine для рендеринга — Draw не должен паниковать.
	eng := engine.New(200, 40, 30)
	eng.SetRoot(sp)
	eng.Start()
	<-eng.Frames()
	eng.Stop()
}

// ─── StackPanel XAML ────────────────────────────────────────────────────────

func TestStackPanel_XAML_Horizontal(t *testing.T) {
	xaml := []byte(`
<Panel Width="600" Height="400" Background="Transparent">
    <StackPanel Orientation="Horizontal" Background="#2D2D30"
                Spacing="4" Padding="5" Name="sp"
                Left="0" Top="0" Width="400" Height="40">
        <Button Content="OK"     Width="80" Height="28" Name="btnOK"/>
        <Button Content="Cancel" Width="80" Height="28" Name="btnCancel"/>
    </StackPanel>
</Panel>`)

	root, reg, err := widget.LoadUIFromXAML(xaml)
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	if root == nil {
		t.Fatal("root is nil")
	}

	sp, ok := reg["sp"].(*widget.StackPanel)
	if !ok {
		t.Fatalf("sp not found or wrong type: %T", reg["sp"])
	}
	if sp.Orientation != widget.OrientationHorizontal {
		t.Fatal("expected Horizontal")
	}
	if sp.Spacing != 4 {
		t.Fatalf("Spacing = %d, want 4", sp.Spacing)
	}

	// Buttons should exist
	if _, ok := reg["btnOK"].(*widget.Button); !ok {
		t.Fatal("btnOK not found")
	}
	if _, ok := reg["btnCancel"].(*widget.Button); !ok {
		t.Fatal("btnCancel not found")
	}
}

func TestStackPanel_XAML_Vertical(t *testing.T) {
	xaml := []byte(`
<Panel Width="200" Height="300" Background="Transparent">
    <StackPanel Orientation="Vertical" Name="sp"
                Left="0" Top="0" Width="200" Height="300">
        <Label Text="First" Width="180" Height="20"/>
        <Label Text="Second" Width="180" Height="20"/>
    </StackPanel>
</Panel>`)

	_, reg, err := widget.LoadUIFromXAML(xaml)
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}

	sp, ok := reg["sp"].(*widget.StackPanel)
	if !ok {
		t.Fatalf("sp not found or wrong type: %T", reg["sp"])
	}
	if sp.Orientation != widget.OrientationVertical {
		t.Fatal("expected Vertical")
	}
	if len(sp.Children()) != 2 {
		t.Fatalf("expected 2 children, got %d", len(sp.Children()))
	}
}

func TestStackPanel_XAML_DefaultVertical(t *testing.T) {
	xaml := []byte(`
<Panel Width="200" Height="300" Background="Transparent">
    <StackPanel Name="sp" Left="0" Top="0" Width="200" Height="300">
        <Button Content="A" Width="80" Height="28"/>
    </StackPanel>
</Panel>`)

	_, reg, err := widget.LoadUIFromXAML(xaml)
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}

	sp := reg["sp"].(*widget.StackPanel)
	if sp.Orientation != widget.OrientationVertical {
		t.Fatal("expected Vertical by default")
	}
}
