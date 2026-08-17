package tests

// Регрессионные тесты находок аудита SEC-7…SEC-11 и PERF-10:
//
//	SEC-7  — безлимитная рекурсия в XAML-парсере/билдере и в TreeView;
//	SEC-8  — path traversal через атрибуты-пути в XAML;
//	SEC-9  — декомпрессионная бомба (заголовок PNG с гигантскими размерами);
//	SEC-10 — размеры холста без валидации в engine.New/SetResolution;
//	SEC-11 — TreeView не отписывался от прежнего ItemsSource;
//	PERF-10 — visibleNodes пересобирался на каждый Draw/ввод.

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	dg "github.com/oops1/headless-gui/v3/widget/datagrid"
	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// ─── SEC-7: предел вложенности XAML ────────────────────────────────────────

// TestXAMLDeepNestingRejected — разметка глубиной 10 000 уровней возвращает
// ошибку, а не исчерпывает стек. Переполнение стека в Go нельзя перехватить
// через recover: процесс умирает целиком, поэтому «не паникует» здесь
// означает «тест вообще дошёл до конца».
func TestXAMLDeepNestingRejected(t *testing.T) {
	const depth = 10000
	var b strings.Builder
	b.WriteString(`<Canvas Width="100" Height="100">`)
	for i := 0; i < depth; i++ {
		b.WriteString("<Panel>")
	}
	for i := 0; i < depth; i++ {
		b.WriteString("</Panel>")
	}
	b.WriteString("</Canvas>")

	_, _, err := widget.LoadUIFromXAML([]byte(b.String()))
	if err == nil {
		t.Fatal("XAML глубиной 10000 разобран без ошибки — предел вложенности не работает")
	}
	if !strings.Contains(err.Error(), "вложенность") {
		t.Fatalf("ожидалась ошибка о вложенности, получено: %v", err)
	}
}

// TestXAMLShallowNestingStillWorks — предел не мешает нормальной вёрстке:
// разумная вложенность (30 уровней) по-прежнему собирается.
func TestXAMLShallowNestingStillWorks(t *testing.T) {
	const depth = 30
	var b strings.Builder
	b.WriteString(`<Canvas Width="200" Height="200">`)
	for i := 0; i < depth; i++ {
		b.WriteString(`<Panel Background="Transparent">`)
	}
	b.WriteString(`<Label Text="дно"/>`)
	for i := 0; i < depth; i++ {
		b.WriteString("</Panel>")
	}
	b.WriteString("</Canvas>")

	root, _, err := widget.LoadUIFromXAML([]byte(b.String()))
	if err != nil {
		t.Fatalf("вложенность %d должна собираться: %v", depth, err)
	}
	if root == nil {
		t.Fatal("корневой виджет nil")
	}
}

// ─── SEC-7: циклическая модель TreeView ────────────────────────────────────

// cycNode — узел иерархической модели, который можно замкнуть на себя.
type cycNode struct {
	Name  string
	Kids  []*cycNode
}

// TestTreeViewCyclicModelNoStackOverflow — модель, в которой узел ссылается
// сам на себя (и пара узлов ссылается друг на друга), строится в конечное
// дерево, а не уходит в бесконечную рекурсию.
func TestTreeViewCyclicModelNoStackOverflow(t *testing.T) {
	self := &cycNode{Name: "self"}
	self.Kids = []*cycNode{self} // прямой цикл

	a := &cycNode{Name: "a"}
	bn := &cycNode{Name: "b"}
	a.Kids = []*cycNode{bn}
	bn.Kids = []*cycNode{a} // взаимный цикл

	oc := dg.NewObservableCollectionFrom([]interface{}{self, a})

	tv := treeview.New()
	tv.SetItemTemplate(&treeview.HierarchicalDataTemplate{
		HeaderPath:      "Name",
		ItemsSourcePath: "Kids",
	})
	tv.SetItemsSource(oc) // не должно уйти в бесконечную рекурсию

	roots := tv.Roots()
	if len(roots) != 2 {
		t.Fatalf("корней %d, ожидалось 2", len(roots))
	}
	if got := countNodes(roots); got > 16 {
		t.Fatalf("дерево из циклической модели разрослось до %d узлов — цикл не отсечён", got)
	}
	// Прямой цикл: повторное вхождение узла в дерево создаётся (ссылка в
	// модели есть, молча терять её нельзя), но НЕ раскрывается.
	if n := len(roots[0].Children); n != 1 {
		t.Fatalf("у self ожидался 1 потомок-повтор, получено %d", n)
	}
	if n := len(roots[0].Children[0].Children); n != 0 {
		t.Fatalf("повторное вхождение узла раскрыто (%d потомков) — цикл не отсечён", n)
	}
	// Взаимный цикл a → b → a: раскрывается ровно до повторного a.
	if n := len(roots[1].Children); n != 1 {
		t.Fatalf("у a ожидался 1 потомок (b), получено %d", n)
	}
	if n := len(roots[1].Children[0].Children[0].Children); n != 0 {
		t.Fatal("взаимный цикл a→b→a раскрыт дальше повтора")
	}
}

