package engine

// popuparena_test.go — арена попапов (ADR-0009, приёмка 5.3): картинка
// каждого хостового попапа побайтно равна рендеру в свежий клон холста; при
// трёх открытых меню память попапов — одна заготовка размером с самое
// большое меню плюс картинки (прежде — холст с двумя буферами на каждое);
// без попапов и без хоста заготовка отпущена.

import (
	"bytes"
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v4/core/overlay"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// arenaScene — движок с тремя раскрытыми списками разной ширины и длины и
// стоком попапов, запоминающим последний набор кадров.
type arenaScene struct {
	e      *Engine
	dds    []*widget.Dropdown
	mu     sync.Mutex
	frames []PopupFrame
}

func newArenaScene(t *testing.T, scale float64) *arenaScene {
	t.Helper()
	e := New(800, 600, 20)
	t.Cleanup(e.Stop)
	if scale != 1 {
		e.SetScale(scale)
	}
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 800, 600))
	s := &arenaScene{e: e}
	// Порядок обхода: маленький, большой, средний — заготовка растёт и
	// переиспользуется меньшими.
	specs := []struct {
		r     image.Rectangle
		items []string
	}{
		{image.Rect(20, 20, 140, 46), []string{"а", "б"}},
		{image.Rect(200, 20, 520, 46), []string{"Первый длинный пункт", "Второй", "Третий", "Четвёртый", "Пятый", "Шестой", "Седьмой"}},
		{image.Rect(560, 20, 760, 46), []string{"Альфа", "Бета", "Гамма", "Дельта"}},
	}
	for _, sp := range specs {
		dd := widget.NewDropdown(sp.items...)
		dd.SetBounds(sp.r)
		root.AddChild(dd)
		s.dds = append(s.dds, dd)
	}
	e.SetRoot(root)
	e.SetPopupSink(func(fr []PopupFrame) {
		s.mu.Lock()
		s.frames = append(s.frames[:0], fr...)
		s.mu.Unlock()
	})
	return s
}

func (s *arenaScene) open(v bool) {
	for _, dd := range s.dds {
		dd.SetOpen(v)
	}
}

// checkIdentity — каждый кадр попапа равен рендеру в свежий клон.
func (s *arenaScene) checkIdentity(t *testing.T, name string) {
	t.Helper()
	s.mu.Lock()
	frames := append([]PopupFrame(nil), s.frames...)
	s.mu.Unlock()
	if len(frames) != len(s.dds) {
		t.Fatalf("%s: попапов %d, ждали %d", name, len(frames), len(s.dds))
	}
	for i, dd := range s.dds {
		fr := frames[i]
		want := overlay.RenderOverlay(s.e.canvas, dd, fr.Rect)
		if fr.Img.Rect != want.Rect {
			t.Fatalf("%s: попап %d размером %v, свежий клон %v", name, i, fr.Img.Rect, want.Rect)
		}
		if !bytes.Equal(fr.Img.Pix, want.Pix) {
			t.Errorf("%s: попап %d расходится со свежим клоном", name, i)
		}
	}
}

func TestPopupArena_IdentityWithFreshClone(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25} {
		t.Run(scaleName(k), func(t *testing.T) {
			s := newArenaScene(t, k)
			s.open(true)
			s.e.renderFrame()
			s.checkIdentity(t, "три меню")
			// Наведение в большом меню: перерисовка в заготовку, где лежал
			// прошлый попап.
			big := s.dds[1].OverlayBounds()
			s.dds[1].OnMouseMove(big.Min.X+10, big.Min.Y+40)
			s.e.Invalidate()
			s.e.renderFrame()
			s.checkIdentity(t, "наведение")
		})
	}
}

func TestPopupArena_MemoryWithThreeMenus(t *testing.T) {
	testreset.Globals(t)
	s := newArenaScene(t, 1)
	s.open(true)
	s.e.renderFrame()
	s.mu.Lock()
	frames := append([]PopupFrame(nil), s.frames...)
	s.mu.Unlock()
	if len(frames) != 3 {
		t.Fatalf("попапов %d, ждали 3", len(frames))
	}
	maxArea, imgs, old := 0, 0, 0
	for _, fr := range frames {
		a := fr.Img.Rect.Dx() * fr.Img.Rect.Dy() * 4
		maxArea = max(maxArea, a)
		imgs += a
		old += 3 * a // прежде: холст (front и back) и картинка на каждое меню
	}
	got := s.e.Stats().PopupBytes
	if want := uint64(maxArea + imgs); got != want {
		t.Errorf("PopupBytes = %d, ждали заготовку %d + картинки %d = %d", got, maxArea, imgs, want)
	}
	if got >= uint64(old) {
		t.Errorf("PopupBytes = %d не меньше прежней схемы (%d)", got, old)
	}
	t.Logf("три меню: арена %d байт (заготовка %d + картинки %d), прежняя схема %d", got, maxArea, imgs, old)

	// Все меню закрыты — заготовка отпущена.
	s.open(false)
	s.e.renderFrame()
	if got := s.e.Stats().PopupBytes; got != 0 {
		t.Errorf("после закрытия меню PopupBytes = %d, ждали 0", got)
	}
	if s.e.ov.Arena().Canvas() != nil {
		t.Error("после закрытия меню заготовка арены не отпущена")
	}
}

// Хост попапов снят — ближайший кадр отпускает заготовку.
func TestPopupArena_ReleasedWhenSinkRemoved(t *testing.T) {
	testreset.Globals(t)
	s := newArenaScene(t, 1)
	s.open(true)
	s.e.renderFrame()
	if s.e.ov.Arena().Canvas() == nil {
		t.Fatal("заготовка арены не заведена при открытых меню")
	}
	s.e.SetPopupSink(nil)
	s.e.renderFrame()
	if s.e.ov.Arena().Canvas() != nil || s.e.Stats().PopupBytes != 0 {
		t.Errorf("после снятия хоста заготовка %v, PopupBytes %d", s.e.ov.Arena().Canvas() != nil, s.e.Stats().PopupBytes)
	}
}
