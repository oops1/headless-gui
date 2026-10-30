package engine

// input.go — ввод движка (ADR-0011, задача 7.2): доставку событий, фокус,
// захват мыши, касания, IME и принудительный курсор ведёт core/input
// (input.Dispatcher, поле e.in). Здесь — публичные методы ввода Engine,
// которые отдают работу ему (открытый API не меняется), и адаптер inputHost:
// то, что ввод берёт у движка (корень, модальные окна, масштаб, инвалидация,
// очередь Post, граница сбоев, подсказка).

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/core/input"
	"github.com/oops1/headless-gui/v3/widget"
)

// inputHost — движок глазами ввода (input.Host). Методы зовутся без замков
// пакета input; здесь каждый берёт только свой листовой замок движка.
type inputHost struct{ e *Engine }

var _ input.Host = inputHost{}

// Root — корень дерева (под e.mu).
func (h inputHost) Root() widget.Widget {
	h.e.mu.RLock()
	defer h.e.mu.RUnlock()
	return h.e.root
}

// TopModal — верхнее модальное окно в холсте.
func (h inputHost) TopModal() widget.ModalWidget { return h.e.topModal() }

// Modals — копия стека модальных окон (под modMu).
func (h inputHost) Modals() []widget.ModalWidget {
	h.e.modMu.Lock()
	defer h.e.modMu.Unlock()
	return append([]widget.ModalWidget(nil), h.e.modals...)
}

// RequestCloseModal — закрытие модального окна по Esc или щелчку мимо.
func (h inputHost) RequestCloseModal(m widget.ModalWidget) { h.e.requestCloseModal(m) }

// Scale — HiDPI-масштаб.
func (h inputHost) Scale() float64 { return h.e.Scale() }

// Invalidate — полный кадр.
func (h inputHost) Invalidate() { h.e.Invalidate() }

// InvalidateRect — область в логических координатах экрана.
func (h inputHost) InvalidateRect(r image.Rectangle) { h.e.InvalidateRect(r) }

// Post — очередь горутины кадра.
func (h inputHost) Post(fn func()) { h.e.Post(fn) }

// InLoop — горутина кадра или цикл не запущен: выполнять сразу.
func (h inputHost) InLoop() bool {
	return h.e.lp.InLoop()
}

// GuardInput — граница сбоев доставки события (failure.go).
func (h inputHost) GuardInput(where string) func() { return h.e.guardInput(where) }

// RecordMouse — отсчёт задержки подсказки (tooltip.go).
func (h inputHost) RecordMouse(x, y int) { h.e.recordMouse(x, y) }

// InvalidateShownTooltip — стереть показанную подсказку (tooltip.go).
func (h inputHost) InvalidateShownTooltip() { h.e.invalidateShownTooltip() }

// CaptureManager — сам движок, как у injectCaptureManager при SetRoot: виджет
// получает один менеджер на обоих путях и достаёт из него Post движка.
func (h inputHost) CaptureManager() widget.CaptureManager { return h.e }

// SetCapture направляет все события мыши на указанный виджет.
func (e *Engine) SetCapture(w widget.Widget) {
	e.in.SetCapture(w)
}

// ReleaseCapture отменяет захват мыши.
func (e *Engine) ReleaseCapture() {
	e.in.ReleaseCapture()
}

// SetFocus передаёт фокус ввода виджету w.
// Если w == nil — фокус снимается со всех виджетов.
func (e *Engine) SetFocus(w widget.Widget) {
	e.in.SetFocus(w)
}

// SendKeyEvent доставляет клавиатурное событие виджету с фокусом.
// Tab / Shift+Tab перехватываются для переключения фокуса между виджетами.
//
// Полной инвалидации здесь нет: виджет с фокусом самоинвалидируется при
// изменении своего состояния (текст, каретка, выделение), Tab-навигация
// инвалидирует старый/новый фокус точечно, командные хоткеи — полностью
// (команда может изменить что угодно).
func (e *Engine) SendKeyEvent(ev widget.KeyEvent) {
	e.in.SendKeyEvent(ev)
}

// CursorAt возвращает форму курсора для точки (x, y — физические пиксели):
// курсор самого глубокого виджета под точкой, реализующего
// widget.CursorProvider (иначе — стрелка).
func (e *Engine) CursorAt(x, y int) widget.Cursor {
	return e.in.CursorAt(x, y)
}

