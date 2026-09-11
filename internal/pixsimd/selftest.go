package pixsimd

import (
	"bytes"
	"fmt"
)

// selfTest прогоняет каждое ядро набора cand на заготовленных данных и
// сравнивает со скалярным. Ошибка — первое расхождение или паника ядра.
//
// Данные подобраны под края, на которых ломаются векторные версии: длины
// меньше ширины вектора и с хвостом, маска 0 и 255, цвет без premultiply
// (сумма каналов переваливает за 255), область размытия с остатками по
// строкам и столбцам и радиус больше самой области. Прогон — микросекунды:
// при запуске программы его не заметно.
func selfTest(cand kernels) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("самопроверка: паника: %v", r)
		}
	}()
	g := &lcg{s: 0x9E3779B9}

	for _, n := range []int{1, 7, 8, 9, 16, 31, 67} {
		mask := g.bytes(n)
		for i := range mask {
			switch i % 5 {
			case 0:
				mask[i] = 0
			case 1:
				mask[i] = 255
			}
		}
		for _, c := range [][4]uint32{{0, 120, 215, 255}, {255, 255, 255, 40}, {200, 10, 90, 128}} {
			sr, sg, sb, sa := c[0]*0x101, c[1]*0x101, c[2]*0x101, c[3]*0x101
			dst := g.bytes(n * 4)
			want := bytes.Clone(dst)
			generic.blendMaskRow(want, mask, sr, sg, sb, sa)
			cand.blendMaskRow(dst, mask, sr, sg, sb, sa)
			if !bytes.Equal(dst, want) {
				return fmt.Errorf("самопроверка: blendMaskRow, длина %d", n)
			}

			a := (m16 - sa) * 0x101
			row := g.bytes(n * 4)
			want = bytes.Clone(row)
			generic.overSolidRow(want, a, sr, sg, sb, sa)
			cand.overSolidRow(row, a, sr, sg, sb, sa)
			if !bytes.Equal(row, want) {
				return fmt.Errorf("самопроверка: overSolidRow, длина %d", n)
			}
		}

		src := g.bytes(n * 4)
		got, want := make([]byte, n*4), make([]byte, n*4)
		generic.swapRB(want, src)
		cand.swapRB(got, src)
		if !bytes.Equal(got, want) {
			return fmt.Errorf("самопроверка: swapRB, длина %d", n)
		}
		generic.swapRBOpaque(want, src)
		cand.swapRBOpaque(got, src)
		if !bytes.Equal(got, want) {
			return fmt.Errorf("самопроверка: swapRBOpaque, длина %d", n)
		}
	}

	// Область 21×13 внутри буфера пошире: пиксели вокруг обязаны остаться
	// нетронутыми, поэтому сравнивается весь буфер, а не только область.
	const w, h, stride = 21, 13, 25 * 4
	base := g.bytes((h+2)*stride + 8)
	var bbG, bbC BlurBuf
	for _, radius := range []int{1, 3, 9, 20} {
		for _, pass := range []struct {
			name     string
			ref, got func(*BlurBuf, []byte, int, int, int, int)
		}{
			{"blurRows", generic.blurRows, cand.blurRows},
			{"blurCols", generic.blurCols, cand.blurCols},
		} {
			want := bytes.Clone(base)
			got := bytes.Clone(base)
			pass.ref(&bbG, want[stride+4:], stride, w, h, radius)
			pass.got(&bbC, got[stride+4:], stride, w, h, radius)
			if !bytes.Equal(got, want) {
				return fmt.Errorf("самопроверка: %s, радиус %d", pass.name, radius)
			}
		}
	}
	return nil
}

// lcg — детерминированный генератор для самопроверки: одинаковые данные при
// каждом запуске, без зависимостей и без выделения памяти на состояние.
type lcg struct{ s uint32 }

func (g *lcg) bytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		g.s = g.s*1664525 + 1013904223
		b[i] = byte(g.s >> 24)
	}
	return b
}
