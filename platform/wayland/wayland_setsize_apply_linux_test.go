//go:build linux && !android

package wayland

import (
	"image"
	"testing"
)

// SetSize на Wayland, на проводе: плавающее окно само применяет размер
// (onResize, буфер нового размера, commit), независимо от resizable. Окно
// развёрнутое, полноэкранное или прилепленное размер не выбирает: заказ ждёт
// возврата в обычное состояние.

// wlSzWindow — окно 1400×860 с записью вызовов onResize.
func wlSzWindow(t *testing.T) (*wlTestConn, *[][2]int) {
	t.Helper()
	c := newWlTestWindow(t)
	c.w.width, c.w.height = 1400, 860
	var calls [][2]int
	c.w.onResize = func(w, h int) { calls = append(calls, [2]int{w, h}) }
	return c, &calls
}

// wlSzLimits — пары min/max, которые окно успело отправить.
func wlSzLimits(t *testing.T, msgs []wlWireMsg) (mins, maxs [][2]uint32) {
	t.Helper()
	for _, m := range msgs {
		if m.obj != 12 || len(m.args) < 2 {
			continue
		}
		switch m.opcode {
		case xdgToplevelSetMinSize:
			mins = append(mins, [2]uint32{m.args[0], m.args[1]})
		case xdgToplevelSetMaxSize:
			maxs = append(maxs, [2]uint32{m.args[0], m.args[1]})
		}
	}
	return
}

// wlSzCommits — сколько раз окно коммитило поверхность.
func wlSzCommits(msgs []wlWireMsg) int {
	n := 0
	for _, m := range msgs {
		if m.obj == 11 && m.opcode == wlSurfaceCommit {
			n++
		}
	}
	return n
}

// Сценарий отчёта Tandem: окно с запретом ресайза 1400×860 → SetSize(520,370):
// onResize один раз, GetSize 520×370, фиксация перенесена на новый размер,
// буфер нового размера закоммичен; configure с тем же размером второго onResize
// не даёт. Затем обратно.
func TestWayland_SetSizeApply_FixedWindowRoundTrip(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(false)

	c.w.SetSize(520, 370)
	if len(*calls) != 1 || (*calls)[0] != [2]int{520, 370} {
		t.Fatalf("onResize: %v, ждал один вызов 520x370", *calls)
	}
	if w, h := c.w.GetSize(); w != 520 || h != 370 {
		t.Errorf("GetSize = %dx%d, ждал 520x370", w, h)
	}
	mins, maxs := wlSzLimits(t, c.readAllWl(t))
	if len(mins) != 1 || mins[0] != [2]uint32{520, 370} || len(maxs) != 1 || maxs[0] != [2]uint32{520, 370} {
		t.Errorf("пределы: min %v max %v, ждал по 520x370", mins, maxs)
	}

	// Движок отвечает кадром нового размера — окно коммитит буфер под него.
	c.w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 520, 370)))
	if c.w.poolW != 520 || c.w.poolH != 370 || !c.w.hasFrame {
		t.Fatalf("буфер %dx%d (кадр %v), ждал закоммиченный 520x370", c.w.poolW, c.w.poolH, c.w.hasFrame)
	}
	if n := wlSzCommits(c.readAllWl(t)); n == 0 {
		t.Error("буфер нового размера не закоммичен")
	}

	// Компоновщик подтвердил тем же размером: второго onResize нет.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(520, 370))
	if len(*calls) != 1 {
		t.Errorf("configure с тем же размером вызвал onResize: %v", *calls)
	}

	// Обратно.
	c.w.SetSize(1400, 860)
	if len(*calls) != 2 || (*calls)[1] != [2]int{1400, 860} {
		t.Fatalf("onResize: %v, ждал вторым 1400x860", *calls)
	}
	if w, h := c.w.GetSize(); w != 1400 || h != 860 {
		t.Errorf("GetSize = %dx%d, ждал 1400x860", w, h)
	}
	mins, maxs = wlSzLimits(t, c.readAllWl(t))
	if len(mins) != 1 || mins[0] != [2]uint32{1400, 860} || len(maxs) != 1 || maxs[0] != [2]uint32{1400, 860} {
		t.Errorf("пределы: min %v max %v, ждал по 1400x860", mins, maxs)
	}
	c.w.BlitRGBA(image.NewRGBA(image.Rect(0, 0, 1400, 860)))
	if c.w.poolW != 1400 || c.w.poolH != 860 {
		t.Errorf("буфер %dx%d, ждал 1400x860", c.w.poolW, c.w.poolH)
	}
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1400, 860))
	if len(*calls) != 2 {
		t.Errorf("configure с тем же размером вызвал onResize: %v", *calls)
	}
}

