package widget

import (
	"image"
	"testing"
)

// Элемент разметки без Width/Height раньше получал прямоугольник
// Rect(Left, Top, 0, 0): image.Rect нормализует границы, и подпись с
// Canvas.Left="24" Canvas.Top="264" жила в коробке 24×264 — ширина равнялась
// собственной координате. Текст рисовался за её пределами: при полной
// перерисовке был виден целиком, а при частичной обрезался и пропадал.

const labelSizeXAML = `<Canvas Width="800" Height="400" Background="#101010">
  <TextBlock Name="head" Left="24" Top="264" Text="Длинный заголовок раздела витрины"/>
  <TextBlock Name="fixed" Left="24" Top="40" Width="300" Height="36" Text="С заданным размером"/>
</Canvas>`

func TestXAMLCanvas_LabelWithoutSize_GetsMeasuredBox(t *testing.T) {
	useTestMeasurer(t)

	root, reg, err := LoadUIFromXAML([]byte(labelSizeXAML))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	defer ReleaseXAML(root)
	root.SetBounds(image.Rect(0, 0, 800, 400))

	head, ok := reg["head"].(*Label)
	if !ok {
		t.Fatalf("head — %T, ждал *Label", reg["head"])
	}
	b := head.Bounds()

	// Позиция — та, что задана разметкой.
	if b.Min.X != 24 || b.Min.Y != 264 {
		t.Errorf("подпись начинается в (%d,%d), ждал (24,264)", b.Min.X, b.Min.Y)
	}
	// Коробка вмещает текст: иначе он рисуется за её пределами.
	wantW := rawMeasure(head.Text(), DefaultFontSizePt)
	if wantW <= 0 {
		t.Fatalf("измеритель вернул %d — тест бесполезен", wantW)
	}
	if b.Dx() < wantW {
		t.Errorf("ширина коробки %d, текст занимает %d — подпись обрежется", b.Dx(), wantW)
	}
	if b.Dy() < int(DefaultFontSizePt) {
		t.Errorf("высота коробки %d — строка туда не помещается", b.Dy())
	}

	// Явно заданный размер разметка по-прежнему уважает.
	fixed := reg["fixed"].(*Label)
	if fb := fixed.Bounds(); fb.Dx() != 300 || fb.Dy() != 36 {
		t.Errorf("подпись с Width/Height получила %v, ждал 300×36", fb)
	}
}

// Тот же перекос ломал и сдвиг: подпись, у которой ширина равна Left, при
// переезде Canvas уезжала вместе с ним, но её коробка оставалась чужой.
func TestXAMLCanvas_LabelBoxFollowsCanvas(t *testing.T) {
	useTestMeasurer(t)

	root, reg, err := LoadUIFromXAML([]byte(labelSizeXAML))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	defer ReleaseXAML(root)
	root.SetBounds(image.Rect(0, 0, 800, 400))
	head := reg["head"].(*Label)
	before := head.Bounds().Dx()

	root.SetBounds(image.Rect(100, 50, 900, 450))
	after := head.Bounds()
	if after.Min.X != 124 || after.Min.Y != 314 {
		t.Errorf("после переезда подпись в (%d,%d), ждал (124,314)", after.Min.X, after.Min.Y)
	}
	if after.Dx() != before {
		t.Errorf("ширина коробки изменилась при переезде: было %d, стало %d", before, after.Dx())
	}
}
