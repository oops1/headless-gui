package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// styleNative — бэкенд со всеми возможностями style.go. can — что он умеет из
// Borderless и Tool; ownerOK — принимает ли владельца.
type styleNative struct {
	native.NativeWindow
	can     native.Style
	ownerOK bool

	applied []native.Style
	owners  []native.NativeWindow
	raised  int
	lowered int
	x, y    int
	moved   bool
	closes  int
}

func (s *styleNative) ApplyStyle(st native.Style) native.Style {
	s.applied = append(s.applied, st)
	return native.Style{Borderless: st.Borderless && s.can.Borderless, Tool: st.Tool && s.can.Tool}
}

func (s *styleNative) SetOwnerWindow(o native.NativeWindow) bool {
	if !s.ownerOK {
		return false
	}
	s.owners = append(s.owners, o)
	return true
}

func (s *styleNative) RaiseWindow() bool { s.raised++; return true }
func (s *styleNative) LowerWindow() bool { s.lowered++; return true }

func (s *styleNative) MoveTo(x, y int) bool { s.x, s.y, s.moved = x, y, true; return true }

func (s *styleNative) ScreenPosition() (int, int, bool) { return s.x, s.y, s.moved }

func (s *styleNative) Close() { s.closes++ }

func newStyleNative() *styleNative {
	return &styleNative{can: native.Style{Borderless: true, Tool: true}, ownerOK: true}
}

// Стиль до Run запоминается, applyPendingStyle отдаёт его бэкенду; Style
// после — то, что бэкенд применил.
func TestStyle_ApplyPendingStyle(t *testing.T) {
	if !styleBeforeRun() {
		t.Skip("на macOS стиль до Run не запоминается (TestStyle_DarwinBeforeRun)")
	}
	win := &Window{}
	want := native.Style{Borderless: true, Tool: true}
	if !win.SetStyle(want) {
		t.Fatal("SetStyle до Run = false")
	}
	if got := win.Style(); got != want {
		t.Fatalf("Style до Run = %+v", got)
	}
	be := newStyleNative()
	be.can.Tool = false
	win.native = be
	win.applyPendingStyle()
	if len(be.applied) != 1 || be.applied[0] != want {
		t.Fatalf("бэкенду ушло %v", be.applied)
	}
	if got := win.Style(); got != (native.Style{Borderless: true}) {
		t.Errorf("Style после создания = %+v, ждал то, что бэкенд применил", got)
	}
}

// Нулевой стиль до Run бэкенду не отдаётся: обычное окно создаётся как прежде.
func TestStyle_ZeroBeforeRunNotSent(t *testing.T) {
	win := &Window{}
	if !win.SetStyle(native.Style{}) {
		t.Fatal("нулевой стиль до Run = false")
	}
	be := newStyleNative()
	win.native = be
	win.applyPendingStyle()
	if len(be.applied) != 0 || len(be.owners) != 0 {
		t.Fatalf("бэкенду ушло: стили %v, владельцы %v", be.applied, be.owners)
	}
}

// После Run ответ — по тому, что бэкенд применил целиком.
func TestStyle_AfterRunHonest(t *testing.T) {
	be := newStyleNative()
	be.can.Tool = false
	win := &Window{}
	win.native = be
	if win.SetStyle(native.Style{Borderless: true, Tool: true}) {
		t.Error("SetStyle = true, хотя Tool бэкенд не применил")
	}
	if got := win.Style(); got != (native.Style{Borderless: true}) {
		t.Errorf("Style = %+v", got)
	}
	if !win.SetStyle(native.Style{Borderless: true}) {
		t.Error("выполнимый стиль = false")
	}
	plain := &Window{}
	plain.native = struct{ native.NativeWindow }{}
	if plain.SetStyle(native.Style{Borderless: true}) || plain.Style() != (native.Style{}) {
		t.Error("бэкенд без стиля ответил true")
	}
	if !plain.SetStyle(native.Style{}) {
		t.Error("нулевой стиль у бэкенда без стиля = false")
	}
}

