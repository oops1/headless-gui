package keyboard

import (
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

func TestLockKeys_SetOps(t *testing.T) {
	k := CapsLock | ScrollLock
	if !k.Has(CapsLock) || !k.Has(ScrollLock) || !k.Has(CapsLock|ScrollLock) {
		t.Errorf("Has не видит своих флагов: %v", k)
	}
	if k.Has(NumLock) || k.Has(CapsLock|NumLock) {
		t.Errorf("Has видит чужой флаг: %v", k)
	}
	if k.Has(0) {
		t.Error("Has(0) должно быть false: пустой набор ничего не доказывает")
	}
	if k.Empty() || !LockKeys(0).Empty() {
		t.Error("Empty ошибается")
	}
	// Лишние биты вне известных флагов набор не заполняют.
	if !LockKeys(0x80).Empty() {
		t.Error("неизвестный бит сделал набор непустым")
	}
}

func TestLockKeys_String(t *testing.T) {
	for _, tc := range []struct {
		k    LockKeys
		want string
	}{
		{0, "-"},
		{CapsLock, "caps"},
		{NumLock | CapsLock, "caps|num"},
		{ScrollLock, "scroll"},
		{CapsLock | NumLock | ScrollLock | KanaLock, "caps|num|scroll|kana"},
		{KanaLock, "kana"},
	} {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("String(%d) = %q, ждал %q", tc.k, got, tc.want)
		}
	}
}

// Флаги независимы и умещаются в байт: приложение вправе хранить набор как
// число и сравнивать с ==.
func TestLockKeys_FlagValuesDistinct(t *testing.T) {
	seen := LockKeys(0)
	for _, f := range []LockKeys{CapsLock, NumLock, ScrollLock, KanaLock} {
		if f == 0 || seen&f != 0 || f&(f-1) != 0 {
			t.Fatalf("флаг %d пересекается с другими или не одиночный бит", f)
		}
		seen |= f
	}
	if seen != AllLocks {
		t.Errorf("AllLocks = %d, а флаги вместе дают %d", AllLocks, seen)
	}
}

func TestLockKeys_FromToggles(t *testing.T) {
	on := map[int]bool{native.VK_CAPITAL: true, native.VK_SCROLL: true}
	got := LocksFromToggles(func(vk int) bool { return on[vk] })
	if got != CapsLock|ScrollLock {
		t.Errorf("LocksFromToggles = %v, ждал caps|scroll", got)
	}
	if LocksFromToggles(func(int) bool { return false }) != 0 {
		t.Error("ничего не включено, а набор не пуст")
	}
	if LocksFromToggles(func(vk int) bool { return vk == native.VK_KANA }) != KanaLock {
		t.Error("VK_KANA не стал KanaLock")
	}
	if LocksFromToggles(func(vk int) bool { return vk == native.VK_NUMLOCK }) != NumLock {
		t.Error("VK_NUMLOCK не стал NumLock")
	}
}

// Нажимаются только клавиши отличающихся флагов, в порядке Caps, Num, Scroll,
// Kana; совпадающие не трогаются.
func TestLockKeys_ToggleKeys(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cur, want LockKeys
		vks       []int
	}{
		{"одинаково", CapsLock | NumLock, CapsLock | NumLock, nil},
		{"включить Num", 0, NumLock, []int{native.VK_NUMLOCK}},
		{"Num не трогает Caps", CapsLock, CapsLock | NumLock, []int{native.VK_NUMLOCK}},
		{"выключить Caps", CapsLock | NumLock, NumLock, []int{native.VK_CAPITAL}},
		{"заменить Caps на Scroll", CapsLock, ScrollLock, []int{native.VK_CAPITAL, native.VK_SCROLL}},
		{"все", 0, CapsLock | NumLock | ScrollLock | KanaLock, []int{native.VK_CAPITAL, native.VK_NUMLOCK, native.VK_SCROLL, native.VK_KANA}},
		{"все выключить", CapsLock | NumLock | ScrollLock | KanaLock, 0, []int{native.VK_CAPITAL, native.VK_NUMLOCK, native.VK_SCROLL, native.VK_KANA}},
	} {
		if got := LockToggleKeys(tc.cur, tc.want); !reflect.DeepEqual(got, tc.vks) {
			t.Errorf("%s: нажать %v, ждал %v", tc.name, got, tc.vks)
		}
	}
}

func TestLockKeys_IsLockVK(t *testing.T) {
	for _, vk := range []int{native.VK_CAPITAL, native.VK_NUMLOCK, native.VK_SCROLL, native.VK_KANA} {
		if !IsLockVK(vk) {
			t.Errorf("vk %#x не признан переключателем", vk)
		}
	}
	for _, vk := range []int{native.VK_A, native.VK_SHIFT, native.VK_CONTROL, native.VK_PAUSE, 0} {
		if IsLockVK(vk) {
			t.Errorf("vk %#x признан переключателем", vk)
		}
	}
}