func countNodes(items []*treeview.TreeViewItem) int {
	n := 0
	for _, it := range items {
		n += 1 + countNodes(it.Children)
	}
	return n
}

// ─── SEC-8: path traversal через атрибуты XAML ─────────────────────────────

// TestXAMLResourceTraversalBlocked — при загрузке ИЗ ФАЙЛА пути ресурсов
// удерживаются внутри каталога разметки: «../secret.png» и абсолютный путь
// не читаются, а соседний файл — читается.
func TestXAMLResourceTraversalBlocked(t *testing.T) {
	base := t.TempDir()
	uiDir := filepath.Join(base, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// «Секрет» лежит рядом с каталогом разметки, но вне его.
	secret := filepath.Join(base, "secret.png")
	writeTinyPNG(t, secret)
	// Легальный ресурс — внутри каталога разметки.
	writeTinyPNG(t, filepath.Join(uiDir, "ok.png"))

	const tmpl = `<Canvas Width="200" Height="200">
	  <Image  x:Name="imgBad"  Left="0" Top="0"  Width="16" Height="16" Source="%s"/>
	  <Image  x:Name="imgOk"   Left="0" Top="20" Width="16" Height="16" Source="ok.png"/>
	  <Button x:Name="btnBad"  Left="0" Top="40" Width="60" Height="20" Content="b" IconSource="%s"/>
	  <Button x:Name="btnOk"   Left="0" Top="60" Width="60" Height="20" Content="o" IconSource="ok.png"/>
	  <Panel  x:Name="pnlBad"  Left="0" Top="80" Width="60" Height="20" BackgroundImage="%s"/>
	  <Panel  x:Name="pnlOk"   Left="0" Top="100" Width="60" Height="20" BackgroundImage="ok.png"/>
	</Canvas>`

	for _, bad := range []string{"../secret.png", `..\secret.png`, secret} {
		xaml := strings.ReplaceAll(tmpl, "%s", bad)
		_, reg, err := widget.LoadUIFromXAMLWithBase([]byte(xaml), uiDir)
		if err != nil {
			t.Fatalf("путь %q: неожиданная ошибка разбора: %v", bad, err)
		}

		if iw, ok := reg["imgBad"].(*widget.ImageWidget); !ok {
			t.Fatalf("imgBad не ImageWidget")
		} else if iw.Image() != nil {
			t.Fatalf("Image Source=%q прочитан за пределами baseDir", bad)
		}
		if iw, ok := reg["imgOk"].(*widget.ImageWidget); !ok {
			t.Fatalf("imgOk не ImageWidget")
		} else if iw.Image() == nil {
			t.Fatal("Image Source=\"ok.png\" не загружен — сломан обычный случай")
		}

		if b, ok := reg["btnBad"].(*widget.Button); !ok {
			t.Fatalf("btnBad не Button")
		} else if b.Icon != nil {
			t.Fatalf("Button IconSource=%q прочитан за пределами baseDir", bad)
		}
		if b, ok := reg["btnOk"].(*widget.Button); !ok {
			t.Fatalf("btnOk не Button")
		} else if b.Icon == nil {
			t.Fatal("Button IconSource=\"ok.png\" не загружен — сломан обычный случай")
		}

		if p, ok := reg["pnlBad"].(*widget.Panel); !ok {
			t.Fatalf("pnlBad не Panel")
		} else if p.BackgroundImage != nil {
			t.Fatalf("Panel BackgroundImage=%q прочитан за пределами baseDir", bad)
		}
		if p, ok := reg["pnlOk"].(*widget.Panel); !ok {
			t.Fatalf("pnlOk не Panel")
		} else if p.BackgroundImage == nil {
			t.Fatal("Panel BackgroundImage=\"ok.png\" не загружен — сломан обычный случай")
		}
	}
}

// TestXAMLImageResolvedFromBaseDir — <Image Source="…"> резолвится от каталога
// XAML-файла, как иконки кнопок и SVGIcon. Раньше путь уходил в SetSource как
// есть и относительный резолвился от текущего каталога процесса.
func TestXAMLImageResolvedFromBaseDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "img")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTinyPNG(t, filepath.Join(sub, "pic.png"))

	xamlPath := filepath.Join(dir, "ui.xaml")
	xaml := `<Canvas Width="100" Height="100">
	  <Image x:Name="pic" Left="0" Top="0" Width="16" Height="16" Source="img/pic.png"/>
	</Canvas>`
	if err := os.WriteFile(xamlPath, []byte(xaml), 0o644); err != nil {
		t.Fatal(err)
	}

	_, reg, err := widget.LoadUIFromXAMLFile(xamlPath)
	if err != nil {
		t.Fatalf("LoadUIFromXAMLFile: %v", err)
	}
	iw, ok := reg["pic"].(*widget.ImageWidget)
	if !ok {
		t.Fatal("pic не ImageWidget")
	}
	if iw.Image() == nil {
		t.Fatal("Image Source=\"img/pic.png\" не разрешён относительно каталога XAML-файла")
	}
}

