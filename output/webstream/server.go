package webstream

import (
	"bytes"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/png"
	"log"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "embed"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
)

//go:embed viewer.html
var viewerHTML []byte

// Типы бинарных сообщений сервер → клиент.
const (
	msgTiles byte = 0x01 // [type][u16 n]{[u16 x][u16 y][u16 w][u16 h][u32 len][png]}×n
	msgInit  byte = 0x02 // [type][u16 width][u16 height]
)

// Лимиты по умолчанию.
const (
	defaultMaxClients = 16
	clientQueueSize   = 60
	maxSnapshotJobs   = 2
)

// Options — настройки доступа и лимитов (см. NewWithOptions).
type Options struct {
	Token          string   // если задан — нужен ?token= или Authorization: Bearer
	AllowedOrigins []string // дополнительные Origin для WebSocket
	MaxClients     int      // одновременных вьюверов; 0 → 16
}

// Server стримит кадры движка в браузерные вьюверы и возвращает ввод.
//
//	eng := engine.New(1060, 700, 30)
//	// ... построить UI, eng.Start()
//	srv := webstream.New(eng)
//	go srv.Run()                     // потребляет eng.Frames()
//	http.ListenAndServe("127.0.0.1:8080", srv)
//
// Server — единственный потребитель eng.Frames(). Поддерживает несколько
// одновременных вьюверов: держит композит текущего кадра и отдаёт новому
// клиенту полный снимок (keyframe), дальше — только дельта-тайлы.
type Server struct {
	eng  *engine.Engine
	opts Options

	mu        sync.Mutex
	clients   map[*wsConn]*client
	composite *image.RGBA // текущее полное состояние экрана (физич. пиксели)
	w, h      int

	// Счётчики для /stats: сколько кадров, тайлов и байтов ушло зрителям.
	// Полезны и в демонстрации, и при отладке «почему тормозит».
	frames, tiles, bytes int64
	started              time.Time

	kfMsg   []byte // кэш keyframe-сообщения на номер кадра kfSeq
	kfSeq   int64
	snapPNG []byte // кэш /snapshot.png на номер кадра snapSeq
	snapSeq int64
	snapSem chan struct{}

	dropped  atomic.Int64
	input    chan inputEvent
	inputOne sync.Once
	stopOne  sync.Once
	stop     chan struct{}

	// Таймауты сокета; тесты укорачивают их до подключения клиентов.
	readTimeout, writeTimeout, pingInterval time.Duration
}

// client — состояние одного вьювера.
type client struct {
	ch     chan outMsg
	needKF bool // дельта потеряна, ждёт полный кадр
}

// Stats — сводка по стриму (см. Server.Stats и HTTP-эндпойнт /stats).
type Stats struct {
	Viewers      int   `json:"viewers"`      // сколько браузеров смотрит сейчас
	Frames       int64 `json:"frames"`       // кадров разослано
	Tiles        int64 `json:"tiles"`        // тайлов в них
	Bytes        int64 `json:"bytes"`        // суммарный объём тайлов (до умножения на зрителей)
	Width        int   `json:"width"`        // размер холста, физические пиксели
	Height       int   `json:"height"`       //
	UptimeMS     int64 `json:"uptimeMs"`     // сколько сервер уже стримит
	InputDropped int64 `json:"inputDropped"` // событий ввода отброшено (лимиты)
}

// Stats возвращает текущую сводку. Потокобезопасно.
func (s *Server) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{
		Viewers:      len(s.clients),
		Frames:       s.frames,
		Tiles:        s.tiles,
		Bytes:        s.bytes,
		Width:        s.w,
		Height:       s.h,
		UptimeMS:     time.Since(s.started).Milliseconds(),
		InputDropped: s.dropped.Load(),
	}
}

// New создаёт стример для движка с настройками по умолчанию.
func New(eng *engine.Engine) *Server { return NewWithOptions(eng, Options{}) }

// NewWithOptions создаёт стример с токеном доступа, allowlist и лимитом зрителей.
func NewWithOptions(eng *engine.Engine, opts Options) *Server {
	if opts.MaxClients <= 0 {
		opts.MaxClients = defaultMaxClients
	}
	w, h := eng.PhysicalSize()
	return &Server{
		eng:       eng,
		opts:      opts,
		clients:   make(map[*wsConn]*client),
		composite: image.NewRGBA(image.Rect(0, 0, w, h)),
		w:         w,
		h:         h,
		started:   time.Now(),
		snapSem:   make(chan struct{}, maxSnapshotJobs),
		input:     make(chan inputEvent, inputQueueSize),
		stop:      make(chan struct{}),

		readTimeout:  defaultReadTimeout,
		writeTimeout: defaultWriteTimeout,
		pingInterval: defaultPingInterval,
	}
}

