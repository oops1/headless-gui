package engine

import (
	"fmt"
	"image"
	_ "image/jpeg" // декодер JPEG для SetBackgroundFile
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"

	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// Engine управляет холстом, деревом виджетов и циклом рендеринга.
//
// Жизненный цикл:
//
//	eng := engine.New(1920, 1024, 20)
//	eng.SetBackgroundFile("gui/background.png")
//	eng.SetRoot(rootWidget)
//	eng.Start()
//	for frame := range eng.Frames() { /* обрабатываем тайлы */ }
//	eng.Stop()
//
// Поток рендеринга и поток потребителя разделены: рендер-горутина
// складывает готовые кадры в буферизованный канал (глубина 8).
// Если потребитель отстаёт, излишние кадры пропускаются.
type Engine struct {
	canvas    *Canvas
	fontCache *FontCache
	bgSrc     image.Image // исходный фон (до масштабирования); нужен при SetResolution
	root      widget.Widget
	mu        sync.RWMutex // защищает УКАЗАТЕЛИ root/canvas/bgSrc (короткие секции)

	// frameMu сериализует кадр (draw+diff) со структурными операциями над
	// канвасом/шрифтами (SetResolution, RegisterFont*, SetTheme и т.п.).
	// В отличие от прежней схемы, e.mu больше НЕ удерживается на весь кадр —
	// SetRoot/Root и события не блокируются рендером.
	// Порядок захвата: frameMu → e.mu (никогда наоборот).
	frameMu sync.Mutex

	// ── Рендер по запросу (см. invalidate.go) ───────────────────────────────
	onDemand  atomic.Bool
	invGen    atomic.Uint64
	damageMu  sync.Mutex
	damage    image.Rectangle // объединение InvalidateRect с прошлого кадра
	damageAll bool            // Invalidate() — полный diff

	focus    focusManager  // текущий виджет с фокусом
	captured widget.Widget // виджет, захвативший мышь (drag)
	capMu    sync.Mutex

	// pressConsumer — виджет, поглотивший последний press ЛКМ.
	// При release: если этот виджет больше не под курсором
	// (удалён/закрыт), событие проглатывается, чтобы не пролетело
	// на виджет, оказавшийся под курсором после закрытия.
	pressConsumer widget.Widget

	modals []widget.ModalWidget // стек модальных виджетов (последний = верхний)
	modMu  sync.Mutex

	// modalHost — опциональный хост нативных модалок (window.dialogHost на
	// Win32). Если установлен и принимает модалку, она живёт в собственном
	// нативном окне ОС, а не в холсте движка. onModalClosed — колбэк,
	// вызываемый в конце CloseModal (см. modalhost.go). hostMu защищает оба.
	modalHost     ModalHost
	onModalClosed func(widget.ModalWidget)
	hostMu        sync.Mutex

	frameSeq atomic.Uint64
	frames   chan output.Frame
	quit     chan struct{}
	done     chan struct{}

	fps     int     // целевой FPS, 1–120
	userDPI float64 // пользовательский DPI шрифтов (без учёта HiDPI-масштаба)

	// scaleBits — текущий HiDPI-масштаб (math.Float64bits) для lock-free
	// чтения из InvalidateRect/toLogical: они вызываются из сеттеров
	// виджетов, в т.ч. когда движок уже держит e.mu (например, root.SetBounds
	// из SetResolution) — брать e.mu там нельзя (RWMutex нереентерабелен).
	scaleBits atomic.Uint64
	saveDir  string       // если не пусто — сохранять PNG в эту директорию
	saveCh   chan saveJob // канал для асинхронного сохранения
	saveDone chan struct{} // закрывается, когда saveWorker завершил запись всех PNG

	// ── Tooltip ─────────────────────────────────────────────────────────────
	ttMu       sync.Mutex
	ttEnabled  bool          // показывать всплывающие подсказки
	ttDelay    time.Duration // задержка появления после остановки курсора
	ttMouseX   int
	ttMouseY   int
	ttLastMove time.Time
	ttHasMouse bool            // курсор хотя бы раз входил на холст
	ttShownAt  image.Rectangle // область показанной подсказки (пустая — не показана)
}

type saveJob struct {
	path string
	seq  uint64
	snap *image.RGBA // снапшот front-буфера на момент рендера
}

// New создаёт движок.
//
//	width, height — размер виртуального экрана в пикселях
//	fps           — целевая частота кадров (рекомендуется 15–25)
func New(width, height, fps int) *Engine {
	if fps < 1 {
		fps = 20
	}
	fc := newFontCache("assets")
	e := &Engine{
		fontCache: fc,
		canvas:    newCanvas(width, height, fc),
		frames:    make(chan output.Frame, 8),
		quit:      make(chan struct{}),
		done:      make(chan struct{}),
		fps:       fps,
		userDPI:   DefaultDPI,
		ttEnabled: true,
		ttDelay:   600 * time.Millisecond,
	}
	e.scaleBits.Store(math.Float64bits(1))
	// Рендер по запросу — режим по умолчанию (v3.5): виджеты самоинвалидируются
	// (Base.Invalidate, авто-damage в SetBounds/сеттерах), события и слой данных
	// инвалидируют через движок. Кадры рендерятся только при изменениях UI,
	// причём частично — в пределах damage-области. Прямые записи в
	// экспортированные поля виджетов требуют Invalidate()/Engine.Invalidate().
	// Опт-аут (прежнее поведение «рендер каждый тик»): SetRenderOnDemand(false).
	e.onDemand.Store(true)
	// Best-effort: подгружаем системные шрифты с широким покрытием символов
	// (✓ ✗ ⚠, box-drawing, стрелки) как fallback к встроенному Go Regular (BUG-2).
	for _, p := range systemFallbackFontPaths() {
		if data, err := os.ReadFile(p); err == nil {
			e.canvas.AddFallbackFont(data)
		}
	}
	// Встроенные жирный/курсивный шрифты для FontWeight/FontStyle (P1).
	e.canvas.RegisterFont(widget.BuiltinFontBold, gobold.TTF)
	e.canvas.RegisterFont(widget.BuiltinFontItalic, goitalic.TTF)
	e.canvas.RegisterFont(widget.BuiltinFontBoldItalic, gobolditalic.TTF)
	// Сообщаем виджетам размер канваса (для удержания popup-меню в пределах экрана).
	widget.SetScreenBounds(width, height)
	// Авто-регистрация пользовательских шрифтов из assets/fonts (Roboto, Inter, …).
	e.loadFontDirectory(filepath.Join("assets", "fonts"))
	// Шрифт по умолчанию — Roboto, если он есть в assets/fonts; иначе остаётся
	// встроенный Go Regular.
	e.canvas.SetDefaultFont("Roboto")
	// Слой данных (биндинги/{Loc}/live-коллекции) сообщает об изменениях UI —
	// нужно для рендера по запросу (см. invalidate.go). Последний созданный
	// движок выигрывает (на процесс — один активный движок).
	widget.SetUIChangeNotifier(e.Invalidate)
	// Точечная инвалидация от виджетов (авто-damage): hover/press/фокус/сеттеры
	// сообщают свой прямоугольник — кадр перерисовывается и диффается частично.
	widget.SetUIRectChangeNotifier(e.InvalidateRect)
	// Точный замер текста для компоновки до отрисовки (размеры диалогов).
	widget.SetTextMeasurer(e.canvas.MeasureText)
	return e
}

// SetRoot устанавливает корневой виджет.
//
// Поведение bounds:
//
//   - Если у переданного виджета bounds пустые (нулевой Rect) — что
//     характерно для виджетов, созданных через NewXxx() без явного
//     SetBounds — корню назначается прямоугольник всего холста.
//   - Если bounds непустые (например XAML-загрузчик уже выставил
//     Width/Height из <Window Width=… Height=…/>) — они сохраняются.
//     Это позволяет XAML-разработчику задать «логический размер окна»
//     меньше канваса (типичный сценарий: окно по центру холста)
//     или больше канваса (виртуальная сцена под скроллом).
//
// Безопасно вызывать до Start() или во время работы движка.
// Рекурсивно инжектит CaptureManager виджетам, поддерживающим CaptureAware.
//
// Если нужно безусловно растянуть корень на весь канвас независимо
// от XAML — используйте SetRootFullCanvas.
func (e *Engine) SetRoot(w widget.Widget) {
	// Готовим виджет ДО публикации указателя: рендер-горутина видит либо
	// старый root, либо полностью подготовленный новый (лейаут + capture).
	e.mu.RLock()
	cw, ch := e.canvas.LogicalSize()
	e.mu.RUnlock()
	if w.Bounds().Empty() {
		w.SetBounds(image.Rect(0, 0, cw, ch))
	}
	injectCaptureManager(w, e)
	e.mu.Lock()
	e.root = w
	e.mu.Unlock()
	e.Invalidate()
}

// SetRootFullCanvas устанавливает корневой виджет и принудительно
// растягивает его на весь холст (старое поведение SetRoot до фикса A9).
// Используйте, когда XAML задал размеры, но вам нужен fullscreen.
func (e *Engine) SetRootFullCanvas(w widget.Widget) {
	e.mu.RLock()
	cw, ch := e.canvas.LogicalSize()
	e.mu.RUnlock()
	w.SetBounds(image.Rect(0, 0, cw, ch))
	injectCaptureManager(w, e)
	e.mu.Lock()
	e.root = w
	e.mu.Unlock()
	e.Invalidate()
}

// injectCaptureManager рекурсивно раздаёт CaptureManager по дереву виджетов.
func injectCaptureManager(w widget.Widget, cm widget.CaptureManager) {
	if ca, ok := w.(widget.CaptureAware); ok {
		ca.SetCaptureManager(cm)
	}
	for _, child := range w.Children() {
		injectCaptureManager(child, cm)
	}
}

// Root возвращает текущий корневой виджет (или nil).
func (e *Engine) Root() widget.Widget {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.root
}

// Frames возвращает канал только для чтения.
// Каждый кадр в канале содержит только изменившиеся тайлы.
// Тайлы — в ФИЗИЧЕСКИХ пикселях (логические × Scale).
// Канал закрывается после Stop().
func (e *Engine) Frames() <-chan output.Frame {
	return e.frames
}

// CanvasSize возвращает ЛОГИЧЕСКИЙ размер холста (в нём живут виджеты).
// При Scale == 1 совпадает с физическим.
func (e *Engine) CanvasSize() (w, h int) {
	return e.canvas.LogicalSize()
}

// PhysicalSize возвращает ФИЗИЧЕСКИЙ размер буферов кадра
// (размер тайлов в Frames и нативного окна на HiDPI-мониторе).
func (e *Engine) PhysicalSize() (w, h int) {
	return e.canvas.W, e.canvas.H
}

// Scale возвращает текущий HiDPI-масштаб (1.0 по умолчанию). Lock-free.
func (e *Engine) Scale() float64 {
	return math.Float64frombits(e.scaleBits.Load())
}

// SetScale задаёт HiDPI-масштаб: логический размер холста сохраняется,
// физические буферы пересоздаются (логический × k), шрифты перерендериваются
// в физическом DPI. События мыши принимаются в физических координатах и
// переводятся в логические автоматически.
func (e *Engine) SetScale(k float64) {
	if k <= 0 {
		k = 1
	}
	e.frameMu.Lock() // буферы меняются — ждём конца кадра
	defer e.frameMu.Unlock()
	e.mu.Lock()
	if e.canvas.scale == k {
		e.mu.Unlock()
		return
	}
	lw, lh := e.canvas.LogicalSize()
	e.canvas = e.canvas.cloneForSize(lw, lh, k, e.bgSrc)
	e.canvas.setDPIAll(e.userDPI * k)
	e.scaleBits.Store(math.Float64bits(k))
	e.mu.Unlock()
	e.Invalidate()
}

// SetResolution изменяет ЛОГИЧЕСКОЕ разрешение холста (масштаб сохраняется).
// Вызывать до Start() или когда движок остановлен.
// Если был установлен фон, он автоматически перемасштабируется под новый размер.
// Корневой виджет получает обновлённые bounds. Зарегистрированные шрифты
// и fallback-цепочка сохраняются.
func (e *Engine) SetResolution(width, height int) {
	e.frameMu.Lock() // ждём конца текущего кадра — буферы меняются
	defer e.frameMu.Unlock()
	e.mu.Lock()
	e.canvas = e.canvas.cloneForSize(width, height, e.canvas.scale, e.bgSrc)
	root := e.root
	e.mu.Unlock()
	// ВАЖНО: SetBounds — вне e.mu. Изменение bounds триггерит авто-damage
	// (notifyRectChanged → InvalidateRect); повторный захват e.mu на том же
	// потоке дал бы дедлок (см. scaleBits).
	if root != nil {
		root.SetBounds(image.Rect(0, 0, width, height))
	}
	widget.SetScreenBounds(width, height)

	// Открытые модалки центрированы под прежний холст: при уменьшении окна
	// они уезжали за край и обрезались (диалог «в пределах родительского
	// окна» — иного в софтверном рендере нет, но он обязан быть виден целиком).
	e.modMu.Lock()
	modals := make([]widget.ModalWidget, len(e.modals))
	copy(modals, e.modals)
	e.modMu.Unlock()
	for _, m := range modals {
		b := m.Bounds()
		if x, y := clampToCanvas(b, width, height); x != b.Min.X || y != b.Min.Y {
			moveModalTo(m, x, y)
		}
	}
	e.Invalidate()
}

// SetBackgroundFile загружает изображение (PNG или JPEG) из файла и масштабирует его
// до размера холста. Исходный файл сохраняется — при последующих вызовах SetResolution
// фон автоматически перемасштабируется без повторной загрузки.
func (e *Engine) SetBackgroundFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	e.frameMu.Lock() // фон читается blitBackground — меняем между кадрами
	e.mu.Lock()
	e.bgSrc = img
	e.canvas.setBackground(img)
	e.mu.Unlock()
	e.frameMu.Unlock()
	e.Invalidate()
	return nil
}

