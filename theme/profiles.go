package theme

import "time"

// Встроенные профили тем.
//
// Порядок объявления выбран не по красоте, а по возрастанию требований к
// рендереру: Windows 2000 не нуждается ни в размытии, ни в прозрачности, ни
// в скруглениях — на нём проверяется сама архитектура. Windows 10 добавляет
// плоскую палитру, Windows 11 — стекло и скругления, macOS — иную форму
// (док вместо полосы кнопок), и на нём проверяется механизм презентеров.
//
// Каждая тема объявлена парой: общий профиль и его разновидность, которая
// переопределяет десяток токенов и наследует остальное. Тёмная тема, в
// которой пришлось бы переписать всю палитру, означала бы, что общее не
// вынесено в родителя.

// Имена встроенных профилей.
const (
	ProfileWindows2000     = "Windows2000"
	ProfileWindows2000Blue = "Windows2000 Blue"
	ProfileWindows10       = "Windows10"
	ProfileWindows10Dark   = "Windows10 Dark"
	ProfileWindows11       = "Windows11"
	ProfileWindows11Dark   = "Windows11 Dark"
	ProfileMacOS           = "macOS"
	ProfileMacOSDark       = "macOS Dark"
)

// RegisterBuiltinProfiles регистрирует в менеджере все встроенные профили.
// Возвращает первую ошибку регистрации, если она случится.
func RegisterBuiltinProfiles(m *Manager) error {
	for _, p := range []*Profile{
		Windows2000Profile(),
		Windows2000BlueProfile(),
		Windows10Profile(),
		Windows10DarkProfile(),
		Windows11Profile(),
		Windows11DarkProfile(),
		MacOSProfile(),
		MacOSDarkProfile(),
	} {
		if err := m.RegisterTheme(p); err != nil {
			return err
		}
	}
	return nil
}

// ─── Windows 2000 ───────────────────────────────────────────────────────────

// Windows2000Profile — классическая тема: объёмные фаски, прямые углы,
// никаких анимаций и никакой прозрачности.
//
// Отсутствие анимаций выражено данными, а не кодом: длительности нулевые,
// и компонент, который честно спрашивает у темы длительность, переключается
// мгновенно, не зная, что тема «классическая».
func Windows2000Profile() *Profile {
	face := RGB(212, 208, 200)   // «лицо» элементов управления
	light := RGB(255, 255, 255)  // светлая грань фаски
	shadow := RGB(128, 128, 128) // тёмная грань
	dark := RGB(64, 64, 64)      // внутренняя тень
	navy := RGB(10, 36, 106)     // заголовок активного окна
	text := RGB(0, 0, 0)
	selection := RGB(10, 36, 106)

	p := NewProfile(ProfileWindows2000)
	p.SetColor("accent", navy).
		SetColor("surface", face).
		SetColor("text", text).
		SetColor("border", shadow).
		SetColor("selection", selection).
		SetColor("bevel.light", light).
		SetColor("bevel.shadow", shadow).
		SetColor("bevel.dark", dark)

	p.SetMetric("control.corner", 0).
		SetMetric("window.corner", 0).
		SetMetric("control.pad.x", 8).
		SetMetric("control.pad.y", 4).
		SetMetric("taskbar.height", 28).
		SetMetric("taskbar.pad.x", 2).
		SetMetric("taskbar.gap", 2).
		SetMetric("tray.icon.size", 14).
		SetMetric("startbutton.icon.size", 14).
		SetMetric("startbutton.icon.gap", 2).
		SetMetric("startbutton.label.gap", 4).
		SetMetric("startbutton.label.width", 34).
		SetMetric("taskbutton.width", 150).
		SetMetric("taskbutton.width.min", 60).
		SetMetric("taskbutton.icon.size", 14).
		SetMetric("taskbutton.gap", 2).
		SetMetric("taskbutton.label.gap", 4)

	p.SetFlag("style.classic3d", true).
		SetFlag("style.mac.titlebar", false).
		SetFlag("taskbar.centered", false)

	p.Fonts["default"] = FontSpec{Size: 9}

	// Анимаций нет — нулевая длительность и есть отказ от движения.
	for _, k := range []Key{"window.open", "menu.open", "hover", "taskbar.item"} {
		p.Anims[k] = AnimSpec{}
	}

	bevel := &BevelSpec{Light: light, Shadow: shadow, Dark: dark, Width: 2}
	sunken := &BevelSpec{Light: light, Shadow: shadow, Dark: dark, Width: 2, Sunken: true}

	// Панель задач и её элементы: то же «лицо» и та же фаска, что у кнопок, —
	// в этой теме панель и есть ряд кнопок.
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{
		Fill:  C(face),
		Bevel: bevel,
	})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{
		Fill: C(face), Text: C(text), Bevel: bevel,
		PadX: N(6), PadY: N(3),
	})
	p.SetStyle("startbutton", "", StatePressed, StyleDelta{Bevel: sunken})
	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{
		Fill: C(face), Text: C(text), Bevel: bevel, PadX: N(6),
	})
	p.SetStyle("taskbutton", "", StateActive, StyleDelta{Bevel: sunken})
	p.SetStyle("taskbutton", "", StatePressed, StyleDelta{Bevel: sunken})

	p.SetStyle("tray.network", "", StateNormal, StyleDelta{Fill: C(face), Text: C(text)})
	p.SetStyle("tray.volume", "", StateNormal, StyleDelta{Fill: C(face), Text: C(text)})
	p.SetStyle("tray.power", "", StateNormal, StyleDelta{Fill: C(face), Text: C(text)})
	p.SetStyle("clock", "", StateNormal, StyleDelta{
		Fill: C(face), Text: C(text), Bevel: sunken, PadX: N(6),
	})

	p.SetStyle("menu", "", StateNormal, StyleDelta{Fill: C(face), Text: C(text), Bevel: bevel})
	p.SetStyle("menu", "item", StateHover, StyleDelta{
		Fill: C(selection), Text: C(RGB(255, 255, 255)),
	})
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(face), Bevel: bevel})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{
		Fill: C(RGB(128, 128, 128)), Text: C(RGB(212, 208, 200)),
	})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		Fill: C(navy), Text: C(RGB(255, 255, 255)),
	})
	return p
}

