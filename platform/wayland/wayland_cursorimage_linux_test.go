//go:build linux && !android

package wayland

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/pointer"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// Wayland: курсор из картинки (Engine.SetCursorImage) — на проводе. Курсор
// Wayland — поверхность с wl_shm-буфером; картинка получает свой буфер, кэш не
// даёт строить его заново, масштаб окна переходит в буферный масштаб
// курсорной поверхности.

func wlciImage(w, h int, c color.NRGBA, hx, hy int) *base.CursorImage {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return base.NewCursorImage(img, hx, hy)
}

// wlciRead читает всё, что окно успело отправить. Сообщение с файловым
// дескриптором (create_pool) приходит отдельным recvmsg, поэтому одного чтения,
// как в read, мало: читаем, пока поток не затихнет.
func wlciRead(t *testing.T, c *wlTestConn) []byte {
	t.Helper()
	out := c.read(t)
	buf := make([]byte, 4096)
	for {
		c.peer.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		n, err := c.peer.Read(buf)
		if err != nil || n == 0 {
			return out
		}
		out = append(out, buf[:n]...)
	}
}

func wlciWindow(t *testing.T) *wlTestConn {
	t.Helper()
	c := newWlTestWindow(t)
	c.w.ptrEnterSerial.Store(77)
	c.w.gCompositorVer = wlCompositorVersionBufferDamage
	return c
}

func TestWayland_CursorImage_BuffersAndSetCursor(t *testing.T) {
	c := wlciWindow(t)
	ci := wlciImage(40, 30, color.NRGBA{R: 255, A: 128}, 5, 7)
	c.w.SetCursorImage(ci)
	msgs := splitWlMsgs(t, wlciRead(t, c))

	pool, ok := findWlMsg(msgs, c.w.shmID, wlShmCreatePool)
	if !ok || len(pool.args) != 2 || pool.args[1] != 40*30*4 {
		t.Fatalf("create_pool: %v %v", pool, ok)
	}
	buf, ok := findWlMsg(msgs, pool.args[0], wlShmPoolCreateBuffer)
	if !ok || len(buf.args) != 6 {
		t.Fatalf("create_buffer не отправлен: %v", msgs)
	}
	if got := buf.args[1:]; got[0] != 0 || got[1] != 40 || got[2] != 30 || got[3] != 160 || got[4] != wlShmFormatARGB8888 {
		t.Errorf("create_buffer %v, ждал [offset 0, 40, 30, stride 160, ARGB8888]", got)
	}
	if _, ok := findWlMsg(msgs, pool.args[0], wlShmPoolDestroy); !ok {
		t.Error("пул не уничтожен: память держал бы он, а не буфер")
	}
	surf := c.w.cursorSurfID
	if surf == 0 {
		t.Fatal("курсорная поверхность не создана")
	}
	att, ok := findWlMsg(msgs, surf, wlSurfaceAttach)
	if !ok || att.args[0] != buf.args[0] {
		t.Errorf("к поверхности прикреплён %v, ждал буфер %d", att.args, buf.args[0])
	}
	if _, ok := findWlMsg(msgs, surf, wlSurfaceCommit); !ok {
		t.Error("commit курсорной поверхности не отправлен")
	}
	sc, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok || len(sc.args) != 4 || sc.args[0] != 77 || sc.args[1] != surf || sc.args[2] != 5 || sc.args[3] != 7 {
		t.Errorf("set_cursor %v, ждал [77 %d 5 7]", sc.args, surf)
	}
	// Масштаб 1: буферный масштаб поверхности не трогаем.
	if _, ok := findWlMsg(msgs, surf, wlSurfaceSetBufferScale); ok {
		t.Error("set_buffer_scale при масштабе 1")
	}
}

