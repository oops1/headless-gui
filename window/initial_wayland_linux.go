//go:build linux && !android

package window

// initial_wayland_linux.go — начальное положение и «развёрнуто» на Wayland
// (initial.go). Положение окна задаёт компоновщик, поэтому x, y из
// SetInitialBounds игнорируются; размер приходит аргументами Create.

// setInitialPlacement запоминает «развёрнуто» до Create. Реализует
// initialPlaceBackend.
func (w *WaylandWindow) setInitialPlacement(p initialPlacement) { w.initial = p }

// restoreNormalSize подменяет размер из configure размером, с которым окно
// создавалось, если оно начало жизнь развёрнутым и теперь возвращено в
// обычное состояние. Срабатывает один раз.
func (w *WaylandWindow) restoreNormalSize(nw, nh int, maximized, fullscreen bool) (int, int) {
	rw, rh, used := wlNormalSize(nw, nh, maximized, fullscreen, w.normW, w.normH)
	if used {
		w.normW, w.normH = 0, 0
	}
	return rw, rh
}
