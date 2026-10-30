package draw

// fontsize.go — кегль по умолчанию для текста без своего размера. Его читает
// холст (DrawText) и виджеты; кто его меняет и когда (приложение или тема) —
// политика пакета widget (widget.SetDefaultFontSize).

import (
	"math"
	"sync/atomic"
)

// defaultFontBits — текущий размер (биты float64); 0 — не задан, то есть
// DefaultFontSizePt. Читается из Draw и из измерений без замков.
var defaultFontBits atomic.Uint64

// DefaultFontSize возвращает кегль (pt) для текста, которому свой не задан:
// DefaultFontSizePt, пока не назначен другой.
func DefaultFontSize() float64 {
	if b := defaultFontBits.Load(); b != 0 {
		return math.Float64frombits(b)
	}
	return DefaultFontSizePt
}

// StoreDefaultFontSize ставит кегль по умолчанию; ноль, отрицательное и
// нечисловое значения снимают его (DefaultFontSizePt). Только хранилище:
// приоритеты приложения и темы решает widget.SetDefaultFontSize.
func StoreDefaultFontSize(pt float64) {
	if pt <= 0 || math.IsNaN(pt) || math.IsInf(pt, 0) {
		defaultFontBits.Store(0)
		return
	}
	defaultFontBits.Store(math.Float64bits(pt))
}

// DefaultFontSizeSet сообщает, назначен ли кегль явно.
func DefaultFontSizeSet() bool { return defaultFontBits.Load() != 0 }
