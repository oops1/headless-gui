// xaml_props.go — применение XAML attached-свойств к виджетам.
//
// Содержит: applyGridAttachedProps, applyMargin, applyAlignment,
// applyIsEnabled, applyDockAttachedProp, loadImageFile.
package widget

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// ─── Grid attached properties ───────────────────────────────────────────────

// applyGridAttachedProps читает Grid.Row, Grid.Column и т.д. из XAML-атрибутов
// и устанавливает их в Base виджета.
func applyGridAttachedProps(w Widget, el xElement) {
	type gridSetter interface {
		GetGridRow() int // наличие этого метода означает, что Base встроен
	}
	// Все наши виджеты встраивают Base, поэтому можно писать напрямую.
	// Используем рефлексию через интерфейс не нужно — у нас есть конкретный тип.
	// Простой подход: пишем через указатель на Base.
	type baseAccessor interface {
		Widget
		GetGridRow() int
	}
	if _, ok := w.(baseAccessor); !ok {
		return
	}

	row := xatoi(el.attr("Grid.Row"))
	col := xatoi(el.attr("Grid.Column"))
	rowSpan := xatoi(el.attr("Grid.RowSpan"))
	colSpan := xatoi(el.attr("Grid.ColumnSpan"))

	// Нужно добраться до Base. Используем сеттер-интерфейс.
	type gridPropsSetter interface {
		SetGridProps(row, col, rowSpan, colSpan int)
	}
	if gs, ok := w.(gridPropsSetter); ok {
		gs.SetGridProps(row, col, rowSpan, colSpan)
	}
}

// applyMargin читает Margin из XAML-атрибутов и устанавливает в Base.
func applyMargin(w Widget, el xElement) {
	ms := el.attr("Margin")
	if ms == "" {
		return
	}
	m := parseMargin(ms)
	type marginSetter interface {
		SetMargin(m Margin)
	}
	if setter, ok := w.(marginSetter); ok {
		setter.SetMargin(m)
	}
}

// applyAlignment читает HorizontalAlignment и VerticalAlignment из XAML-атрибутов.
func applyAlignment(w Widget, el xElement) {
	type alignSetter interface {
		SetHAlign(a HorizontalAlignment)
		SetVAlign(a VerticalAlignment)
	}
	as, ok := w.(alignSetter)
	if !ok {
		return
	}
	if ha := el.attr("HorizontalAlignment"); ha != "" {
		switch strings.ToLower(ha) {
		case "left":
			as.SetHAlign(HAlignLeft)
		case "center":
			as.SetHAlign(HAlignCenter)
		case "right":
			as.SetHAlign(HAlignRight)
		case "stretch":
			as.SetHAlign(HAlignStretch)
		}
	}
	if va := el.attr("VerticalAlignment"); va != "" {
		switch strings.ToLower(va) {
		case "top":
			as.SetVAlign(VAlignTop)
		case "center":
			as.SetVAlign(VAlignCenter)
		case "bottom":
			as.SetVAlign(VAlignBottom)
		case "stretch":
			as.SetVAlign(VAlignStretch)
		}
	}
}

// ─── ToolTip ──────────────────────────────────────────────────────────────

// applyToolTip читает ToolTip из XAML-атрибутов и устанавливает в Base.
// Поддерживает простой синтаксис ToolTip="текст".
func applyToolTip(w Widget, el xElement) {
	tip := el.attr("ToolTip", "ToolTipService.ToolTip")
	if tip == "" {
		return
	}
	type tipSetter interface {
		SetToolTip(s string)
	}
	if ts, ok := w.(tipSetter); ok {
		ts.SetToolTip(tip)
	}
}

// ─── Visibility ─────────────────────────────────────────────────────────────

