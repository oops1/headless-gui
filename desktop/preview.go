// preview.go — предпросмотр окна при наведении на кнопку панели задач.
//
// Наведение на кнопку окна показывает всплывающее окно с миниатюрой этого
// окна, его значком и заголовком — так это устроено в Windows. Уводим курсор
// — окно закрывается; ведём вдоль панели — окно не мигает, а переезжает к
// соседней кнопке.
//
// Собрано на общей основе всплывающих панелей (Flyout): открытие от значка,
// закрытие по клику мимо и Esc, вынос в оверлей — всё оттуда. Предпросмотр —
// пятая такая панель, а не новый механизм.
//
// Цена по сети — главное требование к этой панели, и оно задаёт устройство:
//
//   - миниатюра берётся ПО ТАЙМЕРУ, а не в каждом кадре. Живая миниатюра в
//     каждом кадре — прямая дорога к тому, от чего движок ушёл в 3.16.1:
//     неподвижный рабочий стол начинал слать кадры непрерывно;
//   - таймеры сделаны анимациями без покадрового колбэка. Такая анимация
//     кадров не требует (см. Engine.animationNeeded): движок продвигает её в
//     StepAnimations и зовёт OnDone, а кадр случается, только если OnDone
//     что-то заявил;
//   - открытие, переезд и закрытие заявляют СВОЙ прямоугольник, а не экран.
package desktop