// Та же картинка повторно не меняет ничего, а вернувшаяся после другой берётся
// из кэша — новый пул и буфер не создаются.
func TestWayland_CursorImage_Cached(t *testing.T) {
	c := wlciWindow(t)
	a := wlciImage(16, 16, color.NRGBA{R: 255, A: 255}, 1, 1)
	b := wlciImage(16, 16, color.NRGBA{G: 255, A: 255}, 1, 1)
	c.w.SetCursorImage(a)
	splitWlMsgs(t, wlciRead(t, c))
	n := c.w.imgCreated

	c.w.SetCursorImage(wlciImage(16, 16, color.NRGBA{R: 255, A: 255}, 1, 1)) // равное содержимое
	c.w.SetCursorImage(b)
	msgs := splitWlMsgs(t, wlciRead(t, c))
	if c.w.imgCreated != n+1 {
		t.Fatalf("построено %d буферов, ждал %d", c.w.imgCreated, n+1)
	}
	if _, ok := findWlMsg(msgs, c.w.shmID, wlShmCreatePool); !ok {
		t.Fatal("новая картинка без нового пула")
	}

	c.w.SetCursorImage(a) // вернулись к первой
	msgs = splitWlMsgs(t, wlciRead(t, c))
	if c.w.imgCreated != n+1 {
		t.Errorf("первая картинка построена заново: %d", c.w.imgCreated)
	}
	if _, ok := findWlMsg(msgs, c.w.shmID, wlShmCreatePool); ok {
		t.Error("для картинки из кэша создан пул")
	}
	if _, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor); !ok {
		t.Error("картинка из кэша не показана")
	}
}

// Кэш ограничен: сверх pointer.CursorCacheSize давний буфер уничтожается.
func TestWayland_CursorImage_LRUDestroysOldBuffer(t *testing.T) {
	c := wlciWindow(t)
	var first uint32
	for i := 0; i < pointer.CursorCacheSize+1; i++ {
		c.w.SetCursorImage(wlciImage(8, 8, color.NRGBA{R: uint8(i + 1), A: 255}, 0, 0))
		msgs := splitWlMsgs(t, wlciRead(t, c))
		if i == 0 {
			att, ok := findWlMsg(msgs, c.w.cursorSurfID, wlSurfaceAttach)
			if !ok {
				t.Fatal("нет attach")
			}
			first = att.args[0]
		}
		if i < pointer.CursorCacheSize {
			if _, ok := findWlMsg(msgs, first, wlBufferDestroy); ok {
				t.Fatalf("буфер уничтожен слишком рано, шаг %d", i)
			}
		} else if _, ok := findWlMsg(msgs, first, wlBufferDestroy); !ok {
			t.Errorf("давний буфер %d не уничтожен при переполнении кэша: %v", first, msgs)
		}
	}
	if n := c.w.imgCursors.Len(); n != pointer.CursorCacheSize {
		t.Errorf("в кэше %d буферов, ждал %d", n, pointer.CursorCacheSize)
	}
}

// Масштаб окна переходит в буферный масштаб курсорной поверхности, горячая
// точка делится на него; форма после картинки возвращает масштаб 1.
func TestWayland_CursorImage_Scale(t *testing.T) {
	c := wlciWindow(t)
	c.w.scale.current = 2
	c.w.SetCursorImage(wlciImage(64, 64, color.NRGBA{B: 255, A: 255}, 20, 11))
	msgs := splitWlMsgs(t, wlciRead(t, c))
	surf := c.w.cursorSurfID
	bs, ok := findWlMsg(msgs, surf, wlSurfaceSetBufferScale)
	if !ok || bs.args[0] != 2 {
		t.Fatalf("set_buffer_scale %v, ждал [2]", bs.args)
	}
	sc, _ := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if sc.args[2] != 10 || sc.args[3] != 5 {
		t.Errorf("горячая точка %v, ждал (10,5) = (20,11)/2", sc.args[2:])
	}
	// Дробный масштаб округляется до ближайшего целого.
	if got := func() int { c.w.scale.current = 1.5; return c.w.cursorBufferScale() }(); got != 2 {
		t.Errorf("масштаб 1.5 → %d, ждал 2", got)
	}
	c.w.scale.current = 2
	// Форма вместо картинки: масштаб поверхности снова 1, картинки нет.
	c.w.SetCursor(pointer.CursorIBeam)
	msgs = splitWlMsgs(t, wlciRead(t, c))
	bs, ok = findWlMsg(msgs, surf, wlSurfaceSetBufferScale)
	if !ok || bs.args[0] != 1 {
		t.Errorf("после формы set_buffer_scale %v, ждал [1]", bs.args)
	}
	if c.w.cursorImg != nil {
		t.Error("картинка пережила SetCursor")
	}
}

