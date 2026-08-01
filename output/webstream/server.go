package webstream

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log"
	"net/http"
	"sync"
	"time"

	_ "embed"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

//go:embed viewer.html
var viewerHTML []byte

// Типы бинарных сообщений сервер → клиент.
const (
	msgTiles byte = 0x01 // [type][u16 n]{[u16 x][u16 y][u16 w][u16 h][u32 len][png]}×n
	msgInit  byte = 0x02 // [type][u16 width][u16 height]
)

// Server стримит кадры движка в браузерные вьюверы и возвращает ввод.
//
//	eng := engine.New(1060, 700, 30)
//	// ... построить UI, eng.Start()
//	srv := webstream.New(eng)
//	go srv.Run()                     // потребляет eng.Frames()
//	http.ListenAndServe(":8080", srv) // "/" — вьювер, "/ws" — поток
//
// Server — единственный потребитель eng.Frames(). Поддерживает несколько
// одновременных вьюверов: держит композит текущего кадра и отдаёт новому
// клиенту полный снимок (keyframe), дальше — только дельта-тайлы.
type Server struct {
	eng *engine.Engine

	mu        sync.Mutex
	clients   map[*wsConn]chan outMsg
	composite *image.RGBA // текущее полное состояние экрана (физич. пиксели)
	w, h      int

	// Счётчики для /stats: сколько кадров, тайлов и байтов ушло зрителям.
	// Полезны и в демонстрации, и при отладке «почему тормозит».
	frames, tiles, bytes int64
	started              time.Time
}

// Stats — сводка по стриму (см. Server.Stats и HTTP-эндпойнт /stats).
type Stats struct {
	Viewers  int   `json:"viewers"`  // сколько браузеров смотрит сейчас
	Frames   int64 `json:"frames"`   // кадров разослано
	Tiles    int64 `json:"tiles"`    // тайлов в них
	Bytes    int64 `json:"bytes"`    // суммарный объём тайлов (до умножения на зрителей)
	Width    int   `json:"width"`    // размер холста, физические пиксели
	Height   int   `json:"height"`   //
	UptimeMS int64 `json:"uptimeMs"` // сколько сервер уже стримит
}

// Stats возвращает текущую сводку. Потокобезопасно.
func (s *Server) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{
		Viewers:  len(s.clients),
		Frames:   s.frames,
		Tiles:    s.tiles,
		Bytes:    s.bytes,
		Width:    s.w,
		Height:   s.h,
		UptimeMS: time.Since(s.started).Milliseconds(),
	}
}

// New создаёт стример для движка.
func New(eng *engine.Engine) *Server {
	w, h := eng.PhysicalSize()
	return &Server{
		eng:       eng,
		clients:   make(map[*wsConn]chan outMsg),
		composite: image.NewRGBA(image.Rect(0, 0, w, h)),
		w:         w,
		h:         h,
		started:   time.Now(),
	}
}

// Run потребляет канал кадров движка до его закрытия (обычно в goroutine).
func (s *Server) Run() {
	for f := range s.eng.Frames() {
		if len(f.Tiles) == 0 {
			continue
		}
		s.mu.Lock()
		for _, t := range f.Tiles {
			s.applyTile(t)
		}
		msg, err := encodeTiles(f.Tiles)
		if err != nil {
			s.mu.Unlock()
			log.Printf("webstream: кодирование кадра %d: %v", f.Seq, err)
			continue
		}
		s.frames++
		s.tiles += int64(len(f.Tiles))
		s.bytes += int64(len(msg))
		for _, ch := range s.clients {
			select {
			case ch <- outMsg{op: opBinary, data: msg}:
			default: // медленный клиент — кадр пропускается (дельты догонят)
			}
		}
		s.mu.Unlock()
	}
}

// applyTile вносит тайл в композит (под s.mu).
func (s *Server) applyTile(t output.DirtyTile) {
	src := &image.RGBA{Pix: t.Data, Stride: t.W * 4, Rect: image.Rect(0, 0, t.W, t.H)}
	for y := 0; y < t.H; y++ {
		dstOff := s.composite.PixOffset(t.X, t.Y+y)
		copy(s.composite.Pix[dstOff:dstOff+t.W*4], src.Pix[y*src.Stride:(y+1)*src.Stride])
	}
}

// keyframe формирует полный снимок композита в виде сетки тайлов (под s.mu).
func (s *Server) keyframe() ([]byte, error) {
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
	return encodeTiles(tiles)
}

// encodeTiles сериализует тайлы: заголовок + PNG на каждый тайл.
func encodeTiles(tiles []output.DirtyTile) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(msgTiles)
	var n16 [2]byte
	binary.BigEndian.PutUint16(n16[:], uint16(len(tiles)))
	buf.Write(n16[:])

	var pngBuf bytes.Buffer
	for _, t := range tiles {
		img := &image.RGBA{Pix: t.Data, Stride: t.W * 4, Rect: image.Rect(0, 0, t.W, t.H)}
		pngBuf.Reset()
		if err := png.Encode(&pngBuf, img); err != nil {
			return nil, err
		}
		var hdr [12]byte
		binary.BigEndian.PutUint16(hdr[0:], uint16(t.X))
		binary.BigEndian.PutUint16(hdr[2:], uint16(t.Y))
		binary.BigEndian.PutUint16(hdr[4:], uint16(t.W))
		binary.BigEndian.PutUint16(hdr[6:], uint16(t.H))
		binary.BigEndian.PutUint32(hdr[8:], uint32(pngBuf.Len()))
		buf.Write(hdr[:])
		buf.Write(pngBuf.Bytes())
	}
	return buf.Bytes(), nil
}