// SendMouseMove уведомляет всё дерево виджетов о перемещении курсора в (x, y).
// Если есть виджет, захвативший мышь — событие идёт только ему.
// Если активен модальный виджет — broadcast только внутри него.
// Иначе — broadcast всему дереву.
//
// Полной инвалидации здесь больше нет: hover-изменения виджеты сообщают сами
// (Base.Invalidate при фактической смене состояния), drag двигает панели через
// SetBounds (авто-инвалидация old∪new). Кадры рендерятся только когда картинка
// действительно меняется.
func (e *Engine) SendMouseMove(x, y int) {
	e.in.SendMouseMove(x, y)
}

// SendMouseButton уведомляет дерево о нажатии/отпускании кнопки мыши в (x, y).
// Если мышь захвачена — событие идёт только захватчику.
// Иначе: проверяем, хочет ли какой-либо предок захватить мышь (WantsCapture),
// затем передаём событие самому верхнему виджету под курсором.
func (e *Engine) SendMouseButton(x, y int, btn widget.MouseButton, pressed bool) {
	e.in.SendMouseButton(x, y, btn, pressed)
}

// SendMouseWheelPixels доставляет точную пиксельную дельту прокрутки в точке
// (xPhys, yPhys — физические пиксели окна/кадра). dy>0 — вниз, dx>0 — вправо.
// Событие всплывает от самого глубокого виджета под курсором к корню; первый
// виджет, реализующий wheelPixelHandler и поглотивший дельту, останавливает
// всплытие.
//
// Фолбэк: если точную дельту никто не принял (виджет знает лишь тиковое
// колесо), синтезируем эквивалентные тики — старый тиковый путь остаётся
// рабочим (headless-контракт). Инвалидация — только область получателя.
func (e *Engine) SendMouseWheelPixels(xPhys, yPhys int, dx, dy float64) {
	e.in.SendMouseWheelPixels(xPhys, yPhys, dx, dy)
}

// SendFilesDropped доставляет событие сброса файлов из ОС в точку (x, y —
// ФИЗИЧЕСКИЕ пиксели окна/кадра, как у SendMouse*). Событие всплывает от
// самого глубокого виджета под точкой к корню; первый виджет, реализующий
// widget.FileDropTarget и вернувший true, поглощает событие и останавливает
// всплытие (bubbling, как у колеса).
//
// paths — абсолютные пути к сброшенным файлам. Координаты, переданные виджету,
// уже логические. Позволяет headless-тестам синтетически «сбрасывать» файлы.
func (e *Engine) SendFilesDropped(x, y int, paths []string) {
	e.in.SendFilesDropped(x, y, paths)
}

// SetDoubleClickTime задаёт интервал, внутри которого второе нажатие считается
// двойным щелчком, а третье — тройным.
//
// Значение системное: человек выставляет его в параметрах мыши, и угадывать
// за него не нужно. Окно сообщает его при запуске, если бэкенд умеет узнать
// (Win32 — GetDoubleClickTime); иначе остаётся значение по умолчанию.
// d <= 0 возвращает к значению по умолчанию.
func (e *Engine) SetDoubleClickTime(d time.Duration) {
	e.in.SetDoubleClickTime(d)
}

// SendTouch доставляет событие касания и сообщает, принято ли оно.
//
// ev.X, ev.Y — физические пиксели окна, как у SendMouseMove; нулевое ev.Time
// движок заменяет временем прихода события. true — касание принадлежит виджету
// (на Began: нашёлся владелец; на остальных фазах: владелец есть и получил
// событие); false — касание ничьё, и решать его судьбу вызывающему (окно
// превращает такое в мышь, см. window.Window.SetTouchEnabled). События, чей
// палец движку неизвестен (Moved без Began), тоже дают false.
//
// Вход для бэкендов окна и для тестов: headless-приложение кормит этим движок
// так же, как SendMouseButton.
func (e *Engine) SendTouch(ev widget.TouchEvent) bool {
	return e.in.SendTouch(ev)
}

// CancelTouches отменяет все активные касания: каждому владельцу уходит
// TouchCancelled в точке, где касание было в последний раз. Зовут, когда ввод у
// окна отобрали — потеря фокуса, системный жест, — и конец касаний не придёт.
func (e *Engine) CancelTouches() {
	e.in.CancelTouches()
}

// TouchActive — сколько касаний сейчас принадлежат виджетам.
func (e *Engine) TouchActive() int {
	return e.in.TouchActive()
}

