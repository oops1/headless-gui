// desiredsize.go — сколько места просит содержимое.
//
// Ни StackPanel, ни ScrollView не считали свой размер по детям: высоту
// прокрутки выставляло приложение, а панель раскладывала по уже заданным
// размерам. Двухпроходного Measure/Arrange здесь нет и не планируется — это
// другая архитектура раскладки, — но одного вопроса «сколько тебе надо» уже
// достаточно для главного больного случая: сворачиваемая группа внутри
// прокрутки. Каждое раскрытие меняло высоту содержимого, и приложение
// пересчитывало её руками в колбэке на каждом экране с динамическим составом.
package widget

// DesiredSizer — виджет, умеющий сказать, сколько места ему нужно.
//
// Ноль по оси означает «мне всё равно, решай сам»: разделителю не важна длина,
// метке — высота строки задаётся шрифтом. Контейнер подставит на место нуля
// своё значение, как делал и раньше.
//
// Реализуют его немногие — у большинства виджетов размер задаёт разметка или
// приложение. Именно поэтому спрашиваем интерфейсом, а не методом Widget:
// пустая реализация у сотни типов не сообщала бы ничего, кроме шума.
type DesiredSizer interface {
	DesiredSize() (w, h int)
}

// desiredOf возвращает желаемый размер виджета.
//
// Порядок такой: сперва DesiredSize (виджет знает про себя больше всех), потом
// размер, заданный разметкой (GetXAMLSize), и лишь потом текущие границы. Тот
// же порядок использует раскладка DockPanel и StackPanel — иначе виджет,
// который уже разложили, навсегда закрепил бы за собой первую попавшуюся
// высоту.
func desiredOf(w Widget) (int, int) {
	dw, dh := 0, 0
	if ds, ok := w.(DesiredSizer); ok {
		dw, dh = ds.DesiredSize()
	}
	if xw, xh := xamlSizeOf(w); dw <= 0 && xw > 0 {
		dw = xw
	} else if xh > 0 && dh <= 0 {
		dh = xh
	}
	b := w.Bounds()
	if dw <= 0 {
		dw = b.Dx()
	}
	if dh <= 0 {
		dh = b.Dy()
	}
	return dw, dh
}

// marginOf возвращает внешние отступы виджета.
func marginOf(w Widget) Margin {
	if mg, ok := w.(interface{ GetMargin() Margin }); ok {
		return mg.GetMargin()
	}
	return Margin{}
}

// DesiredSize сообщает, сколько места нужно содержимому панели.
//
// Скрытые дети не занимают ничего — ни своего размера, ни промежутка после
// себя: свёрнутая группа не должна оставлять по себе пустоту, ради этого всё и
// затевалось.
func (sp *StackPanel) DesiredSize() (int, int) {
	along, across := 0, 0
	n := 0
	for _, child := range sp.children {
		if !IsWidgetVisible(child) {
			continue
		}
		cw, ch := desiredOf(child)
		m := marginOf(child)
		if n > 0 {
			along += sp.Spacing
		}
		if sp.Orientation == OrientationHorizontal {
			along += cw + m.Left + m.Right
			if v := ch + m.Top + m.Bottom; v > across {
				across = v
			}
		} else {
			along += ch + m.Top + m.Bottom
			if v := cw + m.Left + m.Right; v > across {
				across = v
			}
		}
		n++
	}
	if n == 0 {
		return 0, 0
	}
	pad := sp.Padding * 2
	if sp.Orientation == OrientationHorizontal {
		return along + pad, across + pad
	}
	return across + pad, along + pad
}

// ContentSize возвращает размер содержимого прокрутки, посчитанный по детям.
//
// Считается так же, как вертикальный StackPanel: дети идут сверху вниз в
// порядке добавления — ровно это ScrollView и показывает.
func (sv *ScrollView) ContentSize() (int, int) {
	w, h := 0, 0
	for _, child := range sv.children {
		if !IsWidgetVisible(child) {
			continue
		}
		cw, ch := desiredOf(child)
		m := marginOf(child)
		// По нижнему краю ребёнка, а не суммой высот: дети прокрутки обычно
		// расставлены абсолютными координатами внутри неё, и сумма высот дала
		// бы не тот ответ, что видно на экране.
		if bottom := child.Bounds().Max.Y - sv.bounds.Min.Y + m.Bottom; bottom > h {
			h = bottom
		}
		if ch == 0 && cw == 0 {
			continue
		}
		if right := child.Bounds().Max.X - sv.bounds.Min.X + m.Right; right > w {
			w = right
		}
	}
	return w, h
}

// FitContent пересчитывает ContentHeight по детям.
//
// Зовётся приложением после того, как состав или размеры содержимого
// изменились: раскрыли группу, добавили строку, скрыли блок. Автоматически
// делать это на каждый кадр нельзя — обход детей на каждом кадре стоит дороже,
// чем один вызов там, где содержимое действительно поменялось, а меняется оно
// на порядки реже, чем рисуется.
func (sv *ScrollView) FitContent() {
	_, h := sv.ContentSize()
	if sv.ContentHeight == h {
		return
	}
	sv.ContentHeight = h
	// Прокрутка могла оказаться за новым концом: содержимое сжалось.
	sv.SetScrollY(sv.ScrollY())
	sv.Invalidate()
}

// layoutSizeOf возвращает размер ребёнка для раскладки контейнера.
//
// Спрашивает DesiredSize только у тех, кто его реализует, и берёт лишь
// ненулевые оси. Для всех прочих — текущие границы, ровно как было: контейнеры
// раскладывают чужие виджеты уже много версий, и менять им правила ради двух
// новых типов значило бы переписать раскладку у всех.
func layoutSizeOf(w Widget) (int, int) {
	b := w.Bounds()
	cw, ch := b.Dx(), b.Dy()
	if ds, ok := w.(DesiredSizer); ok {
		dw, dh := ds.DesiredSize()
		if dw > 0 {
			cw = dw
		}
		if dh > 0 {
			ch = dh
		}
	}
	return cw, ch
}

// Relayout пересчитывает раскладку панели.
//
// Нужен, когда размер ребёнка изменился сам по себе — свернули Expander,
// скрыли блок, дописали строк в список. Панель об этом не узнаёт: у виджета
// нет ссылки на родителя, и уведомить его некому.
//
// Одна строка в обработчике вместо ручного пересчёта высот — то, ради чего
// заводился DesiredSize.
func (sp *StackPanel) Relayout() {
	sp.layout()
	sp.Invalidate()
}

// DesiredSize сообщает высоту, которую занимает раскрывающаяся панель.
//
// Свёрнутая — только заголовок: в столбике она не должна держать место под
// спрятанное содержимое. Развёрнутая — заголовок плюс самое высокое из
// содержимого: дети получают всю область содержимого целиком, поэтому нужен
// максимум, а не сумма.
func (e *Expander) DesiredSize() (int, int) {
	h := e.headerH()
	if !e.IsExpanded {
		return 0, h
	}
	content := 0
	for _, c := range e.children {
		if !IsWidgetVisible(c) {
			continue
		}
		if _, ch := desiredOf(c); ch > content {
			content = ch
		}
	}
	return 0, h + content + 1 // +1 — нижняя рамка панели
}
