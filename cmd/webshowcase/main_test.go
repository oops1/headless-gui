package main

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// xamlPath — разметка витрины относительно каталога пакета.
const xamlPath = "../../assets/ui/showcase.xaml"

// buildForTest поднимает витрину так же, как это делает main, но без сети.
func buildForTest(t *testing.T) *webShowcase {
	t.Helper()
	app, err := buildShowcase(xamlPath, "RU")
	if err != nil {
		t.Fatalf("сборка витрины: %v", err)
	}
	t.Cleanup(func() { widget.SetLanguage("RU") })
	return app
}

// clickWidget шлёт движку настоящий клик в центр виджета — тот же путь, что и
// у браузерного зрителя (webstream.dispatchInput зовёт эти же методы).
func clickWidget(app *webShowcase, w widget.Widget) {
	b := w.Bounds()
	x, y := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
	app.eng.SendMouseMove(x, y)
	app.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	app.eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

// TestShowcaseLoads — витрина собирается из той же разметки и находит ключевые
// виджеты: значит браузерный режим показывает то же, что и оконный.
func TestShowcaseLoads(t *testing.T) {
	app := buildForTest(t)
	for _, id := range []string{
		"mainTabs", "mainMenu", "eventLog", "langToggle", "themeSelect",
		"txtLogin", "dlgInfo", "pbCPU", "lblCPU",
	} {
		if app.reg[id] == nil {
			t.Errorf("в разметке не найден виджет %q", id)
		}
	}
	if app.log == nil {
		t.Fatal("журнал событий не подключён")
	}
	if app.log.Items() == nil || len(app.log.Items()) == 0 {
		t.Error("в журнале нет стартовой записи")
	}
}

// TestLanguageSwitchByMouse — клик по кнопке языка (обычным вводом, как из
// браузера) переключает язык всего интерфейса, включая заголовки вкладок и
// пункты меню.
func TestLanguageSwitchByMouse(t *testing.T) {
	app := buildForTest(t)

	tabs, _ := app.reg["mainTabs"].(*widget.TabControl)
	menu, _ := app.reg["mainMenu"].(*widget.MenuBar)
	lang, _ := app.reg["langToggle"].(*widget.Button)
	if tabs == nil || menu == nil || lang == nil {
		t.Fatal("нет вкладок, меню или кнопки языка")
	}

	if got := tabs.TabHeader(0); got != "Ввод данных" {
		t.Fatalf("до переключения вкладка 0 = %q", got)
	}
	if got := menu.Items()[0].Text; got != "Файл" {
		t.Fatalf("до переключения меню 0 = %q", got)
	}
	if !strings.HasPrefix(lang.Text, "Язык") {
		t.Fatalf("до переключения кнопка языка = %q", lang.Text)
	}

	clickWidget(app, lang)

	if widget.Language() != "EN" {
		t.Fatalf("язык интерфейса = %q, ожидался EN", widget.Language())
	}
	if got := tabs.TabHeader(0); got != "Input" {
		t.Errorf("после переключения вкладка 0 = %q, want Input", got)
	}
	if got := menu.Items()[0].Text; got != "File" {
		t.Errorf("после переключения меню 0 = %q, want File", got)
	}
	if lang.Text != "Language: EN" {
		t.Errorf("кнопка языка = %q", lang.Text)
	}
}

// TestButtonsLogEvents — клики по кнопкам формы доезжают до обработчиков:
// в журнале появляются записи (в браузере именно это и видно).
func TestButtonsLogEvents(t *testing.T) {
	app := buildForTest(t)
	before := len(app.log.Items())

	for _, id := range []string{"btnAccent", "btnDefault", "btnDanger", "btnExport"} {
		b, _ := app.reg[id].(*widget.Button)
		if b == nil {
			t.Fatalf("кнопка %q не найдена", id)
		}
		clickWidget(app, b)
	}

	items := app.log.Items()
	if len(items) != before+4 {
		t.Fatalf("записей в журнале %d, ожидалось %d", len(items), before+4)
	}
	// Журнал ведётся на языке интерфейса (сейчас русский).
	joined := strings.Join(items, "\n")
	for _, want := range []string{"Вход: user=", "Отмена нажата", "Удалить нажата", "Экспорт настроек"} {
		if !strings.Contains(joined, want) {
			t.Errorf("в журнале нет записи %q:\n%s", want, joined)
		}
	}
}

// TestClearLog — кнопка очистки журнала работает и пишет о себе.
func TestClearLog(t *testing.T) {
	app := buildForTest(t)
	b, _ := app.reg["btnClearLog"].(*widget.Button)
	if b == nil {
		t.Skip("в разметке нет кнопки очистки журнала")
	}
	// Кнопка живёт на другой вкладке: мышью до неё не достучаться, пока та
	// не активна, поэтому дёргаем обработчик — проверяем именно обвязку.
	b.OnClick()
	items := app.log.Items()
	if len(items) != 1 || !strings.Contains(items[0], "Журнал очищен") {
		t.Errorf("после очистки журнал = %v", items)
	}
}

// TestWindowOnlyFeaturesExplain — кнопки трея в браузере не молчат, а
// объясняют, почему они недоступны без окна ОС.
func TestWindowOnlyFeaturesExplain(t *testing.T) {
	app := buildForTest(t)
	b, _ := app.reg["sysBalloonInfo"].(*widget.Button)
	if b == nil {
		t.Skip("в разметке нет кнопки balloon")
	}
	before := len(app.log.Items())
	b.OnClick() // вкладка «Система» неактивна — проверяем обвязку напрямую
	items := app.log.Items()
	if len(items) <= before {
		t.Fatal("клик по кнопке трея не дошёл до обработчика")
	}
	if !strings.Contains(items[len(items)-1], "браузер") && !strings.Contains(items[len(items)-1], "окн") {
		t.Errorf("последняя запись журнала не про окно: %q", items[len(items)-1])
	}
}