// SaveFrames включает сохранение каждого изменившегося кадра как PNG в директорию dir.
// Вызывать до Start(). Все кадры гарантированно сохраняются (отправка блокирующая).
// Stop() дожидается записи всех оставшихся PNG перед возвратом.
func (e *Engine) SaveFrames(dir string) {
	e.saveDir = dir
	e.saveCh = make(chan saveJob, 512)
	e.saveDone = make(chan struct{})
}

// RegisterFont регистрирует именованный шрифт (TTF-данные) в движке.
// fontName соответствует FontFamily в XAML (например "Segoe UI", "Roboto").
// Шрифт будет использоваться виджетами через DrawTextFont.
func (e *Engine) RegisterFont(fontName string, ttfData []byte) {
	e.frameMu.Lock() // кэш шрифтов читается отрисовкой — меняем между кадрами
	defer e.frameMu.Unlock()
	e.canvas.RegisterFont(fontName, ttfData)
}

// RegisterFontFile регистрирует именованный шрифт из TTF/OTF-файла.
func (e *Engine) RegisterFontFile(fontName, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("RegisterFontFile %q: %w", path, err)
	}
	e.RegisterFont(fontName, data)
	return nil
}

// loadFontDirectory регистрирует все .ttf/.otf из каталога dir как именованные
// шрифты. Имя = имя файла без расширения (напр. "Roboto-Regular"). Для файлов
// вида "Семейство-Regular" дополнительно регистрируется псевдоним-семейство
// ("Roboto"), чтобы XAML FontFamily="Roboto" работал из коробки.
func (e *Engine) loadFontDirectory(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".ttf" && ext != ".otf" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		e.canvas.RegisterFont(stem, data)
		if fam := fontFamilyAlias(stem); fam != "" && fam != stem && !e.canvas.hasFont(fam) {
			e.canvas.RegisterFont(fam, data) // семейство → Regular-вес
		}
	}
}

