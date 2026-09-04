package widget

// measure.go — замер ширины текста для кода компоновки, работающего ДО
// отрисовки (диалоги вычисляют свой размер при создании, когда DrawContext
// ещё недоступен). Движок регистрирует измеритель при старте
// (SetTextMeasurer); до регистрации действует грубая эвристика.

import (
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// textMeasurer — функция замера ширины строки шрифтом default (px).
type textMeasurer func(text string, sizePt float64) int

var uiTextMeasurer atomic.Value // textMeasurer

// Измеритель, в отличие от прочего состояния конвейера, один на процесс —
// иначе и быть не может: MeasureUIText зовут из кода компоновки, у которого
// нет ни движка, ни контекста отрисовки (диалог считает свой размер при
// создании). Отвечает последний зарегистрированный.
//
// Реестр, а не одна переменная, нужен для снятия: движок регистрирует
// измеритель при старте и снимает в Stop. Без реестра остановленный движок
// оставлял бы измерителем свой мёртвый холст, и следующие замеры шли бы по
// шрифтам движка, которого уже нет. Со снятием управление возвращается
// предыдущему живому.
var (
	measurersMu  sync.Mutex
	measurers    []registeredMeasurer
	measurerSeq  uint64
	baseMeasurer textMeasurer // заданный через SetTextMeasurer, без дескриптора
)

type registeredMeasurer struct {
	handle uint64
	fn     textMeasurer
	set    Measurers
}

// Measurers — набор точных измерителей, который регистрирует движок.
//
// Их три, потому что разметку считают в трёх разных случаях: обычная подпись
// шрифтом темы, подпись ИМЕНОВАННЫМ шрифтом (моноширинный текст diff-виджета,
// жирный заголовок) и позиции символов для каретки и выделения.
//
// Text обязателен; остальные необязательны — измеритель без них просто не
// отвечает на соответствующие вопросы, и вызывающий получает эвристику.
type Measurers struct {
	// Text — ширина строки шрифтом по умолчанию.
	Text func(text string, sizePt float64) int
	// TextFont — ширина строки ИМЕНОВАННЫМ шрифтом. Пустое имя означает
	// шрифт по умолчанию.
	TextFont func(text string, sizePt float64, family string) int
	// RunePositions — накопленная ширина после каждого символа
	// (len(runes)+1 значений, первое — ноль).
	RunePositions func(text string, sizePt float64, family string) []int
}

// SetTextMeasurer регистрирует точный измеритель текста без дескриптора.
//
// Оставлен для потребителей, которые ставят свой измеритель раз и навсегда.
// Движок пользуется RegisterTextMeasurer: тот умеет сниматься.
func SetTextMeasurer(m func(text string, sizePt float64) int) {
	if m == nil {
		return
	}
	measurersMu.Lock()
	baseMeasurer = textMeasurer(m)
	measurersMu.Unlock()
	publishMeasurer()
}

// RegisterTextMeasurer добавляет измеритель и делает его действующим.
// Возвращает дескриптор для UnregisterTextMeasurer.
func RegisterTextMeasurer(m func(text string, sizePt float64) int) uint64 {
	return RegisterMeasurers(Measurers{Text: m})
}

// RegisterMeasurers регистрирует полный набор измерителей и возвращает
// дескриптор для UnregisterTextMeasurer.
//
// Этим пользуется движок: холст умеет мерить и именованным шрифтом, и по
// символам, а до этого набора наружу выходил только замер шрифтом по
// умолчанию. Из-за этого раскладка, посчитанная ВНЕ отрисовки, считалась не
// тем шрифтом, которым потом рисовали, — колонки моноширинного текста
// разъезжались.
func RegisterMeasurers(set Measurers) uint64 {
	if set.Text == nil {
		return 0
	}
	measurersMu.Lock()
	measurerSeq++
	h := measurerSeq
	measurers = append(measurers, registeredMeasurer{
		handle: h, fn: textMeasurer(set.Text), set: set,
	})
	measurersMu.Unlock()
	publishMeasurer()
	return h
}

// activeSet возвращает действующий набор измерителей.
func activeSet() Measurers {
	measurersMu.Lock()
	defer measurersMu.Unlock()
	if n := len(measurers); n > 0 {
		return measurers[n-1].set
	}
	return Measurers{Text: baseMeasurer}
}

// MeasureUITextFont возвращает ширину строки ИМЕНОВАННЫМ шрифтом — вне
// отрисовки, там же, где работает MeasureUIText.
//
// Ради этого запрос и подан: MeasureUIText меряет шрифтом по умолчанию, а
// DrawTextFont рисует указанным семейством. Пока моноширинный шрифт не
// зарегистрирован, он подменяется дефолтным и разницы не видно; как только
// появится — раскладка, посчитанная не тем шрифтом, разъедется.
//
// Пустое имя семейства означает шрифт по умолчанию: тогда это ровно
// MeasureUIText. Движок без зарегистрированного набора отвечает той же
// эвристикой, что и MeasureUIText, — раскладка будет грубой, но не сломанной.
func MeasureUITextFont(text string, sizePt float64, family string) int {
	if family == "" {
		return MeasureUIText(text, sizePt)
	}
	set := activeSet()
	if set.TextFont == nil {
		return MeasureUIText(text, sizePt)
	}

	rev := TextMetricsRev()
	if w, ok := fontMemoGet(rev, family, sizePt, text); ok {
		return w
	}
	w := set.TextFont(text, sizePt, family)
	fontMemoStore(rev, family, sizePt, text, w)
	return w
}

// MeasureUIRunePositions возвращает накопленную ширину после каждого символа
// (len(runes)+1 значений, первое — ноль) — вне отрисовки.
//
// Нужна для попадания курсора и выделения в тексте, разложенном до кадра.
// Пустое имя семейства — шрифт по умолчанию. Без зарегистрированного набора
// возвращает nil: врать позициями хуже, чем честно сказать «не знаю».
func MeasureUIRunePositions(text string, sizePt float64, family string) []int {
	set := activeSet()
	if set.RunePositions == nil {
		return nil
	}
	return set.RunePositions(text, sizePt, family)
}

// Мемоизация замеров ИМЕНОВАННЫМ шрифтом — отдельная от основной.
//
// Отдельная намеренно: у основной ключ «кегль и текст», и вплетать в него имя
// семейства значило бы удлинить самый горячий путь замера ради случая, когда
// шрифт задан явно. Здесь ключ — семейство, кегль и текст.
var (
	fontMemoMu    sync.Mutex
	fontMemo      map[string]map[float64]map[string]int
	fontMemoRev   uint64
	fontMemoCount int
)

func fontMemoGet(rev uint64, family string, sizePt float64, text string) (int, bool) {
	fontMemoMu.Lock()
	defer fontMemoMu.Unlock()
	if fontMemoRev != rev {
		fontMemo, fontMemoCount, fontMemoRev = nil, 0, rev
		return 0, false
	}
	w, ok := fontMemo[family][sizePt][text]
	return w, ok
}

func fontMemoStore(rev uint64, family string, sizePt float64, text string, w int) {
	fontMemoMu.Lock()
	defer fontMemoMu.Unlock()
	if fontMemoRev != rev {
		return
	}
	if fontMemoCount >= measureMemoMax {
		fontMemo, fontMemoCount = nil, 0
	}
	if fontMemo == nil {
		fontMemo = make(map[string]map[float64]map[string]int, 2)
	}
	bySize := fontMemo[family]
	if bySize == nil {
		bySize = make(map[float64]map[string]int, 4)
		fontMemo[family] = bySize
	}
	byText := bySize[sizePt]
	if byText == nil {
		byText = make(map[string]int, 64)
		bySize[sizePt] = byText
	}
	if _, dup := byText[text]; !dup {
		fontMemoCount++
	}
	byText[text] = w
}

// UnregisterTextMeasurer снимает измеритель по дескриптору. Действующим
// становится предыдущий живой. Идемпотентно.
func UnregisterTextMeasurer(handle uint64) {
	if handle == 0 {
		return
	}
	measurersMu.Lock()
	for i := range measurers {
		if measurers[i].handle == handle {
			measurers = append(measurers[:i], measurers[i+1:]...)
			break
		}
	}
	measurersMu.Unlock()
	publishMeasurer()
}

// publishMeasurer выставляет действующий измеритель — последний
// зарегистрированный, иначе заданный через SetTextMeasurer.
func publishMeasurer() {
	measurersMu.Lock()
	var active textMeasurer
	if n := len(measurers); n > 0 {
		active = measurers[n-1].fn
	} else {
		active = baseMeasurer
	}
	measurersMu.Unlock()

	if active == nil {
		// Возврата к эвристике нет: снять последний измеритель и остаться
		// без него — редкость (все движки остановлены), а хранить в
		// atomic.Value типизированный nil нельзя, Load вернул бы его как
		// годный.
		return
	}
	uiTextMeasurer.Store(active)
	BumpTextMetricsRev() // прежние замеры больше не годятся
}

// Мемоизация замеров: ключ — кегль и текст; сброс при смене ревизии метрик
// и при переполнении.
const measureMemoMax = 4096

var (
	measureMu    sync.Mutex
	measureMemo  map[float64]map[string]int
	measureRev   uint64
	measureCount int
)

// MeasureUIText возвращает ширину строки в пикселях шрифтом default.
// До регистрации измерителя — эвристика ~0.65·sizePt на символ.
func MeasureUIText(text string, sizePt float64) int {
	m, ok := uiTextMeasurer.Load().(textMeasurer)
	if !ok {
		return int(float64(len([]rune(text))) * sizePt * 0.65)
	}
	rev := TextMetricsRev()
	measureMu.Lock()
	if measureRev != rev {
		measureMemo, measureCount, measureRev = nil, 0, rev
	}
	bySize := measureMemo[sizePt]
	if w, hit := bySize[text]; hit {
		measureMu.Unlock()
		return w
	}
	measureMu.Unlock()
	w := m(text, sizePt)
	measureStore(rev, sizePt, text, w)
	return w
}

// measureUIBytes — как MeasureUIText, но по байтам: попадание в кэш
// не аллоцирует строку.
func measureUIBytes(b []byte, sizePt float64) int {
	m, ok := uiTextMeasurer.Load().(textMeasurer)
	if !ok {
		return int(float64(utf8.RuneCount(b)) * sizePt * 0.65)
	}
	rev := TextMetricsRev()
	measureMu.Lock()
	if measureRev != rev {
		measureMemo, measureCount, measureRev = nil, 0, rev
	}
	bySize := measureMemo[sizePt]
	if w, hit := bySize[string(b)]; hit {
		measureMu.Unlock()
		return w
	}
	measureMu.Unlock()
	text := string(b)
	w := m(text, sizePt)
	measureStore(rev, sizePt, text, w)
	return w
}

func measureStore(rev uint64, sizePt float64, text string, w int) {
	measureMu.Lock()
	defer measureMu.Unlock()
	if measureRev != rev {
		return
	}
	if measureCount >= measureMemoMax {
		measureMemo, measureCount = nil, 0
	}
	if measureMemo == nil {
		measureMemo = make(map[float64]map[string]int, 4)
	}
	bySize := measureMemo[sizePt]
	if bySize == nil {
		bySize = make(map[string]int, 64)
		measureMemo[sizePt] = bySize
	}
	if _, dup := bySize[text]; !dup {
		measureCount++
	}
	bySize[text] = w
}

// wrapTextPx переносит текст по словам так, чтобы каждая строка была не шире
// maxW пикселей (шрифт default, sizePt). Явные \n сохраняются.
func wrapTextPx(text string, maxW int, sizePt float64) []string {
	var result []string
	var line []byte
	for _, paragraph := range splitLines(text) {
		words := fieldsSpace(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line = append(line[:0], words[0]...)
		for _, w := range words[1:] {
			n := len(line)
			line = append(line, ' ')
			line = append(line, w...)
			if measureUIBytes(line, sizePt) > maxW {
				result = append(result, string(line[:n]))
				line = append(line[:0], w...)
			}
		}
		result = append(result, string(line))
	}
	if len(result) == 0 {
		result = []string{""}
	}
	return result
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func fieldsSpace(s string) []string {
	var out []string
	start := -1
	for i, r := range s {
		if r == ' ' || r == '\t' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}
