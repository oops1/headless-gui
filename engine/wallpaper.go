package engine

import (
	"fmt"
	"image"
)

// SetWallpaperSource задаёт картинку, по которой считается материал Mica
// (theme.MaterialMica, MaterialMicaAlt), отдельно от фона движка.
//
// Без него Mica размывает фон, заданный SetBackground/SetBackgroundFile, —
// обычный случай: обои рабочего стола и есть фон движка. Источник нужен, когда
// обои рисует не движок, а виджет (корневая панель с изображением), или когда
// Mica должна быть тоньше/темнее самих обоев: фон движка при этом не трогается.
// Картинка растягивается на весь холст, как фон; не меняйте её после передачи.
// nil снимает источник — Mica снова берёт фон движка.
//
// Нет ни источника, ни фона — Mica нечего размывать, и слои рисуются сплошным
// цветом темы (BackdropSpec.Fallback). Смена источника перерисовывает кадр.
func (e *Engine) SetWallpaperSource(img image.Image) error {
	if img != nil {
		if b := img.Bounds(); b.Empty() {
			return fmt.Errorf("engine: SetWallpaperSource: изображение пустое %v", b)
		}
	}
	e.frameMu.Lock() // источник читается при отрисовке — меняем между кадрами
	e.mu.Lock()
	e.canvas.setWallpaperSource(img)
	e.mu.Unlock()
	e.frameMu.Unlock()
	e.Invalidate()
	return nil
}
