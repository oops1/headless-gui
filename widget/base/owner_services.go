package base

// owner_services.go — службы владельца дерева (ADR-0008, задача 5.2).
//
// Замер текста, ключ цикла анимаций и растеризатор <text> в SVG у дерева с
// владельцем берутся у владельца: у каждого движка свои шрифты, DPI и
// масштаб, и окно на втором мониторе с другим масштабом мерило бы строки
// шрифтами первого. Владелец объявляет службы необязательными интерфейсами;
// у дерева без владельца (и у того, что считают до привязки — размер диалога
// при создании) остаются глобальные функции: MeasureUIText и прочие отвечают
// последним зарегистрированным движком, как прежде.

import (
	"sync"

	"github.com/oops1/headless-gui/v4/widget/svg"
)

// OwnerMeasurer — владелец, который меряет текст своими шрифтами.
type OwnerMeasurer interface {
	OwnerMeasurers() *MeasureSet
}

// OwnerAnimationLoop — владелец со своим циклом анимаций: анимация виджета
// его дерева (AnimateOwned с виджетом-владельцем) шагает этим циклом,
// откуда бы её ни завели (StepAnimationsFor(key)).
type OwnerAnimationLoop interface {
	OwnerLoopKey() any
}

// OwnerTextRasterizer — владелец со своим растеризатором <text> для SVG.
type OwnerTextRasterizer interface {
	OwnerTextRasterizer() svg.TextRasterizer
}

// OwnerPoster — владелец, который выполняет функцию в потоке интерфейса своего
// дерева (движок — Engine.Post).
type OwnerPoster interface {
	OwnerPost(fn func())
}

// PostFor ставит fn в поток интерфейса движка дерева w (Engine.Post): так
// виджет, которому состояние приходит с чужой горутины (таймер, источник
// данных), меняет себя там же, где идут ввод и раскладка. Возвращает false и
// fn не вызывает, если у дерева w нет такого владельца (виджет вне дерева
// движка) — тогда решает вызывающий.
func PostFor(w Widget, fn func()) bool {
	if fn == nil || w == nil {
		return false
	}
	p, ok := OwnerOf(w).(OwnerPoster)
	if !ok {
		return false
	}
	p.OwnerPost(fn)
	return true
}

// MeasureSet — набор измерителей владельца с собственной памятью замеров
// (ключ — семейство, кегль, текст; сброс по ревизии метрик текста и при
// переполнении). Создаётся владельцем один раз (NewMeasureSet).
type MeasureSet struct {
	m Measurers

	mu    sync.Mutex
	rev   uint64
	memo  map[string]map[float64]map[string]int
	count int
}

// NewMeasureSet создаёт набор над измерителями m; без m.Text — nil.
func NewMeasureSet(m Measurers) *MeasureSet {
	if m.Text == nil {
		return nil
	}
	return &MeasureSet{m: m}
}

// width — ширина строки семейством family ("" — шрифт по умолчанию).
func (s *MeasureSet) width(text string, sizePt float64, family string) int {
	rev := TextMetricsRev()
	s.mu.Lock()
	if s.rev != rev {
		s.memo, s.count, s.rev = nil, 0, rev
	}
	if w, ok := s.memo[family][sizePt][text]; ok {
		s.mu.Unlock()
		return w
	}
	s.mu.Unlock()
	var w int
	if family != "" && s.m.TextFont != nil {
		w = s.m.TextFont(text, sizePt, family)
	} else {
		w = s.m.Text(text, sizePt)
	}
	s.store(rev, family, sizePt, text, w)
	return w
}

// widthBytes — width по байтам: попадание в память не аллоцирует строку.
func (s *MeasureSet) widthBytes(b []byte, sizePt float64) int {
	rev := TextMetricsRev()
	s.mu.Lock()
	if s.rev != rev {
		s.memo, s.count, s.rev = nil, 0, rev
	}
	if w, ok := s.memo[""][sizePt][string(b)]; ok {
		s.mu.Unlock()
		return w
	}
	s.mu.Unlock()
	text := string(b)
	w := s.m.Text(text, sizePt)
	s.store(rev, "", sizePt, text, w)
	return w
}

