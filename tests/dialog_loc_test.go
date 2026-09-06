package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Локализация штатных диалогов из таблицы приложения — запрос GG-15.
//
// Ключи «dlg.*» переопределялись обычным RegisterStrings всегда. Не хватало
// другого: у приложения СВОИ имена ключей, и чтобы диалоги говорили из той же
// таблицы, значения приходилось дублировать под вторым именем и следить, чтобы
// переводы не разъехались.

func TestDialogLoc_OverrideByTheSameKey(t *testing.T) {
	widget.ClearStringAliases()
	t.Cleanup(widget.ClearStringAliases)

	widget.SetLanguage("RU")
	if got := widget.Tr("dlg.cancel"); got != "Отмена" {
		t.Fatalf("встроенный перевод dlg.cancel = %q", got)
	}

	// Прямое переопределение — то, что работало и раньше.
	widget.RegisterStrings("RU", map[string]string{"dlg.cancel": "Не надо"})
	t.Cleanup(func() { widget.RegisterStrings("RU", map[string]string{"dlg.cancel": "Отмена"}) })

	if got := widget.Tr("dlg.cancel"); got != "Не надо" {
		t.Errorf("после переопределения dlg.cancel = %q", got)
	}
}

func TestDialogLoc_AliasToApplicationKey(t *testing.T) {
	widget.ClearStringAliases()
	t.Cleanup(widget.ClearStringAliases)

	widget.RegisterStrings("RU", map[string]string{"btn.ok": "Готово"})
	widget.RegisterStrings("EN", map[string]string{"btn.ok": "Done"})
	widget.SetLanguage("RU")

	widget.AliasString("dlg.ok", "btn.ok")
	if got := widget.Tr("dlg.ok"); got != "Готово" {
		t.Errorf("по псевдониму dlg.ok = %q, ждали «Готово»", got)
	}

	// Псевдоним живёт вместе с языком: одна таблица на всё окно.
	widget.SetLanguage("EN")
	if got := widget.Tr("dlg.ok"); got != "Done" {
		t.Errorf("после смены языка dlg.ok = %q, ждали Done", got)
	}
	widget.SetLanguage("RU")

	// Снятие псевдонима возвращает встроенный перевод.
	widget.AliasString("dlg.ok", "")
	if got := widget.Tr("dlg.ok"); got != "OK" {
		t.Errorf("после снятия псевдонима dlg.ok = %q, ждали встроенный OK", got)
	}
}

// Псевдоним на ключ, которого в таблице приложения нет, не ломает диалог:
// берётся встроенный перевод.
func TestDialogLoc_AliasToMissingKeyFallsBack(t *testing.T) {
	widget.ClearStringAliases()
	t.Cleanup(widget.ClearStringAliases)
	widget.SetLanguage("RU")

	widget.AliasString("dlg.close", "нет.такого.ключа")
	if got := widget.Tr("dlg.close"); got != "Закрыть" {
		t.Errorf("псевдоним на пустоту дал %q, ждали встроенное «Закрыть»", got)
	}
}

// Псевдоним старше собственного перевода ключа: оба заданы приложением, и
// побеждает тот, что назван конкретнее.
func TestDialogLoc_AliasBeatsDirectOverride(t *testing.T) {
	widget.ClearStringAliases()
	t.Cleanup(widget.ClearStringAliases)

	widget.RegisterStrings("RU", map[string]string{
		"dlg.retry": "Прямое",
		"app.retry": "Через псевдоним",
	})
	t.Cleanup(func() { widget.RegisterStrings("RU", map[string]string{"dlg.retry": "Повторить"}) })
	widget.SetLanguage("RU")

	widget.AliasString("dlg.retry", "app.retry")
	if got := widget.Tr("dlg.retry"); got != "Через псевдоним" {
		t.Errorf("dlg.retry = %q, ждали «Через псевдоним»", got)
	}
}

// Смена псевдонима будит подписчиков языка: открытый диалог обязан перечитать
// надписи, иначе кнопка останется со старым текстом до смены языка.
func TestDialogLoc_AliasNotifiesListeners(t *testing.T) {
	widget.ClearStringAliases()
	t.Cleanup(widget.ClearStringAliases)

	fired := 0
	id := widget.AddLanguageListener(func(string) { fired++ })
	t.Cleanup(func() { widget.RemoveLanguageListener(id) })

	widget.AliasString("dlg.no", "app.no")
	if fired == 0 {
		t.Error("смена псевдонима не разбудила подписчиков языка")
	}
}
