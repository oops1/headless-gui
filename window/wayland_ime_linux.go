//go:build linux && !android

package window

// wayland_ime_linux.go — редактор метода ввода на Wayland (text-input-v3).
//
// На Wayland клиент не разговаривает с редактором метода ввода напрямую:
// между ними стоит компоновщик. Он сообщает поверхности, что ввод начался,
// шлёт набираемое (preedit) и готовый текст (commit), а клиент взамен
// рассказывает, где стоит каретка, — чтобы окно кандидатов встало под
// набираемым словом.
//
// Протокол необязательный: компоновщик вправе его не предлагать (у WinLine
// его сейчас нет). Тогда всё работает как раньше — до поля доходят только
// готовые символы.
//
// Особенность третьей версии: события копятся и применяются ОДНОЙ порцией по
// событию done. Применять их поодиночке нельзя — preedit и commit в одной
// порции означают «введи это и покажи вот такое набираемое», а порознь они
// встали бы в поле в неверном порядке.

import (
	"encoding/binary"
	"image"
	"sync"
)

const (
	// zwp_text_input_manager_v3
	wlTextInputMgrGetTextInput = 1

	// zwp_text_input_v3 — запросы.
	wlTextInputEnable             = 1
	wlTextInputDisable            = 2
	wlTextInputSetSurroundingText = 3
	wlTextInputSetContentType     = 5
	wlTextInputSetCursorRectangle = 6
	wlTextInputCommit             = 7

	// zwp_text_input_v3 — события.
	wlTextInputEvEnter          = 0
	wlTextInputEvLeave          = 1
	wlTextInputEvPreeditString  = 2
	wlTextInputEvCommitString   = 3
	wlTextInputEvDeleteSurrText = 4
	wlTextInputEvDone           = 5
)

// wlTextInput — состояние редактора метода ввода для окна.
type wlTextInput struct {
	mu sync.Mutex

	id      uint32 // объект zwp_text_input_v3 (0 — расширения нет)
	entered bool   // компоновщик сообщил, что ввод идёт в нашу поверхность
	serial  uint32 // число отправленных commit: его ждёт протокол

	// Накопленное до события done.
	preedit      string
	preeditCaret int
	hasPreedit   bool
	commit       string
	hasCommit    bool

	// Колбэки наружу (window.setupIME).
	onComposition func(text string, caret int)
	onCommit      func(text string)
	caret         func() (image.Rectangle, bool)
}

// SetOnIMEComposition подключает приёмник набираемого текста. Реализует
// imeBackend.
func (w *WaylandWindow) SetOnIMEComposition(fn func(text string, caret int)) {
	w.textInput.mu.Lock()
	w.textInput.onComposition = fn
	w.textInput.mu.Unlock()
}

// SetOnIMECommit подключает приёмник готового текста.
func (w *WaylandWindow) SetOnIMECommit(fn func(text string)) {
	w.textInput.mu.Lock()
	w.textInput.onCommit = fn
	w.textInput.mu.Unlock()
}

// SetIMECaretProvider подключает источник места каретки.
func (w *WaylandWindow) SetIMECaretProvider(fn func() (image.Rectangle, bool)) {
	w.textInput.mu.Lock()
	w.textInput.caret = fn
	w.textInput.mu.Unlock()
}

// setupTextInput создаёт объект ввода для места (seat). Зовётся из Create
// после привязки глобалов.
func (w *WaylandWindow) setupTextInput() {
	if w.textInputMgrID == 0 || w.seatID == 0 {
		return
	}
	id := w.newID()
	w.send(newWlMsg(w.textInputMgrID, wlTextInputMgrGetTextInput).
		putUint(id).putUint(w.seatID), -1)
	w.textInput.mu.Lock()
	w.textInput.id = id
	w.textInput.mu.Unlock()
}

// isTextInputObject — событие пришло объекту ввода.
func (w *WaylandWindow) isTextInputObject(obj uint32) bool {
	w.textInput.mu.Lock()
	defer w.textInput.mu.Unlock()
	return obj != 0 && obj == w.textInput.id
}

