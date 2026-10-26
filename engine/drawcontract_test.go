package engine

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// contractBreaker рисует прямоугольник и делает в Draw то, что ему скажут.
type contractBreaker struct {
	widget.Base
	onDraw func(c *contractBreaker)
}

func (c *contractBreaker) Draw(ctx widget.DrawContext) {
	r := c.Bounds()
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{R: 200, A: 255})
	if c.onDraw != nil {
		c.onDraw(c)
	}
}

// contractScene — движок с корнем-панелью и нарушителем в ней; записи
// проверки собираются в срез.
type contractScene struct {
	eng     *Engine
	breaker *contractBreaker
	mu      sync.Mutex
	records []string
}

func newContractScene(t *testing.T, onDraw func(c *contractBreaker)) *contractScene {
	t.Helper()
	eng := New(200, 120, 30)
	t.Cleanup(eng.Stop)
	root := widget.NewPanel(color.RGBA{R: 40, G: 80, B: 120, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 120))
	br := &contractBreaker{onDraw: onDraw}
	br.SetBounds(image.Rect(20, 20, 80, 60))
	root.AddChild(br)
	eng.SetRoot(root)
	sc := &contractScene{eng: eng, breaker: br}
	eng.contract.logf = func(format string, args ...any) {
		sc.mu.Lock()
		sc.records = append(sc.records, fmt.Sprintf(format, args...))
		sc.mu.Unlock()
	}
	return sc
}

func (sc *contractScene) got() []string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return append([]string(nil), sc.records...)
}

// Invalidate из Draw называется один раз, с типом и путём виджета.
func TestDrawContract_InvalidateInDraw(t *testing.T) {
	testreset.Globals(t)
	sc := newContractScene(t, func(c *contractBreaker) { c.Invalidate() })
	sc.eng.SetDrawContractCheck(true)
	for i := 0; i < 3; i++ {
		sc.eng.RenderOnce()
	}
	recs := sc.got()
	if len(recs) != 1 {
		t.Fatalf("записей %d, ждали одну: %q", len(recs), recs)
	}
	for _, want := range []string{"Invalidate", "*engine.contractBreaker", "Panel > contractBreaker"} {
		if !strings.Contains(recs[0], want) {
			t.Errorf("в записи нет %q: %s", want, recs[0])
		}
	}
}

// Выключенная проверка ничего не пишет, и виджет работает как раньше.
func TestDrawContract_OffByDefault(t *testing.T) {
	testreset.Globals(t)
	sc := newContractScene(t, func(c *contractBreaker) { c.Invalidate() })
	if sc.eng.DrawContractCheck() {
		t.Fatal("проверка включена по умолчанию")
	}
	for i := 0; i < 3; i++ {
		sc.eng.RenderOnce()
	}
	if recs := sc.got(); len(recs) != 0 {
		t.Fatalf("выключенная проверка писала: %q", recs)
	}
	// Включили и выключили — снова тихо.
	sc.eng.SetDrawContractCheck(true)
	sc.eng.SetDrawContractCheck(false)
	sc.eng.RenderOnce()
	if recs := sc.got(); len(recs) != 0 {
		t.Fatalf("после выключения писала: %q", recs)
	}
}

// Остановленный движок снимает учёт рисующихся виджетов: он общий на процесс,
// и иначе кадры всех прочих движков платили бы за чужую отладку.
func TestDrawContract_StopReleasesTrace(t *testing.T) {
	testreset.Globals(t)
	eng := New(50, 50, 30)
	eng.SetDrawContractCheck(true)
	if !eng.DrawContractCheck() || widget.DrawTraceUsers() == 0 {
		t.Fatal("проверка не включилась")
	}
	eng.Stop()
	if eng.DrawContractCheck() {
		t.Fatal("после Stop проверка осталась включённой")
	}
	if n := widget.DrawTraceUsers(); n != 0 {
		t.Fatalf("после Stop учёт держат %d потребителей", n)
	}
}

