// Package goid сообщает идентификатор текущей горутины.
//
// Рантайм его не публикует, а движку он нужен, чтобы узнавать свою горутину:
// Flush изнутри цикла не должен ждать сам себя, а анимация — шагать на том
// цикле, на горутине которого её завели (GG-68). Идентификатор берётся из
// первой строки runtime.Stack: «goroutine 17 [running]:».
package goid

import "runtime"

// Current возвращает идентификатор текущей горутины.
func Current() uint64 {
	// «goroutine » + 20 цифр uint64 + « [» — ровно 32 байта.
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	const prefix = len("goroutine ")
	if n <= prefix {
		return 0
	}
	var id uint64
	for _, c := range buf[prefix:n] {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}
