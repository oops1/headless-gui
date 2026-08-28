// themeicons.go — реестр иконок темы (theme.IconResolver).
//
// Компонент просит иконку по имени («start», «volume.muted»), а тема решает,
// какой картинкой это нарисовать: файлом SVG, заранее зарегистрированными
// данными/изображением или встроенным набором, нарисованным кодом. IconSet —
// единственная реализация theme.IconResolver в этом пакете; theme ничего не
// знает о растеризации (см. комментарий у theme.IconResolver).
//
// Растеризация SVG дорога, а иконку одного размера просят на каждом кадре
// отрисовки — поэтому результат кэшируется по (имя/путь, размер) и отдаётся
// повторно тем же указателем без пересчёта.
package widget

import (
	"image"
	"image/color"
	"log"
	"sync"

	"github.com/oops1/headless-gui/v3/theme"
	svgPkg "github.com/oops1/headless-gui/v3/widget/svg"
)

// ─── IconSet ─────────────────────────────────────────────────────────────────

// iconSource — то, из чего IconSet умеет получить растровую картинку нужного
// размера: разобранный SVG, готовое изображение или генератор кода.
type iconSource interface {
	render(size int) image.Image
}

// svgSource — иконка из SVG-документа: перерастеризуется под каждый size,
// currentColor подставляется цветом текста темы (как у SVGIcon/TrayIcon),
// tint выключен — сохраняются собственные цвета документа.
//
// Кэш растеризаций — у самого *svg.Document (RasterizeCached), поэтому
// повторный запрос того же размера возвращает тот же *image.RGBA без
// повторной работы, даже в обход верхнего кэша IconSet.
type svgSource struct{ doc *svgPkg.Document }

func (s svgSource) render(size int) image.Image {
	if s.doc == nil {
		return nil
	}
	return s.doc.RasterizeCached(size, size, win10.LabelText, false)
}

// imageSource — заранее готовое изображение (RegisterImage). Размер не
// подгоняется: вызывающий регистрирует изображение уже нужного вида.
type imageSource struct{ img image.Image }

func (s imageSource) render(int) image.Image { return s.img }

// funcSource — иконка, нарисованная кодом (встроенный набор, BuiltinIcons).
type funcSource struct{ fn func(size int) *image.RGBA }

func (s funcSource) render(size int) image.Image {
	if s.fn == nil {
		return nil
	}
	if img := s.fn(size); img != nil {
		return img
	}
	return nil
}

// cacheKey — ключ верхнего кэша растеризаций: 'n' — по зарегистрированному
// имени, 'p' — по разрешённому пути файла.
type cacheKey struct {
	kind byte
	id   string
	size int
}

// IconSet — реализация theme.IconResolver: реестр иконок темы.
//
// Источник иконки — SVG-файл под baseDir (IconRef.Source) либо заранее
// зарегистрированные данные/изображение (IconRef.Name). Если заданы оба —
// приоритет у Name: зарегистрированное важнее файла.
//
// Все методы безопасны для вызова из нескольких горутин — ResolveIcon зовут
// из отрисовки, Register/RegisterImage могут прийти в это же время из
// загрузки темы.
type IconSet struct {
	baseDir string

	mu      sync.RWMutex
	sources map[string]iconSource // имя → источник (Register/RegisterImage/BuiltinIcons)
	paths   map[string]iconSource // разрешённый путь файла → источник (лениво, при первом запросе)

	cacheMu sync.RWMutex
	cache   map[cacheKey]image.Image

	phMu         sync.RWMutex
	placeholders map[int]image.Image // заглушка на размер
}

// NewIconSet создаёт пустой набор иконок. baseDir — каталог, относительно
// которого разрешаются пути IconRef.Source; выход за его пределы («../…»)
// отклоняется тем же способом, что и ресурсы XAML (SEC-8,
// resolveXAMLResource) — см. widget/xaml_props.go.
func NewIconSet(baseDir string) *IconSet {
	return &IconSet{baseDir: baseDir}
}

// ResolveIcon реализует theme.IconResolver. Никогда не возвращает nil: если
// иконка не найдена (нет файла, битый SVG, пустая ссылка, путь вне baseDir),
// отдаётся нарисованный заглушечный глиф нужного размера — пропуск иконки не
// должен ронять отрисовку.
func (s *IconSet) ResolveIcon(ref theme.IconRef, size int) image.Image {
	if size < 1 {
		size = 1
	}

	switch {
	case ref.Name != "":
		if img := s.resolveNamed(ref.Name, size); img != nil {
			return img
		}
	case ref.Source != "":
		if img := s.resolveSourced(ref.Source, size); img != nil {
			return img
		}
	}
	return s.placeholder(size)
}