// Анимация, заведённая из Draw, — нарушение.
func TestDrawContract_AnimateInDraw(t *testing.T) {
	testreset.Globals(t)
	sc := newContractScene(t, func(c *contractBreaker) {
		widget.Animate(10*time.Millisecond, nil, nil)
	})
	sc.eng.SetDrawContractCheck(true)
	sc.eng.RenderOnce()
	recs := sc.got()
	if len(recs) != 1 || !strings.Contains(recs[0], "Animate") {
		t.Fatalf("ждали одну запись об Animate: %q", recs)
	}
}

// Уведомление с другой горутины посреди кадра нарушением не считается:
// фоновая работа вправе инвалидировать в любой момент.
func TestDrawContract_OtherGoroutineIsFine(t *testing.T) {
	testreset.Globals(t)
	sc := newContractScene(t, func(c *contractBreaker) {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Invalidate()
		}()
		wg.Wait()
	})
	sc.eng.SetDrawContractCheck(true)
	sc.eng.RenderOnce()
	if recs := sc.got(); len(recs) != 0 {
		t.Fatalf("уведомление с чужой горутины записано: %q", recs)
	}
}

// knownDrawContractFindings — нарушения в собственных компонентах движка,
// найденные проверкой при её появлении (этап 0.3 работ к 4.0). Оба —
// анимация, заведённая из Draw:
//
//   - ClockItem.ensureTick заводит секундный тик при первом Draw, чтобы
//     анимация досталась горутине того движка, который часы рисует;
//   - motion.style (все компоненты desktop) замечает в Draw смену состояния
//     наведения и заводит переход цвета.
//
// С пропуском поддеревьев они не ломаются: смена наведения сама инвалидирует
// ячейку, а часы рисуются в первом полном кадре. Убрать их — решение о
// жизненном цикле анимаций (где их заводить, если не в Draw), а не правка в
// одну строку; до него список держит тест строгим ко всему новому.
var knownDrawContractFindings = []string{
	"Animate из Draw у *desktop.ClockItem",
	"Animate из Draw у *desktop.StartButton",
}

func isKnownDrawContractFinding(rec string) bool {
	for _, k := range knownDrawContractFindings {
		if strings.Contains(rec, k) {
			return true
		}
	}
	return false
}

// Сцена рабочего стола из бенчмарков контракт соблюдает: ни одной записи,
// кроме известных (knownDrawContractFindings).
func TestDrawContract_DesktopSceneIsClean(t *testing.T) {
	testreset.Globals(t)
	for _, tc := range []struct {
		name  string
		build func() pipeDesktop
	}{
		{"desktop", buildPipelineDesktop},
		{"desktop+apps", func() pipeDesktop { return buildPipelineDesktopApps().pipeDesktop }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := tc.build()
			defer sc.closeAll()
			eng := sc.eng
			// Сцена бенчмарков движок не останавливает, а учёт рисующихся
			// общий на процесс: без Stop он остался бы включённым у соседей.
			defer eng.Stop()
			var recs []string
			var mu sync.Mutex
			eng.contract.logf = func(format string, args ...any) {
				mu.Lock()
				recs = append(recs, fmt.Sprintf(format, args...))
				mu.Unlock()
			}
			eng.SetDrawContractCheck(true)
			eng.RenderOnce()
			eng.Invalidate()
			eng.RenderOnce()
			btn := sc.startBtn.Bounds()
			eng.SendMouseMove((btn.Min.X+btn.Max.X)/2, (btn.Min.Y+btn.Max.Y)/2)
			eng.RenderOnce()
			mu.Lock()
			defer mu.Unlock()
			var fresh []string
			for _, r := range recs {
				if isKnownDrawContractFinding(r) {
					t.Logf("известное: %s", r)
					continue
				}
				fresh = append(fresh, r)
			}
			if len(fresh) != 0 {
				t.Fatalf("новые нарушения контракта в сцене (%d):\n%s", len(fresh), strings.Join(fresh, "\n"))
			}
		})
	}
}