// ─── SEC-9: декомпрессионная бомба ─────────────────────────────────────────

// TestImageBombRejectedByHeader — PNG, у которого в заголовке заявлено
// 40000×40000 (1.6 Гпикс ≈ 6.4 ГБ в RGBA), отвергается по заголовку.
// Сам файл — несколько десятков байт, декодировать нечего: важно, что до
// аллокации дело не доходит.
func TestImageBombRejectedByHeader(t *testing.T) {
	dir := t.TempDir()
	bomb := filepath.Join(dir, "bomb.png")
	if err := os.WriteFile(bomb, pngHeaderOnly(40000, 40000), 0o644); err != nil {
		t.Fatal(err)
	}

	// Заголовок должен читаться штатными средствами — иначе тест проверяет
	// не то (отказ по «битому файлу», а не по размеру).
	f, err := os.Open(bomb)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, cerr := image.DecodeConfig(f)
	f.Close()
	if cerr != nil {
		t.Fatalf("заголовок PNG не читается: %v", cerr)
	}
	if cfg.Width != 40000 || cfg.Height != 40000 {
		t.Fatalf("в заголовке %dx%d, ожидалось 40000x40000", cfg.Width, cfg.Height)
	}

	if _, err := widget.LoadImageFile(bomb); err == nil {
		t.Fatal("LoadImageFile принял изображение 40000x40000")
	} else if !strings.Contains(err.Error(), "превышает предел") {
		t.Fatalf("ожидался отказ по пределу площади, получено: %v", err)
	}

	iw := widget.NewImageWidget()
	if err := iw.SetSource(bomb); err == nil {
		t.Fatal("ImageWidget.SetSource принял изображение 40000x40000")
	}

	eng := engine.New(64, 64, 10)
	if err := eng.SetBackgroundFile(bomb); err == nil {
		t.Fatal("engine.SetBackgroundFile принял изображение 40000x40000")
	}
}

// TestImageWithinLimitStillDecodes — обычная картинка проходит проверку.
func TestImageWithinLimitStillDecodes(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "ok.png")
	writeTinyPNG(t, ok)
	if _, err := widget.LoadImageFile(ok); err != nil {
		t.Fatalf("нормальная картинка отвергнута: %v", err)
	}
}

// TestMaxImagePixelsConfigurable — предел настраивается и восстанавливается.
func TestMaxImagePixelsConfigurable(t *testing.T) {
	orig := widget.MaxImagePixels()
	defer widget.SetMaxImagePixels(orig)

	widget.SetMaxImagePixels(4) // 2x2 пикселя
	dir := t.TempDir()
	p := filepath.Join(dir, "small.png")
	writeTinyPNG(t, p) // 8x8 — больше нового предела
	if _, err := widget.LoadImageFile(p); err == nil {
		t.Fatal("предел MaxImagePixels не применяется")
	}

	widget.SetMaxImagePixels(0) // 0 → значение по умолчанию
	if widget.MaxImagePixels() != orig {
		t.Fatalf("SetMaxImagePixels(0) дал %d, ожидалось значение по умолчанию %d",
			widget.MaxImagePixels(), orig)
	}
}

