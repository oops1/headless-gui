//go:build linux && !android

package x11

// x11_style_linux.go — стиль окна (native.Styler), владелец
// (native.OwnerSetter), Z-порядок (native.Stacker) и положение с честным
// ответом (native.Positioner), ADR-0013 §4.
//
//	Borderless  рамки менеджера у окна нет и так (Motif decorations=0);
//	            BeginResize отказывает, а _GTK_FRAME_EXTENTS (нулевые)
//	            говорит композитору, что тень окно рисует само (mutter,
//	            picom с обычным shadow-exclude её тогда не рисуют)
//	Tool        _NET_WM_STATE_SKIP_TASKBAR, _SKIP_PAGER и, для KWin,
//	            _KDE_NET_WM_STATE_SKIP_SWITCHER: без кнопки на панели задач
//	            и вне переключателя. Тип _NET_WM_WINDOW_TYPE_UTILITY не
//	            ставится: у mutter такое окно не получает фокус при показе,
//	            а KWin прячет окна UTILITY с владельцем, пока активно чужое
//	            приложение
//	владелец    WM_TRANSIENT_FOR без _NET_WM_STATE_MODAL; тип окна — явно
//	            NORMAL: окно с WM_TRANSIENT_FOR без типа менеджер обязан
//	            считать диалогом (EWMH, _NET_WM_WINDOW_TYPE)
//	Z-порядок   ConfigureWindow stack-mode Above / Below
//
// До MapWindow всё ставится свойствами (так EWMH задаёт начальное состояние),
// у показанного окна — ClientMessage _NET_WM_STATE и сменой свойств. Решает
// менеджер: true значит «запрос отправлен». У закрытого окна ответ — false.

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// xAtomWMTransientFor — предопределённый атом WM_TRANSIENT_FOR; xAtomWindow —
// тип XA_WINDOW; xAtomCardinal — тип CARDINAL.
const (
	xAtomWMTransientFor = 68
	xAtomWindow         = 33
	xAtomCardinal       = 6
)

// x11Style — стиль и владелец окна.
type x11Style struct {
	// borderless и tool — заказанный стиль (Style.Borderless, Style.Tool).
	borderless atomic.Bool
	tool       atomic.Bool
	// owner — владелец, заказанный до Create: Create ставит WM_TRANSIENT_FOR
	// до MapWindow.
	owner *X11Window

	// Атомы — интернируются в Create: у показанного окна запрос-ответ по
	// соединению недопустим (его читает цикл событий, см. GetPosition).
	atomType         uint32 // _NET_WM_WINDOW_TYPE
	atomTypeNormal   uint32 // _NET_WM_WINDOW_TYPE_NORMAL
	atomSkipTaskbar  uint32 // _NET_WM_STATE_SKIP_TASKBAR
	atomSkipPager    uint32 // _NET_WM_STATE_SKIP_PAGER
	atomSkipSwitcher uint32 // _KDE_NET_WM_STATE_SKIP_SWITCHER (KWin)
	atomFrameExtents uint32 // _GTK_FRAME_EXTENTS
}

var (
	_ native.Styler      = (*X11Window)(nil)
	_ native.OwnerSetter = (*X11Window)(nil)
	_ native.Stacker     = (*X11Window)(nil)
	_ native.Positioner  = (*X11Window)(nil)
)

// internStyleAtoms интернирует атомы стиля (Create, до показа окна).
func (w *X11Window) internStyleAtoms() {
	w.style.atomType = w.x11InternAtom("_NET_WM_WINDOW_TYPE")
	w.style.atomTypeNormal = w.x11InternAtom("_NET_WM_WINDOW_TYPE_NORMAL")
	w.style.atomSkipTaskbar = w.x11InternAtom("_NET_WM_STATE_SKIP_TASKBAR")
	w.style.atomSkipPager = w.x11InternAtom("_NET_WM_STATE_SKIP_PAGER")
	w.style.atomSkipSwitcher = w.x11InternAtom("_KDE_NET_WM_STATE_SKIP_SWITCHER")
	w.style.atomFrameExtents = w.x11InternAtom("_GTK_FRAME_EXTENTS")
}

// alive — окно создано и не закрыто: запросам есть куда идти.
func (w *X11Window) alive() bool { return w.wid != 0 && !w.closed.Load() }

// toolAtoms — атомы есть: Tool выполним. До Create — неизвестно, считается,
// что да. SKIP_SWITCHER — расширение KWin, без него Tool всё равно выполним.
func (w *X11Window) toolAtoms() bool {
	if w.wid == 0 {
		return true
	}
	return w.atomNetWMState != 0 && w.style.atomSkipTaskbar != 0 && w.style.atomSkipPager != 0
}

// skipAtoms — состояния _NET_WM_STATE окна-инструмента.
func (w *X11Window) skipAtoms() []uint32 {
	atoms := []uint32{w.style.atomSkipTaskbar, w.style.atomSkipPager}
	if w.style.atomSkipSwitcher != 0 {
		atoms = append(atoms, w.style.atomSkipSwitcher)
	}
	return atoms
}

// styleStateAtoms — состояния _NET_WM_STATE, которые требует стиль
// (applyInitialState кладёт их в начальное значение свойства).
func (w *X11Window) styleStateAtoms() []uint32 {
	if !w.style.tool.Load() || !w.toolAtoms() {
		return nil
	}
	return w.skipAtoms()
}

