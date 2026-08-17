package webstream

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// Вектор из RFC 6455 §1.3.
func TestWSAccept(t *testing.T) {
	got := wsAccept("dGhlIHNhbXBsZSBub25jZQ==")
	if got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("wsAccept = %q", got)
	}
}

// Кодирование тайлов: заголовки + валидный PNG нужного размера.
func TestEncodeTiles(t *testing.T) {
	data := make([]byte, 8*4*4)
	for i := range data {
		data[i] = byte(i)
	}
	msg, err := encodeTiles([]output.DirtyTile{{X: 64, Y: 128, W: 8, H: 4, Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if msg[0] != msgTiles || binary.BigEndian.Uint16(msg[1:]) != 1 {
		t.Fatalf("заголовок: % x", msg[:3])
	}
	x := binary.BigEndian.Uint16(msg[3:])
	y := binary.BigEndian.Uint16(msg[5:])
	w := binary.BigEndian.Uint16(msg[7:])
	h := binary.BigEndian.Uint16(msg[9:])
	plen := binary.BigEndian.Uint32(msg[11:])
	if x != 64 || y != 128 || w != 8 || h != 4 {
		t.Fatalf("координаты: %d,%d %dx%d", x, y, w, h)
	}
	img, err := png.Decode(bytes.NewReader(msg[15 : 15+plen]))
	if err != nil {
		t.Fatalf("PNG не декодируется: %v", err)
	}
	if img.Bounds().Dx() != 8 || img.Bounds().Dy() != 4 {
		t.Fatalf("размер PNG: %v", img.Bounds())
	}
}

func TestOriginOK(t *testing.T) {
	cases := []struct {
		origin, host string
		allowed      []string
		want         bool
	}{
		{"", "example.com", nil, true},
		{"http://example.com", "example.com", nil, true},
		{"https://example.com", "example.com", nil, true},
		{"http://evil.com", "example.com", nil, false},
		{"null", "example.com", nil, false},
		{"http://evil.com", "example.com", []string{"http://evil.com"}, true},
		{"http://evil.com", "example.com", []string{"evil.com"}, true},
	}
	for _, c := range cases {
		if got := originOK(c.origin, c.host, c.allowed); got != c.want {
			t.Errorf("originOK(%q, %q, %v) = %v", c.origin, c.host, c.allowed, got)
		}
	}
}

// ─── Сырой WebSocket-клиент для e2e (только для теста) ──────────────────────

type rawWS struct {
	c  net.Conn
	br *bufio.Reader
}

// dialOpts — что подставить в тестовое рукопожатие.
type dialOpts struct {
	query   string
	origin  string
	version string
}

// handshake возвращает соединение и строку статуса ответа.
func handshake(t *testing.T, srvURL string, o dialOpts) (*rawWS, string) {
	t.Helper()
	u, _ := url.Parse(srvURL)
	c, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	ver := o.version
	if ver == "" {
		ver = "13"
	}
	req := "GET /ws" + o.query + " HTTP/1.1\r\nHost: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: " + ver + "\r\n"
	if o.origin != "" {
		req += "Origin: " + o.origin + "\r\n"
	}
	req += "\r\n"
	if _, err := c.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("ответ сервера: %v", err)
	}
	for { // остальные заголовки до пустой строки
		l, err := br.ReadString('\n')
		if err != nil || l == "\r\n" {
			break
		}
	}
	return &rawWS{c: c, br: br}, status
}

func dialWS(t *testing.T, srvURL string) *rawWS {
	t.Helper()
	ws, status := handshake(t, srvURL, dialOpts{})
	if !strings.Contains(status, "101") {
		t.Fatalf("рукопожатие: %q", status)
	}
	return ws
}

// readAny возвращает следующий кадр как есть (кадры сервера не маскированы).
func (r *rawWS) readAny() (byte, []byte, error) {
	r.c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var hdr [2]byte
	if _, err := io.ReadFull(r.br, hdr[:]); err != nil {
		return 0, nil, err
	}
	op := hdr[0] & 0x0F
	length := uint64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var e [2]byte
		if _, err := io.ReadFull(r.br, e[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(e[:]))
	case 127:
		var e [8]byte
		if _, err := io.ReadFull(r.br, e[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(e[:])
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r.br, data); err != nil {
		return 0, nil, err
	}
	return op, data, nil
}

// read возвращает следующее сообщение данных, пропуская ping сервера.
func (r *rawWS) read(t *testing.T) []byte {
	t.Helper()
	for {
		op, data, err := r.readAny()
		if err != nil {
			t.Fatalf("чтение кадра: %v", err)
		}
		if op == opPing || op == opPong {
			continue
		}
		return data
	}
}

// frame собирает клиентский кадр; mask=false — намеренное нарушение протокола.
func frame(op byte, fin, mask bool, payload []byte) []byte {
	b := []byte{op}
	if fin {
		b[0] |= 0x80
	}
	var flag byte
	if mask {
		flag = 0x80
	}
	switch {
	case len(payload) < 126:
		b = append(b, byte(len(payload))|flag)
	case len(payload) <= 0xFFFF:
		b = append(b, 126|flag, byte(len(payload)>>8), byte(len(payload)))
	default:
		b = append(b, 127|flag)
		var e [8]byte
		binary.BigEndian.PutUint64(e[:], uint64(len(payload)))
		b = append(b, e[:]...)
	}
	if !mask {
		return append(b, payload...)
	}
	m := [4]byte{0x12, 0x34, 0x56, 0x78}
	b = append(b, m[:]...)
	for i, p := range payload {
		b = append(b, p^m[i%4])
	}
	return b
}

// send отправляет текстовое сообщение с маской (клиент обязан маскировать).
func (r *rawWS) send(t *testing.T, s string) {
	t.Helper()
	if _, err := r.c.Write(frame(opText, true, true, []byte(s))); err != nil {
		t.Fatal(err)
	}
}

// ─── Тестовое окружение ─────────────────────────────────────────────────────

// newTestServer поднимает движок с одной кнопкой и стример поверх него.
func newTestServer(t *testing.T, opts Options, tune ...func(*Server)) (*Server, *httptest.Server, *widget.Button) {
	t.Helper()
	eng := engine.New(256, 128, 30)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 256, 128))
	btn := widget.NewWin10AccentButton("OK")
	btn.SetBounds(image.Rect(10, 10, 110, 44))
	root.AddChild(btn)
	eng.SetRoot(root)
	eng.Start()
	t.Cleanup(eng.Stop)

	srv := NewWithOptions(eng, opts)
	for _, f := range tune {
		f(srv)
	}
	go srv.Run()
	t.Cleanup(func() { srv.Close() })
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	return srv, hs, btn
}

// waitViewers ждёт нужное число зрителей.
func waitViewers(t *testing.T, srv *Server, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if srv.Stats().Viewers == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("зрителей %d, ожидалось %d", srv.Stats().Viewers, want)
}

// Сквозной тест: init + keyframe при подключении, клик по кнопке из
// «браузера» меняет UI, дельта-тайлы доезжают до клиента.
func TestServer_EndToEnd(t *testing.T) {
	_, hs, btn := newTestServer(t, Options{})
	clicked := make(chan struct{}, 1)
	btn.OnClick = func() {
		btn.SetText("DONE")
		clicked <- struct{}{}
	}

	ws := dialWS(t, hs.URL)
	defer ws.c.Close()

	// 1. init
	init := ws.read(t)
	if init[0] != msgInit {
		t.Fatalf("первое сообщение не init: 0x%x", init[0])
	}
	if w := binary.BigEndian.Uint16(init[1:]); w != 256 {
		t.Fatalf("init width=%d", w)
	}

	// 2. keyframe — полная сетка тайлов 256×128 → 4×2 = 8 тайлов.
	kf := ws.read(t)
	if kf[0] != msgTiles {
		t.Fatalf("второе сообщение не tiles: 0x%x", kf[0])
	}
	if n := binary.BigEndian.Uint16(kf[1:]); n != 8 {
		t.Fatalf("keyframe: %d тайлов, ожидалось 8", n)
	}

	// 3. Клик по кнопке «из браузера».
	ws.send(t, `{"t":"mb","x":40,"y":25,"b":0,"p":true}`)
	ws.send(t, `{"t":"mb","x":40,"y":25,"b":0,"p":false}`)
	select {
	case <-clicked:
	case <-time.After(3 * time.Second):
		t.Fatal("клик не дошёл до кнопки")
	}

	// 4. Дельта после изменения текста кнопки.
	delta := ws.read(t)
	if delta[0] != msgTiles {
		t.Fatalf("дельта не tiles: 0x%x", delta[0])
	}
	if n := binary.BigEndian.Uint16(delta[1:]); n == 0 || n > 8 {
		t.Fatalf("дельта: %d тайлов", n)
	}

	// 5. Клавиатура: Escape не должен уронить сервер (нет модалок).
	ws.send(t, `{"t":"kd","c":27}`)
	ws.send(t, `{"t":"ku","c":27}`)
	fmt.Fprintln(io.Discard)
}

// ─── Доступ ─────────────────────────────────────────────────────────────────

// Без токена закрыты /ws, /snapshot.png и /stats; вьювер отдаётся всегда.
func TestTokenRequired(t *testing.T) {
	_, hs, _ := newTestServer(t, Options{Token: "s3cret"})

	for _, p := range []string{"/snapshot.png", "/stats"} {
		resp, err := http.Get(hs.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s без токена: %d", p, resp.StatusCode)
		}

		resp, err = http.Get(hs.URL + p + "?token=s3cret")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s с токеном в query: %d", p, resp.StatusCode)
		}

		req, _ := http.NewRequest("GET", hs.URL+p, nil)
		req.Header.Set("Authorization", "Bearer s3cret")
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s с Bearer: %d", p, resp.StatusCode)
		}
	}

	if resp, err := http.Get(hs.URL + "/"); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("вьювер без токена: %d", resp.StatusCode)
		}
	}

	if _, status := handshake(t, hs.URL, dialOpts{}); !strings.Contains(status, "401") {
		t.Errorf("/ws без токена: %q", status)
	}
	ws, status := handshake(t, hs.URL, dialOpts{query: "?token=s3cret"})
	if !strings.Contains(status, "101") {
		t.Fatalf("/ws с токеном: %q", status)
	}
	defer ws.c.Close()
	if init := ws.read(t); init[0] != msgInit {
		t.Errorf("после авторизации пришло 0x%x", init[0])
	}
}

