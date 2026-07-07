//go:build !windows

package widget

// isHiddenFSEntry: вне Windows скрытость определяется точкой в начале имени
// (проверяется в readDirEntries), отдельных атрибутов нет.
func isHiddenFSEntry(dir, name string) bool { return false }
