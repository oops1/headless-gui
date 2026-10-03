package widget

// richtext_edit.go — RichText как редактор: ввод, удаление, отмена, оформление
// выделения, буфер обмена, выгрузка и загрузка HTML, контекстное меню.
//
// Принцип. Каждая правка — один проход «замок → правка документа → сверка
// производных → замок снят → перерисовка и OnChange» (edit). Так нигде не
// забывается пересобрать doc и сдвинуть прокрутку к каретке, а чужой код
// (OnChange) никогда не вызывается под замком: обработчик вправе звать
// методы виджета, и под замком это была бы тупиковая блокировка.
//
// «Изменилось ли содержимое» определяется по Revision документа, а не по
// возвращаемому значению каждой правки: Undo, ApplyStyle без видимой разницы,
// удаление пустого диапазона сами решают, меняют ли они документ, и виджету
// не нужно дублировать это знание, рискуя разойтись.

import (
	"image"
	"strings"
)

// richTabSpaces — сколько пробелов вставляет Tab при AcceptTab.
const richTabSpaces = 4

// ─── Общий каркас правки ────────────────────────────────────────────────────

// edit выполняет fn под замком как правку. false — виджет не редактор и fn не
// вызывалась; иначе — менялось ли содержимое. Незавершённая композиция IME
// перед любой другой правкой принимается как текст: её запись в истории —
// «верхняя», и правка поверх неё сломала бы откат композиции.
func (t *RichText) edit(fn func()) bool {
	t.mu.Lock()
	if !t.Editable {
		t.mu.Unlock()
		return false
	}
	t.layoutLocked()
	rev0 := t.rdoc.Revision()
	scroll0, anchor0, caret0, eol0 := t.scrollY, t.selAnchor, t.selCaret, t.caretEOL
	t.imeFinishLocked()
	fn()
	changed := t.rdoc.Revision() != rev0
	if changed {
		t.contentChangedLocked()
	}
	visual := changed || t.scrollY != scroll0 || t.selAnchor != anchor0 ||
		t.selCaret != caret0 || t.caretEOL != eol0
	onCh := t.OnChange
	t.mu.Unlock()
	if visual {
		t.Invalidate()
	}
	if changed && onCh != nil {
		onCh()
	}
	return changed
}

// contentChangedLocked сверяет производное состояние с изменившимся
// документом: пересобирает doc, зажимает выделение в новую длину и
// прокручивает к каретке — набор у нижнего края не должен уводить каретку за
// экран. Вызывать под t.mu.
func (t *RichText) contentChangedLocked() {
	t.noParas = false
	t.syncDocLocked()
	n := len(t.doc.runes)
	t.selCaret = clampInt(t.selCaret, 0, n)
	if t.selAnchor > n {
		t.selAnchor = n
	}
	t.ensureCaretVisibleLocked()
}

// setCaretEditLocked ставит каретку после правки: выделения нет, мигание
// начинается заново. В отличие от placeCaretLocked набор не прерывает и
// «стиль набора» не сбрасывает — каретку двинула сама правка.
func (t *RichText) setCaretEditLocked(pos int) {
	t.selAnchor = -1
	t.selCaret = pos
	t.caretEOL = false
	t.wantXOk = false
	t.caretStamp = richNowMs()
}

// selOrCaretLocked — выделение, а без него — пустой диапазон в каретке.
func (t *RichText) selOrCaretLocked() (lo, hi int) {
	if lo, hi = t.selRangeLocked(); lo != hi {
		return lo, hi
	}
	c := t.caretLocked()
	return c, c
}

// replaceLocked заменяет [lo, hi) тем, что вставит ins (получает позицию
// вставки, возвращает позицию за вставленным), одним действием истории:
// удаление выделения и вставка поверх — один Ctrl+Z.
func (t *RichText) replaceLocked(lo, hi int, ins func(at int) int) int {
	if lo == hi {
		return ins(lo)
	}
	d := t.rdoc
	d.BeginGroup()
	d.Delete(lo, hi)
	end := ins(lo)
	d.EndGroup()
	return end
}

// ─── Стиль набора ───────────────────────────────────────────────────────────

