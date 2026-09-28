package widget

import (
	"image"
	"image/color"
)

// colorpicker_input.go — мышь и клавиатура поля цвета.
//
// Клавиатура та же, что у поля даты: набор прямо в поле, Enter применяет,
// Esc откатывает, Alt+↓ и F4 раскрывают палитру. В раскрытой палитре стрелки
// ходят по образцам, Enter выбирает; каналы двигаются мышью, а с клавиатуры —
// теми же стрелками с Ctrl, чтобы не занимать обычные.

// change выполняет fn под замком и рассылает последствия: перерисовку и, если
// цвет сменился, OnChanged с командой. Колбэки — вне замка: обработчик почти
// наверняка обратится к контролу.
func (p *ColorPicker) change(fn func()) {
	p.mu.Lock()
	before := p.value
	wasOpen := p.open
	fn()
	after := p.value
	open := p.open
	cb := p.OnChanged
	cmd := p.commands["ValueChangedCommand"]
	p.mu.Unlock()

	if open || wasOpen {
		notifyUIChanged() // палитра лежит вне границ поля
	} else {
		p.Invalidate()
	}
	if before == after {
		return
	}
	if cb != nil {
		cb(after)
	}
	if cmd != nil && cmd.CanExecute(after) {
		cmd.Execute(after)
	}
}

// Dismiss — контракт Dismissable: щелчок мимо закрывает палитру.
func (p *ColorPicker) Dismiss() {
	p.mu.Lock()
	open := p.open
	p.mu.Unlock()
	if open {
		p.SetDropDownOpen(false)
	}
}

// SetFocused принимает и отдаёт фокус. Уходя, поле применяет набранное: код,
// оставленный в поле и забытый, иначе пропал бы молча.
func (p *ColorPicker) SetFocused(v bool) {
	p.mu.Lock()
	was := p.focused
	p.focused = v
	editing := p.editing
	if v && !was {
		// Фокус выделяет содержимое целиком, как во всяком поле ввода:
		// иначе первый набранный знак дописался бы к коду и дал мешанину
		// вроде «#000000#E8».
		p.selectAll = true
	}
	p.mu.Unlock()
	if was && !v && editing {
		p.commit()
		p.mu.Lock()
		p.revertLocked() // неразобранное не держим: поле снова показывает цвет
		p.mu.Unlock()
	}
	p.Invalidate()
}

// IsFocused сообщает, в фокусе ли поле.
func (p *ColorPicker) IsFocused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.focused
}

// AcceptsFocus — поле принимает фокус по Tab, как всякое поле ввода.
func (p *ColorPicker) AcceptsFocus() bool { return p.IsEnabled() }

// OnKeyEvent — клавиатура поля и раскрытой палитры.
func (p *ColorPicker) OnKeyEvent(e KeyEvent) {
	if !e.Pressed || !p.IsEnabled() {
		return
	}
	alt := e.Mod&ModAlt != 0
	ctrl := e.Mod&ModCtrl != 0
	p.change(func() {
		if p.open {
			p.dropKeyLocked(e, alt, ctrl)
			return
		}
		switch {
		case e.Code == KeyF4, alt && e.Code == KeyDown:
			p.commitInPlaceLocked()
			p.setOpenLocked(true)
		case e.Code == KeyEnter:
			p.commitInPlaceLocked()
		case e.Code == KeyEscape:
			p.revertLocked()
		case e.Code == KeyBackspace:
			p.beginEditLocked()
			if p.selectAll {
				p.text, p.selectAll = nil, false
			} else if n := len(p.text); n > 0 {
				p.text = p.text[:n-1]
			}
			p.invalid = false
		case e.Code == KeyDelete:
			p.beginEditLocked()
			p.text, p.selectAll, p.invalid = nil, false, false
		case ctrl && e.Code == KeyA:
			p.selectAll = true
		case isHexRune(e.Rune):
			p.beginEditLocked()
			if p.selectAll {
				p.text, p.selectAll = nil, false
			}
			if len(p.text) < 8 { // «#RRGGBB» и не больше
				p.text = append(p.text, e.Rune)
			}
			p.invalid = false
		}
	})
}