// applyStyleBeforeMap ставит владельца и признак Borderless, заказанные до
// Create (Create, до MapWindow: менеджер и композитор читают их при показе).
func (w *X11Window) applyStyleBeforeMap() {
	owner := w.style.owner
	w.style.owner = nil
	if owner != nil && owner.alive() {
		w.setWindowType(w.style.atomTypeNormal)
		w.x11ChangeProperty(w.wid, xAtomWMTransientFor, xAtomWindow, 32, uint32ToBytes(owner.wid))
	}
	if w.style.borderless.Load() {
		w.setFrameExtents(true)
	}
}

// setWindowType ставит _NET_WM_WINDOW_TYPE.
func (w *X11Window) setWindowType(typ uint32) {
	if w.style.atomType == 0 || typ == 0 {
		return
	}
	w.x11ChangeProperty(w.wid, w.style.atomType, 4 /*ATOM*/, 32, uint32ToBytes(typ))
}

// setFrameExtents ставит или снимает нулевые _GTK_FRAME_EXTENTS: «рамку и
// тень окно рисует само». Геометрию окна нулевые отступы не меняют.
func (w *X11Window) setFrameExtents(on bool) {
	a := w.style.atomFrameExtents
	if a == 0 {
		return
	}
	if on {
		w.x11ChangeProperty(w.wid, a, xAtomCardinal, 32, make([]byte, 16)) // left, right, top, bottom
		return
	}
	w.x11DeleteProperty(w.wid, a)
}

// ApplyStyle реализует native.Styler. Borderless X11 умеет всегда (рамки
// менеджера и так нет); Tool — если сервер дал атомы EWMH. Закрытое окно
// ничего не меняет и отвечает стилем, который был.
func (w *X11Window) ApplyStyle(s native.Style) native.Style {
	if w.wid != 0 && w.closed.Load() {
		return native.Style{Borderless: w.style.borderless.Load(), Tool: w.style.tool.Load()}
	}
	got := native.Style{Borderless: s.Borderless, Tool: s.Tool && w.toolAtoms()}
	if w.style.borderless.Swap(got.Borderless) != got.Borderless && w.wid != 0 {
		w.setFrameExtents(got.Borderless)
	}
	if w.style.tool.Swap(got.Tool) != got.Tool && w.wid != 0 {
		action := uint32(netWMStateRemove)
		if got.Tool {
			action = netWMStateAdd
		}
		w.sendSkipStates(action)
	}
	return got
}

// sendSkipStates добавляет или снимает состояния окна-инструмента у
// показанного окна: ClientMessage _NET_WM_STATE (свойство состояний у
// показанного окна пишет менеджер).
func (w *X11Window) sendSkipStates(action uint32) {
	if w.atomNetWMState == 0 {
		return
	}
	for _, a := range w.skipAtoms() {
		if a != 0 {
			w.x11Send(x11NetWMStateMessage(w.rootWin, w.wid, w.atomNetWMState, action, a))
		}
	}
}

// SetOwnerWindow реализует native.OwnerSetter: WM_TRANSIENT_FOR без
// _NET_WM_STATE_MODAL (в отличие от SetOwner модального диалога). ID окон
// глобальны на сервере, поэтому разные соединения окон не мешают. Закрытый
// владелец не принимается.
func (w *X11Window) SetOwnerWindow(owner native.NativeWindow) bool {
	var px *X11Window
	if owner != nil {
		var ok bool
		if px, ok = owner.(*X11Window); !ok || px == w || !px.alive() {
			return false
		}
	}
	if w.wid == 0 {
		w.style.owner = px
		return true
	}
	if w.closed.Load() {
		return false
	}
	if px == nil {
		w.x11DeleteProperty(w.wid, xAtomWMTransientFor)
		return true
	}
	w.setWindowType(w.style.atomTypeNormal)
	w.x11ChangeProperty(w.wid, xAtomWMTransientFor, xAtomWindow, 32, uint32ToBytes(px.wid))
	return true
}

// x11DeleteProperty удаляет свойство окна (DeleteProperty).
func (w *X11Window) x11DeleteProperty(wid, property uint32) {
	buf := make([]byte, 12)
	buf[0] = 19                                // DeleteProperty
	binary.LittleEndian.PutUint16(buf[2:4], 3) // length
	binary.LittleEndian.PutUint32(buf[4:8], wid)
	binary.LittleEndian.PutUint32(buf[8:12], property)
	w.x11Send(buf)
}

// RaiseWindow реализует native.Stacker (stack-mode Above). Фокус не
// передаётся: в отличие от SetForeground, без _NET_ACTIVE_WINDOW и
// SetInputFocus.
func (w *X11Window) RaiseWindow() bool {
	if !w.alive() {
		return false
	}
	w.x11StackWindow(x11StackAbove)
	return true
}

// LowerWindow реализует native.Stacker (stack-mode Below). Окно «поверх
// всех» менеджер опускает в пределах своего слоя.
func (w *X11Window) LowerWindow() bool {
	if !w.alive() {
		return false
	}
	w.x11StackWindow(x11StackBelow)
	return true
}

// MoveTo реализует native.Positioner. У развёрнутого, свёрнутого и
// полноэкранного окна место задаёт менеджер — false.
func (w *X11Window) MoveTo(x, y int) bool {
	if !w.alive() || w.fs.on.Load() || w.maximized || w.act.minimized.Load() {
		return false
	}
	w.SetPosition(x, y)
	return true
}

// ScreenPosition реализует native.Positioner: кэш положения (GetPosition).
// У свёрнутого окна ok=false, как на Win32.
func (w *X11Window) ScreenPosition() (x, y int, ok bool) {
	if !w.alive() || w.act.minimized.Load() {
		return 0, 0, false
	}
	w.posMu.Lock()
	defer w.posMu.Unlock()
	return w.posX, w.posY, w.posCached
}