// applyVisibility читает Visibility из XAML-атрибутов (WPF):
// "Visible" (default), "Collapsed", "Hidden" → скрытие виджета.
func applyVisibility(w Widget, el xElement) {
	v := el.attr("Visibility")
	if v == "" {
		return
	}
	type visSetter interface {
		SetVisible(b bool)
	}
	vs, ok := w.(visSetter)
	if !ok {
		return
	}
	switch strings.ToLower(v) {
	case "collapsed", "hidden":
		vs.SetVisible(false)
	case "visible":
		vs.SetVisible(true)
	}
}

// ─── ShowLocaleIndicator ────────────────────────────────────────────────────

// applyLocaleIndicator читает ShowLocaleIndicator из XAML-атрибутов и
// устанавливает соответствующее поле у Window/Dialog/Panel.
func applyLocaleIndicator(w Widget, el xElement) {
	v := el.attr("ShowLocaleIndicator", "ShowLocale")
	if v == "" {
		return
	}
	on := strings.EqualFold(v, "true") || v == "1"
	switch t := w.(type) {
	case *Window:
		t.ShowLocaleIndicator = on
	case *Dialog:
		t.ShowLocaleIndicator = on
	case *Panel:
		t.ShowLocaleIndicator = on
	}
}

// ─── TabIndex ───────────────────────────────────────────────────────────────

// applyTabIndex читает TabIndex из XAML и задаёт порядок Tab-навигации.
func applyTabIndex(w Widget, el xElement) {
	v := el.attr("TabIndex")
	if v == "" {
		return
	}
	type tabSetter interface{ SetTabIndex(int) }
	if ts, ok := w.(tabSetter); ok {
		ts.SetTabIndex(xatoi(v))
	}
}

// ─── Общий набор attached-свойств ──────────────────────────────────────────

// applyCommonProps применяет полный набор общих XAML-свойств к виджету:
// Grid.*, DockPanel.Dock, Margin, Alignment, IsEnabled, ToolTip, Visibility,
// ShowLocaleIndicator, Cursor. Используется как контейнерами, так и листовыми
// виджетами, чтобы поведение было единообразным (см. BUG-1).
func applyCommonProps(w Widget, el xElement) {
	applyGridAttachedProps(w, el)
	applyDockAttachedProp(w, el)
	applyMargin(w, el)
	applyAlignment(w, el)
	applyIsEnabled(w, el)
	applyToolTip(w, el)
	applyVisibility(w, el)
	applyLocaleIndicator(w, el)
	applyTabIndex(w, el)
	applyCursor(w, el)
}

// applyCursor читает XAML-атрибут Cursor= и задаёт принудительный курсор виджету.
func applyCursor(w Widget, el xElement) {
	cs := strings.ToLower(strings.TrimSpace(el.attr("Cursor")))
	if cs == "" {
		return
	}
	sc, ok := w.(interface{ SetCursor(Cursor) })
	if !ok {
		return
	}
	switch cs {
	case "ibeam":
		sc.SetCursor(CursorIBeam)
	case "hand":
		sc.SetCursor(CursorHand)
	case "sizewe":
		sc.SetCursor(CursorSizeWE)
	case "sizens":
		sc.SetCursor(CursorSizeNS)
	case "arrow":
		sc.SetCursor(CursorArrow)
	}
}

// ─── IsEnabled ──────────────────────────────────────────────────────────────

// applyIsEnabled читает IsEnabled из XAML-атрибутов и устанавливает в Base.
// WPF по умолчанию IsEnabled=True, поэтому false нужно задавать явно.
func applyIsEnabled(w Widget, el xElement) {
	type enabledSetter interface {
		SetEnabled(v bool)
	}
	es, ok := w.(enabledSetter)
	if !ok {
		return
	}
	if v := el.attr("IsEnabled"); strings.EqualFold(v, "False") {
		es.SetEnabled(false)
	}
}

// ─── DockPanel.Dock attached property ───────────────────────────────────────