// Owned: true без владельца невыполнимо, false снимает владельца.
func TestStyle_OwnedFollowsOwner(t *testing.T) {
	be := newStyleNative()
	win := &Window{}
	win.native = be
	if win.SetStyle(native.Style{Owned: true}) {
		t.Error("Owned=true без владельца = true")
	}
	owner := &Window{}
	owner.native = newStyleNative()
	if !win.SetOwner(owner) {
		t.Fatal("SetOwner = false")
	}
	if !win.Style().Owned {
		t.Fatal("Style.Owned = false при владельце")
	}
	if !win.SetStyle(native.Style{Owned: true, Borderless: true}) {
		t.Error("Owned=true при владельце = false")
	}
	if !win.SetStyle(native.Style{}) {
		t.Fatal("SetStyle(Owned=false) = false")
	}
	if win.Style().Owned || len(be.owners) != 2 || be.owners[1] != nil {
		t.Errorf("владелец не снят: Owned %v, вызовы %v", win.Style().Owned, be.owners)
	}
}

// Окно не может принадлежать себе или своему потомку по владению.
func TestSetOwner_RejectsCycle(t *testing.T) {
	a, b, c := &Window{}, &Window{}, &Window{}
	for _, w := range []*Window{a, b, c} {
		w.native = newStyleNative()
	}
	if a.SetOwner(a) {
		t.Error("окно приняло себя владельцем")
	}
	if !b.SetOwner(a) || !c.SetOwner(b) {
		t.Fatal("цепочка a ← b ← c не построилась")
	}
	if a.SetOwner(c) {
		t.Error("a приняло владельцем своего потомка c")
	}
}

// Владелец до Run: applyPendingStyle отдаёт его бэкенду; владелец, которого к
// этому времени нет, отбрасывается.
func TestSetOwner_ApplyPendingStyle(t *testing.T) {
	if !styleBeforeRun() {
		t.Skip("на macOS владелец до Run не запоминается")
	}
	owner := &Window{}
	ownerNative := newStyleNative()
	owner.native = ownerNative
	win := &Window{}
	if !win.SetOwner(owner) {
		t.Fatal("SetOwner до Run = false")
	}
	be := newStyleNative()
	win.native = be
	win.applyPendingStyle()
	if len(be.owners) != 1 || be.owners[0] != ownerNative || !win.Style().Owned {
		t.Fatalf("владелец до Run: вызовы %v, Owned %v", be.owners, win.Style().Owned)
	}

	late := &Window{}
	late.SetOwner(&Window{}) // владелец ещё не показан
	lateBe := newStyleNative()
	late.native = lateBe
	late.applyPendingStyle()
	if len(lateBe.owners) != 0 || late.Style().Owned {
		t.Errorf("непоказанный владелец принят: %v, Owned %v", lateBe.owners, late.Style().Owned)
	}
}

// Бэкенд без владельца (Wayland, macOS): владельца не принимает, снимать нечего.
func TestSetOwner_NoBackendSupport(t *testing.T) {
	win := &Window{}
	win.native = struct{ native.NativeWindow }{}
	owner := &Window{}
	owner.native = newStyleNative()
	if win.SetOwner(owner) || win.Style().Owned {
		t.Error("бэкенд без владельца принял его")
	}
	if !win.SetOwner(nil) {
		t.Error("SetOwner(nil) без владельца = false")
	}

	refusing := &Window{}
	be := newStyleNative()
	be.ownerOK = false
	refusing.native = be
	if refusing.SetOwner(owner) || refusing.Style().Owned {
		t.Error("отказ бэкенда не дошёл до ответа")
	}
	unshown := &Window{}
	unshown.native = newStyleNative()
	if unshown.SetOwner(&Window{}) {
		t.Error("принят владелец, который ещё не показан")
	}
}

// Raise, Lower, SetPosition и Position — к бэкенду; без него и до Run — false.
func TestStackAndPosition(t *testing.T) {
	be := newStyleNative()
	win := &Window{}
	win.native = be
	if !win.Raise() || !win.Lower() || be.raised != 1 || be.lowered != 1 {
		t.Errorf("Raise и Lower: %d, %d", be.raised, be.lowered)
	}
	if !win.SetPosition(40, 30) {
		t.Fatal("SetPosition = false")
	}
	if x, y, ok := win.Position(); !ok || x != 40 || y != 30 {
		t.Errorf("Position = %d, %d, %v", x, y, ok)
	}
	for _, w := range []*Window{{}, {surface: surface{native: struct{ native.NativeWindow }{}}}} {
		if w.Raise() || w.Lower() || w.SetPosition(1, 1) {
			t.Error("Raise, Lower или SetPosition без возможности = true")
		}
		if _, _, ok := w.Position(); ok {
			t.Error("Position без возможности ok")
		}
	}
}

