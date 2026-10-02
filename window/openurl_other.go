//go:build (!windows && !linux && !darwin) || android

// openurl_other.go — платформы без системного способа открыть ссылку или файл
// (Android, BSD и прочее). Публичные функции возвращают ErrOpenUnsupported:
// приложение может показать «не поддержано» вместо молчаливого бездействия.
package window

func platformOpenURL(string) error { return ErrOpenUnsupported }

func platformOpenFile(string) error { return ErrOpenUnsupported }

func platformRevealFile(string) error { return ErrOpenUnsupported }
