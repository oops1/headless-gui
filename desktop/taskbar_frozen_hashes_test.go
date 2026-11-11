package desktop

// Хэши кадров, снятые до работы над панелью Windows 11 (см. taskbar_frozen_test.go).
//
// v3.38, по замечанию WinLine после v3.32.0 п.1: хэши Windows 2000 (обе) и
// macOS (обе) обновлены осознанно. Глиф значка трея раньше рисовался внутри
// PadY стиля и уменьшался (у Windows 2000 14 px -> 6, у macOS 16 -> 4); теперь
// он размером tray.icon.size. Попиксельным сравнением старого и нового кадров
// подтверждено: изменилась только область значков трея и кнопки уведомлений,
// остальное, включая все кадры Windows 10, совпадает побайтно.
//
// v4.4: все хэши обновлены осознанно — движок по умолчанию рисует текст с
// полным слоем качества (engine.DefaultTextQuality: хинтинг по обеим осям и
// контраст), меняются подписи панели и часы. FROZEN_PRINT=1 печатает новые.
var frozenHashes = map[string]string{
	"Windows2000 light=false @1":      "f705a4a4b335fe2f",
	"Windows2000 light=false @2":      "df1686c42f8545a5",
	"Windows2000 Blue light=false @1": "36809dc1928b4a60",
	"Windows2000 Blue light=false @2": "1d3928fda6f266df",
	"Windows10 light=false @1":        "dea0e8cf05a1b729",
	"Windows10 light=false @2":        "30170b403578eac3",
	"Windows10 light=true @1":         "11eef40a3cf5d37e",
	"Windows10 light=true @2":         "a1b7e8369c72e653",
	"Windows10 Dark light=false @1":   "6fb292fa51b51794",
	"Windows10 Dark light=false @2":   "6a1b074b4f0a00a2",
	"macOS light=false @1":            "ec0a39a3be0c26cc",
	"macOS light=false @2":            "a20e84b6f8d1330e",
	"macOS Dark light=false @1":       "247051b34f427dbf",
	"macOS Dark light=false @2":       "445e80e0c895aef5",
}

// frozenHashesFMA — те же кадры в сборке, где компилятор сливает a*b+c в одну
// FMA-инструкцию (arm64 — macOS на GitHub Actions; amd64 при GOAMD64=v3).
// Округление FMA другое, и младшие биты сглаживания значков и глифов
// отличаются. Здесь только расходящиеся ключи, остальные берутся из
// frozenHashes. Сняты с GOAMD64=v3 (v4.4: FROZEN_PRINT=1 GOAMD64=v3).
var frozenHashesFMA = map[string]string{
	"Windows2000 light=false @1":      "ff80227a5c8fc23b",
	"Windows2000 Blue light=false @1": "a2d9e0d9a97a6d23",
	"Windows10 light=false @2":        "87c28559060a54c2",
	"Windows10 light=true @2":         "5a3d9e78962a7983",
	"Windows10 Dark light=false @2":   "ee83760178ca66be",
}
