package widget

import (
	"image"
	"image/color"
	"math"
	"sync/atomic"
	"time"
)

// ─── Modal-интерфейс ────────────────────────────────────────────────────────

// ModalWidget — виджет, который блокирует весь ввод под собой.
// Движок проверяет наличие модальных виджетов перед dispatch'ем событий.
type ModalWidget interface {
	Widget
	// IsModal возвращает true, пока модальный виджет активен.
	IsModal() bool
	// DimColor возвращает цвет затемнения фона (обычно полупрозрачный чёрный).
	DimColor() color.RGBA
}

// ─── Dialog ─────────────────────────────────────────────────────────────────

// Геометрия современного диалога (не-Classic3D темы).
const (
	dlgCorner    = 8  // радиус скругления корпуса
	dlgTitleH    = 34 // высота заголовка
	dlgPad       = 14 // горизонтальный отступ контента
	dlgShadowW   = 5  // ширина мягкой тени справа/снизу
	dlgCloseSize = 24 // зона кнопки ✕

	dlgFadeDur = 140 * time.Millisecond // длительность fade-in затемнения
)

// Dialog — модальный диалог в стиле активной темы.
//
// Отрисовывается поверх всех виджетов с затемнением фона.
// Весь ввод ограничен содержимым диалога — клики вне него игнорируются.
//
// Использование:
//
//	dlg := widget.NewDialog("Подтверждение", 400, 200)
//	dlg.AddChild(label)
//	dlg.AddChild(okBtn)
//	eng.ShowModal(dlg)
//	// ...
//	eng.CloseModal(dlg)
type Dialog struct {
	Base

	Title       string
	Background  color.RGBA
	BorderColor color.RGBA
	TitleColor  color.RGBA
	TitleBG     color.RGBA
	Dim         color.RGBA // затемнение фона
	Shadow      color.RGBA // тень под диалогом (A=0 — без тени)
	TitleHeight int
	CornerRadius int // радиус скругления корпуса (по умолчанию dlgCorner)

	// ShowLocaleIndicator — показывать индикатор текущей локали (напр. «EN»)
	// в заголовке диалога. По умолчанию выключен (принятый дизайн — ✕).
	ShowLocaleIndicator bool

	// ShowCloseButton — рисовать кнопку ✕ в заголовке (по умолчанию true).
	// Клик по ✕ эквивалентен Escape: CancelAction + закрытие модалки.
	ShowCloseButton bool

	// DefaultAction вызывается по Enter (кнопка по умолчанию диалога).
	DefaultAction func()
	// CancelAction вызывается по Escape/✕ в дополнение к закрытию (может быть
	// nil; само закрытие модалки выполняет движок).
	CancelAction func()
	// CopyText, если задан, вызывается по Ctrl+C и его результат кладётся
	// в буфер обмена (MessageBox формирует Windows-подобный дамп).
	CopyText func() string

	modal      atomic.Bool // управляется движком: true пока диалог показан;
	//                        читается рендер-циклом (IsModal), пишется из
	//                        ShowModal/CloseModal — доступ атомарный.
	closer     func() // закрытие модалки движком (устанавливает ShowModal)
	closeBtn   *dialogCloseBtn
	localeSubs []int // id подписчиков на смену языка (снимаются при закрытии)

	// fading — true, только когда тик fade-анимации РЕАЛЬНО вызвался хотя бы
	// раз (взводится изнутри тика — часы анимации ленивые, см. anim.go).
	// Пока false, DimColor() возвращает каноническую Dim напрямую — так
	// показ диалога без работающего движка (тесты, мгновенный ShowModal)
	// рисуется с целевым затемнением сразу, без "провала в прозрачность".
	fading atomic.Bool
	// dimAlpha — текущая (анимируемая) альфа затемнения, 0..255 в uint32.
	dimAlpha atomic.Uint32
	// fadeAnim — хендл текущей fade-анимации (для явного Stop при закрытии
	// диалога посреди fade-in; поле трогается только из горутины SetModal,
	// поэтому без мьютекса — SetModal и так не конкурентен сам с собой по
	// контракту движка: показывает/закрывает один и тот же диалог не более
	// одного раза одновременно).
	fadeAnim *Animation

	// ── Перетаскивание за заголовок (как у Window/Panel) ────────────────────
	dragging   bool
	dragLastX  int
	dragLastY  int
	capMgr     CaptureManager

	// OnDragMove, если задан, вызывается при перетаскивании за заголовок ВМЕСТО
	// сдвига виджета по холсту (по образцу Window.OnDragMove). Используется,
	// когда диалог показан в собственном нативном окне (window.dialogHost):
	// сам виджет в своём холсте неподвижен, двигается нативное окно ОС.
	OnDragMove func(dx, dy int)
}