// Windows2000BlueProfile — та же классика в синих тонах: отличается
// палитрой, всё остальное наследует.
func Windows2000BlueProfile() *Profile {
	p := NewProfile(ProfileWindows2000Blue)
	p.Parent = ProfileWindows2000

	face := RGB(198, 210, 236)
	p.SetColor("surface", face).
		SetColor("accent", RGB(0, 84, 227))
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{Fill: C(RGB(58, 110, 216))})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{Fill: C(RGB(60, 152, 60))})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		Fill: C(RGB(0, 84, 227)), Text: C(RGB(255, 255, 255)),
	})
	return p
}

// ─── Windows 10 ─────────────────────────────────────────────────────────────

// Windows10Profile — плоская тема: прямые углы, сплошные цвета, короткие
// анимации.
func Windows10Profile() *Profile {
	accent := RGB(0, 120, 215)
	surface := RGB(243, 243, 243)
	text := RGB(0, 0, 0)

	p := NewProfile(ProfileWindows10)
	p.SetColor("accent", accent).
		SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGB(200, 200, 200)).
		SetColor("selection", accent)

	p.SetMetric("control.corner", 0).
		SetMetric("window.corner", 0).
		SetMetric("control.pad.x", 12).
		SetMetric("control.pad.y", 6).
		SetMetric("taskbar.height", 40).
		SetMetric("taskbar.pad.x", 0).
		SetMetric("taskbar.gap", 2).
		SetMetric("tray.icon.size", 16).
		SetMetric("startbutton.icon.size", 18).
		SetMetric("startbutton.icon.gap", 2).
		SetMetric("startbutton.label.gap", 6).
		SetMetric("startbutton.label.width", 40).
		SetMetric("taskbutton.width", 160).
		SetMetric("taskbutton.width.min", 64).
		SetMetric("taskbutton.icon.size", 16).
		SetMetric("taskbutton.gap", 2).
		SetMetric("taskbutton.label.gap", 6)

	p.SetFlag("style.classic3d", false).
		SetFlag("style.mac.titlebar", false).
		SetFlag("taskbar.centered", false)

	p.Fonts["default"] = FontSpec{Size: 9}
	p.Anims["hover"] = AnimSpec{Duration: 120 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["menu.open"] = AnimSpec{Duration: 150 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["window.open"] = AnimSpec{Duration: 150 * time.Millisecond, Curve: "out-cubic"}

	taskbarFill := RGB(31, 31, 31)
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{Fill: C(taskbarFill)})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{
		Fill: C(taskbarFill), Text: C(RGB(255, 255, 255)), PadX: N(10),
	})
	p.SetStyle("startbutton", "", StateHover, StyleDelta{Fill: C(RGB(56, 56, 56))})
	p.SetStyle("startbutton", "", StatePressed, StyleDelta{Fill: C(RGB(72, 72, 72))})

	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{
		Fill: C(taskbarFill), Text: C(RGB(230, 230, 230)), PadX: N(8),
	})
	p.SetStyle("taskbutton", "", StateHover, StyleDelta{Fill: C(RGB(56, 56, 56))})
	p.SetStyle("taskbutton", "", StateActive, StyleDelta{
		Fill: C(RGB(64, 64, 64)), Border: C(accent), BorderWidth: N(1),
	})
	p.SetStyle("taskbutton", "", StateDisabled, StyleDelta{Text: C(RGB(150, 150, 150))})

	for _, comp := range []string{"tray.network", "tray.volume", "tray.power"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{
			Fill: C(taskbarFill), Text: C(RGB(230, 230, 230)), PadX: N(6),
		})
		p.SetStyle(comp, "", StateHover, StyleDelta{Fill: C(RGB(56, 56, 56))})
	}
	p.SetStyle("clock", "", StateNormal, StyleDelta{
		Fill: C(taskbarFill), Text: C(RGB(240, 240, 240)), PadX: N(10),
	})
	p.SetStyle("clock", "", StateHover, StyleDelta{Fill: C(RGB(56, 56, 56))})

	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Fill: C(RGB(43, 43, 43)), Text: C(RGB(240, 240, 240)),
		Border: C(RGB(70, 70, 70)), BorderWidth: N(1), Elevation: N(6),
		Shadow: C(RGBA(0, 0, 0, 90)),
	})
	p.SetStyle("menu", "item", StateHover, StyleDelta{Fill: C(RGB(62, 62, 62))})
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface)})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{
		Fill: C(RGB(90, 90, 90)), Text: C(RGB(200, 200, 200)),
	})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		Fill: C(accent), Text: C(RGB(255, 255, 255)),
	})
	return p
}

