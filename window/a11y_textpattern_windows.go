//go:build windows

// a11y_textpattern_windows.go — паттерн Text UI Automation: ITextProvider у
// поля ввода и документа и ITextRangeProvider для диапазонов внутри него.
//
// Зачем. Паттерн Value отдаёт содержимое одной строкой. Для однострочного
// поля этого хватает, для документа — нет: экранный диктор и NVDA читают
// многострочный текст кусками (символ, слово, строка), идут за кареткой и
// озвучивают выделение, а в одной строке ни каретки, ни выделения, ни границ
// строк нет. Без паттерна Text редактор объявлял весь текст заново на каждое
// нажатие клавиши.
//
// Как устроено. Элемент отдаёт ITextProvider (он живёт внутри uiaElement, как
// Invoke/Toggle/Value). Диапазон — ОТДЕЛЬНЫЙ COM-объект: пара позиций в рунах
// (a11yRange) плюс элемент, к которому он привязан. Вся арифметика диапазонов
// (сдвиг на единицы, сравнение концов, расширение) — в чистом
// a11y_textrange.go; здесь только COM: разбор аргументов, счётчики ссылок,
// SAFEARRAY и вызовы в виджет.
//
// Текст всегда спрашивается у ЖИВОГО виджета (widget.AccessTextProvider), а не
// из снимка семантики: снимок отстаёт на сто пятьдесят миллисекунд, а
// скринридер, прочитав слово под кареткой по устаревшему снимку, назвал бы
// слово, которое уже стёрто. Цена — перевод строки в руны на каждый вызов
// (O(длина текста)); для документов в сотни килобайт это незаметно, для
// мегабайтов — предмет будущей оптимизации.
//
// Диапазоны не следят за правками: позиции остаются прежними, а при каждом
// вызове усекаются до текущей длины текста (a11yClampRange). Системные поля
// двигают диапазоны вслед за вставкой; скринридеры же после любого
// TextChanged сами берут свежие диапазоны, так что на практике это не
// мешает, а защищает от паники на срезе.
//
// Что НЕ поддержано (честно):
//   - Атрибуты текста (GetAttributeValue, FindAttribute): на любой атрибут
//     отвечаем «не поддерживается» — законный ответ, формата в простом тексте
//     нет, а скринридер просто не объявляет шрифт и цвет.
//   - GetBoundingRectangles: точных прямоугольников символов виджет не
//     публикует, поэтому на любой диапазон отдаём ОДИН прямоугольник — весь
//     элемент. Увеличитель и подсветка диктора покажут поле целиком, а не
//     символ; пустой массив был бы хуже — клиенты читают его как «диапазон
//     невидим» и не показывают ничего.
//   - ScrollIntoView: интерфейса прокрутки у виджета нет, вызов принимается и
//     ничего не делает. Каретка при Select и так уезжает в видимую область.
//   - RangeFromPoint: символьной геометрии нет, поэтому возвращается пустой
//     диапазон на каретке — последнее известное место внимания.
//   - GetVisibleRanges: границы видимой области виджет не публикует; весь
//     текст считается видимым (один диапазон на документ).
//   - RangeFromChild: у текста нет встроенных объектов (картинок, ссылок),
//     поэтому у него нет и детей; GetChildren отдаёт пустой массив.
//   - Выделение единственное (SupportedTextSelection_Single): AddToSelection
//     работает, лишь пока выделения нет.
//   - Единицы Format и Page трактуются как Document (см. a11y_textrange.go).
package window

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/oops1/headless-gui/v3/widget"
	"golang.org/x/sys/windows"
)

const (
	// uiaPatternText — UIA_TextPatternId.
	uiaPatternText = 10014

	// События паттерна Text: по ним NVDA и диктор узнают, что каретка или
	// выделение сдвинулись, и перечитывают то место, где они теперь стоят.
	uiaEventTextSelectionChanged = 20014
	uiaEventTextChanged          = 20015

	// SupportedTextSelection_Single: выделение одно.
	uiaSelectionSingle = 1

	// Типы для SAFEARRAY.
	vtUnknown = 13

	// UIA_E_INVALIDOPERATION — операция не поддерживается в нынешнем состоянии.
	uiaEInvalidOperation = uintptr(0x80131509)
)