// SetModifiers сообщает движку, какие модификаторы зажаты сейчас.
//
// Вызывает бэкенд окна при каждом нажатии и отпускании Ctrl/Shift/Alt
// (window.Run делает это сам). Всё, что движок разошлёт после этого —
// нажатия, движения, колесо — уедет с этими модификаторами в MouseEvent.Mod.
//
// Приложению, которое кормит движок событиями само (headless, тесты, свой
// бэкенд), достаточно позвать это перед SendMouseButton:
//
//	eng.SetModifiers(widget.ModCtrl)
//	eng.SendMouseButton(x, y, widget.MouseLeft, true)
//
// Состояние, а не параметр события: у SendMouseButton, SendMouseMove и
// SendMouseWheelPixels пришлось бы менять сигнатуры — все три, — и всякое
// приложение, собранное с прежней версией, перестало бы компилироваться ради
// параметра, который в большинстве вызовов равен нулю.
func (e *Engine) SetModifiers(mod widget.KeyMod) {
	e.in.SetModifiers(mod)
}

// Modifiers возвращает модификаторы, которыми движок помечает события мыши.
func (e *Engine) Modifiers() widget.KeyMod {
	return e.in.Modifiers()
}

// SendMouseWheelRaw доставляет сырое событие колеса в точку (xPhys, yPhys —
// физические пиксели окна/кадра, как у SendMouseWheelPixels). dx, dy — в
// единицах widget.WheelNotch (120 на щелчок), dy > 0 — вниз, dx > 0 — вправо;
// precise — высокоточная дельта (тачпад).
//
// Получатели — по порядку: виджет, захвативший мышь, затем открытый оверлей под
// курсором, затем путь hit-test от самого глубокого виджета к корню (при
// модальном окне — внутри него). Первый, кто реализует
// widget.WheelEventHandler и вернул true, поглощает событие. Результат —
// «принято ли»: окно по нему решает, запускать ли прежний пиксельный путь для
// этого же движения колеса (принявшему сырое событие пиксели не нужны, а
// прокручивать дерево дважды нельзя). Headless-код вызывает метод напрямую и
// вправе игнорировать результат.
//
// Дельты не масштабируются HiDPI-коэффициентом: это не пиксели.
func (e *Engine) SendMouseWheelRaw(xPhys, yPhys int, dx, dy float64, precise bool) bool {
	return e.in.SendMouseWheelRaw(xPhys, yPhys, dx, dy, precise)
}

// SendPointerLeave сообщает виджетам, что указатель покинул окно: каждому
// видимому виджету дерева (и модального окна, и открытых оверлеев), который
// реализует widget.PointerLeaveHandler, приходит OnPointerLeave. Сам по себе
// метод наведение не снимает — для этого окно присылает ещё и движение в точку
// вне окна (SendMouseMove), которое идёт перед вызовом.
func (e *Engine) SendPointerLeave() {
	e.in.SendPointerLeave()
}

// SendComposition передаёт фокусному виджету набираемый текст; caret —
// позиция курсора ВНУТРИ него (в рунах).
//
// Пустой текст означает «набранное убрать»: так система сообщает об отмене
// последнего слога.
func (e *Engine) SendComposition(text string, caret int) {
	e.in.SendComposition(text, caret)
}

// CommitComposition завершает ввод: текст становится содержимым поля.
func (e *Engine) CommitComposition(text string) {
	e.in.CommitComposition(text)
}

// CancelComposition отменяет ввод: набранное убирается без следа.
func (e *Engine) CancelComposition() {
	e.in.CancelComposition()
}

// IMEActive сообщает, принимает ли фокусный виджет незавершённый ввод.
//
// По нему окно решает, включать ли ввод вообще: над кнопкой или списком
// редактор метода ввода не нужен, и держать его включённым — значит
// показывать человеку окно кандидатов там, где печатать некуда.
func (e *Engine) IMEActive() bool {
	return e.in.IMEActive()
}

// CaretRect возвращает место каретки фокусного виджета в ФИЗИЧЕСКИХ пикселях
// окна и признак, что оно известно.
//
// Физические, а не логические: координаты уходят прямо в системный вызов
// (окно кандидатов на Windows, прямоугольник курсора на Wayland), а система
// считает в пикселях экрана.
func (e *Engine) CaretRect() (image.Rectangle, bool) {
	return e.in.CaretRect()
}