// fontFamilyAlias возвращает имя семейства для файла-стема (только для Regular-веса):
// "Roboto-Regular" → "Roboto"; "Inter" → "Inter"; "Roboto-Bold" → "".
func fontFamilyAlias(stem string) string {
	if i := strings.IndexByte(stem, '-'); i > 0 {
		if strings.EqualFold(stem[i+1:], "regular") {
			return stem[:i]
		}
		return ""
	}
	return stem
}

// RegisterFontDir регистрирует все шрифты из каталога (TTF/OTF) как именованные.
// Вызывать до Start().
func (e *Engine) RegisterFontDir(dir string) {
	e.frameMu.Lock()
	defer e.frameMu.Unlock()
	e.loadFontDirectory(dir)
}

// SetDefaultFont делает зарегистрированный шрифт шрифтом по умолчанию.
// Возвращает false, если шрифт с таким именем не зарегистрирован.
func (e *Engine) SetDefaultFont(name string) bool {
	e.frameMu.Lock()
	defer e.frameMu.Unlock()
	ok := e.canvas.SetDefaultFont(name)
	if ok {
		e.Invalidate()
	}
	return ok
}

// AvailableFonts возвращает список зарегистрированных именованных шрифтов.
func (e *Engine) AvailableFonts() []string {
	e.frameMu.Lock()
	defer e.frameMu.Unlock()
	return e.canvas.fontNames()
}