// baseStyleLocked — оформление, которое получит текст, вставленный на месте
// [lo, hi), если «стиль набора» не задан: у выделения — оформление его первого
// символа (как в Word: напечатали поверх слова — новое слово такое же), у
// каретки — символа слева, а в начале абзаца — первого. Вызывать под t.mu.
//
// Ссылка — исключение. Набор у границы ссылки не продолжает её: иначе слово,
// приписанное после ссылки, становилось бы частью адреса, и выйти из ссылки
// было бы нечем. Внутри ссылки (символы по обе стороны каретки — та же
// ссылка) набор остаётся ссылкой.
func (t *RichText) baseStyleLocked(lo, hi int) RichRun {
	d := t.doc
	ps, pe := 0, 0
	if len(d.paraStart) > 0 {
		pi := d.paraOf(lo)
		ps, pe = d.paraStart[pi], d.paraEnd(pi)
	}
	if lo < hi {
		if lo < pe {
			return t.rdoc.StyleAt(lo + 1)
		}
		return t.rdoc.StyleAt(lo)
	}
	st := t.rdoc.StyleAt(lo)
	if st.Link != "" && !(lo > ps && lo < pe && t.rdoc.StyleAt(lo+1).Link == st.Link) {
		st.Link = ""
	}
	return st
}

// styleForLocked — оформление вставляемого на месте [lo, hi) текста с учётом
// «стиля набора».
func (t *RichText) styleForLocked(lo, hi int) RichRun {
	if t.pendOn {
		s := t.pendStyle
		s.Text = ""
		return s
	}
	return t.baseStyleLocked(lo, hi)
}

// SelectionStyle — оформление в каретке или в начале выделения (поле Text
// пусто). Для панели инструментов: по нему видно, нажата ли кнопка «Ж»
// (RichRun.IsBold), какой кегль и цвет. В каретке учитывается и «стиль
// набора»: нажали Ctrl+B — кнопка «Ж» должна нажаться сразу, до первой
// буквы.
func (t *RichText) SelectionStyle() RichRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	lo, hi := t.selOrCaretLocked()
	return t.styleForLocked(lo, hi)
}

// SelectionParagraphFormat — выравнивание и отступы абзаца под кареткой (в
// выделении — абзаца его начала); Runs пусты. Для панели: показать, какое
// выравнивание включено.
func (t *RichText) SelectionParagraphFormat() RichParagraph {
	t.mu.Lock()
	defer t.mu.Unlock()
	lo, _ := t.selOrCaretLocked()
	return t.rdoc.ParagraphFormatAt(lo)
}

// SetSelectionStyle применяет fn к оформлению выделенного текста: цвет, кегль,
// шрифт, ссылка и всё, что есть у RichRun. fn не должна менять Text — если
// изменит, текст восстанавливается. Без выделения fn меняет «стиль набора»:
// следующий набранный текст получит это оформление (до движения каретки).
// Одна правка — одна запись отмены. Вне режима Editable — ничего не делает.
func (t *RichText) SetSelectionStyle(fn func(*RichRun)) {
	if fn == nil {
		return
	}
	t.edit(func() {
		lo, hi := t.selOrCaretLocked()
		if lo == hi {
			base := t.styleForLocked(lo, hi)
			fn(&base)
			base.Text = ""
			t.pendStyle, t.pendOn = base, true
			return
		}
		t.rdoc.ApplyStyle(lo, hi, fn)
	})
}

// SetParagraphFormat применяет fn к абзацам выделения (без выделения — к
// абзацу каретки): выравнивание и отступы. fn не должна менять Runs. Вне
// режима Editable — ничего не делает.
func (t *RichText) SetParagraphFormat(fn func(*RichParagraph)) {
	if fn == nil {
		return
	}
	t.edit(func() {
		lo, hi := t.selOrCaretLocked()
		t.rdoc.SetParagraphFormat(lo, hi, fn)
	})
}

