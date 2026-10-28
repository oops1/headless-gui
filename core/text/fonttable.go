package text

import (
	"sort"
	"sync"
)

// FontTable — именованные шрифты холста под замком.
//
// Таблица общая у холста и его клонов (смена размера, всплывающий оверлей,
// растр повёрнутого текста): шрифт, зарегистрированный после клонирования,
// виден всем, как и прежде, когда клоны делили голую map.
//
// Замок нужен потому, что читают её не только из горутины кадра. Измеритель
// текста процесса (widget.MeasureUITextFont и родня) зовут из горутины
// приложения, а RegisterFont потребитель тоже вызывает из своей — при печати
// это фоновая горутина задания. Без замка запись в map шла одновременно с
// чтением (замечание WinLine после v3.33.0: гонка в TestExportPDF Блокнота).
type FontTable struct {
	mu sync.RWMutex
	m  map[string]*FontCache
}

func NewFontTable() *FontTable {
	return &FontTable{m: make(map[string]*FontCache)}
}

// Get возвращает шрифт по имени.
func (t *FontTable) Get(name string) (*FontCache, bool) {
	t.mu.RLock()
	fc, ok := t.m[name]
	t.mu.RUnlock()
	return fc, ok
}

// Set кладёт шрифт под имя, заменяя прежний.
func (t *FontTable) Set(name string, fc *FontCache) {
	t.mu.Lock()
	t.m[name] = fc
	t.mu.Unlock()
}

// Names — отсортированный список имён.
func (t *FontTable) Names() []string {
	t.mu.RLock()
	out := make([]string, 0, len(t.m))
	for k := range t.m {
		out = append(out, k)
	}
	t.mu.RUnlock()
	sort.Strings(out)
	return out
}

// All — снимок шрифтов для обхода без замка: обработчик (смена DPI,
// подпикселя) вправе работать долго и не должен держать регистрацию.
func (t *FontTable) All() []*FontCache {
	t.mu.RLock()
	out := make([]*FontCache, 0, len(t.m))
	for _, fc := range t.m {
		out = append(out, fc)
	}
	t.mu.RUnlock()
	return out
}