func (s *IconSet) resolveNamed(name string, size int) image.Image {
	key := cacheKey{kind: 'n', id: name, size: size}
	if img, ok := s.cacheGet(key); ok {
		return img
	}

	s.mu.RLock()
	src, ok := s.sources[name]
	s.mu.RUnlock()
	if !ok {
		return nil
	}

	img := src.render(size)
	if img == nil {
		return nil
	}
	s.cacheSet(key, img)
	return img
}

func (s *IconSet) resolveSourced(rawSrc string, size int) image.Image {
	// Путь удерживается внутри baseDir тем же способом, что и ресурсы XAML
	// (SEC-8) — абсолютные пути и выходы через "../" отклоняются.
	path, err := resolveXAMLResource(s.baseDir, rawSrc)
	if err != nil {
		return nil
	}

	key := cacheKey{kind: 'p', id: path, size: size}
	if img, ok := s.cacheGet(key); ok {
		return img
	}

	s.mu.RLock()
	src, ok := s.paths[path]
	s.mu.RUnlock()
	if !ok {
		doc, perr := svgPkg.ParseFile(path)
		if perr != nil {
			return nil
		}
		src = svgSource{doc: doc}
		s.mu.Lock()
		if s.paths == nil {
			s.paths = map[string]iconSource{}
		}
		s.paths[path] = src
		s.mu.Unlock()
	}

	img := src.render(size)
	if img == nil {
		return nil
	}
	s.cacheSet(key, img)
	return img
}

// Register регистрирует иконку по имени из данных SVG (лимиты
// svg.MaxFileBytes/svg.MaxDepth). Битые данные — не регистрируются: имя
// продолжит отдавать заглушку, ошибка пишется в журнал, отрисовка не падает.
func (s *IconSet) Register(name string, svgData []byte) {
	if name == "" {
		return
	}
	doc, err := svgPkg.Parse(svgData)
	if err != nil {
		log.Printf("widget: IconSet.Register(%q): %v", name, err)
		return
	}
	s.setSource(name, svgSource{doc: doc})
}

// RegisterImage регистрирует иконку по имени из готового изображения —
// без растеризации, отдаётся как есть.
func (s *IconSet) RegisterImage(name string, img image.Image) {
	if name == "" || img == nil {
		return
	}
	s.setSource(name, imageSource{img: img})
}

// registerFunc регистрирует иконку, нарисованную кодом (BuiltinIcons).
func (s *IconSet) registerFunc(name string, fn func(size int) *image.RGBA) {
	s.setSource(name, funcSource{fn: fn})
}

func (s *IconSet) setSource(name string, src iconSource) {
	s.mu.Lock()
	if s.sources == nil {
		s.sources = map[string]iconSource{}
	}
	s.sources[name] = src
	s.mu.Unlock()
	s.invalidateNamed(name)
}

// invalidateNamed сбрасывает кэш растеризаций для имени — повторная
// регистрация того же имени не должна отдавать картинку от предыдущей.
func (s *IconSet) invalidateNamed(name string) {
	s.cacheMu.Lock()
	for k := range s.cache {
		if k.kind == 'n' && k.id == name {
			delete(s.cache, k)
		}
	}
	s.cacheMu.Unlock()
}

func (s *IconSet) cacheGet(k cacheKey) (image.Image, bool) {
	s.cacheMu.RLock()
	img, ok := s.cache[k]
	s.cacheMu.RUnlock()
	return img, ok
}

func (s *IconSet) cacheSet(k cacheKey, img image.Image) {
	s.cacheMu.Lock()
	if s.cache == nil {
		s.cache = map[cacheKey]image.Image{}
	}
	s.cache[k] = img
	s.cacheMu.Unlock()
}

// placeholder возвращает (и кэширует по размеру) заглушечный глиф: рамка с
// диагональным крестом.
func (s *IconSet) placeholder(size int) image.Image {
	s.phMu.RLock()
	if img, ok := s.placeholders[size]; ok {
		s.phMu.RUnlock()
		return img
	}
	s.phMu.RUnlock()

	img := drawPlaceholder(size)

	s.phMu.Lock()
	if s.placeholders == nil {
		s.placeholders = map[int]image.Image{}
	}
	if existing, ok := s.placeholders[size]; ok {
		s.phMu.Unlock()
		return existing
	}
	s.placeholders[size] = img
	s.phMu.Unlock()
	return img
}

// ─── Встроенный набор, нарисованный кодом ───────────────────────────────────

