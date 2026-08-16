package tests

// audit_datagrid_lock_test.go — PERF-8: тяжёлая работа больше не держит лок.
//
// DataGrid.Draw держал dg.mu на весь кадр (мышь ждала медленную отрисовку),
// а CollectionView.Refresh держал v.mu на весь filter+sort+group (Items() и
// Count() вставали, а предикат фильтра, дёрнувший Count(), вешал поток).

import (
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
	dg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// blockingCtx замирает на первой же отрисовке текста, изображая медленный
// кадр, и отпускает поток только по внешней команде.
type blockingCtx struct {
	nopCtx
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingCtx) DrawTextSize(s string, x, y int, sz float64, col color.RGBA) {
	c.once.Do(func() {
		close(c.entered)
		<-c.release
	})
}

// withinDeadline выполняет fn и валит тест, если она не уложилась в срок.
func withinDeadline(t *testing.T, what string, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s: заблокировано на время отрисовки/пересчёта — лок держится слишком долго", what)
	}
}

// TestPERF8_DrawDoesNotBlockInput — пока идёт кадр, мышь, клавиатура и
// публичные геттеры продолжают работать.
func TestPERF8_DrawDoesNotBlockInput(t *testing.T) {
	g, oc := newSecGrid(30)

	ctx := &blockingCtx{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	drawDone := make(chan struct{})
	go func() {
		defer close(drawDone)
		g.Draw(ctx)
	}()

	select {
	case <-ctx.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("отрисовка не началась")
	}

	// Кадр «завис» посреди отрисовки — ввод обязан проходить.
	withinDeadline(t, "OnMouseMove во время кадра", time.Second, func() {
		g.OnMouseMove(50, 60)
	})
	withinDeadline(t, "OnMouseButton во время кадра", time.Second, func() {
		g.OnMouseButton(50, 60, 0, true)
	})
	withinDeadline(t, "SelectedItem во время кадра", time.Second, func() {
		_ = g.SelectedItem()
	})
	withinDeadline(t, "ScrollBy во время кадра", time.Second, func() {
		g.ScrollBy(20)
	})
	withinDeadline(t, "изменение коллекции во время кадра", time.Second, func() {
		oc.Add(&secRow{Name: "hot", Age: 99})
		oc.RemoveAt(0)
	})

	close(ctx.release)
	<-drawDone
}

// TestPERF8_RefreshDoesNotBlockReaders — Items()/Count() доступны, пока
// CollectionView пересчитывает представление.
func TestPERF8_RefreshDoesNotBlockReaders(t *testing.T) {
	src := cvBenchSource(2000)
	v := widget.NewCollectionView(src)

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	// SetFilter сам зовёт Refresh, поэтому уводим его в отдельную горутину:
	// пересчёт «зависнет» внутри предиката, изображая долгую работу.
	filterDone := make(chan struct{})
	go func() {
		defer close(filterDone)
		v.SetFilter(func(it interface{}) bool {
			once.Do(func() {
				close(entered)
				<-release
			})
			return true
		})
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("пересчёт не дошёл до предиката фильтра")
	}

	withinDeadline(t, "Count() во время Refresh", time.Second, func() {
		_ = v.Count()
	})
	withinDeadline(t, "Items() во время Refresh", time.Second, func() {
		_ = v.Items()
	})
	withinDeadline(t, "Groups() во время Refresh", time.Second, func() {
		_ = v.Groups()
	})
	close(release)
	<-filterDone
}

// TestPERF8_FilterMayCallView — предикат фильтра может дёргать само
// представление: раньше это был гарантированный дедлок на v.mu.
func TestPERF8_FilterMayCallView(t *testing.T) {
	src := cvBenchSource(200)
	v := widget.NewCollectionView(src)

	withinDeadline(t, "фильтр, зовущий Count()", 3*time.Second, func() {
		v.SetFilter(func(it interface{}) bool {
			_ = v.Count()
			_ = v.Groups()
			return it.(*cvBenchRow).Age >= 45
		})
	})
	if v.Count() == 0 {
		t.Fatal("фильтр не применился")
	}
}

// TestPERF8_ConcurrentRefreshAndReads — параллельные Refresh/чтения/мутации
// не роняют представление и не рвут его согласованность (запускать с -race).
func TestPERF8_ConcurrentRefreshAndReads(t *testing.T) {
	src := cvBenchSource(500)
	v := widget.NewCollectionView(src)
	v.SetSort(widget.SortDescription{Property: "Name"})

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			v.Refresh()
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = v.Items()
			_ = v.Count()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			src.Add(&cvBenchRow{Name: "z", Age: i})
			src.RemoveAt(0)
		}
	}()

	time.Sleep(250 * time.Millisecond)
	close(stop)
	wg.Wait()

	// После затишья представление обязано соответствовать источнику.
	v.Refresh()
	if got, want := v.Count(), src.Count(); got != want {
		t.Fatalf("после гонок представление рассинхронизировано: %d против %d", got, want)
	}
}

// TestPERF8_GridDrawStillCorrectAfterUnlocking — кадр вне лока рисует ровно
// то же, что и раньше: текст ячеек соответствует модели.
func TestPERF8_GridDrawStillCorrectAfterUnlocking(t *testing.T) {
	g := dg.New()
	g.RowHeight, g.HeaderHeight = 20, 20
	col := dg.NewTextColumn("Name", "Name")
	col.SetActualWidth(100)
	g.AddColumn(col)

	oc := dg.NewObservableCollection()
	oc.Add(&secRow{Name: "alpha"})
	oc.Add(&secRow{Name: "beta"})
	g.SetItemsSource(oc)
	g.SetBounds(image.Rect(0, 0, 200, 20+2*20))

	rec := &recordingCtx{}
	g.Draw(rec)
	if !hasText(rec.texts, "alpha") || !hasText(rec.texts, "beta") {
		t.Fatalf("кадр не содержит текста строк: %v", rec.texts)
	}

	oc.Set(1, &secRow{Name: "gamma"})
	rec.texts = nil
	g.Draw(rec)
	if !hasText(rec.texts, "gamma") || hasText(rec.texts, "beta") {
		t.Fatalf("кадр после изменения модели: %v", rec.texts)
	}
}

// recordingCtx запоминает нарисованный текст.
type recordingCtx struct {
	nopCtx
	texts []string
}

func (r *recordingCtx) DrawTextSize(s string, x, y int, sz float64, col color.RGBA) {
	r.texts = append(r.texts, s)
}

func hasText(texts []string, want string) bool {
	for _, s := range texts {
		if s == want {
			return true
		}
	}
	return false
}