// Run потребляет канал кадров движка до его закрытия (обычно в goroutine).
func (s *Server) Run() {
	s.startInput()
	defer s.Close()
	for f := range s.eng.Frames() {
		if len(f.Tiles) == 0 {
			continue
		}
		s.mu.Lock()
		for _, t := range f.Tiles {
			s.applyTile(t)
		}
		s.frames++
		s.tiles += int64(len(f.Tiles))
		s.mu.Unlock()

		msg, err := encodeTiles(f.Tiles) // кодирование вне s.mu
		if err != nil {
			log.Printf("webstream: кодирование кадра %d: %v", f.Seq, err)
			continue
		}

		s.mu.Lock()
		s.bytes += int64(len(msg))
		lost := false
		for _, c := range s.clients {
			if c.needKF {
				lost = true
				continue
			}
			select {
			case c.ch <- outMsg{op: opBinary, data: msg}:
			default: // медленный клиент — дельта пропущена, дошлём полный кадр
				c.needKF = true
				lost = true
			}
		}
		s.mu.Unlock()
		if lost {
			s.sendKeyframes()
		}
	}
}

// Close останавливает диспетчер ввода и разрывает соединения вьюверов.
func (s *Server) Close() error {
	s.stopOne.Do(func() { close(s.stop) })
	s.mu.Lock()
	for ws := range s.clients {
		ws.Close()
	}
	s.mu.Unlock()
	return nil
}

// sendKeyframes досылает полный кадр клиентам, потерявшим дельту.
func (s *Server) sendKeyframes() {
	kf, err := s.keyframeMsg()
	if err != nil {
		log.Printf("webstream: keyframe: %v", err)
		return
	}
	s.mu.Lock()
	for _, c := range s.clients {
		if !c.needKF {
			continue
		}
		select {
		case c.ch <- outMsg{op: opBinary, data: kf}:
			c.needKF = false
		default:
		}
	}
	s.mu.Unlock()
}

// applyTile вносит тайл в композит (под s.mu).
func (s *Server) applyTile(t output.DirtyTile) {
	src := &image.RGBA{Pix: t.Data, Stride: t.W * 4, Rect: image.Rect(0, 0, t.W, t.H)}
	for y := 0; y < t.H; y++ {
		dstOff := s.composite.PixOffset(t.X, t.Y+y)
		copy(s.composite.Pix[dstOff:dstOff+t.W*4], src.Pix[y*src.Stride:(y+1)*src.Stride])
	}
}

// snapshotTiles режет композит на сетку тайлов (под s.mu).
func (s *Server) snapshotTiles() []output.DirtyTile {
	var tiles []output.DirtyTile
	for y := 0; y < s.h; y += output.TileSize {
		for x := 0; x < s.w; x += output.TileSize {
			w, h := output.TileSize, output.TileSize
			if x+w > s.w {
				w = s.w - x
			}
			if y+h > s.h {
				h = s.h - y
			}
			data := make([]byte, w*h*4)
			for row := 0; row < h; row++ {
				off := s.composite.PixOffset(x, y+row)
				copy(data[row*w*4:(row+1)*w*4], s.composite.Pix[off:off+w*4])
			}
			tiles = append(tiles, output.DirtyTile{X: x, Y: y, W: w, H: h, Data: data})
		}
	}
	return tiles
}

// keyframeMsg возвращает полный кадр из кэша либо кодирует его вне s.mu.
func (s *Server) keyframeMsg() ([]byte, error) {
	s.mu.Lock()
	seq := s.frames
	if s.kfMsg != nil && s.kfSeq == seq {
		msg := s.kfMsg
		s.mu.Unlock()
		return msg, nil
	}
	tiles := s.snapshotTiles()
	s.mu.Unlock()

	msg, err := encodeTiles(tiles)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.kfMsg == nil || seq >= s.kfSeq {
		s.kfMsg, s.kfSeq = msg, seq
	}
	s.mu.Unlock()
	return msg, nil
}

// snapshotPNG возвращает холст одним PNG: кэш на кадр, кодирование вне s.mu.
func (s *Server) snapshotPNG() ([]byte, error) {
	s.mu.Lock()
	seq := s.frames
	if s.snapPNG != nil && s.snapSeq == seq {
		b := s.snapPNG
		s.mu.Unlock()
		return b, nil
	}
	snap := image.NewRGBA(s.composite.Rect)
	copy(snap.Pix, s.composite.Pix)
	s.mu.Unlock()

	s.snapSem <- struct{}{} // не больше двух кодировок разом
	defer func() { <-s.snapSem }()

	var buf bytes.Buffer
	if err := pngEncoder.Encode(&buf, snap); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	s.mu.Lock()
	if s.snapPNG == nil || seq >= s.snapSeq {
		s.snapPNG, s.snapSeq = b, seq
	}
	s.mu.Unlock()
	return b, nil
}

