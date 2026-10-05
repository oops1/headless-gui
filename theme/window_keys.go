package theme

// Метрики и токены заголовка окна (widget.Window).
//
// Профиль объявляет их через SetMetric и SetColor; мост (widget.Materialize)
// переносит их в widget.ThemeStyle, откуда окно читает геометрию. Профиль,
// который их не объявляет, остаётся прежним: заголовок 32 px (24 у
// классики), квадратные кнопки управления.
const (
	// KeyWindowTitleBarHeight — высота заголовка окна, px. Классика Windows
	// 2000 — 18: полоса настоящей системы заметно ниже современной.
	// Не действует на явно заданный Window.TitleBarHeight и на режим вкладок
	// в заголовке: вкладкам нужна высота, которую даёт им окно.
	KeyWindowTitleBarHeight Key = "window.titlebar.height"

	// KeyWindowCaptionButtonW и KeyWindowCaptionButtonH — размер кнопки
	// управления окна (свернуть, развернуть, закрыть) в классике, px.
	// Windows 2000 — 16×14. Не заданы — кнопка квадратная, на 6 px ниже
	// заголовка.
	KeyWindowCaptionButtonW Key = "window.caption.button.w"
	KeyWindowCaptionButtonH Key = "window.caption.button.h"

	// KeyWindowCaptionIconSize — сторона значка окна в заголовке
	// (Window.SetIcon), px. Не задан — 16.
	KeyWindowCaptionIconSize Key = "window.caption.icon.size"

	// KeyWindowTitleGradient2 и KeyWindowTitleGradient2Inactive — вторая
	// точка градиента заголовка активного и неактивного окна. Первая — заливка
	// стиля window/titlebar. Не заданы (A=0) — заголовок сплошной.
	KeyWindowTitleGradient2         Key = "window.titlebar.gradient2"
	KeyWindowTitleGradient2Inactive Key = "window.titlebar.gradient2.inactive"
)
