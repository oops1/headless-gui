package tests

// notifier_broadcast_test.go — фаза 0 докинг-панелей: широковещательный реестр
// нотификаторов движков. Проверяет, что при ДВУХ живых движках инвалидация
// виджета доходит до «своего» движка (а не утекает в последний созданный),
// что Stop снимает регистрацию (нет утечки реестра), и что hosted-диалог с
// фейковым ModalHost не мешает родителю перерисовываться.

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// newBroadcastEngine — движок с полноэкранным корнем-панелью и одной меткой,
// tooltips выключены. Возвращает движок и метку в его дереве.
func newBroadcastEngine(w, h int, text string) (*engine.Engine, *widget.Label) {
	eng := engine.New(w, h, 50)
	eng.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, w, h))
	lbl := widget.NewLabel(text, color.RGBA{R: 230, G: 230, B: 230, A: 255})
	lbl.SetBounds(image.Rect(10, 10, w-10, 40))
	root.AddChild(lbl)
	eng.SetRoot(root)
	return eng, lbl
}

// TestBroadcast_InvalidateReachesOwnEngine: два движка с разными деревьями.
// Изменение виджета в дереве A рождает кадр движка A; движок B продолжает
// работать (реагирует на собственную инвалидацию).
func TestBroadcast_InvalidateReachesOwnEngine(t *testing.T) {
	engA, lblA := newBroadcastEngine(320, 200, "A")
	engB, _ := newBroadcastEngine(320, 200, "B")
	// Порядок создания: B создан ПОСЛЕ A. В прежней схеме «последний выигрывает»
	// уведомления виджетов A уходили бы в B, и A не перерисовался бы.

	engA.SetRenderOnDemand(true)
	engB.SetRenderOnDemand(true)
	engA.Start()
	defer engA.Stop()
	engB.Start()
	defer engB.Stop()

	if !waitCount(engA, 1) || !waitCount(engB, 1) {
		t.Fatal("первый кадр одного из движков не отрендерился")
	}
	time.Sleep(120 * time.Millisecond) // дать осесть стартовым инвалидациям

	baseA := engA.RenderCount()
	baseB := engB.RenderCount()

	// Меняем метку в дереве A — авто-damage через широковещание.
	lblA.SetText("A changed")

	if !waitCount(engA, baseA+1) {
		t.Fatalf("движок A не отрендерил кадр после изменения СВОЕГО виджета (RenderCount=%d, base=%d)", engA.RenderCount(), baseA)
	}

	// Движок B не сломан: реагирует на собственную явную инвалидацию.
	baseB = engB.RenderCount()
	engB.Invalidate()
	if !waitCount(engB, baseB+1) {
		t.Fatalf("движок B не отрендерил кадр после собственного Invalidate (RenderCount=%d, base=%d)", engB.RenderCount(), baseB)
	}
	_ = baseB
}

// TestBroadcast_StopUnregisters: Stop снимает регистрацию движка в реестре
// нотификаторов — размер реестра возвращается к исходному (нет утечки).
func TestBroadcast_StopUnregisters(t *testing.T) {
	n0 := widget.UINotifierCount()

	eng, _ := newBroadcastEngine(200, 120, "x")
	n1 := widget.UINotifierCount()
	if n1 != n0+1 {
		t.Fatalf("engine.New должен зарегистрировать ровно один приёмник: было %d, стало %d", n0, n1)
	}

	// Второй движок — ещё +1.
	eng2, _ := newBroadcastEngine(200, 120, "y")
	if got := widget.UINotifierCount(); got != n0+2 {
		t.Fatalf("после второго New ожидали %d приёмников, получили %d", n0+2, got)
	}

	// Stop ждёт завершения цикла рендера, поэтому движок нужно запустить.
	eng.Start()
	eng2.Start()

	eng.Stop()
	if got := widget.UINotifierCount(); got != n0+1 {
		t.Fatalf("после Stop первого движка ожидали %d приёмников, получили %d", n0+1, got)
	}
	eng2.Stop()
	if got := widget.UINotifierCount(); got != n0 {
		t.Fatalf("после Stop обоих движков реестр должен вернуться к %d, получили %d (утечка)", n0, got)
	}
}

// fakeSecondEngineHost — фейковый engine.ModalHost, который на ShowModal
// поднимает ВТОРОЙ движок (имитируя hosted-диалог в собственном окне), а на
// CloseModal останавливает его. Используется, чтобы проверить: живой второй
// движок не мешает родителю перерисовываться.
type fakeSecondEngineHost struct {
	child *engine.Engine
}

func (h *fakeSecondEngineHost) ShowModal(m widget.ModalWidget) bool {
	// Поднимаем отдельный движок ровно по размеру «диалога».
	b := m.Bounds()
	child, _ := newBroadcastEngine(b.Dx(), b.Dy(), "dlg")
	child.SetRenderOnDemand(true)
	child.Start()
	h.child = child
	return true // модалка целиком у хоста — движок родителя её не отслеживает
}

func (h *fakeSecondEngineHost) CloseModal(m widget.ModalWidget) bool {
	if h.child != nil {
		h.child.Stop()
		h.child = nil
	}
	return true
}

// TestBroadcast_HostedDialogParentKeepsRendering: родитель + hosted-диалог
// (второй движок). Пока второй движок жив, родитель продолжает получать
// уведомления и рендерить кадры; второй движок тоже рендерит свои изменения;
// после закрытия диалога реестр без утечки.
func TestBroadcast_HostedDialogParentKeepsRendering(t *testing.T) {
	n0 := widget.UINotifierCount()

	parent, parentLbl := newBroadcastEngine(400, 300, "parent")
	parent.SetRenderOnDemand(true)
	parent.Start()
	defer parent.Stop()

	host := &fakeSecondEngineHost{}
	parent.SetModalHost(host)

	waitCount(parent, 1)
	time.Sleep(120 * time.Millisecond)

	// Показываем hosted-диалог → фейковый хост поднимает второй движок.
	dlg := widget.NewDialog("hosted", 200, 140)
	parent.ShowModal(dlg)
	if host.child == nil {
		t.Fatal("фейковый хост не поднял второй движок")
	}
	// Теперь в реестре родитель + ребёнок (+ базовый фон от других тестов n0).
	if got := widget.UINotifierCount(); got != n0+2 {
		t.Fatalf("при живом hosted-диалоге ожидали %d приёмников, получили %d", n0+2, got)
	}

	// Родитель продолжает перерисовываться при живом втором движке.
	base := parent.RenderCount()
	parentLbl.SetText("parent changed")
	if !waitCount(parent, base+1) {
		t.Fatalf("родитель не перерисовался при живом hosted-диалоге (RenderCount=%d, base=%d)", parent.RenderCount(), base)
	}

	// Второй движок тоже жив и рендерит собственную инвалидацию.
	cbase := host.child.RenderCount()
	host.child.Invalidate()
	if !waitCount(host.child, cbase+1) {
		t.Fatalf("второй (hosted) движок не отрендерил собственную инвалидацию (RenderCount=%d, base=%d)", host.child.RenderCount(), cbase)
	}

	// Закрываем диалог → хост останавливает второй движок → реестр без утечки.
	parent.CloseModal(dlg)
	if got := widget.UINotifierCount(); got != n0+1 {
		t.Fatalf("после закрытия hosted-диалога ожидали %d приёмников (только родитель), получили %d", n0+1, got)
	}
}
