//go:build linux && !android

package window

import (
	"image"
	"testing"
)

// На Wayland клиент не разговаривает с редактором метода ввода напрямую:
// между ними стоит компоновщик. Он шлёт набираемое и готовый текст, а клиент
// взамен рассказывает, где каретка, — чтобы окно кандидатов встало под
// набираемым словом, а не в углу экрана.

// wlStr — строка в формате протокола: длина с нулём, байты, нуль, добивка до
// четырёх.
func wlStr(s string) []byte {
	b := wlU32(uint32(len(s) + 1))
	b = append(b, s...)
	b = append(b, 0)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func newTextInputWindow(t *testing.T) (*wlTestConn, *[]string) {
	t.Helper()
	c := newWlTestWindow(t)
	c.w.textInput.id = 33

	var log []string
	c.w.SetOnIMEComposition(func(text string, caret int) {
		log = append(log, "preedit:"+text)
	})
	c.w.SetOnIMECommit(func(text string) { log = append(log, "commit:"+text) })
	c.w.SetIMECaretProvider(func() (image.Rectangle, bool) {
		return image.Rect(40, 60, 41, 78), true
	})
	return c, &log
}

// События копятся и применяются одной порцией: preedit и commit в одной
// порции означают «введи это и покажи вот такое набираемое», а порознь они
// встали бы в поле в неверном порядке.
func TestWaylandIME_AppliesOnDone(t *testing.T) {
	c, log := newTextInputWindow(t)

	body := append(wlStr("にほ"), wlU32(3, 3)...) // текст, cursor_begin, cursor_end
	c.w.handleEvent(33, wlTextInputEvPreeditString, body)
	if len(*log) != 0 {
		t.Fatalf("набираемое применено до done: %v", *log)
	}

	c.w.handleEvent(33, wlTextInputEvDone, wlU32(1))
	if len(*log) != 1 || (*log)[0] != "preedit:にほ" {
		t.Errorf("после done: %v", *log)
	}
}

// Готовый текст идёт первым, набираемое — следом: порядок задан протоколом.
func TestWaylandIME_CommitBeforePreedit(t *testing.T) {
	c, log := newTextInputWindow(t)

	c.w.handleEvent(33, wlTextInputEvPreeditString, append(wlStr("ご"), wlU32(0, 0)...))
	c.w.handleEvent(33, wlTextInputEvCommitString, wlStr("日本"))
	c.w.handleEvent(33, wlTextInputEvDone, wlU32(2))

	if len(*log) != 2 || (*log)[0] != "commit:日本" || (*log)[1] != "preedit:ご" {
		t.Errorf("порядок применения: %v", *log)
	}
}

// Порция без набираемого означает, что его больше нет: иначе прежнее
// осталось бы висеть в поле.
func TestWaylandIME_CommitClearsPreedit(t *testing.T) {
	c, log := newTextInputWindow(t)

	c.w.handleEvent(33, wlTextInputEvCommitString, wlStr("日"))
	c.w.handleEvent(33, wlTextInputEvDone, wlU32(3))

	if len(*log) != 2 || (*log)[1] != "preedit:" {
		t.Errorf("набираемое не убрано: %v", *log)
	}
}

// Компоновщик сообщил, что ввод идёт в нашу поверхность: включаем ввод и
// сразу говорим, где каретка.
func TestWaylandIME_EnterEnables(t *testing.T) {
	c, _ := newTextInputWindow(t)

	c.w.handleEvent(33, wlTextInputEvEnter, wlU32(c.w.surfaceID))
	msgs := splitWlMsgs(t, c.drain(t))

	if _, ok := findWlMsg(msgs, 33, wlTextInputEnable); !ok {
		t.Fatalf("ввод не включён: %+v", msgs)
	}
	rect, ok := findWlMsg(msgs, 33, wlTextInputSetCursorRectangle)
	if !ok || len(rect.args) != 4 {
		t.Fatalf("место каретки не отправлено: %+v", msgs)
	}
	if int32(rect.args[0]) != 40 || int32(rect.args[1]) != 60 {
		t.Errorf("каретка %v, ждал 40,60", rect.args[:2])
	}
	// Протокол требует commit после набора запросов — иначе компоновщик их
	// не применит.
	if _, ok := findWlMsg(msgs, 33, wlTextInputCommit); !ok {
		t.Error("порция не завершена commit")
	}
}

// Курсор внутри набираемого приходит в БАЙТАХ, а полю нужны руны: на
// иероглифах это втрое разные числа.
func TestWlRunesBefore(t *testing.T) {
	const s = "にほん" // по три байта на знак
	if got := wlRunesBefore(s, 6); got != 2 {
		t.Errorf("рун до шестого байта %d, ждал 2", got)
	}
	if got := wlRunesBefore(s, 0); got != 0 {
		t.Errorf("в начале %d", got)
	}
	if got := wlRunesBefore(s, 100); got != 3 {
		t.Errorf("за концом %d, ждал 3", got)
	}
}