// ─── Поддержка паттерна ──────────────────────────────────────────────────────

// uiaSupportsText — отдавать ли паттерн Text у элемента.
//
// Условие тройное. Роль — та же, что у Value (поле ввода и документ). Виджет
// обязан уметь отдавать текст (AccessTextProvider): без него паттерн нечем
// кормить, а пустой ITextProvider заставил бы диктор объявить «пустой
// документ» у поля, в котором на самом деле что-то есть. И это не поле
// пароля: системное поле с ES_PASSWORD паттерн Text не отдаёт совсем, и мы
// следуем ему — иначе скринридер читал бы пароль по символам.
func uiaSupportsText(e *uiaElement) bool {
	if e == nil {
		return false
	}
	node := e.node()
	if node == nil || node.Widget == nil || !uiaSupportsValue(node.Info.Role) {
		return false
	}
	if a11yHasState(node.Info.States, widget.StatePassword) {
		return false
	}
	_, ok := widget.AccessTextOf(node.Widget)
	return ok
}

// outSimple отдаёт IRawElementProviderSimple элемента наружу (с AddRef).
func (e *uiaElement) outSimple() uintptr {
	e.refs.Add(1)
	return e.simplePtr()
}

// ─── Диапазон как COM-объект ─────────────────────────────────────────────────

// uiaTextRange — COM-объект диапазона. Первое слово — указатель на vtable,
// АДРЕС этого поля и есть COM-указатель (как у uiaElement).
//
// ВРЕМЯ ЖИЗНИ И ОСВОБОЖДЕНИЕ. Элементы дерева живут вместе с мостом и по
// Release не освобождаются: их немного, и они переиспользуются. Диапазонов же
// скринридер создаёт тысячи (на каждый шаг чтения — несколько), так что
// «живут до закрытия окна» означало бы утечку, растущую с каждой прочитанной
// строкой. Поэтому здесь Release до нуля НАСТОЯЩИЙ: объект снимается с
// регистрации (uiaRanges), и сборщик мусора его забирает.
//
// Почему это безопасно:
//
//  1. COM-указатель, который держит клиент, — просто число. Ни один метод не
//     разыменовывает this: он идёт в карту uiaRanges и работает с найденным
//     Go-объектом. Освобождённый и снятый с регистрации объект карта не найдёт
//     — вызов на «висячем» указателе (ошибка клиента) закончится E_FAIL, а не
//     чтением чужой памяти.
//  2. Пока объект зарегистрирован, карта держит на него Go-ссылку, и память
//     не соберут. Вызов, который УЖЕ идёт (метод взял указатель из карты и
//     работает), тоже держит ссылку на Go-стеке — Release из другого потока
//     вычеркнет объект из карты, но память до конца вызова живёт.
//  3. AddRef/QueryInterface не воскрешают умерший объект: счётчик
//     увеличивается только пока он положителен (compare-and-swap). Клиент,
//     соблюдающий правила COM, держит свою ссылку и сам до нуля не доведёт;
//     гонка «последний Release против чужого AddRef» возможна лишь у
//     клиента, который нарушает контракт.
//  4. Элементы не освобождаются, поэтому ссылка на элемент внутри диапазона
//     (elem) всегда годна; AddRef на элемент для неё не нужен.
//  5. Последний оставшийся риск — повторное использование адреса: после
//     сборки объекта новый диапазон может получить тот же адрес, и «висячий»
//     указатель клиента попадёт на него. Это нарушение контракта COM клиентом
//     (вызов после последнего Release) и к порче памяти не ведёт — только к
//     тому, что вызов обслужит чужой диапазон.
//
// Если клиент не отпустил диапазоны, а окно закрыто, мост снимает их при
// остановке (uiaForgetRangesOf): иначе они жили бы до конца процесса.
type uiaTextRange struct {
	vt uintptr // указатель на vtable — первое слово COM-объекта

	// refs — счётчик ссылок COM. Атомарный: Release приходит из потоков UIA.
	// Ноль означает «освобождён».
	refs atomic.Int32

	elem *uiaElement

	mu sync.Mutex
	r  a11yRange
}

var (
	uiaRangeMu sync.RWMutex
	uiaRanges  = map[uintptr]*uiaTextRange{} // адрес поля-vtable → диапазон
)