// Растяжимое окно тоже меняет размер по запросу приложения (раньше запрос
// молча игнорировался), а пределов не трогает.
func TestWayland_SetSizeApply_ResizableWindow(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(true)

	c.w.SetSize(520, 370)
	if len(*calls) != 1 || (*calls)[0] != [2]int{520, 370} {
		t.Fatalf("onResize: %v, ждал один вызов 520x370", *calls)
	}
	if w, h := c.w.GetSize(); w != 520 || h != 370 {
		t.Errorf("GetSize = %dx%d, ждал 520x370", w, h)
	}
	if mins, maxs := wlSzLimits(t, c.readAllWl(t)); len(mins)+len(maxs) != 0 {
		t.Errorf("растяжимому окну отправлены пределы: min %v max %v", mins, maxs)
	}
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(520, 370))
	if len(*calls) != 1 {
		t.Errorf("configure с тем же размером вызвал onResize: %v", *calls)
	}
}

// Размер совпал с текущим — ни onResize, ни сообщений композитору.
func TestWayland_SetSizeApply_SameSizeIsNoOp(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(false)

	c.w.SetSize(1400, 860)
	if len(*calls) != 0 {
		t.Errorf("onResize при том же размере: %v", *calls)
	}
	if msgs := c.readAllWl(t); len(msgs) != 0 {
		t.Errorf("отправлено при том же размере: %+v", msgs)
	}
	// Нулевой и отрицательный — тоже.
	c.w.SetSize(0, 300)
	c.w.SetSize(300, -1)
	if len(*calls) != 0 || len(c.readAllWl(t)) != 0 {
		t.Error("недопустимый размер принят")
	}
}

// Компоновщик не согласился и назвал другой размер (минимум, тайлинг): onResize
// сработает ещё раз уже с ним, GetSize отвечает им.
func TestWayland_SetSizeApply_CompositorOverrides(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(true)

	c.w.SetSize(520, 370)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(600, 400))
	if len(*calls) != 2 || (*calls)[1] != [2]int{600, 400} {
		t.Fatalf("onResize: %v, ждал вторым 600x400", *calls)
	}
	if w, h := c.w.GetSize(); w != 600 || h != 400 {
		t.Errorf("GetSize = %dx%d, ждал фактический 600x400", w, h)
	}
}

// HiDPI: размер окна — в пикселях буфера, пределы в протоколе — в
// поверхностных единицах.
func TestWayland_SetSizeApply_HiDPILimitsInSurfaceUnits(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.scale.current = 2
	c.w.resizable.Store(false)

	c.w.SetSize(1040, 740)
	if len(*calls) != 1 || (*calls)[0] != [2]int{1040, 740} {
		t.Fatalf("onResize: %v, ждал размер в пикселях буфера 1040x740", *calls)
	}
	mins, maxs := wlSzLimits(t, c.readAllWl(t))
	if len(mins) != 1 || mins[0] != [2]uint32{520, 370} || len(maxs) != 1 || maxs[0] != [2]uint32{520, 370} {
		t.Errorf("пределы: min %v max %v, ждал по 520x370 поверхностных единиц", mins, maxs)
	}
	// configure в поверхностных единицах с тем же размером — не новость.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(520, 370))
	if len(*calls) != 1 {
		t.Errorf("configure с тем же размером вызвал onResize: %v", *calls)
	}
}