// ─── SEC-10: границы размера холста ────────────────────────────────────────

// TestEngineNewClampsSize — некорректные и неподъёмные размеры приводятся к
// допустимым, без паники и без аллокации гигабайтов.
func TestEngineNewClampsSize(t *testing.T) {
	cases := []struct{ w, h int }{
		{-5, 0},
		{0, 0},
		{-1, -1},
		{1 << 20, 1 << 20},
		{1 << 30, 4},
	}
	for _, c := range cases {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)

		eng := engine.New(c.w, c.h, 10)
		w, h := eng.CanvasSize()

		runtime.ReadMemStats(&after)

		if w < 1 || h < 1 {
			t.Fatalf("New(%d, %d): размер холста %dx%d — меньше минимального", c.w, c.h, w, h)
		}
		if int64(w)*int64(h) > engine.MaxCanvasPixels {
			t.Fatalf("New(%d, %d): площадь холста %dx%d выше предела", c.w, c.h, w, h)
		}
		if w > engine.MaxCanvasSide || h > engine.MaxCanvasSide {
			t.Fatalf("New(%d, %d): сторона холста %dx%d выше предела", c.w, c.h, w, h)
		}
		// Порог намеренно щедрый: движок тянет за собой шрифты и кэши.
		// Он ловит именно «выделили гигабайты под растр».
		if grew := int64(after.TotalAlloc - before.TotalAlloc); grew > 1<<30 {
			t.Fatalf("New(%d, %d) выделил %d байт", c.w, c.h, grew)
		}
	}
}

// TestEngineSetResolutionClampsSize — SetResolution держит те же границы.
func TestEngineSetResolutionClampsSize(t *testing.T) {
	eng := engine.New(320, 240, 10)

	eng.SetResolution(-100, -100)
	if w, h := eng.CanvasSize(); w < 1 || h < 1 {
		t.Fatalf("SetResolution(-100,-100) дал %dx%d", w, h)
	}

	eng.SetResolution(1<<20, 1<<20)
	w, h := eng.CanvasSize()
	if int64(w)*int64(h) > engine.MaxCanvasPixels || w > engine.MaxCanvasSide || h > engine.MaxCanvasSide {
		t.Fatalf("SetResolution(1<<20,1<<20) дал %dx%d — вне границ", w, h)
	}

	// Нормальный размер по-прежнему проходит один в один.
	eng.SetResolution(800, 600)
	if w, h := eng.CanvasSize(); w != 800 || h != 600 {
		t.Fatalf("SetResolution(800,600) дал %dx%d", w, h)
	}
}

// ─── SEC-11: TreeView отписывается от прежнего ItemsSource ─────────────────

// TestTreeViewItemsSourceUnsubscribe — перебиндовка снимает подписку с
// прежней коллекции, а не копит обработчики.
func TestTreeViewItemsSourceUnsubscribe(t *testing.T) {
	tv := treeview.New()
	tv.SetItemTemplate(&treeview.HierarchicalDataTemplate{HeaderPath: "Name"})

	first := dg.NewObservableCollection()
	tv.SetItemsSource(first)
	if got := first.HandlerCount(); got != 1 {
		t.Fatalf("после первой привязки подписчиков %d, ожидался 1", got)
	}

	const n = 5
	var last *dg.ObservableCollection
	for i := 0; i < n; i++ {
		last = dg.NewObservableCollection()
		tv.SetItemsSource(last)
	}

	if got := first.HandlerCount(); got != 0 {
		t.Fatalf("на прежнем источнике осталось %d подписчиков — утечка SEC-11", got)
	}
	if got := last.HandlerCount(); got != 1 {
		t.Fatalf("на текущем источнике %d подписчиков, ожидался 1", got)
	}

	// Отвязка (nil) тоже снимает подписку.
	tv.SetItemsSource(nil)
	if got := last.HandlerCount(); got != 0 {
		t.Fatalf("после SetItemsSource(nil) осталось %d подписчиков", got)
	}
}