// styleInitFake — подставной бэкенд bringUp со стилем, владельцем и
// системным ресайзом: пишет в общий журнал initFake, чтобы был виден порядок
// относительно Create.
type styleInitFake struct {
	*initFake
	noTool bool
}

func (f styleInitFake) ApplyStyle(s native.Style) native.Style {
	f.log = append(f.log, "style")
	if f.noTool {
		s.Tool = false
	}
	return native.Style{Borderless: s.Borderless, Tool: s.Tool}
}

func (f styleInitFake) SetOwnerWindow(o native.NativeWindow) bool {
	if o == nil {
		f.log = append(f.log, "owner-nil")
	} else {
		f.log = append(f.log, "owner")
	}
	return true
}

func (f styleInitFake) BeginMove() bool      { return true }
func (f styleInitFake) BeginResize(int) bool { return true }

// Стиль и владелец, заказанные до Run, доходят до бэкенда в bringUp ДО Create;
// Style после — ответ бэкенда; у Borderless-окна нет зон ресайза корпуса.
func TestStyle_BeforeRunReachesBackendBeforeCreate(t *testing.T) {
	if !styleBeforeRun() {
		t.Skip("на macOS стиль до Run не запоминается")
	}
	for _, noTool := range []bool{false, true} {
		log := &initFake{}
		fake := styleInitFake{initFake: log, noTool: noTool}
		old := newNativeWindow
		newNativeWindow = func() native.NativeWindow { return fake }
		owner := &Window{}
		owner.native = newStyleNative()
		eng := engine.New(800, 600, 30)
		ww := widget.NewWindow("x", 800, 600)
		eng.SetRoot(ww)
		win := New(eng, "style")
		win.SetStyle(native.Style{Borderless: true, Tool: true})
		win.SetOwner(owner)
		err := win.bringUp()
		newNativeWindow = old
		eng.Stop()
		if err != nil {
			t.Fatalf("bringUp: %v", err)
		}
		iStyle, iOwner, iCreate := -1, -1, -1
		for i, e := range log.log {
			switch e {
			case "style":
				iStyle = i
			case "owner":
				iOwner = i
			case "create":
				iCreate = i
			}
		}
		if iStyle < 0 || iOwner < 0 || iCreate < 0 || iStyle > iCreate || iOwner > iCreate {
			t.Fatalf("порядок вызовов %v, ждал style и owner перед create", log.log)
		}
		want := native.Style{Borderless: true, Tool: !noTool, Owned: true}
		if got := win.Style(); got != want {
			t.Errorf("noTool=%v: Style после bringUp = %+v, ждал %+v", noTool, got, want)
		}
		if ww.OnNativeResize != nil {
			t.Error("у Borderless-окна корпус получил зоны ресайза (OnNativeResize)")
		}
		if ww.OnNativeMove == nil {
			t.Error("Borderless-окно потеряло перемещение силами системы")
		}
	}
}

// edgeNative — бэкенд со стилем, системным ресайзом и скруглением, которое
// зависит от Borderless (как у Win32 и Wayland).
type edgeNative struct {
	*styleNative
	borderless bool
}

func (e *edgeNative) ApplyStyle(s native.Style) native.Style {
	e.borderless = s.Borderless
	return e.styleNative.ApplyStyle(s)
}
func (e *edgeNative) BeginMove() bool      { return true }
func (e *edgeNative) BeginResize(int) bool { return true }
func (e *edgeNative) RoundsCorners() bool  { return !e.borderless }

// SetStyle после запуска переводит корпус виджетного окна: Borderless — без
// зон ресайза и с прямыми углами, обратно — как было.
func TestStyle_LiveSyncsWidgetWindow(t *testing.T) {
	eng := engine.New(200, 100, 30)
	defer eng.Stop()
	ww := widget.NewWindow("x", 200, 100)
	ww.CornerRadius = 8
	eng.SetRoot(ww)
	win := &Window{}
	win.eng = eng
	win.native = &edgeNative{styleNative: newStyleNative()}
	win.syncEdgeResize()
	win.syncSquareCorners()
	if ww.OnNativeResize == nil || ww.SquareCorners {
		t.Fatal("обычное окно: нет зон ресайза или прямые углы")
	}
	if !win.SetStyle(native.Style{Borderless: true}) {
		t.Fatal("SetStyle(Borderless) = false")
	}
	eng.Flush()
	if ww.OnNativeResize != nil {
		t.Error("Borderless: у корпуса остались зоны ресайза")
	}
	if !ww.SquareCorners {
		t.Error("Borderless: корпус не перешёл на прямые углы")
	}
	win.SetStyle(native.Style{})
	eng.Flush()
	if ww.OnNativeResize == nil || ww.SquareCorners {
		t.Error("обычный стиль: зоны ресайза или скругление не вернулись")
	}
}

