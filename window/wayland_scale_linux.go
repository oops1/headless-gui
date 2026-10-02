//go:build linux && !android

package window

// wayland_scale_linux.go — HiDPI на Wayland: масштаб поверхности.
//
// Движок всегда рисовал окно один к одному: на мониторе с удвоением всё
// выходило вдвое мельче положенного, и единственным средством была
// переменная HEADLESS_GUI_SCALE, то есть человек должен был знать о ней.
//
// На Wayland размеры окна (xdg_toplevel.configure, set_min_size) считаются в
// «поверхностных» единицах, а буфер клиент присылает в своих пикселях. Как
// связаны эти две сетки, говорит масштаб, и узнать его можно двумя путями:
//
//   - wl_output.scale — целое удвоение (или утроение) монитора, на котором
//     оказалось окно (о попадании сообщает wl_surface.enter);
//   - wp_fractional_scale_v1.preferred_scale — дробный масштаб (1.25, 1.5),
//     который компоновщик СЧИТАЕТ правильным для этой поверхности. Он
//     точнее: целым масштабом дробный не выразить, и клиенту приходилось бы
//     рисовать крупнее и позволять компоновщику ужимать картинку.
//
// При дробном масштабе буфер остаётся в своих пикселях, а wp_viewport
// говорит компоновщику, какого размера эта картинка в поверхностных
// единицах. При целом — хватает wl_surface.set_buffer_scale.

import (
	"image"
	"sync"
)

const (
	// wl_output
	wlOutputEvScale = 3

	// wl_surface события
	wlSurfaceEvEnter = 0

	// wl_surface.set_buffer_scale
	wlSurfaceSetBufferScale = 8
	// wl_surface.damage_buffer — повреждение в пикселях БУФЕРА, а не
	// поверхности. При масштабе ≠ 1 это единственный честный способ
	// сказать, что изменилось: wl_surface.damage считает в поверхностных
	// единицах, и на дробном масштабе область уезжала бы на полпикселя.
	wlSurfaceDamageBuffer = 9

	// wp_viewporter / wp_viewport
	wpViewporterGetViewport  = 1
	wpViewportSetDestination = 2

	// wp_fractional_scale_manager_v1 / wp_fractional_scale_v1
	wpFracMgrGetFractionalScale = 1
	wpFracEvPreferredScale      = 0

	// Знаменатель дробного масштаба: протокол шлёт его числителем 120-х
	// долей (120 — ровно 1.0, 180 — 1.5).
	wlFracScaleDenom = 120

	// Версия wl_compositor, с которой есть damage_buffer.
	wlCompositorVersionBufferDamage = 4
	// Версия wl_output, с которой приходит scale.
	wlOutputVersionScale = 2
)

// wlScale — масштаб поверхности окна.
type wlScale struct {
	mu sync.Mutex

	// outputs — масштаб каждого известного монитора, entered — тот, на
	// котором сейчас окно (о попадании сообщает wl_surface.enter).
	outputs map[uint32]float64
	entered uint32

	// preferred — дробный масштаб от компоновщика (0 — расширения нет).
	preferred float64

	// current — применённый масштаб; 1, пока ничего не известно.
	current float64

	viewportID uint32 // wp_viewport поверхности (0 — расширения нет)
	fracID     uint32 // wp_fractional_scale_v1 (0 — расширения нет)

	onChange func(k float64)
}

// factor — текущий масштаб поверхности (всегда ≥ 1 по смыслу, но не меньше
// 0.5: границы те же, что у HEADLESS_GUI_SCALE).
func (w *WaylandWindow) scaleFactor() float64 {
	w.scale.mu.Lock()
	defer w.scale.mu.Unlock()
	if w.scale.current <= 0 {
		return 1
	}
	return w.scale.current
}

// SetOnDpiChanged подключает уведомление о смене масштаба. Реализует
// dpiChangeNotifier — тот же путь, которым на Windows приходит перенос окна
// на монитор с другим DPI.
func (w *WaylandWindow) SetOnDpiChanged(fn func(scale float64)) {
	w.scale.mu.Lock()
	w.scale.onChange = fn
	w.scale.mu.Unlock()
}

// toSurface переводит размер из пикселей буфера в поверхностные единицы —
// те, в которых считает xdg-shell (configure, set_min_size, set_max_size).
func (w *WaylandWindow) toSurface(v int) int {
	k := w.scaleFactor()
	if k == 1 {
		return v
	}
	return int(float64(v)/k + 0.5)
}

// fromSurface — обратный перевод: поверхностные единицы в пиксели буфера.
// В них движок получает размер окна и рисует кадр.
func (w *WaylandWindow) fromSurface(v int) int {
	k := w.scaleFactor()
	if k == 1 {
		return v
	}
	return int(float64(v)*k + 0.5)
}

// noteOutputScale запоминает масштаб монитора и, если окно на нём, применяет.
func (w *WaylandWindow) noteOutputScale(output uint32, k float64) {
	w.scale.mu.Lock()
	if w.scale.outputs == nil {
		w.scale.outputs = map[uint32]float64{}
	}
	w.scale.outputs[output] = k
	apply := w.scale.entered == output
	w.scale.mu.Unlock()
	if apply {
		w.recomputeScale()
	}
}

// noteSurfaceEnter запоминает, на какой монитор попало окно.
func (w *WaylandWindow) noteSurfaceEnter(output uint32) {
	w.scale.mu.Lock()
	w.scale.entered = output
	w.scale.mu.Unlock()
	w.recomputeScale()
}

