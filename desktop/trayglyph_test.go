package desktop

// Тесты значков трея из набора иконок темы, обобщённого значка, кнопки центра
// уведомлений и полоски «Показать рабочий стол».

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// hiCtx — записывающий контекст с HiDPI-масштабом и записью картинок.
type hiCtx struct {
	recCtx
	k      float64
	images []imgRec
}

type imgRec struct {
	img        image.Image
	x, y, w, h int
}

func (c *hiCtx) DrawImageScaled(src image.Image, x, y, w, h int) {
	c.images = append(c.images, imgRec{src, x, y, w, h})
}
func (c *hiCtx) Scale() float64 { return c.k }

const solidSVG = `<svg viewBox="0 0 24 24"><rect width="24" height="24" fill="#fff"/></svg>`

// themeWithIcons — тема из testThemeManager плюс набор иконок и привязки
// ключей трея к зарегистрированным именам.
func themeWithIcons(t *testing.T, bind map[theme.Key]string, extra func(p *theme.Profile)) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	p := theme.NewProfile("Icons")
	for _, c := range []string{ComponentNetwork, ComponentVolume, ComponentPower, ComponentTrayIcon, ComponentTrayNotifications, ComponentTrayShowDesktop} {
		p.SetStyle(c, "", theme.StateNormal, theme.StyleDelta{
			Text: theme.C(theme.RGB(240, 240, 240)),
			PadX: theme.N(2), PadY: theme.N(2),
		})
	}
	p.SetMetric(KeyTrayIconSize, 16)
	set := widget.NewIconSet("")
	for k, name := range bind {
		set.Register(name, []byte(solidSVG))
		p.Icons[k] = theme.IconRef{Name: name}
	}
	if extra != nil {
		extra(p)
	}
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Icons"); err != nil {
		t.Fatal(err)
	}
	m.SetIconResolver(set)
	return m
}

// Тема с иконкой звука: значок берётся из набора, а не рисуется фигурами.
func TestVolumeItem_UsesThemeIcon(t *testing.T) {
	tm := themeWithIcons(t, map[theme.Key]string{"tray.volume.icon": "vol"}, nil)
	st := NewFakeSystemStatus()
	it := NewVolumeStatus(tm, st)
	defer it.Close()
	it.SetBounds(image.Rect(0, 0, 20, 16))

	ctx := &hiCtx{k: 1}
	it.Draw(ctx)
	if len(ctx.images) != 1 {
		t.Fatalf("иконка темы не нарисована: картинок %d", len(ctx.images))
	}
	// Шкала громкости фигурами не рисуется: только подложка (если есть).
	for _, f := range ctx.fills {
		if f.w == 4 && f.h > 0 { // полоска шкалы
			t.Errorf("рядом с иконкой темы нарисованы фигуры: %+v", f)
		}
	}
}

// Без иконки в теме значки рисуются прежними фигурами (запасной вариант).
func TestTrayItems_FallBackToShapes(t *testing.T) {
	tm := testThemeManager(t)
	st := NewFakeSystemStatus()
	for name, it := range map[string]Item{
		"network": NewNetworkStatus(tm, st),
		"volume":  NewVolumeStatus(tm, st),
		"power":   NewPowerStatus(tm, st),
	} {
		it.SetBounds(image.Rect(0, 0, 20, 16))
		ctx := &hiCtx{k: 1}
		it.Draw(ctx)
		if len(ctx.images) != 0 {
			t.Errorf("%s: без иконки в теме нарисована картинка", name)
		}
		if len(ctx.fills) < 2 {
			t.Errorf("%s: фигуры не нарисованы (%d заливок)", name, len(ctx.fills))
		}
	}
}

// Иконка запрашивается в физическом размере: на 150 % квадрат 12 px — 18 px.
func TestTrayGlyph_RequestedInPhysicalSize(t *testing.T) {
	tm := themeWithIcons(t, map[theme.Key]string{"tray.network.icon": "net"}, nil)
	st := NewFakeSystemStatus()
	it := NewNetworkStatus(tm, st)
	defer it.Close()
	it.SetBounds(image.Rect(0, 0, 20, 16))

	for _, tc := range []struct {
		k           float64
		wantPhys    int
		wantLogical int
	}{{1, 12, 12}, {1.5, 18, 12}, {2, 24, 12}} {
		ctx := &hiCtx{k: tc.k}
		it.Draw(ctx)
		if len(ctx.images) != 1 {
			t.Fatalf("k=%v: картинок %d", tc.k, len(ctx.images))
		}
		r := ctx.images[0]
		if r.img.Bounds().Dx() != tc.wantPhys {
			t.Errorf("k=%v: иконка %d px, нужен физический размер %d", tc.k, r.img.Bounds().Dx(), tc.wantPhys)
		}
		if r.w != tc.wantLogical || r.h != tc.wantLogical {
			t.Errorf("k=%v: логический размер %dx%d, ожидалось %d", tc.k, r.w, r.h, tc.wantLogical)
		}
	}
}