// Компоновщик, связанный на wl_compositor v1, не знает set_buffer_scale: его
// не отправляем даже при масштабе 2.
func TestWayland_CursorImage_NoBufferScaleOnOldCompositor(t *testing.T) {
	c := wlciWindow(t)
	c.w.gCompositorVer = 1
	c.w.scale.current = 2
	c.w.SetCursorImage(wlciImage(8, 8, color.NRGBA{A: 255}, 3, 3))
	msgs := splitWlMsgs(t, wlciRead(t, c))
	if _, ok := findWlMsg(msgs, c.w.cursorSurfID, wlSurfaceSetBufferScale); ok {
		t.Error("set_buffer_scale на wl_compositor v1")
	}
	sc, _ := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if sc.args[2] != 3 {
		t.Errorf("горячая точка %v, ждал нетронутую (3,3)", sc.args[2:])
	}
}

// Указателя в окне ещё не было (нет serial'а enter): картинка запоминается и
// показывается на enter.
func TestWayland_CursorImage_AppliedOnEnter(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.SetCursorImage(wlciImage(8, 8, color.NRGBA{A: 255}, 0, 0))
	c.w.ptrEnterSerial.Store(9)
	c.w.applyCursor()
	msgs := splitWlMsgs(t, wlciRead(t, c))
	sc, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok || sc.args[0] != 9 || sc.args[1] == 0 {
		t.Fatalf("после enter картинка не показана: %v", msgs)
	}
}

// Над попапом (xdg_popup) действует курсор-картинка движка; снятая — форма.
func TestWayland_CursorImage_OverPopup(t *testing.T) {
	c := wlciWindow(t)
	c.w.ptrOverPopup.Store(true)
	img := wlciImage(8, 8, color.NRGBA{R: 9, A: 255}, 2, 2)
	c.w.SetPopupCursorImage(img)
	msgs := splitWlMsgs(t, wlciRead(t, c))
	sc, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok || sc.args[1] == 0 || sc.args[2] != 2 {
		t.Fatalf("над попапом картинка не показана: %v", msgs)
	}
	c.w.SetPopupCursor(pointer.CursorNone, true) // форма вместо картинки
	msgs = splitWlMsgs(t, wlciRead(t, c))
	sc, ok = findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok || sc.args[1] != 0 {
		t.Errorf("форма None над попапом не скрыла курсор: %v", msgs)
	}
	if c.w.popupImg != nil {
		t.Error("картинка попапа пережила SetPopupCursor")
	}
}

// Без wl_shm курсор-картинку не построить: окно не падает, пул не создаёт, а
// картинку помнит.
func TestWayland_CursorImage_NoShm(t *testing.T) {
	c := wlciWindow(t)
	shm := c.w.shmID
	c.w.shmID = 0
	c.w.gCursorShapeMgr = 30 // форма по имени, чтобы по проводу было что видеть
	c.w.SetCursorImage(wlciImage(8, 8, color.NRGBA{A: 255}, 0, 0))
	msgs := splitWlMsgs(t, wlciRead(t, c))
	for _, m := range msgs {
		if m.opcode == wlShmCreatePool && m.obj == shm {
			t.Errorf("пул создан без wl_shm: %v", m)
		}
	}
	if c.w.cursorImg == nil {
		t.Error("картинка не запомнена")
	}
}

// ─── Захват указателя ────────────────────────────────────────────────────────

func wlplWindow(t *testing.T) *wlTestConn {
	t.Helper()
	c := wlciWindow(t)
	c.w.registryID = 2
	c.w.plock.gConstraints = 40
	c.w.plock.gRelative = 41
	return c
}

func TestWayland_PointerLock_NeedsBothGlobals(t *testing.T) {
	for _, tc := range []struct{ cons, rel uint32 }{{0, 0}, {40, 0}, {0, 41}} {
		c := wlplWindow(t)
		c.w.plock.gConstraints, c.w.plock.gRelative = tc.cons, tc.rel
		if c.w.SetPointerLock(true) {
			t.Errorf("constraints=%d relative=%d: захват включён без расширения", tc.cons, tc.rel)
		}
		if c.w.plock.lockedID.Load() != 0 {
			t.Error("объект захвата создан при отказе")
		}
	}
	if c := wlplWindow(t); !c.w.SetPointerLock(false) {
		t.Error("выключение вернуло false")
	}
}

