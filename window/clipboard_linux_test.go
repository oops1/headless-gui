//go:build linux && !android

package window

import (
	"encoding/binary"
	"testing"
	"time"
)

// Буфер обмена на Linux работал только через xclip/xsel: подпроцесс, которого
// в системе может не быть вовсе (в пакете WinLine его и нет), да ещё и с
// обрезкой завершающего перевода строки. Под Wayland не работал никак.

// ─── Wayland ────────────────────────────────────────────────────────────────

func TestWlClipboard_SetTextOffersTextTypes(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.inputSerial.Store(77)

	c.w.clip.SetText("abc\n")
	msgs := splitWlMsgs(t, c.read(t))

	if _, ok := findWlMsg(msgs, 30, wlDataDevMgrCreateDataSource); !ok {
		t.Fatalf("источник буфера не создан, пришло: %+v", msgs)
	}
	// Типы объявляются все: разные тулкиты спрашивают разное.
	offers := 0
	for _, m := range msgs {
		if m.opcode == wlDataSourceOffer && m.obj != 30 && m.obj != 31 {
			offers++
		}
	}
	if offers < 4 {
		t.Errorf("объявлено типов: %d, ждал минимум четыре", offers)
	}
	sel, ok := findWlMsg(msgs, 31, wlDataDeviceSetSelection)
	if !ok {
		t.Fatal("set_selection не отправлен")
	}
	if len(sel.args) != 2 || sel.args[1] != 77 {
		t.Errorf("set_selection %v, ждал serial 77", sel.args)
	}
}

// Без serial'а ввода компоновщик откажет: set_selection принимается только
// как следствие действия пользователя.
func TestWlClipboard_NoSerialNoSelection(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31

	c.w.clip.SetText("abc")
	c.peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 256)
	if n, err := c.peer.Read(buf); err == nil && n > 0 {
		t.Errorf("без serial'а отправлено %d байт", n)
	}
}

// Пока владеем буфером сами, вставка не ходит через компоновщика: свой же
// текст лежит рядом.
func TestWlClipboard_GetTextReturnsOwned(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.inputSerial.Store(5)

	c.w.clip.SetText("abc\n")
	if got := c.w.clip.GetText(); got != "abc\n" {
		t.Errorf("GetText = %q, ждал «abc\\n» (с переводом строки)", got)
	}
}

// Чужой selection забирает владение: дальше текст надо спрашивать у нового
// владельца, а не отдавать свой устаревший.
func TestWlClipboard_ForeignSelectionDropsOwnership(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.inputSerial.Store(5)
	c.w.clip.SetText("наш текст")

	// Компоновщик прислал чужой offer, в котором текста нет.
	body := make([]byte, 4)
	binary.LittleEndian.PutUint32(body[0:4], 44)
	c.w.clip.handleSelection(44)

	if got := c.w.clip.GetText(); got != "" {
		t.Errorf("GetText = %q, ждал пусто: владелец сменился, а типов у offer нет", got)
	}
}

// Пустой selection (id == 0) означает «буфер пуст».
func TestWlClipboard_EmptySelection(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.clip.handleSelection(0)
	if got := c.w.clip.GetText(); got != "" {
		t.Errorf("GetText = %q при пустом буфере", got)
	}
}

// Запрос данных у владельца не ждёт вечно: чужой процесс вправе не ответить.
func TestWlClipboard_ReceiveTimesOut(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.clip.handleSelection(50)
	c.w.offerSetText(50)

	start := time.Now()
	got := c.w.clip.GetText() // никто не пишет в канал — должен выйти по тайм-ауту
	elapsed := time.Since(start)

	if got != "" {
		t.Errorf("GetText = %q, ждал пусто", got)
	}
	if elapsed > 2*time.Second {
		t.Errorf("ожидание заняло %v — интерфейс так и будет стоять", elapsed)
	}
}

// Типы offer разбираются: текстовый буфер и список файлов — разные вещи, и
// путать их нельзя, иначе в редактор вставится список путей.
func TestWlClipboard_TextAndFileOffersAreDistinct(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSet(60, true) // text/uri-list — перетаскивание файлов
	c.w.offerSetText(61)   // text/plain — буфер обмена

	if c.w.offerHasText(60) {
		t.Error("список файлов принят за текст")
	}
	if !c.w.offerHasText(61) {
		t.Error("текстовое предложение не распознано")
	}
	if !c.w.offerGet(60) {
		t.Error("предложение файлов потерялось")
	}
}

func TestIsTextMime(t *testing.T) {
	for _, m := range []string{mimeTextUTF8, mimeTextPlain, mimeUTF8Str, mimeTextStr} {
		if !isTextMime(m) {
			t.Errorf("тип %q не признан текстом", m)
		}
	}
	for _, m := range []string{mimeTextUriList, "image/png", ""} {
		if isTextMime(m) {
			t.Errorf("тип %q признан текстом", m)
		}
	}
}

// ─── X11 ────────────────────────────────────────────────────────────────────

// Владея буфером, отвечаем из своей копии — без похода в сервер.
func TestX11Clipboard_OwnedText(t *testing.T) {
	w := &X11Window{wid: 1, atomClipboard: 100}
	c := newX11Clipboard(w)
	c.owned, c.owns = "abc\n", true

	if got := c.GetText(); got != "abc\n" {
		t.Errorf("GetText = %q, ждал «abc\\n»", got)
	}
}

// SelectionClear забирает владение: дальше текст у нового владельца.
func TestX11Clipboard_SelectionClear(t *testing.T) {
	w := &X11Window{wid: 1, atomClipboard: 100}
	c := newX11Clipboard(w)
	c.owned, c.owns = "наш", true

	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[12:16], 100) // selection = CLIPBOARD
	c.handleClear(buf)

	if c.owns || c.owned != "" {
		t.Error("владение не снято")
	}
}

// Чужое выделение (например, PRIMARY) наш буфер не трогает.
func TestX11Clipboard_ClearOfOtherSelectionIgnored(t *testing.T) {
	w := &X11Window{wid: 1, atomClipboard: 100}
	c := newX11Clipboard(w)
	c.owned, c.owns = "наш", true

	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[12:16], 101) // другое выделение
	c.handleClear(buf)

	if !c.owns {
		t.Error("чужое SelectionClear сняло наше владение")
	}
}

// SelectionNotify чужого выделения не наш: его разберёт XDND.
func TestX11Clipboard_NotifyOfOtherSelectionNotOurs(t *testing.T) {
	w := &X11Window{wid: 1, atomClipboard: 100}
	c := newX11Clipboard(w)

	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[12:16], 102) // XdndSelection, например
	if c.handleNotify(buf) {
		t.Error("буфер обмена забрал чужое SelectionNotify")
	}
}
