package native

// popup.go — приёмники ввода дочерних попапов бэкенда.

// ChildPopupHandlers — приёмники ввода попапов. Координаты локальные для
// попапа и физические, как и всюду на границе с бэкендом.
//
//facadegen:skip
type ChildPopupHandlers struct {
	Move   func(id uintptr, x, y int)
	Button func(id uintptr, x, y, button int, pressed bool)
	Wheel  func(id uintptr, x, y int, dx, dy float64)
	// Done — попап закрыл не движок, а система: щелчок снаружи при захвате
	// ввода. Оверлей в дереве виджетов надо убрать, иначе он останется
	// открытым без окна.
	Done func(id uintptr)
}
