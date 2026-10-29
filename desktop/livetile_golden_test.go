package desktop_test

// Golden-снимок меню «Пуск» с живыми плитками (задача 6.3, ADR-0010).
//
// Эталон — desktop/testdata/golden/startmenu_livetiles_win10.png. Регенерация
// после осознанного изменения рендера:
//
//	GOLDEN_UPDATE=1 go test ./desktop/ -run TestGolden_LiveTiles
//
// Кадр детерминирован: встроенные шрифты и значки, фиксированный язык и
// анимации доведены до конца до снимка, у живых плиток по одному состоянию
// (без перебора по таймеру).

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/internal/testengine"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// liveTilesGroups — группы плиток сцены: две живые (часы и почта с метой),
// статичная широкая и статичная малая.
func liveTilesGroups(icons *widget.IconSet, clock, mail *desktop.FakeTileSource) []desktop.TileGroup {
	ico := func(name string) image.Image {
		return icons.ResolveIcon(theme.IconRef{Name: name}, 24)
	}
	return []desktop.TileGroup{
		{ID: "work", Title: "Работа", Tiles: []desktop.Tile{
			{ID: "clock", App: "clock", Size: desktop.TileMedium,
				Content: desktop.TileContent{Title: "Часы", Icon: ico("battery")},
				Live:    desktop.LiveTile{Source: clock}},
			{ID: "mail", App: "mail", Size: desktop.TileMedium,
				Content: desktop.TileContent{Title: "Почта", Icon: ico("network.ethernet")},
				Live:    desktop.LiveTile{Source: mail}},
			{ID: "term", App: "term", Size: desktop.TileWide,
				Content: desktop.TileContent{Title: "Терминал", Subtitle: "Открыт", Icon: ico("start")}},
		}},
		{ID: "rest", Title: "Прочее", Tiles: []desktop.Tile{
			{ID: "files", App: "files", Size: desktop.TileSmall,
				Content: desktop.TileContent{Title: "Файлы", Icon: ico("start")}},
		}},
	}
}

// liveTilesScene собирает меню с живыми плитками под Windows 10 и открывает
// его. Источники подписываются при открытии и отдают текущее состояние
// синхронно, поэтому снимок уже показывает живое содержимое.
func liveTilesScene(t *testing.T, w, h int) (*image.RGBA, *desktop.StartMenu) {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	icons := widget.BuiltinIcons()
	m.SetIconResolver(icons)

	// Состояние источника заменяет всё содержимое плитки, значок тоже: он
	// задан в состоянии, иначе живая плитка остаётся без значка.
	ico := func(name string) image.Image { return icons.ResolveIcon(theme.IconRef{Name: name}, 24) }
	clock := desktop.NewFakeTileSource(desktop.TileContent{Title: "15:09", Subtitle: "Суббота", Icon: ico("battery")})
	mail := desktop.NewFakeTileSource(desktop.TileContent{Title: "Почта", Subtitle: "Новое письмо", Badge: "3", Icon: ico("network.ethernet")})

	root := widget.NewPanel(color.RGBA{R: 30, G: 60, B: 110, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	cat := desktop.NewStaticAppCatalog(
		desktop.AppInfo{ID: "clock", Title: "Часы"},
		desktop.AppInfo{ID: "mail", Title: "Почта"},
		desktop.AppInfo{ID: "term", Title: "Терминал"},
		desktop.AppInfo{ID: "files", Title: "Файлы"},
	)
	menu := desktop.NewStartMenu(m, cat)
	menu.Screen = image.Rect(0, 0, w, h)
	menu.SetTileGroups(liveTilesGroups(icons, clock, mail))
	root.AddChild(menu)

	eng := testengine.New(t, w, h, 30)
	eng.SetTooltipsEnabled(false)
	eng.SetRoot(root)
	menu.Open(image.Rect(0, h-40, 48, h))

	// Анимации открытия доводим до конца: кадр не должен зависеть от часов.
	now := time.Now()
	widget.StepAnimations(now)
	widget.StepAnimations(now.Add(time.Hour))

	img := eng.RenderOnce()
	if img == nil {
		t.Fatal("кадр не отрисован")
	}
	return img, menu
}

func TestGolden_LiveTiles(t *testing.T) {
	testreset.Globals(t)
	t.Cleanup(widget.StopAllAnimations)
	const w, h = 640, 480
	img, menu := liveTilesScene(t, w, h)
	if !menu.IsOpen() {
		t.Fatal("меню не открылось")
	}
	tilesGoldenCompare(t, "startmenu_livetiles_win10", img)
}

// tilesGoldenCompare сравнивает кадр с эталоном testdata/golden/<name>.png
// побайтно в едином представлении (через PNG-roundtrip, как в engine).
func tilesGoldenCompare(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".png")

	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		t.Logf("эталон обновлён: %s", path)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("эталон %s отсутствует (сгенерируйте: GOLDEN_UPDATE=1 go test ./desktop/ -run TestGolden_LiveTiles): %v", path, err)
	}
	defer f.Close()
	want, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if want.Bounds() != got.Bounds() {
		t.Fatalf("%s: размер %v, эталон %v", name, got.Bounds(), want.Bounds())
	}
	b := got.Bounds()
	diff := 0
	var first image.Point
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g := color.NRGBAModel.Convert(got.At(x, y))
			w := color.NRGBAModel.Convert(want.At(x, y))
			if g != w {
				if diff == 0 {
					first = image.Pt(x, y)
				}
				diff++
			}
		}
	}
	if diff > 0 {
		t.Errorf("%s: %d пикселей отличаются от эталона, первый в %v", name, diff, first)
	}
}