// Ключи иконок идут от конкретного к общему и зависят от состояния.
func TestTrayIconKeys(t *testing.T) {
	has := func(keys []theme.Key, want theme.Key) bool {
		for _, k := range keys {
			if k == want {
				return true
			}
		}
		return false
	}
	if k := networkIconKeys(NetState{Kind: NetWiFi, Quality: 1}); k[0] != "tray.network.wifi.4" || !has(k, KeyTrayNetworkIcon) {
		t.Errorf("wifi: %v", k)
	}
	if k := networkIconKeys(NetState{Kind: NetNone}); len(k) != 1 || k[0] != "tray.network.none" {
		t.Errorf("none: %v", k)
	}
	if k := volumeIconKeys(VolState{Muted: true, Level: 0.5}); k[0] != "tray.volume.muted" {
		t.Errorf("muted: %v", k)
	}
	if k := volumeIconKeys(VolState{Level: 0}); k[0] != "tray.volume.0" {
		t.Errorf("0: %v", k)
	}
	if k := volumeIconKeys(VolState{Level: 1}); k[0] != "tray.volume.3" {
		t.Errorf("full: %v", k)
	}
	if k := volumeIconKeys(VolState{Level: 0.4}); k[0] != "tray.volume.2" {
		t.Errorf("0.4: %v", k)
	}
	if k := powerIconKeys(PowerState{Charge: 0.55, OnAC: true}); k[0] != "tray.power.ac.6" || !has(k, KeyTrayPowerIcon) {
		t.Errorf("ac: %v", k)
	}
	if k := powerIconKeys(PowerState{Charge: 1}); k[0] != "tray.power.10" {
		t.Errorf("full: %v", k)
	}
}

// Самый конкретный ключ побеждает общий.
func TestTrayGlyph_SpecificKeyWins(t *testing.T) {
	tm := themeWithIcons(t, map[theme.Key]string{
		"tray.volume.muted": "muted",
		"tray.volume.icon":  "generic",
	}, nil)
	if themeGlyph(tm, volumeIconKeys(VolState{Muted: true}), 16) == nil {
		t.Fatal("иконка muted не найдена")
	}
	// Для небезмолвного состояния отдаётся общая.
	if themeGlyph(tm, volumeIconKeys(VolState{Level: 0.5}), 16) == nil {
		t.Fatal("общая иконка не найдена")
	}
	// Для ключа без привязки — nil, а не заглушка.
	if themeGlyph(tm, []theme.Key{"tray.nothing"}, 16) != nil {
		t.Fatal("для необъявленного ключа вернулась иконка")
	}
}

// Перекраска по флагу темы: цвет значка — цвет текста, альфа сохранена.
func TestTrayGlyph_Tint(t *testing.T) {
	tm := themeWithIcons(t, map[theme.Key]string{"tray.volume.icon": "vol"}, func(p *theme.Profile) {
		p.SetFlag(KeyTrayIconTint, true)
	})
	it := NewVolumeStatus(tm, NewFakeSystemStatus())
	defer it.Close()
	it.SetBounds(image.Rect(0, 0, 20, 16))
	ctx := &hiCtx{k: 1}
	it.Draw(ctx)
	if len(ctx.images) != 1 {
		t.Fatal("иконка не нарисована")
	}
	got := ctx.images[0].img.(*image.RGBA).RGBAAt(5, 5)
	if got != (color.RGBA{R: 240, G: 240, B: 240, A: 255}) {
		t.Errorf("пиксель после перекраски %v, ожидался цвет текста", got)
	}
}

// ─── Обобщённый значок ──────────────────────────────────────────────────────

