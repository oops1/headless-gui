//go:build !windows

package core

// AllowForeground на этой платформе не имеет аналога и всегда возвращает false.
//
// Право вывести окно на передний план здесь доказывается токеном активации,
// который запустивший процесс кладёт в окружение второго процесса
// (TakeActivationToken), а второй процесс передаёт первому строкой и тот зовёт
// Window.ActivateWithToken. Win32 же выдаёт право явно, процессом
// (AllowSetForegroundWindow).
func AllowForeground(pid int) bool { return false }
