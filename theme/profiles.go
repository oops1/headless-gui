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
	navy := RGB(10, 36, 106)     // акцент: заголовок активного окна и выделение
	text := RGB(0, 0, 0)

	p := NewProfile(ProfileWindows2000)
	// Имя notificationcenter — продолжение notifications (см. Windows10Profile).
	p.SetStyleBase("notificationcenter", "notifications")
	p.SetColor("accent", navy).
		SetColor("surface", face).
		SetColor("text", text).
		SetColor("border", shadow).
		SetColor("bevel.light", light).
		SetColor("bevel.shadow", shadow).
		SetColor("bevel.dark", dark)

	p.SetMetric("control.corner", 0).
		SetMetric("window.corner", 0).
		SetMetric("control.pad.x", 8).
		SetMetric("control.pad.y", 4).
		SetMetric("taskbar.height", 28).
		// Толщина панели у бокового края: часам нужна ширина строки «15:09».
		SetMetric("taskbar.width", 62).
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
		SetMetric("taskbutton.label.gap", 4).
		SetMetric("startmenu.width", 180).
		SetMetric("startmenu.row.height", 20).
		SetMetric("startmenu.icon.size", 16).
		SetMetric("quicksettings.width", 176).
		SetMetric("quicksettings.tile", 52).
		SetMetric("quicksettings.gap", 4).
		SetMetric("notifications.width", 240).
		SetMetric("notifications.card.height", 52).
		SetMetric("notifications.gap", 4).
		SetMetric("calendar.cell", 20).
		SetMetric("calendar.width", 164).
		SetMetric("calendar.header.height", 22).
		SetMetric("tray.label.width.min", 28)

	p.SetFlag("style.classic3d", true).
		SetFlag("style.mac.titlebar", false).
		SetFlag("taskbar.centered", false).
		SetFlag("taskbar.separators", true). // хваталки между секциями
		SetFlag("startbutton.label", true).  // «Пуск» рядом со значком
		SetFlag("taskbutton.label", true).
		// Предпросмотра окон Windows 2000 не имела — выключаем.
		SetFlag("preview", false).
		// Даты в классических часах нет вовсе — она живёт в подсказке.
		SetFlag("clock.date", false)

	// Значок кнопки «Пуск» — из набора иконок темы: оболочка вправе
	// подменить его своим, не трогая компонент.
	p.Icons["startbutton.icon"] = IconRef{Name: "start"}
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
	// Пока меню «Пуск» открыто, кнопка остаётся вдавленной.
	p.SetStyle("startbutton", "", StateActive, StyleDelta{Bevel: sunken})
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
		FillFrom: KeySelection, TextFrom: KeyAccentText,
	})
	// Недоступный пункт — тёмно-серый на «лице»: приглушённый цвет по
	// умолчанию на нём пропадает. Размеры меню — в menu.go.
	p.SetStyle("menu", "item", StateDisabled, StyleDelta{Text: C(RGB(128, 128, 128))})
	declareClassicMenu(p)
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(face), Bevel: bevel})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{
		Fill: C(RGB(128, 128, 128)), Text: C(RGB(212, 208, 200)),
	})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		FillFrom: KeyAccent, TextFrom: KeyAccentText,
	})

	// Всплывающие панели — меню «Пуск», быстрые настройки, уведомления,
	// календарь. Заливка и цвет текста НЕ задаются намеренно: они приходят из
	// плоских токенов "surface" и "text", и тёмная разновидность темы меняет
	// именно их, а не переписывает эти стили заново.
	for _, comp := range []string{"startmenu", "quicksettings", "notifications", "calendar"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{
			Corner: N(0), PadX: N(2), PadY: N(2),
			Bevel: &BevelSpec{Light: light, Shadow: shadow, Dark: dark},
		})
	}
	p.SetStyle("startmenu", "", StateHover, StyleDelta{FillFrom: KeySelection, TextFrom: KeyAccentText})
	p.SetStyle("startmenu", "section", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200)), PadX: N(4)})
	p.SetStyle("quicksettings", "slider", StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0)})
	p.SetStyle("quicksettings", "slider.fill", StateNormal, StyleDelta{FillFrom: KeySelection, Corner: N(0)})
	for _, tile := range []string{"tile.network", "tile.volume", "tile.power"} {
		p.SetStyle("quicksettings", tile, StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0)})
		p.SetStyle("quicksettings", tile, StateActive, StyleDelta{
			FillFrom: KeySelection, TextFrom: KeyAccentText, Corner: N(0),
		})
	}
	// Карточка уведомления: рамка задаёт важность, заливка приходит из палитры.
	p.SetStyle("notifications", "card.info", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0), PadX: N(2), PadY: N(4),
	})
	p.SetStyle("notifications", "card.warning", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0), PadX: N(2), PadY: N(4),
		Border: C(RGB(220, 150, 20)), BorderWidth: N(1),
	})
	p.SetStyle("notifications", "card.error", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0), PadX: N(2), PadY: N(4),
		Border: C(RGB(200, 60, 60)), BorderWidth: N(1),
	})
	p.SetStyle("notifications", "empty", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	p.SetStyle("notifications", "clear", StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0)})
	p.SetStyle("calendar", "weekday", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	p.SetStyle("calendar", "day", StateNormal, StyleDelta{Corner: N(0)})
	p.SetStyle("calendar", "day", StateHover, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0)})
	p.SetStyle("calendar", "day", StateActive, StyleDelta{
		FillFrom: KeySelection, TextFrom: KeyAccentText, Corner: N(0),
	})
	p.SetStyle("calendar", "day", StateFocused, StyleDelta{
		BorderFrom: KeySelection, BorderWidth: N(1), Corner: N(0),
	})
	p.SetStyle("calendar", "day", StateDisabled, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	inheritTrayStyles(p)
	addDialogStyles(p)
	return p
}

