package engine

import (
	"fmt"
	"image"
	_ "image/jpeg" // декодер JPEG для SetBackgroundFile
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/output"
	"github.com/oops1/headless-gui/widget"
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
	mu        sync.RWMutex // защищает root, canvas, bgSrc при изменении

	focus    focusManager  // текущий виджет с фокусом
	captured widget.Widget // виджет, захвативший мышь (drag)
	capMu    sync.Mutex

	modals []widget.ModalWidget // стек модальных виджетов (последний = верхний)
	modMu  sync.Mutex

	frameSeq atomic.Uint64
	frames   chan output.Frame
	quit     chan struct{}
	done     chan struct{}

	fps      int          // целевой FPS, 1–120
	saveDir  string       // если не пусто — сохранять PNG в эту директорию
	saveCh   chan saveJob // канал для асинхронного сохранения
	saveDone chan struct{} // закрывается, когда saveWorker завершил запись всех PNG
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
	return &Engine{
		fontCache: fc,
		canvas:    newCanvas(width, height, fc),
		frames:    make(chan output.Frame, 8),
		quit:      make(chan struct{}),
		done:      make(chan struct{}),
		fps:       fps,
	}
}

// SetRoot устанавливает корневой виджет и задаёт ему bounds равным всему холсту.
// Безопасно вызывать до Start() или во время работы движка.
// Рекурсивно инжектит CaptureManager виджетам, поддерживающим CaptureAware.
func (e *Engine) SetRoot(w widget.Widget) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.root = w
	w.SetBounds(image.Rect(0, 0, e.canvas.W, e.canvas.H))
	injectCaptureManager(w, e)
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

// Frames возвращает канал только для чтения.
// Каждый кадр в канале содержит только изменившиеся тайлы.
// Канал закрывается после Stop().
func (e *Engine) Frames() <-chan output.Frame {
	return e.frames
}

// CanvasSize возвращает размер холста в пикселях.
func (e *Engine) CanvasSize() (w, h int) {
	return e.canvas.W, e.canvas.H
}

// SetResolution изменяет разрешение холста.
// Вызывать до Start() или когда движок остановлен.
// Если был установлен фон, он автоматически перемасштабируется под новый размер.
// Корневой виджет получает обновлённые bounds.
func (e *Engine) SetResolution(width, height int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.canvas = newCanvas(width, height, e.fontCache)
	if e.bgSrc != nil {
		e.canvas.setBackground(e.bgSrc)
	}
	if e.root != nil {
		e.root.SetBounds(image.Rect(0, 0, width, height))
	}
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
	e.mu.Lock()
	e.bgSrc = img
	e.canvas.setBackground(img)
	e.mu.Unlock()
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
	e.mu.Lock()
	defer e.mu.Unlock()
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

// SetDPI изменяет DPI для рендеринга шрифтов (по умолчанию 96).
// Вызывать до Start(). Сбрасывает кэш шрифтов.
func (e *Engine) SetDPI(dpi float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fontCache.SetDPI(dpi)
}

// SetTheme применяет тему к глобальным цветам и ко всему дереву виджетов.
// Текущие виджеты немедленно получают новые цвета; новые виджеты будут создаваться
// с обновлёнными цветами по умолчанию.
func (e *Engine) SetTheme(t *widget.Theme) {
	widget.ApplyGlobalTheme(t)
	e.mu.Lock()
	root := e.root
	if root != nil {
		applyThemeToTree(root, t)
	}
	e.mu.Unlock()
}

// applyThemeToTree рекурсивно применяет тему к дереву виджетов.
func applyThemeToTree(w widget.Widget, t *widget.Theme) {
	if th, ok := w.(widget.Themeable); ok {
		th.ApplyTheme(t)
	}
	for _, child := range w.Children() {
		applyThemeToTree(child, t)
	}
}

// Start запускает цикл рендеринга в отдельной горутине.
// Вызывать не более одного раза.
func (e *Engine) Start() {
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

// ─── Modal ──────────────────────────────────────────────────────────────────

// ShowModal показывает модальный виджет поверх всего UI.
// Диалог центрируется на экране. Весь ввод ограничивается модальным виджетом.
// CaptureManager инжектится автоматически.
func (e *Engine) ShowModal(m widget.ModalWidget) {
	// Центрируем диалог
	b := m.Bounds()
	cx := (e.canvas.W - b.Dx()) / 2
	cy := (e.canvas.H - b.Dy()) / 2
	m.SetBounds(image.Rect(cx, cy, cx+b.Dx(), cy+b.Dy()))

	// Пересчитываем bounds дочерних виджетов относительно новой позиции
	contentOff := image.Pt(cx-b.Min.X, cy-b.Min.Y)
	for _, child := range m.Children() {
		cb := child.Bounds()
		child.SetBounds(cb.Add(contentOff))
	}

	injectCaptureManager(m, e)

	e.modMu.Lock()
	e.modals = append(e.modals, m)
	e.modMu.Unlock()
}

// CloseModal закрывает указанный модальный виджет (удаляет из стека).
// Если m == nil — закрывает верхний модальный виджет.
func (e *Engine) CloseModal(m widget.ModalWidget) {
	e.modMu.Lock()
	defer e.modMu.Unlock()

	if m == nil && len(e.modals) > 0 {
		e.modals = e.modals[:len(e.modals)-1]
		return
	}
	for i, modal := range e.modals {
		if modal == m {
			e.modals = append(e.modals[:i], e.modals[i+1:]...)
			return
		}
	}
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

	for {
		select {
		case <-ticker.C:
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
	e.mu.RLock()
	defer e.mu.RUnlock()

	e.canvas.blitBackground()

	if e.root != nil {
		e.root.Draw(e.canvas)
		// Overlay-слой: рисуем popup-элементы (dropdown-списки и пр.) поверх всего дерева.
		drawOverlays(e.root, e.canvas)
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
		// Затемнение фона
		dim := m.DimColor()
		if dim.A > 0 {
			e.canvas.FillRectAlpha(0, 0, e.canvas.W, e.canvas.H, dim)
		}
		// Отрисовка модального виджета
		m.Draw(e.canvas)
		drawOverlays(m, e.canvas)
	}

	tiles := e.canvas.diffAndSync()

	seq := e.frameSeq.Add(1)

	if e.saveDir != "" && len(tiles) > 0 {
		// Снимаем копию front-буфера СЕЙЧАС, пока он актуален.
		snap := image.NewRGBA(e.canvas.front.Rect)
		copy(snap.Pix, e.canvas.front.Pix)
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