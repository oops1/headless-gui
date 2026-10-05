package desktop_test

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Снимки центра уведомлений Windows 10: тёмный и светлый, с группами,
// действиями и быстрыми действиями. Кадры сохраняются при NC_OUT (каталог) —
// смотреть глазами.

// ncGlyph рисует значок-заглушку: круг с буквой-меткой из цвета c на
// прозрачном фоне. Настоящие значки приносит потребитель.
func ncGlyph(side int, c color.RGBA, kind int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	cx, cy := float64(side)/2, float64(side)/2
	r := float64(side) / 2
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d2 := dx*dx + dy*dy
			switch kind {
			case 0: // диск
				if d2 <= r*r {
					img.SetRGBA(x, y, c)
				}
			case 1: // кольцо
				if d2 <= r*r && d2 >= (r*0.62)*(r*0.62) {
					img.SetRGBA(x, y, c)
				}
			case 2: // квадрат со скруглением
				if dx > -r*0.8 && dx < r*0.8 && dy > -r*0.8 && dy < r*0.8 {
					img.SetRGBA(x, y, c)
				}
			}
		}
	}
	return img
}

func ncSceneWallpaper(w, h int) *widget.Panel {
	root := widget.NewPanel(color.RGBA{R: 0, G: 120, B: 215, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	// Полосы, чтобы размытие под панелью было заметно.
	for i := 0; i < 8; i++ {
		p := widget.NewPanel(color.RGBA{R: uint8(10 + 20*i), G: uint8(90 + 12*i), B: 200, A: 255})
		p.ShowHeader = false
		p.SetBounds(image.Rect(i*w/8, 0, (i+1)*w/8, h))
		root.AddChild(p)
	}
	return root
}

// ncSampleNotes заполняет центр примером из снимка Windows 10.
func ncSampleNotes(clock time.Time) *desktop.FakeNotifications {
	ns := desktop.NewFakeNotifications()
	blue := color.RGBA{R: 40, G: 140, B: 230, A: 255}
	white := color.RGBA{R: 235, G: 240, B: 245, A: 255}
	green := color.RGBA{R: 60, G: 170, B: 90, A: 255}
	ns.Add(desktop.Notification{
		AppID: "cisco", AppName: "Cisco Secure Client", Icon: ncGlyph(48, white, 0),
		Title: "AnyConnect VPN", Body: "Подключено: gate.avanpost.ru",
		Timestamp: clock.Add(-30 * time.Hour),
	})
	ns.Add(desktop.Notification{
		AppID: "cisco", AppName: "Cisco Secure Client", Icon: ncGlyph(48, white, 0),
		Title: "AnyConnect VPN", Body: "Подключено: gate.avanpost.ru",
		Timestamp: clock.Add(-52 * time.Hour),
	})
	ns.Add(desktop.Notification{
		AppID: "mail", AppName: "Почта", Icon: ncGlyph(48, green, 2),
		Title: "Анна Иванова", Body: "Привет! Посмотри, пожалуйста, договор до конца дня.",
		Timestamp: clock.Add(-40 * time.Minute),
		Actions: []desktop.NotificationAction{
			{ID: "reply", Kind: desktop.NotificationActionReply},
			{ID: "open", Kind: desktop.NotificationActionLink, Title: "Открыть письмо"},
		},
	})
	ns.Add(desktop.Notification{
		AppID: "onedrive", AppName: "OneDrive", Icon: ncGlyph(48, blue, 0),
		Title:     "Включить \"Архивация Windows\"",
		Body:      "Автоматическое сохранение рабочего стола, документов и изображений в OneDrive, чтобы защитить их и получить к ним доступ с других устройств.",
		Timestamp: clock.Add(-2*time.Hour - 32*time.Minute),
		Actions: []desktop.NotificationAction{
			{ID: "when", Kind: desktop.NotificationActionSelect, Title: "Напомнить еще раз через:",
				Options: []string{"1 день", "1 неделю", "1 месяц"}, Selected: 1},
			{ID: "ok", Kind: desktop.NotificationActionButton, Title: "Приступим"},
			{ID: "no", Kind: desktop.NotificationActionButton, Title: "Нет"},
		},
	})
	return ns
}

func ncSampleQuick() *desktop.QuickActionList {
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	return desktop.NewQuickActionList(
		desktop.QuickAction{ID: "location", Title: "Расположение", Icon: ncGlyph(32, white, 1), Disabled: true},
		desktop.QuickAction{ID: "bt", Title: "Не подключено", Icon: ncGlyph(32, white, 2), On: true},
		desktop.QuickAction{ID: "night", Title: "Ночной свет", Icon: ncGlyph(32, white, 0)},
		desktop.QuickAction{ID: "plane", Title: "Режим \"в самолёте\"", Icon: ncGlyph(32, white, 1)},
		desktop.QuickAction{ID: "net", Title: "Сеть", Icon: ncGlyph(32, white, 0), On: true},
		desktop.QuickAction{ID: "vpn", Title: "VPN", Icon: ncGlyph(32, white, 2)},
		desktop.QuickAction{ID: "focus", Title: "Помощник по фокусировке", Icon: ncGlyph(32, white, 1)},
		desktop.QuickAction{ID: "tablet", Title: "Режим планшета", Icon: ncGlyph(32, white, 2)},
	)
}

// ncSceneOpts — что менять в снимке.
type ncSceneOpts struct {
	light    bool
	scale    float64
	w, h     int
	expanded bool
	collapse string                               // группа, которую надо свернуть
	extra    func(ns *desktop.FakeNotifications)  // дополнительные уведомления
	after    func(nc *desktop.NotificationCenter) // действия пользователя после открытия
}

// ncRender собирает рабочий стол с открытым центром Windows 10 и снимает кадр.
func ncRender(t *testing.T, o ncSceneOpts) (*image.RGBA, *desktop.NotificationCenter) {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	m.SetFlag(theme.KeyTaskbarLight, o.light)
	m.SetIconResolver(widget.BuiltinIcons())

	w, h := o.w, o.h
	if w == 0 {
		w, h = 1280, 760
	}
	root, bar := buildScene(t, m, w, h)
	t.Cleanup(bar.Close)

	clock := time.Date(2026, 10, 5, 7, 37, 0, 0, time.Local)
	ns := ncSampleNotes(clock)
	if o.extra != nil {
		o.extra(ns)
	}
	btn := desktop.NewNotificationButton(m, ns)
	bar.AddItem(desktop.SlotTray, btn)
	bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))

	nc := desktop.NewNotificationCenter(m, ns)
	nc.Clock = desktop.NewFakeClock(clock)
	nc.Culture = desktop.LocaleCulture{}
	nc.SetQuickActions(ncSampleQuick())
	nc.Screen = image.Rect(0, 0, w, h)
	nc.SetQuickExpanded(o.expanded)
	if o.collapse != "" {
		nc.SetGroupCollapsed(desktop.AppID(o.collapse), true)
	}
	fm := desktop.NewFlyoutManager()
	fm.Register("notifications", nc)
	root.AddChild(fm)
	t.Cleanup(nc.Close)

	scale := o.scale
	if scale <= 0 {
		scale = 1
	}
	eng := engine.New(w, h, 30) // размер логический, физический — умноженный на масштаб
	eng.SetScale(scale)
	eng.SetRoot(root)
	// Значок в трее ниже панели, а центр должен лечь на неё вплотную: якорем
	// служит полоса панели под значком.
	bb := btn.Bounds()
	nc.Open(image.Rect(bb.Min.X, bar.Bounds().Min.Y, bb.Max.X, bar.Bounds().Max.Y))
	nc.Settle()
	if o.after != nil {
		o.after(nc)
	}
	eng.Invalidate()
	return eng.RenderOnce(), nc
}

