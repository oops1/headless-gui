package widget

import "testing"

// useMeasurers подменяет набор зарегистрированных измерителей на время теста
// (без аргументов — «движка нет», как в headless без engine.New) и возвращает
// прежний.
//
// Список правится напрямую, а не через RegisterMeasurers: та публикует
// измеритель в общий atomic.Value, который обратно в «пусто» не вернуть, и
// остальные тесты пакета, рассчитанные на эвристику ширины, ломались бы
// (ширина пунктов меню и флажков) — зависимость от порядка тестов.
func useMeasurers(t *testing.T, sets ...Measurers) {
	t.Helper()
	measurersMu.Lock()
	savedList, savedBase := measurers, baseMeasurer
	measurers, baseMeasurer = nil, nil
	for i, s := range sets {
		measurers = append(measurers, registeredMeasurer{handle: uint64(i + 1), set: s})
	}
	measurersMu.Unlock()
	t.Cleanup(func() {
		measurersMu.Lock()
		measurers, baseMeasurer = savedList, savedBase
		measurersMu.Unlock()
	})
}

// Без движка метрики — правдоподобное приближение от кегля, а не нули:
// нулевая высота строки схлопнула бы раскладку.
func TestMeasureUIFontMetrics_ApproxWithoutEngine(t *testing.T) {
	useMeasurers(t)

	m := MeasureUIFontMetrics(DefaultFontSizePt, "")
	if m.Ascent <= 0 || m.Descent <= 0 {
		t.Fatalf("приближение дало нулевые метрики: %+v", m)
	}
	if m.Ascent <= m.Descent {
		t.Errorf("подъём %d не больше спуска %d", m.Ascent, m.Descent)
	}
	if m.Height != m.Ascent+m.Descent+m.LineGap {
		t.Errorf("Height %d != %d+%d+%d", m.Height, m.Ascent, m.Descent, m.LineGap)
	}
	// 10pt при 96 DPI — около 13 px кегля; высота строки у реальных шрифтов
	// интерфейса 1.1–1.3 от него.
	if m.Height < 13 || m.Height > 20 {
		t.Errorf("высота строки %d для 10pt вне правдоподобного диапазона 13..20", m.Height)
	}

	// Имя семейства без движка ни на что не влияет.
	if got := MeasureUIFontMetrics(DefaultFontSizePt, "что-угодно"); got != m {
		t.Errorf("семейство без движка изменило приближение: %+v != %+v", got, m)
	}

	// Пропорциональность кеглю.
	big := MeasureUIFontMetrics(40, "")
	if d := big.Height - 4*m.Height; d < -4 || d > 4 {
		t.Errorf("Height 10pt=%d, 40pt=%d — не пропорционально", m.Height, big.Height)
	}
}

// Мелкий кегль не должен давать нулевой подъём или спуск, а нулевой и
// отрицательный — шрифта вовсе.
func TestMeasureUIFontMetrics_ApproxEdges(t *testing.T) {
	useMeasurers(t)

	tiny := MeasureUIFontMetrics(0.1, "")
	if tiny.Ascent < 1 || tiny.Descent < 1 || tiny.Ascent <= tiny.Descent {
		t.Errorf("крошечный кегль: %+v", tiny)
	}
	for _, sz := range []float64{0, -5} {
		if got := MeasureUIFontMetrics(sz, ""); got != (FontMetrics{}) {
			t.Errorf("кегль %v: ожидались нули, получено %+v", sz, got)
		}
	}
}

// Измеритель без FontMetrics (старый, заданный SetTextMeasurer или набором без
// этого поля) не ломает вызов: ответ — приближение, как и для ширин.
func TestMeasureUIFontMetrics_MeasurerWithoutMetrics(t *testing.T) {
	useMeasurers(t, Measurers{Text: func(text string, sizePt float64) int { return 1 }})

	m := MeasureUIFontMetrics(DefaultFontSizePt, "")
	if m != approxFontMetrics(DefaultFontSizePt) {
		t.Errorf("без FontMetrics ожидалось приближение, получено %+v", m)
	}
}

// Зарегистрированный измеритель отвечает за метрики; Height всегда сумма
// частей, даже если измеритель вернул мусор, а отрицательные значения
// обрезаются.
func TestMeasureUIFontMetrics_UsesRegistered(t *testing.T) {
	var gotSize float64
	var gotFamily string
	useMeasurers(t, Measurers{
		Text: func(text string, sizePt float64) int { return 1 },
		FontMetrics: func(sizePt float64, family string) (int, int, int) {
			gotSize, gotFamily = sizePt, family
			return 15, 4, 2
		},
	})

	m := MeasureUIFontMetrics(12, "моно")
	if m != (FontMetrics{Ascent: 15, Descent: 4, LineGap: 2, Height: 21}) {
		t.Errorf("получено %+v", m)
	}
	if gotSize != 12 || gotFamily != "моно" {
		t.Errorf("измеритель получил (%v, %q)", gotSize, gotFamily)
	}

	useMeasurers(t, Measurers{
		Text: func(text string, sizePt float64) int { return 1 },
		FontMetrics: func(float64, string) (int, int, int) {
			return 10, -3, -1
		},
	})
	m = MeasureUIFontMetrics(12, "")
	if m != (FontMetrics{Ascent: 10, Descent: 0, LineGap: 0, Height: 10}) {
		t.Errorf("отрицательные значения не обрезаны: %+v", m)
	}
}