// ─── PERF-10: кэш плоского списка видимых узлов ────────────────────────────

// TestTreeViewVisibleCacheCorrect — кэш инвалидируется на всех структурных
// изменениях: раскрытие, добавление и удаление узлов видны сразу.
func TestTreeViewVisibleCacheCorrect(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))

	root := treeview.NewItem("Root")
	child := treeview.NewItem("Child")
	root.AddChild(child)
	tw.AddRoot(root)

	// Свёрнут: видна только строка Root — клик по второй строке ни во что
	// не попадает и выделение не меняется.
	tw.OnMouseMove(50, 11)
	tw.OnMouseButton(widget.MouseEvent{X: 50, Y: 11, Button: widget.MouseLeft, Pressed: true})
	if got := tw.SelectedNode(); got != root {
		t.Fatalf("выделен %v, ожидался Root", got)
	}

	// Раскрываем через API — кэш обязан протухнуть.
	tw.Tree.ExpandItem(root)
	tw.OnMouseButton(widget.MouseEvent{X: 50, Y: 33, Button: widget.MouseLeft, Pressed: true})
	if got := tw.SelectedNode(); got != child {
		t.Fatalf("после раскрытия выделен %v, ожидался Child", got)
	}

	// Добавление потомка через TreeViewItem.AddChild тоже инвалидирует кэш.
	sub := treeview.NewItem("Sub")
	child.AddChild(sub)
	tw.Tree.ExpandItem(child)
	tw.OnMouseButton(widget.MouseEvent{X: 50, Y: 55, Button: widget.MouseLeft, Pressed: true})
	if got := tw.SelectedNode(); got != sub {
		t.Fatalf("после AddChild выделен %v, ожидался Sub", got)
	}

	// Удаление узлов — тоже: список перестаёт быть прокручиваемым,
	// а это считается по длине того самого flat-списка.
	big := treeview.NewItem("Big")
	for i := 0; i < 30; i++ {
		big.AddChild(treeview.NewItem("x"))
	}
	tw.ClearRoots()
	tw.AddRoot(big)
	tw.Tree.ExpandItem(big)
	if !tw.Tree.WheelScroll(false) {
		t.Fatal("31 строка в окне высотой 200px должна прокручиваться")
	}
	big.ClearChildren()
	if tw.Tree.WheelScroll(false) {
		t.Fatal("после ClearChildren список всё ещё прокручивается — кэш не инвалидирован")
	}
}

// TestTreeViewVisibleCacheNoRealloc — повторный ввод без изменений модели не
// пересобирает плоский список. Раньше каждый mousemove обходил всё дерево и
// аллоцировал новый срез.
func TestTreeViewVisibleCacheNoRealloc(t *testing.T) {
	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))

	root := treeview.NewItem("Root")
	for i := 0; i < 200; i++ {
		root.AddChild(treeview.NewItem("узел"))
	}
	root.Expanded = true
	tw.AddRoot(root) // AddRoot помечает кэш устаревшим

	tw.OnMouseMove(50, 11) // прогрев: первая сборка кэша

	allocs := testing.AllocsPerRun(50, func() {
		tw.OnMouseMove(50, 11) // та же точка: hover не меняется
	})
	if allocs > 4 {
		t.Fatalf("mousemove без изменений модели даёт %.0f аллокаций — flat-список пересобирается", allocs)
	}
}

// ─── Вспомогательное ───────────────────────────────────────────────────────

// writeTinyPNG кладёт по пути path валидный PNG 8×8.
func writeTinyPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pngHeaderOnly собирает PNG из сигнатуры и одного IHDR с заданными
// размерами. Пиксельных данных нет — их и не должно понадобиться:
// проверка обязана сработать на заголовке.
func pngHeaderOnly(w, h uint32) []byte {
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})

	var ihdr bytes.Buffer
	binary.Write(&ihdr, binary.BigEndian, w)
	binary.Write(&ihdr, binary.BigEndian, h)
	ihdr.Write([]byte{
		8, // bit depth
		6, // color type: RGBA
		0, // compression
		0, // filter
		0, // interlace
	})

	binary.Write(&out, binary.BigEndian, uint32(ihdr.Len()))
	chunk := append([]byte("IHDR"), ihdr.Bytes()...)
	out.Write(chunk)
	binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return out.Bytes()
}
