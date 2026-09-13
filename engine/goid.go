package engine

import (
	"bytes"
	"runtime"
	"strconv"
)

// curGoroutineID — номер текущей горутины.
//
// Язык его не отдаёт намеренно, и логику на нём строить нельзя. Здесь он нужен
// ровно для одного: понять, что Flush зовут из горутины самого движка, — ждать
// там завершения собственной очереди значило бы повиснуть навсегда. Стоит
// около микросекунды, поэтому на горячем пути не используется.
func curGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	b := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	if i := bytes.IndexByte(b, ' '); i > 0 {
		b = b[:i]
	}
	id, _ := strconv.ParseUint(string(b), 10, 64)
	return id
}
