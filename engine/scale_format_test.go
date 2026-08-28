package engine

import (
	"testing"
)

// Смена разрешения сохраняет настройки движка, но не чужой буфер.
//
// Клон холста наследует то, что задавало приложение: пропуск поддеревьев и
// порядок каналов СВОЕГО буфера. Буфер потребителя, отданный через
// SetSurface, при этом не переезжает — он размера прежнего экрана, и его
// порядок каналов относится только к нему. Наследовать его формат означало
// бы кодировать собственную память чужим порядком: кадр уехал бы с
// переставленными красным и синим.
func TestSetResolution_KeepsOwnFormatNotForeignOne(t *testing.T) {
	e := New(320, 240, 60)

	e.SetSubtreeCulling(false)
	if err := e.SetPixelFormat(FormatRGBA); err != nil {
		t.Fatal(err)
	}

	// Потребитель отдал свою память в другом порядке каналов.
	pix := make([]byte, 320*240*4)
	if err := e.SetSurface(pix, 320*4, FormatBGRX); err != nil {
		t.Fatal(err)
	}
	if got := e.PixelFormat(); got != FormatBGRX {
		t.Fatalf("до смены разрешения формат %v, ждали формат чужого буфера", got)
	}

	e.SetResolution(640, 480)

	if got := e.PixelFormat(); got != FormatRGBA {
		t.Errorf("после смены разрешения формат %v — движок рисует в свою "+
			"память порядком каналов чужой", got)
	}
	if e.SubtreeCulling() {
		t.Error("смена разрешения молча вернула пропуск поддеревьев")
	}
}