// uiaLookupRange находит живой диапазон по COM-указателю; nil — указатель
// чужой или объект уже освобождён.
func uiaLookupRange(this uintptr) *uiaTextRange {
	uiaRangeMu.RLock()
	tr := uiaRanges[this]
	uiaRangeMu.RUnlock()
	return tr
}

// uiaLiveRanges — сколько диапазонов сейчас зарегистрировано (для тестов
// освобождения: утечка проявляется ростом этого числа).
func uiaLiveRanges() int {
	uiaRangeMu.RLock()
	defer uiaRangeMu.RUnlock()
	return len(uiaRanges)
}

// newTextRange создаёт диапазон элемента со счётчиком ссылок 1 — эта ссылка
// принадлежит тому, кто его получил, и уходит клиенту в out-параметре.
func (e *uiaElement) newTextRange(r a11yRange) *uiaTextRange {
	tr := &uiaTextRange{elem: e, r: r}
	tr.vt = uiaTextRangeVTable()
	tr.refs.Store(1)
	uiaRangeMu.Lock()
	uiaRanges[tr.ptr()] = tr
	uiaRangeMu.Unlock()
	return tr
}

func (tr *uiaTextRange) ptr() uintptr { return uintptr(unsafe.Pointer(&tr.vt)) }

// tryAddRef увеличивает счётчик, если объект ещё жив; false — уже освобождён.
func (tr *uiaTextRange) tryAddRef() (int32, bool) {
	for {
		n := tr.refs.Load()
		if n <= 0 {
			return 0, false
		}
		if tr.refs.CompareAndSwap(n, n+1) {
			return n + 1, true
		}
	}
}

// uiaForgetRangesOf снимает с регистрации все диапазоны моста — при его
// остановке, когда окно закрыто, а клиент свои ссылки так и не отпустил.
func uiaForgetRangesOf(b *uiaBridge) {
	uiaRangeMu.Lock()
	for k, tr := range uiaRanges {
		if tr.elem != nil && tr.elem.b == b {
			delete(uiaRanges, k)
			tr.refs.Store(0)
		}
	}
	uiaRangeMu.Unlock()
}

// get/set — доступ к позициям под замком диапазона.
func (tr *uiaTextRange) get() a11yRange {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.r
}

func (tr *uiaTextRange) set(r a11yRange) {
	tr.mu.Lock()
	tr.r = r
	tr.mu.Unlock()
}

// load возвращает живой текст элемента рунами и диапазон, усечённый до него.
// ok=false — элемента (или текста у него) уже нет.
func (tr *uiaTextRange) load() ([]rune, a11yRange, widget.AccessTextProvider, bool) {
	tp, ok := uiaTextOf(tr.elem)
	if !ok {
		return nil, a11yRange{}, nil, false
	}
	rs := []rune(tp.AccessText())
	return rs, a11yClampRange(tr.get(), len(rs)), tp, true
}

// ─── Таблицы виртуальных методов ─────────────────────────────────────────────

var (
	uiaTextVTOnce  sync.Once
	uiaVTText      uintptr
	uiaVTTextRange uintptr
)