// Развёрнутому окну размер выбирает компоновщик: SetSize запоминает заказ,
// GetSize отвечает размером развёрнутого окна, пределов не шлёт. После
// «восстановить» (configure без состояний) окно принимает заказанный размер
// один раз и фиксируется на нём.
func TestWayland_SetSizeApply_MaximizedKeepsOrderForRestore(t *testing.T) {
	for _, restored := range []struct {
		name string
		w, h int32
	}{
		{"компоновщик не назвал размер (0x0)", 0, 0},
		{"компоновщик вернул прежний размер", 1400, 860},
	} {
		t.Run(restored.name, func(t *testing.T) {
			c, calls := wlSzWindow(t)
			c.w.resizable.Store(false)
			c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateMaximized))
			if len(*calls) != 1 {
				t.Fatalf("onResize на развёртывание: %v", *calls)
			}
			c.readAllWl(t)

			c.w.SetSize(520, 370)
			if len(*calls) != 1 {
				t.Errorf("развёрнутое окно изменило размер: %v", *calls)
			}
			if w, h := c.w.GetSize(); w != 1920 || h != 1080 {
				t.Errorf("GetSize = %dx%d, ждал размер развёрнутого окна 1920x1080", w, h)
			}
			if msgs := c.readAllWl(t); len(msgs) != 0 {
				t.Errorf("развёрнутому окну отправлено: %+v", msgs)
			}

			c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(restored.w, restored.h))
			if len(*calls) != 2 || (*calls)[1] != [2]int{520, 370} {
				t.Fatalf("onResize после восстановления: %v, ждал вторым 520x370", *calls)
			}
			if w, h := c.w.GetSize(); w != 520 || h != 370 {
				t.Errorf("GetSize = %dx%d, ждал 520x370", w, h)
			}
			mins, maxs := wlSzLimits(t, c.readAllWl(t))
			if len(mins) != 1 || mins[0] != [2]uint32{520, 370} || len(maxs) != 1 || maxs[0] != [2]uint32{520, 370} {
				t.Errorf("пределы: min %v max %v, ждал по 520x370", mins, maxs)
			}
			// Заказ потрачен, окно зафиксировано на нём: чужой размер из
			// следующего configure окно с запретом ресайза не принимает
			// (floatingSize, v3.43.3).
			c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(700, 500))
			if len(*calls) != 2 {
				t.Errorf("onResize: %v — окно с запретом ресайза приняло 700x500 от компоновщика", *calls)
			}
			if w, h := c.w.GetSize(); w != 520 || h != 370 {
				t.Errorf("GetSize = %dx%d, ждал закреплённые 520x370", w, h)
			}
		})
	}
}

// Окно с запретом ресайза после Maximize + SetSize + Restore фиксируется на
// заказанном размере, а не на размере развёрнутого окна (раньше Restore
// фиксировал окно на текущем, то есть развёрнутом, размере).
func TestWayland_SetSizeApply_RestorePinsOrderedSize(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(false)
	c.w.Maximize()
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateMaximized))
	c.readAllWl(t)

	c.w.SetSize(520, 370)
	c.w.Restore()
	msgs := c.readAllWl(t)
	mins, maxs := wlSzLimits(t, msgs)
	if len(mins) != 1 || mins[0] != [2]uint32{520, 370} || len(maxs) != 1 || maxs[0] != [2]uint32{520, 370} {
		t.Errorf("пределы при Restore: min %v max %v, ждал по 520x370", mins, maxs)
	}
	if _, ok := findWlMsg(msgs, 12, xdgToplevelUnsetMaximized); !ok {
		t.Error("unset_maximized не отправлен")
	}
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(0, 0))
	if len(*calls) != 2 || (*calls)[1] != [2]int{520, 370} {
		t.Errorf("onResize: %v, ждал вторым 520x370", *calls)
	}
}

// Окно после Restore из развёрнутого — обычное плавающее: SetSize работает
// сразу, как у окна, которое не разворачивали.
func TestWayland_SetSizeApply_AfterRestore(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(false)
	c.w.Maximize()
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateMaximized))
	c.w.Restore()
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1400, 860))
	if c.w.IsMaximized() {
		t.Fatal("окно всё ещё развёрнуто")
	}
	n := len(*calls)
	c.readAllWl(t)

	c.w.SetSize(520, 370)
	if len(*calls) != n+1 || (*calls)[n] != [2]int{520, 370} {
		t.Fatalf("onResize: %v, ждал 520x370 последним", *calls)
	}
	if w, h := c.w.GetSize(); w != 520 || h != 370 {
		t.Errorf("GetSize = %dx%d, ждал 520x370", w, h)
	}
	mins, maxs := wlSzLimits(t, c.readAllWl(t))
	if len(mins) != 1 || mins[0] != [2]uint32{520, 370} || len(maxs) != 1 || maxs[0] != [2]uint32{520, 370} {
		t.Errorf("пределы: min %v max %v, ждал по 520x370", mins, maxs)
	}
}