// isHexRune — знак, из которого может состоять код цвета.
func isHexRune(r rune) bool {
	switch {
	case r == '#':
		return true
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		return true
	}
	return false
}

// beginEditLocked переводит поле в набор: текст начинается с того, что было
// видно, — как в обычном поле ввода.
func (p *ColorPicker) beginEditLocked() {
	if p.editing {
		return
	}
	p.text = []rune(p.textLocked())
	p.editing = true
}

// commitInPlaceLocked применяет набранное под уже взятым замком: рассылку
// берёт на себя change, внутри которой это и зовётся.
func (p *ColorPicker) commitInPlaceLocked() {
	p.commitLocked()
}

// dropKeyLocked — клавиши раскрытой палитры.
func (p *ColorPicker) dropKeyLocked(e KeyEvent, alt, ctrl bool) {
	// Ctrl+стрелки правят канал: обычные стрелки ходят по образцам, и
	// отдавать их каналам значило бы отнять главное движение палитры.
	if ctrl {
		switch e.Code {
		case KeyLeft, KeyRight:
			ch := p.cursorChannelLocked()
			step := 1
			if e.Code == KeyLeft {
				step = -1
			}
			p.setChannelLocked(ch, cpChannel(p.value, ch)+step)
			return
		}
	}
	switch e.Code {
	case KeyEscape, KeyF4:
		p.setOpenLocked(false)
	case KeyUp:
		if alt {
			p.setOpenLocked(false)
			return
		}
		p.moveCursorLocked(-cpPaletteCols)
	case KeyDown:
		p.moveCursorLocked(cpPaletteCols)
	case KeyLeft:
		p.moveCursorLocked(-1)
	case KeyRight:
		p.moveCursorLocked(1)
	case KeyHome:
		p.cursor = 0
	case KeyEnd:
		p.cursor = len(p.palette) - 1
	case KeyEnter, KeySpace:
		if p.cursor >= 0 && p.cursor < len(p.palette) {
			p.setValueLocked(p.palette[p.cursor])
			p.setOpenLocked(false)
		}
	}
}

// cursorChannelLocked — канал, который правят с клавиатуры: тот, что под
// мышью, иначе красный.
func (p *ColorPicker) cursorChannelLocked() int {
	if p.hoverBand >= 0 {
		return p.hoverBand
	}
	return 0
}

// moveCursorLocked двигает курсор по сетке образцов, не выходя за неё.
func (p *ColorPicker) moveCursorLocked(delta int) {
	n := len(p.palette)
	if n == 0 {
		return
	}
	i := p.cursor + delta
	if i < 0 || i >= n {
		return // край сетки: курсор не перескакивает через ряд
	}
	p.cursor = i
}

// setValueLocked задаёт цвет под уже взятым замком (рассылку берёт change).
func (p *ColorPicker) setValueLocked(c color.RGBA) {
	c.A = 255
	if p.value == c {
		return
	}
	p.value = c
	p.editing, p.invalid = false, false
	p.text = nil
}

// setChannelLocked правит один канал под уже взятым замком.
func (p *ColorPicker) setChannelLocked(ch, v int) {
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	c := p.value
	switch ch {
	case 0:
		c.R = uint8(v)
	case 1:
		c.G = uint8(v)
	case 2:
		c.B = uint8(v)
	}
	p.setValueLocked(c)
}

// OnMouseButton — щелчки по полю, кнопке, образцам и полосам каналов.
func (p *ColorPicker) OnMouseButton(e MouseEvent) bool {
	if !p.IsEnabled() || e.Button != MouseLeft {
		return false
	}
	if !e.Pressed {
		p.mu.Lock()
		was := p.dragChannel >= 0
		p.dragChannel = -1
		p.mu.Unlock()
		return was
	}
	handled := false
	p.change(func() {
		field := p.Base.Bounds()
		pt := image.Pt(e.X, e.Y)
		if p.open {
			handled = true
			drop := cpDropRect(field, cpRows(len(p.palette)))
			if pt.In(field) || !pt.In(drop) {
				p.setOpenLocked(false)
				return
			}
			if i := p.swatchAtLocked(drop, pt); i >= 0 {
				p.cursor = i
				p.setValueLocked(p.palette[i])
				p.setOpenLocked(false)
				return
			}
			if ch, band := p.bandAtLocked(drop, pt); ch >= 0 {
				p.dragChannel = ch
				p.setChannelLocked(ch, cpValueAtX(band, e.X))
			}
			return
		}
		if !pt.In(field) {
			return
		}
		handled = true
		p.focused = true
		// Щелчок по образцу или стрелке раскрывает палитру, по коду —
		// ставит поле в набор: у образца и кода разные назначения.
		if pt.In(cpButtonRect(field)) || pt.In(cpSwatchRect(field)) {
			p.commitInPlaceLocked()
			p.setOpenLocked(true)
			return
		}
		p.beginEditLocked()
		p.selectAll = true
	})
	return handled
}