func uiaInitTextVTables() {
	uiaTextVTOnce.Do(func() {
		uiaInitVTables()
		// IUnknown элемента для ITextProvider — общий с остальными паттернами
		// (счётчик ссылок один на элемент).
		qi := windows.NewCallback(uiaQueryInterface)
		addRef := windows.NewCallback(uiaAddRef)
		release := windows.NewCallback(uiaRelease)

		// ITextProvider: порядок методов — по определению интерфейса.
		uiaVTText = newVTable(qi, addRef, release,
			windows.NewCallback(uiaTextGetSelection),
			windows.NewCallback(uiaTextGetVisibleRanges),
			windows.NewCallback(uiaTextRangeFromChild),
			windows.NewCallback(uiaTextRangeFromPoint),
			windows.NewCallback(uiaTextGetDocumentRange),
			windows.NewCallback(uiaTextGetSupportedSelection),
		)

		// ITextRangeProvider: у диапазона СВОЙ IUnknown — у него свой счётчик.
		uiaVTTextRange = newVTable(
			windows.NewCallback(uiaRangeQueryInterface),
			windows.NewCallback(uiaRangeAddRef),
			windows.NewCallback(uiaRangeRelease),
			windows.NewCallback(uiaRangeClone),
			windows.NewCallback(uiaRangeCompare),
			windows.NewCallback(uiaRangeCompareEndpoints),
			windows.NewCallback(uiaRangeExpandToEnclosingUnit),
			windows.NewCallback(uiaRangeFindAttribute),
			windows.NewCallback(uiaRangeFindText),
			windows.NewCallback(uiaRangeGetAttributeValue),
			windows.NewCallback(uiaRangeGetBoundingRectangles),
			windows.NewCallback(uiaRangeGetEnclosingElement),
			windows.NewCallback(uiaRangeGetText),
			windows.NewCallback(uiaRangeMove),
			windows.NewCallback(uiaRangeMoveEndpointByUnit),
			windows.NewCallback(uiaRangeMoveEndpointByRange),
			windows.NewCallback(uiaRangeSelect),
			windows.NewCallback(uiaRangeAddToSelection),
			windows.NewCallback(uiaRangeRemoveFromSelection),
			windows.NewCallback(uiaRangeScrollIntoView),
			windows.NewCallback(uiaRangeGetChildren),
		)
	})
}

func uiaTextVTable() uintptr      { uiaInitTextVTables(); return uiaVTText }
func uiaTextRangeVTable() uintptr { uiaInitTextVTables(); return uiaVTTextRange }

// ─── SAFEARRAY ───────────────────────────────────────────────────────────────

// safeArrayOfRanges создаёт SAFEARRAY(VT_UNKNOWN) из диапазонов — так UIA
// принимает массив ITextRangeProvider. SafeArrayPutElement для VT_UNKNOWN
// берёт собственную ссылку (AddRef), поэтому свои ссылки на диапазоны
// отпускаем: владельцем остаётся массив, а потом клиент, который его
// уничтожит. Возвращает 0, если массив создать не удалось (ссылки при этом
// тоже отпущены).
func safeArrayOfRanges(ranges []*uiaTextRange) uintptr {
	release := func() {
		for _, tr := range ranges {
			uiaRangeRelease(tr.ptr())
		}
	}
	sa, _, _ := procSafeArrayCreateVector.Call(uintptr(vtUnknown), 0, uintptr(len(ranges)))
	if sa == 0 {
		release()
		return 0
	}
	for i, tr := range ranges {
		idx := int32(i)
		// Для VT_UNKNOWN pv — сам указатель интерфейса, без второго уровня.
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)), tr.ptr())
	}
	release()
	return sa
}

// safeArrayOfDoubles создаёт SAFEARRAY(VT_R8) — так UIA принимает
// прямоугольники диапазона (четвёрки left, top, width, height).
func safeArrayOfDoubles(vals []float64) uintptr {
	sa, _, _ := procSafeArrayCreateVector.Call(uintptr(vtR8), 0, uintptr(len(vals)))
	if sa == 0 {
		return 0
	}
	for i := range vals {
		idx := int32(i)
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)),
			uintptr(unsafe.Pointer(&vals[i])))
	}
	return sa
}

// ─── ITextProvider (методы элемента) ─────────────────────────────────────────

// uiaTextElement находит элемент по COM-указателю ITextProvider и проверяет,
// что текст у него ещё есть: дерево могло перестроиться, а клиент держит
// старый указатель.
func uiaTextElement(this uintptr) (*uiaElement, widget.AccessTextProvider, uintptr) {
	e := uiaLookup(this)
	if e == nil {
		return nil, nil, eFail
	}
	tp, ok := uiaTextOf(e)
	if !ok {
		return nil, nil, uiaEElementNotAvailable
	}
	return e, tp, sOK
}

// uiaCurrentSelection — выделение виджета, усечённое до длины текста. Без
// выделения — пустой диапазон на каретке: так UIA и ждёт его в GetSelection.
func uiaCurrentSelection(tp widget.AccessTextProvider, n int) a11yRange {
	from, to := tp.AccessSelection()
	return a11yClampRange(a11yRange{from, to}, n)
}

