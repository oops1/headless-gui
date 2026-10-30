//go:build linux && !android

package wayland

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/clipimage"
	"github.com/oops1/headless-gui/v4/internal/dib"
	"github.com/oops1/headless-gui/v4/widget/base"
)

// Изображение в буфере обмена Wayland. Компоновщик подменён парой сокетов.

func newWlImageClipboard(t *testing.T) *wlTestConn {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	c.w.inputSerial.Store(9)
	return c
}

// requestMime играет получателя: просит у нашего источника данные типа mime и
// читает всё, что тот напишет в канал.
func requestMime(t *testing.T, c *wlTestConn, mime string) []byte {
	t.Helper()
	r, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Dup(int(wr.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	wr.Close()
	c.w.clip.handleSourceSend(c.w.clip.source, mime, fd)
	r.SetReadDeadline(time.Now().Add(3 * time.Second))
	got, _ := io.ReadAll(r)
	r.Close()
	return got
}

// SetImage объявляет единственный тип — image/png, без текстовых.
func TestWlClipboard_SetImageOffersOnlyPNG(t *testing.T) {
	c := newWlImageClipboard(t)
	if err := c.w.clip.SetImage(testClipPicture()); err != nil {
		t.Fatal(err)
	}
	raw := c.read(t)
	msgs := splitWlMsgs(t, raw)
	if _, ok := findWlMsg(msgs, 30, wlDataDevMgrCreateDataSource); !ok {
		t.Fatalf("источник не создан: %+v", msgs)
	}
	offers := 0
	for _, m := range msgs {
		if m.opcode == wlDataSourceOffer && m.obj != 30 && m.obj != 31 {
			offers++
		}
	}
	if offers != 1 || !bytes.Contains(raw, []byte("image/png\x00")) || bytes.Contains(raw, []byte("text/plain")) {
		t.Errorf("объявлено типов %d, ждал один image/png без текстовых", offers)
	}
	sel, ok := findWlMsg(msgs, 31, wlDataDeviceSetSelection)
	if !ok || len(sel.args) != 2 || sel.args[1] != 9 {
		t.Errorf("set_selection: %v %v", sel.args, ok)
	}
}

// Без serial'а ввода — ошибка, а не молчание; пустая картинка — своя ошибка.
func TestWlClipboard_SetImageErrors(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.dataDevMgrID = 30
	c.w.dataDeviceID = 31
	if err := c.w.clip.SetImage(testClipPicture()); !errors.Is(err, base.ErrClipboardImageUnsupported) {
		t.Errorf("без serial'а: %v", err)
	}
	c.w.inputSerial.Store(1)
	if err := c.w.clip.SetImage(image.NewNRGBA(image.Rect(0, 0, 0, 0))); !errors.Is(err, base.ErrClipboardImageEmpty) {
		t.Errorf("пустая: %v", err)
	}
	c2 := newWlTestWindow(t) // без wl_data_device
	c2.w.inputSerial.Store(1)
	if err := c2.w.clip.SetImage(testClipPicture()); !errors.Is(err, base.ErrClipboardImageUnsupported) {
		t.Errorf("без wl_data_device: %v", err)
	}
}

// PNG кодируется лениво — на запрос получателя, один раз; на текст картинка
// отвечает пустотой.
func TestWlClipboard_ImageServedLazilyAsPNG(t *testing.T) {
	c := newWlImageClipboard(t)
	src := testClipPicture()
	if err := c.w.clip.SetImage(src); err != nil {
		t.Fatal(err)
	}
	own := c.w.clip.ownedImg
	if own == nil || own.Encoded() {
		t.Fatal("PNG закодирован до первого запроса")
	}
	src.SetNRGBA(0, 0, color.NRGBA{1, 1, 1, 255}) // правка оригинала после копирования

	got := requestMime(t, c, "image/png")
	if !own.Encoded() {
		t.Error("после запроса PNG не запомнен")
	}
	img, err := clipimage.DecodePNG(got)
	if err != nil {
		t.Fatalf("на image/png отдано не PNG: %v", err)
	}
	want := testClipPicture() // снимок на момент копирования
	samePicture(t, img, want)

	again := requestMime(t, c, "image/png")
	if !bytes.Equal(got, again) {
		t.Error("второй получатель получил другой PNG")
	}
	if txt := requestMime(t, c, mimeTextUTF8); len(txt) != 0 {
		t.Errorf("на текст картинка отдала %q", txt)
	}
}

// Пока владеем — картинка из своей копии (своей: правка отданной не портит
// буфер); текста, HTML и файлов в буфере нет.
func TestWlClipboard_GetImageOwned(t *testing.T) {
	c := newWlImageClipboard(t)
	if err := c.w.clip.SetImage(testClipPicture()); err != nil {
		t.Fatal(err)
	}
	got, ok := c.w.clip.GetImage()
	if !ok {
		t.Fatal("GetImage: нет")
	}
	samePicture(t, got, testClipPicture())
	got.(*image.NRGBA).SetNRGBA(0, 0, color.NRGBA{9, 9, 9, 9})
	again, _ := c.w.clip.GetImage()
	samePicture(t, again, testClipPicture())

	if s := c.w.clip.GetText(); s != "" {
		t.Errorf("текст рядом с картинкой: %q", s)
	}
	if _, ok := c.w.clip.GetHTML(); ok {
		t.Error("HTML рядом с картинкой")
	}
	if _, _, ok := c.w.clip.GetFiles(); ok {
		t.Error("файлы рядом с картинкой")
	}
}

// Текст, файлы и чужой selection вытесняют картинку; и наоборот.
func TestWlClipboard_ImageReplacedByOtherContent(t *testing.T) {
	c := newWlImageClipboard(t)
	c.w.clip.SetImage(testClipPicture())
	c.w.clip.SetText("x")
	if _, ok := c.w.clip.GetImage(); ok {
		t.Error("после SetText осталась картинка")
	}
	c.w.clip.SetImage(testClipPicture())
	c.w.clip.SetFiles([]string{"/a"}, false)
	if _, ok := c.w.clip.GetImage(); ok {
		t.Error("после SetFiles осталась картинка")
	}
	c.w.clip.SetImage(testClipPicture())
	c.w.clip.handleSelection(44) // чужой offer без типов
	if _, ok := c.w.clip.GetImage(); ok {
		t.Error("после чужого selection осталась картинка")
	}
	if c.w.clip.ownedImg != nil {
		t.Error("владение картинкой не снято")
	}
	c.w.clip.SetText("t")
	c.w.clip.SetImage(testClipPicture())
	if s := c.w.clip.GetText(); s != "" {
		t.Errorf("текст после SetImage: %q", s)
	}
}

// Источник отменён компоновщиком — картинка больше не наша.
func TestWlClipboard_ImageCancelled(t *testing.T) {
	c := newWlImageClipboard(t)
	c.w.clip.SetImage(testClipPicture())
	c.w.clip.handleSourceCancelled(c.w.clip.source)
	if c.w.clip.ownedImg != nil {
		t.Error("после cancelled картинка осталась")
	}
}

// Типы offer: PNG предпочтительнее BMP независимо от порядка; чужой мусор не
// принимается за картинку; offer забывается.
func TestWlClipboard_OfferImageMime(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSetImage(60, "image/bmp")
	c.w.offerSetImage(60, "image/png")
	c.w.offerSetImage(61, "image/png")
	c.w.offerSetImage(61, "image/bmp")
	c.w.offerSetImage(62, "image/x-bmp")
	for _, id := range []uint32{60, 61} {
		if m := c.w.offerImageMime(id); m != "image/png" {
			t.Errorf("offer %d: %q, ждал image/png", id, m)
		}
	}
	if m := c.w.offerImageMime(62); m != "image/x-bmp" {
		t.Errorf("offer 62: %q", m)
	}
	if m := c.w.offerImageMime(63); m != "" {
		t.Errorf("offer 63 без картинки: %q", m)
	}
	c.w.offerDelete(60)
	if m := c.w.offerImageMime(60); m != "" {
		t.Errorf("после offerDelete: %q", m)
	}
}

// fakeImageOwner играет владельца: пишет parts в канал receive, выдерживая gap
// между кусками, и запоминает запрошенные типы.
func fakeImageOwner(t *testing.T, c *wlTestConn, parts [][]byte, gap time.Duration) (asked *[]string, wait func()) {
	var mu sync.Mutex
	var got []string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf, oob := make([]byte, 256), make([]byte, 64)
		c.peer.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, oobn, _, _, err := c.peer.ReadMsgUnix(buf, oob)
		if err != nil || n < 8 {
			return
		}
		mime, _ := wlString(buf[8:n], 0)
		mu.Lock()
		got = append(got, mime)
		mu.Unlock()
		msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil || len(msgs) == 0 {
			return
		}
		fds, err := syscall.ParseUnixRights(&msgs[0])
		if err != nil || len(fds) == 0 {
			return
		}
		f := os.NewFile(uintptr(fds[0]), "peer-write")
		defer f.Close()
		for i, p := range parts {
			if i > 0 {
				time.Sleep(gap)
			}
			f.Write(p)
		}
	}()
	return &got, func() { c.peer.SetReadDeadline(time.Now()); wg.Wait() }
}

func foreignImageOffer(c *wlTestConn, id uint32, mimes ...string) {
	for _, m := range mimes {
		c.w.offerSetImage(id, m)
	}
	c.w.clip.handleSelection(id)
}

// Чужой PNG читается; запрашивается именно image/png.
func TestWlClipboard_GetImageForeignPNG(t *testing.T) {
	c := newWlTestWindow(t)
	png, _ := clipimage.EncodePNG(testClipPicture())
	foreignImageOffer(c, 50, "image/bmp", "image/png")
	asked, wait := fakeImageOwner(t, c, [][]byte{png}, 0)
	img, ok := c.w.clip.GetImage()
	wait()
	if !ok {
		t.Fatal("GetImage: нет")
	}
	samePicture(t, img, testClipPicture())
	if len(*asked) != 1 || (*asked)[0] != "image/png" {
		t.Errorf("запрошено %v", *asked)
	}
}

// Только BMP — запрашивается он, разбирается DIB с альфой.
func TestWlClipboard_GetImageForeignBMPFallback(t *testing.T) {
	c := newWlTestWindow(t)
	foreignImageOffer(c, 51, "image/bmp")
	asked, wait := fakeImageOwner(t, c, [][]byte{dib.EncodeBMP(testClipPicture())}, 0)
	img, ok := c.w.clip.GetImage()
	wait()
	if !ok {
		t.Fatal("GetImage: нет")
	}
	samePicture(t, img, testClipPicture())
	if len(*asked) != 1 || (*asked)[0] != "image/bmp" {
		t.Errorf("запрошено %v", *asked)
	}
}

// В буфере текст — владельца не спрашиваем и не ждём.
func TestWlClipboard_GetImageForeignTextOnly(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.offerSetText(52)
	c.w.clip.handleSelection(52)
	start := time.Now()
	if _, ok := c.w.clip.GetImage(); ok {
		t.Error("текст принят за картинку")
	}
	if time.Since(start) > clipboardReadTimeout/2 {
		t.Error("GetImage спрашивал владельца, хотя картинка не предложена")
	}
}

// Молчащий владелец не подвешивает вызов: срок — 300 мс.
func TestWlClipboard_GetImageSilentOwnerTimesOut(t *testing.T) {
	c := newWlTestWindow(t)
	foreignImageOffer(c, 53, "image/png")
	start := time.Now()
	if _, ok := c.w.clip.GetImage(); ok {
		t.Error("молчание принято за картинку")
	}
	d := time.Since(start)
	if d < clipboardReadTimeout/2 || d > 2*time.Second {
		t.Errorf("ожидание заняло %v, ждал около %v", d, clipboardReadTimeout)
	}
}

// Срок отсчитывается на каждый кусок: владелец, отдающий картинку дольше 300 мс
// суммарно, но без пауз длиннее срока, успевает; оборвавшийся на полпути — нет.
func TestWlClipboard_GetImageIdleTimeoutPerChunk(t *testing.T) {
	png, _ := clipimage.EncodePNG(testClipPicture())
	half := len(png) / 2

	c := newWlTestWindow(t)
	foreignImageOffer(c, 54, "image/png")
	gap := clipboardReadTimeout * 2 / 3
	_, wait := fakeImageOwner(t, c, [][]byte{png[:half/2], png[half/2 : half], png[half:]}, gap)
	start := time.Now()
	img, ok := c.w.clip.GetImage()
	wait()
	if !ok {
		t.Fatal("медленный, но живой владелец не дождался")
	}
	if time.Since(start) < clipboardReadTimeout {
		t.Skip("тест не проверил суммарную длительность")
	}
	samePicture(t, img, testClipPicture())

	// Оборванная передача: канал остаётся открытым, данных больше нет.
	c2 := newWlTestWindow(t)
	foreignImageOffer(c2, 55, "image/png")
	_, wait2 := fakeImageOwner(t, c2, [][]byte{png[:half], nil}, 3*clipboardReadTimeout)
	if _, ok := c2.w.clip.GetImage(); ok {
		t.Error("оборванный PNG принят")
	}
	wait2()
}

// Мусор под видом PNG и картинка-бомба отвергаются, а не роняют.
func TestWlClipboard_GetImageRejectsGarbage(t *testing.T) {
	c := newWlTestWindow(t)
	foreignImageOffer(c, 56, "image/png")
	_, wait := fakeImageOwner(t, c, [][]byte{[]byte("это не картинка")}, 0)
	if _, ok := c.w.clip.GetImage(); ok {
		t.Error("мусор принят за PNG")
	}
	wait()

	// Заголовок обещает 60000×60000 точек в нескольких байтах.
	bomb := hugePNGHeader(60000, 60000)
	c2 := newWlTestWindow(t)
	foreignImageOffer(c2, 57, "image/png")
	_, wait2 := fakeImageOwner(t, c2, [][]byte{bomb}, 0)
	if _, ok := c2.w.clip.GetImage(); ok {
		t.Error("картинка-бомба принята")
	}
	wait2()
}

// Предел размера: сверх него — отказ, а не обрезанные данные; в пределах — всё.
func TestWlClipboard_ReceiveIdleLimit(t *testing.T) {
	c := newWlTestWindow(t)
	foreignImageOffer(c, 58, "image/png")
	_, wait := fakeImageOwner(t, c, [][]byte{bytes.Repeat([]byte{7}, 500)}, 0)
	if got := c.w.clip.receiveIdle(58, "image/png", clipboardReadTimeout, 100); got != nil {
		t.Errorf("сверх предела отдано %d байт", len(got))
	}
	wait()

	c2 := newWlTestWindow(t)
	foreignImageOffer(c2, 59, "image/png")
	_, wait2 := fakeImageOwner(t, c2, [][]byte{bytes.Repeat([]byte{7}, 500)}, 0)
	if got := c2.w.clip.receiveIdle(59, "image/png", clipboardReadTimeout, 1000); len(got) != 500 {
		t.Errorf("в пределах отдано %d байт, ждал 500", len(got))
	}
	wait2()
}
