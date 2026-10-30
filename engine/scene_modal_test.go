package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Модальное окно над окном программы: затемнение поверх его пикселей, ни
// щелчков, ни движения, ни клавиш программе, видео закрыто целиком; после
// закрытия клавиши снова идут программе.
func TestSceneModal_CoversSurface(t *testing.T) {
	testreset.Globals(t)
	sc := newSceneInput(t)
	eng := sc.eng
	eng.SetRenderOnDemand(true)

	// Программа с фокусом.
	eng.SendMouseButton(60, 50, widget.MouseLeft, true)
	eng.SendMouseButton(60, 50, widget.MouseLeft, false)
	if !sc.surf.IsFocused() {
		t.Fatal("фокус не у окна программы — тест не о том")
	}
	before := eng.RenderOnce().RGBAAt(40, 40)

	dlg := widget.NewDialog("Пароль", 120, 80)
	eng.ShowModal(dlg)
	after := eng.RenderOnce().RGBAAt(40, 40)
	if after == before {
		t.Fatalf("окно программы под модальным не затемнено: %v", after)
	}

	sc.log.events = nil
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyA, Pressed: true})
	eng.SendMouseMove(30, 30) // в окне программы, вне диалога
	eng.SendMouseButton(30, 30, widget.MouseLeft, true)
	eng.SendMouseButton(30, 30, widget.MouseLeft, false)
	for _, ev := range sc.log.events {
		if ev != "leave" {
			t.Fatalf("при открытом модальном окне программа получила %q (все: %q)", ev, sc.log.events)
		}
	}

	// Видео под затемнением закрыто целиком.
	for i := 0; i < 30; i++ {
		sc.surf.InvalidateRect(image.Rect(0, 0, 10, 10))
	}
	fr := eng.RenderFrameNow()
	for _, v := range fr.Videos {
		if v.ID == sc.surf.SurfaceID() {
			if len(v.Cover) == 0 || v.Cover[len(v.Cover)-1] != v.Rect {
				t.Fatalf("видео под затемнением не закрыто: rect %v cover %v", v.Rect, v.Cover)
			}
		}
	}

	eng.CloseModal(dlg)
	sc.log.events = nil
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyA, Pressed: true})
	if sc.log.count("key") != 1 {
		t.Fatalf("после закрытия клавиши не вернулись программе: %q", sc.log.events)
	}
}