// toggleLocked переключает свойство. На выделении: если оно уже включено у
// ВСЕГО выделенного — выключается, иначе включается у всего (так в Word:
// выделили «обычное и жирное» — Ctrl+B делает всё жирным, а не меняет
// местами). Без выделения — меняет «стиль набора».
func (t *RichText) toggleLocked(get func(RichRun) bool, set func(*RichRun, bool)) {
	lo, hi := t.selOrCaretLocked()
	if lo == hi {
		base := t.styleForLocked(lo, hi)
		set(&base, !get(base))
		base.Text = ""
		t.pendStyle, t.pendOn = base, true
		return
	}
	all, any := true, false
	for _, p := range t.rdoc.Fragment(lo, hi) {
		for _, r := range p.Runs {
			if r.Text == "" {
				continue
			}
			any = true
			if !get(r) {
				all = false
			}
		}
	}
	on := !(any && all)
	t.rdoc.ApplyStyle(lo, hi, func(r *RichRun) { set(r, on) })
}

// ToggleBold переключает жирный у выделения; без выделения — у следующего
// набранного текста. У курсивного рана жирность даёт жирный курсив
// (BuiltinFontBoldItalic), и обратно. Шрифт, которого движок не знает по имени
// (RegisterFont), жирным сделать нельзя — синтетического утолщения нет, — и
// его раны остаются как есть. Вне режима Editable — ничего не делает.
func (t *RichText) ToggleBold() {
	t.edit(func() {
		t.toggleLocked(RichRun.IsBold, func(r *RichRun, on bool) {
			if _, it, known := richFontFlags(r.Font); known {
				r.Font = richFontFromFlags(on, it)
			}
		})
	})
}

// ToggleItalic — то же для курсива (см. ToggleBold).
func (t *RichText) ToggleItalic() {
	t.edit(func() {
		t.toggleLocked(RichRun.IsItalic, func(r *RichRun, on bool) {
			if b, _, known := richFontFlags(r.Font); known {
				r.Font = richFontFromFlags(b, on)
			}
		})
	})
}

// ToggleUnderline переключает подчёркивание.
func (t *RichText) ToggleUnderline() {
	t.edit(func() {
		t.toggleLocked(func(r RichRun) bool { return r.Underline },
			func(r *RichRun, on bool) { r.Underline = on })
	})
}

// ToggleStrike переключает зачёркивание.
func (t *RichText) ToggleStrike() {
	t.edit(func() {
		t.toggleLocked(func(r RichRun) bool { return r.Strike },
			func(r *RichRun, on bool) { r.Strike = on })
	})
}

// ─── Ввод и удаление ────────────────────────────────────────────────────────

// typeString набирает s на месте каретки (поверх выделения — замещая его).
// Подряд набранные символы одним оформлением склеиваются в одну запись отмены.
func (t *RichText) typeString(s string) bool {
	return t.edit(func() {
		lo, hi := t.selOrCaretLocked()
		end := t.rdoc.TypeReplace(lo, hi, s, t.styleForLocked(lo, hi))
		t.setCaretEditLocked(end)
	})
}

// enter — Enter: новый абзац; inline (Shift+Enter) — мягкий перевод строки
// внутри абзаца.
//
// «Стиль набора» после Enter сбрасывается: оформление уходит в новый абзац
// самой вставкой (пустой абзац помнит стиль), и держать его ещё и отдельно —
// значит однажды расхождение между двумя копиями.
func (t *RichText) enter(inline bool) bool {
	return t.edit(func() {
		lo, hi := t.selOrCaretLocked()
		style := t.styleForLocked(lo, hi)
		end := t.replaceLocked(lo, hi, func(at int) int {
			if inline {
				return t.rdoc.InsertInline(at, "\n", style)
			}
			return t.rdoc.Insert(at, "\n", style)
		})
		t.setCaretEditLocked(end)
		t.pendOn = false
	})
}

// backspace удаляет выделение, а без него — символ перед кареткой (слово — с
// Ctrl). В начале абзаца удаляется только разделитель, то есть абзац
// сливается с предыдущим, и «слово назад» через границу не уходит: Ctrl+
// Backspace в начале строки в Word тоже убирает только перевод строки.
func (t *RichText) backspace(word bool) bool {
	return t.edit(func() {
		lo, hi := t.selRangeLocked()
		if lo == hi {
			c := t.caretLocked()
			if c == 0 {
				return
			}
			from := c - 1
			if word && c > t.doc.paraStart[t.doc.paraOf(c)] {
				from = t.doc.caretWordLeft(c)
			}
			lo, hi = from, c
		}
		t.rdoc.Delete(lo, hi)
		t.setCaretEditLocked(lo)
	})
}