// encodeInit — сообщение с размером холста.
func (s *Server) encodeInit() []byte {
	msg := make([]byte, 5)
	msg[0] = msgInit
	binary.BigEndian.PutUint16(msg[1:], uint16(s.w))
	binary.BigEndian.PutUint16(msg[3:], uint16(s.h))
	return msg
}

// ServeHTTP: "/" — встроенный вьювер, "/ws" — поток тайлов + ввод,
// "/stats" — сводка по стриму в JSON.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(viewerHTML)
	case "/snapshot.png":
		// Текущее состояние холста одним PNG: удобно для скриншотов в
		// документации, для мониторинга и для тестов, которым не нужен
		// WebSocket.
		s.mu.Lock()
		snap := image.NewRGBA(s.composite.Rect)
		copy(snap.Pix, s.composite.Pix)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, snap)
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
	ws, err := upgradeWS(w, r)
	if err != nil {
		return
	}
	defer ws.Close()

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

	// Регистрация + init + keyframe (под мьютексом, чтобы не потерять
	// дельты между снимком и подпиской).
	out := make(chan outMsg, 60)
	s.mu.Lock()
	kf, kerr := s.keyframe()
	s.clients[ws] = out
	s.mu.Unlock()
	if kerr != nil {
		log.Printf("webstream: keyframe: %v", kerr)
		return
	}
	defer func() {
		s.mu.Lock()
		delete(s.clients, ws)
		s.mu.Unlock()
	}()

	if err := ws.WriteMessage(opBinary, s.encodeInit()); err != nil {
		return
	}
	if err := ws.WriteMessage(opBinary, kf); err != nil {
		return
	}

	// Писатель: кадры из канала → сокет.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for m := range out {
			if err := ws.WriteMessage(m.op, m.data); err != nil {
				return
			}
		}
	}()

	// Читатель: JSON-события ввода → движок.
	for {
		op, data, err := ws.ReadMessage()
		if err != nil {
			break
		}
		if op == opText {
			s.dispatchInput(data, out)
		}
	}
	s.mu.Lock()
	delete(s.clients, ws)
	s.mu.Unlock()
	close(out)
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

// inputEvent — событие ввода от браузерного клиента.
type inputEvent struct {
	T  string  `json:"t"`            // "mm" | "mb" | "wh" | "kd" | "ku" | "pg"
	TS float64 `json:"ts,omitempty"` // метка времени пинга — возвращается как есть
	X  int     `json:"x,omitempty"`  // координаты (физические пиксели холста)
	Y  int     `json:"y,omitempty"`
	B  int     `json:"b,omitempty"` // кнопка (widget.MouseButton)
	P  bool    `json:"p,omitempty"` // pressed
	D  int     `json:"d,omitempty"` // направление колеса: -1 вверх, +1 вниз
	C  int     `json:"c,omitempty"` // KeyCode (совпадает с VK/keyCode браузера)
	R  int     `json:"r,omitempty"` // руна (codepoint) для печатных клавиш
	M  int     `json:"m,omitempty"` // модификаторы: 1=Ctrl 2=Shift 4=Alt
}

// dispatchInput транслирует событие клиента в движок.
func (s *Server) dispatchInput(data []byte, out chan<- outMsg) {
	var ev inputEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return
	}
	switch ev.T {
	case "pg":
		// Эхо-пинг: вьювер меряет задержку туда-обратно. Ответ уходит через
		// канал писателя — писать в сокет из читающей горутины нельзя.
		select {
		case out <- outMsg{op: opText, data: []byte(fmt.Sprintf(`{"t":"pg","ts":%v}`, ev.TS))}:
		default:
		}
	case "mm":
		s.eng.SendMouseMove(ev.X, ev.Y)
	case "mb":
		s.eng.SendMouseButton(ev.X, ev.Y, widget.MouseButton(ev.B), ev.P)
	case "wh":
		btn := widget.MouseWheelUp
		if ev.D > 0 {
			btn = widget.MouseWheelDown
		}
		s.eng.SendMouseButton(ev.X, ev.Y, btn, true)
	case "kd", "ku":
		mod := widget.KeyMod(0)
		if ev.M&1 != 0 {
			mod |= widget.ModCtrl
		}
		if ev.M&2 != 0 {
			mod |= widget.ModShift
		}
		if ev.M&4 != 0 {
			mod |= widget.ModAlt
		}
		r := rune(0)
		if ev.R >= 32 && mod&widget.ModCtrl == 0 {
			r = rune(ev.R)
		}
		s.eng.SendKeyEvent(widget.KeyEvent{
			Code:    widget.KeyCode(ev.C),
			Rune:    r,
			Mod:     mod,
			Pressed: ev.T == "kd",
		})
	}
}