// uiaTextGetSelection — ITextProvider::GetSelection. Выделение одно, поэтому
// массив из одного диапазона; без выделения — пустой диапазон на каретке.
func uiaTextGetSelection(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e, tp, hr := uiaTextElement(this)
	if hr != sOK {
		return hr
	}
	n := len([]rune(tp.AccessText()))
	sel := uiaCurrentSelection(tp, n)
	if sa := safeArrayOfRanges([]*uiaTextRange{e.newTextRange(sel)}); sa != 0 {
		*out = sa
		uiaLog("Text.GetSelection(%d) → [%d,%d)", e.id, sel.Start, sel.End)
		return sOK
	}
	return eFail
}

// uiaTextGetVisibleRanges — ITextProvider::GetVisibleRanges. Границ видимой
// области виджет не публикует: считаем видимым весь текст.
func uiaTextGetVisibleRanges(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e, tp, hr := uiaTextElement(this)
	if hr != sOK {
		return hr
	}
	n := len([]rune(tp.AccessText()))
	if sa := safeArrayOfRanges([]*uiaTextRange{e.newTextRange(a11yRange{0, n})}); sa != 0 {
		*out = sa
		return sOK
	}
	return eFail
}

// uiaTextRangeFromChild — ITextProvider::RangeFromChild. Встроенных объектов
// у простого текста нет, а значит, нет и детей, чей диапазон можно вернуть.
func uiaTextRangeFromChild(this uintptr, child uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	return eInvalidArg
}

// uiaTextRangeFromPoint — ITextProvider::RangeFromPoint.
//
// Точка приходит структурой UiaPoint (два double, 16 байт): на x64 такие
// структуры передаются по ССЫЛКЕ в целочисленном регистре, так что Go-колбэк
// её видит (в отличие от двух отдельных double у ElementProviderFromPoint, см.
// uiaElementProviderFromPoint). Но символьной геометрии у виджета нет,
// определить по точке позицию нечем — отдаём пустой диапазон на каретке.
func uiaTextRangeFromPoint(this uintptr, pt uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e, tp, hr := uiaTextElement(this)
	if hr != sOK {
		return hr
	}
	n := len([]rune(tp.AccessText()))
	sel := uiaCurrentSelection(tp, n)
	*out = e.newTextRange(a11yRange{sel.End, sel.End}).ptr()
	return sOK
}

// uiaTextGetDocumentRange — ITextProvider::get_DocumentRange: весь текст.
func uiaTextGetDocumentRange(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	e, tp, hr := uiaTextElement(this)
	if hr != sOK {
		return hr
	}
	n := len([]rune(tp.AccessText()))
	*out = e.newTextRange(a11yRange{0, n}).ptr()
	uiaLog("Text.DocumentRange(%d) → [0,%d)", e.id, n)
	return sOK
}

// uiaTextGetSupportedSelection — ITextProvider::get_SupportedTextSelection.
func uiaTextGetSupportedSelection(this uintptr, out *int32) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = uiaSelectionSingle
	return sOK
}

// ─── IUnknown диапазона ──────────────────────────────────────────────────────

func uiaRangeQueryInterface(this uintptr, riid *comGUID, ppv *uintptr) uintptr {
	if ppv == nil || riid == nil {
		return eInvalidArg
	}
	*ppv = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eNoInterface
	}
	if !riid.equals(&iidIUnknown) && !riid.equals(&iidTextRangeProvider) {
		return eNoInterface
	}
	if _, ok := tr.tryAddRef(); !ok {
		return eNoInterface
	}
	*ppv = tr.ptr()
	return sOK
}

func uiaRangeAddRef(this uintptr) uintptr {
	if tr := uiaLookupRange(this); tr != nil {
		if n, ok := tr.tryAddRef(); ok {
			return uintptr(n)
		}
	}
	return 0
}

// uiaRangeRelease уменьшает счётчик и при нуле СНИМАЕТ диапазон с
// регистрации — см. «Время жизни» у uiaTextRange.
func uiaRangeRelease(this uintptr) uintptr {
	tr := uiaLookupRange(this)
	if tr == nil {
		return 0 // уже освобождён: повторный Release не должен ничего портить
	}
	n := tr.refs.Add(-1)
	if n > 0 {
		return uintptr(n)
	}
	tr.refs.Store(0)
	uiaRangeMu.Lock()
	delete(uiaRanges, this)
	uiaRangeMu.Unlock()
	return 0
}