// deleteForward — Delete: выделение или символ за кареткой (слово — с Ctrl,
// но не дальше конца абзаца: слияние абзацев — отдельное Delete).
func (t *RichText) deleteForward(word bool) bool {
	return t.edit(func() {
		lo, hi := t.selRangeLocked()
		if lo == hi {
			c := t.caretLocked()
			if c >= len(t.doc.runes) {
				return
			}
			to := c + 1
			if pi := t.doc.paraOf(c); word && c < t.doc.paraEnd(pi) {
				to = min(t.doc.caretWordRight(c), t.doc.paraEnd(pi))
			}
			lo, hi = c, to
		}
		t.rdoc.Delete(lo, hi)
		t.setCaretEditLocked(lo)
	})
}

// ─── Отмена ─────────────────────────────────────────────────────────────────

// Undo отменяет последнюю правку; каретка и выделение встают туда, куда
// указывает запись истории. false — отменять нечего или виджет не редактор.
func (t *RichText) Undo() bool { return t.undoRedo(false) }

// Redo возвращает последнюю отменённую правку.
func (t *RichText) Redo() bool { return t.undoRedo(true) }

func (t *RichText) undoRedo(redo bool) bool {
	return t.edit(func() {
		var (
			sel RichDocSel
			ok  bool
		)
		if redo {
			sel, ok = t.rdoc.Redo()
		} else {
			sel, ok = t.rdoc.Undo()
		}
		if !ok {
			return
		}
		t.noParas = false
		t.syncDocLocked()
		n := len(t.doc.runes)
		a, c := clampInt(sel.Anchor, 0, n), clampInt(sel.Caret, 0, n)
		t.setCaretEditLocked(c)
		if a != c {
			t.selAnchor = a
		}
		t.pendOn = false
	})
}

// CanUndo — есть ли что отменять (для доступности кнопки панели).
func (t *RichText) CanUndo() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rdoc.CanUndo()
}

// CanRedo — есть ли что возвращать.
func (t *RichText) CanRedo() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rdoc.CanRedo()
}

// ─── Буфер обмена ───────────────────────────────────────────────────────────

// Cut вырезает выделенное в буфер обмена (простой текст и HTML рядом, как
// Copy) одной записью отмены. false — выделения нет или виджет не редактор.
func (t *RichText) Cut() bool {
	t.mu.Lock()
	lo, hi := t.selRangeLocked()
	if !t.Editable || lo == hi {
		t.mu.Unlock()
		return false
	}
	plain := string(t.doc.runes[lo:hi])
	html := richSelectionHTML(t.parasLocked(), t.doc, lo, hi)
	t.mu.Unlock()
	// В буфер — вне замка: платформенный буфер может ходить в ОС долго.
	SetClipboardHTML(html, plain)
	return t.edit(func() {
		// Выделение могло измениться, пока буфер заполнялся: вырезаем только
		// то, что действительно скопировано.
		if l, h := t.selRangeLocked(); l != lo || h != hi {
			return
		}
		t.rdoc.Delete(lo, hi)
		t.setCaretEditLocked(lo)
	})
}

// Paste вставляет из буфера обмена на место каретки (поверх выделения —
// замещая его, одной записью отмены). Сначала берётся HTML — так вставка из
// Word и браузера сохраняет жирный, курсив, цвет, ссылки и абзацы, — иначе
// простой текст оформлением каретки. false — буфер пуст или виджет не
// редактор.
func (t *RichText) Paste() bool {
	if !t.isEditable() {
		return false
	}
	// Буфер читается вне замка: он может ходить в ОС долго.
	var paras []RichParagraph
	if html, ok := ClipboardHTML(); ok {
		paras = RichParagraphsFromHTML(html)
	}
	plain := ""
	if len(paras) == 0 {
		// HTML нет или в нём ни одного символа — берём простой текст.
		if plain = richNormalizeText(ClipboardGetText()); plain == "" {
			return false
		}
	}
	return t.edit(func() {
		lo, hi := t.selOrCaretLocked()
		var end int
		if len(paras) > 0 {
			end = t.replaceLocked(lo, hi, func(at int) int {
				return t.rdoc.InsertParagraphs(at, paras)
			})
		} else {
			style := t.styleForLocked(lo, hi)
			end = t.replaceLocked(lo, hi, func(at int) int {
				return t.rdoc.Insert(at, plain, style)
			})
		}
		t.setCaretEditLocked(end)
	})
}

