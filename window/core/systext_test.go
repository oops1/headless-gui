package core

import (
	"os"
	"slices"
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

// ClearType на своём мониторе — субпиксельный слой с порядком подпикселей;
// всё остальное — слой движка по умолчанию.
func TestTextQualityFor(t *testing.T) {
	lcd := engine.TextQuality{Hinting: true, HintingX: true, LCD: true}
	lcdBGR := lcd
	lcdBGR.LCDOrder = engine.LCDOrderBGR
	def := engine.DefaultTextQuality()
	for _, c := range []struct {
		name string
		tr   native.TextRendering
		want engine.TextQuality
	}{
		{"неизвестно", native.TextRendering{}, def},
		{"неизвестно, но ClearType", native.TextRendering{Subpixel: true}, def},
		{"серое", native.TextRendering{Known: true}, def},
		{"ClearType RGB", native.TextRendering{Known: true, Subpixel: true}, lcd},
		{"ClearType BGR", native.TextRendering{Known: true, Subpixel: true, BGR: true}, lcdBGR},
		{"ClearType в RDP", native.TextRendering{Known: true, Subpixel: true, Remote: true}, def},
	} {
		if got := TextQualityFor(c.tr); got != c.want {
			t.Errorf("%s: %+v, ждал %+v", c.name, got, c.want)
		}
	}
}

// qualityEngine — подставной движок окна со слоем качества текста.
type qualityEngine struct {
	postEngine
	q    engine.TextQuality
	sets int
}

func (e *qualityEngine) SetTextQuality(q engine.TextQuality) { e.q, e.sets = q, e.sets+1 }
func (e *qualityEngine) TextQuality() engine.TextQuality     { return e.q }

// textNative — бэкенд, который знает сглаживание текста ОС.
type textNative struct {
	native.NativeWindow
	tr native.TextRendering
	fn func()
}

func (n *textNative) TextRendering() native.TextRendering { return n.tr }
func (n *textNative) SetOnTextRenderingChanged(fn func()) { n.fn = fn }

// Режим, заказанный до Run, ставит слой при подъёме окна; смена настройки ОС
// доходит до движка его горутиной, повтор без перемены движок не трогает;
// выключение снимает подписку и слой оставляет.
func TestWindowTextQualityFromSystem(t *testing.T) {
	eng := &qualityEngine{postEngine: postEngine{root: widget.NewWindow("T", 100, 100)}}
	win := New(eng, "T")
	win.SetTextQualityFromSystem(true)
	if eng.sets != 0 {
		t.Fatalf("до Run слой поставлен %d раз", eng.sets)
	}

	nw := &textNative{tr: native.TextRendering{Known: true, Subpixel: true}}
	win.native = nw
	win.syncSystemText() // как в bringUp
	if !eng.q.LCD || eng.q.LCDOrder != engine.LCDOrderRGB || eng.sets != 1 {
		t.Fatalf("ClearType: слой %+v, поставлен %d раз", eng.q, eng.sets)
	}
	if nw.fn == nil {
		t.Fatal("подписка на смену сглаживания не поднята")
	}

	nw.fn() // WM_SETTINGCHANGE без перемены
	if eng.sets != 1 {
		t.Errorf("повтор без перемены поставил слой ещё раз: %d", eng.sets)
	}
	posts := eng.posts
	nw.tr.Remote = true
	nw.fn() // зашли по RDP
	if eng.q != engine.DefaultTextQuality() {
		t.Errorf("в RDP слой %+v, ждал слой по умолчанию", eng.q)
	}
	if eng.posts == posts {
		t.Error("слой поставлен мимо горутины движка (Post)")
	}
	nw.tr.Remote, nw.tr.BGR = false, true
	nw.fn() // вернулись к своему монитору BGR
	if !eng.q.LCD || eng.q.LCDOrder != engine.LCDOrderBGR {
		t.Errorf("ClearType BGR: слой %+v", eng.q)
	}

	before := eng.q
	win.SetTextQualityFromSystem(false)
	if nw.fn != nil {
		t.Error("режим выключен, подписка осталась")
	}
	if eng.q != before {
		t.Errorf("выключение режима сменило слой: %+v", eng.q)
	}

	// Бэкенд без настройки сглаживания — слой движка по умолчанию.
	eng.q = engine.TextQuality{}
	win.native = &squareNative{rounds: true}
	win.SetTextQualityFromSystem(true)
	if eng.q != engine.DefaultTextQuality() {
		t.Errorf("бэкенд без настройки: слой %+v", eng.q)
	}
}

// Вторичный движок диалога или плавающей панели берёт слой качества текста
// у движка родительского окна.
func TestInheritText_Quality(t *testing.T) {
	parentEng := engine.New(100, 100, 30)
	defer parentEng.Stop()
	q := engine.TextQuality{Hinting: true, HintingX: true, LCD: true, LCDOrder: engine.LCDOrderBGR}
	parentEng.SetTextQuality(q)
	parent := &Window{}
	parent.eng = parentEng

	child := engine.New(50, 50, 30)
	defer child.Stop()
	inheritText(child, parent)
	if got := child.TextQuality(); got != q {
		t.Errorf("вторичный движок: %+v, ждал %+v", got, q)
	}
	inheritText(child, nil) // без родителя — ничего
}

// Вторичный движок диалога получает шрифт приложения и шрифт по умолчанию
// движка окна — диалог в своём окне ОС рисует текст тем же шрифтом.
func TestInheritText_Fonts(t *testing.T) {
	data, err := os.ReadFile("../../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		t.Skipf("шрифт недоступен: %v", err)
	}
	parentEng := engine.New(100, 100, 30)
	defer parentEng.Stop()
	parentEng.RegisterFont("AppUI", data)
	parentEng.SetDefaultFont("AppUI")
	parent := &Window{}
	parent.eng = parentEng

	child := engine.New(50, 50, 30)
	defer child.Stop()
	inheritText(child, parent)
	if !slices.Contains(child.AvailableFonts(), "AppUI") {
		t.Errorf("шрифт приложения не дошёл до диалога: %v", child.AvailableFonts())
	}
}

// Сквозной путь: диалог, вынесенный хостом в своё окно ОС, получает шрифт по
// умолчанию и слой качества текста главного окна (до v4.5 — Roboto и слой по
// умолчанию, что бы ни задало приложение).
func TestHostedDialog_InheritsText(t *testing.T) {
	testreset.Globals(t)
	data, err := os.ReadFile("../../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		t.Skipf("шрифт недоступен: %v", err)
	}
	r := newHostedRig(t, nil)
	r.eng.RegisterFont("AppUI", data)
	r.eng.SetDefaultFont("AppUI")
	q := engine.TextQuality{Hinting: true, HintingX: true, LCD: true}
	r.eng.SetTextQuality(q)

	dlg := widget.NewDialog("Сравнение файлов", 320, 200)
	r.eng.ShowModal(dlg)
	if r.fake(1) == nil {
		t.Fatal("окно ОС для диалога не создано")
	}
	r.win.modalHost.mu.Lock()
	var de *engine.Engine
	if len(r.win.modalHost.stack) > 0 {
		de = r.win.modalHost.stack[0].eng
	}
	r.win.modalHost.mu.Unlock()
	if de == nil {
		t.Fatal("вторичного движка диалога нет")
	}
	if !slices.Contains(de.AvailableFonts(), "AppUI") {
		t.Errorf("шрифт приложения не дошёл до диалога: %v", de.AvailableFonts())
	}
	if got := de.TextQuality(); got != q {
		t.Errorf("слой качества диалога %+v, ждал %+v", got, q)
	}
	r.eng.CloseModal(dlg)
}