// OnLanguageChange регистрирует применение перевода при смене языка
// интерфейса, пока диалог открыт. apply вызывается сразу (для текущего
// языка) и далее при каждом SetLanguage; подписка снимается в SetModal(false).
func (d *Dialog) OnLanguageChange(apply func()) {
	apply()
	id := AddLanguageListener(func(string) {
		apply()
		d.Invalidate()
	})
	d.localeSubs = append(d.localeSubs, id)
}

// OnCancel вызывается движком при закрытии диалога по Escape (и кнопкой ✕
// через RequestClose) — сообщает результат отмены.
func (d *Dialog) OnCancel() {
	if d.CancelAction != nil {
		d.CancelAction()
	}
}

// SetCloser задаёт функцию закрытия модалки. Вызывается движком в ShowModal;
// функция должна выполнить OnCancel-семантику и CloseModal.
func (d *Dialog) SetCloser(close func()) { d.closer = close }

// RequestClose закрывает диалог путём отмены (кнопка ✕): как Escape.
func (d *Dialog) RequestClose() {
	if d.closer != nil {
		d.closer()
	}
}

// HandleInputBinding обрабатывает клавиши диалога до фокус-диспатча:
// Enter → DefaultAction, Ctrl+C → копирование содержимого (см. CopyText).
// Движок вызывает его у верхнего модального виджета (см. SendKeyEvent).
func (d *Dialog) HandleInputBinding(code KeyCode, mod KeyMod) bool {
	switch {
	case code == KeyEnter && mod == 0 && d.DefaultAction != nil:
		d.DefaultAction()
		return true
	case code == KeyC && mod&ModCtrl != 0 && d.CopyText != nil:
		ClipboardSetText(d.CopyText())
		return true
	}
	return false
}

// NewDialog создаёт модальный диалог заданного размера.
// Диалог центрируется на экране при показе через Engine.ShowModal.
// Цвета берутся из активной темы (глобальная палитра win10).
func NewDialog(title string, width, height int) *Dialog {
	d := &Dialog{
		Title:           title,
		Background:      win10.DialogBG,
		BorderColor:     win10.Border,
		TitleColor:      win10.TitleText,
		TitleBG:         win10.DialogTitleBG,
		Dim:             win10.DialogDim,
		Shadow:          win10.ShadowColor,
		TitleHeight:     dlgTitleH,
		CornerRadius:    dlgCorner,
		ShowCloseButton: true,
		Base: Base{
			bounds: image.Rect(0, 0, width, height),
		},
	}
	d.modal.Store(true)
	d.closeBtn = &dialogCloseBtn{owner: d}
	d.closeBtn.SetBounds(image.Rect(
		width-dlgCloseSize-6, (dlgTitleH-dlgCloseSize)/2,
		width-6, (dlgTitleH-dlgCloseSize)/2+dlgCloseSize))
	d.AddChild(d.closeBtn)
	// dimAlpha изначально равна канонической альфе Dim — до первого показа
	// через ShowModal DimColor() и так возвращает d.Dim напрямую (fading
	// ещё false), это лишь безопасное стартовое значение поля.
	d.dimAlpha.Store(uint32(d.Dim.A))
	return d
}

// IsModal реализует ModalWidget.
func (d *Dialog) IsModal() bool { return d.modal.Load() }