// Чужой Origin отбивается, свой и allowlist проходят.
func TestOriginRejected(t *testing.T) {
	_, hs, _ := newTestServer(t, Options{AllowedOrigins: []string{"http://friend.local"}})
	host := strings.TrimPrefix(hs.URL, "http://")

	if _, status := handshake(t, hs.URL, dialOpts{origin: "http://evil.example"}); !strings.Contains(status, "403") {
		t.Errorf("чужой Origin: %q", status)
	}
	if ws, status := handshake(t, hs.URL, dialOpts{origin: "http://" + host}); !strings.Contains(status, "101") {
		t.Errorf("свой Origin: %q", status)
	} else {
		ws.c.Close()
	}
	if ws, status := handshake(t, hs.URL, dialOpts{origin: "http://friend.local"}); !strings.Contains(status, "101") {
		t.Errorf("Origin из allowlist: %q", status)
	} else {
		ws.c.Close()
	}
	if _, status := handshake(t, hs.URL, dialOpts{version: "8"}); !strings.Contains(status, "400") {
		t.Errorf("версия 8: %q", status)
	}
}

// ─── Протокол ───────────────────────────────────────────────────────────────

// Немаскированный клиентский кадр → Close 1002 и разрыв.
func TestUnmaskedFrameClosesConn(t *testing.T) {
	srv, hs, _ := newTestServer(t, Options{})
	ws := dialWS(t, hs.URL)
	defer ws.c.Close()
	ws.read(t) // init
	ws.read(t) // keyframe

	if _, err := ws.c.Write(frame(opText, true, false, []byte(`{"t":"pg"}`))); err != nil {
		t.Fatal(err)
	}
	code := waitCloseCode(t, ws)
	if code != closeProtocol {
		t.Fatalf("код закрытия %d, ожидался %d", code, closeProtocol)
	}
	waitViewers(t, srv, 0, 3*time.Second)
}