// pngEncoder — общий кодировщик PNG для тайлов.
//
// Два решения ради скорости, и оба ощутимы на порядок:
//   - BestSpeed вместо компрессии по умолчанию: содержимое тайла — куски
//     интерфейса (плоские заливки, текст), они и так сжимаются отлично, а
//     стандартный уровень тратил ~140 мкс на тайл — 8-9 мс на средний кадр
//     и ~80 мс на keyframe, всё в один поток.
//   - BufferPool: png.Encode без него аллоцирует ~850 КБ на КАЖДЫЙ тайл
//     (внутренний zlib-писатель и буферы строк) — при 30 FPS это давило GC.
var pngEncoder = png.Encoder{
	CompressionLevel: png.BestSpeed,
	BufferPool:       &pngPool{},
}

// pngPool — пул png.EncoderBuffer (потокобезопасность даёт sync.Pool).
type pngPool struct{ p sync.Pool }

func (pp *pngPool) Get() *png.EncoderBuffer {
	b, _ := pp.p.Get().(*png.EncoderBuffer)
	return b // nil допустим — кодировщик создаст новый
}
func (pp *pngPool) Put(b *png.EncoderBuffer) { pp.p.Put(b) }

// encodedTile — один тайл, упакованный воркером.
type encodedTile struct {
	hdr [12]byte
	png []byte
}

// encodeTiles сериализует тайлы: заголовок + PNG на каждый тайл.
// Тайлы кодируются ПАРАЛЛЕЛЬНО: они независимы, а именно кодирование — самая
// дорогая часть стрима (движок рендерит кадр быстрее, чем один поток
// упаковывал). Порядок в выходном буфере сохраняется — клиент полагается на
// заголовки, но детерминированный вывод дешевле и в отладке, и в тестах.
func encodeTiles(tiles []output.DirtyTile) ([]byte, error) {
	encoded := make([]encodedTile, len(tiles))
	workers := runtime.GOMAXPROCS(0)
	if workers > len(tiles) {
		workers = len(tiles)
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	var firstErr atomic.Pointer[error]
	next := atomic.Int64{}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var pngBuf bytes.Buffer
			for {
				i := int(next.Add(1)) - 1
				if i >= len(tiles) {
					return
				}
				t := tiles[i]
				img := &image.RGBA{Pix: t.Data, Stride: t.W * 4, Rect: image.Rect(0, 0, t.W, t.H)}
				pngBuf.Reset()
				if err := pngEncoder.Encode(&pngBuf, img); err != nil {
					firstErr.CompareAndSwap(nil, &err)
					return
				}
				et := &encoded[i]
				binary.BigEndian.PutUint16(et.hdr[0:], uint16(t.X))
				binary.BigEndian.PutUint16(et.hdr[2:], uint16(t.Y))
				binary.BigEndian.PutUint16(et.hdr[4:], uint16(t.W))
				binary.BigEndian.PutUint16(et.hdr[6:], uint16(t.H))
				binary.BigEndian.PutUint32(et.hdr[8:], uint32(pngBuf.Len()))
				et.png = append(et.png[:0], pngBuf.Bytes()...)
			}
		}()
	}
	wg.Wait()
	if p := firstErr.Load(); p != nil {
		return nil, *p
	}

	total := 3
	for i := range encoded {
		total += 12 + len(encoded[i].png)
	}
	buf := make([]byte, 0, total)
	buf = append(buf, msgTiles)
	var n16 [2]byte
	binary.BigEndian.PutUint16(n16[:], uint16(len(tiles)))
	buf = append(buf, n16[:]...)
	for i := range encoded {
		buf = append(buf, encoded[i].hdr[:]...)
		buf = append(buf, encoded[i].png...)
	}
	return buf, nil
}

// encodeInit — сообщение с размером холста.
func (s *Server) encodeInit() []byte {
	msg := make([]byte, 5)
	msg[0] = msgInit
	binary.BigEndian.PutUint16(msg[1:], uint16(s.w))
	binary.BigEndian.PutUint16(msg[3:], uint16(s.h))
	return msg
}

// authorized проверяет токен доступа: ?token= или Authorization: Bearer.
func (s *Server) authorized(r *http.Request) bool {
	if s.opts.Token == "" {
		return true
	}
	got := r.URL.Query().Get("token")
	if got == "" {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			got = strings.TrimPrefix(h, "Bearer ")
		}
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.opts.Token)) == 1
}