// SetCursorOverride задаёт форму курсора, которая перекрывает курсор любого
// виджета под мышью, пока не будет снята ClearCursorOverride.
//
// Форма применяется к окну немедленно — без ожидания движения мыши: так можно
// спрятать курсор (widget.CursorNone) по таймеру бездействия. Безопасен для
// вызова с любой горутины. Повторный вызов с той же формой окно не беспокоит.
//
// Форма действует и над вынесенными попапами (меню и выпадающие списки в
// отдельных окнах ОС): окно движка (window.Run) переносит её на каждый попап,
// в том числе открытый уже после вызова. Снятая — возвращает попапам стрелку.
//
// Заменяет и курсор-картинку (SetCursorImage): действует последний вызов.
func (e *Engine) SetCursorOverride(c widget.Cursor) {
	e.in.SetCursorOverride(c)
}

// SetCursorImage показывает указатель из картинки: поверх курсора любого
// виджета и над окнами-попапами, как SetCursorOverride. Картинка — NRGBA (прямая
// альфа) до 384×384 (RDP; допустимо до widget.MaxCursorImageSide), hotspotX,
// hotspotY — горячая точка в её пикселях.
//
// Картинка копируется: приложение вправе менять свой буфер сразу после вызова.
// Повторная установка той же картинки (тот же хеш содержимого и горячей точки)
// окно не беспокоит и не создаёт системный курсор заново — берётся из кэша окна
// (до 16 последних картинок), так что слать указатель на каждое движение мыши
// не накладно. Безопасен для вызова с любой горутины.
//
// Сброс — ClearCursorOverride или SetCursorImage(nil, 0, 0). Пустая или
// недопустимая картинка (сторона больше widget.MaxCursorImageSide) действует
// как nil: снимает курсор-картинку, но не принудительную форму SetCursorOverride.
// Ограничение не зависит от платформы: курсор WinLine RDP — до 32×32, но
// картинки крупнее допустимы, если система их показывает.
func (e *Engine) SetCursorImage(img *image.NRGBA, hotspotX, hotspotY int) {
	e.in.SetCursorImage(img, hotspotX, hotspotY)
}

// ClearCursorOverride снимает принудительный курсор (форму и картинку): форму
// снова определяет виджет под мышью. Окно возвращает её сразу, по последнему
// положению мыши.
func (e *Engine) ClearCursorOverride() {
	e.in.ClearCursorOverride()
}

// CursorOverride возвращает принудительную форму курсора и признак, что она
// задана. Курсор-картинка (SetCursorImage) виден здесь как «задан, стрелка»;
// сама картинка — в CursorImageOverride.
func (e *Engine) CursorOverride() (widget.Cursor, bool) {
	return e.in.CursorOverride()
}

// CursorImageOverride возвращает курсор-картинку, заданную SetCursorImage.
// Значение неизменяемое, его можно хранить и читать с любой горутины.
func (e *Engine) CursorImageOverride() (*widget.CursorImage, bool) {
	return e.in.CursorImageOverride()
}

// SetOnCursorOverrideChanged подключает уведомление о том, что принудительный
// курсор (форма или картинка) задан, сменён или снят. Его ставит окно
// (window.Run): по уведомлению оно заново спрашивает CursorAt и отдаёт форму
// ОС. fn зовётся на горутине того, кто менял override, вне замков движка; nil
// снимает уведомление.
func (e *Engine) SetOnCursorOverrideChanged(fn func()) {
	e.in.SetOnCursorOverrideChanged(fn)
}

// RequestFocus отдаёт фокус виджету w так, как это делает просьба самого
// виджета во время доставки события (internal/focusreq): прежний владелец
// запоминается, и ReturnFocus вернёт ему фокус. Зовётся приложением там, где
// виджет открыт НЕ событием движка — панель, поднятая по глобальной клавише
// (Win) из сессии, по таймеру или из другой горутины: событие в такой миг никто
// не доставляет, и виджет сам фокус попросить не может.
//
// Безопасно из любой горутины. На горутине кадра (и у незапущенного движка)
// выполняется сразу, из других — ставится в очередь Post и выполнится в
// начале следующего прохода цикла, в порядке вызовов относительно
// ReturnFocus. Фокус при этом, как и у SetFocus, инвалидирует области старого
// и нового владельца. nil и w, уже владеющий фокусом, ничего не меняют.
func (e *Engine) RequestFocus(w widget.Widget) {
	e.in.RequestFocus(w)
}

// ReturnFocus возвращает фокус тому, у кого его забрал RequestFocus для w, —
// когда программно открытая панель закрылась. Если фокус успели передать
// дальше (щелчок по другому виджету), его не трогает. Потокобезопасно так же,
// как RequestFocus.
func (e *Engine) ReturnFocus(w widget.Widget) {
	e.in.ReturnFocus(w)
}