// RegisterFallbackFont добавляет fallback-шрифт (TTF/OTF-данные) для рун,
// отсутствующих в основном шрифте (BUG-2). Fallback-шрифты применяются
// в порядке регистрации после встроенного и системных. Вызывать до Start().
func (e *Engine) RegisterFallbackFont(ttfData []byte) bool {
	e.frameMu.Lock()
	defer e.frameMu.Unlock()
	return e.canvas.AddFallbackFont(ttfData)
}

// RegisterFallbackFontFile добавляет fallback-шрифт из TTF/OTF-файла.
func (e *Engine) RegisterFallbackFontFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("RegisterFallbackFontFile %q: %w", path, err)
	}
	if !e.RegisterFallbackFont(data) {
		return fmt.Errorf("RegisterFallbackFontFile %q: невалидный шрифт", path)
	}
	return nil
}

// SetDPI изменяет пользовательский DPI для рендеринга шрифтов (по умолчанию
// 96). Итоговый DPI растеризации = DPI × HiDPI-масштаб. Применяется ко всем
// шрифтам (default, именованные, fallback), кэши сбрасываются.
func (e *Engine) SetDPI(dpi float64) {
	e.frameMu.Lock()
	defer e.frameMu.Unlock()
	e.userDPI = dpi
	e.canvas.setDPIAll(dpi * e.canvas.scale)
	e.Invalidate()
}

