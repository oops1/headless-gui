package engine

// textcopy.go — текстовые настройки одного движка для другого: вторичные
// движки окна (диалог, плавающая панель в собственном окне ОС) рисуют текст
// теми же шрифтами и тем же слоем качества, что главное окно.

import (
	"slices"

	hgtext "github.com/oops1/headless-gui/v4/core/text"
	"github.com/oops1/headless-gui/v4/widget"
)

// fontOp — шрифт, который приложение отдало движку после создания:
// именованный (RegisterFont, RegisterFontDir, RegisterFontFS) или запасной
// (RegisterFallbackFont). Данные — общие на процесс байты шрифта
// (text.InternedFontData), а не копия приложения: повторные регистрации
// одного шрифта память не множат.
type fontOp struct {
	fallback bool
	name     string
	data     []byte
}

// registerNamedLocked регистрирует именованный шрифт и запоминает его для
// CopyTextSettingsFrom. Под frameMu (или в конструкторе).
func (e *Engine) registerNamedLocked(name string, data []byte) {
	e.canvas.RegisterFont(name, data)
	if e.canvas.HasFont(name) {
		e.recordFontLocked(fontOp{name: name, data: hgtext.InternedFontData(data)})
	}
}

// recordFontLocked дописывает шрифт в журнал. Повторная регистрация имени
// заменяет прежнюю запись, повторный запасной шрифт не записывается: журнал
// приложения, перерегистрирующего шрифты при смене темы, не растёт.
func (e *Engine) recordFontLocked(op fontOp) {
	if len(op.data) == 0 {
		return
	}
	for i, o := range e.fontOps {
		if o.fallback != op.fallback {
			continue
		}
		if (op.fallback && &o.data[0] == &op.data[0]) || (!op.fallback && o.name == op.name) {
			e.fontOps = slices.Delete(e.fontOps, i, i+1)
			break
		}
	}
	e.fontOps = append(e.fontOps, op)
}

// CopyTextSettingsFrom даёт движку текстовые настройки движка src: шрифты,
// которые приложение зарегистрировало у src после его создания (именованные,
// в том числе подменённые встроенные вроде жирного, и запасные) — в том же
// порядке, шрифт по умолчанию (SetDefaultFont), DPI шрифтов (SetDPI), слой
// качества текста (SetTextQuality) и подпиксельное позиционирование, если
// его выбрало приложение (SetTextSubpixel).
//
// Так окно передаёт их вторичным движкам — диалогам и плавающим панелям в
// собственных окнах ОС: без этого диалог рисовал текст шрифтом и слоем по
// умолчанию, что бы ни задало приложение. Шрифты из assets/fonts движок
// находит сам и не копирует. Настройки src, изменённые позже, сами не
// доходят. Вызывать до SetRoot; src не меняется.
func (e *Engine) CopyTextSettingsFrom(src *Engine) {
	if src == nil || src == e {
		return
	}
	src.frameMu.Lock()
	ops := slices.Clone(src.fontOps)
	def := src.defaultFont
	dpi := src.userDPI
	q := src.canvas.DefaultFont().Quality()
	pinned, sub := src.subpixelPinned, src.canvas.DefaultFont().Subpixel()
	src.frameMu.Unlock()

	e.frameMu.Lock()
	for _, op := range ops {
		if op.fallback {
			if e.canvas.AddFallbackFont(op.data) {
				e.recordFontLocked(op)
			}
			continue
		}
		e.registerNamedLocked(op.name, op.data)
	}
	if def != "" && e.canvas.SetDefaultFont(def) {
		e.defaultFont = def
	}
	if dpi != e.userDPI {
		e.userDPI = dpi
		e.canvas.SetDPIAll(dpi * e.canvas.Scale())
	}
	e.canvas.SetTextQuality(q)
	if pinned {
		e.subpixelPinned, e.subpixelOwn = true, false
		e.setSubpixelLocked(sub)
	}
	e.frameMu.Unlock()
	widget.BumpTextMetricsRev() // замеры шрифтами движка устарели
	e.Invalidate()
}
