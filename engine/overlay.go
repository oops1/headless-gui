package engine

// overlay.go — оверлеи движка вне кадра холста (ADR-0011, задача 7.4):
// хостовые попапы с ареной и переходами, хост нативных модальных окон и
// подписчики их закрытия, всплывающую подсказку ведёт core/overlay
// (overlay.Manager, поле e.ov). Здесь — публичные методы Engine, которые
// отдают работу ему (открытый API не меняется), алиасы его типов
// (PopupFrame, PopupTransition, ModalHost), адаптер overlayHost и
// внутренние помощники кадра. Модальные окна в холсте (ShowModal,
// CloseModal) и переходы оверлеев в холсте (overlaytrans.go) — у движка.

import (
	"image"
	"time"

	"github.com/oops1/headless-gui/v3/core/input"
	"github.com/oops1/headless-gui/v3/core/overlay"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// PopupFrame — кадр одного активного оверлея для нативного окна-попапа.
type PopupFrame = overlay.PopupFrame

// PopupTransition — шаг перехода окна-попапа.
type PopupTransition = overlay.PopupTransition

// ModalHost — хост, показывающий модальные виджеты в собственных нативных
// окнах ОС. Реализуется пакетом window для бэкендов с поддержкой owner-окон
// (Фаза 1 — только Win32). Если хост не установлен или отказывается принять
// модалку, движок показывает её в своём холсте — headless-контракт и прежнее
// in-canvas поведение (центрирование, клэмп) сохраняются без изменений.
type ModalHost = overlay.ModalHost

// overlayHost — движок глазами оверлеев (overlay.Host).
type overlayHost struct{ e *Engine }

var _ overlay.Host = overlayHost{}

// TopModal — верхнее модальное окно в холсте.
func (h overlayHost) TopModal() widget.ModalWidget { return h.e.topModal() }

// DamageRect — область изменилась, дерево под ней — нет.
func (h overlayHost) DamageRect(r image.Rectangle) { h.e.damageRect(r) }

// BumpGen — кадр без области (шаг перехода попапа). Без замков.
func (h overlayHost) BumpGen() { h.e.lp.BumpGen() }

// ThemeManager — менеджер тем последнего ApplyThemeProfile. Без замков.
func (h overlayHost) ThemeManager() *theme.Manager { return h.e.themeMgr.Load() }

// TransitionsOn — переходы оверлеев включены (overlaytrans.go). Без замков.
func (h overlayHost) TransitionsOn() bool { return h.e.ovt.on.Load() }

// SlideDir — сдвиг оверлея в начале появления (overlaytrans.go).
func (h overlayHost) SlideDir(w widget.Widget, r image.Rectangle, d int) image.Point {
	return ovtSlideDir(w, r, d)
}

// ── Внутренние помощники кадра поверх оверлеев ─────────────────────────────

// renderPopups отдаёт стоку изменившиеся хостовые попапы (под frameMu).
func (e *Engine) renderPopups(canvas *Canvas, root widget.Widget, modals []widget.ModalWidget, damage image.Rectangle, dirtyAll bool) {
	e.ov.RenderPopups(canvas, root, modals, damage, dirtyAll)
}

// getPopupSink — установленный хост попапов (nil — оверлеи в холсте).
func (e *Engine) getPopupSink() func([]PopupFrame) { return e.ov.PopupSink() }

// getModalHost — хост нативных модальных окон (nil — в холсте).
func (e *Engine) getModalHost() ModalHost { return e.ov.NativeModalHost() }

// fireOnModalClosed — уведомить подписчиков о закрытии модального окна.
func (e *Engine) fireOnModalClosed(m widget.ModalWidget) { e.ov.FireModalClosed(m) }

// drawTooltip рисует подсказку на прямом пути (под frameMu).
func (e *Engine) drawTooltip(c *Canvas, root widget.Widget) { e.ov.DrawTooltip(c, root) }

// tooltipPlan — подсказка кадра на пути команд (toplayers.go).
func (e *Engine) tooltipPlan(c *Canvas, root widget.Widget) overlay.TooltipPlan {
	return e.ov.PlanTooltip(c, root)
}

// drawTooltipBox рисует плашку подсказки в контекст c.
func (e *Engine) drawTooltipBox(c widget.DrawContext, text string, r image.Rectangle) {
	e.ov.DrawTooltipBox(c, text, r)
}

// tooltipMayAppear — подсказка может дозреть к ближайшему кадру.
func (e *Engine) tooltipMayAppear(frameInterval time.Duration) bool {
	return e.ov.TooltipMayAppear(frameInterval)
}

// recordMouse — курсор в (x, y): отсчёт задержки подсказки.
func (e *Engine) recordMouse(x, y int) { e.ov.RecordMouse(x, y) }

// invalidateShownTooltip — стереть показанную подсказку.
func (e *Engine) invalidateShownTooltip() { e.ov.InvalidateShownTooltip() }

// CloseAllOverlays закрывает все Dismissable-оверлеи (dropdown/меню) в дереве и
// в модалках. Используется хостом при деактивации окна-носителя (клик в другое
// приложение): системные меню в этом случае закрываются.
func (e *Engine) CloseAllOverlays() {
	e.mu.RLock()
	root := e.root
	e.mu.RUnlock()
	if root != nil {
		// keep=nil и точка вне любого экрана: закрыть ВСЕ, включая панели,
		// которые считают своей площадь соседки по группе.
		input.DismissOutside(root, nil, widget.CursorNowhere, widget.CursorNowhere)
	}
	e.modMu.Lock()
	modals := make([]widget.ModalWidget, len(e.modals))
	copy(modals, e.modals)
	e.modMu.Unlock()
	for _, m := range modals {
		input.DismissOutside(m, nil, widget.CursorNowhere, widget.CursorNowhere)
	}
}

// SetPopupFade сообщает, что хост попапов понимает PopupFrame.Transition:
// держит окно исчезающего оверлея, пока его ID в кадрах, и прикладывает
// непрозрачность и сдвиг. По умолчанию — нет: хостовые попапы появляются и
// исчезают сразу, как прежде. Переходы включаются ещё и
// SetOverlayTransitions — общим выключателем всех оверлеев.
func (e *Engine) SetPopupFade(on bool) {
	e.ov.SetPopupFade(on)
}

// PopupFade сообщает, объявил ли хост попапов переходы.
func (e *Engine) PopupFade() bool {
	return e.ov.PopupFade()
}

// SetPopupSink регистрирует хост popup-оверлеев. sink вызывается из рендер-цикла
// со всеми активными оверлеями текущего кадра (пустой slice — закрыть все окна).
// nil снимает хост (оверлеи снова рисуются в холст).
//
// Дополнительно выставляет глобальный флаг widget.SetPopupsHosted — при нём
// popup-меню не клэмпятся в границы канваса (экранным позиционированием
// занимается хост).
func (e *Engine) SetPopupSink(sink func(frames []PopupFrame)) {
	e.ov.SetPopupSink(sink)
}

// SetModalHost устанавливает хост нативных модалок (nil — снять).
// Обычно вызывается пакетом window в Window.Run() при поддержке бэкенда.
func (e *Engine) SetModalHost(h ModalHost) {
	e.ov.SetModalHost(h)
}

// SetOnModalClosed регистрирует колбэк, вызываемый в конце CloseModal после
// фактического закрытия модалки ЭТИМ движком (т.е. когда она была в стеке
// движка, а не у хоста). Используется хостом на вторичном движке: диалог,
// закрытый своим closer'ом (✕/Escape) → secondEng.CloseModal → колбэк →
// хост уничтожает нативное окно.
func (e *Engine) SetOnModalClosed(fn func(m widget.ModalWidget)) {
	e.ov.SetOnModalClosed(fn)
}

// AddOnModalClosed подписывает колбэк на закрытие модалки и возвращает
// дескриптор для RemoveOnModalClosed.
//
// SetOnModalClosed хранит ОДИН колбэк, и он занят хостом нативных окон:
// приложение, подписавшееся им, вытесняло хост или вытеснялось само (GG-82).
// Подписок может быть сколько угодно; зовутся они в порядке подписки, после
// колбэка SetOnModalClosed.
//
// Колбэк приходит и для диалога, показанного в собственном окне ОС, — в том
// числе когда его закрыли крестиком окна или Alt+F4.
func (e *Engine) AddOnModalClosed(fn func(m widget.ModalWidget)) int {
	return e.ov.AddOnModalClosed(fn)
}

// RemoveOnModalClosed снимает подписку по дескриптору (no-op, если нет).
func (e *Engine) RemoveOnModalClosed(id int) {
	e.ov.RemoveOnModalClosed(id)
}

// NotifyModalClosed сообщает подписчикам движка, что модалка закрыта.
//
// Зовётся хостом нативных окон: диалог в собственном окне ОС закрывается мимо
// CloseModal этого движка — его закрывает вторичный движок окна, — и без этого
// приложение не узнавало о закрытии крестиком или Alt+F4 (GG-82).
func (e *Engine) NotifyModalClosed(m widget.ModalWidget) {
	e.ov.NotifyModalClosed(m)
}

// SetTooltipsEnabled включает/выключает показ всплывающих подсказок.
func (e *Engine) SetTooltipsEnabled(v bool) {
	e.ov.SetTooltipsEnabled(v)
}

// TooltipsEnabled сообщает, включён ли показ подсказок.
func (e *Engine) TooltipsEnabled() bool {
	return e.ov.TooltipsEnabled()
}

// SetTooltipDelay задаёт задержку появления подсказки после остановки курсора.
func (e *Engine) SetTooltipDelay(d time.Duration) {
	e.ov.SetTooltipDelay(d)
}
