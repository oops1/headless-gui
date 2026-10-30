package contracttest

import "time"

// Poll — Env.Pump для бэкенда, который сам читает события окна в своей
// горутине (насос на соединение окна): ждёт cond, проверяя его раз в 5 мс.
func Poll(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			return cond()
		}
		time.Sleep(5 * time.Millisecond)
	}
	return true
}