// DimColor реализует ModalWidget. Пока идёт fade-in (см. SetModal), отдаёт
// промежуточную альфу; иначе — каноническое значение Dim напрямую, поэтому
// конечное состояние без анимации (или после её завершения) совпадает с тем,
// что было бы без анимационного фреймворка вообще.
func (d *Dialog) DimColor() color.RGBA {
	if d.fading.Load() {
		a := uint8(d.dimAlpha.Load())
		return color.RGBA{R: d.Dim.R, G: d.Dim.G, B: d.Dim.B, A: a}
	}
	return d.Dim
}

// setDimAlpha — публичный потокобезопасный сеттер текущей альфы затемнения;
// вызывается ТОЛЬКО из тика fade-анимации (см. SetModal). Инвалидирует ВЕСЬ
// UI (notifyUIChanged), а не только bounds диалога: затемнение рисуется
// движком на весь экран, и точечная инвалидация d.Invalidate() оставила бы
// dim ВНЕ прямоугольника диалога замороженным на альфе первого кадра —
// частичная перерисовка клипует FillRectAlpha по damage-области.
func (d *Dialog) setDimAlpha(a uint8) {
	d.dimAlpha.Store(uint32(a))
	notifyUIChanged()
}

// SetModal управляет модальным состоянием (вызывается движком).
// true (показ через ShowModal) — запускает fade-in затемнения (кроме
// Classic3D, где анимации отключены целиком — мгновенная целевая альфа).
// false (закрытие) — останавливает fade-анимацию (если ещё шла) и снимает
// подписки на смену языка.
func (d *Dialog) SetModal(v bool) {
	d.modal.Store(v)
	if v {
		if currentStyle().Classic3D {
			// Классика Win2000: анимации отключены целиком — сразу целевая альфа.
			d.fading.Store(false)
			d.dimAlpha.Store(uint32(d.Dim.A))
		} else {
			target := d.Dim.A
			d.dimAlpha.Store(0)
			// fading взводится ВНУТРИ тика, а не здесь: часы анимации ленивые
			// (см. widget/anim.go — старт на первом StepAnimations), поэтому
			// между регистрацией и первым реальным тиком DimColor() должен
			// по-прежнему отдавать канонический Dim, а не "ещё не сдвинутую"
			// dimAlpha=0 — иначе показ диалога без работающего движка
			// (тесты, мгновенный ShowModal) рисовался бы без затемнения.
			//
			// AnimateOwned(d,"fade",...) сам останавливает предыдущую
			// fade-анимацию этого диалога, если ShowModal вызвали повторно
			// до завершения предыдущего fade-in.
			d.fadeAnim = AnimateOwned(d, "fade", dlgFadeDur, EaseOutCubic, func(t float64) {
				d.fading.Store(true)
				d.setDimAlpha(uint8(math.Round(LerpF(0, float64(target), t))))
				if t >= 1.0 {
					d.fading.Store(false)
				}
			})
		}
		return
	}
	// Диалог закрывается: явно останавливаем ещё бегущий fade-in — Stop
	// идемпотентен и безопасен, даже если анимация уже сама завершилась.
	// Без этого тик мог бы продолжить дёргать setDimAlpha/Invalidate уже
	// закрытого диалога до конца длительности — не паника (виджет валиден),
	// но лишняя работа, которую чище оборвать сразу.
	if d.fadeAnim != nil {
		d.fadeAnim.Stop()
		d.fadeAnim = nil
	}
	d.fading.Store(false)
	for _, id := range d.localeSubs {
		RemoveLanguageListener(id)
	}
	d.localeSubs = nil
}

// ContentBounds возвращает прямоугольник для размещения дочерних виджетов
// (под заголовком, с отступами).
func (d *Dialog) ContentBounds() image.Rectangle {
	b := d.bounds
	return image.Rect(
		b.Min.X+dlgPad,
		b.Min.Y+d.TitleHeight+12,
		b.Max.X-dlgPad,
		b.Max.Y-12,
	)
}