// BuiltinIcons — набор иконок панели задач, не требующий ни одного файла на
// диске: имена регистрируются как генераторы, рисующие *image.RGBA
// примитивами (прямоугольники, линии) под запрошенный размер.
func BuiltinIcons() *IconSet {
	s := NewIconSet("")
	s.registerFunc("start", drawStartIcon)
	s.registerFunc("volume", drawVolumeIcon)
	s.registerFunc("volume.muted", drawVolumeMutedIcon)
	s.registerFunc("network.wifi", drawWifiIcon)
	s.registerFunc("network.ethernet", drawEthernetIcon)
	s.registerFunc("battery", drawBatteryIcon)
	return s
}

// ─── Рисование примитивами ───────────────────────────────────────────────────

// iconFG — цвет по умолчанию для встроенных иконок и заглушки: нейтральный
// серый, различимый и на светлой, и на тёмной панели задач.
var iconFG = color.RGBA{R: 0xC8, G: 0xC8, B: 0xC8, A: 0xFF}

var placeholderFG = color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}

func newIconCanvas(size int) *image.RGBA {
	if size < 1 {
		size = 1
	}
	return image.NewRGBA(image.Rect(0, 0, size, size))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fillRect закрашивает прямоугольник (обрезая по границам изображения).
func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// strokeRect рисует прямоугольную рамку толщиной w.
func strokeRect(img *image.RGBA, r image.Rectangle, c color.RGBA, w int) {
	if w < 1 {
		w = 1
	}
	fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w), c)
	fillRect(img, image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y), c)
	fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y), c)
	fillRect(img, image.Rect(r.Max.X-w, r.Min.Y, r.Max.X, r.Max.Y), c)
}

