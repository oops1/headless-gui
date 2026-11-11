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
	"Windows2000 light=false @1":      "8b86b2dfeb11664b",
	"Windows2000 light=false @2":      "d5134cfb956edff4",
	"Windows2000 Blue light=false @1": "5349517d72bb9c71",
	"Windows2000 Blue light=false @2": "e0420a79a8115610",
	"Windows10 light=false @1":        "fc0c69cdb6a358dc",
	"Windows10 light=false @2":        "c0a0edfe6160757e",
	"Windows10 light=true @1":         "969d062d68bf642f",
	"Windows10 light=true @2":         "62c4f4fda72f29c4",
	"Windows10 Dark light=false @1":   "2abdafb28b6094e0",
	"Windows10 Dark light=false @2":   "6c4db9ba9f27bacb",
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
	"Windows2000 light=false @1":      "972a1464ac882706",
	"Windows2000 Blue light=false @1": "5f28ba683bd396a3",
	"Windows10 light=false @2":        "a511304183304754",
	"Windows10 light=true @2":         "79dfa0693ee1e100",
	"Windows10 Dark light=false @2":   "e0ff78535a98339c",
}