// Draw рисует диалог (без затемнения — затемнение рисует движок).
func (d *Dialog) Draw(ctx DrawContext) {
	b := d.bounds
	st := currentStyle()
	// Страховка на случай прямой записи в ShowCloseButton (см.
	// syncCloseButtonVisible) — БЕЗ Invalidate: мы и так внутри Draw,
	// звать перерисовку из середины перерисовки незачем и небезопасно
	// (это ровно то, из-за чего исходно ловили баг с пропуском Draw).
	d.syncCloseButtonVisible()

	if st.Classic3D {
		// Классика Win2000: квадрат, градиентный заголовок, рамка.
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), d.Background)
		if d.TitleHeight > 0 {
			fillTitleBar(ctx, image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+d.TitleHeight), d.TitleBG)
			textY := b.Min.Y + (d.TitleHeight-13)/2
			drawTitleText(ctx, d.Title, b.Min.X+10, textY, d.TitleColor)
			if d.ShowLocaleIndicator {
				drawLocaleBadge(ctx, b.Max.X-8, b.Min.Y, d.TitleHeight, d.TitleColor)
			}
			ctx.DrawHLine(b.Min.X, b.Min.Y+d.TitleHeight, b.Dx(), d.BorderColor)
		}
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), d.BorderColor)
		d.drawChildren(ctx)
		return
	}

	// Современный вид (принятый дизайн): скругление, мягкая тень, ✕.
	cr := d.CornerRadius
	if cr < 0 {
		cr = 0
	}

	// Тень: полосы с честным альфа-смешиванием справа и снизу.
	if d.Shadow.A > 0 {
		sc := d.Shadow
		half := color.RGBA{R: sc.R, G: sc.G, B: sc.B, A: sc.A / 2}
		ctx.FillRectAlpha(b.Min.X+cr, b.Max.Y, b.Dx()-cr+3, 3, sc)
		ctx.FillRectAlpha(b.Min.X+cr+2, b.Max.Y+3, b.Dx()-cr, 2, half)
		ctx.FillRectAlpha(b.Max.X, b.Min.Y+cr, 3, b.Dy()-cr, sc)
		ctx.FillRectAlpha(b.Max.X+3, b.Min.Y+cr+2, 2, b.Dy()-cr-2, half)
	}

	// Корпус и заголовок.
	ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, d.Background)
	if d.TitleHeight > 0 {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), d.TitleHeight, cr, d.TitleBG)
		ctx.FillRect(b.Min.X, b.Min.Y+d.TitleHeight-cr, b.Dx(), cr, d.TitleBG)
		textY := b.Min.Y + (d.TitleHeight-14)/2
		ctx.DrawTextFont(d.Title, b.Min.X+dlgPad, textY, 11, BuiltinFontBold, d.TitleColor)
		if d.ShowLocaleIndicator {
			drawLocaleBadge(ctx, b.Max.X-dlgCloseSize-12, b.Min.Y, d.TitleHeight, d.TitleColor)
		}
	}
	ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, d.BorderColor)

	d.drawChildren(ctx)
}

// SetShowCloseButton — предпочтительный способ поменять ShowCloseButton в
// рантайме: сразу синхронизирует видимость кнопки ✕ (и, если она реально
// изменилась, зовёт Invalidate — здесь это уместно, это настоящее внешнее
// изменение состояния, а не побочный эффект отрисовки). Поле ShowCloseButton
// остаётся публичным для обратной совместимости: тем, кто пишет в него
// напрямую, видимость всё равно досинхронизируют Draw и Children() —
// см. syncCloseButtonVisible.
func (d *Dialog) SetShowCloseButton(v bool) {
	d.ShowCloseButton = v
	d.closeBtn.SetVisible(v)
}

// syncCloseButtonVisible приводит видимость кнопки ✕ в соответствие текущему
// значению ShowCloseButton. Вызывается из Draw и Children() как страховка —
// НАПРЯМУЮ трогает hidden, а не через SetVisible, поэтому не дёргает
// Invalidate: это идемпотентная подстройка состояния, а не решение о
// перерисовке (которое отдельно принимает SetShowCloseButton).
func (d *Dialog) syncCloseButtonVisible() {
	d.closeBtn.hidden = !d.ShowCloseButton
}

