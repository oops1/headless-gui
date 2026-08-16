package widget

import (
	"image/color"
	"sync"

	"github.com/oops1/headless-gui/v3/widget/svg"
)

// SVGIcon — виджет темизируемой векторной иконки (подмножество SVG).
//
// Иконка загружается через SetSVG/SetSVGFile, растеризуется по размеру bounds
// с сохранением пропорций (центрируется) и рисуется через DrawContext.DrawImage.
//
// Перекраска:
//   - fill="currentColor" в SVG заменяется на текущий Color виджета;
//   - при Tint=true в Color перекрашивается ВЕСЬ контент (монохромный режим).
//
// Темизация: если цвет не задан явно (SetColor), ApplyTheme берёт цвет текста
// темы (Theme.LabelText) — иконка следует за темой «под текст».
//
// Использование в XAML (тег добавляется отдельно, см. отчёт):
//
//	<SVGIcon Source="assets/menu.svg" Color="#FF3366" Tint="True"/>
type SVGIcon struct {
	Base

	mu       sync.Mutex
	doc      *svg.Document
	color    color.RGBA
	explicit bool // цвет задан явно (SetColor) — ApplyTheme его не трогает
	tint     bool
	err      error
}

// NewSVGIcon создаёт пустой виджет иконки. Цвет по умолчанию — цвет текста
// текущей глобальной темы (следует за темой, пока не задан явно).
func NewSVGIcon() *SVGIcon {
	return &SVGIcon{color: win10.LabelText}
}

// NewSVGIconFromData создаёт виджет и сразу загружает SVG-данные.
func NewSVGIconFromData(data []byte) *SVGIcon {
	ic := NewSVGIcon()
	ic.SetSVG(data)
	return ic
}

// SetSVG загружает иконку из SVG-данных (лимиты svg.MaxFileBytes и svg.MaxDepth).
func (w *SVGIcon) SetSVG(data []byte) error {
	doc, err := svg.Parse(data)
	w.mu.Lock()
	w.doc = doc
	w.err = err
	w.mu.Unlock()
	w.Invalidate()
	return err
}

// SetSVGFile загружает иконку из файла.
func (w *SVGIcon) SetSVGFile(path string) error {
	doc, err := svg.ParseFile(path)
	w.mu.Lock()
	w.doc = doc
	w.err = err
	w.mu.Unlock()
	w.Invalidate()
	return err
}

// SetColor задаёт цвет иконки явно (перекрывает темизацию). Используется как
// значение currentColor и как цвет перекраски при Tint.
func (w *SVGIcon) SetColor(c color.RGBA) {
	w.mu.Lock()
	changed := w.color != c || !w.explicit
	w.color = c
	w.explicit = true
	w.mu.Unlock()
	if changed {
		w.Invalidate()
	}
}

// Color возвращает текущий цвет иконки.
func (w *SVGIcon) Color() color.RGBA {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.color
}

// SetTint включает/выключает монохромную перекраску всего контента в Color.
// При выключенном Tint перекрашивается только fill="currentColor".
func (w *SVGIcon) SetTint(on bool) {
	w.mu.Lock()
	changed := w.tint != on
	w.tint = on
	w.mu.Unlock()
	if changed {
		w.Invalidate()
	}
}

// Tint сообщает, включён ли монохромный режим перекраски.
func (w *SVGIcon) Tint() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tint
}

// Err возвращает ошибку последней загрузки SVG (или nil).
func (w *SVGIcon) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

// Document возвращает разобранный документ (или nil) — для тестов/интеграции.
func (w *SVGIcon) Document() *svg.Document {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.doc
}

// Draw растеризует иконку под размер bounds и рисует её через DrawImage.
func (w *SVGIcon) Draw(ctx DrawContext) {
	b := w.bounds
	if b.Empty() {
		return
	}
	w.mu.Lock()
	doc := w.doc
	col := w.color
	tint := w.tint
	w.mu.Unlock()
	if doc == nil {
		w.drawChildren(ctx)
		return
	}
	img := doc.RasterizeCached(b.Dx(), b.Dy(), col, tint)
	if img != nil {
		ctx.DrawImage(img, b.Min.X, b.Min.Y)
	}
	w.drawChildren(ctx)
}

// ApplyTheme применяет тему: если цвет не задан явно, берётся Theme.LabelText.
func (w *SVGIcon) ApplyTheme(t *Theme) {
	w.mu.Lock()
	if !w.explicit {
		changed := w.color != t.LabelText
		w.color = t.LabelText
		w.mu.Unlock()
		if changed {
			w.Invalidate()
		}
		return
	}
	w.mu.Unlock()
}
