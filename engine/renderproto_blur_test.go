package engine

// renderproto_blur_test.go — три варианта размытой подложки (BlurBehind) в
// тайловой модели рендера (задача 4.3, решение по этапу 4а):
//
//   - blurBarrier — барьер: кадр делится на сегменты, размытие идёт на полном
//     рабочем буфере, тайлы сегментов рисуются все (renderSegmented);
//   - blurHalo — ореол: тайл под слоем размытия рисует команды кадра в холст
//     с запасом по периметру и размывает сам; запас выбран так, чтобы
//     результат в центре не зависел от края холста, а сетка уменьшения
//     подложки совпадала с движком; барьера и рабочего буфера нет;
//   - blurCache — кэш размытия: результат BlurBehind хранится по ключу из
//     команды и команд под ней; пока содержимое под слоем не менялось, тайлы
//     блитят готовый растр, кадр идёт без читающих команд.
//
// Снимки панели задач по вариантам пишет TestRenderProto_BlurVariantsImages
// (переменная окружения PROTO_OUT=каталог).

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v4/core/overlay"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/internal/workpool"
	"github.com/oops1/headless-gui/v4/output"
)

type protoBlurMode uint8

const (
	blurBarrier protoBlurMode = iota
	blurHalo
	blurCache
)

var protoBlurModeNames = [...]string{"барьер", "ореол", "кэш размытия"}

// ─── Ореол ───────────────────────────────────────────────────────────────────

// haloFor — запас ореола для радиуса размытия: два прохода box-blur радиусом
// radius/4 на уменьшенной в 4 раза подложке плюс неполные блоки по краям;
// кратен 4, чтобы сетка уменьшения в холсте ореола совпала с сеткой движка.
func protoHaloFor(radius int) int {
	const k = 4
	sr := radius / k
	if sr < 1 {
		sr = 1
	}
	h := (2*sr+1)*k + k + k
	return (h + k - 1) &^ (k - 1)
}

// prepareHalo находит область размытия кадра, запас ореола и пересчитывает
// хэши тайлов под размытием по командам в пределах их ореола: результат
// такого тайла зависит и от соседей.
func (f *protoFrame) prepareHalo() {
	f.blurRect = image.Rectangle{}
	h := 0
	for i := range f.cmds {
		c := &f.cmds[i]
		if !c.readsPixels || c.op != opBlurBehind {
			continue
		}
		r := c.rect.Intersect(c.clip)
		if c.round != (image.Rectangle{}) {
			r = r.Intersect(c.round)
		}
		f.blurRect = f.blurRect.Union(r)
		if hh := protoHaloFor(c.r); hh > h {
			h = hh
		}
	}
	if h != f.halo {
		f.halo = h
		f.haloPool = nil
	}
	if f.blurRect.Empty() {
		return
	}
	for i := range f.hash {
		tx, ty := i%f.tilesX, i/f.tilesX
		tile := image.Rect(tx*f.ts, ty*f.ts, (tx+1)*f.ts, (ty+1)*f.ts)
		if !tile.Overlaps(f.blurRect) {
			continue
		}
		halo := tile.Inset(-h).Intersect(image.Rect(0, 0, f.w, f.h))
		hv := uint64(protoFNVOffset)
		for ci := range f.cmds {
			if f.cmds[ci].bbox.Overlaps(halo) {
				hv = protoMixU64(hv, f.cmds[ci].hash)
			}
		}
		f.hash[i] = hv
	}
}

// haloWorker выдаёт холст ореола заданного размера: ореол обрезается кадром
// (размытие движка зажимает подложку краем кадра, холст ореола обязан
// зажимать её там же), поэтому у краевых тайлов свои размеры.
//
// Возвращает и канал пула: вернуть холст надо в него же, а читать карту пула
// без замка нельзя — соседний тайл другого размера может её пополнять.
func (f *protoFrame) haloWorker(size image.Point) (*protoWorker, chan *protoWorker) {
	f.haloMu.Lock()
	if f.haloPool == nil {
		f.haloPool = make(map[image.Point]chan *protoWorker)
	}
	ch := f.haloPool[size]
	if ch == nil {
		ch = make(chan *protoWorker, cap(f.workers))
		for i := 0; i < cap(f.workers); i++ {
			c := f.haloSrc.CloneForSize(size.X, size.Y, 1, nil)
			c.InheritWallpaper(f.haloSrc)
			ch <- &protoWorker{c: c, ctx: &overlay.Translating{Inner: c}}
		}
		f.haloPool[size] = ch
	}
	f.haloMu.Unlock()
	return <-ch, ch
}