// У закрытого окна ни стиль, ни владелец, ни Z-порядок, ни положение не
// меняются, и владельцем оно быть не может.
func TestStyle_ClosedWindowRefuses(t *testing.T) {
	be := newStyleNative()
	win := &Window{}
	win.native = be
	win.closeRequested.Store(true)
	owner := &Window{}
	owner.native = newStyleNative()
	if win.SetStyle(native.Style{Borderless: true}) || win.SetOwner(owner) || win.Raise() ||
		win.Lower() || win.SetPosition(1, 1) {
		t.Error("закрытое окно ответило true")
	}
	if _, _, ok := win.Position(); ok {
		t.Error("Position закрытого окна ok")
	}
	if len(be.applied) != 0 || len(be.owners) != 0 || be.raised != 0 || be.lowered != 0 || be.moved {
		t.Errorf("закрытому окну ушли вызовы: %+v", be)
	}
	open := &Window{}
	open.native = newStyleNative()
	if open.SetOwner(win) {
		t.Error("закрытое окно принято владельцем")
	}
}

// На macOS бэкенд ни стиля, ни владельца не умеет: до Run заказ не
// запоминается, и Style не показывает невыполненного.
func TestStyle_DarwinBeforeRun(t *testing.T) {
	if styleBeforeRun() {
		t.Skip("не macOS")
	}
	win := &Window{}
	if win.SetStyle(native.Style{Borderless: true}) || win.Style() != (native.Style{}) {
		t.Errorf("стиль до Run на macOS: Style = %+v", win.Style())
	}
	owner := &Window{}
	if win.SetOwner(owner) || win.Style().Owned {
		t.Error("владелец до Run на macOS принят")
	}
	if !win.SetStyle(native.Style{}) || !win.SetOwner(nil) {
		t.Error("нулевой стиль или снятие владельца на macOS = false")
	}
}

// Закрытие владельца освобождает окно (Close): оно остаётся открытым, без
// владельца, и бэкенд снимает владельца до закрытия окна ОС.
func TestSetOwner_OwnerCloseReleases(t *testing.T) {
	ownerBe := newStyleNative()
	owner := &Window{}
	owner.native = ownerBe
	be := newStyleNative()
	win := &Window{}
	win.native = be
	win.style.live = true // окно ОС создано (как после bringUp)
	if !win.SetOwner(owner) {
		t.Fatal("SetOwner = false")
	}
	owner.Close()
	if len(be.owners) != 2 || be.owners[1] != nil {
		t.Fatalf("владелец не снят у бэкенда: %v", be.owners)
	}
	if win.Style().Owned || win.closeRequested.Load() || be.closes != 0 {
		t.Errorf("окно не освобождено: Owned %v, закрыто %v/%d", win.Style().Owned, win.closeRequested.Load(), be.closes)
	}
	if ownerBe.closes != 1 {
		t.Errorf("владелец закрыт %d раз", ownerBe.closes)
	}
	owner.style.mu.Lock()
	n := len(owner.style.owned)
	owner.style.mu.Unlock()
	if n != 0 {
		t.Errorf("закрытый владелец помнит %d окон", n)
	}
}

// Закрытие средствами ОС (без хука) тоже освобождает окна — до того, как окно
// ОС уничтожится.
func TestSetOwner_OSCloseReleases(t *testing.T) {
	owner, fn, _, _ := closeTestWindow(t)
	be := newStyleNative()
	win := &Window{}
	win.native = be
	win.style.live = true // окно ОС создано (как после bringUp)
	if !win.SetOwner(owner) {
		t.Fatal("SetOwner = false")
	}
	if !fn.onClose() {
		t.Fatal("окно ОС не отпущено")
	}
	if len(be.owners) != 2 || be.owners[1] != nil || win.Style().Owned {
		t.Errorf("окно не освобождено: вызовы %v, Owned %v", be.owners, win.Style().Owned)
	}
}

// Владелец, закрытый в обход Close (флаг уже стоит), владельцем не считается.
func TestStyle_ClosedOwnerNotOwned(t *testing.T) {
	owner := &Window{}
	owner.native = newStyleNative()
	win := &Window{}
	win.native = newStyleNative()
	win.SetOwner(owner)
	owner.closeRequested.Store(true)
	if win.Style().Owned {
		t.Error("Owned=true при закрытом владельце")
	}
	if win.SetStyle(native.Style{Owned: true}) {
		t.Error("SetStyle(Owned) при закрытом владельце = true")
	}
}