func (s *MeasureSet) store(rev uint64, family string, sizePt float64, text string, w int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rev != rev {
		return
	}
	if s.count >= measureMemoMax {
		s.memo, s.count = nil, 0
	}
	if s.memo == nil {
		s.memo = make(map[string]map[float64]map[string]int, 2)
	}
	bySize := s.memo[family]
	if bySize == nil {
		bySize = make(map[float64]map[string]int, 4)
		s.memo[family] = bySize
	}
	byText := bySize[sizePt]
	if byText == nil {
		byText = make(map[string]int, 64)
		bySize[sizePt] = byText
	}
	if _, dup := byText[text]; !dup {
		s.count++
	}
	byText[text] = w
}

// fontMetrics — вертикальные метрики (как MeasureUIFontMetrics).
func (s *MeasureSet) fontMetrics(sizePt float64, family string) FontMetrics {
	if sizePt <= 0 {
		return FontMetrics{}
	}
	if s.m.FontMetrics == nil {
		return approxFontMetrics(sizePt)
	}
	a, d, g := s.m.FontMetrics(sizePt, family)
	a, d, g = max(a, 0), max(d, 0), max(g, 0)
	return FontMetrics{Ascent: a, Descent: d, LineGap: g, Height: a + d + g}
}

// MeasureSetOf — набор измерителей владельца дерева w; nil — глобальный путь.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func MeasureSetOf(w Widget) *MeasureSet {
	if w == nil {
		return nil
	}
	if om, ok := OwnerOf(w).(OwnerMeasurer); ok {
		return om.OwnerMeasurers()
	}
	return nil
}

// MeasureTextFor — ширина строки шрифтом по умолчанию так, как её нарисует
// движок дерева виджета w; дерево без владельца — MeasureUIText.
func MeasureTextFor(w Widget, text string, sizePt float64) int {
	if s := MeasureSetOf(w); s != nil {
		return s.width(text, sizePt, "")
	}
	return MeasureUIText(text, sizePt)
}

// MeasureTextFontFor — MeasureUITextFont для дерева виджета w.
func MeasureTextFontFor(w Widget, text string, sizePt float64, family string) int {
	if s := MeasureSetOf(w); s != nil {
		return s.width(text, sizePt, family)
	}
	return MeasureUITextFont(text, sizePt, family)
}

// MeasureRunePositionsFor — MeasureUIRunePositions для дерева виджета w.
func MeasureRunePositionsFor(w Widget, text string, sizePt float64, family string) []int {
	if s := MeasureSetOf(w); s != nil {
		if s.m.RunePositions == nil {
			return nil
		}
		return s.m.RunePositions(text, sizePt, family)
	}
	return MeasureUIRunePositions(text, sizePt, family)
}

// FontMetricsFor — MeasureUIFontMetrics для дерева виджета w.
func FontMetricsFor(w Widget, sizePt float64, family string) FontMetrics {
	if s := MeasureSetOf(w); s != nil {
		return s.fontMetrics(sizePt, family)
	}
	return MeasureUIFontMetrics(sizePt, family)
}

// MeasureBytesFor — measureUIBytes для дерева виджета w.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func MeasureBytesFor(w Widget, b []byte, sizePt float64) int {
	if s := MeasureSetOf(w); s != nil {
		return s.widthBytes(b, sizePt)
	}
	return measureUIBytes(b, sizePt)
}

// TextRasterizerFor — растеризатор <text> владельца дерева w; nil — действующий
// глобальный (svg.RegisterTextRasterizer). Для svg.ParseWith.
func TextRasterizerFor(w Widget) svg.TextRasterizer {
	if w == nil {
		return nil
	}
	if ot, ok := OwnerOf(w).(OwnerTextRasterizer); ok {
		return ot.OwnerTextRasterizer()
	}
	return nil
}

// ownerLoopKey — ключ цикла анимаций владельца дерева виджета owner; nil —
// не виджет или владелец без своего цикла.
func ownerLoopKey(owner any) any {
	bp, ok := owner.(baseProvider)
	if !ok {
		return nil
	}
	if ol, ok := bp.baseRef().ownerOf().(OwnerAnimationLoop); ok {
		return ol.OwnerLoopKey()
	}
	return nil
}
