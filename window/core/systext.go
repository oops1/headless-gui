package core

// systext.go — текст «как в системе»: слой качества текста движка по
// настройке сглаживания ОС (ClearType) и удалённому сеансу.
//
// Субпиксельный слой (TextQuality.LCD) по цифрам совпадает с ClearType, но
// движок сам его не включает: он хорош только на мониторе с известным
// порядком подпикселей и вреден в удалённом сеансе, где кадр сжимают и
// показывают на чужом мониторе. Знает это ОС — окно спрашивает её.

import (
	"sync"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/platform/native"
)

// textRenderingSource — бэкенд, который знает настройку сглаживания текста
// ОС и сообщает о её смене (Win32). Без интерфейса настройка неизвестна.
type textRenderingSource interface {
	TextRendering() native.TextRendering
	SetOnTextRenderingChanged(fn func())
}

// textQualitySink — движок со слоем качества текста (реализует
// *engine.Engine).
type textQualitySink interface {
	SetTextQuality(q engine.TextQuality)
	TextQuality() engine.TextQuality
}

// TextQualityFor — слой качества текста движка для настройки сглаживания ОС:
//
//   - ClearType на своём мониторе — субпиксельный слой с привязкой к сетке по
//     обеим осям и порядком подпикселей монитора:
//     TextQuality{Hinting, HintingX, LCD, LCDOrder};
//   - серое сглаживание, сглаживание выключено, удалённый сеанс (RDP) или
//     настройка неизвестна — слой движка по умолчанию,
//     engine.DefaultTextQuality() (серый).
//
// Выключенное в ОС сглаживание движок не повторяет: текст без сглаживания
// он не рисует.
func TextQualityFor(tr native.TextRendering) engine.TextQuality {
	if !tr.Known || !tr.Subpixel || tr.Remote {
		return engine.DefaultTextQuality()
	}
	q := engine.TextQuality{Hinting: true, HintingX: true, LCD: true, LCDOrder: engine.LCDOrderRGB}
	if tr.BGR {
		q.LCDOrder = engine.LCDOrderBGR
	}
	return q
}

// systemTextState — режим «текст как в системе» окна.
type systemTextState struct {
	mu sync.Mutex
	on bool
	// applied — слой, который режим поставил последним (have — ставил):
	// повторное уведомление без перемены движок не трогает.
	applied engine.TextQuality
	have    bool
}

// SetTextQualityFromSystem включает режим «текст как в системе»: слой
// качества текста движка выбирается по настройке сглаживания ОС
// (TextQualityFor) и меняется вместе с ней — когда человек включает или
// выключает ClearType, меняет порядок подпикселей в «Настройке текста
// ClearType», заходит в этот сеанс по RDP или возвращается за свой монитор.
//
// Слой ставится движку окна и вторичным движкам его диалогов и плавающих
// панелей в собственных окнах ОС. Можно вызывать до Run (слой встанет до
// первого кадра) и во время работы. Выключение (false) снимает наблюдение и
// оставляет слой как есть; явный SetTextQuality движка после включения
// действует до следующей перемены настройки ОС.
//
// Поддержано на Windows (SystemParametersInfo SPI_GETFONTSMOOTHING*,
// GetSystemMetrics SM_REMOTESESSION; уведомления WM_SETTINGCHANGE и
// WM_WTSSESSION_CHANGE). На Linux, macOS и в window/minimal настройка
// неизвестна — ставится engine.DefaultTextQuality().
func (win *Window) SetTextQualityFromSystem(on bool) {
	win.sysText.mu.Lock()
	win.sysText.on = on
	win.sysText.have = false
	running := win.native != nil
	win.sysText.mu.Unlock()
	if running {
		win.syncSystemText()
	}
}

// SystemTextRendering — настройка сглаживания текста ОС для этого окна
// сейчас. До Run и на бэкенде, который её не знает, Known=false.
func (win *Window) SystemTextRendering() native.TextRendering {
	if src, ok := win.native.(textRenderingSource); ok {
		return src.TextRendering()
	}
	return native.TextRendering{}
}

// syncSystemText приводит подписку бэкенда к режиму и, если он включён,
// ставит слой сразу. Из bringUp — до первого кадра — и из
// SetTextQualityFromSystem.
func (win *Window) syncSystemText() {
	win.sysText.mu.Lock()
	on := win.sysText.on
	win.sysText.mu.Unlock()
	if src, ok := win.native.(textRenderingSource); ok {
		if on {
			src.SetOnTextRenderingChanged(func() { win.post(win.applySystemText) })
		} else {
			src.SetOnTextRenderingChanged(nil)
		}
	}
	if on {
		win.applySystemText()
	}
}

// stopSystemText снимает подписку бэкенда при закрытии окна.
func (win *Window) stopSystemText() {
	if src, ok := win.native.(textRenderingSource); ok {
		src.SetOnTextRenderingChanged(nil)
	}
}

// applySystemText читает настройку ОС и ставит слой движку окна и вторичным
// движкам, если он изменился.
func (win *Window) applySystemText() {
	tr := win.SystemTextRendering()
	q := TextQualityFor(tr)
	st := &win.sysText
	st.mu.Lock()
	if !st.on || (st.have && st.applied == q) {
		st.mu.Unlock()
		return
	}
	st.applied, st.have = q, true
	st.mu.Unlock()
	if s, ok := win.eng.(textQualitySink); ok {
		s.SetTextQuality(q)
	}
	for _, e := range win.hostedEngines() {
		e.SetTextQuality(q)
	}
}

// hostedEngines — вторичные движки окна: диалоги и плавающие панели в
// собственных окнах ОС.
func (win *Window) hostedEngines() []*engine.Engine {
	var out []*engine.Engine
	if h := win.modalHost; h != nil {
		h.mu.Lock()
		for _, m := range h.stack {
			if m.eng != nil {
				out = append(out, m.eng)
			}
		}
		h.mu.Unlock()
	}
	if h := win.dockHost; h != nil {
		h.mu.Lock()
		for _, f := range h.floats {
			if f.eng != nil {
				out = append(out, f.eng)
			}
		}
		h.mu.Unlock()
	}
	return out
}

// inheritText даёт вторичному движку (диалог, плавающая панель) текстовые
// настройки движка родительского окна: шрифты приложения, шрифт по
// умолчанию, DPI шрифтов, слой качества (engine.CopyTextSettingsFrom). Явный
// SetTextQuality приложения, режим «как в системе» и свой шрифт доходят и до
// окон диалогов. Движок окна — не *engine.Engine: только слой качества, если
// он его знает.
func inheritText(eng *engine.Engine, parent *Window) {
	if parent == nil {
		return
	}
	switch pe := parent.eng.(type) {
	case *engine.Engine:
		eng.CopyTextSettingsFrom(pe)
	case textQualitySink:
		eng.SetTextQuality(pe.TextQuality())
	}
}
