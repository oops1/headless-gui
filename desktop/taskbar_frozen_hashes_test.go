package desktop

// Хэши кадров, снятые до работы над панелью Windows 11 (см. taskbar_frozen_test.go).
//
// v3.38, по замечанию WinLine после v3.32.0 п.1: хэши Windows 2000 (обе) и
// macOS (обе) обновлены осознанно. Глиф значка трея раньше рисовался внутри
// PadY стиля и уменьшался (у Windows 2000 14 px -> 6, у macOS 16 -> 4); теперь
// он размером tray.icon.size. Попиксельным сравнением старого и нового кадров
// подтверждено: изменилась только область значков трея и кнопки уведомлений,
// остальное, включая все кадры Windows 10, совпадает побайтно.
var frozenHashes = map[string]string{
	"Windows2000 light=false @1":      "dd7b2c2184f0682c",
	"Windows2000 light=false @2":      "140710cbf19415ff",
	"Windows2000 Blue light=false @1": "7d259db8f6185a03",
	"Windows2000 Blue light=false @2": "a58b78c3bef061c9",
	"Windows10 light=false @1":        "01aec744db276ab6",
	"Windows10 light=false @2":        "d0a56381a5062356",
	"Windows10 light=true @1":         "b3bfdeef4602e131",
	"Windows10 light=true @2":         "8f804af1483d72f7",
	"Windows10 Dark light=false @1":   "34e205db33f26161",
	"Windows10 Dark light=false @2":   "a57f4d1f5fbfecb0",
	"macOS light=false @1":            "656ea90969e943af",
	"macOS light=false @2":            "61742e7c8e00660d",
	"macOS Dark light=false @1":       "67c3f45a71940d04",
	"macOS Dark light=false @2":       "a35133a1306d6b2b",
}

// frozenHashesFMA — те же кадры в сборке, где компилятор сливает a*b+c в одну
// FMA-инструкцию (arm64 — macOS на GitHub Actions; amd64 при GOAMD64=v3).
// Округление FMA другое, и на масштабе 2 младшие биты сглаживания значков
// отличаются. Здесь только расходящиеся ключи, остальные берутся из
// frozenHashes. Сняты с GOAMD64=v3 и совпали с кадрами CI на macOS arm64.
var frozenHashesFMA = map[string]string{
	"Windows2000 light=false @2":      "c67387a801106c07",
	"Windows2000 Blue light=false @2": "bb3311fa7efaa623",
	"Windows10 light=false @2":        "39bd078fe56ce743",
	"Windows10 light=true @2":         "9c781242aef22070",
	"Windows10 Dark light=false @2":   "2c2d8bb365f7ac6c",
}
