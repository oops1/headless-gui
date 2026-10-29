package core

import (
	"image"
	"testing"
)

// Арифметика полного экрана Win32 (fullscreen_rect.go): запоминание,
// восстановление, перестановка по монитору. Чистые функции — идут на любой
// платформе.

const (
	tsPopup      = 0x80000000
	tsVisible    = 0x10000000
	tsMinimizeBx = 0x00020000
)

var (
	monMain  = image.Rect(0, 0, 1920, 1080)
	monRight = image.Rect(1920, 0, 5760, 2160) // 4K справа, выше основного
)

// Вход: стиль теряет рамку ресайза и ничего больше, окно встаёт на монитор целиком.
func TestFsEnterPlan(t *testing.T) {
	style := uint32(tsPopup | tsVisible | tsMinimizeBx | fsStyleThickFrame)
	ns, target := fsEnterPlan(style, monRight)
	if ns&fsStyleThickFrame != 0 {
		t.Error("WS_THICKFRAME остался")
	}
	if ns != style&^fsStyleThickFrame {
		t.Errorf("кроме рамки стиль не должен меняться: %#x → %#x", style, ns)
	}
	if target != monRight {
		t.Errorf("прямоугольник %v, ждал весь монитор %v", target, monRight)
	}
}

// Выход возвращает рамку ресайза и прежний прямоугольник.
func TestFsLeavePlan_RestoresStyleAndRect(t *testing.T) {
	orig := uint32(tsPopup | tsVisible | fsStyleThickFrame)
	snap := fsSnapshot{Rect: image.Rect(300, 200, 1100, 800), Style: orig}
	fs, _ := fsEnterPlan(orig, monMain)

	ns, rc := fsLeavePlan(fs, snap, []image.Rectangle{monMain})
	if ns&fsStyleThickFrame == 0 {
		t.Error("рамка ресайза не вернулась")
	}
	if rc != snap.Rect {
		t.Errorf("прямоугольник после выхода %v, ждал %v", rc, snap.Rect)
	}
}

// Окно без рамки ресайза (resizable=false) рамки не получает.
func TestFsLeavePlan_NoFrameStaysNoFrame(t *testing.T) {
	orig := uint32(tsPopup | tsVisible)
	snap := fsSnapshot{Rect: image.Rect(0, 0, 400, 300), Style: orig}
	ns, _ := fsLeavePlan(orig, snap, []image.Rectangle{monMain})
	if ns&fsStyleThickFrame != 0 {
		t.Error("выход навязал рамку окну, у которого её не было")
	}
}

// Остальные биты стиля берутся текущие: их могли менять за время полного экрана.
func TestFsLeavePlan_KeepsCurrentOtherBits(t *testing.T) {
	snap := fsSnapshot{Rect: image.Rect(0, 0, 400, 300), Style: tsPopup | fsStyleThickFrame}
	cur := uint32(tsPopup | tsMinimizeBx) // за время полного экрана добавили MINIMIZEBOX
	ns, _ := fsLeavePlan(cur, snap, []image.Rectangle{monMain})
	if ns&tsMinimizeBx == 0 {
		t.Error("изменённые за время полного экрана биты потеряны")
	}
}

// Монитор, на котором окно было, отключили: окно возвращается на оставшийся,
// а не за край экрана.
func TestFsLeavePlan_MonitorGone(t *testing.T) {
	snap := fsSnapshot{Rect: image.Rect(2500, 300, 3300, 900), Style: fsStyleThickFrame}
	_, rc := fsLeavePlan(0, snap, []image.Rectangle{monMain})
	if !rc.In(monMain) {
		t.Errorf("окно %v осталось вне монитора %v", rc, monMain)
	}
	if rc.Dx() != 800 || rc.Dy() != 600 {
		t.Errorf("размер изменился без нужды: %v", rc)
	}
}

// Окно больше оставшегося монитора сжимается.
func TestFsFitToMonitors_Shrinks(t *testing.T) {
	rc := fsFitToMonitors(image.Rect(4000, 0, 5800, 1500), []image.Rectangle{monMain})
	if !rc.In(monMain) {
		t.Errorf("окно %v не помещено в монитор", rc)
	}
}

// Окно, чуть выступающее за край, не двигается (его двигает пользователь);
// выступающее сильнее — возвращается.
func TestFsFitToMonitors_Threshold(t *testing.T) {
	ms := []image.Rectangle{monMain}
	edge := image.Rect(1800, 100, 2200, 500) // 120 из 400 по ширине внутри — 30%
	if got := fsFitToMonitors(edge, ms); got != edge {
		t.Errorf("окно на 30%% видно, а его передвинули: %v", got)
	}
	away := image.Rect(1900, 100, 2300, 500) // 20 из 400 — 5%
	if got := fsFitToMonitors(away, ms); got == away {
		t.Error("окно, видимое на 5%, оставлено за краем")
	}
}

// Нет данных о мониторах — прямоугольник не трогаем.
func TestFsFitToMonitors_NoMonitors(t *testing.T) {
	r := image.Rect(10, 10, 100, 100)
	if got := fsFitToMonitors(r, nil); got != r {
		t.Errorf("без мониторов прямоугольник изменён: %v", got)
	}
}

// Перестановка полноэкранного окна: на другой монитор, под новое разрешение;
// уже на месте — ничего.
func TestFsRefit(t *testing.T) {
	if _, ok := fsRefit(monMain, monMain); ok {
		t.Error("окно уже на мониторе, а перестановка запрошена")
	}
	// Перенос Win+Shift+стрелка: окно прежнего размера оказалось на 4K-мониторе.
	moved := image.Rect(1920, 0, 3840, 1080)
	got, ok := fsRefit(moved, monRight)
	if !ok || got != monRight {
		t.Errorf("после переноса: %v %v, ждал %v", got, ok, monRight)
	}
	// Сменилось разрешение монитора.
	got, ok = fsRefit(monMain, image.Rect(0, 0, 1280, 720))
	if !ok || got != image.Rect(0, 0, 1280, 720) {
		t.Errorf("после смены разрешения: %v %v", got, ok)
	}
	// Монитор не определён — не двигаем.
	if _, ok := fsRefit(monMain, image.Rectangle{}); ok {
		t.Error("пустой монитор вызвал перестановку")
	}
}
