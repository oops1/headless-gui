// input_queue.go — ввод окна идёт движку через его очередь (GG-68).
//
// Насос событий ОС и цикл кадров — разные горутины, а дерево виджетов
// синхронизации не имеет. Пока окно звало SendMouseMove и SendKeyEvent прямо
// из насоса, обработчики виджетов выполнялись на нём, а функции из Post,
// анимации и отрисовка — на горутине движка, и общего замка у них не было:
// обработчик нажатия и функция из Post могли менять одно поле одновременно.
//
// Теперь событие ставится в очередь движка (engine.Post) и выполняется на его
// горутине — там же, где всё остальное. Насос ОС ничего не ждёт: ни отрисовки,
// ни обработчика.
package window

import "sync"

// onEngine выполняет fn на горутине движка.
//
// Движок без Post (своя реализация EngineAPI) получает вызов сразу, на
// вызывающей горутине, — как было до очереди. Отдельная горутина здесь не
// годится: она потеряла бы порядок событий.
func (s *surface) onEngine(fn func()) {
	if p, ok := s.eng.(poster); ok {
		p.Post(fn)
		return
	}
	fn()
}

// onEngineFunc оборачивает колбэк приложения, который бэкенд зовёт со своего
// потока (клик по уведомлению), чтобы тот выполнялся на горутине движка.
func (s *surface) onEngineFunc(fn func()) func() {
	if fn == nil {
		return nil
	}
	return func() { s.onEngine(fn) }
}

// post ставит событие окна в очередь движка. Движение мыши, стоящее в очереди
// до него, больше не дополняется: следующее движение встанет уже после этого
// события.
func (s *surface) post(fn func()) {
	s.in.post(s.onEngine, fn)
}

// onNative выполняет вызов окна ОС на потоке его цикла сообщений.
//
// Обработчики виджетов теперь выполняются на горутине движка, а Win32
// привязывает окно к потоку: SetCursor с чужого потока не действует, а
// ShowWindow выполняется асинхронно. Бэкенд без маршалинга (Wayland, Cocoa)
// получает вызов сразу.
func (s *surface) onNative(fn func()) {
	if inv, ok := s.native.(uiThreadInvoker); ok {
		inv.InvokeOnUIThread(fn)
		return
	}
	fn()
}

// deliverMove — движение мыши на горутине движка: событие, затем форма курсора
// под указателем.
//
// Курсор окну ОС отдаётся на его потоке и только при смене: иначе каждое
// движение мыши стоило бы сообщения потоку окна.
func (s *surface) deliverMove(x, y int) {
	x, y = s.toContent(x, y)
	s.eng.SendMouseMove(x, y)
	sc, ok := s.native.(interface{ SetCursor(c int) })
	if !ok {
		return
	}
	c := int(s.eng.CursorAt(x, y))
	if s.cursorSet && c == s.cursor {
		return
	}
	s.cursor, s.cursorSet = c, true
	s.onNative(func() { sc.SetCursor(c) })
}

// inputQueue склеивает движения мыши, стоящие в очереди движка.
//
// Мышь присылает сотни движений в секунду, а движку нужно последнее: пока
// поставленное движение не выполнено, новое только обновляет его координаты.
// Любое другое событие закрывает стоящее движение — следующее встанет после
// него, и порядок «движение — нажатие — движение» не нарушится.
//
// Нулевое значение готово к работе.
type inputQueue struct {
	mu   sync.Mutex
	open *queuedMove // движение в очереди, которое ещё можно дополнить
}

// queuedMove — движение мыши, ждущее выполнения.
type queuedMove struct {
	x, y int
	send func(x, y int)
}

// move ставит движение в очередь через run или дополняет уже стоящее.
func (q *inputQueue) move(run func(func()), x, y int, send func(x, y int)) {
	q.mu.Lock()
	if m := q.open; m != nil {
		m.x, m.y, m.send = x, y, send
		q.mu.Unlock()
		return
	}
	m := &queuedMove{x: x, y: y, send: send}
	q.open = m
	q.mu.Unlock()

	run(func() {
		q.mu.Lock()
		if q.open == m {
			q.open = nil
		}
		x, y, send := m.x, m.y, m.send
		q.mu.Unlock()
		send(x, y)
	})
}

// post закрывает стоящее движение и ставит fn в очередь через run.
func (q *inputQueue) post(run func(func()), fn func()) {
	q.mu.Lock()
	q.open = nil
	q.mu.Unlock()
	run(fn)
}
