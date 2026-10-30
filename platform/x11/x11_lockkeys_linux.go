//go:build linux && !android

package x11

// x11_lockkeys_linux.go — клавиши-переключатели X11: обмен с сервером в цикле
// событий окна (откуда берётся состояние и как — в x11_lockkeys.go).
//
// Все запросы здесь с ответом, а читать сокет вне цикла событий нельзя
// (x11touch_linux.go): ответ мог бы достаться не тому, кто его ждёт, или
// отнять событие у цикла. Поэтому, как у захвата клавиатуры и подписки на
// владельца буфера, вопрос приложения не посылается сразу, а встаёт в очередь
// заданий окна; цикл событий будится сообщением самому себе (SendEvent без
// ответа), исполняет задание на верхнем уровне, где запрос-ответ безопасен, и
// отдаёт итог ждущему. Ждущий ограничен сроком: цикл не крутится (окно ещё не
// запущено или уже закрыто) — вопрос не виснет.
//
// Расширение и имена индикаторов узнаются при первом вопросе (несколько
// запросов, миллисекунды) и запоминаются до MappingNotify: смена раскладки
// могла переименовать индикаторы и переставить модификаторы. Само состояние
// не запоминается — оно меняется чужими руками в любой момент, и каждый вопрос
// стоит одного-двух обращений к серверу.

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/platform/keyboard"
)

// x11LockTimeout — сколько приложение ждёт ответ цикла событий.
const x11LockTimeout = time.Second

// x11LockJob — вопрос или просьба приложения.
type x11LockJob struct {
	set  bool              // выставить want, а не прочитать
	want keyboard.LockKeys // для set
	done chan x11LockResult
}

// x11LockResult — итог задания.
type x11LockResult struct {
	keys keyboard.LockKeys // для чтения: состояние
	ok   bool              // для чтения: удалось узнать; для set: удалось выставить
}

// x11LockState — очередь заданий окна и то, что цикл событий узнал о сервере.
type x11LockState struct {
	mu    sync.Mutex
	jobs  []*x11LockJob
	dirty atomic.Bool // в очереди есть задания
	// looping — цикл событий читает соединение и его можно будить.
	looping atomic.Bool
	// stale — раскладка менялась (MappingNotify), договорённость устарела.
	stale atomic.Bool

	// Дальше — только цикл событий.
	probed bool
	major  byte // опкод XKEYBOARD, если xkb
	m      x11LockMap
}

// invalidate просит перечитать договорённость: поменялась раскладка.
func (s *x11LockState) invalidate() { s.stale.Store(true) }

// lkAsk ставит задание в очередь и ждёт итог. ok == false — цикл событий не
// ответил.
func (w *X11Window) lkAsk(job *x11LockJob) (x11LockResult, bool) {
	if w.wid == 0 || w.conn == nil || !w.lk.looping.Load() {
		return x11LockResult{}, false
	}
	job.done = make(chan x11LockResult, 1)
	w.lk.mu.Lock()
	w.lk.jobs = append(w.lk.jobs, job)
	w.lk.mu.Unlock()
	w.lk.dirty.Store(true)
	// Цикл спит в чтении: будим сообщением самому себе (см. x11_keygrab_linux.go).
	w.x11Send(x11SelfWakeRequest(w.wid, w.atomTargets))
	t := time.NewTimer(x11LockTimeout)
	defer t.Stop()
	select {
	case r := <-job.done:
		return r, true
	case <-t.C:
		return x11LockResult{}, false
	}
}

// LockKeys — состояние переключателей (lockKeysBackend).
func (w *X11Window) LockKeys() (keyboard.LockKeys, bool) {
	r, ok := w.lkAsk(&x11LockJob{})
	if !ok || !r.ok {
		return 0, false
	}
	return r.keys, true
}

// SetLockKeys приводит переключатели к want (lockKeysSetter). Нужен XKB:
// зафиксированные модификаторы ядерным протоколом не поменять.
func (w *X11Window) SetLockKeys(want keyboard.LockKeys) bool {
	r, ok := w.lkAsk(&x11LockJob{set: true, want: want})
	return ok && r.ok
}

// x11SyncLockKeys исполняет задания приложения. Зовётся циклом событий между
// событиями, на верхнем уровне, где запрос-ответ безопасен.
func (w *X11Window) x11SyncLockKeys() {
	if !w.lk.dirty.Swap(false) {
		return
	}
	w.lk.mu.Lock()
	jobs := w.lk.jobs
	w.lk.jobs = nil
	w.lk.mu.Unlock()
	for _, job := range jobs {
		job.done <- w.lkRun(job)
	}
}

// lkFailAll отвечает «не вышло» всем ждущим: цикл событий закончился.
func (w *X11Window) lkFailAll() {
	w.lk.mu.Lock()
	jobs := w.lk.jobs
	w.lk.jobs = nil
	w.lk.mu.Unlock()
	for _, job := range jobs {
		job.done <- x11LockResult{}
	}
}

// lkRun исполняет одно задание. Только цикл событий.
func (w *X11Window) lkRun(job *x11LockJob) x11LockResult {
	if !w.lkProbe() {
		return x11LockResult{}
	}
	cur, ok := w.lkReadState()
	if !ok {
		return x11LockResult{}
	}
	if !job.set {
		return x11LockResult{keys: cur, ok: true}
	}
	return x11LockResult{ok: w.lkApply(cur, job.want)}
}

