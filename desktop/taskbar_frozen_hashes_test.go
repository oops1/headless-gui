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
//
// v4.5: все хэши обновлены осознанно — привязка вертикальных штрихов
// (HintingX) больше не округляет толщину ствола, далёкую от целой (Go.Git
// GG-98), меняются те же подписи и часы.
var frozenHashes = map[string]string{
	"Windows2000 light=false @1":      "ff2ea71c8bc23612",
	"Windows2000 light=false @2":      "4b9845847bcb113f",
	"Windows2000 Blue light=false @1": "b7ba6854874d05e2",
	"Windows2000 Blue light=false @2": "100ac3d4f1f47ba2",
	"Windows10 light=false @1":        "c769c79513e97029",
	"Windows10 light=false @2":        "6de2a24be3a47131",
	"Windows10 light=true @1":         "5e1a99f731b4b16c",
	"Windows10 light=true @2":         "de04a15745d7bb41",
	"Windows10 Dark light=false @1":   "b96562d4807354ff",
	"Windows10 Dark light=false @2":   "b5f5ebc5899069e4",
	"macOS light=false @1":            "35f287e5ab230767",
	"macOS light=false @2":            "703791f69b74dbdc",
	"macOS Dark light=false @1":       "e00d6216566cb5e8",
	"macOS Dark light=false @2":       "4ed2bf937069a8dd",
}

// frozenHashesFMA — те же кадры в сборке, где компилятор сливает a*b+c в одну
// FMA-инструкцию (arm64 — macOS на GitHub Actions; amd64 при GOAMD64=v3).
// Округление FMA другое, и младшие биты сглаживания значков и глифов
// отличаются. Здесь только расходящиеся ключи, остальные берутся из
// frozenHashes. Сняты с GOAMD64=v3 (v4.5: FROZEN_PRINT=1 GOAMD64=v3).
var frozenHashesFMA = map[string]string{
	"Windows10 light=false @2":      "1ff56bd071d3593a",
	"Windows10 light=true @2":       "3b88efeb41697d26",
	"Windows10 Dark light=false @2": "993811d6ddee08b1",
}
