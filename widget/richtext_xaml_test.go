package widget

import (
	"image"
	"image/color"
	"testing"
)

const richXAML = `<Canvas Width="400" Height="300">
  <RichText x:Name="doc" Left="0" Top="0" Width="400" Height="300" FontSize="11" Padding="8,6"
            Foreground="#102030" LinkForeground="#0000FF">
    <Paragraph FontSize="20" FontWeight="Bold" Margin="2,0,0,6">Title</Paragraph>
    <Paragraph TextAlignment="Center">
      <Run Text="plain, "/>
      <Run Text="red" Foreground="#FF0000" FontStyle="Italic"/>
      <Bold><Run Text=" bold-run"/></Bold>
      <Hyperlink NavigateUri="https://example.com">site</Hyperlink>
      <LineBreak/>
      <Run Text="gone" TextDecorations="Strikethrough"/>
    </Paragraph>
  </RichText>
</Canvas>`

func TestRichText_XAML(t *testing.T) {
	root, reg, err := LoadUIFromXAML([]byte(richXAML))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	defer ReleaseXAML(root)
	rt, ok := reg["doc"].(*RichText)
	if !ok {
		t.Fatalf("doc — %T, ждали *RichText", reg["doc"])
	}
	if rt.FontSize != 11 || rt.PaddingX != 8 || rt.PaddingY != 6 {
		t.Errorf("кегль %v, отступы %d,%d", rt.FontSize, rt.PaddingX, rt.PaddingY)
	}
	if rt.TextColor != (color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 255}) {
		t.Errorf("Foreground %v", rt.TextColor)
	}
	if rt.LinkColor != (color.RGBA{B: 255, A: 255}) {
		t.Errorf("LinkForeground %v", rt.LinkColor)
	}
	// Абзацы не стали детьми-виджетами.
	if n := len(rt.Children()); n != 0 {
		t.Errorf("у RichText %d детей-виджетов", n)
	}

	ps := rt.Paragraphs()
	if len(ps) != 2 {
		t.Fatalf("абзацев %d, ждали 2", len(ps))
	}
	h := ps[0]
	if len(h.Runs) != 1 || h.Runs[0].Text != "Title" || h.Runs[0].Size != 20 ||
		h.Runs[0].Font != BuiltinFontBold {
		t.Errorf("заголовок: %+v", h.Runs)
	}
	if h.Indent != 2 || h.SpaceBefore != 0 || h.SpaceAfter != 6 {
		t.Errorf("поля заголовка: %d/%d/%d", h.Indent, h.SpaceBefore, h.SpaceAfter)
	}

	p := ps[1]
	if p.Align != TextAlignCenter {
		t.Errorf("выравнивание %v", p.Align)
	}
	want := []RichRun{
		{Text: "plain, "},
		{Text: "red", Color: color.RGBA{R: 255, A: 255}, Font: BuiltinFontItalic},
		{Text: " bold-run", Font: BuiltinFontBold},
		{Text: "site", Link: "https://example.com"},
		{Text: "\n"},
		{Text: "gone", Strike: true},
	}
	if len(p.Runs) != len(want) {
		t.Fatalf("ранов %d, ждали %d: %+v", len(p.Runs), len(want), p.Runs)
	}
	for i := range want {
		if p.Runs[i] != want[i] {
			t.Errorf("ран %d: %+v, ждали %+v", i, p.Runs[i], want[i])
		}
	}

	// Построенный виджет рисуется и раскладывается.
	rt.SetBounds(image.Rect(0, 0, 400, 300))
	if rt.ContentHeight() <= 0 {
		t.Error("пустая высота содержимого")
	}
}

func TestRichText_XAMLPlainTextAndInheritance(t *testing.T) {
	root, reg, err := LoadUIFromXAML([]byte(`<Canvas Width="100" Height="100">
	  <RichText x:Name="a" Text="one" Width="100" Height="50"/>
	  <RichText x:Name="b" Width="100" Height="50">
	    <Paragraph FontWeight="Bold" Foreground="#00FF00">
	      <Run Text="inherits"/>
	      <Run Text="resets" FontWeight="Normal"/>
	      <Span FontFamily="Mono"><Run Text="nested"/></Span>
	    </Paragraph>
	  </RichText>
	</Canvas>`))
	if err != nil {
		t.Fatal(err)
	}
	defer ReleaseXAML(root)
	if got := reg["a"].(*RichText).Text(); got != "one" {
		t.Errorf("Text=%q", got)
	}
	runs := reg["b"].(*RichText).Paragraphs()[0].Runs
	if len(runs) != 3 {
		t.Fatalf("ранов %d", len(runs))
	}
	green := color.RGBA{G: 255, A: 255}
	if runs[0].Font != BuiltinFontBold || runs[0].Color != green {
		t.Errorf("ран не унаследовал оформление абзаца: %+v", runs[0])
	}
	if runs[1].Font != "" || runs[1].Color != green {
		t.Errorf("FontWeight=Normal не сбросил жирность: %+v", runs[1])
	}
	// Явный FontFamily важнее жирности абзаца: начертание не синтезируется.
	if runs[2].Font != "Mono" {
		t.Errorf("вложенный Span: шрифт %q", runs[2].Font)
	}
}