// Windows2000BlueProfile — та же классика в синих тонах: отличается
// палитрой, всё остальное наследует.
func Windows2000BlueProfile() *Profile {
	p := NewProfile(ProfileWindows2000Blue)
	p.Parent = ProfileWindows2000

	face := RGB(198, 210, 236)
	// Заголовок окна идёт за акцентом (SetAccent его перекрашивает). Выделение
	// меню в синей разновидности всегда было тёмно-синим, не акцентным: оно
	// объявлено здесь явно, чтобы вид без SetAccent остался прежним, и поэтому
	// остаётся решением профиля, когда акцент меняют.
	p.SetColor("surface", face).
		SetColor("accent", RGB(0, 84, 227)).
		SetColor("selection", RGB(10, 36, 106))
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{Fill: C(RGB(58, 110, 216))})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{Fill: C(RGB(60, 152, 60))})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		FillFrom: KeyAccent, TextFrom: KeyAccentText,
	})
	addDialogStyles(p)
	return p
}

// ─── Windows 10 ─────────────────────────────────────────────────────────────

// KeyTaskbarLight — флаг светлого оформления оболочки Windows 10: светлая
// панель задач и светлые панели «Пуска» и центра уведомлений (в Windows 10 это
// один режим «Светлый»). По умолчанию флага нет — оболочка тёмная, как была.
// Включить можно в профиле (SetFlag) или на лету через Manager.SetFlag.
const KeyTaskbarLight Key = "taskbar.light"