// SetTheme применяет тему к глобальным цветам и ко всему дереву виджетов.
// Текущие виджеты немедленно получают новые цвета; новые виджеты будут создаваться
// с обновлёнными цветами по умолчанию.
func (e *Engine) SetTheme(t *widget.Theme) {
	widget.ApplyGlobalTheme(t)
	e.frameMu.Lock() // массовая мутация цветов дерева — не во время отрисовки
	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
	if root != nil {
		widget.ApplyThemeTree(root, t)
	}
	// Модальные виджеты живут вне дерева root — темизируем отдельно.
	e.modMu.Lock()
	modals := make([]widget.ModalWidget, len(e.modals))
	copy(modals, e.modals)
	e.modMu.Unlock()
	for _, m := range modals {
		widget.ApplyThemeTree(m, t)
	}
	e.frameMu.Unlock()
	e.Invalidate()
}

// Start запускает цикл рендеринга в отдельной горутине.
// Вызывать не более одного раза.
func (e *Engine) Start() {
	e.Invalidate() // гарантируем рендер первого кадра в любом режиме
	if e.saveDir != "" {
		go e.saveWorker()
	}
	go e.loop()
}

// Stop останавливает цикл рендеринга и ждёт его завершения.
// После Stop канал Frames() закрывается.
func (e *Engine) Stop() {
	close(e.quit)
	<-e.done
	close(e.frames)
	if e.saveCh != nil {
		close(e.saveCh)   // saveWorker дочитает оставшиеся задачи
		<-e.saveDone      // ждём пока все PNG записаны на диск
	}
}