// runHaloTile рисует тайл под размытием: все команды кадра в пределах ореола
// (обрезанного кадром) идут в холст размером с ореол, размытие читает
// подложку из него же, во front попадает только середина.
func (f *protoFrame) runHaloTile(i int, tile image.Rectangle) {
	h := f.halo
	halo := tile.Inset(-h).Intersect(image.Rect(0, 0, f.w, f.h))
	w, pool := f.haloWorker(halo.Size())
	defer func() { pool <- w }()
	c, t := w.c, w.ctx
	t.DX, t.DY = halo.Min.X, halo.Min.Y
	c.ClearRoundClip()
	c.ClearClip()
	cur, round := image.Rect(0, 0, f.w, f.h), protoRound{}
	t.SetClip(cur)
	for ci := range f.cmds {
		if f.cmds[ci].bbox.Overlaps(halo) {
			protoReplay(t, &cur, &round, &f.cmds[ci])
		}
	}
	c.ClearRoundClip()
	back := c.Back()
	center := tile.Sub(halo.Min)
	if !protoTileEqualAt(back, center, f.front, tile) {
		protoCopyTile(f.front, tile.Min, back, center.Min, tile.Size())
		f.tileDirty[i] = true
	}
}

// ─── Кэш размытия ────────────────────────────────────────────────────────────

// resolveBlurCache заменяет каждую BlurBehind готовым растром: ключ — сама
// команда и команды под ней (в области чтения); промах считает размытие на
// холсте размером с область чтения тем же кодом, что и движок.
func (f *protoFrame) resolveBlurCache(cmds []protoCmd) []protoCmd {
	if f.blurCache == nil {
		f.blurCache = make(map[uint64]*image.RGBA)
	}
	out := make([]protoCmd, len(cmds))
	copy(out, cmds)
	frame := image.Rect(0, 0, f.w, f.h)
	for i := range out {
		c := &out[i]
		if !c.readsPixels || c.op != opBlurBehind {
			continue
		}
		pr := c.rect.Intersect(c.clip).Intersect(frame)
		if pr.Empty() {
			continue
		}
		radius := c.r
		if radius <= 0 {
			radius = 1
		}
		src := pr.Inset(-radius).Intersect(frame)
		key := protoMixU64(protoFNVOffset, c.hash)
		for j := 0; j < i; j++ {
			if cmds[j].bbox.Overlaps(src) {
				key = protoMixU64(key, cmds[j].hash)
			}
		}
		img, ok := f.blurCache[key]
		if !ok {
			f.blurMisses++
			img = f.renderBlurRegion(cmds[:i], c, src, pr)
			f.blurCache[key] = img
		} else {
			f.blurHits++
		}
		*c = protoCmd{op: opBlitPixels, rgba: img, rect: img.Bounds(), x: pr.Min.X, y: pr.Min.Y, flag: true,
			clip: c.clip, round: c.round, roundR: c.roundR, bbox: pr}
		c.computeHash()
	}
	return out
}

// renderBlurRegion — промах кэша: команды под слоем рисуются в холст размером
// с область чтения, там же выполняется размытие, результат в области слоя
// снимается в картинку.
func (f *protoFrame) renderBlurRegion(before []protoCmd, blur *protoCmd, src, pr image.Rectangle) *image.RGBA {
	// Холст области переиспользуется между промахами того же размера.
	c := f.blurRegion
	if c == nil || c.W != src.Dx() || c.H != src.Dy() {
		c = f.haloSrc.CloneForSize(src.Dx(), src.Dy(), 1, nil)
		c.InheritWallpaper(f.haloSrc)
		f.blurRegion = c
	}
	c.ClearRoundClip()
	c.ClearClip()
	t := &overlay.Translating{Inner: c, DX: src.Min.X, DY: src.Min.Y}
	cur, round := image.Rect(0, 0, f.w, f.h), protoRound{}
	t.SetClip(cur)
	for i := range before {
		if before[i].bbox.Overlaps(src) {
			protoReplay(t, &cur, &round, &before[i])
		}
	}
	protoReplay(t, &cur, &round, blur)
	c.ClearRoundClip()
	img := image.NewRGBA(image.Rect(0, 0, pr.Dx(), pr.Dy()))
	protoCopyTile(img, image.Point{}, c.Back(), pr.Min.Sub(src.Min), pr.Size())
	return img
}

// ─── Тесты тождественности по вариантам ──────────────────────────────────────

func protoBlurFrame(eng *Engine, mode protoBlurMode) *protoFrame {
	f := newProtoFrame(eng.canvas, true, false)
	f.mode = mode
	return f
}