// isEditable — читает Editable под замком: поле меняет приложение, а читают
// события из другой горутины.
func (t *RichText) isEditable() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Editable
}

// ─── Выгрузка и загрузка HTML ───────────────────────────────────────────────

// HTML возвращает весь документ как HTML-фрагмент — тем же путём, что Copy
// кладёт в буфер: выгружается только оформление, заданное явно (цвет, кегль и
// шрифт по умолчанию — свойство виджета и темы, а не документа).
func (t *RichText) HTML() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Конец диапазона — за последней позицией: выгрузка отбрасывает абзац, до
	// начала которого диапазон лишь «доходит», и пустой последний абзац
	// пропал бы.
	return richSelectionHTML(t.parasLocked(), t.doc, 0, len(t.doc.runes)+1)
}

// SetHTML заменяет содержимое разобранным HTML (RichParagraphsFromHTML).
// Как SetParagraphs: история отмены и выделение сбрасываются, OnChange не
// вызывается.
func (t *RichText) SetHTML(src string) {
	t.SetParagraphs(RichParagraphsFromHTML(src))
}

// ─── Tab ────────────────────────────────────────────────────────────────────

// AcceptsTab — контракт TabAcceptor: при AcceptTab в редакторе Tab вставляет
// отступ, а не уводит фокус. В режиме просмотра вставлять некуда — Tab
// остаётся обходу фокуса. Ctrl+Tab навигацией остаётся всегда.
func (t *RichText) AcceptsTab() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Editable && t.AcceptTab
}

// ─── Клавиатура редактора ───────────────────────────────────────────────────

// richPrintable — руна, которую набирают как символ: без управляющих
// (Backspace, Enter, Tab приходят с Rune < 32, а 0x7f — это Delete).
func richPrintable(r rune) bool {
	return r >= 32 && r != 0x7f && !(r >= 0x80 && r < 0xa0)
}

// editKey обрабатывает клавишу редактирования. true — клавиша разобрана;
// false — её дальше разберёт общий путь (навигация, Ctrl+C, Ctrl+A).
//
// Ctrl+B/I/U не реагируют на автоповтор: удержание не должно мигать жирным
// десять раз в секунду. Приложение, которому эти сочетания нужны для своего,
// перехватывает их раньше через InputBindings — движок отдаёт им дорогу до
// фокусного виджета.
func (t *RichText) editKey(e KeyEvent) bool {
	if !t.isEditable() {
		return false
	}
	ctrl := e.Mod&ModCtrl != 0
	shift := e.Mod&ModShift != 0
	alt := e.Mod&ModAlt != 0

	if ctrl && !alt {
		switch e.Code {
		case KeyZ:
			if shift {
				t.Redo()
			} else {
				t.Undo()
			}
		case KeyY:
			t.Redo()
		case KeyX:
			t.Cut()
		case KeyV:
			t.Paste()
		case KeyB:
			if !e.Repeat {
				t.ToggleBold()
			}
		case KeyI:
			if !e.Repeat {
				t.ToggleItalic()
			}
		case KeyU:
			if !e.Repeat {
				t.ToggleUnderline()
			}
		case KeyBackspace:
			t.backspace(true)
		case KeyDelete:
			t.deleteForward(true)
		default:
			// Ctrl+C, Ctrl+A, Ctrl+Insert, Ctrl+стрелки — общий путь. Прочие
			// сочетания с Ctrl символа не печатают.
			return false
		}
		return true
	}

	switch e.Code {
	case KeyBackspace:
		t.backspace(false)
	case KeyDelete:
		if shift {
			t.Cut()
		} else {
			t.deleteForward(false)
		}
	case KeyInsert:
		if !shift {
			return false
		}
		t.Paste()
	case KeyEnter:
		t.enter(shift)
	case KeyTab:
		if !t.AcceptsTab() || shift {
			return false
		}
		t.typeString(strings.Repeat(" ", richTabSpaces))
	default:
		// Alt без Ctrl — команды приложения, а не буквы; Ctrl+Alt — AltGr
		// европейских раскладок, он как раз печатает символ.
		if alt && !ctrl || !richPrintable(e.Rune) {
			return false
		}
		t.typeString(string(e.Rune))
	}
	return true
}

