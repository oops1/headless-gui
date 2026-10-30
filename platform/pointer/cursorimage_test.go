package pointer

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

func TestCursorLRU_EvictsOldestAndFrees(t *testing.T) {
	var freed []int
	c := NewCursorLRU[int](3, func(v int) { freed = append(freed, v) }, nil)
	for i := 1; i <= 3; i++ {
		c.Put(ciKey(i), i*10)
	}
	c.Get(ciKey(1)) // 1 стал свежим, самый давний теперь 2
	c.Put(ciKey(4), 40)
	if !reflect.DeepEqual(freed, []int{20}) {
		t.Fatalf("освобождены %v, ждал [20] (самый давний)", freed)
	}
	if _, ok := c.Get(ciKey(2)); ok {
		t.Error("вытесненный курсор остался в кэше")
	}
	for _, i := range []int{1, 3, 4} {
		if v, ok := c.Get(ciKey(i)); !ok || v != i*10 {
			t.Errorf("ключ %d: %v %v", i, v, ok)
		}
	}
	if c.Len() != 3 {
		t.Errorf("в кэше %d записей, ждал 3", c.Len())
	}
}

// Показываемый сейчас курсор не вытесняется, даже будучи самым давним.
func TestCursorLRU_KeepsShownCursor(t *testing.T) {
	var freed []int
	shown := 10
	c := NewCursorLRU[int](2, func(v int) { freed = append(freed, v) }, func(v int) bool { return v == shown })
	c.Put(ciKey(1), 10)
	c.Put(ciKey(2), 20)
	c.Put(ciKey(3), 30) // самый давний — 10, но он показан: уходит 20
	if !reflect.DeepEqual(freed, []int{20}) {
		t.Fatalf("освобождены %v, ждал [20]", freed)
	}
	if _, ok := c.Get(ciKey(1)); !ok {
		t.Error("показанный курсор вытеснен")
	}
	shown = 99 // показ сменился — 10 снова обычная запись
	c.Put(ciKey(4), 40)
	c.Put(ciKey(5), 50)
	if !reflect.DeepEqual(freed, []int{20, 30, 10}) {
		t.Errorf("после смены показа освобождены %v, ждал [20 30 10]", freed)
	}
}

func TestCursorLRU_ClearFreesAll(t *testing.T) {
	n := 0
	c := NewCursorLRU[int](CursorCacheSize, func(int) { n++ }, nil)
	for i := 0; i < CursorCacheSize; i++ {
		c.Put(ciKey(i), i)
	}
	c.Clear()
	if n != CursorCacheSize || c.Len() != 0 {
		t.Errorf("clear: освобождено %d из %d, осталось %d", n, CursorCacheSize, c.Len())
	}
}

func TestCursorPremul(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 255}) // непрозрачный — без изменений
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 0, B: 128, A: 128})  // половина
	img.SetNRGBA(2, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 0})  // прозрачный → нули
	ci := base.NewCursorImage(img, 0, 0)

	rgba := CursorPremulRGBA(ci)
	want := []byte{200, 100, 50, 255, 128, 0, 64, 128, 0, 0, 0, 0}
	if !reflect.DeepEqual(rgba.Pix, want) {
		t.Errorf("RGBA домноженный: %v, ждал %v", rgba.Pix, want)
	}
	bgra := CursorPremulBGRA(ci)
	wantB := []byte{50, 100, 200, 255, 64, 0, 128, 128, 0, 0, 0, 0}
	if !reflect.DeepEqual(bgra, wantB) {
		t.Errorf("BGRA домноженный: %v, ждал %v", bgra, wantB)
	}
}

func TestCursorMonoPoints(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	img.SetNRGBA(0, 0, color.NRGBA{A: 255})                         // чёрный непрозрачный: виден, рисунок
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255}) // белый: виден, фон
	img.SetNRGBA(2, 0, color.NRGBA{A: 100})                         // ниже порога: нет
	src, mask := CursorMonoPoints(base.NewCursorImage(img, 0, 0), 128)
	if !reflect.DeepEqual(mask, [][2]int16{{0, 0}, {1, 0}}) {
		t.Errorf("маска %v", mask)
	}
	if !reflect.DeepEqual(src, [][2]int16{{0, 0}}) {
		t.Errorf("рисунок %v", src)
	}
}

func TestCursorScaledHotspot(t *testing.T) {
	for _, tc := range []struct{ hot, scale, want int }{
		{16, 1, 16}, {16, 2, 8}, {17, 2, 8}, {5, 0, 5}, {0, 3, 0},
	} {
		if got := CursorScaledHotspot(tc.hot, tc.scale); got != tc.want {
			t.Errorf("hot=%d scale=%d: %d, ждал %d", tc.hot, tc.scale, got, tc.want)
		}
	}
}

func ciKey(i int) [32]byte {
	var k [32]byte
	k[0] = byte(i)
	return k
}