func TestTrayIcon_SVGRasterizedPhysical(t *testing.T) {
	tm := themeWithIcons(t, nil, nil)
	ic, err := NewTraySVGIcon(tm, []byte(solidSVG))
	if err != nil {
		t.Fatal(err)
	}
	ic.SetBounds(image.Rect(0, 0, 20, 16))
	for _, tc := range []struct {
		k    float64
		phys int
	}{{1, 12}, {1.25, 15}, {1.5, 18}, {2, 24}} {
		ctx := &hiCtx{k: tc.k}
		ic.Draw(ctx)
		if len(ctx.images) != 1 {
			t.Fatalf("k=%v: картинок %d", tc.k, len(ctx.images))
		}
		if got := ctx.images[0].img.Bounds().Dx(); got != tc.phys {
			t.Errorf("k=%v: растр %d px, ожидался %d (физический размер)", tc.k, got, tc.phys)
		}
	}
}

func TestTrayIcon_BadSVGKeepsPrevious(t *testing.T) {
	tm := themeWithIcons(t, nil, nil)
	ic, _ := NewTraySVGIcon(tm, []byte(solidSVG))
	if err := ic.SetSVG([]byte("не svg")); err == nil {
		t.Fatal("мусор принят как SVG")
	}
	ic.SetBounds(image.Rect(0, 0, 20, 16))
	ctx := &hiCtx{k: 1}
	ic.Draw(ctx)
	if len(ctx.images) != 1 {
		t.Error("прежний значок потерян после неудачной загрузки")
	}
}

func TestTrayIcon_ImageSourceGetsPhysicalSize(t *testing.T) {
	tm := themeWithIcons(t, nil, nil)
	ic := NewTrayIcon(tm)
	var asked []int
	ic.SetImageSource(func(size int) image.Image {
		asked = append(asked, size)
		return image.NewRGBA(image.Rect(0, 0, size, size))
	})
	ic.SetBounds(image.Rect(0, 0, 20, 16))
	ic.Draw(&hiCtx{k: 1.5})
	if len(asked) != 1 || asked[0] != 18 {
		t.Errorf("источник спросили о размерах %v, ожидался [18]", asked)
	}
}

func TestTrayIcon_ClickAndTooltip(t *testing.T) {
	tm := themeWithIcons(t, nil, nil)
	ic := NewTrayImageIcon(tm, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	ic.SetBounds(image.Rect(0, 0, 20, 16))
	clicks := 0
	ic.OnClick = func() { clicks++ }
	ic.OnMouseButton(widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: true})
	ic.OnMouseButton(widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: false})
	if clicks != 1 {
		t.Errorf("кликов %d, ожидался 1", clicks)
	}
	// Отпускание мимо — клика нет.
	ic.OnMouseButton(widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft, Pressed: true})
	ic.OnMouseButton(widget.MouseEvent{X: 50, Y: 50, Button: widget.MouseLeft, Pressed: false})
	if clicks != 1 {
		t.Errorf("клик засчитан при отпускании мимо: %d", clicks)
	}

	ic.SetToolTip("Облако")
	if ic.GetToolTip() != "Облако" {
		t.Errorf("подсказка %q", ic.GetToolTip())
	}
	widget.RegisterStrings("EN", map[string]string{"test.cloud": "Cloud"})
	widget.RegisterStrings("RU", map[string]string{"test.cloud": "Облако (рус)"})
	ic.SetToolTipKey("test.cloud")
	prev := widget.Language()
	defer widget.SetLanguage(prev)
	widget.SetLanguage("EN")
	if ic.GetToolTip() != "Cloud" {
		t.Errorf("EN: %q", ic.GetToolTip())
	}
	widget.SetLanguage("RU")
	if ic.GetToolTip() != "Облако (рус)" {
		t.Errorf("RU: %q — подсказка не следует за языком", ic.GetToolTip())
	}
}

// ─── Кнопка центра уведомлений ──────────────────────────────────────────────

func TestNotificationButton_CountAndTooltip(t *testing.T) {
	tm := themeWithIcons(t, nil, nil)
	notes := NewFakeNotifications()
	b := NewNotificationButton(tm, notes)
	defer b.Close()
	b.SetBounds(image.Rect(0, 0, 20, 16))

	prev := widget.Language()
	defer widget.SetLanguage(prev)
	widget.SetLanguage("EN")

	if got := b.GetToolTip(); got != "No new notifications" {
		t.Errorf("пусто: %q", got)
	}
	ctx := &hiCtx{k: 1}
	b.Draw(ctx)
	if len(ctx.texts) != 0 {
		t.Errorf("при нуле нарисован счётчик: %v", ctx.texts)
	}

	notes.Add(Notification{Title: "a"})
	notes.Add(Notification{Title: "b"})
	if b.Count() != 2 {
		t.Fatalf("счётчик %d, ожидалось 2", b.Count())
	}
	if got := b.GetToolTip(); got != "2 new notifications" {
		t.Errorf("EN: %q", got)
	}
	widget.SetLanguage("RU")
	if got := b.GetToolTip(); got != "2 новых уведомления" {
		t.Errorf("RU: %q", got)
	}

	ctx = &hiCtx{k: 1}
	b.Draw(ctx)
	if !containsText(ctx.texts, "2") {
		t.Errorf("счётчик не нарисован: %v", ctx.texts)
	}
}