// ─── Контекстное меню ───────────────────────────────────────────────────────

// ContextMenuAt — контракт ContextMenuProvider: меню редактора под точкой.
// Вырезать/Копировать/Вставить/Выделить всё, с теми же строками, что у
// TextBox (перевод — на стороне приложения, как и там). nil — виджет не
// редактор, точка вне текста или приложение задало своё ContextMenu: его
// покажет движок следующим шагом, и мешать ему не нужно.
//
// Правый щелчок вне выделения переносит каретку под курсор — иначе «Вставить»
// вставило бы туда, где каретка осталась от прошлого щелчка, а не туда, куда
// указал человек. Внутри выделения оно сохраняется: по нему и хотят вырезать.
func (t *RichText) ContextMenuAt(x, y int) *PopupMenu {
	t.mu.Lock()
	b := t.Base.Bounds()
	if !t.Editable || t.Base.ContextMenu != nil || !image.Pt(x, y).In(b) {
		t.mu.Unlock()
		return nil
	}
	t.layoutLocked()
	if t.barOn && x >= b.Max.X-richBarW {
		t.mu.Unlock()
		return nil
	}
	cx, cy := t.contentPointLocked(x, y)
	off, eol := t.lay.hitCaret(t.measurer, cx, cy)
	lo, hi := t.selRangeLocked()
	moved := false
	if lo == hi || off < lo || off > hi {
		t.placeCaretLocked(off, eol, false, false)
		lo, hi = 0, 0
		moved = true
	}
	hasSel := lo != hi
	hasText := len(t.doc.runes) > 0
	t.mu.Unlock()
	if moved {
		t.Invalidate()
	}
	_, hasHTML := ClipboardHTML()
	hasClip := hasHTML || ClipboardGetText() != ""

	t.menuMu.Lock()
	defer t.menuMu.Unlock()
	return t.menu.build(x, y, []MenuItem{
		{Text: "Cut", Disabled: !hasSel, OnClick: func() { t.Cut() }},
		{Text: "Copy", Disabled: !hasSel, OnClick: func() { t.Copy() }},
		{Text: "Paste", Disabled: !hasClip, OnClick: func() { t.Paste() }},
		{Separator: true},
		{Text: "Select All", Disabled: !hasText, OnClick: t.SelectAll},
	})
}

// ─── Оверлей контекстного меню ──────────────────────────────────────────────
//
// Меню открывает движок (ContextMenuAt), а рисует его владелец: оверлеи
// собираются обходом дерева, и меню, ни за кем не закреплённое, не
// нарисовалось бы. Контракт тот же, что у таблицы и дерева.

// HasOverlay реализует OverlayDrawer.
func (t *RichText) HasOverlay() bool {
	t.menuMu.Lock()
	defer t.menuMu.Unlock()
	return t.menu.open()
}

// DrawOverlay рисует открытое меню поверх всего UI.
func (t *RichText) DrawOverlay(ctx DrawContext) {
	t.menuMu.Lock()
	defer t.menuMu.Unlock()
	t.menu.drawOverlay(ctx)
}

// OverlayBounds — прямоугольник открытого меню (для выноса в окно ОС).
func (t *RichText) OverlayBounds() image.Rectangle {
	t.menuMu.Lock()
	defer t.menuMu.Unlock()
	return t.menu.overlayBounds()
}

// Dismiss закрывает меню. Реализует Dismissable.
func (t *RichText) Dismiss() {
	t.menuMu.Lock()
	defer t.menuMu.Unlock()
	t.menu.dismiss()
}