// applyDockAttachedProp читает DockPanel.Dock из XAML-атрибутов и устанавливает в Base.
func applyDockAttachedProp(w Widget, el xElement) {
	dock := el.attr("DockPanel.Dock")
	if dock == "" {
		return
	}
	type dockSetter interface {
		SetDock(d DockSide)
	}
	if ds, ok := w.(dockSetter); ok {
		switch strings.ToLower(dock) {
		case "top":
			ds.SetDock(DockTop)
		case "bottom":
			ds.SetDock(DockBottom)
		case "left":
			ds.SetDock(DockLeft)
		case "right":
			ds.SetDock(DockRight)
		}
	}
}

// ─── Резолв путей к ресурсам из XAML (SEC-8) ────────────────────────────────

// ErrResourceOutsideBase — путь ресурса выводит за пределы каталога XAML-файла.
var ErrResourceOutsideBase = errors.New("xaml: путь ресурса выходит за пределы каталога разметки")

// xamlResourceLogged — уже залогированные отклонённые пути (чтобы разметка,
// повторяющая один и тот же плохой путь в сотне элементов, не залила журнал).
var xamlResourceLogged sync.Map

// resolveXAMLResource приводит путь ресурса из XAML-атрибута (Image Source,
// Button IconSource, Panel BackgroundImage, SVGIcon Source, TrayIcon) к пути
// на диске и проверяет, что чтение остаётся в дозволенных границах.
//
// Политика (SEC-8):
//
//   - baseDir ЗАДАН — разметка пришла ИЗ ФАЙЛА. Такой XAML нередко приходит
//     извне (тема, плагин, скачанный макет), поэтому он трактуется как данные,
//     а не как код: путь считается относительным от каталога XAML-файла и
//     ОБЯЗАН остаться внутри него. Абсолютные пути («C:\Windows\…», «/etc/…»)
//     и выходы наружу («..\..\secret.png») отклоняются — иначе один атрибут
//     Source= давал бы чтение любого файла, доступного процессу.
//
//   - baseDir ПУСТ — разметка передана строкой из кода (LoadUIFromXAML,
//     LoadUIFromXAMLWithContext). Строку сформировал сам программист, границы
//     задавать не от кого и незачем: относительный путь резолвится от текущего
//     каталога, абсолютный разрешён — ровно как при прямом os.Open в его коде.
//
// Символические ссылки намеренно не разыменовываются: путь проверяется
// лексически (после filepath.Clean). Для дерева ресурсов приложения этого
// достаточно, а EvalSymlinks на каждую иконку — лишний поход в ФС.
func resolveXAMLResource(baseDir, src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", errors.New("xaml: пустой путь ресурса")
	}
	if baseDir == "" {
		// Загрузка из строки — доверяем вызывающему коду (см. комментарий выше).
		return filepath.Clean(src), nil
	}
	// Абсолютный путь, корневой («/x», «\x») или с указанием тома («C:x»)
	// уводит из baseDir ещё до Join — отклоняем сразу.
	if filepath.IsAbs(src) || strings.HasPrefix(src, "/") || strings.HasPrefix(src, `\`) ||
		filepath.VolumeName(src) != "" {
		err := fmt.Errorf("%w: абсолютный путь %q", ErrResourceOutsideBase, src)
		logResourceRejected(src, err)
		return "", err
	}
	full := filepath.Join(baseDir, src) // Join уже делает Clean
	rel, err := filepath.Rel(baseDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rerr := fmt.Errorf("%w: %q", ErrResourceOutsideBase, src)
		logResourceRejected(src, rerr)
		return "", rerr
	}
	return full, nil
}

// logResourceRejected пишет в журнал один раз на каждый отклонённый путь.
func logResourceRejected(src string, err error) {
	if _, seen := xamlResourceLogged.LoadOrStore(src, struct{}{}); seen {
		return
	}
	log.Printf("xaml: ресурс отклонён: %v", err)
}

// ─── Ограничения на декодирование изображений (SEC-9) ───────────────────────

const (
	// MaxImageFileBytes — предельный размер файла изображения. Больше этого
	// не читаем вовсе: PNG/JPEG таких размеров в UI не бывает, а вот «файл»
	// из /dev/zero или сетевой шары может быть бесконечным.
	MaxImageFileBytes = 256 << 20 // 256 МБ

	// defaultMaxImagePixels — предел площади изображения по умолчанию.
	// 64 Мпикс в RGBA — это 256 МБ на буфер; заголовок PNG допускает
	// 65535×65535 (≈4.3 Гпикс, 16 ГБ при декодировании) при считанных
	// килобайтах самого файла — классическая декомпрессионная бомба.
	defaultMaxImagePixels = 64 << 20 // 64 Мпикс
)

// maxImagePixels — текущий предел площади (атомарно: настраивается из кода
// приложения, читается из потоков отрисовки/загрузки).
var maxImagePixels atomic.Int64

func init() { maxImagePixels.Store(defaultMaxImagePixels) }

// MaxImagePixels возвращает текущий предел площади декодируемого изображения
// (в пикселях).
func MaxImagePixels() int64 { return maxImagePixels.Load() }

// SetMaxImagePixels меняет предел площади декодируемого изображения.
// Значение ≤ 0 возвращает предел по умолчанию (64 Мпикс).
func SetMaxImagePixels(n int64) {
	if n <= 0 {
		n = defaultMaxImagePixels
	}
	maxImagePixels.Store(n)
}

// checkImageBounds проверяет заявленные в заголовке размеры изображения.
func checkImageBounds(w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("некорректный размер изображения %dx%d", w, h)
	}
	limit := MaxImagePixels()
	if px := int64(w) * int64(h); px > limit {
		return fmt.Errorf("изображение %dx%d (%d пикс.) превышает предел %d пикс.", w, h, px, limit)
	}
	return nil
}

// DecodeImageBounded декодирует изображение из r с защитой от
// декомпрессионной бомбы (SEC-9).
//
// Сначала читается ТОЛЬКО заголовок (image.DecodeConfig) и проверяется
// заявленная площадь: если она превышает MaxImagePixels, декодирование не
// начинается вовсе — то есть память под гигабайтный растр не выделяется.
// Дополнительно поток ограничен MaxImageFileBytes.
//
// Экспортирована, чтобы движок (engine.SetBackgroundFile) читал изображения
// через ту же проверку: engine импортирует widget, обратное невозможно.
func DecodeImageBounded(r io.Reader) (image.Image, error) {
	if rs, ok := r.(io.ReadSeeker); ok {
		// Файл: читаем заголовок и перематываемся — без буферизации всего
		// содержимого в память.
		cfg, _, err := image.DecodeConfig(bufio.NewReader(io.LimitReader(rs, MaxImageFileBytes)))
		if err != nil {
			return nil, err
		}
		if err := checkImageBounds(cfg.Width, cfg.Height); err != nil {
			return nil, err
		}
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bufio.NewReader(io.LimitReader(rs, MaxImageFileBytes)))
		return img, err
	}

	// Поток без перемотки — буферизуем, но не больше предела.
	data, err := io.ReadAll(io.LimitReader(r, MaxImageFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxImageFileBytes {
		return nil, fmt.Errorf("изображение больше предела %d байт", int64(MaxImageFileBytes))
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := checkImageBounds(cfg.Width, cfg.Height); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// LoadImageFile загружает PNG или JPEG файл и возвращает *image.RGBA.
// Экспортирован для использования в приложениях (иконки TreeView, и т.д.).
func LoadImageFile(path string) (*image.RGBA, error) {
	return loadImageFile(path)
}

// loadImageFile загружает PNG или JPEG файл и возвращает *image.RGBA.
// Размер файла и площадь изображения ограничены (см. DecodeImageBounded).
func loadImageFile(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// image.Decode использует зарегистрированные декодеры (png, jpeg);
	// перед ним проверяется заголовок — защита от бомбы (SEC-9).
	img, err := DecodeImageBounded(f)
	if err != nil {
		return nil, err
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	// Конвертируем в RGBA
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba, nil
}