// Три варианта размытия дают кадр рабочего стола, равный кадру движка; тик
// часов (другая строка в области панели) — тоже.
func TestRenderProto_BlurVariantsMatchDirect(t *testing.T) {
	testreset.Globals(t)
	sc := buildPipelineDesktop()
	defer sc.closeAll()
	defer sc.eng.Stop()
	want := protoDirectFrame(sc.eng)
	rec := newProtoRecorder(sc.eng.canvas)
	for mode := blurBarrier; mode <= blurCache; mode++ {
		f := protoBlurFrame(sc.eng, mode)
		cmds := protoRecord(rec, sc.eng.root)
		f.render(cmds)
		if tiles, px := protoMismatch(f.front, want); px != 0 {
			t.Errorf("%s: кадр отличается от движка на %d пикселей в %d тайлах", protoBlurModeNames[mode], px, tiles)
		}
		// Повтор без изменений.
		f.render(protoRecord(rec, sc.eng.root))
		if f.dirty != 0 {
			t.Errorf("%s: кадр без изменений дал %d грязных тайлов", protoBlurModeNames[mode], f.dirty)
		}
		t.Logf("%s: без изменений пропущено %d из %d тайлов, нарисовано %d; кэш размытия: попаданий %d, промахов %d",
			protoBlurModeNames[mode], f.skipped, len(f.hash), f.rendered.Load(), f.blurHits, f.blurMisses)
	}
}

// protoClockCmd — индекс команды текста часов (текст в полосе панели задач).
func protoClockCmd(cmds []protoCmd, frameH int) int {
	for i := range cmds {
		if cmds[i].op == opText && cmds[i].bbox.Min.Y > frameH-64 {
			return i
		}
	}
	return -1
}

// ─── Снимки для решения владельца ────────────────────────────────────────────

// protoCrop — увеличенный в k раз вырез кадра.
func protoCrop(src *image.RGBA, r image.Rectangle, k int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx()*k, r.Dy()*k))
	for y := 0; y < r.Dy()*k; y++ {
		for x := 0; x < r.Dx()*k; x++ {
			o := src.PixOffset(r.Min.X+x/k, r.Min.Y+y/k)
			d := out.PixOffset(x, y)
			copy(out.Pix[d:d+4], src.Pix[o:o+4])
		}
	}
	return out
}

// protoDiffMap — карта расхождений: кадр приглушён, отличающиеся пиксели
// красные.
func protoDiffMap(a, b *image.RGBA) *image.RGBA {
	out := image.NewRGBA(a.Bounds())
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			o := a.PixOffset(x, y)
			pa, pb := a.Pix[o:o+4], b.Pix[o:o+4]
			d := out.PixOffset(x, y)
			if string(pa) != string(pb) {
				out.Pix[d], out.Pix[d+1], out.Pix[d+2], out.Pix[d+3] = 255, 0, 0, 255
				continue
			}
			g := uint8((uint32(pa[0]) + uint32(pa[1]) + uint32(pa[2])) / 3 / 2)
			out.Pix[d], out.Pix[d+1], out.Pix[d+2], out.Pix[d+3] = g, g, g, 255
		}
	}
	return out
}

func protoWritePNG(t *testing.T, dir, name string, img image.Image) {
	t.Helper()
	fh, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	if err := png.Encode(fh, img); err != nil {
		t.Fatal(err)
	}
}

// TestRenderProto_BlurVariantsImages пишет в PROTO_OUT вырезы панели задач
// (×3) по вариантам и карты расхождений с кадром движка; без переменной
// пропускается.
func TestRenderProto_BlurVariantsImages(t *testing.T) {
	testreset.Globals(t)
	dir := os.Getenv("PROTO_OUT")
	if dir == "" {
		t.Skip("PROTO_OUT не задан")
	}
	sc := buildPipelineDesktop()
	defer sc.closeAll()
	defer sc.eng.Stop()
	want := protoDirectFrame(sc.eng)
	crop := image.Rect(180, pipeH-64, 620, pipeH)
	protoWritePNG(t, dir, "taskbar_engine.png", protoCrop(want, crop, 3))
	rec := newProtoRecorder(sc.eng.canvas)
	names := [...]string{"barrier", "halo", "cache"}
	for mode := blurBarrier; mode <= blurCache; mode++ {
		f := protoBlurFrame(sc.eng, mode)
		f.render(protoRecord(rec, sc.eng.root))
		protoWritePNG(t, dir, "taskbar_"+names[mode]+".png", protoCrop(f.front, crop, 3))
		protoWritePNG(t, dir, "diff_"+names[mode]+".png", protoDiffMap(want, f.front))
		tiles, px := protoMismatch(f.front, want)
		t.Logf("%s: расхождение %d пикселей в %d тайлах", protoBlurModeNames[mode], px, tiles)
	}
	// Граница тайлов крупно: стык двух тайлов под панелью при ореоле.
	f := protoBlurFrame(sc.eng, blurHalo)
	f.render(protoRecord(rec, sc.eng.root))
	seam := image.Rect(300, pipeH-56, 340, pipeH-16)
	protoWritePNG(t, dir, "seam_halo_x8.png", protoCrop(f.front, seam, 8))
	protoWritePNG(t, dir, "seam_engine_x8.png", protoCrop(want, seam, 8))
}