// ─── Accessibility ──────────────────────────────────────────────────────────

// AccessibilityTree возвращает семантический снапшот UI: дерево ролей, имён,
// значений и состояний (см. widget.BuildAccessTree). Модальные диалоги
// добавляются детьми корня с состоянием modal. Снапшот сериализуем в JSON —
// в стриминговых сценариях передаётся side-channel'ом рядом с тайлами кадра
// и озвучивается скринридером на стороне клиента.
func (e *Engine) AccessibilityTree() *widget.AccessNode {
	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
	focused := e.focus.get()

	tree := widget.BuildAccessTree(root, focused)
	if tree == nil {
		return nil
	}

	e.modMu.Lock()
	modals := make([]widget.ModalWidget, len(e.modals))
	copy(modals, e.modals)
	e.modMu.Unlock()
	for _, m := range modals {
		if mn := widget.BuildAccessTree(m, focused); mn != nil {
			mn.States = append(mn.States, widget.StateModal)
			tree.Children = append(tree.Children, mn)
		}
	}
	return tree
}

// ─── Modal ──────────────────────────────────────────────────────────────────

// ShowModal показывает модальный виджет поверх всего UI.
// Диалог центрируется на экране. Весь ввод ограничивается модальным виджетом.
// CaptureManager инжектится автоматически.
func (e *Engine) ShowModal(m widget.ModalWidget) {
	// Хост нативных модалок (Win32): если он принял модалку, она целиком
	// живёт в собственном окне ОС — НЕ добавляем в стек, не инжектим capture,
	// не центрируем. false → обычный in-canvas путь ниже.
	if h := e.getModalHost(); h != nil {
		if h.ShowModal(m) {
			return
		}
	}

	// Центрируем диалог (в логических координатах)
	e.mu.RLock()
	cw, ch := e.canvas.LogicalSize()
	e.mu.RUnlock()
	b := m.Bounds()
	cx := (cw - b.Dx()) / 2
	cy := (ch - b.Dy()) / 2
	// Диалог больше холста: прижимаем к левому/верхнему краю, а не центрируем
	// в минус — титлбар и ✕ должны оставаться видимыми и достижимыми.
	if cx < 0 {
		cx = 0
	}
	if cy < 0 {
		cy = 0
	}

	moveModalTo(m, cx, cy)

	injectCaptureManager(m, e)

	// Кнопка ✕ диалога закрывает модалку с семантикой отмены (как Escape).
	if s, ok := m.(interface{ SetCloser(func()) }); ok {
		s.SetCloser(func() {
			if c, ok := m.(interface{ OnCancel() }); ok {
				c.OnCancel()
			}
			e.CloseModal(m)
		})
	}

	// Сообщаем виджету, что он показан как модальный (Dialog запускает здесь
	// fade-in затемнения); дублирует уже true modal-флаг конструктора, но
	// это единственный момент, когда движок точно знает про показ.
	if sm, ok := m.(interface{ SetModal(bool) }); ok {
		sm.SetModal(true)
	}

	e.modMu.Lock()
	e.modals = append(e.modals, m)
	e.modMu.Unlock()
	e.Invalidate()
}

// moveModalTo перемещает модальный виджет в позицию (x, y), сохраняя размер.
// Если SetBounds виджета сам пересчитывает позиции дочерних (Canvas, Grid,
// DockPanel через собственный layout) — ручной сдвиг не нужен; если нет
// (Panel, Dialog) — сдвигаем детей вручную. Определяем по первому ребёнку.
func moveModalTo(m widget.ModalWidget, x, y int) {
	b := m.Bounds()
	children := m.Children()
	var firstChildBefore image.Rectangle
	if len(children) > 0 {
		firstChildBefore = children[0].Bounds()
	}

	m.SetBounds(image.Rect(x, y, x+b.Dx(), y+b.Dy()))

	if len(children) > 0 && children[0].Bounds() == firstChildBefore {
		contentOff := image.Pt(x-b.Min.X, y-b.Min.Y)
		for _, child := range children {
			widget.ShiftWidget(child, contentOff.X, contentOff.Y)
		}
	}
}