// ─── ITextRangeProvider ──────────────────────────────────────────────────────

// uiaRangeClone — Clone: независимая копия с теми же границами.
func uiaRangeClone(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	*out = tr.elem.newTextRange(tr.get()).ptr()
	return sOK
}

// uiaRangeCompare — Compare: одинаковы ли диапазоны (тот же элемент, те же
// границы). Диапазон другого элемента — не ошибка, а «не равен».
func uiaRangeCompare(this uintptr, other uintptr, out *int32) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	o := uiaLookupRange(other)
	if o == nil {
		return eInvalidArg
	}
	if o.elem != tr.elem {
		return sOK
	}
	rs, r, _, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	if r == a11yClampRange(o.get(), len(rs)) {
		*out = 1
	}
	return sOK
}

// uiaRangeCompareEndpoints — CompareEndpoints.
func uiaRangeCompareEndpoints(this uintptr, ep int32, other uintptr, oep int32, out *int32) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	o := uiaLookupRange(other)
	if o == nil || o.elem != tr.elem || !a11yEndpoint(ep).valid() || !a11yEndpoint(oep).valid() {
		return eInvalidArg
	}
	rs, r, _, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	*out = int32(a11yCompareEndpoints(r, a11yEndpoint(ep), a11yClampRange(o.get(), len(rs)), a11yEndpoint(oep)))
	return sOK
}

// uiaRangeExpandToEnclosingUnit — ExpandToEnclosingUnit.
func uiaRangeExpandToEnclosingUnit(this uintptr, unit int32) uintptr {
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	if !a11yTextUnit(unit).valid() {
		return eInvalidArg
	}
	rs, r, _, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	tr.set(a11yExpandToUnit(rs, r, a11yTextUnit(unit)))
	return sOK
}

// uiaRangeFindAttribute — FindAttribute. Ни один атрибут не поддерживается
// (см. GetAttributeValue), поэтому диапазона с заданным значением атрибута
// быть не может: «не найдено» — NULL и S_OK.
//
// Значение VARIANT (24 байта) приходит по ссылке, как любая структура больше
// восьми байт на x64; читать его не нужно.
func uiaRangeFindAttribute(this uintptr, attr int32, val uintptr, backward int32, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	if uiaLookupRange(this) == nil {
		return eFail
	}
	return sOK
}

// uiaRangeFindText — FindText: поиск строки внутри диапазона.
func uiaRangeFindText(this uintptr, bstr uintptr, backward, ignoreCase int32, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	needle := bstrToString(bstr)
	if needle == "" {
		return eInvalidArg
	}
	rs, r, _, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	if found, ok := a11yFindText(rs, r, needle, backward != 0, ignoreCase != 0); ok {
		*out = tr.elem.newTextRange(found).ptr()
	}
	return sOK
}

// uiaRangeGetAttributeValue — GetAttributeValue: любой атрибут отвечает
// «не поддерживается» — особым синглтоном UIA (UiaGetReservedNotSupportedValue).
// Это законный ответ: клиент не объявляет атрибут, а не считает провайдер
// сломанным. Пустой VARIANT вместо него означал бы «значение атрибута —
// пустое», то есть неправду.
func uiaRangeGetAttributeValue(this uintptr, attr int32, out *comVariant) uintptr {
	if out == nil {
		return eInvalidArg
	}
	out.setEmpty()
	if uiaLookupRange(this) == nil {
		return eFail
	}
	if uiaCore.Load() != nil || procUiaGetReservedNotSupported.Find() != nil {
		return eNotImpl
	}
	var punk uintptr
	if hr, _, _ := procUiaGetReservedNotSupported.Call(uintptr(unsafe.Pointer(&punk))); hr != 0 || punk == 0 {
		return eNotImpl
	}
	// Синглтон статический: AddRef не нужен, клиентский VariantClear его
	// Release'ом ничего не сломает.
	out.vt = vtUnknown
	out.val = [2]uintptr{punk, 0}
	return sOK
}

// uiaRangeGetBoundingRectangles — GetBoundingRectangles: один прямоугольник —
// весь элемент (почему не точный и не пустой массив — см. заголовок файла).
func uiaRangeGetBoundingRectangles(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	rc := tr.elem.b.boundsOf(tr.elem.id)
	sa := safeArrayOfDoubles([]float64{rc.left, rc.top, rc.width, rc.height})
	if sa == 0 {
		return eFail
	}
	*out = sa
	return sOK
}

