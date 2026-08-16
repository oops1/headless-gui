package webstream

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// Лимиты ввода.
const (
	inputQueueSize    = 256
	inputBlockTimeout = 100 * time.Millisecond
	maxInputMessage   = 4 << 10
	ratePerSec        = 500
	rateBurst         = 100
	maxKeyCode        = 255
	maxRune           = 0x10FFFF
)

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

// bucket — лимит частоты событий клиента, без блокировок.
type bucket struct {
	tokens float64
	last   time.Time
}

func (b *bucket) allow() bool {
	now := time.Now()
	if b.last.IsZero() {
		b.tokens = rateBurst
	} else {
		b.tokens += now.Sub(b.last).Seconds() * ratePerSec
		if b.tokens > rateBurst {
			b.tokens = rateBurst
		}
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// startInput поднимает единственный диспетчер: движку нужен однопоточный ввод.
func (s *Server) startInput() {
	s.inputOne.Do(func() { go s.dispatchLoop() })
}

func (s *Server) dispatchLoop() {
	for {
		select {
		case ev := <-s.input:
			s.applyInput(ev)
		case <-s.stop:
			return
		}
	}
}

// handleInput разбирает событие клиента: лимит частоты, валидация, очередь.
func (s *Server) handleInput(data []byte, c *client, lim *bucket) {
	var ev inputEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return
	}
	if !lim.allow() {
		s.dropped.Add(1)
		return
	}
	if !s.validInput(&ev) {
		return
	}
	if ev.T == "pg" {
		// Эхо-пинг через писателя: из читателя писать нельзя.
		select {
		case c.ch <- outMsg{op: opText, data: pongMsg(ev.TS)}:
		default:
		}
		return
	}
	s.enqueue(ev)
}

// enqueue: при переполнении движение мыши дропается, клики ждут таймаут.
func (s *Server) enqueue(ev inputEvent) {
	select {
	case s.input <- ev:
		return
	default:
	}
	if ev.T == "mm" {
		s.dropped.Add(1)
		return
	}
	t := time.NewTimer(inputBlockTimeout)
	defer t.Stop()
	select {
	case s.input <- ev:
	case <-t.C:
		s.dropped.Add(1)
	case <-s.stop:
	}
}

// validInput проверяет координаты, кнопку, код клавиши и руну.
func (s *Server) validInput(ev *inputEvent) bool {
	switch ev.T {
	case "pg":
		return true
	case "mm", "wh":
		return ev.X >= 0 && ev.Y >= 0 && ev.X <= s.w && ev.Y <= s.h
	case "mb":
		return ev.X >= 0 && ev.Y >= 0 && ev.X <= s.w && ev.Y <= s.h &&
			ev.B >= int(widget.MouseLeft) && ev.B <= int(widget.MouseMiddle)
	case "kd", "ku":
		return ev.C >= 0 && ev.C <= maxKeyCode && ev.R >= 0 && ev.R <= maxRune
	}
	return false
}

// applyInput транслирует событие в движок (только из горутины диспетчера).
func (s *Server) applyInput(ev inputEvent) {
	switch ev.T {
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

// pongMsg собирает эхо-ответ без Sprintf.
func pongMsg(ts float64) []byte {
	b := make([]byte, 0, 32)
	b = append(b, `{"t":"pg","ts":`...)
	b = strconv.AppendFloat(b, ts, 'f', -1, 64)
	return append(b, '}')
}
