package engine

// scalewithin.go — масштаб интерфейса при фиксированном физическом размере
// кадра (ADR-0002, задача 3.1; запрос WinLine W4).
//
// SetScale держит логический размер и меняет физический: окно 1280×800 при
// масштабе 1,5 рисуется в 1920×1200 — так работает HiDPI у окна ОС. Клиенту
// удалённого рабочего стола нужно наоборот: физический кадр задан
// подключением, 1920×1080, а интерфейс пользователь хочет крупнее. До сих
// пор «масштаб 100–200 %» в настройках WinLine был недоступен: движок на
// SetScale отдавал кадр не того размера.
//
// SetScaleWithin держит физический размер и выводит логический: при 1,5 на
// 1920×1080 сцена становится 1280×720 логических точек. Всё остальное —
// координаты событий, damage, попапы, внешние поверхности 1:1 — и так живёт
// в двух системах координат и ничего нового не узнаёт. Режим запоминается:
// SetScale в нём тоже держит физический размер; SetPhysicalResolution меняет
// размер кадра, не трогая масштаб; обычный SetResolution (логический размер)
// выходит из режима.

import (
	hgraster "github.com/oops1/headless-gui/v3/core/raster"
	"math"

	"github.com/oops1/headless-gui/v3/widget"
)

// SetScaleWithin задаёт масштаб интерфейса, сохраняя физический размер
// кадра: логический размер холста становится ⌈W/k⌉×⌈H/k⌉ (наружу, чтобы
// сцена накрыла кадр целиком). k ≤ 0 — 1. Холст пересоздаётся, и буферы
// SetSurface/SetSurfaces отдаются заново, как после SetResolution.
func (e *Engine) SetScaleWithin(k float64) {
	if k <= 0 {
		k = 1
	}
	e.frameMu.Lock()
	defer e.frameMu.Unlock()
	e.mu.Lock()
	pw, ph := e.canvas.W, e.canvas.H
	if e.scaleWithin && e.canvas.Scale() == k {
		e.mu.Unlock()
		return
	}
	lw, lh := logicalFor(pw, ph, k)
	e.canvas = e.canvas.CloneForPhys(pw, ph, lw, lh, k, e.bgSrc)
	e.canvasRef.Store(e.canvas)
	e.canvas.SetDPIAll(e.userDPI * k)
	e.scaleBits.Store(math.Float64bits(k))
	e.scaleWithin = true
	e.mu.Unlock()
	e.surfaces.drop() // вне e.mu: у набора свой замок (ADR-0011); frameMu удерживается
	widget.BumpTextMetricsRev() // ширины текста изменились — сброс кэшей переноса
	e.fitTreeToCanvas(lw, lh)
}

// SetPhysicalResolution задаёт физический размер кадра, сохраняя масштаб:
// логический размер выводится как в SetScaleWithin. Для потребителя, у
// которого размер кадра диктует клиент (смена разрешения по RDP), а масштаб —
// пользователь. Включает режим «масштаб внутри кадра».
func (e *Engine) SetPhysicalResolution(pw, ph int) {
	pw, ph = hgraster.ClampCanvasSize(pw, ph)
	e.frameMu.Lock()
	defer e.frameMu.Unlock()
	e.mu.Lock()
	k := e.canvas.Scale()
	lw, lh := logicalFor(pw, ph, k)
	e.canvas = e.canvas.CloneForPhys(pw, ph, lw, lh, k, e.bgSrc)
	e.canvasRef.Store(e.canvas)
	e.scaleWithin = true
	e.mu.Unlock()
	e.surfaces.drop() // вне e.mu: у набора свой замок (ADR-0011); frameMu удерживается
	e.fitTreeToCanvas(lw, lh)
}

// ScaleWithin сообщает, держит ли движок физический размер кадра при смене
// масштаба.
func (e *Engine) ScaleWithin() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.scaleWithin
}

// logicalFor — логический размер под физический pw×ph при масштабе k, с
// округлением наружу.
func logicalFor(pw, ph int, k float64) (int, int) {
	if k == 1 {
		return pw, ph
	}
	return hgraster.ClampCanvasSize(int(math.Ceil(float64(pw)/k)), int(math.Ceil(float64(ph)/k)))
}