// uiaRangeGetEnclosingElement — GetEnclosingElement: элемент, внутри которого
// лежит диапазон. Вложенных элементов у текста нет — это сам элемент.
func uiaRangeGetEnclosingElement(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	*out = tr.elem.outSimple()
	return sOK
}

// uiaRangeGetText — GetText: текст диапазона, не длиннее maxLen символов
// (-1 — без ограничения). Усечение считается в рунах; для символов за
// пределами BMP UIA считает в UTF-16, но расхождение на длине усечения
// (клиент просит «не больше N») безвредно.
func uiaRangeGetText(this uintptr, maxLen int32, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	if maxLen < -1 {
		return eInvalidArg
	}
	rs, r, _, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	text := a11yRangeText(rs, r, int(maxLen))
	var v comVariant
	v.setString(text)
	*out = v.val[0] // BSTR уходит клиенту во владение (пустая строка — NULL)
	return sOK
}

// uiaRangeMove — Move: сдвиг всего диапазона на count единиц.
func uiaRangeMove(this uintptr, unit, count int32, moved *int32) uintptr {
	if moved == nil {
		return eInvalidArg
	}
	*moved = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	if !a11yTextUnit(unit).valid() {
		return eInvalidArg
	}
	rs, r, _, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	nr, n := a11yMoveRange(rs, r, a11yTextUnit(unit), int(count))
	tr.set(nr)
	*moved = int32(n)
	return sOK
}

// uiaRangeMoveEndpointByUnit — MoveEndpointByUnit: сдвиг одного конца.
func uiaRangeMoveEndpointByUnit(this uintptr, ep, unit, count int32, moved *int32) uintptr {
	if moved == nil {
		return eInvalidArg
	}
	*moved = 0
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	if !a11yEndpoint(ep).valid() || !a11yTextUnit(unit).valid() {
		return eInvalidArg
	}
	rs, r, _, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	nr, n := a11yMoveEndpoint(rs, r, a11yEndpoint(ep), a11yTextUnit(unit), int(count))
	tr.set(nr)
	*moved = int32(n)
	return sOK
}

// uiaRangeMoveEndpointByRange — MoveEndpointByRange: конец этого диапазона
// переезжает на конец другого.
func uiaRangeMoveEndpointByRange(this uintptr, ep int32, target uintptr, tep int32) uintptr {
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	o := uiaLookupRange(target)
	if o == nil || o.elem != tr.elem || !a11yEndpoint(ep).valid() || !a11yEndpoint(tep).valid() {
		return eInvalidArg
	}
	rs, r, _, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	other := a11yClampRange(o.get(), len(rs))
	tr.set(a11yMoveEndpointToRange(r, a11yEndpoint(ep), other, a11yEndpoint(tep)))
	return sOK
}

// uiaSelectRange выделяет диапазон в виджете — на горутине движка: дерево
// виджетов принадлежит ей, а вызов приходит из потока UIA (как в uiaSetValue).
//
// Непустой диапазон выделяется, если виджет принимает выделение
// (AccessSelectionSetter); иначе остаётся поставить каретку на его конец —
// лучше, чем отказ: скринридер хотя бы переведёт курсор. Пустой диапазон —
// просто каретка.
func uiaSelectRange(e *uiaElement, tp widget.AccessTextProvider, r a11yRange) uintptr {
	sel, canSel := tp.(widget.AccessSelectionSetter)
	caret, canCaret := tp.(widget.AccessCaretSetter)
	if !canSel && !canCaret {
		return eNotImpl
	}
	e.b.win.postToEngine(func() {
		switch {
		case canSel:
			sel.AccessSetSelection(r.Start, r.End)
		case r.Empty():
			caret.AccessSetCaret(r.Start)
		default:
			caret.AccessSetCaret(r.End)
		}
	})
	e.b.markDirty() // каретка переехала — пора поднять события
	return sOK
}

// uiaRangeSelect — Select.
func uiaRangeSelect(this uintptr) uintptr {
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	_, r, tp, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	uiaLog("Range.Select(%d, [%d,%d))", tr.elem.id, r.Start, r.End)
	return uiaSelectRange(tr.elem, tp, r)
}

