package native

// textrender.go — как ОС сглаживает текст на экране.

// TextRendering — настройка сглаживания текста в ОС. По ней окно выбирает
// слой качества текста движка (window.Window.SetTextQualityFromSystem):
// субпиксельный, как у ClearType, или серый.
type TextRendering struct {
	// Known — бэкенд прочитал настройку ОС. false — не умеет (Linux, macOS,
	// window/minimal), окна ещё нет или чтение не удалось.
	Known bool
	// Subpixel — включено субпиксельное сглаживание (Windows: ClearType).
	// Выключенное сглаживание и серое — false.
	Subpixel bool
	// BGR — порядок подпикселей монитора синий-слева (Windows: ориентация
	// сглаживания FE_FONTSMOOTHINGORIENTATIONBGR).
	BGR bool
	// Remote — окно показано в удалённом сеансе (RDP): картинку сжимают и
	// передают по сети, на чужой монитор с неизвестным порядком подпикселей, и
	// цветная кайма там только вредит.
	Remote bool
}