// clampToCanvas возвращает позицию, при которой прямоугольник b максимально
// вписан в холст cw×ch: сначала прижимаем к правому/нижнему краю, затем
// гарантируем неотрицательный верхний левый угол (если b больше холста —
// приоритет левому/верхнему краю: там титлбар и ✕ диалога).
func clampToCanvas(b image.Rectangle, cw, ch int) (int, int) {
	x, y := b.Min.X, b.Min.Y
	if x+b.Dx() > cw {
		x = cw - b.Dx()
	}
	if x < 0 {
		x = 0
	}
	if y+b.Dy() > ch {
		y = ch - b.Dy()
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

// CloseModal закрывает указанный модальный виджет (удаляет из стека).
// Если m == nil — закрывает верхний модальный виджет.
func (e *Engine) CloseModal(m widget.ModalWidget) {
	// Хост нативных модалок: если модалка у него — пусть закрывает сам.
	if h := e.getModalHost(); h != nil {
		if h.CloseModal(m) {
			return
		}
	}

	// Определяем закрываемую модалку и удаляем её из стека под modMu; сам
	// SetModal(false) и onModalClosed вызываем ВНЕ modMu (колбэки могут
	// повторно входить в движок).
	var closed widget.ModalWidget
	e.modMu.Lock()
	if m == nil {
		if len(e.modals) > 0 {
			closed = e.modals[len(e.modals)-1]
			e.modals = e.modals[:len(e.modals)-1]
		}
	} else {
		for i, modal := range e.modals {
			if modal == m {
				closed = modal
				e.modals = append(e.modals[:i], e.modals[i+1:]...)
				break
			}
		}
	}
	e.modMu.Unlock()

	if closed != nil {
		// Уведомляем диалог о закрытии (снимает подписки локали, останавливает
		// fade и т.п.).
		if sm, ok := closed.(interface{ SetModal(bool) }); ok {
			sm.SetModal(false)
		}
		e.fireOnModalClosed(closed)
	}
	e.Invalidate()
}

// topModal возвращает верхний модальный виджет или nil.
func (e *Engine) topModal() widget.ModalWidget {
	e.modMu.Lock()
	defer e.modMu.Unlock()
	if len(e.modals) == 0 {
		return nil
	}
	return e.modals[len(e.modals)-1]
}

// ─── внутренние методы ───────────────────────────────────────────────────────

func (e *Engine) saveWorker() {
	defer close(e.saveDone)
	if err := mkdirAll(e.saveDir); err != nil {
		return
	}
	for job := range e.saveCh {
		savePNG(job.snap, job.path)
	}
}

// savePNG кодирует RGBA-изображение в PNG-файл.
func savePNG(img *image.RGBA, path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

func (e *Engine) loop() {
	defer close(e.done)

	interval := time.Duration(float64(time.Second) / float64(e.fps))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// lastGen — поколение инвалидации, отрендеренное последним кадром
	// (on-demand). Сентинел гарантирует рендер первого кадра.
	lastGen := ^uint64(0)

	for {
		select {
		case <-ticker.C:
			// Продвигаем анимации ДО решения о пропуске кадра: тики зовут
			// сеттеры виджетов, те самоинвалидируются (авто-damage), поэтому
			// damage от тиков попадёт в invGen ниже и кадр перерисуется
			// частично. Вызывается в любом режиме — анимации живут и при
			// SetRenderOnDemand(false).
			widget.StepAnimations(time.Now())
			if e.onDemand.Load() {
				gen := e.invGen.Load()
				if gen == lastGen && !e.animationNeeded(interval) {
					continue // UI не менялся — пропускаем кадр целиком
				}
				lastGen = gen // снимаем ДО рендера: инвалидация во время кадра не потеряется
			}
			frame := e.renderFrame()
			if len(frame.Tiles) == 0 {
				continue
			}
			select {
			case e.frames <- frame:
			default:
				// Потребитель не успевает — кадр отбрасывается
			}
		case <-e.quit:
			return
		}
	}
}

func (e *Engine) renderFrame() output.Frame {
	// frameMu гарантирует целостность канваса на время кадра; e.mu берётся
	// лишь на мгновение — снять указатели. SetRoot/Root/события рендером
	// больше не блокируются.
	e.frameMu.Lock()
	defer e.frameMu.Unlock()

	e.mu.RLock()
	canvas := e.canvas
	root := e.root
	e.mu.RUnlock()

	damage, damageAll := e.consumeDamage()

	// Частичная перерисовка: в on-demand режиме при InvalidateRect фон и
	// дерево рисуются только в damage-области (базовый клип канваса). Вне
	// damage back-буфер хранит прошлый кадр — он совпадает с front, поэтому
	// и отрисовка, и diff вне damage не нужны (контракт InvalidateRect:
	// вызывающий заявляет ВСЕ изменившиеся области).
	partial := e.onDemand.Load() && !damageAll && !damage.Empty()
	if partial {
		damage = damage.Intersect(image.Rect(0, 0, canvas.W, canvas.H))
		if damage.Empty() {
			return output.Frame{Seq: e.frameSeq.Add(1), Timestamp: time.Now()}
		}
		canvas.blitBackgroundIn(damage)
		canvas.setBaseClip(damage)
		defer canvas.clearBaseClip()
	} else {
		canvas.blitBackground()
	}

	// Корневое дерево: рисуем root и его overlay-слой (popup/dropdown).
	// Без этого вызова на канвасе остаётся только blitBackground —
	// именно сюда «уехал» баг с чёрным экраном при последнем appendF.
	if root != nil {
		root.Draw(canvas)
		drawOverlays(root, canvas)
	}

	// Модальные виджеты: затемнение + диалог поверх всего
	e.modMu.Lock()
	modals := make([]widget.ModalWidget, len(e.modals))
	copy(modals, e.modals)
	e.modMu.Unlock()

	for _, m := range modals {
		if !m.IsModal() {
			continue
		}
		// Затемнение фона (логические координаты)
		dim := m.DimColor()
		if dim.A > 0 {
			lw, lh := canvas.LogicalSize()
			canvas.FillRectAlpha(0, 0, lw, lh, dim)
		}
		// Отрисовка модального виджета
		m.Draw(canvas)
		drawOverlays(m, canvas)
	}

	// Всплывающая подсказка (поверх всего, включая модальные диалоги).
	e.drawTooltip(canvas, root)

	// Diff: при частичной перерисовке сравниваем только тайлы,
	// пересекающие damage-область (контракт InvalidateRect).
	var tiles []output.DirtyTile
	if partial {
		tiles = canvas.diffAndSyncIn(damage)
	} else {
		tiles = canvas.diffAndSync()
	}

	seq := e.frameSeq.Add(1)

	if e.saveDir != "" && len(tiles) > 0 {
		// Снимаем копию front-буфера СЕЙЧАС, пока он актуален.
		snap := image.NewRGBA(canvas.front.Rect)
		copy(snap.Pix, canvas.front.Pix)
		path := filepath.Join(e.saveDir, fmt.Sprintf("frame_%06d.png", seq))
		e.saveCh <- saveJob{path: path, seq: seq, snap: snap}
	}

	return output.Frame{
		Seq:       seq,
		Timestamp: time.Now(),
		Tiles:     tiles,
	}
}

// drawOverlays рекурсивно обходит дерево виджетов и вызывает DrawOverlay
// у тех, кто реализует OverlayDrawer и имеет активный overlay (например, открытый dropdown).
// Вызывается ПОСЛЕ отрисовки всего дерева — overlay рисуется поверх всех виджетов.
func drawOverlays(w widget.Widget, ctx widget.DrawContext) {
	if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
		od.DrawOverlay(ctx)
	}
	for _, child := range w.Children() {
		drawOverlays(child, ctx)
	}
}

func mkdirAll(dir string) error {
	return os.MkdirAll(dir, 0o755)
}