// Фейковый источник: подписка получает текущее состояние сразу, Set рассылает
// новое, отписка прекращает доставку.
func TestFakeTileSource_SubscribeSyncSetAndUnsubscribe(t *testing.T) {
	testreset.Globals(t)
	src := desktop.NewFakeTileSource(desktop.TileContent{Title: "раз"})
	var got []string
	unsub := src.Subscribe(func(c desktop.TileContent) { got = append(got, c.Title) })
	src.Set(desktop.TileContent{Title: "два"})
	unsub()
	unsub()
	src.Set(desktop.TileContent{Title: "три"})
	if want := []string{"раз", "два"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("доставлено %v, ждали %v", got, want)
	}
	if c, ok := src.Current(); !ok || c.Title != "три" {
		t.Errorf("текущее состояние %q %v, ждали «три»", c.Title, ok)
	}
	empty := desktop.NewFakeTileSource()
	if _, ok := empty.Current(); ok {
		t.Error("пустой источник сообщил состояние")
	}
}

// Подписка параллельно с записями: много подписок идут, пока один писатель
// меняет состояние. Каждая подписка остаётся активной до конца, и в конце
// каждый подписчик видит ровно то, что вернул Current, — без возвратов назад
// (писатель один, номер состояния растёт вместе со значением Badge).
func TestFakeTileSource_SubscribeRacingSetEndsOnCurrent(t *testing.T) {
	testreset.Globals(t)
	src := desktop.NewFakeTileSource(desktop.TileContent{Badge: fmt.Sprintf("%08d", 0)})
	const sets, subscribers, perSub = 3000, 4, 25

	type seen struct {
		mu   sync.Mutex
		list []string
	}
	var mu sync.Mutex
	var recs []*seen
	var wg sync.WaitGroup
	for i := 0; i < subscribers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < perSub; k++ {
				rec := &seen{}
				src.Subscribe(func(c desktop.TileContent) {
					rec.mu.Lock()
					rec.list = append(rec.list, c.Badge)
					rec.mu.Unlock()
				})
				mu.Lock()
				recs = append(recs, rec)
				mu.Unlock()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for k := 1; k <= sets; k++ {
			src.Set(desktop.TileContent{Badge: fmt.Sprintf("%08d", k)})
		}
	}()
	wg.Wait()

	cur, ok := src.Current()
	if !ok {
		t.Fatal("состояния нет")
	}
	bad := 0
	for i, rec := range recs {
		rec.mu.Lock()
		list := append([]string(nil), rec.list...)
		rec.mu.Unlock()
		if len(list) == 0 {
			t.Errorf("подписка %d не получила ни одного состояния", i)
			continue
		}
		back := false
		for j := 1; j < len(list); j++ {
			if list[j] < list[j-1] {
				back = true
				break
			}
		}
		if back {
			t.Errorf("подписка %d: доставка вернулась назад", i)
		}
		if last := list[len(list)-1]; last != cur.Badge {
			bad++
			if bad <= 3 {
				t.Errorf("подписка %d закончила на %q, Current — %q", i, last, cur.Badge)
			}
		}
	}
}

// Несколько писателей сразу: порядок между ними не задан, но подписчик в конце
// видит именно то состояние, которое источник считает текущим.
func TestFakeTileSource_ConcurrentSettersEndOnCurrent(t *testing.T) {
	testreset.Globals(t)
	src := desktop.NewFakeTileSource(desktop.TileContent{Badge: "start"})
	const setters, sets, subscribers = 8, 200, 4

	type seen struct {
		mu   sync.Mutex
		last string
	}
	recs := make([]*seen, subscribers)
	var wg sync.WaitGroup
	for i := 0; i < subscribers; i++ {
		rec := &seen{}
		recs[i] = rec
		wg.Add(1)
		go func() {
			defer wg.Done()
			src.Subscribe(func(c desktop.TileContent) {
				rec.mu.Lock()
				rec.last = c.Badge
				rec.mu.Unlock()
			})
		}()
	}
	for g := 0; g < setters; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := 0; k < sets; k++ {
				src.Set(desktop.TileContent{Badge: fmt.Sprintf("%d-%d", g, k)})
			}
		}(g)
	}
	wg.Wait()

	cur, ok := src.Current()
	if !ok {
		t.Fatal("состояния нет")
	}
	for i, rec := range recs {
		rec.mu.Lock()
		last := rec.last
		rec.mu.Unlock()
		if last != cur.Badge {
			t.Errorf("подписчик %d закончил на %q, Current — %q", i, last, cur.Badge)
		}
	}
}
