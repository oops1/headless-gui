package desktop

// Хэши кадров, снятые до работы над панелью Windows 11 (см. taskbar_frozen_test.go).
var frozenHashes = map[string]string{
	"Windows2000 light=false @1":      "11b519e006ea338c",
	"Windows2000 light=false @2":      "59a317245f588668",
	"Windows2000 Blue light=false @1": "540a8e9f65f77375",
	"Windows2000 Blue light=false @2": "902a79afd3e50ceb",
	"Windows10 light=false @1":        "01aec744db276ab6",
	"Windows10 light=false @2":        "d0a56381a5062356",
	"Windows10 light=true @1":         "b3bfdeef4602e131",
	"Windows10 light=true @2":         "8f804af1483d72f7",
	"Windows10 Dark light=false @1":   "34e205db33f26161",
	"Windows10 Dark light=false @2":   "a57f4d1f5fbfecb0",
	"macOS light=false @1":            "27b25830d556d3db",
	"macOS light=false @2":            "94dd8a1549704c64",
	"macOS Dark light=false @1":       "e2e3a696210d7374",
	"macOS Dark light=false @2":       "cb7075f9f76fd073",
}

// frozenHashesFMA — те же кадры в сборке, где компилятор сливает a*b+c в одну
// FMA-инструкцию (arm64 — macOS на GitHub Actions; amd64 при GOAMD64=v3).
// Округление FMA другое, и на масштабе 2 младшие биты сглаживания значков
// отличаются. Здесь только расходящиеся ключи, остальные берутся из
// frozenHashes. Сняты с GOAMD64=v3 и совпали с кадрами CI на macOS arm64.
var frozenHashesFMA = map[string]string{
	"Windows2000 light=false @2":      "19eb9cd1fc81dd5e",
	"Windows2000 Blue light=false @2": "7bc41ceda88709a1",
	"Windows10 light=false @2":        "39bd078fe56ce743",
	"Windows10 light=true @2":         "9c781242aef22070",
	"Windows10 Dark light=false @2":   "2c2d8bb365f7ac6c",
}
