// Мост между плоской темой виджетов и профилями токенов проверяется одним
// требованием: перевод туда и обратно ничего не теряет.
//
// Это и есть обещание совместимости из плана работ — старая widget.Theme
// становится производной от токенов, а код, который знает только её,
// не должен заметить подмены. Если хоть одно из семи десятков полей не
// покрыто таблицей привязок, round-trip его обнулит и тест назовёт поле.
package tests

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// roundTrip прогоняет плоскую тему через профиль токенов и обратно.
func roundTrip(t *testing.T, src *widget.Theme) *widget.Theme {
	t.Helper()
	p := widget.ProfileFromTheme(src)

	m := theme.NewManager()
	if err := m.RegisterTheme(p); err != nil {
		t.Fatalf("регистрация профиля %q: %v", p.Name, err)
	}
	if err := m.SetTheme(p.Name); err != nil {
		t.Fatalf("установка темы %q: %v", p.Name, err)
	}
	return widget.Materialize(m.Active())
}

// TestBridge_RoundTripKeepsEveryPreset — все шесть готовых пресетов
// переживают перевод в профиль и обратно без единого изменённого поля.
func TestBridge_RoundTripKeepsEveryPreset(t *testing.T) {
	for _, name := range widget.ThemeNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			src := widget.ThemeByName(name)
			if src == nil {
				t.Fatalf("пресет %q не найден", name)
			}
			got := roundTrip(t, src)

			// Сравниваем поле за полем: reflect.DeepEqual сказал бы только
			// «не равны», а нам нужно имя потерянного поля — иначе поиск
			// пропущенной привязки среди семи десятков превращается в
			// перебор.
			diffFields(t, src, got)
		})
	}
}

// diffFields сравнивает две плоские темы поле за полем и сообщает о каждом
// расхождении с именем поля.
func diffFields(t *testing.T, want, got *widget.Theme) {
	t.Helper()
	wv, gv := reflect.ValueOf(*want), reflect.ValueOf(*got)
	typ := wv.Type()

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		w, g := wv.Field(i).Interface(), gv.Field(i).Interface()
		if reflect.DeepEqual(w, g) {
			continue
		}
		if f.Name == "Style" {
			diffStyle(t, want.Style, got.Style)
			continue
		}
		t.Errorf("поле %s: было %v, стало %v — привязка потеряна", f.Name, w, g)
	}
}

func diffStyle(t *testing.T, want, got widget.ThemeStyle) {
	t.Helper()
	wv, gv := reflect.ValueOf(want), reflect.ValueOf(got)
	typ := wv.Type()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		w, g := wv.Field(i).Interface(), gv.Field(i).Interface()
		if !reflect.DeepEqual(w, g) {
			t.Errorf("Style.%s: было %v, стало %v", f.Name, w, g)
		}
	}
}

// TestBridge_EveryColorFieldIsBound — каждое цветовое поле темы имеет
// привязку. Проверка идёт «от противного»: тема, у которой все поля
// заполнены разными цветами, после round-trip обязана остаться собой.
// Непокрытое поле обнулится и будет названо.
func TestBridge_EveryColorFieldIsBound(t *testing.T) {
	src := &widget.Theme{Style: widget.ThemeStyle{Name: "Проба"}}
	v := reflect.ValueOf(src).Elem()
	typ := v.Type()

	// Каждому полю — свой неповторимый цвет: так совпадение по случайности
	// исключено, а перепутанные местами привязки видны сразу.
	n := 0
	rgbaType := reflect.TypeOf(color.RGBA{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type != rgbaType {
			continue
		}
		n++
		c := color.RGBA{R: uint8(n * 3), G: uint8(n*5 + 1), B: uint8(n*7 + 2), A: 255}
		v.Field(i).Set(reflect.ValueOf(c))
	}
	if n < 70 {
		t.Fatalf("в теме нашлось всего %d цветовых полей — тест смотрит не туда", n)
	}

	got := roundTrip(t, src)
	diffFields(t, src, got)
}

// TestBridge_StyleShapeSurvives — форма темы (радиусы, флаги вида, цвета
// фаски) переживает перевод: она живёт в метриках и флагах, а не в цветах.
func TestBridge_StyleShapeSurvives(t *testing.T) {
	src := widget.ThemeByName("Win2000")
	if src == nil {
		t.Fatal("пресет Win2000 не найден")
	}
	if !src.Style.Classic3D {
		t.Fatal("у Win2000 сброшен Classic3D — проверять нечего")
	}
	got := roundTrip(t, src)

	if !got.Style.Classic3D {
		t.Error("флаг Classic3D потерян")
	}
	if got.Style.BevelLight != src.Style.BevelLight ||
		got.Style.BevelShadow != src.Style.BevelShadow ||
		got.Style.BevelDark != src.Style.BevelDark {
		t.Error("цвета фаски потеряны")
	}
	if got.Style.Name != src.Style.Name {
		t.Errorf("имя вида темы: было %q, стало %q", src.Style.Name, got.Style.Name)
	}

	mac := widget.ThemeByName("Mac")
	gotMac := roundTrip(t, mac)
	if !gotMac.Style.MacTitleBar {
		t.Error("флаг MacTitleBar потерян")
	}
	if gotMac.Style.WindowCorner != mac.Style.WindowCorner {
		t.Errorf("радиус окна: было %d, стало %d", mac.Style.WindowCorner, gotMac.Style.WindowCorner)
	}
}

// TestBridge_MaterializeSurvivesEmptyTheme — материализация темы, которая
// ничего не объявляет, не паникует и не портит структуру: отсутствие
// профиля не должно ронять приложение.
func TestBridge_MaterializeSurvivesEmptyTheme(t *testing.T) {
	m := theme.NewManager()
	empty := theme.NewProfile("Пустая")
	if err := m.RegisterTheme(empty); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Пустая"); err != nil {
		t.Fatal(err)
	}
	got := widget.Materialize(m.Active())
	if got == nil {
		t.Fatal("Materialize вернул nil")
	}
	if got.Style.Name != "Пустая" {
		t.Errorf("имя вида = %q", got.Style.Name)
	}
	if widget.Materialize(nil) == nil {
		t.Error("Materialize(nil) вернул nil вместо пустой темы")
	}
}