func TestWayland_PointerLock_RequestsOnWire(t *testing.T) {
	c := wlplWindow(t)
	if !c.w.SetPointerLock(true) {
		t.Fatal("захват отвергнут")
	}
	msgs := splitWlMsgs(t, wlciRead(t, c))
	// Оба менеджера связаны через реестр.
	binds := 0
	for _, m := range msgs {
		if m.obj == c.w.registryID && m.opcode == wlRegistryBind {
			binds++
		}
	}
	if binds != 2 {
		t.Errorf("bind: %d, ждал 2", binds)
	}
	l := &c.w.plock
	rel, ok := findWlMsg(msgs, l.relMgrID, zwpRelMgrGetRelative)
	if !ok || len(rel.args) != 2 || rel.args[0] != l.relID.Load() || rel.args[1] != c.w.pointerID {
		t.Errorf("get_relative_pointer %v", rel.args)
	}
	lock, ok := findWlMsg(msgs, l.constraintsID, zwpConstraintsLockPointer)
	want := []uint32{l.lockedID.Load(), c.w.surfaceID, c.w.pointerID, 0, zwpLifetimePersistent}
	if !ok || len(lock.args) != 5 {
		t.Fatalf("lock_pointer: %v", msgs)
	}
	for i := range want {
		if lock.args[i] != want[i] {
			t.Errorf("lock_pointer аргумент %d = %d, ждал %d", i, lock.args[i], want[i])
		}
	}
	// Повторное включение: ни одного нового запроса (иначе ошибка протокола).
	if !c.w.SetPointerLock(true) {
		t.Error("повторное включение вернуло false")
	}
	c.peer.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if n, _ := c.peer.Read(make([]byte, 64)); n != 0 {
		t.Errorf("повторное включение отправило %d байт", n)
	}
}

func TestWayland_PointerLock_LockedAndRelativeMotion(t *testing.T) {
	c := wlplWindow(t)
	c.w.SetPointerLock(true)
	splitWlMsgs(t, wlciRead(t, c))
	l := &c.w.plock
	var motions [][2]float64
	var changes []bool
	c.w.SetOnRelativeMotion(func(dx, dy float64) { motions = append(motions, [2]float64{dx, dy}) })
	c.w.SetOnPointerLockChanged(func(a bool) { changes = append(changes, a) })

	// Сдвиг до locked не доставляется (указатель ещё не заперт).
	body := wlU32(0, 100, 2<<8, 3<<8, 0xFFFFFD00 /* -3 */, 1<<7 /* 0.5 */)
	c.w.handleEvent(l.relID.Load(), zwpRelativeEvRelativeMove, body)
	if len(motions) != 0 {
		t.Fatalf("сдвиг до locked доставлен: %v", motions)
	}

	c.w.handleEvent(l.lockedID.Load(), zwpLockedEvLocked, nil)
	if !l.active.Load() || len(changes) != 1 || !changes[0] {
		t.Fatalf("locked не принят: active=%v changes=%v", l.active.Load(), changes)
	}
	// При захвате курсор спрятан: set_cursor(serial, NULL).
	msgs := splitWlMsgs(t, wlciRead(t, c))
	sc, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok || sc.args[1] != 0 {
		t.Errorf("при захвате курсор не спрятан: %v", msgs)
	}

	// Берётся НЕускоренный сдвиг (dx_unaccel, dy_unaccel), не первый dx, dy.
	c.w.handleEvent(l.relID.Load(), zwpRelativeEvRelativeMove, body)
	if len(motions) != 1 || motions[0] != [2]float64{-3, 0.5} {
		t.Errorf("сдвиг %v, ждал [[-3 0.5]] (неускоренный)", motions)
	}

	// unlocked (потеря фокуса): сдвиги больше не доставляются, курсор вернулся.
	c.w.handleEvent(l.lockedID.Load(), zwpLockedEvUnlocked, nil)
	if l.active.Load() || len(changes) != 2 || changes[1] {
		t.Errorf("unlocked не принят: active=%v changes=%v", l.active.Load(), changes)
	}
	c.w.handleEvent(l.relID.Load(), zwpRelativeEvRelativeMove, body)
	if len(motions) != 1 {
		t.Errorf("сдвиг после unlocked доставлен: %v", motions)
	}
	msgs = splitWlMsgs(t, wlciRead(t, c))
	sc, ok = findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor)
	if !ok || sc.args[1] == 0 {
		t.Errorf("после unlocked курсор не возвращён: %v", msgs)
	}

	// Фокус вернулся — persistent-захват снова locked, сдвиги идут.
	c.w.handleEvent(l.lockedID.Load(), zwpLockedEvLocked, nil)
	c.w.handleEvent(l.relID.Load(), zwpRelativeEvRelativeMove, body)
	if len(motions) != 2 || len(changes) != 3 || !changes[2] {
		t.Errorf("восстановление: сдвиги %v уведомления %v", motions, changes)
	}
}