// handleTextInput разбирает события zwp_text_input_v3.
func (w *WaylandWindow) handleTextInput(opcode uint16, b []byte) {
	switch opcode {
	case wlTextInputEvEnter:
		w.textInput.mu.Lock()
		w.textInput.entered = true
		w.textInput.mu.Unlock()
		w.textInputEnable()

	case wlTextInputEvLeave:
		w.textInput.mu.Lock()
		w.textInput.entered = false
		id := w.textInput.id
		w.textInput.mu.Unlock()
		if id != 0 {
			w.send(newWlMsg(id, wlTextInputDisable), -1)
			w.textInputCommit()
		}

	case wlTextInputEvPreeditString:
		text, off := wlString(b, 0)
		begin := 0
		if len(b) >= off+4 {
			begin = int(int32(binary.LittleEndian.Uint32(b[off : off+4])))
		}
		w.textInput.mu.Lock()
		w.textInput.preedit, w.textInput.hasPreedit = text, true
		// Курсор внутри набираемого приходит в БАЙТАХ, а полю нужны руны.
		w.textInput.preeditCaret = wlRunesBefore(text, begin)
		w.textInput.mu.Unlock()

	case wlTextInputEvCommitString:
		text, _ := wlString(b, 0)
		w.textInput.mu.Lock()
		w.textInput.commit, w.textInput.hasCommit = text, true
		w.textInput.mu.Unlock()

	case wlTextInputEvDeleteSurrText:
		// Удаление текста вокруг каретки движок пока не поддерживает: поле
		// не умеет отдавать окружение, а без него удалять вслепую опаснее,
		// чем не удалять вовсе.

	case wlTextInputEvDone:
		w.textInputApply()
	}
}

// textInputApply применяет накопленную порцию: сперва готовый текст, потом
// набираемое — порядок задан протоколом.
func (w *WaylandWindow) textInputApply() {
	w.textInput.mu.Lock()
	commit, hasCommit := w.textInput.commit, w.textInput.hasCommit
	preedit, caret, hasPreedit := w.textInput.preedit, w.textInput.preeditCaret, w.textInput.hasPreedit
	onCommit, onComposition := w.textInput.onCommit, w.textInput.onComposition
	w.textInput.commit, w.textInput.hasCommit = "", false
	w.textInput.preedit, w.textInput.hasPreedit = "", false
	w.textInput.mu.Unlock()

	if hasCommit && onCommit != nil {
		onCommit(commit)
	}
	if onComposition != nil {
		if hasPreedit {
			onComposition(preedit, caret)
		} else if hasCommit {
			// Порция без набираемого означает, что его больше нет.
			onComposition("", 0)
		}
	}
	w.textInputUpdateCaret()
}

// textInputEnable включает ввод и сообщает место каретки.
func (w *WaylandWindow) textInputEnable() {
	w.textInput.mu.Lock()
	id := w.textInput.id
	w.textInput.mu.Unlock()
	if id == 0 {
		return
	}
	w.send(newWlMsg(id, wlTextInputEnable), -1)
	// Вид содержимого: обычный текст без подсказок. Без этого запроса
	// компоновщик вправе считать поле паролем и прятать набираемое.
	w.send(newWlMsg(id, wlTextInputSetContentType).putUint(0).putUint(0), -1)
	w.textInputSendCaret(id)
	w.textInputCommit()
}

// textInputUpdateCaret заново сообщает место каретки: она переехала вместе с
// набираемым.
func (w *WaylandWindow) textInputUpdateCaret() {
	w.textInput.mu.Lock()
	id, entered := w.textInput.id, w.textInput.entered
	w.textInput.mu.Unlock()
	if id == 0 || !entered {
		return
	}
	w.textInputSendCaret(id)
	w.textInputCommit()
}

// textInputSendCaret отправляет прямоугольник каретки в ПОВЕРХНОСТНЫХ
// единицах: компоновщик считает в них, а движок отдаёт физические пиксели.
func (w *WaylandWindow) textInputSendCaret(id uint32) {
	w.textInput.mu.Lock()
	fn := w.textInput.caret
	w.textInput.mu.Unlock()
	if fn == nil {
		return
	}
	r, ok := fn()
	if !ok {
		return
	}
	w.send(newWlMsg(id, wlTextInputSetCursorRectangle).
		putInt(int32(w.toSurface(r.Min.X))).putInt(int32(w.toSurface(r.Min.Y))).
		putInt(int32(w.toSurface(r.Dx()))).putInt(int32(w.toSurface(r.Dy()))), -1)
}

// textInputCommit завершает порцию запросов. Протокол требует его после
// любого набора изменений и считает их по порядку.
func (w *WaylandWindow) textInputCommit() {
	w.textInput.mu.Lock()
	id := w.textInput.id
	w.textInput.serial++
	w.textInput.mu.Unlock()
	if id == 0 {
		return
	}
	w.send(newWlMsg(id, wlTextInputCommit), -1)
}

// wlRunesBefore — сколько рун умещается в первых n байтах строки.
//
// Смещения в протоколе байтовые, а поле ввода считает в рунах: на латинице
// это одно и то же, на кириллице и иероглифах — разные числа.
func wlRunesBefore(s string, n int) int {
	if n <= 0 {
		return 0
	}
	if n >= len(s) {
		return len([]rune(s))
	}
	return len([]rune(s[:n]))
}