// swatchAtLocked — образец под точкой; -1 — мимо.
func (p *ColorPicker) swatchAtLocked(drop image.Rectangle, pt image.Point) int {
	for i := range p.palette {
		if pt.In(cpCellRect(drop, i)) {
			return i
		}
	}
	return -1
}

// bandAtLocked — полоса канала под точкой и её прямоугольник; ch=-1 — мимо.
//
// Полоса ловит щелчок с запасом по высоте: дорожка тонкая, и попасть в неё
// точно трудно — промах вместо движения ползунка раздражает.
func (p *ColorPicker) bandAtLocked(drop image.Rectangle, pt image.Point) (int, image.Rectangle) {
	rows := cpRows(len(p.palette))
	for ch := 0; ch < 3; ch++ {
		band := cpBandRect(drop, rows, ch)
		if pt.In(band.Inset(-4)) {
			return ch, band
		}
	}
	return -1, image.Rectangle{}
}

// OnMouseMove — подсветка образца под мышью и перетаскивание ползунка канала.
func (p *ColorPicker) OnMouseMove(x, y int) {
	p.mu.Lock()
	if !p.open {
		p.mu.Unlock()
		return
	}
	drop := cpDropRect(p.Base.Bounds(), cpRows(len(p.palette)))
	pt := image.Pt(x, y)
	hover := p.swatchAtLocked(drop, pt)
	band, _ := p.bandAtLocked(drop, pt)
	drag := p.dragChannel
	changed := hover != p.hoverSwatch || band != p.hoverBand
	p.hoverSwatch, p.hoverBand = hover, band
	p.mu.Unlock()

	if drag >= 0 {
		p.change(func() {
			band := cpBandRect(drop, cpRows(len(p.palette)), drag)
			p.setChannelLocked(drag, cpValueAtX(band, x))
		})
		return
	}
	if changed {
		notifyUIChanged()
	}
}

// WantsCapture — поле берёт мышь, пока тянут ползунок канала: курсор уходит
// за край дорожки, а ползунок должен идти за ним.
func (p *ColorPicker) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed || !p.IsEnabled() {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.open {
		return false
	}
	drop := cpDropRect(p.Base.Bounds(), cpRows(len(p.palette)))
	ch, _ := p.bandAtLocked(drop, image.Pt(e.X, e.Y))
	return ch >= 0
}

// OnMouseWheelPixels — колесо над полосой канала правит её значение.
func (p *ColorPicker) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	handled := false
	p.change(func() {
		if !p.open || dy == 0 {
			return
		}
		drop := cpDropRect(p.Base.Bounds(), cpRows(len(p.palette)))
		ch, _ := p.bandAtLocked(drop, image.Pt(x, y))
		if ch < 0 {
			return
		}
		handled = true
		step := 1
		if dy > 0 {
			step = -1
		}
		p.setChannelLocked(ch, cpChannel(p.value, ch)+step)
	})
	return handled
}

// Cursor — текстовый курсор над кодом, стрелка над образцом, кнопкой и палитрой.
func (p *ColorPicker) Cursor(x, y int) Cursor {
	field := p.Base.Bounds()
	pt := image.Pt(x, y)
	if pt.In(field) && !pt.In(cpButtonRect(field)) && !pt.In(cpSwatchRect(field)) {
		return CursorIBeam
	}
	return CursorArrow
}