// Windows10DarkProfile — тёмная разновидность: меняет палитру поверхностей
// и текста, всё остальное наследует.
func Windows10DarkProfile() *Profile {
	p := NewProfile(ProfileWindows10Dark)
	p.Parent = ProfileWindows10

	surface := RGB(32, 32, 32)
	text := RGB(240, 240, 240)
	p.SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGB(70, 70, 70))
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface)})
	p.SetStyle("menu", "", StateNormal, StyleDelta{Fill: C(RGB(43, 43, 43)), Text: C(text)})
	return p
}

// ─── Windows 11 ─────────────────────────────────────────────────────────────

// Windows11Profile — скруглённая тема со стеклом: панель задач по центру,
// подложка acrylic, мягкие тени у всплывающих поверхностей.
//
// Размытая подложка объявлена честно, а не подделана плоским цветом: пока
// рендерер не умел размывать, такой профиль рисовался бы полупрозрачной
// плёнкой, а с появлением BlurBehind начал бы размывать сам — заменой
// реализации, без правки темы.
func Windows11Profile() *Profile {
	accent := RGB(0, 103, 192)
	surface := RGB(243, 243, 243)
	text := RGB(0, 0, 0)

	p := NewProfile(ProfileWindows11)
	p.SetColor("accent", accent).
		SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGBA(0, 0, 0, 20)).
		SetColor("selection", accent)

	p.SetMetric("control.corner", 6).
		SetMetric("window.corner", 12).
		SetMetric("control.pad.x", 12).
		SetMetric("control.pad.y", 6).
		SetMetric("taskbar.height", 48).
		SetMetric("taskbar.pad.x", 8).
		SetMetric("taskbar.gap", 4).
		SetMetric("tray.icon.size", 16).
		SetMetric("startbutton.icon.size", 20).
		SetMetric("startbutton.icon.gap", 3).
		SetMetric("startbutton.label.gap", 8).
		SetMetric("startbutton.label.width", 44).
		SetMetric("taskbutton.width", 44).
		SetMetric("taskbutton.width.min", 40).
		SetMetric("taskbutton.icon.size", 22).
		SetMetric("taskbutton.gap", 4).
		SetMetric("taskbutton.label.gap", 6)

	p.SetFlag("style.classic3d", false).
		SetFlag("style.mac.titlebar", false).
		SetFlag("taskbar.centered", true).  // кнопки по центру — примета Windows 11
		SetFlag("taskbutton.label", false). // только значки, как в Windows 11
		SetFlag("startbutton.label", false)

	p.Fonts["default"] = FontSpec{Size: 9}
	p.Anims["hover"] = AnimSpec{Duration: 150 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["menu.open"] = AnimSpec{Duration: 200 * time.Millisecond, Curve: "out-back"}
	p.Anims["window.open"] = AnimSpec{Duration: 250 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["taskbar.item"] = AnimSpec{Duration: 180 * time.Millisecond, Curve: "out-cubic"}

	// Панель задач: стекло поверх обоев.
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 30, Tint: RGBA(243, 243, 243, 190),
		},
		Border: C(RGBA(0, 0, 0, 15)), BorderWidth: N(1),
	})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{
		Text: C(text), Corner: N(6), PadX: N(10),
	})
	p.SetStyle("startbutton", "", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})
	p.SetStyle("startbutton", "", StatePressed, StyleDelta{Fill: C(RGBA(0, 0, 0, 32))})

	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{
		Text: C(text), Corner: N(6), PadX: N(8),
	})
	p.SetStyle("taskbutton", "", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})
	p.SetStyle("taskbutton", "", StateActive, StyleDelta{
		Fill: C(RGBA(0, 0, 0, 28)), Border: C(accent), BorderWidth: N(1),
	})

	for _, comp := range []string{"tray.network", "tray.volume", "tray.power"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{Text: C(text), Corner: N(6), PadX: N(6)})
		p.SetStyle(comp, "", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})
	}
	p.SetStyle("clock", "", StateNormal, StyleDelta{Text: C(text), Corner: N(6), PadX: N(10)})
	p.SetStyle("clock", "", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})

	// Всплывающие поверхности: стекло, крупное скругление, мягкая тень.
	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{Mode: BackdropBlur, Radius: 30, Tint: RGBA(249, 249, 249, 205)},
		Text:     C(text), Corner: N(8), Elevation: N(8), Shadow: C(RGBA(0, 0, 0, 70)),
		Border: C(RGBA(0, 0, 0, 15)), BorderWidth: N(1),
	})
	p.SetStyle("menu", "item", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 18)), Corner: N(4)})
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface), Corner: N(12)})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{
		Fill: C(RGB(243, 243, 243)), Text: C(RGB(120, 120, 120)),
	})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		Fill: C(RGB(243, 243, 243)), Text: C(text),
	})
	return p
}