// lkProbe узнаёт у сервера, чем он умеет: XKB, имена индикаторов, модификаторы
// клавиш. Один раз, а после MappingNotify — заново. false — читать состояние
// нечем.
func (w *X11Window) lkProbe() bool {
	s := &w.lk
	if s.probed && !s.stale.Swap(false) {
		return true
	}
	s.probed = false
	s.major, s.m = 0, x11LockMap{}

	// XKB: расширение, согласование версии, индикаторы по именам.
	if reply, ok := w.x11Reply(x11QueryExtensionRequest(xkbExtensionName)); ok {
		if major, _, present := parseQueryExtensionReply(reply); present {
			if reply, ok := w.x11Reply(x11XkbUseExtensionRequest(major)); ok && parseXkbUseExtensionReply(reply) {
				s.major, s.m.xkb = major, true
			}
		}
	}
	if s.m.xkb {
		if reply, ok := w.x11Reply(x11XkbGetIndicatorNamesRequest(s.major)); ok {
			if atoms, ok := parseXkbIndicatorNamesReply(reply); ok {
				s.m.indicator = x11LockIndicatorBits(atoms, func(name string) uint32 {
					reply, ok := w.x11Reply(x11InternAtomRequest(name, true))
					if !ok || len(reply) < 12 {
						return 0
					}
					return binary.LittleEndian.Uint32(reply[8:12])
				})
			}
		}
	}

	// Модификаторы клавиш: Caps Lock всегда на Lock, остальные — по символу.
	s.m.mods[0] = x11LockMask
	mapped := false
	if reply, ok := w.x11Reply(x11GetModifierMappingRequest()); ok {
		if perMod, codes, ok := parseModifierMappingReply(reply); ok && w.keysyms != nil {
			syms := w.lkKeysymsOf
			s.m.mods[1] = x11ModMaskForKeysym(perMod, codes, syms, keysymNumLock)
			s.m.mods[2] = x11ModMaskForKeysym(perMod, codes, syms, keysymScrollLock)
			s.m.mods[3] = x11ModMaskForKeysym(perMod, codes, syms, keysymKanaLock)
			mapped = true
		}
	}
	if !mapped {
		s.m.mods[1] = x11Mod2Mask // таблицы нет: штатный Mod2 для Num Lock
	}
	s.probed = true
	return true
}

// lkKeysymsOf — символы клавиши keycode по таблице, загруженной окном.
func (w *X11Window) lkKeysymsOf(code byte) []uint32 {
	if w.keysyms == nil || w.symsPerCode == 0 || code < w.minKeycode {
		return nil
	}
	row := int(code-w.minKeycode) * w.symsPerCode
	if row+w.symsPerCode > len(w.keysyms) {
		return nil
	}
	return w.keysyms[row : row+w.symsPerCode]
}

// lkReadState читает состояние переключателей теми запросами, что нужны.
func (w *X11Window) lkReadState() (keyboard.LockKeys, bool) {
	s := &w.lk
	var (
		indicators uint32
		locked     uint8
		ptr        uint16
	)
	if s.m.xkb {
		if s.m.needsIndicators() {
			reply, ok := w.x11Reply(x11XkbGetIndicatorStateRequest(s.major))
			if !ok {
				return 0, false
			}
			if indicators, ok = parseXkbIndicatorStateReply(reply); !ok {
				return 0, false
			}
		}
		if s.m.needsMods() {
			reply, ok := w.x11Reply(x11XkbGetStateRequest(s.major))
			if !ok {
				return 0, false
			}
			if locked, ok = parseXkbGetStateReply(reply); !ok {
				return 0, false
			}
		}
	} else {
		reply, ok := w.x11Reply(x11QueryPointerRequest(w.rootWin))
		if !ok {
			return 0, false
		}
		if ptr, ok = parseQueryPointerMask(reply); !ok {
			return 0, false
		}
	}
	return s.m.resolve(indicators, locked, ptr), true
}

// lkApply приводит переключатели от cur к want и проверяет итог. true — все
// отличавшиеся клавиши теперь как просили.
func (w *X11Window) lkApply(cur, want keyboard.LockKeys) bool {
	s := &w.lk
	if !s.m.xkb {
		return cur == want // менять нечем; совпало — и хорошо
	}
	var affect, locks uint8
	complete := true
	handled := keyboard.LockKeys(0)
	for i, f := range x11LockFlags {
		if (cur^want)&f == 0 {
			continue
		}
		mask := s.m.mods[i]
		if mask == 0 {
			complete = false // у клавиши нет модификатора: XKB её не переключит
			continue
		}
		affect |= mask
		if want&f != 0 {
			locks |= mask
		}
		handled |= f
	}
	if affect == 0 {
		return complete
	}
	w.x11Send(x11XkbLatchLockStateRequest(s.major, affect, locks))
	// У запроса нет ответа, а ошибку сервер пришлёт вместо чего-то другого:
	// проверяем отдельно (GetInputFocus), иначе она досталась бы следующему
	// запросу и сбила бы разбор его ответа.
	if !w.x11SyncCheck() {
		return false
	}
	got, ok := w.lkReadState()
	if !ok {
		return false
	}
	return complete && (got^want)&handled == 0
}