// Склейка фрагментов ограничена: превышение → Close 1009.
func TestFragmentLimit(t *testing.T) {
	srv, hs, _ := newTestServer(t, Options{})
	ws := dialWS(t, hs.URL)
	defer ws.c.Close()
	ws.read(t)
	ws.read(t)

	chunk := bytes.Repeat([]byte("x"), 400<<10)
	ws.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := ws.c.Write(frame(opText, false, true, chunk)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := ws.c.Write(frame(opContinuation, false, true, chunk)); err != nil {
			break // сервер мог закрыть соединение уже на втором куске
		}
	}
	code := waitCloseCode(t, ws)
	if code != closeTooBig {
		t.Fatalf("код закрытия %d, ожидался %d", code, closeTooBig)
	}
	waitViewers(t, srv, 0, 3*time.Second)
}

// waitCloseCode читает кадры до Close и возвращает его код.
func waitCloseCode(t *testing.T, ws *rawWS) uint16 {
	t.Helper()
	for {
		op, data, err := ws.readAny()
		if err != nil {
			t.Fatalf("ожидался Close, получено: %v", err)
		}
		if op == opClose {
			if len(data) < 2 {
				return 0
			}
			return binary.BigEndian.Uint16(data)
		}
	}
}

// ─── Ввод ───────────────────────────────────────────────────────────────────

