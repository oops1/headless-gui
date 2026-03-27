package widget

import (
	"image"
	"image/color"
	_ "image/jpeg" // поддержка JPEG
	_ "image/png"  // поддержка PNG
	"os"
	"sync"
)

// ImageWidget — виджет для отображения растрового изображения (PNG/JPEG).
//
// Source задаёт путь к файлу при создании через XAML (атрибут Source=).
// Изображение масштабируется к размеру bounds виджета (Stretch=Fill, по умолчанию).
//
// Использование в XAML:
//
//	<Image x:Name="logo" Canvas.Left="100" Canvas.Top="50"
//	       Width="200" Height="100" Source="assets/logo.png"/>
type ImageWidget struct {
	Base

	mu      sync.RWMutex
	img     image.Image
	Source  string // путь к файлу (только для чтения после SetSource)
	Stretch ImageStretch
	Fallback color.RGBA // цвет-заглушка если изображение не загружено
}

// ImageStretch задаёт способ масштабирования изображения.
type ImageStretch int

const (
	// ImageStretchFill — изображение растягивается под весь bounds (по умолчанию).
	ImageStretchFill ImageStretch = iota
	// ImageStretchUniform — сохраняет пропорции, вписывая в bounds с полями.
	ImageStretchUniform
	// ImageStretchNone — оригинальный размер, обрезается по bounds.
	ImageStretchNone
)

// NewImageWidget создаёт пустой виджет. Загрузите изображение через SetSource или SetImage.
func NewImageWidget() *ImageWidget {
	return &ImageWidget{
		Stretch:  ImageStretchFill,
		Fallback: color.RGBA{R: 40, G: 40, B: 44, A: 255},
	}
}

// SetSource загружает изображение из файла (PNG или JPEG).
// Потокобезопасно. Возвращает ошибку если файл недоступен или формат не поддерживается.
func (w *ImageWidget) SetSource(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.img = img
	w.Source = path
	w.mu.Unlock()
	return nil
}

// SetImage устанавливает уже загруженное изображение.
func (w *ImageWidget) SetImage(img image.Image) {
	w.mu.Lock()
	w.img = img
	w.mu.Unlock()
}

// Image возвращает текущее изображение (или nil).
func (w *ImageWidget) Image() image.Image {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.img
}

func (w *ImageWidget) Draw(ctx DrawContext) {
	b := w.bounds
	w.mu.RLock()
	img := w.img
	w.mu.RUnlock()

	if img == nil {
		// Заглушка
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), w.Fallback)
		w.drawChildren(ctx)
		return
	}

	ib := img.Bounds()

	switch w.Stretch {
	case ImageStretchFill:
		ctx.DrawImageScaled(img, b.Min.X, b.Min.Y, b.Dx(), b.Dy())

	case ImageStretchUniform:
		// Вписываем с сохранением пропорций
		sx := float64(b.Dx()) / float64(ib.Dx())
		sy := float64(b.Dy()) / float64(ib.Dy())
		scale := sx
		if sy < sx {
			scale = sy
		}
		dw := int(float64(ib.Dx()) * scale)
		dh := int(float64(ib.Dy()) * scale)
		dx := b.Min.X + (b.Dx()-dw)/2
		dy := b.Min.Y + (b.Dy()-dh)/2
		ctx.DrawImageScaled(img, dx, dy, dw, dh)

	case ImageStretchNone:
		ctx.DrawImage(img, b.Min.X, b.Min.Y)
	}

	w.drawChildren(ctx)
}
