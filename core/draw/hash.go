package draw

// hash.go — хэш команды и обход команд, задевающих область (ADR-0005).
//
// Хэш нужен кэшу размытой подложки: ключ слоя складывается из команды слоя
// и команд, нарисованных до неё в области чтения; хэш-дифф тайлов (4а.3)
// переиспользует его. Хэш — по аргументам команды и её клипу; картинка — по
// адресу значения (содержимое картинки сменяется вместе с журналом сбросов
// ImageInvalidateCache, который вызывающий подмешивает отдельно).

import (
	"image"
	"math"
	"reflect"
)

const (
	hashOffset = 14695981039346656037
	hashPrime  = 1099511628211
)

// mix подмешивает v в h (FNV-1a по байтам 64-битного слова).
func mix(h, v uint64) uint64 {
	for i := 0; i < 8; i++ {
		h = (h ^ (v & 0xFF)) * hashPrime
		v >>= 8
	}
	return h
}

func mixInt(h uint64, v int) uint64       { return mix(h, uint64(int64(v))) }
func mixFloat(h uint64, v float64) uint64 { return mix(h, math.Float64bits(v)) }
func mixRect(h uint64, r image.Rectangle) uint64 {
	return mixInt(mixInt(mixInt(mixInt(h, r.Min.X), r.Min.Y), r.Max.X), r.Max.Y)
}
func mixString(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h = (h ^ uint64(s[i])) * hashPrime
	}
	return mix(h, uint64(len(s)))
}

// Mix подмешивает слово v в хэш h.
func Mix(h, v uint64) uint64 { return mix(h, v) }

// PointerOf — адрес значения за интерфейсом (картинка, буфер); 0 — не
// указатель. Идентичность картинки для ключей кэшей.
func PointerOf(v any) uintptr { return uintptr(pointerOf(v)) }

// pointerOf — адрес значения за интерфейсом (картинка, буфер); 0 — не
// указатель.
func pointerOf(v any) uint64 {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Func, reflect.UnsafePointer:
		return uint64(rv.Pointer())
	}
	return 0
}

// Hash подмешивает команду в h: код операции, аргументы, клип. Ссылка на
// список ребёнка (OpChild) подмешивает хэш его текущего списка, группа
// прозрачности — хэш вложенного списка.
func (c *Cmd) Hash(h uint64) uint64 {
	h = mix(h, uint64(c.Op))
	if c.Flag {
		h = mix(h, 1)
	}
	h = mixInt(mixInt(mixInt(mixInt(mixInt(h, int(c.X)), int(c.Y)), int(c.W)), int(c.H)), int(c.R))
	h = mixInt(mixInt(h, int(c.I1)), int(c.I2))
	h = mixFloat(mixFloat(h, c.F[0]), c.F[1])
	h = mix(h, uint64(c.Col.R)|uint64(c.Col.G)<<8|uint64(c.Col.B)<<16|uint64(c.Col.A)<<24)
	h = mixString(mixString(h, c.S), c.Font)
	if c.Clip != nil {
		s := c.Clip
		if s.NoClip {
			h = mix(h, 1)
		} else {
			h = mixRect(h, s.Clip)
		}
		h = mixInt(mixRect(h, s.Round), s.RoundR)
	}
	switch c.Op {
	case OpChild:
		if c.Holder != nil {
			h = c.Holder.CmdList().Hash(h)
		}
	case OpDirect:
		h = mix(h, pointerOf(c.Holder))
	}
	if e := c.Ext; e != nil {
		h = mix(mix(h, pointerOf(e.Img)), pointerOf(e.RGBA))
		h = mixRect(h, e.Rect)
		for _, p := range e.Pts {
			h = mixInt(mixInt(h, p.X), p.Y)
		}
		for _, p := range e.PtsF {
			h = mixFloat(mixFloat(h, p.X), p.Y)
		}
		for _, f := range e.F {
			h = mixFloat(h, f)
		}
		h = mix(h, uint64(e.Filter))
		if e.Sub != nil {
			h = e.Sub.Hash(h)
		}
	}
	return h
}

// Hash подмешивает все команды списка (и списки детей по ссылкам) в h.
func (l *List) Hash(h uint64) uint64 {
	if l == nil {
		return mix(h, 0)
	}
	for i := range l.Cmds {
		h = l.Cmds[i].Hash(h)
	}
	return mix(h, uint64(len(l.Cmds)))
}

// HashSeed — начальное значение хэша.
const HashSeed = uint64(hashOffset)

// VisitOverlapping обходит дерево списков root в порядке рисования и зовёт
// fn для каждой команды, чей охват задевает rect: ссылки раскрываются,
// группы прозрачности отдаются целиком (одной командой), прямые команды —
// как есть. Списки вне rect пропускаются целиком. fn возвращает false, чтобы
// остановить обход.
func VisitOverlapping(root *List, rect image.Rectangle, fn func(c *Cmd) bool) bool {
	return VisitOverlappingOwner(root, rect, func(c *Cmd, _ ListHolder) bool { return fn(c) })
}

// VisitOverlappingOwner — VisitOverlapping, отдающий и холдер списка, в
// котором лежит команда.
func VisitOverlappingOwner(root *List, rect image.Rectangle, fn func(c *Cmd, owner ListHolder) bool) bool {
	if root == nil || !root.BBox.Overlaps(rect) {
		return true
	}
	for i := range root.Cmds {
		c := &root.Cmds[i]
		if c.Op == OpChild {
			if c.Holder != nil && !VisitOverlappingOwner(c.Holder.CmdList(), rect, fn) {
				return false
			}
			continue
		}
		if !c.BBox.Overlaps(rect) {
			continue
		}
		if !fn(c, root.owner) {
			return false
		}
	}
	return true
}