// Windows10Profile — плоская тема: прямые углы, плоские цвета, короткие
// анимации. Панель задач и панели оболочки — акрил: затемнение, размытие
// того, что лежит под ними, и слабый шум; там, где размывать нечем, тема
// отдаёт близкий непрозрачный цвет (BackdropSpec.Fallback).
//
// Акцент — сменяемый токен: стили ссылаются на него (FillFrom/BorderFrom), и
// Manager.SetAccent перекрашивает тему, не пересоздавая компонентов.
func Windows10Profile() *Profile {
	accent := RGB(0, 120, 215)
	surface := RGB(243, 243, 243)
	text := RGB(0, 0, 0)

	p := NewProfile(ProfileWindows10)
	p.SetColor("accent", accent).
		SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGB(200, 200, 200))

	// Выделение (токен "selection") не объявляется: по умолчанию оно равно
	// акценту и следует за его сменой.

	p.SetMetric("control.corner", 0).
		SetMetric("window.corner", 0).
		SetMetric("control.pad.x", 12).
		SetMetric("control.pad.y", 6).
		SetMetric("taskbar.height", 40).
		// Толщина панели у бокового края: по ширине даты «14.03.2026».
		SetMetric("taskbar.width", 62).
		SetMetric("taskbar.pad.x", 0).
		// Замеры Windows 10 при 100 %: элементы панели стоят вплотную, «Пуск»
		// 48×40 со значком 16.
		SetMetric("taskbar.gap", 0).
		SetMetric("tray.icon.size", 16).
		SetMetric("startbutton.icon.size", 16).
		SetMetric("startbutton.icon.gap", 2).
		SetMetric("startbutton.label.gap", 6).
		SetMetric("startbutton.label.width", 40).
		SetMetric("taskbutton.width", 160).
		SetMetric("taskbutton.width.min", 64).
		SetMetric("taskbutton.icon.size", 16).
		SetMetric("taskbutton.gap", 2).
		SetMetric("taskbutton.label.gap", 6).
		SetMetric("startmenu.width", 260).
		SetMetric("startmenu.row.height", 28).
		SetMetric("startmenu.icon.size", 20).
		SetMetric("quicksettings.width", 240).
		SetMetric("quicksettings.tile", 64).
		SetMetric("quicksettings.gap", 6).
		SetMetric("notifications.width", 300).
		SetMetric("notifications.card.height", 60).
		SetMetric("notifications.gap", 6).
		SetMetric("calendar.cell", 28).
		SetMetric("calendar.width", 220).
		SetMetric("calendar.header.height", 30).
		SetMetric("taskbutton.underline", 2).    // полоса под открытым окном
		SetMetric("taskbutton.underline.len", 1). // во всю ширину кнопки
		// Предпросмотр окна при наведении на кнопку.
		SetMetric("preview.width", 200).
		SetMetric("preview.height", 120).
		SetMetric("preview.pad", 6).
		SetMetric("preview.header", 20).
		SetMetric("preview.delay.open", 500).
		SetMetric("preview.delay.close", 250).
		SetMetric("preview.refresh", 200).
		SetMetric("tray.label.width.min", 34)

	p.SetFlag("style.classic3d", false).
		SetFlag("style.mac.titlebar", false).
		SetFlag("taskbar.centered", false).
		// Панель Windows 10 тёмная и при светлой теме окон (светлую включает
		// флаг taskbar.light), а кнопки окон показывают только значки:
		// подписи включаются в настройках.
		SetFlag("startbutton.label", false).
		SetFlag("taskbutton.label", false).
		SetFlag(KeyTaskbarLight, false)
	// Кнопки приложений: размеры, линии состояний, группировка окон.
	declareWin10TaskButtons(p)

	// Значок кнопки «Пуск» берётся из набора иконок темы.
	p.Icons["startbutton.icon"] = IconRef{Name: "start"}

	// Windows 10 пишет Segoe UI — шрифтом несвободным, класть его в пакет
	// нельзя. Заменой выбран Open Sans (OFL, лежит в assets/fonts): рисунок
	// того же гуманистического круга, а ширина строки при кегле ×0,94 ложится
	// в 1 % от Segoe UI (измерения — план WinLine, §2.1). Поэтому 8,5 pt там,
	// где в Windows 10 стоит 9 pt. Семейство названо по-человечески, с
	// пробелом: движок находит его и по такому названию, и по имени файла.
	//
	// Если Open Sans в движке не зарегистрирован (assets/fonts не найден,
	// fonts.Register не вызван), текст пишется шрифтом по умолчанию тем же
	// кеглем: ничего не пропадает, только рисунок букв другой.
	p.Fonts["default"] = FontSpec{Family: "Open Sans", Size: 8.5}
	// На 11 пикселях округление шага глифов до целого делает строку неровной:
	// профиль просит дробное позиционирование (engine.SetTextSubpixel).
	p.Flags[FlagTextSubpixel] = true
	// Кегль по умолчанию для виджетов вне оболочки (заголовки окон, диалоги,
	// вкладки) следует Fonts["default"]: без флага они остались бы на 10 pt.
	p.Flags[FlagFontDefaultGlobal] = true
	// Именованные шрифты оболочки; компонент берёт их через Manager.GetFont.
	// Размеры — Segoe UI Windows 10, умноженные на тот же 0,94.
	p.Fonts["caption"] = FontSpec{Family: "Open Sans", Size: 7.5}                         // 8 pt: время и вторая строка в уведомлениях и плитках
	p.Fonts["title"] = FontSpec{Family: "Open Sans", Size: 10, Weight: WeightSemiBold}    // 10,5 pt Semibold: заголовки групп и уведомлений
	p.Fonts["clock.large"] = FontSpec{Family: "Open Sans", Size: 28, Weight: WeightLight} // крупные часы календаря
	p.Anims["hover"] = AnimSpec{Duration: 120 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["menu.open"] = AnimSpec{Duration: 150 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["window.open"] = AnimSpec{Duration: 150 * time.Millisecond, Curve: "out-cubic"}

	// Панель задач — акрил. Запасной непрозрачный цвет — прежний цвет панели:
	// без размытия она выглядит как раньше.
	taskbarFill := RGB(31, 31, 31)
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{
		Fill:     C(taskbarFill),
		Backdrop: win10Acrylic(RGBA(31, 31, 31, 210), taskbarFill),
	})
	// Наведение, нажатие и «активно» — белые плёнки, а не серые плашки: на
	// акриле под панелью лежат обои, и непрозрачный серый выглядел бы чужим.
	// Альфы подобраны так, чтобы на сплошном taskbarFill получались прежние
	// оттенки (56, 72 и 64).
	hover, pressed, active := RGBA(255, 255, 255, 28), RGBA(255, 255, 255, 47), RGBA(255, 255, 255, 38)
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{
		Fill: C(taskbarFill), Text: C(RGB(255, 255, 255)), PadX: N(16),
	})
	p.SetStyle("startbutton", "", StateHover, StyleDelta{Fill: C(hover)})
	p.SetStyle("startbutton", "", StatePressed, StyleDelta{Fill: C(pressed)})

	// Отступ по бокам 12 при значке 24 даёт кнопку шириной 48, как в Windows 10.
	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{
		Fill: C(taskbarFill), Text: C(RGB(230, 230, 230)), PadX: N(12),
	})
	p.SetStyle("taskbutton", "", StateHover, StyleDelta{Fill: C(hover)})
	// Полоса под кнопкой вместо обводки — как на панели Windows 10.
	p.SetStyle("taskbutton", "", StateActive, StyleDelta{
		Fill: C(active), BorderFrom: KeyAccent,
	})
	p.SetStyle("taskbutton", "", StateDisabled, StyleDelta{Text: C(RGB(150, 150, 150))})

	for _, comp := range []string{"tray.network", "tray.volume", "tray.power"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{
			Fill: C(taskbarFill), Text: C(RGB(230, 230, 230)), PadX: N(6),
		})
		p.SetStyle(comp, "", StateHover, StyleDelta{Fill: C(hover)})
	}
	p.SetStyle("clock", "", StateNormal, StyleDelta{
		Fill: C(taskbarFill), Text: C(RGB(240, 240, 240)), PadX: N(10),
	})
	p.SetStyle("clock", "", StateHover, StyleDelta{Fill: C(hover)})

	// Светлая панель: те же компоненты, другая подкраска и цвет текста.
	// Правила лежат поверх тёмных и включаются флагом taskbar.light.
	lightFill := RGB(240, 240, 240)
	lightHover, lightPressed, lightActive := RGBA(0, 0, 0, 26), RGBA(0, 0, 0, 42), RGBA(0, 0, 0, 34)
	when := func(comp, part string, st State, d StyleDelta) {
		p.SetStyleWhen(KeyTaskbarLight, comp, part, st, d)
	}
	when("taskbar", "", StateNormal, StyleDelta{
		Fill:     C(lightFill),
		Backdrop: win10Acrylic(RGBA(240, 240, 240, 210), lightFill),
	})
	when("startbutton", "", StateNormal, StyleDelta{Text: C(RGB(0, 0, 0))})
	when("startbutton", "", StateHover, StyleDelta{Fill: C(lightHover)})
	when("startbutton", "", StatePressed, StyleDelta{Fill: C(lightPressed)})
	when("taskbutton", "", StateNormal, StyleDelta{Text: C(RGB(20, 20, 20))})
	when("taskbutton", "", StateHover, StyleDelta{Fill: C(lightHover)})
	when("taskbutton", "", StateActive, StyleDelta{Fill: C(lightActive)})
	when("taskbutton", "", StateDisabled, StyleDelta{Text: C(RGB(120, 120, 120))})
	for _, comp := range []string{"tray.network", "tray.volume", "tray.power"} {
		when(comp, "", StateNormal, StyleDelta{Text: C(RGB(20, 20, 20))})
		when(comp, "", StateHover, StyleDelta{Fill: C(lightHover)})
	}
	when("clock", "", StateNormal, StyleDelta{Text: C(RGB(0, 0, 0))})
	when("clock", "", StateHover, StyleDelta{Fill: C(lightHover)})
	declareWin10TaskButtonStyles(p)

	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Fill: C(RGB(43, 43, 43)), Text: C(RGB(240, 240, 240)),
		Border: C(RGB(70, 70, 70)), BorderWidth: N(1), Elevation: N(6),
		Shadow: C(RGBA(0, 0, 0, 90)),
	})
	p.SetStyle("menu", "item", StateHover, StyleDelta{Fill: C(RGB(62, 62, 62))})
	p.SetStyle("menu", "item", StateDisabled, StyleDelta{Text: C(RGB(128, 128, 128))})
	// Светлое меню для светлого режима оболочки: контекстное меню команд кнопок
	// приложений и «Пуска» следует панели, а не остаётся тёмным.
	p.SetStyleWhen(KeyTaskbarLight, "menu", "", StateNormal, StyleDelta{
		Fill: C(RGB(242, 242, 242)), Text: C(RGB(0, 0, 0)),
		Border: C(RGB(204, 204, 204)), Shadow: C(RGBA(0, 0, 0, 50)),
	})
	p.SetStyleWhen(KeyTaskbarLight, "menu", "item", StateHover, StyleDelta{Fill: C(RGB(222, 222, 222))})
	p.SetStyleWhen(KeyTaskbarLight, "menu", "item", StateDisabled, StyleDelta{Text: C(RGB(150, 150, 150))})
	declareWin10Menu(p)
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface)})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{
		Fill: C(RGB(90, 90, 90)), Text: C(RGB(200, 200, 200)),
	})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		FillFrom: KeyAccent, TextFrom: KeyAccentText,
	})

	// Всплывающие панели — меню «Пуск», быстрые настройки, уведомления,
	// календарь. Заливка и цвет текста НЕ задаются намеренно: они приходят из
	// плоских токенов "surface" и "text", и тёмная разновидность темы меняет
	// именно их, а не переписывает эти стили заново.
	for _, comp := range []string{"startmenu", "quicksettings", "notifications", "calendar"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{
			Corner: N(0), PadX: N(8), PadY: N(8),
			Border: C(RGBA(0, 0, 0, 40)), BorderWidth: N(1), Elevation: N(6), Shadow: C(RGBA(0, 0, 0, 70)),
		})
	}
	p.SetStyle("startmenu", "", StateHover, StyleDelta{FillFrom: KeyAccent, TextFrom: KeyAccentText})
	p.SetStyle("startmenu", "section", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200)), PadX: N(6)})
	p.SetStyle("quicksettings", "slider", StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0)})
	p.SetStyle("quicksettings", "slider.fill", StateNormal, StyleDelta{FillFrom: KeyAccent, Corner: N(0)})
	for _, tile := range []string{"tile.network", "tile.volume", "tile.power"} {
		p.SetStyle("quicksettings", tile, StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0)})
		p.SetStyle("quicksettings", tile, StateActive, StyleDelta{
			FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(0),
		})
	}
	// Карточка уведомления: рамка задаёт важность, заливка приходит из палитры.
	p.SetStyle("notifications", "card.info", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0), PadX: N(8), PadY: N(4),
	})
	p.SetStyle("notifications", "card.warning", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0), PadX: N(8), PadY: N(4),
		Border: C(RGB(220, 150, 20)), BorderWidth: N(1),
	})
	p.SetStyle("notifications", "card.error", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0), PadX: N(8), PadY: N(4),
		Border: C(RGB(200, 60, 60)), BorderWidth: N(1),
	})
	p.SetStyle("notifications", "empty", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	p.SetStyle("notifications", "clear", StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0)})
	p.SetStyle("calendar", "weekday", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	p.SetStyle("calendar", "day", StateNormal, StyleDelta{Corner: N(0)})
	p.SetStyle("calendar", "day", StateHover, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(0)})
	p.SetStyle("calendar", "day", StateActive, StyleDelta{
		FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(0),
	})
	p.SetStyle("calendar", "day", StateFocused, StyleDelta{
		BorderFrom: KeyAccent, BorderWidth: N(1), Corner: N(0),
	})
	p.SetStyle("calendar", "day", StateDisabled, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	// Панели оболочки: «Пуск» и центр уведомлений. Новое имя компонента
	// notificationcenter начинается со всех правил notifications, поэтому
	// существующий центр выглядит как раньше, а презентер Windows 10 берёт
	// части (panel, header, card…), которых у плоского центра нет.
	p.SetStyleBase("notificationcenter", "notifications")
	declareWin10Panels(func(comp, part string, st State, d StyleDelta) {
		p.SetStyle(comp, part, st, d)
	}, win10ShellDark)
	declareWin10Panels(func(comp, part string, st State, d StyleDelta) {
		p.SetStyleWhen(KeyTaskbarLight, comp, part, st, d)
	}, win10ShellLight)
	// Размеры центра уведомлений и его презентер (profiles_win10_notify.go).
	declareWin10NotificationMetrics(p)
	declareWin10StartMenu(p)

	// Элементы панели не носят собственной заливки в покое: фон им даёт сама
	// панель, а своя плашка появляется только под курсором и у активного
	// окна. Заливка иначе приходит из общего токена surface, и на панели
	// проступают прямоугольники чуть иного оттенка. Блок стоит последним:
	// SetStyle заменяет запись целиком, и любой стиль этих компонентов ниже
	// вернул бы заливку обратно.
	clearFill := C(RGBA(0, 0, 0, 0))
	for _, comp := range []string{"startbutton", "taskbutton", "clock",
		"tray.network", "tray.volume", "tray.power", "tray.chevron"} {
		st := p.Styles[StyleKey{Component: comp, State: StateNormal}]
		st.Fill = clearFill
		p.SetStyle(comp, "", StateNormal, st)
	}

	inheritTrayStyles(p)
	declareWin10Tray(p)
	addDialogStyles(p)
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
	addDialogStyles(p)
	return p
}