import (
	"image"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ComponentPreview — имя компонента для стилей темы.
const ComponentPreview = "preview"

// Части компонента: миниатюра и строка заголовка над ней.
const (
	previewPartThumb  = "thumb"
	previewPartHeader = "header"
)

// Ключи темы, которыми профиль управляет предпросмотром.
const (
	// KeyPreview — показывать ли предпросмотр вообще. Windows 2000 его не
	// имела, и выключить его для этого профиля исторически верно.
	KeyPreview theme.Key = "preview"
	// KeyPreviewWidth и KeyPreviewHeight — размер миниатюры.
	KeyPreviewWidth  theme.Key = "preview.width"
	KeyPreviewHeight theme.Key = "preview.height"
	// KeyPreviewPad — поля вокруг содержимого панели.
	KeyPreviewPad theme.Key = "preview.pad"
	// KeyPreviewHeader — высота строки со значком и заголовком.
	KeyPreviewHeader theme.Key = "preview.header"
	// KeyPreviewDelayOpen — задержка перед появлением, мс. Без неё панель
	// мигает при проведении курсором вдоль кнопок.
	KeyPreviewDelayOpen theme.Key = "preview.delay.open"
	// KeyPreviewDelayClose — задержка перед закрытием, мс: за это время
	// курсор успевает дойти с кнопки до самой миниатюры.
	KeyPreviewDelayClose theme.Key = "preview.delay.close"
	// KeyPreviewRefresh — как часто обновляется миниатюра, мс. Это
	// предпросмотр, а не второй экран: нескольких кадров в секунду довольно.
	KeyPreviewRefresh theme.Key = "preview.refresh"
)

// Значения по умолчанию — на случай темы, которая ключ не задала. Числа сняты
// с Windows: полсекунды до появления, четверть секунды на увод, миниатюра
// 200x120, обновление пять раз в секунду.
const (
	previewDefWidth   = 200
	previewDefHeight  = 120
	previewDefPad     = 8
	previewDefHeader  = 20
	previewDefOpenMs  = 500
	previewDefCloseMs = 250
	previewDefRefresh = 200
)

// WindowPreview — всплывающая панель с миниатюрой окна.
type WindowPreview struct {
	*Flyout

	tm *theme.Manager
	wm WindowModel

	mu sync.Mutex
	// win — окно, которое показывается сейчас.
	win    WindowInfo
	hasWin bool
	// thumb — последняя взятая миниатюра. Держится и после сворачивания окна:
	// Windows ведёт себя так же, миниатюра свёрнутого окна замирает.
	thumb image.Image
	// openTimer, closeTimer, refresh — анимации-таймеры (см. описание файла).
	openTimer  *widget.Animation
	closeTimer *widget.Animation
	refresh    *widget.Animation
	// pending* — кнопка, ради которой заведён openTimer.
	pendingIdx    int
	pendingAnchor image.Rectangle
	pendingWin    WindowInfo

	// area — область кнопок окон, за наведением которой следит панель.
	area *RunningApplications
}

// NewWindowPreview создаёт панель предпросмотра, оформляемую темой tm и
// берущую окна из wm.
func NewWindowPreview(tm *theme.Manager, wm WindowModel) *WindowPreview {
	p := &WindowPreview{
		Flyout:     NewFlyout(tm, ComponentPreview),
		tm:         tm,
		wm:         wm,
		pendingIdx: -1,
	}
	p.Content = p.draw
	p.Size = p.size
	// Уборка при закрытии — через хук основы: панель гасят и со стороны
	// (движок при клике мимо, Esc), а такое закрытие идёт мимо Close этого
	// типа — встраивание в Go виртуальных вызовов не даёт.
	p.afterClose = p.stopEverything
	return p
}

// Track подключает панель к области кнопок окон: дальше она сама следит за
// наведением и решает, что показывать.
//
// Оболочке остаётся положить обе в дерево. Прямоугольник кнопки панель берёт
// у области (ButtonRect) — считать его снаружи значило бы повторить у себя
// три режима раскладки кнопок и ломать эту копию при каждом их изменении.
func (p *WindowPreview) Track(area *RunningApplications) {
	p.mu.Lock()
	p.area = area
	p.mu.Unlock()
	if area != nil {
		area.SetHoverListener(p.hoverChanged)
	}
}

// Enabled сообщает, хочет ли тема предпросмотра вообще.
func (p *WindowPreview) Enabled() bool {
	return p.tm == nil || p.tm.GetFlag(KeyPreview, true)
}

// previews возвращает источник миниатюр, если модель окон его предоставляет.
func (p *WindowPreview) previews() WindowPreviews {
	src, _ := p.wm.(WindowPreviews)
	return src
}

// hoverChanged — наведение на кнопках изменилось.
func (p *WindowPreview) hoverChanged(idx int) {
	if !p.Enabled() || p.previews() == nil {
		return
	}
	if idx < 0 {
		p.cancelOpen()
		p.scheduleClose()
		return
	}

	p.mu.Lock()
	area := p.area
	p.mu.Unlock()
	if area == nil {
		return
	}
	info, ok := area.WindowAt(idx)
	if !ok {
		return
	}
	anchor := area.ButtonRect(idx)
	if anchor.Empty() {
		return
	}

	p.cancelClose()
	if p.IsOpen() {
		// Панель уже висит: переезжаем к соседней кнопке БЕЗ закрытия.
		// Закрытие с новым открытием даёт заметное мигание.
		p.showFor(info, anchor)
		return
	}
	p.scheduleOpen(idx, info, anchor)
}

// scheduleOpen заводит задержку перед появлением.
func (p *WindowPreview) scheduleOpen(idx int, info WindowInfo, anchor image.Rectangle) {
	p.mu.Lock()
	same := p.pendingIdx == idx && p.openTimer != nil && p.openTimer.Running()
	p.pendingIdx, p.pendingWin, p.pendingAnchor = idx, info, anchor
	p.mu.Unlock()
	if same {
		return // та же кнопка, отсчёт уже идёт
	}

	p.stopOpenTimer()
	// Анимация без покадрового колбэка: кадров она не требует, а по завершении
	// зовёт OnDone. Ровно таймер, и ничего сверх него.
	a := widget.Animate(p.delay(KeyPreviewDelayOpen, previewDefOpenMs), nil, nil)
	a.OnDone = func() {
		p.mu.Lock()
		info, anchor := p.pendingWin, p.pendingAnchor
		p.openTimer = nil
		p.mu.Unlock()
		if !anchor.Empty() {
			p.showFor(info, anchor)
		}
	}
	p.mu.Lock()
	p.openTimer = a
	p.mu.Unlock()
}

// scheduleClose заводит задержку перед закрытием: за это время курсор успеет
// дойти с кнопки до самой миниатюры.
func (p *WindowPreview) scheduleClose() {
	if !p.IsOpen() {
		return
	}
	p.mu.Lock()
	running := p.closeTimer != nil && p.closeTimer.Running()
	p.mu.Unlock()
	if running {
		return
	}

	a := widget.Animate(p.delay(KeyPreviewDelayClose, previewDefCloseMs), nil, nil)
	a.OnDone = func() {
		p.mu.Lock()
		p.closeTimer = nil
		p.mu.Unlock()
		p.Close()
	}
	p.mu.Lock()
	p.closeTimer = a
	p.mu.Unlock()
}

// stopOpenTimer снимает отсчёт до появления, не трогая запомненную кнопку.
func (p *WindowPreview) stopOpenTimer() {
	p.mu.Lock()
	a := p.openTimer
	p.openTimer = nil
	p.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

func (p *WindowPreview) cancelOpen() {
	p.stopOpenTimer()
	p.mu.Lock()
	p.pendingIdx = -1
	p.pendingAnchor = image.Rectangle{}
	p.mu.Unlock()
}

func (p *WindowPreview) cancelClose() {
	p.mu.Lock()
	a := p.closeTimer
	p.closeTimer = nil
	p.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// showFor показывает миниатюру окна info, прижав панель к кнопке anchor.
func (p *WindowPreview) showFor(info WindowInfo, anchor image.Rectangle) {
	p.mu.Lock()
	same := p.hasWin && p.win.ID == info.ID
	p.win, p.hasWin = info, true
	if !same {
		// Миниатюра прежнего окна к этому отношения не имеет: показать её —
		// значит на мгновение подписать чужую картинку чужим заголовком.
		p.thumb = nil
	}
	p.mu.Unlock()

	p.grabThumb()
	p.Align = AlignCenter // панель прижата к своей кнопке по центру
	p.Open(anchor)
	p.startRefresh()
}

// startRefresh заводит повторяющееся обновление миниатюры.
//
// Цепочка ОДНОРАЗОВЫХ таймеров, а не один зациклённый: зациклённая анимация
// по кругу зовёт покадровый колбэк и никогда — OnDone (цикл штатно не
// завершается), а покадровый колбэк здесь и не нужен. Заводить следующий
// таймер из OnDone предыдущего разрешено прямо контрактом аниматора.
func (p *WindowPreview) startRefresh() {
	p.mu.Lock()
	running := p.refresh != nil && p.refresh.Running()
	p.mu.Unlock()
	if running {
		return
	}
	p.armRefresh()
}

func (p *WindowPreview) armRefresh() {
	a := widget.Animate(p.delay(KeyPreviewRefresh, previewDefRefresh), nil, nil)
	a.OnDone = func() {
		p.mu.Lock()
		p.refresh = nil
		p.mu.Unlock()
		if !p.IsOpen() {
			return
		}
		p.grabThumb()
		p.armRefresh()
	}
	p.mu.Lock()
	p.refresh = a
	p.mu.Unlock()
}

func (p *WindowPreview) stopRefresh() {
	p.mu.Lock()
	a := p.refresh
	p.refresh = nil
	p.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// grabThumb берёт свежую миниатюру и заявляет СВОЙ прямоугольник.
//
// Заявляет всегда, а не при изменении картинки: сравнивать изображения
// попиксельно на каждом обновлении дороже, чем перерисовать область размером
// с миниатюру, а решать «изменилось ли окно» — работа источника, не панели.
func (p *WindowPreview) grabThumb() {
	src := p.previews()
	if src == nil {
		return
	}
	p.mu.Lock()
	info, ok := p.win, p.hasWin
	p.mu.Unlock()
	if !ok {
		return
	}

	img := src.Preview(info.ID, p.thumbMax())

	p.mu.Lock()
	p.thumb = img
	p.mu.Unlock()
	if r := p.rect(); !r.Empty() {
		widget.InvalidateRect(r)
	}
}

// Thumbnail возвращает показываемую сейчас миниатюру (nil — её нет).
func (p *WindowPreview) Thumbnail() image.Image {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.thumb
}

// Window возвращает окно, которое показывает панель.
func (p *WindowPreview) Window() (WindowInfo, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.win, p.hasWin
}

// stopEverything снимает таймеры и забывает показанное окно.
//
// Отсчёт до появления снимается тоже: без этого панель, погашенная кликом
// мимо, через мгновение открылась бы снова — таймер-то остался.
func (p *WindowPreview) stopEverything() {
	p.cancelOpen()
	p.cancelClose()
	p.stopRefresh()
	p.mu.Lock()
	p.hasWin = false
	p.thumb = nil
	p.mu.Unlock()
}

// Close закрывает панель и снимает все её таймеры.
func (p *WindowPreview) Close() {
	p.Flyout.Close()
	// Закрытие закрытой — не событие, и afterClose тогда не зовётся: убираем
	// сами, иначе отсчёт до появления пережил бы явное закрытие.
	p.stopEverything()
}

// OnMouseMove держит панель открытой, пока курсор на ней самой: иначе она
// закрылась бы ровно в тот момент, когда до неё довели курсор.
func (p *WindowPreview) OnMouseMove(x, y int) {
	if !p.IsOpen() || widget.CursorIsNowhere(x, y) {
		return
	}
	if image.Pt(x, y).In(p.rect()) {
		p.cancelClose()
		return
	}
	p.scheduleClose()
}

// OnMouseButton: клик по миниатюре поднимает окно и закрывает предпросмотр.
func (p *WindowPreview) OnMouseButton(e widget.MouseEvent) bool {
	if !p.IsOpen() || e.Button != widget.MouseLeft {
		return p.Flyout.OnMouseButton(e)
	}
	if !image.Pt(e.X, e.Y).In(p.rect()) {
		return p.Flyout.OnMouseButton(e)
	}
	if !e.Pressed {
		return true
	}

	p.mu.Lock()
	info, ok := p.win, p.hasWin
	p.mu.Unlock()
	if ok && p.wm != nil {
		p.wm.Activate(info.ID)
	}
	p.Close()
	return true
}

// ─── Размер и отрисовка ──────────────────────────────────────────────────────

func (p *WindowPreview) metric(k theme.Key, def float64) int {
	if p.tm == nil {
		return int(def)
	}
	if v := p.tm.GetMetric(k); v > 0 {
		return int(v)
	}
	return int(def)
}

func (p *WindowPreview) delay(k theme.Key, defMs float64) time.Duration {
	return time.Duration(p.metric(k, defMs)) * time.Millisecond
}

// thumbMax — размер, в который вписывается миниатюра.
func (p *WindowPreview) thumbMax() image.Point {
	return image.Pt(
		p.metric(KeyPreviewWidth, previewDefWidth),
		p.metric(KeyPreviewHeight, previewDefHeight),
	)
}

func (p *WindowPreview) size() image.Point {
	max := p.thumbMax()
	pad := p.metric(KeyPreviewPad, previewDefPad)
	header := p.metric(KeyPreviewHeader, previewDefHeader)
	return image.Pt(max.X+2*pad, max.Y+header+2*pad)
}

// draw рисует строку «значок и заголовок» и под ней миниатюру.
func (p *WindowPreview) draw(ctx widget.DrawContext, r image.Rectangle) {
	if r.Empty() {
		return
	}
	p.mu.Lock()
	info, ok := p.win, p.hasWin
	thumb := p.thumb
	p.mu.Unlock()
	if !ok {
		return
	}

	header := p.metric(KeyPreviewHeader, previewDefHeader)
	headRect := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+header)
	thumbRect := image.Rect(r.Min.X, headRect.Max.Y, r.Max.X, r.Max.Y)

	headStyle := styleOf(p.tm, ComponentPreview, previewPartHeader, theme.StateNormal)
	textRect := headRect
	if info.Icon != nil && header > 0 {
		icon := image.Rect(headRect.Min.X, headRect.Min.Y, headRect.Min.X+header, headRect.Max.Y)
		ctx.DrawImageScaled(info.Icon, icon.Min.X, icon.Min.Y, icon.Dx(), icon.Dy())
		textRect.Min.X = icon.Max.X + header/4
	}
	DrawTextLeftElided(ctx, textRect, info.Title, headStyle)

	thumbStyle := styleOf(p.tm, ComponentPreview, previewPartThumb, theme.StateNormal)
	PaintStyle(ctx, thumbRect, thumbStyle)
	if thumb == nil {
		return
	}
	// Миниатюра вписывается по СВОИМ пропорциям, а не растягивается по
	// прямоугольнику: окно может быть любой формы, и растяжка исказила бы
	// содержимое.
	b := thumb.Bounds()
	fit := fitPreserving(b.Dx(), b.Dy(), thumbRect.Dx(), thumbRect.Dy())
	if fit.X <= 0 || fit.Y <= 0 {
		return
	}
	x := thumbRect.Min.X + (thumbRect.Dx()-fit.X)/2
	y := thumbRect.Min.Y + (thumbRect.Dy()-fit.Y)/2
	ctx.DrawImageScaled(thumb, x, y, fit.X, fit.Y)
}

// fitPreserving вписывает размер srcW x srcH в maxW x maxH, сохраняя пропорции.
func fitPreserving(srcW, srcH, maxW, maxH int) image.Point {
	if srcW <= 0 || srcH <= 0 || maxW <= 0 || maxH <= 0 {
		return image.Point{}
	}
	w, h := maxW, srcH*maxW/srcW
	if h > maxH {
		h = maxH
		w = srcW * maxH / srcH
	}
	return image.Pt(w, h)
}