// Два клиента шлют ввод одновременно: движок дёргается из одной горутины.
func TestTwoClientsParallelInput(t *testing.T) {
	srv, hs, btn := newTestServer(t, Options{})
	clicks := make(chan struct{}, 100)
	btn.OnClick = func() {
		select {
		case clicks <- struct{}{}:
		default:
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		ws := dialWS(t, hs.URL)
		defer ws.c.Close()
		ws.read(t)
		ws.read(t)
		wg.Add(1)
		go func(ws *rawWS) {
			defer wg.Done()
			for j := 0; j < 40; j++ {
				ws.send(t, `{"t":"mm","x":40,"y":25}`)
				ws.send(t, `{"t":"mb","x":40,"y":25,"b":0,"p":true}`)
				ws.send(t, `{"t":"mb","x":40,"y":25,"b":0,"p":false}`)
				ws.send(t, `{"t":"kd","c":9}`)
			}
		}(ws)
	}
	wg.Wait()

	select {
	case <-clicks:
	case <-time.After(3 * time.Second):
		t.Fatal("ни один клик не дошёл")
	}
	if srv.Stats().Viewers != 2 {
		t.Errorf("зрителей %d, ожидалось 2", srv.Stats().Viewers)
	}
}

// Мусорные координаты, кнопки и коды клавиш до движка не доходят.
func TestValidInput(t *testing.T) {
	srv, _, _ := newTestServer(t, Options{})
	bad := []inputEvent{
		{T: "mm", X: -1, Y: 0},
		{T: "mm", X: 10000, Y: 0},
		{T: "mb", X: 10, Y: 10, B: 99},
		{T: "mb", X: 10, Y: 10, B: -1},
		{T: "kd", C: 5000},
		{T: "kd", C: 10, R: -3},
		{T: "zz"},
	}
	for _, ev := range bad {
		if srv.validInput(&ev) {
			t.Errorf("принято мусорное событие %+v", ev)
		}
	}
	good := []inputEvent{
		{T: "mm", X: 10, Y: 10},
		{T: "mb", X: 0, Y: 0, B: 2},
		{T: "wh", X: 5, Y: 5, D: 1},
		{T: "kd", C: 27},
		{T: "pg"},
	}
	for _, ev := range good {
		if !srv.validInput(&ev) {
			t.Errorf("отвергнуто нормальное событие %+v", ev)
		}
	}
}

// Token bucket: burst проходит, дальше поток режется.
func TestRateLimit(t *testing.T) {
	b := &bucket{}
	n := 0
	for i := 0; i < rateBurst*3; i++ {
		if b.allow() {
			n++
		}
	}
	if n > rateBurst+5 {
		t.Fatalf("пропущено %d событий, лимит burst=%d", n, rateBurst)
	}
	if n < rateBurst {
		t.Fatalf("пропущено всего %d, ожидался burst=%d", n, rateBurst)
	}
}

// ─── Таймауты и кэш ─────────────────────────────────────────────────────────

// Мёртвый пир: молчание дольше таймаута закрывает соединение и чистит клиента.
func TestDeadPeerTimeout(t *testing.T) {
	srv, hs, _ := newTestServer(t, Options{}, func(s *Server) {
		s.readTimeout = 200 * time.Millisecond
		s.pingInterval = time.Hour
	})
	ws := dialWS(t, hs.URL)
	defer ws.c.Close()
	ws.read(t)
	ws.read(t)
	waitViewers(t, srv, 1, time.Second)
	waitViewers(t, srv, 0, 5*time.Second) // клиент молчит — сервер его отцепил
}

// Оборванный TCP освобождает горутины клиента.
func TestBrokenPeerReleasesClient(t *testing.T) {
	srv, hs, _ := newTestServer(t, Options{})
	ws := dialWS(t, hs.URL)
	ws.read(t)
	ws.read(t)
	waitViewers(t, srv, 1, time.Second)
	ws.c.Close()
	waitViewers(t, srv, 0, 5*time.Second)
}

// Лишние зрители получают 503.
func TestMaxClients(t *testing.T) {
	srv, hs, _ := newTestServer(t, Options{MaxClients: 1})
	ws := dialWS(t, hs.URL)
	defer ws.c.Close()
	ws.read(t)
	waitViewers(t, srv, 1, time.Second)
	if _, status := handshake(t, hs.URL, dialOpts{}); !strings.Contains(status, "503") {
		t.Fatalf("второй зритель: %q", status)
	}
}

// Неизменившийся кадр не кодируется повторно: отдаётся тот же буфер.
func TestKeyframeCache(t *testing.T) {
	eng := engine.New(128, 128, 30)
	srv := New(eng)
	kf1, err := srv.keyframeMsg()
	if err != nil {
		t.Fatal(err)
	}
	kf2, err := srv.keyframeMsg()
	if err != nil {
		t.Fatal(err)
	}
	if &kf1[0] != &kf2[0] {
		t.Fatal("keyframe закодирован заново без изменения кадра")
	}
	srv.mu.Lock()
	srv.frames++ // новый кадр — кэш обязан протухнуть
	srv.mu.Unlock()
	kf3, err := srv.keyframeMsg()
	if err != nil {
		t.Fatal(err)
	}
	if &kf1[0] == &kf3[0] {
		t.Fatal("после нового кадра отдан старый keyframe")
	}
}

// /snapshot.png отдаёт валидный PNG и кэширует его на кадр.
func TestSnapshotPNG(t *testing.T) {
	srv, hs, _ := newTestServer(t, Options{})
	resp, err := http.Get(hs.URL + "/snapshot.png")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	img, err := png.Decode(resp.Body)
	if err != nil {
		t.Fatalf("PNG не декодируется: %v", err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 128 {
		t.Fatalf("размер снимка: %v", img.Bounds())
	}
	b1, err := srv.snapshotPNG()
	if err != nil {
		t.Fatal(err)
	}
	before := srv.Stats().Frames
	b2, _ := srv.snapshotPNG()
	if srv.Stats().Frames == before && &b1[0] != &b2[0] {
		t.Fatal("снимок закодирован заново без изменения кадра")
	}
}