func TestWayland_PointerLock_Unlock(t *testing.T) {
	c := wlplWindow(t)
	c.w.SetPointerLock(true)
	splitWlMsgs(t, wlciRead(t, c))
	l := &c.w.plock
	locked, rel := l.lockedID.Load(), l.relID.Load()
	var changes []bool
	c.w.SetOnPointerLockChanged(func(a bool) { changes = append(changes, a) })
	c.w.handleEvent(locked, zwpLockedEvLocked, nil)
	splitWlMsgs(t, wlciRead(t, c))

	if !c.w.SetPointerLock(false) {
		t.Fatal("выключение вернуло false")
	}
	msgs := splitWlMsgs(t, wlciRead(t, c))
	if _, ok := findWlMsg(msgs, locked, zwpLockedDestroy); !ok {
		t.Errorf("locked_pointer.destroy не отправлен: %v", msgs)
	}
	if _, ok := findWlMsg(msgs, rel, zwpRelativeDestroy); !ok {
		t.Errorf("relative_pointer.destroy не отправлен: %v", msgs)
	}
	if l.lockedID.Load() != 0 || l.active.Load() {
		t.Error("состояние захвата не сброшено")
	}
	if len(changes) != 2 || changes[1] {
		t.Errorf("уведомления %v, ждал [true false]", changes)
	}
	// Заново можно включить, получив новые объекты.
	if !c.w.SetPointerLock(true) || l.lockedID.Load() == locked {
		t.Error("повторное включение после снятия не создало новый захват")
	}
}

func TestParseRelativeMotion(t *testing.T) {
	body := wlU32(1, 2, 0x100, 0x200, 0xFFFFFA00 /* -6 */, 0x80 /* 0.5 */)
	dx, dy, ok := parseRelativeMotion(body)
	if !ok || dx != -6 || dy != 0.5 {
		t.Errorf("(%v,%v,%v), ждал (-6,0.5,true)", dx, dy, ok)
	}
	if _, _, ok := parseRelativeMotion(body[:20]); ok {
		t.Error("обрезанное тело принято")
	}
}

func TestWlPointerLockMessages(t *testing.T) {
	m := splitWlMsgs(t, wlLockPointerMsg(30, 31, 11, 14).bytes())
	if len(m) != 1 || m[0].obj != 30 || m[0].opcode != zwpConstraintsLockPointer {
		t.Fatalf("lock_pointer: %v", m)
	}
	if got := m[0].args; len(got) != 5 || got[0] != 31 || got[1] != 11 || got[2] != 14 || got[3] != 0 || got[4] != 2 {
		t.Errorf("аргументы lock_pointer %v", got)
	}
	m = splitWlMsgs(t, wlGetRelativePointerMsg(32, 33, 14).bytes())
	if len(m) != 1 || m[0].obj != 32 || m[0].opcode != zwpRelMgrGetRelative || m[0].args[0] != 33 || m[0].args[1] != 14 {
		t.Errorf("get_relative_pointer: %v", m)
	}
}

// Глобалы реестра запоминаются при объявлении компоновщиком.
func TestWayland_PointerLock_RegistryGlobals(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.registryID = 2
	for _, g := range []struct {
		name  uint32
		iface string
	}{{40, "zwp_pointer_constraints_v1"}, {41, "zwp_relative_pointer_manager_v1"}} {
		b := newWlMsg(2, wlRegistryEvGlobal).putUint(g.name).putString(g.iface).putUint(1).bytes()
		c.w.handleEvent(2, wlRegistryEvGlobal, b[8:])
	}
	if c.w.plock.gConstraints != 40 || c.w.plock.gRelative != 41 {
		t.Errorf("глобалы %d %d, ждал 40 41", c.w.plock.gConstraints, c.w.plock.gRelative)
	}
}
