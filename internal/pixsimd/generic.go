package pixsimd

// Скалярные версии — эталон. Формулы перенесены из движка без изменений
// (Canvas.drawAlphaMask, Canvas.fillRectRaw, window.swapRBRow): векторная версия
// обязана давать тот же результат бит в бит, и сравнивается она именно с ними.

const m16 = 1<<16 - 1

func blendMaskRowGeneric(dst, mask []byte, sr, sg, sb, sa uint32) {
	for i, m := range mask {
		ma := uint32(m)
		if ma == 0 {
			continue
		}
		ma |= ma << 8 // 0..0xffff
		a := sa * ma / m16
		inv := m16 - a
		p := dst[i*4 : i*4+4 : i*4+4]
		p[0] = uint8((uint32(p[0])*0x101*inv/m16 + sr*ma/m16) >> 8)
		p[1] = uint8((uint32(p[1])*0x101*inv/m16 + sg*ma/m16) >> 8)
		p[2] = uint8((uint32(p[2])*0x101*inv/m16 + sb*ma/m16) >> 8)
		p[3] = uint8((uint32(p[3])*0x101*inv/m16 + sa*ma/m16) >> 8)
	}
}

func overSolidRowGeneric(row []byte, a, sr, sg, sb, sa uint32) {
	for x := 0; x < len(row); x += 4 {
		row[x+0] = uint8((uint32(row[x+0])*a/m16 + sr) >> 8)
		row[x+1] = uint8((uint32(row[x+1])*a/m16 + sg) >> 8)
		row[x+2] = uint8((uint32(row[x+2])*a/m16 + sb) >> 8)
		row[x+3] = uint8((uint32(row[x+3])*a/m16 + sa) >> 8)
	}
}

func swapRBGeneric(dst, src []byte) {
	for i := 0; i < len(src); i += 4 {
		s := src[i : i+4 : i+4]
		v := uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24
		u := (v & 0xFF00FF00) | (v&0x000000FF)<<16 | (v>>16)&0x000000FF
		d := dst[i : i+4 : i+4]
		d[0] = byte(u)
		d[1] = byte(u >> 8)
		d[2] = byte(u >> 16)
		d[3] = byte(u >> 24)
	}
}

func swapRBOpaqueGeneric(dst, src []byte) {
	for i := 0; i < len(src); i += 4 {
		s := src[i : i+4 : i+4]
		d := dst[i : i+4 : i+4]
		d[0], d[1], d[2], d[3] = s[2], s[1], s[0], 0xFF
	}
}