// notePreferredScale принимает дробный масштаб от компоновщика.
func (w *WaylandWindow) notePreferredScale(scale120 uint32) {
	if scale120 == 0 {
		return
	}
	w.scale.mu.Lock()
	w.scale.preferred = float64(scale120) / wlFracScaleDenom
	w.scale.mu.Unlock()
	w.recomputeScale()
}

// recomputeScale выбирает масштаб и, если он изменился, применяет его к
// поверхности и сообщает окну.
//
// Дробный масштаб главнее целого: он и точнее, и это прямое указание
// компоновщика для ЭТОЙ поверхности, а не свойство монитора.
func (w *WaylandWindow) recomputeScale() {
	w.scale.mu.Lock()
	k := w.scale.preferred
	if k <= 0 {
		k = w.scale.outputs[w.scale.entered]
	}
	if k <= 0 {
		k = 1
	}
	k = clampScale(k)
	if k == w.scale.current || (w.scale.current == 0 && k == 1) {
		w.scale.current = k
		w.scale.mu.Unlock()
		return
	}
	w.scale.current = k
	viewport, frac := w.scale.viewportID, w.scale.fracID
	onChange := w.scale.onChange
	w.scale.mu.Unlock()

	// Целый масштаб без дробного расширения компоновщик понимает сам; при
	// дробном буферный масштаб обязан остаться единицей, а размер картинки
	// в поверхностных единицах задаётся областью просмотра.
	if frac == 0 && viewport == 0 && k == float64(int(k)) {
		w.send(newWlMsg(w.surfaceID, wlSurfaceSetBufferScale).putInt(int32(k)), -1)
	}
	w.applyViewport()

	wlLog("масштаб поверхности: %v", k)
	if onChange != nil {
		onChange(k)
	}
}

// applyViewport сообщает компоновщику, какого размера кадр в поверхностных
// единицах. Без этого при дробном масштабе картинка заняла бы столько
// поверхностных единиц, сколько в ней пикселей, — то есть окно стало бы
// крупнее экрана.
func (w *WaylandWindow) applyViewport() {
	w.scale.mu.Lock()
	viewport := w.scale.viewportID
	w.scale.mu.Unlock()
	if viewport == 0 {
		return
	}
	sw, sh := w.toSurface(w.width), w.toSurface(w.height)
	if sw <= 0 || sh <= 0 {
		return
	}
	w.send(newWlMsg(viewport, wpViewportSetDestination).
		putInt(int32(sw)).putInt(int32(sh)), -1)
}

// setupScaleObjects создаёт объекты масштабирования для поверхности окна.
// Зовётся из Create сразу после wl_surface.
func (w *WaylandWindow) setupScaleObjects() {
	if w.viewporterID != 0 {
		id := w.newID()
		w.send(newWlMsg(w.viewporterID, wpViewporterGetViewport).
			putUint(id).putUint(w.surfaceID), -1)
		w.scale.mu.Lock()
		w.scale.viewportID = id
		w.scale.mu.Unlock()
	}
	if w.fracMgrID != 0 {
		id := w.newID()
		w.send(newWlMsg(w.fracMgrID, wpFracMgrGetFractionalScale).
			putUint(id).putUint(w.surfaceID), -1)
		w.scale.mu.Lock()
		w.scale.fracID = id
		w.scale.mu.Unlock()
	}
}

// isFracObject — событие пришло объекту дробного масштаба этой поверхности.
func (w *WaylandWindow) isFracObject(obj uint32) bool {
	w.scale.mu.Lock()
	defer w.scale.mu.Unlock()
	return obj != 0 && obj == w.scale.fracID
}

// isOutputObject — событие пришло одному из мониторов.
func (w *WaylandWindow) isOutputObject(obj uint32) bool {
	w.scale.mu.Lock()
	defer w.scale.mu.Unlock()
	_, ok := w.scale.outputs[obj]
	return ok
}

// damageSurface помечает изменившуюся область кадра.
//
// Область приходит в пикселях буфера. Пока масштаб единица, это те же
// поверхностные единицы, и годится обычный wl_surface.damage. На HiDPI они
// расходятся: там нужен damage_buffer, который считает ровно в пикселях
// буфера. Если компоновщик слишком стар для него (wl_compositor младше
// четвёртой версии), область переводится в поверхностные единицы с запасом
// в точку — лучше перерисовать лишнее, чем оставить полоску старого кадра.
func (w *WaylandWindow) damageSurface(r image.Rectangle) {
	if r.Empty() {
		return
	}
	if w.scaleFactor() == 1 {
		w.send(newWlMsg(w.surfaceID, wlSurfaceDamage).
			putInt(int32(r.Min.X)).putInt(int32(r.Min.Y)).
			putInt(int32(r.Dx())).putInt(int32(r.Dy())), -1)
		return
	}
	if w.gCompositorVer >= wlCompositorVersionBufferDamage {
		w.send(newWlMsg(w.surfaceID, wlSurfaceDamageBuffer).
			putInt(int32(r.Min.X)).putInt(int32(r.Min.Y)).
			putInt(int32(r.Dx())).putInt(int32(r.Dy())), -1)
		return
	}
	k := w.scaleFactor()
	x := int32(float64(r.Min.X)/k) - 1
	y := int32(float64(r.Min.Y)/k) - 1
	dx := int32(float64(r.Dx())/k) + 2
	dy := int32(float64(r.Dy())/k) + 2
	w.send(newWlMsg(w.surfaceID, wlSurfaceDamage).
		putInt(x).putInt(y).putInt(dx).putInt(dy), -1)
}
