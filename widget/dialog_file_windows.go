//go:build windows

package widget

import (
	"path/filepath"
	"syscall"
)

// isHiddenFSEntry сообщает, скрыт ли элемент каталога средствами ОС.
// Windows: атрибуты Hidden/System (ntuser.dat, junction-папки профиля и т.п. —
// как в проводнике). Ошибка чтения атрибутов трактуется как «не скрыт».
func isHiddenFSEntry(dir, name string) bool {
	p, err := syscall.UTF16PtrFromString(filepath.Join(dir, name))
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return false
	}
	return attrs&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0
}
