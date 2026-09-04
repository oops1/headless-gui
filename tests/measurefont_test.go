package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Замер текста ИМЕНОВАННЫМ шрифтом вне отрисовки.
//
// MeasureUIText меряет шрифтом по умолчанию, а DrawTextFont рисует указанным
// семейством. Пока нужный шрифт не зарегистрирован, он подменяется дефолтным
// и разницы не видно; как только появится — раскладка, посчитанная не тем
// шрифтом, разъедется: ширина колонок и горизонтальная прокрутка текста diff
// считаются ДО кадра, когда контекста отрисовки ещё нет.
//
// Проверяется на встроенном жирном шрифте: его движок регистрирует сам, и
// от шрифта по умолчанию он отличается ровно так, как отличался бы
// моноширинный.

func measuringEngine(t *testing.T) *engine.Engine {
	t.Helper()
	eng := engine.New(400, 300, 60)
	t.Cleanup(eng.Stop)
	return eng
}

func TestMeasureUITextFont_DiffersFromDefault(t *testing.T) {
	measuringEngine(t)

	const text = "Репозиторий"
	def := widget.MeasureUIText(text, widget.DefaultFontSizePt)
	bold := widget.MeasureUITextFont(text, widget.DefaultFontSizePt, widget.BuiltinFontBold)

	if def <= 0 || bold <= 0 {
		t.Fatalf("замер дал %d и %d — измеритель не зарегистрирован", def, bold)
	}
	if def == bold {
		t.Errorf("жирный и обычный шрифт дали одну ширину %d — раскладка "+
			"считается не тем шрифтом, которым рисуют", bold)
	}
}

// Пустое имя семейства — шрифт по умолчанию, то есть ровно MeasureUIText.
func TestMeasureUITextFont_EmptyFamilyIsDefault(t *testing.T) {
	measuringEngine(t)

	const text = "Репозиторий"
	if got, want := widget.MeasureUITextFont(text, widget.DefaultFontSizePt, ""),
		widget.MeasureUIText(text, widget.DefaultFontSizePt); got != want {
		t.Errorf("с пустым семейством %d, шрифтом по умолчанию %d", got, want)
	}
}

// Незнакомое семейство не роняет раскладку: движок подменит его шрифтом по
// умолчанию — так же, как делает при отрисовке.
func TestMeasureUITextFont_UnknownFamilyFallsBack(t *testing.T) {
	measuringEngine(t)

	if got := widget.MeasureUITextFont("текст", widget.DefaultFontSizePt, "нет-такого"); got <= 0 {
		t.Errorf("незнакомое семейство дало ширину %d", got)
	}
}

// Повторный замер отвечает то же самое: у именованного шрифта свой кэш, и
// ошибка в нём выдала бы чужую ширину.
func TestMeasureUITextFont_CacheIsPerFamily(t *testing.T) {
	measuringEngine(t)

	const text = "Ветка"
	bold := widget.MeasureUITextFont(text, widget.DefaultFontSizePt, widget.BuiltinFontBold)
	def := widget.MeasureUITextFont(text, widget.DefaultFontSizePt, "")
	boldAgain := widget.MeasureUITextFont(text, widget.DefaultFontSizePt, widget.BuiltinFontBold)

	if bold != boldAgain {
		t.Errorf("повторный замер жирным дал %d вместо %d", boldAgain, bold)
	}
	if def == bold {
		t.Errorf("замер по умолчанию совпал с жирным (%d) — кэш перепутал семейства", def)
	}
}

// Позиции символов именованным шрифтом — для каретки и выделения.
func TestMeasureUIRunePositions_FollowsTheFamily(t *testing.T) {
	measuringEngine(t)

	const text = "Ветка"
	runes := len([]rune(text))

	pos := widget.MeasureUIRunePositions(text, widget.DefaultFontSizePt, widget.BuiltinFontBold)
	if len(pos) != runes+1 {
		t.Fatalf("получено %d позиций, ждали %d", len(pos), runes+1)
	}
	if pos[0] != 0 {
		t.Errorf("первая позиция %d, ждали ноль", pos[0])
	}
	// Позиции монотонны: каретка не может ехать назад по ходу текста.
	for i := 1; i < len(pos); i++ {
		if pos[i] < pos[i-1] {
			t.Errorf("позиция %d (%d) меньше предыдущей (%d)", i, pos[i], pos[i-1])
		}
	}
	// Последняя позиция — это ширина всей строки тем же шрифтом.
	if want := widget.MeasureUITextFont(text, widget.DefaultFontSizePt, widget.BuiltinFontBold); pos[runes] != want {
		t.Errorf("последняя позиция %d, ширина строки %d — каретка встанет не там",
			pos[runes], want)
	}

	def := widget.MeasureUIRunePositions(text, widget.DefaultFontSizePt, "")
	if len(def) == len(pos) && def[runes] == pos[runes] {
		t.Error("позиции совпали с шрифтом по умолчанию — семейство не учтено")
	}
}