func TestNotificationButton_BigCountAndManual(t *testing.T) {
	tm := themeWithIcons(t, nil, nil)
	b := NewNotificationButton(tm, nil)
	b.SetBounds(image.Rect(0, 0, 20, 16))
	b.SetCount(250)
	ctx := &hiCtx{k: 1}
	b.Draw(ctx)
	if !containsText(ctx.texts, "99+") {
		t.Errorf("большое число не сжато до 99+: %v", ctx.texts)
	}
	// Цифры влезают в значок: кегль ужат до ширины квадрата.
	for _, tx := range ctx.texts {
		if w := ctx.MeasureText(tx.text, tx.size); w > 12 && tx.size > minBadgePt {
			t.Errorf("число %q шириной %d не влезло в значок 12 px при кегле %v", tx.text, w, tx.size)
		}
	}
	b.SetCount(0)
	ctx = &hiCtx{k: 1}
	b.Draw(ctx)
	if len(ctx.texts) != 0 {
		t.Error("после обнуления счётчик остался")
	}
}

func TestNotificationButton_ThemeIconNewState(t *testing.T) {
	tm := themeWithIcons(t, map[theme.Key]string{
		"tray.notifications.icon":     "bell",
		"tray.notifications.icon.new": "bell-new",
	}, nil)
	b := NewNotificationButton(tm, nil)
	b.SetBounds(image.Rect(0, 0, 20, 16))
	ctx := &hiCtx{k: 1}
	b.Draw(ctx)
	if len(ctx.images) != 1 {
		t.Fatalf("иконка темы не нарисована: %d", len(ctx.images))
	}
	b.SetCount(3)
	ctx = &hiCtx{k: 1}
	b.Draw(ctx)
	if len(ctx.images) != 1 || !containsText(ctx.texts, "3") {
		t.Errorf("иконка/счётчик при ненулевом числе: %d картинок, %v", len(ctx.images), ctx.texts)
	}
}

func TestNotificationButton_UnsubscribesOnClose(t *testing.T) {
	notes := NewFakeNotifications()
	b := NewNotificationButton(nil, notes)
	b.Close()
	b.Close() // повторный вызов безопасен
	notes.mu.Lock()
	n := len(notes.subs)
	notes.mu.Unlock()
	if n != 0 {
		t.Errorf("подписок после Close: %d", n)
	}
}

// ─── Полоска «Показать рабочий стол» ────────────────────────────────────────

func TestShowDesktopButton_WidthFromMetric(t *testing.T) {
	tm := themeWithIcons(t, nil, func(p *theme.Profile) {
		p.SetMetric(KeyTrayShowDesktopWidth, 7)
	})
	b := NewShowDesktopButton(tm)
	sz := b.PreferredSize(image.Pt(500, 40))
	if sz != image.Pt(7, 40) {
		t.Errorf("размер %v, ожидалось 7×40", sz)
	}
}

func TestShowDesktopButton_WidthFallsBackToIconSize(t *testing.T) {
	tm := themeWithIcons(t, nil, nil) // метрики нет, размер значка 16
	b := NewShowDesktopButton(tm)
	if got := b.PreferredSize(image.Pt(500, 40)).X; got != 4 {
		t.Errorf("запасная ширина %d, ожидалась четверть значка (4)", got)
	}
	if got := NewShowDesktopButton(nil).PreferredSize(image.Pt(500, 40)).X; got < 1 {
		t.Errorf("без темы ширина %d — полоска пропала", got)
	}
}

