// Package focusreq — запрос фокуса виджетом у движка, который сейчас
// доставляет ему событие.
//
// Клавиши движок отдаёт фокусному виджету. Виджет, открывший что-то
// управляемое с клавиатуры не в ответ на свою клавишу, а по чужой — строка
// меню по Alt+буква, которую приложение получает само и передаёт в
// MenuBar.ActivateMnemonic, — остаётся без следующих клавиш: стрелки и буква
// пункта уходят прежнему фокусному виджету. Попросить фокус ему не у кого:
// движок виджет не знает, а публичный «запрос фокуса» в widget — новый API.
//
// Движок на время доставки события объявляет себя исполнителем запросов
// (Serve), виджет просит (Request) и возвращает фокус, когда закончил
// (Return). Исполнитель привязан к горутине: несколько движков в одном
// процессе доставляют события независимо. Вне доставки запросы ничего не
// делают — тогда фокус ставит приложение (Engine.SetFocus).
package focusreq

import (
	"sync"

	"github.com/oops1/headless-gui/v3/internal/goid"
)

// Server — исполнитель запросов: движок.
type Server interface {
	RequestFocus(w any)
	ReturnFocus(w any)
}

var (
	mu      sync.Mutex
	servers = map[uint64]Server{}
)

// Serve объявляет s исполнителем на текущей горутине и возвращает функцию,
// которая вернёт прежнего (доставка может быть вложенной).
func Serve(s Server) (restore func()) {
	id := goid.Current()
	mu.Lock()
	prev, had := servers[id]
	servers[id] = s
	mu.Unlock()
	return func() {
		mu.Lock()
		if had {
			servers[id] = prev
		} else {
			delete(servers, id)
		}
		mu.Unlock()
	}
}

func current() Server {
	id := goid.Current()
	mu.Lock()
	s := servers[id]
	mu.Unlock()
	return s
}

// Request просит фокус для w. Возвращает false, если событие сейчас никто
// не доставляет.
func Request(w any) bool {
	if s := current(); s != nil {
		s.RequestFocus(w)
		return true
	}
	return false
}

// Return отдаёт фокус тому, у кого его забрал Request для w.
func Return(w any) {
	if s := current(); s != nil {
		s.ReturnFocus(w)
	}
}