// Children переопределяет Base.Children(): движок ходит по нему для
// hit-теста и доставки событий мыши/клавиатуры — а с пропуском отрисовки
// невидимых поддеревьев (SkipSubtree) это может случиться и БЕЗ
// предшествующего вызова Draw. Досинхронизируем видимость кнопки ✕ здесь
// же, а не только в Draw, — иначе клик по «скрытой» ✕ (или мимо видимой)
// мог бы использовать состояние старого кадра.
func (d *Dialog) Children() []Widget {
	d.syncCloseButtonVisible()
	return d.Base.Children()
}

// ApplyTheme обновляет цвета Dialog.
func (d *Dialog) ApplyTheme(t *Theme) {
	d.Background = t.DialogBG
	d.TitleBG = t.DialogTitleBG
	d.TitleColor = t.TitleText
	d.BorderColor = t.Border
	d.Dim = t.DialogDim
	d.Shadow = t.ShadowColor
}

// ─── Перетаскивание за заголовок ─────────────────────────────────────────────

// SetCaptureManager инжектит менеджер захвата мыши (движок вызывает при
// ShowModal через injectCaptureManager).
func (d *Dialog) SetCaptureManager(cm CaptureManager) { d.capMgr = cm }

// titleDragHit — точка в перетаскиваемой зоне заголовка: внутри полосы
// титлбара И не над каким-либо дочерним виджетом (кнопка ✕, а также любой
// контент, размещённый приложением в верхней полосе, сохраняют свои клики).
func (d *Dialog) titleDragHit(x, y int) bool {
	b := d.bounds
	pt := image.Pt(x, y)
	if !pt.In(image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+d.TitleHeight)) {
		return false
	}
	for _, c := range d.children {
		if IsWidgetVisible(c) && pt.In(c.Bounds()) {
			return false
		}
	}
	return true
}

// WantsCapture — захватываем мышь при нажатии на заголовок (drag).
func (d *Dialog) WantsCapture(e MouseEvent) bool {
	return e.Button == MouseLeft && e.Pressed && d.titleDragHit(e.X, e.Y)
}

// OnMouseButton начинает/заканчивает перетаскивание за заголовок.
func (d *Dialog) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	if !e.Pressed {
		if d.dragging {
			d.dragging = false
			if d.capMgr != nil {
				d.capMgr.ReleaseCapture()
			}
			return true
		}
		return false
	}
	if d.titleDragHit(e.X, e.Y) {
		DismissAll(d) // закрываем dropdown/popup внутри диалога перед drag
		d.dragging = true
		d.dragLastX = e.X
		d.dragLastY = e.Y
		return true
	}
	return false
}

// OnMouseMove перемещает диалог вместе с детьми, пока идёт drag.
// Тень рисуется ЗА пределами bounds, а точечная инвалидация клипует по
// damage — поэтому на каждый шаг инвалидируем весь кадр (drag редок).
func (d *Dialog) OnMouseMove(x, y int) {
	if !d.dragging {
		return
	}
	dx, dy := x-d.dragLastX, y-d.dragLastY
	if dx == 0 && dy == 0 {
		return
	}
	if d.OnDragMove != nil {
		// Нативный режим: движется само окно ОС, диалог в своём холсте
		// неподвижен. dragLast НЕ обновляем — координаты мыши относительны
		// окну, и после его сдвига курсор возвращается к точке захвата
		// (как в Window.OnMouseMove). Обновление dragLast дало бы осцилляцию.
		d.OnDragMove(dx, dy)
		return
	}
	d.dragLastX, d.dragLastY = x, y
	ShiftWidget(d, dx, dy)
	notifyUIChanged()
}

// ─── Кнопка ✕ в заголовке ───────────────────────────────────────────────────

// dialogCloseBtn — кнопка закрытия в заголовке диалога. Клик — отмена
// (эквивалент Escape). В классике — выпуклая bevel-кнопка.
type dialogCloseBtn struct {
	Base
	owner *Dialog
	hover bool
	// armed — кнопка «взведена» нажатием. Закрытие срабатывает на ОТПУСКАНИИ
	// и только если курсор всё ещё над кнопкой: нажать, увести мышь и
	// отпустить — значит передумать (семантика кнопок Windows, та же, что у
	// кнопок заголовка окна, см. Window.armedBtn).
	armed  bool
	capMgr CaptureManager
}