// ServeHTTP: "/" — встроенный вьювер, "/ws" — поток тайлов + ввод,
// "/stats" — сводка по стриму в JSON.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(viewerHTML)
		return
	}
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="webstream"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/snapshot.png":
		// Текущее состояние холста одним PNG: удобно для скриншотов в
		// документации, для мониторинга и для тестов, которым не нужен
		// WebSocket.
		b, err := s.snapshotPNG()
		if err != nil {
			http.Error(w, "encode failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(b)
	case "/stats":
		// Вьювер показывает эти числа в строке состояния; приложению они
		// пригодятся для мониторинга (сколько зрителей, сколько трафика).
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.Stats())
	case "/ws":
		s.handleWS(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	full := len(s.clients) >= s.opts.MaxClients
	s.mu.Unlock()
	if full {
		http.Error(w, "too many viewers", http.StatusServiceUnavailable)
		return
	}
	ws, err := upgradeWS(w, r, wsConfig{
		allowedOrigins: s.opts.AllowedOrigins,
		readTimeout:    s.readTimeout,
		writeTimeout:   s.writeTimeout,
	})
	if err != nil {
		return
	}
	defer ws.Close()
	s.startInput()

	// Первый зритель может подключиться раньше, чем движок нарисовал хоть
	// один кадр (при рендере по требованию кадр рождается только на
	// изменение UI, а самый первый мог уйти в никуда, пока Run ещё не начал
	// читать канал). Тогда композит пуст, и клиент увидел бы чёрный
	// прямоугольник — просим движок перерисоваться целиком.
	s.mu.Lock()
	empty := s.frames == 0
	s.mu.Unlock()
	if empty {
		s.eng.Invalidate()
		s.waitFirstFrame(time.Second)
	}

	c := &client{ch: make(chan outMsg, clientQueueSize)}
	s.mu.Lock()
	s.clients[ws] = c
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ws)
		s.mu.Unlock()
	}()

	kf, err := s.keyframeMsg()
	if err != nil {
		log.Printf("webstream: keyframe: %v", err)
		return
	}
	// Дельты, накопленные до снимка, устарели — keyframe их перекрывает.
	for drained := false; !drained; {
		select {
		case <-c.ch:
		default:
			drained = true
		}
	}
	if err := ws.WriteMessage(opBinary, s.encodeInit()); err != nil {
		return
	}
	if err := ws.WriteMessage(opBinary, kf); err != nil {
		return
	}

	// Писатель: кадры из канала и серверные ping → сокет.
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer ws.Close() // застрявший писатель будит читателя
		t := time.NewTicker(s.pingInterval)
		defer t.Stop()
		for {
			select {
			case m, ok := <-c.ch:
				if !ok {
					return
				}
				if err := ws.WriteMessage(m.op, m.data); err != nil {
					return
				}
			case <-t.C:
				if err := ws.WriteMessage(opPing, nil); err != nil {
					return
				}
			case <-s.stop:
				return
			}
		}
	}()

	// Читатель: JSON-события ввода → очередь диспетчера.
	lim := &bucket{}
	for {
		op, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		if op == opText && len(data) <= maxInputMessage {
			s.handleInput(data, c, lim)
		}
	}
	s.mu.Lock()
	delete(s.clients, ws)
	s.mu.Unlock()
	close(c.ch)
	<-done
}

// waitFirstFrame ждёт, пока Run применит первый кадр к композиту (но не
// дольше timeout): без этого новый клиент получил бы пустой снимок.
func (s *Server) waitFirstFrame(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got := s.frames > 0
		s.mu.Unlock()
		if got {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// outMsg — сообщение в сокет клиента: бинарные тайлы или текстовый ответ.
type outMsg struct {
	op   byte
	data []byte
}

// LogListen печатает фактический адрес прослушивания и предупреждает о рисках.
func LogListen(addr, token string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("webstream: слушаю %s", addr)
		return
	}
	shown := addr
	if host == "" || host == "0.0.0.0" || host == "::" {
		if host == "" {
			host = "0.0.0.0"
		}
		shown = host + ":" + port + " (все интерфейсы!)"
	}
	log.Printf("webstream: слушаю http://%s", shown)
	if !loopbackHost(host) && token == "" {
		log.Printf("webstream: ВНИМАНИЕ — доступ снаружи без токена и без TLS; задайте -token и ставьте за TLS-прокси")
	}
}

// loopbackHost — адрес виден только с этой машины.
func loopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
