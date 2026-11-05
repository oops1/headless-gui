package core

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/platform/native"
)

// Источник тени окна → режим оформления попапов движка.

type sysShadowNative struct {
	popupFakeNative
	sys    bool
	corner int // последний SetPopupSystem; -1 — не звали
}

func (s *sysShadowNative) SystemPopupShadow() bool { return s.sys }
func (s *sysShadowNative) SetPopupSystem(c int)    { s.corner = c }
func (s *sysShadowNative) CreatePopup(int, int) error { return nil }

func TestPopupShadowMode(t *testing.T) {
	for _, tc := range []struct {
		src  ShadowSource
		sys  bool
		want engine.PopupShadowMode
	}{
		{ShadowAuto, true, engine.PopupShadowSystem | engine.PopupShadowEngine},
		{ShadowAuto, false, engine.PopupShadowEngine},
		{ShadowSystem, true, engine.PopupShadowSystem},
		{ShadowSystem, false, engine.PopupShadowNone},
		{ShadowEngine, true, engine.PopupShadowEngine},
		{ShadowEngine, false, engine.PopupShadowEngine},
	} {
		if got := popupShadowMode(tc.src, &sysShadowNative{sys: tc.sys}); got != tc.want {
			t.Errorf("источник %d, ОС оттеняет=%v: режим %d, ждали %d", tc.src, tc.sys, got, tc.want)
		}
	}
	// Бэкенд, не знающий о тенях ОС (X11, Wayland), — как без них.
	if got := popupShadowMode(ShadowAuto, &popupFakeNative{}); got != engine.PopupShadowEngine {
		t.Errorf("бэкенд без теней ОС: режим %d", got)
	}
}

// Окно-попап узнаёт, кто его оформляет, до создания: кадр System —
// радиус углов, иначе 0 (альфа точек, тень движка).
func TestPopupHost_SystemDecorBeforeCreate(t *testing.T) {
	for _, tc := range []struct {
		f    engine.PopupFrame
		want int
	}{
		{engine.PopupFrame{System: true, Corner: 8}, 8},
		{engine.PopupFrame{System: true, Corner: 0}, 1},
		{engine.PopupFrame{Pad: 24}, 0},
	} {
		pop := &sysShadowNative{corner: -1}
		prev := newNativeWindow
		newNativeWindow = func() native.NativeWindow { return pop }
		carrier := &popupFakeNative{}
		h := newPopupHost(carrier, carrier, &queueEngine{}, 1, nil)
		f := tc.f
		f.ID, f.Rect, f.Img = 1, image.Rect(0, 0, 10, 10), image.NewRGBA(image.Rect(0, 0, 10, 10))
		h.apply([]engine.PopupFrame{f})
		for len(carrier.invoked) > 0 {
			fn := carrier.invoked[0]
			carrier.invoked = carrier.invoked[1:]
			fn()
		}
		newNativeWindow = prev
		if pop.corner != tc.want {
			t.Errorf("кадр %+v: SetPopupSystem(%d), ждали %d", tc.f, pop.corner, tc.want)
		}
	}
}