// Полный экран: заказанный размер запоминается, выход из него возвращает окну
// именно его (и фиксацию на нём), а не размер до полного экрана.
func TestWayland_SetSizeApply_FullscreenKeepsOrderForLeave(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(false)
	c.w.SetFullscreen(true)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(1920, 1080, xdgStateFullscreen))
	c.readAllWl(t)
	n := len(*calls)

	c.w.SetSize(520, 370)
	if len(*calls) != n {
		t.Errorf("полноэкранное окно изменило размер: %v", *calls)
	}
	if w, h := c.w.GetSize(); w != 1920 || h != 1080 {
		t.Errorf("GetSize = %dx%d, ждал размер полного экрана", w, h)
	}

	c.w.SetFullscreen(false)
	mins, maxs := wlSzLimits(t, c.readAllWl(t))
	if len(mins) != 1 || mins[0] != [2]uint32{520, 370} || len(maxs) != 1 || maxs[0] != [2]uint32{520, 370} {
		t.Errorf("фиксация при выходе: min %v max %v, ждал по 520x370", mins, maxs)
	}
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(0, 0))
	if len(*calls) != n+1 || (*calls)[n] != [2]int{520, 370} {
		t.Errorf("onResize после выхода: %v, ждал 520x370", *calls)
	}
}

// Прилепленное к краю окно (tiled_*) размер не выбирает: заказ ждёт, пока
// компоновщик не отлепит окно.
func TestWayland_SetSizeApply_TiledKeepsOrder(t *testing.T) {
	c, calls := wlSzWindow(t)
	c.w.resizable.Store(true)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(960, 1080, xdgStateTiledLeft))
	if len(*calls) != 1 || (*calls)[0] != [2]int{960, 1080} {
		t.Fatalf("onResize на прилипание: %v", *calls)
	}

	c.w.SetSize(520, 370)
	if len(*calls) != 1 {
		t.Errorf("прилепленное окно изменило размер: %v", *calls)
	}
	if w, h := c.w.GetSize(); w != 960 || h != 1080 {
		t.Errorf("GetSize = %dx%d, ждал размер прилепленного окна", w, h)
	}

	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(0, 0))
	if len(*calls) != 2 || (*calls)[1] != [2]int{520, 370} {
		t.Errorf("onResize после отлипания: %v, ждал вторым 520x370", *calls)
	}
}

// Разбор tiled-состояний configure.
func TestWlParseTiled(t *testing.T) {
	for _, tc := range []struct {
		name   string
		states []uint32
		want   bool
	}{
		{"нет состояний", nil, false},
		{"развёрнуто", []uint32{xdgStateMaximized, xdgStateActivated}, false},
		{"слева", []uint32{xdgStateActivated, xdgStateTiledLeft}, true},
		{"справа", []uint32{xdgStateTiledRight}, true},
		{"сверху", []uint32{xdgStateTiledTop}, true},
		{"снизу", []uint32{xdgStateTiledBottom}, true},
	} {
		if got := wlParseTiled(wlStatesArray(tc.states...)); got != tc.want {
			t.Errorf("%s: wlParseTiled = %v, ждал %v", tc.name, got, tc.want)
		}
	}
	if wlParseTiled(nil) || wlParseTiled([]byte{1, 0}) {
		t.Error("обрезанный массив принят за tiled")
	}
	// Заявленная длина больше фактической — читаем до конца буфера.
	arr := wlStatesArray(xdgStateTiledTop)
	arr[0] = 40
	if !wlParseTiled(arr) {
		t.Error("обрезанный массив с tiled не разобран")
	}
}
