package base

// externalsurface_input.go — ввод для окна чужой программы (ADR-0002, 2.2).
//
// Сессия WinLine показывает окна Wayland-клиентов как ExternalSurface в
// дереве движка, рядом с собственными окнами оболочки. Чтобы Z-порядок был
// один, ввод к ним тоже должен идти через движок: щелчок попадает в то окно,
// что лежит сверху, захват мыши держится на окне, начавшем перетаскивание,
// клавиши идут тому, у кого фокус. Поверхность переадресует события в
// колбэки SurfaceInput, а сессия — в протокол клиента.
//
// Без SetInput поверхность ввода не принимает и ведёт себя как в v3.39:
// щелчок по ней снимает фокус, клавиши не доходят никуда, Tab её обходит.

import "image"

// SurfaceInput — колбэки ввода окна чужой программы. Координаты — в
// логических единицах относительно левого верхнего угла поверхности (как
// surface-local у Wayland). Нулевой колбэк — событие не нужно.
type SurfaceInput struct {
	// Move — указатель над поверхностью (она верхняя под ним) или держит
	// захват: тогда координаты бывают и за её краем.
	Move func(x, y int)
	// Leave — указатель ушёл: на другой виджет, за окно, под поднятое окно.
	Leave func()
	// Button — кнопка мыши. Левая кнопка, нажатая над поверхностью, захватывает
	// мышь до отпускания: движение и отпускание придут сюда, даже когда
	// указатель над панелью задач. true — событие принято.
	Button func(e MouseEvent) bool
	// Wheel — колесо в единицах WheelNotch.
	Wheel func(e WheelEvent) bool
	// Key — клавиша, пока у поверхности фокус. Tab тоже приходит сюда.
	Key func(e KeyEvent)
	// Touch — касание.
	Touch func(e TouchEvent) bool
	// Focus — фокус клавиатуры пришёл или ушёл.
	Focus func(on bool)
	// Cursor — форма курсора над точкой; nil — стрелка.
	Cursor func(x, y int) Cursor
}

func (in *SurfaceInput) active() bool {
	return in != nil && (in.Move != nil || in.Leave != nil || in.Button != nil ||
		in.Wheel != nil || in.Key != nil || in.Touch != nil || in.Focus != nil ||
		in.Cursor != nil)
}

// SetInput задаёт колбэки ввода. Нулевой SurfaceInput снимает их: поверхность
// снова не принимает ввод. Безопасно из любой горутины; колбэки зовутся на
// горутине движка.
func (s *ExternalSurface) SetInput(in SurfaceInput) {
	next := &in
	if !next.active() {
		next = nil
	}
	prev := s.input.Swap(next)
	switch {
	case prev == nil && next != nil:
		pointerOwners.Add(1)
	case prev != nil && next == nil:
		pointerOwners.Add(-1)
	}
}

func (s *ExternalSurface) loadInput() *SurfaceInput { return s.input.Load() }

// local переводит точку кадра поверхности в её собственные координаты.
func (s *ExternalSurface) local(x, y int) (int, int) {
	b := s.Bounds()
	return x - b.Min.X, y - b.Min.Y
}

// PointerOver реализует PointerOwner.
func (s *ExternalSurface) PointerOver(x, y int) {
	if in := s.loadInput(); in != nil && in.Move != nil {
		in.Move(s.local(x, y))
	}
}

// PointerOut реализует PointerOwner.
func (s *ExternalSurface) PointerOut() {
	if in := s.loadInput(); in != nil && in.Leave != nil {
		in.Leave()
	}
}

// OnMouseButton реализует MouseClickHandler.
func (s *ExternalSurface) OnMouseButton(e MouseEvent) bool {
	in := s.loadInput()
	if in == nil || in.Button == nil {
		return false
	}
	e.X, e.Y = s.local(e.X, e.Y)
	return in.Button(e)
}

// WantsCapture реализует CaptureRequester: левая кнопка над поверхностью с
// вводом захватывает мышь, как неявный захват у Wayland.
func (s *ExternalSurface) WantsCapture(e MouseEvent) bool {
	in := s.loadInput()
	return in != nil && in.Button != nil && e.Pressed && e.Button == MouseLeft
}

// OnWheelEvent реализует WheelEventHandler.
func (s *ExternalSurface) OnWheelEvent(e WheelEvent) bool {
	in := s.loadInput()
	if in == nil || in.Wheel == nil {
		return false
	}
	e.X, e.Y = s.local(e.X, e.Y)
	return in.Wheel(e)
}

// OnKeyEvent реализует KeyHandler.
func (s *ExternalSurface) OnKeyEvent(e KeyEvent) {
	if in := s.loadInput(); in != nil && in.Key != nil {
		in.Key(e)
	}
}

// AcceptsTab реализует TabAcceptor: Tab в окне программы — её клавиша, а не
// переход фокуса оболочки.
func (s *ExternalSurface) AcceptsTab() bool {
	in := s.loadInput()
	return in != nil && in.Key != nil
}

// OnTouch реализует TouchHandler.
func (s *ExternalSurface) OnTouch(ev TouchEvent) bool {
	in := s.loadInput()
	if in == nil || in.Touch == nil {
		return false
	}
	p := image.Pt(s.local(ev.X, ev.Y))
	ev.X, ev.Y = p.X, p.Y
	return in.Touch(ev)
}

// SetFocused реализует Focusable.
func (s *ExternalSurface) SetFocused(on bool) {
	if s.focused.Swap(on) == on {
		return
	}
	if in := s.loadInput(); in != nil && in.Focus != nil {
		in.Focus(on)
	}
}

// IsFocused реализует Focusable.
func (s *ExternalSurface) IsFocused() bool { return s.focused.Load() }

// TabIndex исключает поверхность из обхода Tab оболочки: в окно программы
// попадают щелчком, а Tab внутри него — её собственный.
func (s *ExternalSurface) TabIndex() int { return -1 }

// Cursor реализует CursorProvider.
func (s *ExternalSurface) Cursor(x, y int) Cursor {
	if in := s.loadInput(); in != nil && in.Cursor != nil {
		return in.Cursor(s.local(x, y))
	}
	return CursorArrow
}
