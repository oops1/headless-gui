package core

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"sync"
	"testing"
)

// hugePNGHeader — PNG, у которого заявлен размер w×h, а данных нет: проверка
// защиты от картинок-бомб.
func hugePNGHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6 // 8 бит, RGBA
	chunk := append([]byte("IHDR"), ihdr...)
	binary.Write(&b, binary.BigEndian, uint32(13))
	b.Write(chunk)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

// Владеемое изображение — снимок: правка оригинала после копирования в него не
// попадает; PNG кодируется один раз и только по обращению.
func TestOwnedImage_SnapshotAndLazyPNG(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 5, 8, 7)) // начало не в нуле
	src.SetNRGBA(5, 5, color.NRGBA{10, 20, 30, 40})
	o := newOwnedImage(src)
	if o.png != nil {
		t.Fatal("PNG закодирован при создании")
	}
	src.SetNRGBA(5, 5, color.NRGBA{0, 0, 0, 255})

	var wg sync.WaitGroup
	results := make([][]byte, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = o.PNG()
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(results); i++ {
		if len(results[i]) == 0 || &results[i][0] != &results[0][0] {
			t.Fatal("PNG кодируется заново для каждого обращения")
		}
	}
	img := o.Image()
	if img.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Errorf("границы %v: ждал начало в (0,0)", img.Bounds())
	}
	if c := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); c != (color.NRGBA{10, 20, 30, 40}) {
		t.Errorf("снимок %v: правка оригинала просочилась", c)
	}
}
