//go:build windows

package win32

import (
	"image"
	"image/color"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Своя тень окна (shadowwnd_windows.go): окно-тень под окном, когда DWM тени
// не даёт (или выбран движок), и никакого — когда даёт.

var procTestGetPixel = gdi32.NewProc("GetPixel")

func shadowPump() {
	var m msg
	for i := 0; i < 40; i++ {
		for {
			r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
			if r == 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func shadowScreenPixel(x, y int) (uint32, bool) {
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		return 0, false
	}
	c, _, _ := procTestGetPixel.Call(dc, uintptr(x), uintptr(y))
	procReleaseDC.Call(0, dc)
	return uint32(c), uint32(c) != 0xFFFFFFFF
}

// shadowScreenLive сообщает, отражает ли чтение экрана слоёные окна в точке
// (x, y): поверх неё на миг ставится маленькое непрозрачное окно «поверх всех»
// (стандартный STATIC, белый или чёрный — противоположный тому, что на экране),
// и пиксель обязан измениться. Окно слоёное (WS_EX_LAYERED), как окно-тень:
// контроль идёт тем же путём компоновки, что и проверяемое. Не изменился —
// экран наших окон не показывает: кадр рабочего стола не обновляется (фоновый
// сеанс машины CI), слоёные окна в него не попадают или точку закрывает чужое
// окно поверх всех. По такому экрану потемнение не сверить.
func shadowScreenLive(x, y int) bool {
	before, ok := shadowScreenPixel(x, y)
	if !ok {
		return false
	}
	const ssBlackRect, ssWhiteRect = 0x4, 0x6
	style := uintptr(wsPopup | wsVisible | ssWhiteRect)
	if lum(before) > 3*128 {
		style = uintptr(wsPopup | wsVisible | ssBlackRect)
	}
	cls, _ := windows.UTF16PtrFromString("STATIC")
	h, _, _ := procCreateWindowExW.Call(
		uintptr(wsExLayered|wsExTransparent|wsExTopmost|wsExToolwindow|wsExNoactivate),
		uintptr(unsafe.Pointer(cls)), 0, style,
		uintptr(x-8), uintptr(y-8), 16, 16, 0, 0, 0, 0)
	if h == 0 {
		return false
	}
	const lwaAlpha = 0x2
	procSetLayeredWindowAttributes.Call(h, 0, 255, lwaAlpha) // непрозрачное; без вызова слоёное окно не видно
	live := false
	for try := 0; try < 3 && !live; try++ {
		shadowPump()
		c, _ := shadowScreenPixel(x, y)
		live = c != before
	}
	procDestroyWindow.Call(h)
	shadowPump() // экран под пробным окном возвращается к прежнему
	return live
}

func withDwmShadows(t *testing.T, on bool) {
	prev := dwmShadowsOn
	dwmShadowsOn = func() bool { return on }
	t.Cleanup(func() { dwmShadowsOn = prev })
}

// ShadowEngine: тень DWM снята (углы прямые), под окном — окно-тень,
// выходящее за окно на размытие и смещение; на экране под окном темнее.
func TestWin32OwnShadow_Engine(t *testing.T) {
	w := newFsWindow(t)
	w.SetPosition(200, 160)
	w.SetCornerRadius(8)
	// Потемнение сверяется по живому экрану, а окно теста — окно фонового
	// процесса: оно может оказаться под чужим окном (редактор, терминал, окно
	// на машине CI), и тогда под ним читается чужой пиксель. Поднимаем его
	// поверх всех — окно-тень идёт следом (shadowwnd_windows.go); если Windows
	// не дала, пиксели не сверяются, остальное проверяется как прежде.
	onTop := w.SetAlwaysOnTop(true)
	shadowPump()
	wr := w.windowRect()
	// Живой ли экран в точке сверки. На машине CI (windows-latest, прогоны #363
	// и #364: «под окном не темнее: 0a0a0a, было 0a0a0a» при окне поверх всех и
	// ожидании кадра 2,4 с) чтение экрана порой отдаёт один и тот же пиксель,
	// что бы на нём ни показывали. Пробное окно это отличает от пропавшей тени.
	live := shadowScreenLive(wr.Min.X+wr.Dx()/2, wr.Max.Y+8)
	below, ok := shadowScreenPixel(wr.Min.X+wr.Dx()/2, wr.Max.Y+8)
	if !ok {
		t.Skip("экран не читается")
	}
	w.SetEngineShadow(16, 0, 6, color.RGBA{A: 120})
	w.SetShadowSource(shadowEngine)
	shadowPump()
	if w.RoundsCorners() {
		t.Error("при своей тени углы окна должны быть прямыми (DWM снят)")
	}
	sr, shown := w.ownShadowShown()
	if !shown {
		t.Fatal("окно-тень не показано")
	}
	m := shadowMargin(engineShadowSpec{blur: 16, offY: 6})
	if want := wr.Inset(-m); sr != want {
		t.Errorf("окно-тень %v, ждали %v", sr, want)
	}
	// Тень на экран кладёт компоновщик ОС — не в тот же миг, что показано
	// окно-тень: на занятой машине кадр с ней приходит позже. Ждём его, а не
	// читаем один раз. На чёрном экране темнее стать нечему.
	switch {
	case !onTop:
		t.Log("окно теста не поднялось поверх остальных: потемнение экрана не сверяется")
	case !live:
		t.Log("чтение экрана не видит пробное окно в точке сверки: потемнение экрана не сверяется")
	case lum(below) < 8:
		t.Logf("экран под окном чёрный (%06x): потемнение не сверяется", below)
	default:
		var c uint32
		darker := false
		for try := 0; try < 5 && !darker; try++ {
			if try > 0 {
				shadowPump()
			}
			c, _ = shadowScreenPixel(wr.Min.X+wr.Dx()/2, wr.Max.Y+8)
			darker = lum(c) < lum(below)
		}
		if !darker {
			t.Errorf("под окном не темнее: %06x, было %06x", c, below)
		}
	}
	// Развёрнутое окно тени не имеет; восстановленное — снова имеет.
	w.Maximize()
	shadowPump()
	if _, shown := w.ownShadowShown(); shown {
		t.Error("у развёрнутого окна осталась тень")
	}
	w.Restore()
	shadowPump()
	if _, shown := w.ownShadowShown(); !shown {
		t.Error("после восстановления тень не вернулась")
	}
}

// ShadowAuto: своя тень — только когда DWM тени не даёт.
func TestWin32OwnShadow_AutoFollowsDWM(t *testing.T) {
	for _, dwm := range []bool{true, false} {
		withDwmShadows(t, dwm)
		w := newFsWindow(t)
		w.SetShadowSource(shadowAuto)
		shadowPump()
		_, shown := w.ownShadowShown()
		if shown == dwm {
			t.Errorf("тени DWM есть=%v: окно-тень показано=%v", dwm, shown)
		}
		// Только DWM: своей тени нет никогда.
		w.SetShadowSource(shadowSystem)
		shadowPump()
		if _, shown := w.ownShadowShown(); shown {
			t.Errorf("ShadowSystem (тени DWM есть=%v): показано окно-тень", dwm)
		}
	}
}

// Окно-тень следует за окном и уходит вместе с ним.
func TestWin32OwnShadow_FollowsAndDies(t *testing.T) {
	withDwmShadows(t, false)
	w := newFsWindow(t)
	shadowPump()
	w.SetPosition(260, 210)
	w.SetSize(500, 340)
	shadowPump()
	sr, shown := w.ownShadowShown()
	if !shown {
		t.Fatal("окно-тень не показано")
	}
	wr := w.windowRect()
	if !sr.In(wr.Inset(-200)) || !wr.In(sr) {
		t.Errorf("окно-тень %v не вокруг окна %v", sr, wr)
	}
	sh := w.shadow.hwnd
	procDestroyWindow.Call(uintptr(w.hwnd))
	shadowPump()
	if v, _, _ := procIsWindow.Call(sh); v != 0 {
		t.Error("окно-тень пережило окно")
	}
}

// Картинка тени большого окна, собранная из образца, совпадает с размытой
// целиком по краям (середина под окном пуста).
func TestShadowImage_SampleMatchesFull(t *testing.T) {
	sp := engineShadowSpec{blur: 12, offX: 0, offY: 5, col: color.RGBA{A: 100}}
	m := shadowMargin(sp)
	size := image.Pt(300, 200)
	got := shadowImage(size, m, 8, sp)
	full := shadowSample(size, m, 8, sp)
	if got.Rect != full.Rect {
		t.Fatalf("размер %v, ждали %v", got.Rect, full.Rect)
	}
	band := m + 8 + 2
	b := got.Rect
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if x >= band && x < b.Max.X-band && y >= band && y < b.Max.Y-band {
				continue
			}
			g, f := got.RGBAAt(x, y), full.RGBAAt(x, y)
			if d := int(g.A) - int(f.A); d > 1 || d < -1 {
				t.Fatalf("(%d,%d): образец %v, целиком %v", x, y, g, f)
			}
		}
	}
}

func lum(c uint32) int { return int(c&0xFF) + int(c>>8&0xFF) + int(c>>16&0xFF) }