// uiaRangeAddToSelection — AddToSelection. Выделение единственное: добавить
// можно, лишь пока выделения нет (тогда это обычный Select); иначе — штатный
// отказ UIA_E_INVALIDOPERATION.
func uiaRangeAddToSelection(this uintptr) uintptr {
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	rs, r, tp, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	if !uiaCurrentSelection(tp, len(rs)).Empty() {
		return uiaEInvalidOperation
	}
	return uiaSelectRange(tr.elem, tp, r)
}

// uiaRangeRemoveFromSelection — RemoveFromSelection: если диапазон — и есть
// выделение, оно снимается (каретка остаётся в его начале). Диапазон, не
// совпадающий с выделением, ничего не убирает: «исключить из выделения то,
// чего в нём нет» — не ошибка.
func uiaRangeRemoveFromSelection(this uintptr) uintptr {
	tr := uiaLookupRange(this)
	if tr == nil {
		return eFail
	}
	rs, r, tp, ok := tr.load()
	if !ok {
		return uiaEElementNotAvailable
	}
	sel := uiaCurrentSelection(tp, len(rs))
	if sel.Empty() || sel != r {
		return sOK
	}
	return uiaSelectRange(tr.elem, tp, a11yRange{sel.Start, sel.Start})
}

// uiaRangeScrollIntoView — ScrollIntoView: см. заголовок файла, вызов
// принимается и ничего не делает.
func uiaRangeScrollIntoView(this uintptr, alignToTop int32) uintptr {
	if uiaLookupRange(this) == nil {
		return eFail
	}
	return sOK
}

// uiaRangeGetChildren — GetChildren: встроенных объектов нет, массив пуст.
func uiaRangeGetChildren(this uintptr, out *uintptr) uintptr {
	if out == nil {
		return eInvalidArg
	}
	*out = 0
	if uiaLookupRange(this) == nil {
		return eFail
	}
	sa := safeArrayOfRanges(nil)
	if sa == 0 {
		return eFail
	}
	*out = sa
	return sOK
}

// ─── События паттерна Text ───────────────────────────────────────────────────

// emitTextEvents поднимает события паттерна Text для поля с фокусом: сдвинулась
// каретка или выделение — TextSelectionChanged, изменился текст — TextChanged.
//
// Без них скринридер читает поле лишь при получении фокуса, а при движении
// каретки молчит: перечитывать ему нечего, потому что никто не сказал, что
// каретка сдвинулась. Снимок семантики для этого не годится: он о каретке не
// знает, поэтому состояние сравнивается тут, по живому виджету.
//
// Вызывается из цикла событий моста после emitChanges; сам по себе ничего не
// стоит, пока ни один клиент не слушает (цикл тогда не доходит до вызова).
func (b *uiaBridge) emitTextEvents() {
	f := b.focusElement()
	if f == nil || !uiaSupportsText(f) {
		b.textSeen = nil
		return
	}
	tp, ok := uiaTextOf(f)
	if !ok {
		b.textSeen = nil
		return
	}
	cur := textStateOf(tp)
	prev := b.textSeen
	b.textSeen = &cur
	if prev == nil || b.textSeenID != f.id {
		b.textSeenID = f.id
		return // фокус только что пришёл сюда: его объявит событие фокуса
	}
	textCh, selCh := a11yTextDiff(*prev, cur)
	if procUiaRaiseAutomationEvent.Find() != nil {
		return
	}
	if textCh {
		procUiaRaiseAutomationEvent.Call(f.simplePtr(), uiaEventTextChanged)
	}
	if selCh || textCh {
		// Набор текста двигает каретку, и клиент ждёт события в обоих случаях.
		procUiaRaiseAutomationEvent.Call(f.simplePtr(), uiaEventTextSelectionChanged)
	}
}

// textStateOf — текущее текстовое состояние виджета: то, что сравнивается
// между проходами цикла событий.
func textStateOf(tp widget.AccessTextProvider) a11yTextState {
	from, to := tp.AccessSelection()
	return a11yTextState{Text: tp.AccessText(), Caret: tp.AccessCaret(), SelFrom: from, SelTo: to}
}