func ncSave(t *testing.T, img *image.RGBA, name string) {
	dir := os.Getenv("NC_OUT")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

var _ = draw.Draw

func TestVisual_NotificationCenterWin10(t *testing.T) {
	cases := []struct {
		name string
		o    ncSceneOpts
	}{
		{"dark", ncSceneOpts{}},
		{"light", ncSceneOpts{light: true}},
		{"dark_expanded", ncSceneOpts{expanded: true}},
		{"dark_collapsed_group", ncSceneOpts{collapse: "onedrive"}},
		{"dark_200", ncSceneOpts{scale: 2, w: 640, h: 380}},
		{"dark_hover_card", ncSceneOpts{after: func(nc *desktop.NotificationCenter) {
			nc.OnMouseMove(1100, 400)
		}}},
		{"dark_dropdown", ncSceneOpts{after: func(nc *desktop.NotificationCenter) {
			click := func(x, y int) {
				nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
				nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
			}
			click(1080, 507)
			nc.OnMouseMove(1000, 545)
		}}},
		{"dark_focus", ncSceneOpts{after: func(nc *desktop.NotificationCenter) {
			nc.SetFocused(true)
			for i := 0; i < 4; i++ {
				nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
			}
		}}},
		{"light_focus", ncSceneOpts{light: true, after: func(nc *desktop.NotificationCenter) {
			nc.SetFocused(true)
			for i := 0; i < 11; i++ {
				nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
			}
		}}},
		// Середина раскрытия карточки: свёрнули и раскрываем заново, часы
		// анимации остановлены на 50 мс из 150.
		{"dark_card_expanding", ncSceneOpts{after: func(nc *desktop.NotificationCenter) {
			click := func(x, y int) {
				nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
				nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
			}
			click(1248, 373) // шеврон карточки OneDrive: свернуть
			t0 := time.Now()
			widget.StepAnimations(t0)
			widget.StepAnimations(t0.Add(time.Hour))
			click(1248, 373) // и раскрыть
			t1 := time.Now()
			widget.StepAnimations(t1)
			widget.StepAnimations(t1.Add(50 * time.Millisecond))
		}}},
		{"dark_group_collapsing", ncSceneOpts{after: func(nc *desktop.NotificationCenter) {
			nc.SetGroupCollapsed("mail", true)
			t0 := time.Now()
			widget.StepAnimations(t0)
			widget.StepAnimations(t0.Add(50 * time.Millisecond))
		}}},
		{"dark_quick_expanding", ncSceneOpts{after: func(nc *desktop.NotificationCenter) {
			nc.SetQuickExpanded(true)
			t0 := time.Now()
			widget.StepAnimations(t0)
			widget.StepAnimations(t0.Add(50 * time.Millisecond))
		}}},
		// Бегунок схвачен и оттянут вниз: список прокручен, бегунок не гаснет.
		{"dark_scrollbar_drag", ncSceneOpts{after: func(nc *desktop.NotificationCenter) {
			nc.OnMouseMove(1276, 100)
			nc.OnMouseButton(widget.MouseEvent{X: 1276, Y: 100, Button: widget.MouseLeft, Pressed: true})
			nc.OnMouseMove(1276, 190)
		}}},
		{"light_scrollbar_drag", ncSceneOpts{light: true, after: func(nc *desktop.NotificationCenter) {
			nc.OnMouseMove(1276, 100)
			nc.OnMouseButton(widget.MouseEvent{X: 1276, Y: 100, Button: widget.MouseLeft, Pressed: true})
			nc.OnMouseMove(1276, 190)
		}}},
		{"dark_severity", ncSceneOpts{extra: func(ns *desktop.FakeNotifications) {
			clock := time.Date(2026, 10, 5, 7, 37, 0, 0, time.Local)
			ns.Add(desktop.Notification{AppID: "sys", AppName: "Система", Icon: ncGlyph(48, color.RGBA{R: 235, G: 240, B: 245, A: 255}, 2),
				Title: "Низкий заряд батареи", Body: "Осталось 9 %. Подключите питание.",
				Timestamp: clock.Add(-5 * time.Minute), Severity: desktop.SeverityWarning})
			ns.Add(desktop.Notification{AppID: "sys", AppName: "Система", Icon: ncGlyph(48, color.RGBA{R: 235, G: 240, B: 245, A: 255}, 2),
				Title: "Ошибка обновления", Body: "Не удалось установить обновление 0x80070057.",
				Timestamp: clock.Add(-2 * time.Minute), Severity: desktop.SeverityError})
		}}},
		{"light_severity", ncSceneOpts{light: true, extra: func(ns *desktop.FakeNotifications) {
			clock := time.Date(2026, 10, 5, 7, 37, 0, 0, time.Local)
			ns.Add(desktop.Notification{AppID: "sys", AppName: "Система", Icon: ncGlyph(48, color.RGBA{R: 90, G: 90, B: 90, A: 255}, 2),
				Title: "Низкий заряд батареи", Body: "Осталось 9 %. Подключите питание.",
				Timestamp: clock.Add(-5 * time.Minute), Severity: desktop.SeverityWarning})
			ns.Add(desktop.Notification{AppID: "sys", AppName: "Система", Icon: ncGlyph(48, color.RGBA{R: 90, G: 90, B: 90, A: 255}, 2),
				Title: "Ошибка обновления", Body: "Не удалось установить обновление 0x80070057.",
				Timestamp: clock.Add(-2 * time.Minute), Severity: desktop.SeverityError})
		}}},
		{"dark_scrolled", ncSceneOpts{after: func(nc *desktop.NotificationCenter) {
			nc.OnMouseWheelPixels(1000, 300, 0, 200)
			nc.OnMouseMove(1000, 300)
		}}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			img, nc := ncRender(t, c.o)
			if img == nil {
				t.Fatal("кадр не отрисован")
			}
			r := nc.OverlayBounds()
			if r.Empty() {
				t.Fatal("центр не открылся")
			}
			ncSave(t, img, "nc_win10_"+c.name)
		})
	}
}