// ─── Windows 11 ─────────────────────────────────────────────────────────────

// Windows11Profile — скруглённая тема со стеклом: панель задач по центру,
// подложка acrylic, мягкие тени у всплывающих поверхностей.
//
// Размытая подложка объявлена честно, а не подделана плоским цветом: движок
// размывает композицию под панелью (Canvas.BlurBehind), а контекст без
// размытия (например, окно-попап) рисует тот же слой полупрозрачной
// подкраской Tint.
func Windows11Profile() *Profile {
	accent := RGB(0, 103, 192)
	surface := RGB(243, 243, 243)
	text := RGB(0, 0, 0)

	p := NewProfile(ProfileWindows11)
	// Имя notificationcenter — продолжение notifications (см. Windows10Profile).
	p.SetStyleBase("notificationcenter", "notifications")
	p.SetColor("accent", accent).
		SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGBA(0, 0, 0, 20))

	// Выделение (токен "selection") не объявляется: по умолчанию оно равно
	// акценту и следует за его сменой.

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
		SetMetric("taskbutton.label.gap", 6).
		SetMetric("startmenu.width", 300).
		SetMetric("startmenu.row.height", 34).
		SetMetric("startmenu.icon.size", 24).
		SetMetric("quicksettings.width", 280).
		SetMetric("quicksettings.tile", 72).
		SetMetric("quicksettings.gap", 8).
		SetMetric("notifications.width", 340).
		SetMetric("notifications.card.height", 68).
		SetMetric("notifications.gap", 8).
		SetMetric("calendar.cell", 32).
		SetMetric("calendar.width", 260).
		SetMetric("calendar.header.height", 34).
		SetMetric("taskbutton.underline", 3).       // чёрточка под значком
		SetMetric("taskbutton.underline.len", 0.4). // короткая, не во всю кнопку
		// Предпросмотр окна при наведении на кнопку.
		SetMetric("preview.width", 220).
		SetMetric("preview.height", 130).
		SetMetric("preview.pad", 8).
		SetMetric("preview.header", 22).
		SetMetric("preview.delay.open", 400).
		SetMetric("preview.delay.close", 250).
		SetMetric("preview.refresh", 200).
		SetMetric("tray.label.width.min", 36)

	p.SetFlag("style.classic3d", false).
		SetFlag("style.mac.titlebar", false).
		SetFlag("taskbar.centered", true).  // кнопки по центру — примета Windows 11
		SetFlag("taskbutton.label", false). // только значки, как в Windows 11
		SetFlag("startbutton.label", false)

	// Значок кнопки «Пуск» берётся из набора иконок темы.
	p.Icons["startbutton.icon"] = IconRef{Name: "start"}
	p.Fonts["default"] = FontSpec{Size: 9}
	p.Anims["hover"] = AnimSpec{Duration: 150 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["menu.open"] = AnimSpec{Duration: 200 * time.Millisecond, Curve: "out-back"}
	p.Anims["window.open"] = AnimSpec{Duration: 250 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["taskbar.item"] = AnimSpec{Duration: 180 * time.Millisecond, Curve: "out-cubic"}

	// Панель задач: стекло поверх обоев.
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(243, 243, 243, 150),
			Highlight: RGBA(255, 255, 255, 100),
		},
		Border: C(RGBA(0, 0, 0, 15)), BorderWidth: N(1),
	})
	// Цвета текста и поверхности ниже — ссылками на токены "text" и
	// "surface": тёмная разновидность меняет только токены и не повторяет
	// эти стили (предел её собственных токенов — 15).
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{
		TextFrom: "text", Corner: N(6), PadX: N(10),
	})
	p.SetStyle("startbutton", "", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})
	p.SetStyle("startbutton", "", StatePressed, StyleDelta{Fill: C(RGBA(0, 0, 0, 32))})
	// Меню «Пуск» открыто — кнопка подсвечена как нажатая.
	p.SetStyle("startbutton", "", StateActive, StyleDelta{Fill: C(RGBA(0, 0, 0, 32))})

	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{
		TextFrom: "text", Corner: N(6), PadX: N(8),
	})
	p.SetStyle("taskbutton", "", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})
	// Активное окно отмечено меткой под кнопкой, а не обводкой: цвет метки
	// берётся из Border, но саму рамку не рисуем — BorderWidth остаётся нулём.
	p.SetStyle("taskbutton", "", StateActive, StyleDelta{
		Fill: C(RGBA(0, 0, 0, 28)), BorderFrom: KeyAccent,
	})

	for _, comp := range []string{"tray.network", "tray.volume", "tray.power"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{Text: C(text), Corner: N(6), PadX: N(6)})
		p.SetStyle(comp, "", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})
	}
	p.SetStyle("clock", "", StateNormal, StyleDelta{TextFrom: "text", Corner: N(6), PadX: N(10)})
	p.SetStyle("clock", "", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 20))})

	// Всплывающие поверхности: стекло, крупное скругление, мягкая тень.
	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(249, 249, 249, 150),
			Highlight: RGBA(255, 255, 255, 100)},
		Text: C(text), Corner: N(8), Elevation: N(8), Shadow: C(RGBA(0, 0, 0, 70)),
		Border: C(RGBA(0, 0, 0, 15)), BorderWidth: N(1),
	})
	declareWin11Menu(p)
	p.SetStyle("window", "", StateNormal, StyleDelta{FillFrom: "surface", Corner: N(12)})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{
		Fill: C(RGB(243, 243, 243)), Text: C(RGB(120, 120, 120)),
	})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		FillFrom: "surface", TextFrom: "text",
	})

	// Всплывающие панели — меню «Пуск», быстрые настройки, уведомления,
	// календарь. Заливка и цвет текста НЕ задаются намеренно: они приходят из
	// плоских токенов "surface" и "text", и тёмная разновидность темы меняет
	// именно их, а не переписывает эти стили заново.
	for _, comp := range []string{"startmenu", "quicksettings", "notifications", "calendar"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{
			Corner: N(8), PadX: N(10), PadY: N(10),
			Elevation: N(12), Shadow: C(RGBA(0, 0, 0, 70)),
		})
	}
	p.SetStyle("startmenu", "", StateHover, StyleDelta{FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(6)})
	p.SetStyle("startmenu", "section", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200)), PadX: N(6)})
	p.SetStyle("quicksettings", "slider", StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6)})
	p.SetStyle("quicksettings", "slider.fill", StateNormal, StyleDelta{FillFrom: KeyAccent, Corner: N(6)})
	for _, tile := range []string{"tile.network", "tile.volume", "tile.power"} {
		p.SetStyle("quicksettings", tile, StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6)})
		p.SetStyle("quicksettings", tile, StateActive, StyleDelta{
			FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(6),
		})
	}
	// Карточка уведомления: рамка задаёт важность, заливка приходит из палитры.
	p.SetStyle("notifications", "card.info", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6), PadX: N(10), PadY: N(4),
	})
	p.SetStyle("notifications", "card.warning", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6), PadX: N(10), PadY: N(4),
		Border: C(RGB(220, 150, 20)), BorderWidth: N(1),
	})
	p.SetStyle("notifications", "card.error", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6), PadX: N(10), PadY: N(4),
		Border: C(RGB(200, 60, 60)), BorderWidth: N(1),
	})
	p.SetStyle("notifications", "empty", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	p.SetStyle("notifications", "clear", StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6)})
	p.SetStyle("calendar", "weekday", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	p.SetStyle("calendar", "day", StateNormal, StyleDelta{Corner: N(6)})
	p.SetStyle("calendar", "day", StateHover, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6)})
	p.SetStyle("calendar", "day", StateActive, StyleDelta{
		FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(6),
	})
	p.SetStyle("calendar", "day", StateFocused, StyleDelta{
		BorderFrom: KeyAccent, BorderWidth: N(1), Corner: N(6),
	})
	p.SetStyle("calendar", "day", StateDisabled, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	// Элементы панели не носят собственной заливки в покое: фон им даёт сама
	// панель, а своя плашка появляется только под курсором и у активного
	// окна. Заливка иначе приходит из общего токена surface, и на панели
	// проступают прямоугольники чуть иного оттенка. Блок стоит последним:
	// SetStyle заменяет запись целиком, и любой стиль этих компонентов ниже
	// вернул бы заливку обратно.
	clearFill := C(RGBA(0, 0, 0, 0))
	for _, comp := range []string{"startbutton", "taskbutton", "clock",
		"tray.network", "tray.volume", "tray.power", "tray.chevron"} {
		st := p.Styles[StyleKey{Component: comp, State: StateNormal}]
		st.Fill = clearFill
		p.SetStyle(comp, "", StateNormal, st)
	}

	// Быстрые настройки 24H2 — презентер, метрики и части (profiles_win11_quick.go).
	// До материалов: части, объявленные к их вызову, получают сброс Mica и тени.
	declareWin11QuickSettings(p)

	// Mica, MicaAlt и мягкие тени — только под флагами (profiles_win11_material.go).
	declareWin11Materials(p, RGB(224, 224, 224), RGBA(0, 0, 0, 77))

	inheritTrayStyles(p)
	// Панель по замерам: кнопки 40, пилюли, Task View, виджеты, поиск, трей
	// (profiles_win11_taskbar.go). После унаследования стилей трея.
	declareWin11Taskbar(p)
	addDialogStyles(p)
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
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(32, 32, 32, 150),
			Highlight: RGBA(255, 255, 255, 40)},
	})
	// Текст кнопок, часов и окна приходит из токена "text", поверхность окна —
	// из "surface", плёнки подсветки кнопок и заливка поиска — из film.hover,
	// film.pressed и field.fill (стили родителя ссылаются на них).
	hover, pressed, field := win11DarkFilms()
	p.SetColor(KeyFilmHover, hover).SetColor(KeyFilmPressed, pressed).SetColor(KeyFieldFill, field)
	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(44, 44, 44, 150),
			Highlight: RGBA(255, 255, 255, 40)},
		Text: C(text),
	})
	declareWin11DarkMenu(p)

	// Тёмные MicaAlt и мягкая тень — два токена: стили родителя ссылаются на них.
	p.SetColor(KeySurfaceAlt, RGB(14, 14, 14)).
		SetColor(KeyShadowColor, RGBA(0, 0, 0, 115))
	addDialogStyles(p)
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
	// Имя notificationcenter — продолжение notifications (см. Windows10Profile).
	p.SetStyleBase("notificationcenter", "notifications")
	p.SetColor("accent", accent).
		SetColor("surface", surface).
		SetColor("text", text).
		SetColor("border", RGBA(0, 0, 0, 25))

	// Выделение (токен "selection") не объявляется: по умолчанию оно равно
	// акценту и следует за его сменой.

	p.SetMetric("control.corner", 8).
		SetMetric("window.corner", 10).
		SetMetric("control.pad.x", 14).
		SetMetric("control.pad.y", 6).
		// Строка меню сверху тонкая, док снизу — крупный: в macOS это две
		// разные полосы, а не одна панель задач.
		SetMetric("taskbar.height", 26).
		SetMetric("dock.height", 68).
		SetMetric("dock.pad", 8).
		SetMetric("taskbar.pad.x", 12).
		SetMetric("taskbar.gap", 6).
		SetMetric("dock.icon", 52).
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
		SetMetric("taskbutton.label.gap", 0).
		SetMetric("startmenu.width", 280).
		SetMetric("startmenu.row.height", 30).
		SetMetric("startmenu.icon.size", 22).
		SetMetric("quicksettings.width", 260).
		SetMetric("quicksettings.tile", 68).
		SetMetric("quicksettings.gap", 8).
		SetMetric("notifications.width", 320).
		SetMetric("notifications.card.height", 64).
		SetMetric("notifications.gap", 8).
		SetMetric("calendar.cell", 30).
		SetMetric("calendar.width", 240).
		SetMetric("calendar.header.height", 32).
		SetMetric("tray.label.width.min", 34)

	p.SetFlag("preview", false). // у дока свой механизм показа окон, не миниатюра при наведении
		SetFlag("style.classic3d", false).
		SetFlag("style.mac.titlebar", true).
		// Меню Apple прижато к левому краю строки меню; по центру в macOS
		// стоит только док, и центрирует он себя сам — презентером.
		SetFlag("taskbar.centered", false).
		SetFlag("taskbar.top", true). // строка меню прижата к верху экрана
		SetFlag("startbutton.label", false).
		SetFlag("taskbutton.label", false)

	p.Fonts["default"] = FontSpec{Size: 9}
	p.Anims["hover"] = AnimSpec{Duration: 120 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["menu.open"] = AnimSpec{Duration: 180 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["dock.magnify"] = AnimSpec{Duration: 120 * time.Millisecond, Curve: "out-quad"}

	// Док рисует область приложений вместо полосы кнопок.
	p.Presenters["runningapps"] = "dock"

	// Строка меню — полоса во всю ширину экрана: ни скругления, ни тени у
	// неё нет, только стекло и тонкая линия снизу.
	p.SetStyle("taskbar", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(246, 246, 246, 150),
			Highlight: RGBA(255, 255, 255, 100)},
		Border: C(RGBA(0, 0, 0, 30)), BorderWidth: N(1),
	})
	// Док — плавающая скруглённая плашка с тенью.
	p.SetStyle("dockbar", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(246, 246, 246, 150),
			Highlight: RGBA(255, 255, 255, 100)},
		Corner: N(16), Elevation: N(10), Shadow: C(RGBA(0, 0, 0, 60)),
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
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(250, 250, 250, 150),
			Highlight: RGBA(255, 255, 255, 100)},
		Text: C(text), Corner: N(10), Elevation: N(10), Shadow: C(RGBA(0, 0, 0, 60)),
	})
	p.SetStyle("menu", "item", StateHover, StyleDelta{FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(6)})
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface), Corner: N(10)})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{
		Fill: C(RGB(236, 236, 236)), Text: C(RGB(120, 120, 120)),
	})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{
		Fill: C(RGB(236, 236, 236)), Text: C(text),
	})

	// Всплывающие панели — меню «Пуск», быстрые настройки, уведомления,
	// календарь. Заливка и цвет текста НЕ задаются намеренно: они приходят из
	// плоских токенов "surface" и "text", и тёмная разновидность темы меняет
	// именно их, а не переписывает эти стили заново.
	for _, comp := range []string{"startmenu", "quicksettings", "notifications", "calendar"} {
		p.SetStyle(comp, "", StateNormal, StyleDelta{
			Corner: N(10), PadX: N(10), PadY: N(10),
			Elevation: N(10), Shadow: C(RGBA(0, 0, 0, 60)),
		})
	}
	p.SetStyle("startmenu", "", StateHover, StyleDelta{FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(6)})
	p.SetStyle("startmenu", "section", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200)), PadX: N(6)})
	p.SetStyle("quicksettings", "slider", StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6)})
	p.SetStyle("quicksettings", "slider.fill", StateNormal, StyleDelta{FillFrom: KeyAccent, Corner: N(6)})
	for _, tile := range []string{"tile.network", "tile.volume", "tile.power"} {
		p.SetStyle("quicksettings", tile, StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6)})
		p.SetStyle("quicksettings", tile, StateActive, StyleDelta{
			FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(6),
		})
	}
	// Карточка уведомления: рамка задаёт важность, заливка приходит из палитры.
	p.SetStyle("notifications", "card.info", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6), PadX: N(10), PadY: N(4),
	})
	p.SetStyle("notifications", "card.warning", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6), PadX: N(10), PadY: N(4),
		Border: C(RGB(220, 150, 20)), BorderWidth: N(1),
	})
	p.SetStyle("notifications", "card.error", StateNormal, StyleDelta{
		Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6), PadX: N(10), PadY: N(4),
		Border: C(RGB(200, 60, 60)), BorderWidth: N(1),
	})
	p.SetStyle("notifications", "empty", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	p.SetStyle("notifications", "clear", StateNormal, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6)})
	p.SetStyle("calendar", "weekday", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})
	p.SetStyle("calendar", "day", StateNormal, StyleDelta{Corner: N(6)})
	p.SetStyle("calendar", "day", StateHover, StyleDelta{Fill: C(RGBA(128, 128, 128, 60)), Corner: N(6)})
	p.SetStyle("calendar", "day", StateActive, StyleDelta{
		FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(6),
	})
	p.SetStyle("calendar", "day", StateFocused, StyleDelta{
		BorderFrom: KeyAccent, BorderWidth: N(1), Corner: N(6),
	})
	p.SetStyle("calendar", "day", StateDisabled, StyleDelta{Text: C(RGBA(128, 128, 128, 200))})

	// У значка дока нет своей плашки: он лежит прямо на подложке дока, а
	// стиль по умолчанию дал бы ему заливку темы — белый прямоугольник.
	p.SetStyle("dock", "", StateNormal, StyleDelta{Fill: C(RGBA(0, 0, 0, 0))})

	// Подсветка под значком дока — радиальный градиент: свет расходится
	// кругом от значка, и осью такое не выразить. Ради этого случая
	// радиальный градиент и появился в стиле темы.
	p.SetStyle("dock", "", StateHover, StyleDelta{
		Gradient: []GradientStop{
			{Pos: 0, Color: RGBA(255, 255, 255, 90)},
			{Pos: 1, Color: RGBA(255, 255, 255, 0)},
		},
		GradientKind:   GK(GradientRadial),
		GradientRadius: N(1.1),
	})
	p.SetStyle("dock", "", StateActive, StyleDelta{
		Gradient: []GradientStop{
			{Pos: 0, Color: RGBA(255, 255, 255, 40)},
			{Pos: 1, Color: RGBA(255, 255, 255, 0)},
		},
		GradientKind:   GK(GradientRadial),
		GradientRadius: N(1.1),
	})
	// Элементы стеклянной полосы не носят своей заливки: она приходит из
	// общих токенов темы (surface) и ложится светлой капсулой поверх стекла.
	// Фон им даёт сама полоса, а собственный — только под курсором.
	//
	// SetStyle заменяет запись StyleDelta целиком, поэтому блок раньше писал
	// сюда Corner/PadX заново и затирал то, что выше уже задано для
	// startbutton/taskbutton (скругление 8/10 и отступы 10/6) — как и в
	// Windows10Profile/Windows11Profile, меняем только Fill через чтение
	// текущего стиля, а не переопределяем StyleDelta с нуля.
	clearFill := C(RGBA(0, 0, 0, 0))
	for _, comp := range []string{"startbutton", "taskbutton", "clock",
		"tray.network", "tray.volume", "tray.power", "tray.chevron"} {
		st := p.Styles[StyleKey{Component: comp, State: StateNormal}]
		st.Fill = clearFill
		p.SetStyle(comp, "", StateNormal, st)
	}
	// Отдельная строка для startbutton больше не нужна: она задавала ровно
	// то же самое (Fill/Text/Corner/PadX), что уже входит в цикл выше —
	// без переопределения Corner/PadX задавать её отдельно незачем.

	inheritTrayStyles(p)
	addDialogStyles(p)
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
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(40, 40, 42, 150),
			Highlight: RGBA(255, 255, 255, 40)},
	})
	p.SetStyle("startbutton", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("taskbutton", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("clock", "", StateNormal, StyleDelta{Text: C(text)})
	p.SetStyle("menu", "", StateNormal, StyleDelta{
		Backdrop: &BackdropSpec{
			Mode: BackdropBlur, Radius: 22, Tint: RGBA(50, 50, 52, 150),
			Highlight: RGBA(255, 255, 255, 40)},
		Text: C(text),
	})
	p.SetStyle("window", "", StateNormal, StyleDelta{Fill: C(surface)})
	p.SetStyle("window", "titlebar", StateNormal, StyleDelta{Fill: C(RGB(50, 50, 52)), Text: C(RGB(150, 150, 150))})
	p.SetStyle("window", "titlebar", StateFocused, StyleDelta{Fill: C(RGB(50, 50, 52)), Text: C(text)})
	addDialogStyles(p)
	return p
}