// Close до Run — no-op: операции окна после Run работают как обычно.
func TestStyle_CloseBeforeRunIsNoop(t *testing.T) {
	win := &Window{}
	win.Close()
	if win.closeRequested.Load() {
		t.Fatal("Close до Run пометил окно закрытым")
	}
	win.native = newStyleNative()
	if !win.Raise() || !win.SetPosition(1, 1) || !win.SetStyle(native.Style{Borderless: true}) {
		t.Error("окно, которому Close звали до Run, отвечает false")
	}
	other := &Window{}
	other.native = newStyleNative()
	if !other.SetOwner(win) {
		t.Error("окно, которому Close звали до Run, не принято владельцем")
	}
}

// closingCreateFake — бэкенд, во время Create которого происходит during
// (закрывается владелец).
type closingCreateFake struct {
	styleInitFake
	during func()
}

func (f closingCreateFake) Create(title string, w, h int) error {
	f.during()
	return f.initFake.Create(title, w, h)
}

// Владелец закрылся, пока окно создавалось: releaseOwned бэкенд не трогает
// (поток окна читает заказанного владельца), а после Create владелец
// снимается у живого окна (settleOwner).
func TestStyle_OwnerClosedDuringCreate(t *testing.T) {
	if !styleBeforeRun() {
		t.Skip("на macOS владелец до Run не запоминается")
	}
	owner := &Window{}
	owner.native = newStyleNative()
	log := &initFake{}
	fake := closingCreateFake{styleInitFake: styleInitFake{initFake: log}, during: owner.Close}
	old := newNativeWindow
	newNativeWindow = func() native.NativeWindow { return fake }
	t.Cleanup(func() { newNativeWindow = old })
	eng := engine.New(800, 600, 30)
	defer eng.Stop()
	win := New(eng, "style")
	if !win.SetOwner(owner) {
		t.Fatal("SetOwner до Run = false")
	}
	if err := win.bringUp(); err != nil {
		t.Fatalf("bringUp: %v", err)
	}
	iOwner, iCreate, iNil := -1, -1, -1
	for i, e := range log.log {
		switch {
		case e == "owner" && iOwner < 0:
			iOwner = i
		case e == "create":
			iCreate = i
		case e == "owner-nil" && iNil < 0:
			iNil = i
		}
	}
	if iOwner < 0 || iCreate < iOwner || iNil < iCreate {
		t.Fatalf("журнал %v: ждал owner, create, затем owner-nil (снятие у живого окна, не до Create)", log.log)
	}
	if win.Style().Owned {
		t.Error("Owned=true: владелец закрылся во время создания")
	}
}

// Владелец, закрытый до записи окна в его список (гонка SetOwner с Close
// владельца), окно не принимает.
func TestStyle_SetOwnerRefClosedOwner(t *testing.T) {
	owner := &Window{}
	owner.closeRequested.Store(true)
	win := &Window{}
	if win.setOwnerRef(owner) {
		t.Error("setOwnerRef принял закрытого владельца")
	}
	if win.style.owner != nil || len(owner.style.owned) != 0 {
		t.Errorf("окно осталось записанным: владелец %p, список %d", win.style.owner, len(owner.style.owned))
	}
}

// Окно переходит к новому владельцу (он в пути), а прежний в это время
// закрывается: его releaseOwned снимает ссылку, но бэкенд не трогает —
// снятие стёрло бы нового владельца.
func TestStyle_ReleaseKeepsOwnerInFlight(t *testing.T) {
	prevOwner := &Window{}
	prevOwner.native = newStyleNative()
	next := &Window{}
	be := newStyleNative()
	win := &Window{}
	win.native = be
	win.style.live = true
	if !win.SetOwner(prevOwner) {
		t.Fatal("SetOwner = false")
	}
	win.style.mu.Lock()
	win.style.moving = next // SetOwner(next) уже у бэкенда
	win.style.mu.Unlock()
	prevOwner.releaseOwned()
	if len(be.owners) != 1 {
		t.Errorf("releaseOwned прежнего владельца тронул бэкенд: %v", be.owners)
	}
	if !win.setOwnerRef(next) || !win.Style().Owned {
		t.Error("новый владелец не записался")
	}
}
