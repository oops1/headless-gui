package keyboard

// lockkeys.go — набор клавиш-переключателей LockKeys и помощники.

import (
	"strings"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// LockKeys — набор включённых клавиш-переключателей.
type LockKeys uint8

const (
	// CapsLock — включён Caps Lock.
	CapsLock LockKeys = 1 << iota
	// NumLock — включён Num Lock.
	NumLock
	// ScrollLock — включён Scroll Lock.
	ScrollLock
	// KanaLock — включён Kana Lock (японская клавиатура).
	KanaLock
)

// AllLocks — все известные клавиши-переключатели.
//
//facadegen:skip
const AllLocks = CapsLock | NumLock | ScrollLock | KanaLock

// Has сообщает, что в наборе включены ВСЕ перечисленные клавиши.
func (k LockKeys) Has(f LockKeys) bool { return f != 0 && k&f == f }

// Empty — ни один переключатель не включён.
func (k LockKeys) Empty() bool { return k&AllLocks == 0 }

// String — для логов и сообщений: «caps|num», «-» для пустого набора.
func (k LockKeys) String() string {
	var parts []string
	for _, n := range []struct {
		f    LockKeys
		name string
	}{
		{CapsLock, "caps"}, {NumLock, "num"}, {ScrollLock, "scroll"}, {KanaLock, "kana"},
	} {
		if k&n.f != 0 {
			parts = append(parts, n.name)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "|")
}

// lkKeyTable — соответствие флагов клавишам Windows (VK_*): по ним читается
// состояние и ими же переключается на Win32.
var lkKeyTable = [...]struct {
	flag LockKeys
	vk   int
}{
	{CapsLock, native.VK_CAPITAL}, {NumLock, native.VK_NUMLOCK}, {ScrollLock, native.VK_SCROLL}, {KanaLock, native.VK_KANA},
}

// LocksFromToggles собирает набор из состояния каждой клавиши: on(vk) — включена
// ли клавиша-переключатель с этим кодом.
//
//facadegen:skip
func LocksFromToggles(on func(vk int) bool) LockKeys {
	var k LockKeys
	for _, e := range lkKeyTable {
		if on(e.vk) {
			k |= e.flag
		}
	}
	return k
}

// LockToggleKeys — клавиши, которые нужно нажать, чтобы из набора cur получить
// want: по одной на каждый отличающийся флаг, в порядке Caps, Num, Scroll, Kana.
//
//facadegen:skip
func LockToggleKeys(cur, want LockKeys) []int {
	var vks []int
	for _, e := range lkKeyTable {
		if (cur^want)&e.flag != 0 {
			vks = append(vks, e.vk)
		}
	}
	return vks
}

// IsLockVK — код клавиши-переключателя.
//
//facadegen:skip
func IsLockVK(vk int) bool {
	for _, e := range lkKeyTable {
		if e.vk == vk {
			return true
		}
	}
	return false
}
