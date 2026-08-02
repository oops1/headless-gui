package tests

// Регрессионные тесты оптимизаций аудита производительности:
//   - адресная доставка движений мыши (engine.broadcastMouseMove);
//   - кэш переноса текста в Label.

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// moveRecorder — виджет, считающий полученные OnMouseMove.
type moveRecorder struct {
	widget.Base
	moves []image.Point
}

func (m *moveRecorder) Draw(widget.DrawContext) {}
func (m *moveRecorder) OnMouseMove(x, y int) {
	m.moves = append(m.moves, image.Pt(x, y))
}

// TestMouseMoveTargeted — движение мыши доставляется только затронутым
// виджетам: тем, где курсор сейчас, и тем, откуда он ушёл (снять hover).
// Виджет в стороне событий не получает — раньше move рассылался всем подряд.
func TestMouseMoveTargeted(t *testing.T) {
	eng := engine.New(400, 300, 30)

	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	inA := &moveRecorder{}
	inA.SetBounds(image.Rect(10, 10, 110, 60)) // здесь курсор побывает
	root.AddChild(inA)

	inB := &moveRecorder{}
	inB.SetBounds(image.Rect(150, 10, 250, 60)) // сюда курсор придёт
	root.AddChild(inB)

	far := &moveRecorder{}
	far.SetBounds(image.Rect(10, 200, 110, 250)) // в стороне от траектории
	root.AddChild(far)

	eng.SetRoot(root)

	eng.SendMouseMove(50, 30)  // в A
	eng.SendMouseMove(200, 30) // A → B: A получает (уход), B получает (приход)
	eng.SendMouseMove(210, 35) // внутри B

	if len(inA.moves) != 2 { // приход + уход
		t.Errorf("A получил %d событий, ожидалось 2 (вход и уход): %v", len(inA.moves), inA.moves)
	}
	if len(inB.moves) != 2 { // приход + движение внутри
		t.Errorf("B получил %d событий, ожидалось 2: %v", len(inB.moves), inB.moves)
	}
	if len(far.moves) != 0 {
		t.Errorf("виджет в стороне получил %d событий, ожидалось 0: %v", len(far.moves), far.moves)
	}
}

// overlayRecorder — как moveRecorder, но с активным overlay: его видимая
// область шире bounds, поэтому move он должен получать всегда.
type overlayRecorder struct {
	moveRecorder
	overlay bool
}

func (o *overlayRecorder) HasOverlay() bool               { return o.overlay }
func (o *overlayRecorder) DrawOverlay(widget.DrawContext) {}

// TestMouseMoveOverlayAlwaysDelivered — виджет с открытым overlay (dropdown,
// контекстное меню) получает движения даже вне своих bounds: список рисуется
// за их пределами, и hover по пунктам живёт именно там.
func TestMouseMoveOverlayAlwaysDelivered(t *testing.T) {
	eng := engine.New(400, 300, 30)

	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	ov := &overlayRecorder{}
	ov.SetBounds(image.Rect(10, 10, 110, 40))
	root.AddChild(ov)

	eng.SetRoot(root)

	ov.overlay = false
	eng.SendMouseMove(300, 200) // далеко и без overlay — не доставляется
	eng.SendMouseMove(310, 210)
	if len(ov.moves) != 0 {
		t.Fatalf("без overlay виджет получил %d событий: %v", len(ov.moves), ov.moves)
	}

	ov.overlay = true
	eng.SendMouseMove(320, 220) // далеко, но overlay открыт — доставляется
	if len(ov.moves) != 1 {
		t.Errorf("с открытым overlay получено %d событий, ожидалось 1", len(ov.moves))
	}
}

// TestMouseMoveCaptureOutruns — захват мыши доставляет движения капчеру даже
// далеко за его границами (drag ползунка, сплиттера, окна).
func TestMouseMoveCaptureOutruns(t *testing.T) {
	eng := engine.New(400, 300, 30)

	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	rec := &moveRecorder{}
	rec.SetBounds(image.Rect(10, 10, 60, 40))
	root.AddChild(rec)

	eng.SetRoot(root)
	eng.SetCapture(rec)
	defer eng.ReleaseCapture()

	eng.SendMouseMove(390, 290) // далеко за bounds
	if len(rec.moves) != 1 {
		t.Fatalf("капчер получил %d событий, ожидалось 1", len(rec.moves))
	}
}

// countCtx — DrawContext, считающий вызовы измерения текста (для проверки
// кэша переноса). Остальное наследуется от recCtx (см. titlebadge_test.go).
type countCtx struct {
	recCtx
	measures int
}

func (c *countCtx) MeasureText(text string, sizePt float64) int {
	c.measures++
	return c.recCtx.MeasureText(text, sizePt)
}

// TestLabelWrapCached — перенос многострочного текста считается один раз, а
// не на каждом Draw; смена текста или ширины пересчитывает заново.
func TestLabelWrapCached(t *testing.T) {
	l := widget.NewLabel(strings.Repeat("слово ", 60), color.RGBA{R: 255, G: 255, B: 255, A: 255})
	l.WrapText = true
	l.SetBounds(image.Rect(0, 0, 200, 400))

	ctx := &countCtx{}
	l.Draw(ctx)
	first := ctx.measures
	if first == 0 {
		t.Fatal("перенос не вызвал ни одного измерения — тест не про то")
	}

	l.Draw(ctx)
	l.Draw(ctx)
	if ctx.measures != first {
		t.Errorf("повторные Draw добавили измерений: %d → %d (кэш не работает)", first, ctx.measures)
	}

	// Смена текста инвалидирует кэш.
	l.SetText(strings.Repeat("другое ", 50))
	l.Draw(ctx)
	if ctx.measures == first {
		t.Error("после смены текста перенос не пересчитан")
	}
	after := ctx.measures

	// Смена ширины — тоже.
	l.SetBounds(image.Rect(0, 0, 120, 400))
	l.Draw(ctx)
	if ctx.measures == after {
		t.Error("после смены ширины перенос не пересчитан")
	}

	// Ревизия метрик (смена DPI/масштаба движком) сбрасывает кэш.
	before := ctx.measures
	widget.BumpTextMetricsRev()
	l.Draw(ctx)
	if ctx.measures == before {
		t.Error("после смены ревизии метрик перенос не пересчитан")
	}
}