// TestBridge_ProfileIsReadableByHand — профиль, собранный из пресета,
// адресуется теми же тройками, что и написанный вручную: значит, автор
// темы может переопределить кнопку, не зная про плоскую структуру.
func TestBridge_ProfileIsReadableByHand(t *testing.T) {
	base := widget.ProfileFromTheme(widget.ThemeByName("Win11 Dark"))

	custom := theme.NewProfile("Своя")
	custom.Parent = base.Name
	custom.SetStyle("button", "", theme.StateHover, theme.StyleDelta{
		Fill: theme.C(theme.RGB(200, 0, 0)),
	})

	m := theme.NewManager()
	if err := m.RegisterTheme(base); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterTheme(custom); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Своя"); err != nil {
		t.Fatal(err)
	}

	got := widget.Materialize(m.Active())
	if got.BtnHoverBG != theme.RGB(200, 0, 0) {
		t.Errorf("переопределение кнопки не доехало до плоской темы: %v", got.BtnHoverBG)
	}
	// Остальное унаследовано от пресета.
	src := widget.ThemeByName("Win11 Dark")
	if got.BtnBG != src.BtnBG {
		t.Errorf("фон кнопки в покое: было %v, стало %v", src.BtnBG, got.BtnBG)
	}
	if got.WindowBG != src.WindowBG {
		t.Errorf("фон окна: было %v, стало %v", src.WindowBG, got.WindowBG)
	}
}

// TestBridge_EngineSwitchesByProfile — движок переключает тему по имени
// профиля: тот же путь применения, что и у SetTheme, но с наследованием.
func TestBridge_EngineSwitchesByProfile(t *testing.T) {
	m := widget.DefaultThemeManager()
	if len(m.ThemeNames()) != len(widget.ThemeNames()) {
		t.Fatalf("в менеджере %d профилей, пресетов %d",
			len(m.ThemeNames()), len(widget.ThemeNames()))
	}

	eng := engine.New(200, 120, 30)
	root := widget.NewPanel(widget.DarkTheme().WindowBG)
	root.SetBounds(image.Rect(0, 0, 200, 120))
	btn := widget.NewButton("ok")
	btn.SetBounds(image.Rect(10, 10, 90, 40))
	root.AddChild(btn)
	eng.SetRoot(root)

	if err := eng.SetThemeProfile(m, "Win2000"); err != nil {
		t.Fatalf("переключение на Win2000: %v", err)
	}
	if !widget.CurrentThemeStyle().Classic3D {
		t.Error("вид темы не переключился на классический")
	}
	want := widget.ThemeByName("Win2000")
	if btn.Background != want.BtnBG {
		t.Errorf("кнопка не перекрасилась: %v, ждали %v", btn.Background, want.BtnBG)
	}

	if err := eng.SetThemeProfile(m, "Win11 Dark"); err != nil {
		t.Fatalf("переключение на Win11 Dark: %v", err)
	}
	if widget.CurrentThemeStyle().Classic3D {
		t.Error("классический вид не снялся")
	}
	if err := eng.SetThemeProfile(m, "нет-такой"); err == nil {
		t.Error("неизвестный профиль принят без ошибки")
	}
	if err := eng.SetThemeProfile(nil, "Win2000"); err == nil {
		t.Error("nil-менеджер принят без ошибки")
	}

	// Возвращаем процессную палитру к исходной — она общая для всех тестов.
	widget.ApplyGlobalTheme(widget.ThemeByName("Win10 Dark"))
}
