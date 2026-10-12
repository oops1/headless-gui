package engine

import (
	"image"
	"reflect"
	"sync"
	"sync/atomic"
)

// Сброс кэша масштабированных изображений на уровне движка.
//
// Кэш DrawImageScaled живёт на холсте (scaledCache), а холстов у движка
// несколько: основной и по одному на каждый вынесенный в нативное окно попап.
// Приложение, которое переписало пиксели картинки на месте, знает про
// картинку, а не про холсты, — поэтому сброс заказывается у движка
// (Engine.InvalidateImageCache) и доходит до всех холстов через общий журнал.
//
// Журнал, а не прямой обход холстов: заказать сброс могут с любой горутины
// (обработчик события, рабочий поток декодера), а кэш холста принадлежит
// горутине кадра и мьютекса не имеет. Журнал защищён своим мьютексом, а холст
// читает его из своей горутины — одним атомарным сравнением номера на каждое
// обращение к кэшу.

// maxInvLog — сколько последних сбросов хранит журнал. Холст, отставший
// больше, чем на столько записей, сбрасывает кэш целиком: это всегда верно,
// просто дороже.
const maxInvLog = 64

// invRec — одна запись журнала: сброс источника по адресу или всего кэша.
type invRec struct {
	seq uint64
	ptr uintptr
	all bool
}

// imageInvalidator — журнал сбросов кэша изображений одного движка.
// Адреса, а не сами изображения: журнал не должен удерживать картинки живыми.
// Запись о давно умершем источнике безвредна — кэш удерживает источник, пока
// держит его кадр, и записей с чужим адресом там не бывает.
type imageInvalidator struct {
	mu  sync.Mutex
	seq atomic.Uint64 // номер последней записи
	log []invRec
}

// add дописывает сброс в журнал.
func (j *imageInvalidator) add(ptr uintptr, all bool) {
	j.mu.Lock()
	n := j.seq.Load() + 1
	if len(j.log) >= maxInvLog {
		copy(j.log, j.log[1:])
		j.log = j.log[:len(j.log)-1]
	}
	j.log = append(j.log, invRec{seq: n, ptr: ptr, all: all})
	j.seq.Store(n)
	j.mu.Unlock()
}

// since отдаёт записи новее seen и номер последней. lost — журнал уже
// выбросил часть нужных записей, и холст обязан сбросить кэш целиком.
func (j *imageInvalidator) since(seen uint64) (recs []invRec, last uint64, lost bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	last = j.seq.Load()
	if len(j.log) > 0 && j.log[0].seq > seen+1 {
		return nil, last, true
	}
	for _, r := range j.log {
		if r.seq > seen {
			recs = append(recs, r)
		}
	}
	return recs, last, false
}

// syncImageInvalidations применяет к кэшу холста сбросы, заказанные после
// последнего обращения. Дешёвый выход — одно атомарное чтение.
func (c *Canvas) syncImageInvalidations() {
	if c.inv == nil || c.inv.seq.Load() == c.invSeen {
		return
	}
	recs, last, lost := c.inv.since(c.invSeen)
	c.invSeen = last
	if lost {
		c.dropScaled(0, true)
		return
	}
	for _, r := range recs {
		c.dropScaled(r.ptr, r.all)
	}
}

// InvalidateImageCache сбрасывает кэш масштабированных изображений
// (DrawImageScaled, DrawImageRect) для src; nil сбрасывает кэш целиком.
//
// Вызывать после того, как пиксели картинки переписаны на месте: кэш ключует
// источник по идентичности, содержимого он не сравнивает. Новая картинка
// (другой объект) сброса не требует.
//
// Безопасно с любой горутины: сброс записывается в журнал и применяется всеми
// холстами движка — основным и попапов — перед их следующим кадром. Сам по
// себе кадр сброс не заказывает; если картинка уже на экране, вызовите
// Invalidate (или InvalidateRect области картинки).
func (e *Engine) InvalidateImageCache(src image.Image) {
	if src == nil {
		e.imgInv.add(0, true)
		return
	}
	v := reflect.ValueOf(src)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return // такие источники в кэш не попадают
	}
	e.imgInv.add(v.Pointer(), false)
}
