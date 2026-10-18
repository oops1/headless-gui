package output

import "testing"

func TestSlab_ReleaseReturnsOnceAndPinKeeps(t *testing.T) {
	s := GetSlab(1000)
	if len(s.Bytes()) != 0 || cap(s.Bytes()) < 1000 {
		t.Fatalf("буфер len=%d cap=%d", len(s.Bytes()), cap(s.Bytes()))
	}
	var f Frame
	f.AttachSlabs(s)
	f.Release()
	if s.retired.Load() != 1 {
		t.Fatal("Release не вернул буфер")
	}
	// Повторный Release по копии не кладёт буфер в пул второй раз.
	cp := f
	cp.Release()

	p := GetSlab(1000)
	var g Frame
	g.AttachSlabs(p)
	g.Pin()
	g.Release()
	// Закреплённый буфер не возвращается: следующий Get не получит его же.
	for i := 0; i < 50; i++ {
		if q := GetSlab(1000); q == p {
			t.Fatal("закреплённый буфер вернулся в пул")
		}
	}
}

func TestSlab_PoolClassesAndSize(t *testing.T) {
	big := GetSlab(slabSmall + 1)
	if cap(big.Bytes())%slabGranule != 0 || cap(big.Bytes()) <= slabSmall {
		t.Errorf("крупный буфер: cap=%d", cap(big.Bytes()))
	}
	// Освобождённый мелкий буфер не выдаётся на запрос крупнее его ёмкости.
	small := GetSlab(100)
	var f Frame
	f.AttachSlabs(small)
	f.Release()
	got := GetSlab(slabSmall / 2)
	if cap(got.Bytes()) < slabSmall/2 {
		t.Errorf("выдан буфер меньше запрошенного: %d", cap(got.Bytes()))
	}
	// Frame без буферов: Release/Pin безопасны.
	var z Frame
	z.Release()
	z.Pin()
}