// drawLine рисует отрезок толщиной w (алгоритм Брезенхэма, точка — квадрат w×w).
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, w int) {
	if w < 1 {
		w = 1
	}
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		fillRect(img, image.Rect(x0-w/2, y0-w/2, x0-w/2+w, y0-w/2+w), c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// fillTriangleRight закрашивает треугольник с вертикальной гранью в x0 (от
// yTop до yBot) и вершиной в (xTip, середина) — грубый конус динамика.
func fillTriangleRight(img *image.RGBA, x0, yTop, yBot, xTip int, c color.RGBA) {
	if yBot <= yTop {
		return
	}
	midY := (yTop + yBot) / 2
	for y := yTop; y <= yBot; y++ {
		var t float64
		if y <= midY {
			t = float64(y-yTop) / float64(midY-yTop+1)
		} else {
			t = float64(yBot-y) / float64(yBot-midY+1)
		}
		x1 := x0 + int(t*float64(xTip-x0))
		if x1 < x0 {
			x1 = x0
		}
		fillRect(img, image.Rect(x0, y, x1+1, y+1), c)
	}
}

// drawArcCorner рисует грубую дугу четвертью окружности (две грани), центр —
// (cx, cy), радиус r, открытую вверх-вправо: используется для «волн» звука
// и дуг wifi.
func drawArcCorner(img *image.RGBA, cx, cy, r int, c color.RGBA, w int) {
	drawLine(img, cx, cy-r, cx+r, cy, c, w)
}

// drawPlaceholder — заглушечный глиф: рамка с диагональным крестом.
func drawPlaceholder(size int) *image.RGBA {
	img := newIconCanvas(size)
	inset := maxInt(1, size/8)
	th := maxInt(1, size/12)
	r := image.Rect(inset, inset, size-inset, size-inset)
	if r.Dx() <= 0 || r.Dy() <= 0 {
		r = img.Bounds()
	}
	strokeRect(img, r, placeholderFG, th)
	drawLine(img, r.Min.X, r.Min.Y, r.Max.X-1, r.Max.Y-1, placeholderFG, th)
	drawLine(img, r.Max.X-1, r.Min.Y, r.Min.X, r.Max.Y-1, placeholderFG, th)
	return img
}

// drawStartIcon — четыре квадрата решёткой (кнопка «Пуск»).
func drawStartIcon(size int) *image.RGBA {
	img := newIconCanvas(size)
	pad := maxInt(1, size/8)
	gap := maxInt(1, size/10)
	cell := maxInt(1, (size-2*pad-gap)/2)

	colors := [4]color.RGBA{
		{R: 0xF2, G: 0x5A, B: 0x22, A: 0xFF},
		{R: 0x7F, G: 0xBA, B: 0x00, A: 0xFF},
		{R: 0x00, G: 0xA4, B: 0xEF, A: 0xFF},
		{R: 0xFF, G: 0xB9, B: 0x00, A: 0xFF},
	}
	origins := [4]image.Point{
		{X: pad, Y: pad},
		{X: pad + cell + gap, Y: pad},
		{X: pad, Y: pad + cell + gap},
		{X: pad + cell + gap, Y: pad + cell + gap},
	}
	for i, o := range origins {
		fillRect(img, image.Rect(o.X, o.Y, o.X+cell, o.Y+cell), colors[i])
	}
	return img
}

// drawSpeaker рисует общую часть volume/volume.muted: корпус и конус
// динамика. Возвращает координату вершины конуса — откуда рисовать волны
// или крест "приглушено".
func drawSpeaker(img *image.RGBA, size int) (tipX, cy int) {
	cy = size / 2
	boxW := maxInt(1, size/5)
	boxH := maxInt(2, size/3)
	boxX := maxInt(1, size/6)
	fillRect(img, image.Rect(boxX, cy-boxH/2, boxX+boxW, cy+boxH/2), iconFG)

	coneX := boxX + boxW
	coneTip := minInt(size-2, coneX+maxInt(2, size/3))
	fillTriangleRight(img, coneX, cy-boxH, cy+boxH, coneTip, iconFG)
	return coneTip, cy
}

// drawVolumeIcon — динамик с двумя дугами звуковых волн.
func drawVolumeIcon(size int) *image.RGBA {
	img := newIconCanvas(size)
	tipX, cy := drawSpeaker(img, size)
	w := maxInt(1, size/16)
	drawArcCorner(img, tipX, cy, maxInt(1, size/6), iconFG, w)
	drawArcCorner(img, tipX, cy, maxInt(2, size/3-1), iconFG, w)
	return img
}

// drawVolumeMutedIcon — динамик с крестом вместо волн.
func drawVolumeMutedIcon(size int) *image.RGBA {
	img := newIconCanvas(size)
	tipX, cy := drawSpeaker(img, size)
	w := maxInt(1, size/16)
	r := maxInt(2, size/4)
	drawLine(img, tipX, cy-r, tipX+r, cy+r, iconFG, w)
	drawLine(img, tipX+r, cy-r, tipX, cy+r, iconFG, w)
	return img
}

// drawWifiIcon — три вложенные дуги и точка (уровни сигнала).
func drawWifiIcon(size int) *image.RGBA {
	img := newIconCanvas(size)
	cx := size / 2
	cy := size - maxInt(1, size/6)
	w := maxInt(1, size/12)

	dot := maxInt(1, size/10)
	fillRect(img, image.Rect(cx-dot/2, cy-dot/2, cx+dot/2+1, cy+dot/2+1), iconFG)

	for i, r := range [3]int{size / 4, size / 3, size / 2} {
		_ = i
		if r < 2 {
			continue
		}
		// Дуга «домиком» из двух отрезков — раскрыта вверх.
		drawLine(img, cx-r, cy-1, cx, cy-r, iconFG, w)
		drawLine(img, cx, cy-r, cx+r, cy-1, iconFG, w)
	}
	return img
}

// drawEthernetIcon — прямоугольный разъём со штекером.
func drawEthernetIcon(size int) *image.RGBA {
	img := newIconCanvas(size)
	bodyW := maxInt(2, size*3/5)
	bodyH := maxInt(2, size/2)
	bx := (size - bodyW) / 2
	by := size/2 - bodyH/2
	strokeRect(img, image.Rect(bx, by, bx+bodyW, by+bodyH), iconFG, maxInt(1, size/14))

	plugW := maxInt(1, bodyW/3)
	plugH := maxInt(1, size/6)
	px := bx + (bodyW-plugW)/2
	fillRect(img, image.Rect(px, by-plugH, px+plugW, by), iconFG)
	return img
}

// drawBatteryIcon — прямоугольник с делением (уровень заряда) и контактом.
func drawBatteryIcon(size int) *image.RGBA {
	img := newIconCanvas(size)
	bodyW := maxInt(2, size*3/4)
	bodyH := maxInt(2, size/2)
	bx := maxInt(1, size/10)
	by := (size - bodyH) / 2
	th := maxInt(1, size/14)
	strokeRect(img, image.Rect(bx, by, bx+bodyW, by+bodyH), iconFG, th)

	nubW := maxInt(1, size/12)
	nubH := maxInt(1, bodyH/2)
	fillRect(img, image.Rect(bx+bodyW, by+(bodyH-nubH)/2, bx+bodyW+nubW, by+(bodyH-nubH)/2+nubH), iconFG)

	// Уровень заряда — заполненная часть внутри рамки (~60%).
	innerPad := th + maxInt(1, size/20)
	inner := image.Rect(bx+innerPad, by+innerPad, bx+bodyW-innerPad, by+bodyH-innerPad)
	if inner.Dx() > 0 && inner.Dy() > 0 {
		fillW := inner.Dx() * 3 / 5
		fillRect(img, image.Rect(inner.Min.X, inner.Min.Y, inner.Min.X+fillW, inner.Max.Y), iconFG)
	}
	return img
}
