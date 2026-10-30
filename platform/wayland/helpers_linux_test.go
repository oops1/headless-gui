//go:build linux && !android

package wayland

// helpers_linux_test.go — копии помощников тестов window/core: тестовые
// помощники между пакетами не делятся.

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// wheelCall — сырое или пиксельное колесо, как его видит приложение (тесты
// колеса Wayland).
type wheelCall struct {
	x, y    int
	dx, dy  float64
	precise bool
}