func (cb *dialogCloseBtn) Draw(ctx DrawContext) {
	b := cb.bounds
	if b.Empty() {
		return
	}
	st := currentStyle()
	fg := win10.TitleText
	if st.Classic3D {
		// Классика: маленькая выпуклая кнопка «лица» с чёрным ✕.
		ctx.FillRect(b.Min.X, b.Min.Y+2, b.Dx()-2, b.Dy()-4, win10.BtnBG)
		drawBevelRaised(ctx, b.Min.X, b.Min.Y+2, b.Dx()-2, b.Dy()-4, st)
		fg = win10.BtnText
		tw := ctx.MeasureText("x", 10)
		ctx.DrawTextFont("x", b.Min.X+(b.Dx()-2-tw)/2, b.Min.Y+(b.Dy()-13)/2, 10, BuiltinFontBold, fg)
		return
	}
	if cb.hover {
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), 4, win10.BtnHoverBG)
	} else {
		fg = win10.InputPlaceholder // ненавязчивый серый ✕ (как в мокапе)
	}
	tw := ctx.MeasureText("✕", 11)
	ctx.DrawTextSize("✕", b.Min.X+(b.Dx()-tw)/2, b.Min.Y+(b.Dy()-14)/2, 11, fg)
}

func (cb *dialogCloseBtn) OnMouseMove(x, y int) {
	h := image.Pt(x, y).In(cb.bounds)
	if h != cb.hover {
		cb.hover = h
		cb.Invalidate()
	}
}

// SetCaptureManager инжектится движком: захват нужен, чтобы отпускание
// пришло кнопке даже если курсор ушёл с неё.
func (cb *dialogCloseBtn) SetCaptureManager(cm CaptureManager) { cb.capMgr = cm }

// WantsCapture — захватываем мышь на нажатии ради release-семантики.
func (cb *dialogCloseBtn) WantsCapture(e MouseEvent) bool {
	return e.Button == MouseLeft && e.Pressed && image.Pt(e.X, e.Y).In(cb.bounds)
}

func (cb *dialogCloseBtn) OnMouseButton(e MouseEvent) bool {
	if e.Button != MouseLeft {
		return false
	}
	over := image.Pt(e.X, e.Y).In(cb.bounds)

	if e.Pressed {
		if !over {
			return false
		}
		cb.armed = true
		return true
	}

	// Отпускание: закрываем, только если кнопка была взведена и курсор над ней.
	if !cb.armed {
		return false
	}
	cb.armed = false
	if cb.capMgr != nil {
		cb.capMgr.ReleaseCapture()
	}
	if over {
		cb.owner.RequestClose()
	}
	return true
}

// ─── Хелперы для быстрого создания диалогов ─────────────────────────────────

// NewConfirmDialog создаёт диалог подтверждения с кнопками «OK» и «Отмена».
// Кнопки позиционируются автоматически. onResult(true) — OK, onResult(false) — Отмена.
func NewConfirmDialog(title, message string, onResult func(ok bool)) *Dialog {
	const (
		dlgW = 400
		dlgH = 160
	)
	dlg := NewDialog(title, dlgW, dlgH)

	// Все координаты дочерних виджетов — относительно (0,0) диалога.
	// ShowModal сдвинет их при центрировании.
	lbl := NewLabel(message, win10.LabelText)
	lbl.SetBounds(image.Rect(16, dlg.TitleHeight+12, dlgW-16, dlg.TitleHeight+52))

	okBtn := NewWin10AccentButton("  OK  ")
	okBtn.SetBounds(image.Rect(dlgW-200, dlgH-48, dlgW-110, dlgH-12))

	cancelBtn := NewButton("  Отмена  ")
	cancelBtn.SetBounds(image.Rect(dlgW-100, dlgH-48, dlgW-10, dlgH-12))

	okBtn.OnClick = func() {
		if onResult != nil {
			onResult(true)
		}
	}
	cancelBtn.OnClick = func() {
		if onResult != nil {
			onResult(false)
		}
	}

	dlg.AddChild(lbl)
	dlg.AddChild(okBtn)
	dlg.AddChild(cancelBtn)

	return dlg
}