// ─── Бенчмарки по вариантам ──────────────────────────────────────────────────

func benchBlurFull(b *testing.B, mode protoBlurMode) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	rec := newProtoRecorder(sc.eng.canvas)
	f := protoBlurFrame(sc.eng, mode)
	f.render(protoRecord(rec, sc.eng.root))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range f.prevHash {
			f.prevHash[j] = 0
		}
		f.render(protoRecord(rec, sc.eng.root))
	}
}

// benchBlurTick — тик часов: меняется строка часов в удержанном списке
// (и только она), запись заново не делается — как при Р5.
func benchBlurTick(b *testing.B, mode protoBlurMode) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	rec := newProtoRecorder(sc.eng.canvas)
	f := protoBlurFrame(sc.eng, mode)
	cmds := protoRecord(rec, sc.eng.root)
	f.render(cmds)
	ci := protoClockCmd(cmds, pipeH)
	if ci < 0 {
		b.Skip("строка часов в полосе панели не найдена")
	}
	c := &cmds[ci]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.s = fmt.Sprintf("14:%02d", i%60)
		c.bbox = rec.textBox(c.s, c.x, c.y, c.f1, c.font).Intersect(c.clip)
		c.computeHash()
		f.render(cmds)
	}
	b.ReportMetric(float64(f.rendered.Load()), "rendered-tiles")
	b.ReportMetric(float64(f.dirty), "dirty-tiles")
}

// benchBlurUnder — изменение ПОД размытием (окно тащат под панелью): меняется
// цвет одной полосы обоев, пересекающей область чтения размытия. Худший
// случай для кэша (промах каждый кадр) и ореола (все тайлы над полосой).
func benchBlurUnder(b *testing.B, mode protoBlurMode) {
	sc := buildPipelineDesktop()
	b.Cleanup(sc.closeAll)
	rec := newProtoRecorder(sc.eng.canvas)
	f := protoBlurFrame(sc.eng, mode)
	cmds := protoRecord(rec, sc.eng.root)
	f.render(cmds)
	ci := -1
	for i := range cmds {
		c := &cmds[i]
		if c.op == opFillRect && c.bbox.Min.Y >= pipeH-56 && c.bbox.Min.Y < pipeH-48 && c.bbox.Dx() == pipeW {
			ci = i
			break
		}
	}
	if ci < 0 {
		b.Skip("полоса обоев под панелью не найдена")
	}
	c := &cmds[ci]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.col.R = uint8(20 + i%40)
		c.computeHash()
		// Промах каждый кадр: иначе после 40 кадров цвета повторяются и кэш
		// начинает попадать, а меряется худший случай.
		f.blurCache = nil
		f.render(cmds)
	}
	b.ReportMetric(float64(f.rendered.Load()), "rendered-tiles")
	b.ReportMetric(float64(f.dirty), "dirty-tiles")
}

func BenchmarkProtoBlurUnder_Barrier(b *testing.B) { benchBlurUnder(b, blurBarrier) }
func BenchmarkProtoBlurUnder_Halo(b *testing.B)    { benchBlurUnder(b, blurHalo) }
func BenchmarkProtoBlurUnder_Cache(b *testing.B)   { benchBlurUnder(b, blurCache) }

func BenchmarkProtoBlurFull_Barrier(b *testing.B) { benchBlurFull(b, blurBarrier) }
func BenchmarkProtoBlurFull_Halo(b *testing.B)    { benchBlurFull(b, blurHalo) }
func BenchmarkProtoBlurFull_Cache(b *testing.B)   { benchBlurFull(b, blurCache) }
func BenchmarkProtoBlurTick_Barrier(b *testing.B) { benchBlurTick(b, blurBarrier) }
func BenchmarkProtoBlurTick_Halo(b *testing.B)    { benchBlurTick(b, blurHalo) }
func BenchmarkProtoBlurTick_Cache(b *testing.B)   { benchBlurTick(b, blurCache) }

var _ = workpool.Run
var _ = color.RGBA{}
var _ = output.TileSize