// Windows11DarkProfile — тёмная разновидность.
//
// Ровно то, ради чего затевалось наследование: десяток токенов вместо копии
// всей палитры. Тест на размер профиля стережёт это обещание.
func Windows11DarkProfile() *Profile {
	p := NewProfile(ProfileWindows11Dark)
	p.Parent = ProfileWindows11

	surface := RGB(32, 32, 32)
	text := RGB(255, 255, 255)

	p.SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGBA(255, 255, 255, 20))

	p.SetStyle("taskbar", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{Mode: BackdropBlur, Radius: 30, Tint: RGBA(32, 32, 32, 190)},
	})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("startbutton", "", StateHover, StyleDelta{Fill: C(RGBA(255, 255, 255, 24))})
	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("taskbutton", "", StateHover, StyleDelta{Fill: C(RGBA(255, 255, 255, 24))})
	p.SetStyle("clock", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{Mode: BackdropBlur, Radius: 30, Tint: RGBA(44, 44, 44, 205)},
		Text:     C(text),
	})
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface)})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{Fill: C(surface), Text: C(text)})
	return p
}

// ─── macOS ──────────────────────────────────────────────────────────────────

// MacOSProfile — тема, которая меняет не палитру, а форму.
//
// Область приложений здесь не полоса кнопок, а док: элементы по центру, на
// плавающей подложке, значок под курсором увеличивается. Одной палитрой это
// не выражается, поэтому профиль объявляет ПРЕЗЕНТЕР — отдельную отрисовку
// компонента, которую тема приносит с собой. Компонент об этом не знает и
// остаётся тем же самым: тесты на активацию и сворачивание проходят для
// обеих тем без изменений.
func MacOSProfile() *Profile {
	accent := RGB(0, 122, 255)
	surface := RGB(246, 246, 246)
	text := RGB(0, 0, 0)

	p := NewProfile(ProfileMacOS)
	p.SetColor("accent", accent).
		SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGBA(0, 0, 0, 25)).
		SetColor("selection", accent)

	p.SetMetric("control.corner", 8).
		SetMetric("window.corner", 10).
		SetMetric("control.pad.x", 14).
		SetMetric("control.pad.y", 6).
		SetMetric("taskbar.height", 64).
		SetMetric("taskbar.pad.x", 12).
		SetMetric("taskbar.gap", 6).
		SetMetric("dock.icon", 44).
		SetMetric("dock.magnify", 1.6).
		SetMetric("tray.icon.size", 16).
		SetMetric("startbutton.icon.size", 18).
		SetMetric("startbutton.icon.gap", 3).
		SetMetric("startbutton.label.gap", 6).
		SetMetric("startbutton.label.width", 40).
		SetMetric("taskbutton.width", 48).
		SetMetric("taskbutton.width.min", 44).
		SetMetric("taskbutton.icon.size", 32).
		SetMetric("taskbutton.gap", 6).
		SetMetric("taskbutton.label.gap", 0)

	p.SetFlag("style.classic3d", false).
		SetFlag("style.mac.titlebar", true).
		SetFlag("taskbar.centered", true)

	p.Fonts["default"] = FontSpec{Size: 9}
	p.Anims["hover"] = AnimSpec{Duration: 120 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["menu.open"] = AnimSpec{Duration: 180 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["dock.magnify"] = AnimSpec{Duration: 120 * time.Millisecond, Curve: "out-quad"}

	// Док рисует область приложений вместо полосы кнопок.
	p.Presenters["runningapps"] = "dock"

	p.SetStyle("taskbar", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{Mode: BackdropBlur, Radius: 24, Tint: RGBA(246, 246, 246, 170)},
		Corner:   N(16), Elevation: N(10), Shadow: C(RGBA(0, 0, 0, 60)),
		Border: C(RGBA(255, 255, 255, 60)), BorderWidth: N(1),
	})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{Text: C(text), Corner: N(8), PadX: N(10)})
	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{Text: C(text), Corner: N(10), PadX: N(6)})
	p.SetStyle("taskbutton", "", StateActive, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})
	for _, comp := range []string{"tray.network", "tray.volume", "tray.power"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{Text: C(text), PadX: N(8)})
	}
	p.SetStyle("clock", "", StateNormal, StyleDelta{Text: C(text), PadX: N(10)})
	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{Mode: BackdropBlur, Radius: 24, Tint: RGBA(250, 250, 250, 210)},
		Text:     C(text), Corner: N(10), Elevation: N(10), Shadow: C(RGBA(0, 0, 0, 60)),
	})
	p.SetStyle("menu", "item", StateHover, StyleDelta{Fill: C(accent), Text: C(RGB(255, 255, 255)), Corner: N(6)})
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface), Corner: N(10)})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{
		Fill: C(RGB(236, 236, 236)), Text: C(RGB(120, 120, 120)),
	})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		Fill: C(RGB(236, 236, 236)), Text: C(text),
	})
	return p
}

// MacOSDarkProfile — тёмная разновидность.
func MacOSDarkProfile() *Profile {
	p := NewProfile(ProfileMacOSDark)
	p.Parent = ProfileMacOS

	surface := RGB(40, 40, 42)
	text := RGB(255, 255, 255)
	p.SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGBA(255, 255, 255, 30))
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{Mode: BackdropBlur, Radius: 24, Tint: RGBA(40, 40, 42, 180)},
	})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("clock", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{Mode: BackdropBlur, Radius: 24, Tint: RGBA(50, 50, 52, 210)},
		Text:     C(text),
	})
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface)})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{Fill: C(RGB(50, 50, 52)), Text: C(RGB(150, 150, 150))})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{Fill: C(RGB(50, 50, 52)), Text: C(text)})
	return p
}