func TestShowDesktopButton_ClickAndTooltip(t *testing.T) {
	tm := themeWithIcons(t, nil, nil)
	b := NewShowDesktopButton(tm)
	b.SetBounds(image.Rect(100, 0, 104, 40))
	clicks := 0
	b.OnClick = func() { clicks++ }
	b.OnMouseButton(widget.MouseEvent{X: 101, Y: 20, Button: widget.MouseLeft, Pressed: true})
	b.OnMouseButton(widget.MouseEvent{X: 101, Y: 20, Button: widget.MouseLeft, Pressed: false})
	if clicks != 1 {
		t.Errorf("кликов %d", clicks)
	}
	prev := widget.Language()
	defer widget.SetLanguage(prev)
	widget.SetLanguage("EN")
	if got := b.GetToolTip(); got != "Show desktop" {
		t.Errorf("EN: %q", got)
	}
	widget.SetLanguage("RU")
	if got := b.GetToolTip(); got != "Показать рабочий стол" {
		t.Errorf("RU: %q", got)
	}
}

func TestShowDesktopButton_HoverPaintsFill(t *testing.T) {
	tm := themeWithIcons(t, nil, func(p *theme.Profile) {
		p.SetStyle(ComponentTrayShowDesktop, "", theme.StateHover, theme.StyleDelta{Fill: theme.C(theme.RGB(90, 90, 90))})
	})
	b := NewShowDesktopButton(tm)
	b.SetBounds(image.Rect(0, 0, 4, 40))
	idle := &hiCtx{k: 1}
	b.Draw(idle)
	b.OnMouseMove(1, 10)
	hot := &hiCtx{k: 1}
	b.Draw(hot)
	if len(hot.fills) <= len(idle.fills) {
		t.Errorf("наведение не добавило подсветки: %d → %d заливок", len(idle.fills), len(hot.fills))
	}
}

// Встроенные профили: новые компоненты трея унаследовали стили значков
// состояния — иначе подложкой им досталась бы заливка общей поверхности.
func TestBuiltinProfiles_TrayExtrasInheritStyles(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows10, theme.ProfileWindows11Dark, theme.ProfileMacOS} {
		if err := m.SetTheme(name); err != nil {
			t.Fatal(err)
		}
		for _, st := range []theme.State{theme.StateNormal, theme.StateHover} {
			want := m.GetStyle(ComponentVolume, "", st)
			for _, c := range []string{ComponentTrayIcon, ComponentTrayNotifications, ComponentTrayShowDesktop} {
				got := m.GetStyle(c, "", st)
				if got.Fill != want.Fill || got.Text != want.Text {
					t.Errorf("%s/%s/%v: заливка %v текст %v, как у звука — %v/%v",
						name, c, st, got.Fill, got.Text, want.Fill, want.Text)
				}
			}
		}
	}
}

// ─── Значок приложения по размеру ───────────────────────────────────────────

func TestAppInfo_IconFor(t *testing.T) {
	one := image.NewRGBA(image.Rect(0, 0, 64, 64))
	a := AppInfo{Icon: one}
	if a.IconFor(24) != image.Image(one) {
		t.Error("без IconAt должен отдаваться Icon")
	}
	var asked int
	a.IconAt = func(size int) image.Image {
		asked = size
		return image.NewRGBA(image.Rect(0, 0, size, size))
	}
	if got := a.IconFor(36); got.Bounds().Dx() != 36 || asked != 36 {
		t.Errorf("IconAt не вызван с 36: %v %d", got.Bounds(), asked)
	}
	a.IconAt = func(int) image.Image { return nil }
	if a.IconFor(36) != image.Image(one) {
		t.Error("пустой ответ IconAt должен откатываться к Icon")
	}
}

func TestDrawAppIcon_UsesPhysicalSize(t *testing.T) {
	var asked []int
	at := func(size int) image.Image {
		asked = append(asked, size)
		return image.NewRGBA(image.Rect(0, 0, size, size))
	}
	ctx := &hiCtx{k: 1.5}
	drawAppIcon(ctx, image.NewRGBA(image.Rect(0, 0, 64, 64)), at, image.Rect(10, 10, 34, 34))
	if len(asked) != 1 || asked[0] != 36 {
		t.Errorf("спросили %v, ожидалось [36]", asked)
	}
	if len(ctx.images) != 1 || ctx.images[0].w != 24 || ctx.images[0].img.Bounds().Dx() != 36 {
		t.Errorf("нарисовано %+v", ctx.images)
	}
	// Без IconAt — единственная картинка.
	ctx = &hiCtx{k: 1.5}
	drawAppIcon(ctx, image.NewRGBA(image.Rect(0, 0, 64, 64)), nil, image.Rect(0, 0, 24, 24))
	if len(ctx.images) != 1 || ctx.images[0].img.Bounds().Dx() != 64 {
		t.Errorf("запасной значок: %+v", ctx.images)
	}
}
