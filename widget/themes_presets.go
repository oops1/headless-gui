// themes_presets.go — стиль отрисовки контролов и преднастроенные темы.
//
// Тема — это не только палитра: эпохи UI отличаются ФОРМОЙ контролов.
// ThemeStyle описывает геометрию/манеру отрисовки:
//
//   - Win10 — плоские прямоугольные контролы (ControlCorner=0);
//   - Win11 и Mac — скруглённые углы (ControlCorner>0);
//   - Win2000 — классический «объёмный» стиль: прямые углы и bevel-рамки
//     (выпуклые кнопки, утопленные поля ввода) — Classic3D=true.
//
// Преднастроенные комбинации: Win10 Dark/Light, Win11 Dark/Light, Win2000,
// Mac. Доступ по имени — ThemeByName, список — ThemeNames.
package widget

import (
	"image"
	"image/color"
)

// ThemeStyle — параметры отрисовки контролов (форма, а не цвета).
type ThemeStyle struct {
	Name string // имя пресета («Win11 Dark», «Win2000», …)

	// ControlCorner — радиус скругления углов контролов (Button, TextBox,
	// ComboBox, ProgressBar). 0 — прямые углы. Игнорируется при Classic3D.
	ControlCorner int

	// Classic3D — классическая объёмная отрисовка Win9x/2000: кнопки выпуклые
	// (bevel: светлая грань сверху/слева, тёмная снизу/справа), поля ввода
	// утопленные (sunken, грани инвертированы), углы прямые, hover-эффектов нет.
	Classic3D bool

	// Грани bevel для Classic3D.
	BevelLight  color.RGBA // светлая грань (внешняя верх/лево у выпуклых)
	BevelShadow color.RGBA // тёмная грань (низ/право)
	BevelDark   color.RGBA // самая тёмная (внутренняя грань тени)

	// WindowCorner — радиус скругления углов окна (widget.Window). 0 — острые.
	// Win11 ≈ 8, Mac ≈ 10. Применяется через Window.ApplyTheme.
	WindowCorner int

	// MacTitleBar — заголовок окна в стиле macOS (traffic lights слева, текст
	// по центру). Win-стиль при false. Применяется через Window.ApplyTheme.
	MacTitleBar bool
}

// currentStyle возвращает стиль активной темы (для Draw виджетов).
func currentStyle() ThemeStyle {
	if drawStyle != nil {
		return *drawStyle
	}
	return win10.Style
}

// CurrentThemeStyle — публичный доступ к стилю активной темы.
func CurrentThemeStyle() ThemeStyle { return currentStyle() }

// drawStyle — стиль, подменённый на время отрисовки поддерева с собственной
// темой (ThemeScope). nil — рисуется обычное дерево, стиль берётся из общей
// темы.
//
// Обычная переменная без блокировки: кадр рисуется в одной горутине —
// движок последовательно обходит дерево, оверлеи и модалки. Читать её из
// другой горутины некому: currentStyle зовётся только из Draw.
var drawStyle *ThemeStyle

// pushThemeStyle подменяет стиль отрисовки и возвращает функцию возврата.
//
// Возврат к ПРЕЖНЕМУ значению, а не к nil: области темы вкладываются друг в
// друга, и внутренняя не должна сбрасывать внешнюю.
func pushThemeStyle(st ThemeStyle) func() {
	prev := drawStyle
	drawStyle = &st
	return func() { drawStyle = prev }
}

// ─── Отрисовка bevel (Classic3D) ─────────────────────────────────────────────

// drawBevelRaised рисует выпуклую 2-пиксельную рамку Win2000 (кнопка).
func drawBevelRaised(ctx DrawContext, x, y, w, h int, st ThemeStyle) {
	// Внешняя: светлая верх/лево, самая тёмная низ/право.
	ctx.DrawHLine(x, y, w, st.BevelLight)
	ctx.DrawVLine(x, y, h, st.BevelLight)
	ctx.DrawHLine(x, y+h-1, w, st.BevelDark)
	ctx.DrawVLine(x+w-1, y, h, st.BevelDark)
	// Внутренняя: тень низ/право.
	ctx.DrawHLine(x+1, y+h-2, w-2, st.BevelShadow)
	ctx.DrawVLine(x+w-2, y+1, h-2, st.BevelShadow)
}

// drawSunkenRing рисует круглое «утопленное» кольцо (RadioButton Win2000):
// тёмная дуга сверху-слева, светлая снизу-справа.
func drawSunkenRing(ctx DrawContext, cx, cy, r int, dark, light color.RGBA) {
	rOut := float64(r) + 0.5
	rIn := float64(r) - 0.5
	for dy := -r - 1; dy <= r+1; dy++ {
		for dx := -r - 1; dx <= r+1; dx++ {
			d2 := float64(dx*dx + dy*dy)
			if d2 >= rIn*rIn && d2 <= rOut*rOut {
				col := light
				if dx+dy < 0 { // верх-лево
					col = dark
				}
				ctx.SetPixel(cx+dx, cy+dy, col)
			}
		}
	}
}

// drawBevelSunken рисует утопленную 2-пиксельную рамку Win2000
// (поле ввода, нажатая кнопка): грани инвертированы.
func drawBevelSunken(ctx DrawContext, x, y, w, h int, st ThemeStyle) {
	ctx.DrawHLine(x, y, w, st.BevelShadow)
	ctx.DrawVLine(x, y, h, st.BevelShadow)
	ctx.DrawHLine(x, y+h-1, w, st.BevelLight)
	ctx.DrawVLine(x+w-1, y, h, st.BevelLight)
	ctx.DrawHLine(x+1, y+1, w-2, st.BevelDark)
	ctx.DrawVLine(x+1, y+1, h-2, st.BevelDark)
}

// fillTitleBar заливает полосу заголовка: горизонтальный градиент
// TitleBG→TitleBG2 (классика Win2000: navy→голубой) либо сплошной цвет.
func fillTitleBar(ctx DrawContext, r image.Rectangle, bg color.RGBA) {
	fillTitleBarColors(ctx, r, bg, win10.TitleBG2)
}

// fillTitleBarColors — как fillTitleBar, но с явным вторым цветом градиента
// (для неактивного окна: серый градиент Win2000).
func fillTitleBarColors(ctx DrawContext, r image.Rectangle, bg, bg2 color.RGBA) {
	if bg2.A > 0 {
		drawLinearGradient(ctx, r, &LinearGradient{
			Horizontal: true,
			Stops:      []GradientStop{{Offset: 0, Color: bg}, {Offset: 1, Color: bg2}},
		})
		return
	}
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), bg)
}

// drawTitleText выводит текст заголовка: в классике — жирным (как Win2000).
func drawTitleText(ctx DrawContext, text string, x, y int, col color.RGBA) {
	if win10.Style.Classic3D {
		ctx.DrawTextFont(text, x, y, DefaultFontSizePt, BuiltinFontBold, col)
		return
	}
	ctx.DrawText(text, x, y, col)
}

// drawDottedRect рисует пунктирную рамку фокуса (классика Win2000).
func drawDottedRect(ctx DrawContext, x, y, w, h int, col color.RGBA) {
	for i := 0; i < w; i += 2 {
		ctx.SetPixel(x+i, y, col)
		ctx.SetPixel(x+i, y+h-1, col)
	}
	for i := 0; i < h; i += 2 {
		ctx.SetPixel(x, y+i, col)
		ctx.SetPixel(x+w-1, y+i, col)
	}
}

// drawArrowTri рисует маленький треугольник ▲ (up=true) или ▼ для кнопок
// классического скроллбара.
func drawArrowTri(ctx DrawContext, cx, cy int, up bool, col color.RGBA) {
	const h = 3
	for dy := 0; dy <= h; dy++ {
		y := cy - h/2 + dy
		var w int
		if up {
			w = dy
		} else {
			w = h - dy
		}
		ctx.FillRect(cx-w, y, 2*w+1, 1, col)
	}
}

// classicSBBtnH — высота кнопки-стрелки классического скроллбара
// (квадратная: равна ширине полосы).
func classicSBBtnH(trackW int) int { return trackW }

// classicSBInset возвращает «рабочую» область классического скроллбара —
// полосу между кнопками ▲ и ▼.
func classicSBInset(track image.Rectangle) image.Rectangle {
	btn := classicSBBtnH(track.Dx())
	in := image.Rect(track.Min.X, track.Min.Y+btn, track.Max.X, track.Max.Y-btn)
	if in.Dy() < 0 {
		in.Max.Y = in.Min.Y
	}
	return in
}

// sbWorkArea возвращает верх и высоту «рабочей» зоны вертикального скроллбара:
// в классике — полоса между кнопками ▲/▼, иначе — вся высота виджета.
func sbWorkArea(b image.Rectangle, sbw int) (top, h int) {
	top, h = b.Min.Y, b.Dy()
	if currentStyle().Classic3D {
		btn := classicSBBtnH(sbw)
		top += btn
		h -= 2 * btn
		if h < 8 {
			h = 8
		}
	}
	return top, h
}

// drawClassicScrollbar рисует классический вертикальный скроллбар Win2000:
// кнопки ▲/▼ на концах и выпуклый прямоугольный ползунок thumb.
func drawClassicScrollbar(ctx DrawContext, track, thumb image.Rectangle, st ThemeStyle, face, arrowCol color.RGBA) {
	btn := classicSBBtnH(track.Dx())
	top := image.Rect(track.Min.X, track.Min.Y, track.Max.X, track.Min.Y+btn)
	bot := image.Rect(track.Min.X, track.Max.Y-btn, track.Max.X, track.Max.Y)

	for _, r := range []image.Rectangle{top, bot} {
		ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), face)
		drawBevelRaised(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
	}
	drawArrowTri(ctx, top.Min.X+top.Dx()/2, top.Min.Y+top.Dy()/2, true, arrowCol)
	drawArrowTri(ctx, bot.Min.X+bot.Dx()/2, bot.Min.Y+bot.Dy()/2, false, arrowCol)

	if !thumb.Empty() {
		ctx.FillRect(thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), face)
		drawBevelRaised(ctx, thumb.Min.X, thumb.Min.Y, thumb.Dx(), thumb.Dy(), st)
	}
}

// ─── Преднастроенные темы ────────────────────────────────────────────────────

// Win10DarkTheme — Windows 10, тёмная (плоский стиль, прямые углы).
func Win10DarkTheme() *Theme {
	t := DarkTheme()
	t.Style = ThemeStyle{Name: "Win10 Dark"}
	return t
}

// Win10LightTheme — Windows 10, светлая (плоский стиль, прямые углы).
func Win10LightTheme() *Theme {
	t := LightTheme()
	t.Style = ThemeStyle{Name: "Win10 Light"}
	return t
}

// Win11DarkTheme — Windows 11, тёмная: скруглённые контролы (Fluent),
// смягчённая палитра Mica, голубой акцент.
func Win11DarkTheme() *Theme {
	t := DarkTheme()
	t.Style = ThemeStyle{Name: "Win11 Dark", ControlCorner: 6, WindowCorner: 12}

	t.WindowBG = color.RGBA{R: 32, G: 32, B: 32, A: 255}    // #202020 — Mica
	t.PanelBG = color.RGBA{R: 43, G: 43, B: 43, A: 255}     // #2B2B2B
	t.TitleBG = color.RGBA{R: 32, G: 32, B: 32, A: 255}     // безрамочный заголовок
	t.TitleText = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	t.Border = color.RGBA{R: 58, G: 58, B: 58, A: 255} // #3A3A3A

	t.BtnBG = color.RGBA{R: 45, G: 45, B: 45, A: 255}        // #2D2D2D
	t.BtnBorder = color.RGBA{R: 53, G: 53, B: 53, A: 255}    // #353535
	t.BtnHoverBG = color.RGBA{R: 50, G: 50, B: 50, A: 255}   // #323232
	t.BtnPressedBG = color.RGBA{R: 39, G: 39, B: 39, A: 255} // #272727
	t.BtnText = color.RGBA{R: 255, G: 255, B: 255, A: 255}

	t.InputBG = color.RGBA{R: 45, G: 45, B: 45, A: 255}
	t.InputBorder = color.RGBA{R: 56, G: 56, B: 56, A: 255}
	t.InputFocus = color.RGBA{R: 76, G: 194, B: 255, A: 255} // #4CC2FF

	t.Accent = color.RGBA{R: 76, G: 194, B: 255, A: 255}       // #4CC2FF — Win11 accent
	t.ProgressFill = t.Accent
	t.SliderFill = t.Accent
	t.ToggleOnBG = t.Accent
	t.InputCaret = t.Accent
	t.DropItemBG = color.RGBA{R: 0, G: 95, B: 184, A: 220}     // #005FB8
	t.ListItemSelect = color.RGBA{R: 0, G: 95, B: 184, A: 180} // #005FB8
	t.MenuHoverBG = color.RGBA{R: 55, G: 55, B: 55, A: 255}    // #373737
	t.MenuHoverText = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	t.SplitterHoverBG = t.Accent
	t.StatusBarBG = color.RGBA{R: 43, G: 43, B: 43, A: 255}
	t.StatusBarText = color.RGBA{R: 230, G: 230, B: 230, A: 255}
	t.TabContentBG = t.WindowBG
	t.TabActiveBG = t.WindowBG
	t.TabBG = t.PanelBG
	t.DialogTitleBG = t.PanelBG
	return t
}

// Win11LightTheme — Windows 11, светлая: скруглённые контролы, акцент #005FB8.
func Win11LightTheme() *Theme {
	t := LightTheme()
	t.Style = ThemeStyle{Name: "Win11 Light", ControlCorner: 6, WindowCorner: 12}

	t.WindowBG = color.RGBA{R: 243, G: 243, B: 243, A: 255} // #F3F3F3 — Mica
	t.PanelBG = color.RGBA{R: 251, G: 251, B: 251, A: 255}  // #FBFBFB
	t.TitleBG = color.RGBA{R: 243, G: 243, B: 243, A: 255}
	t.Border = color.RGBA{R: 229, G: 229, B: 229, A: 255} // #E5E5E5

	t.BtnBG = color.RGBA{R: 251, G: 251, B: 251, A: 255}
	t.BtnBorder = color.RGBA{R: 229, G: 229, B: 229, A: 255}
	t.BtnHoverBG = color.RGBA{R: 246, G: 246, B: 246, A: 255}
	t.BtnPressedBG = color.RGBA{R: 240, G: 240, B: 240, A: 255}

	t.Accent = color.RGBA{R: 0, G: 95, B: 184, A: 255} // #005FB8 — Win11 accent
	t.InputFocus = t.Accent
	t.ProgressFill = t.Accent
	t.SliderFill = t.Accent
	t.ToggleOnBG = t.Accent
	t.SplitterHoverBG = t.Accent
	return t
}

// Win2000Theme — классическая Windows 2000: серебристая палитра, прямые углы,
// объёмные bevel-рамки, тёмно-синие заголовок/выделение, без hover-эффектов.
func Win2000Theme() *Theme {
	face := color.RGBA{R: 212, G: 208, B: 200, A: 255}   // #D4D0C8 — button face
	navy := color.RGBA{R: 10, G: 36, B: 106, A: 255}     // #0A246A — active title
	black := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}   // #808080

	return &Theme{
		Style: ThemeStyle{
			Name:        "Win2000",
			Classic3D:   true,
			BevelLight:  white,
			BevelShadow: gray,
			BevelDark:   color.RGBA{R: 64, G: 64, B: 64, A: 255}, // #404040
		},

		WindowBG:    face,
		PanelBG:     face,
		TitleBG:     navy,
		TitleBG2:    color.RGBA{R: 166, G: 202, B: 240, A: 255}, // #A6CAF0 — градиент заголовка
		TitleText:   white,
		// Классика: неактивное окно — серый градиент #808080→#C0C0C0,
		// текст — серебристый (как в настоящей Windows 2000).
		TitleBGInactive:   color.RGBA{R: 128, G: 128, B: 128, A: 255},
		TitleBG2Inactive:  color.RGBA{R: 192, G: 192, B: 192, A: 255},
		TitleTextInactive: color.RGBA{R: 212, G: 208, B: 200, A: 255}, // #D4D0C8
		Border:            gray,
		ShadowColor:       color.RGBA{R: 0, G: 0, B: 0, A: 60},

		BtnBG:        face,
		BtnBorder:    gray,
		BtnHoverBG:   face, // классика: hover не меняет фон
		BtnPressedBG: face, // нажатие показывается инверсией bevel
		BtnText:      black,

		InputBG:          white,
		InputBorder:      gray,
		InputFocus:       navy,
		InputText:        black,
		InputCaret:       black,
		InputPlaceholder: gray,

		LabelText: black,
		LabelBG:   color.RGBA{},

		ProgressBG:   white,
		ProgressFill: navy,

		DropBG:     white,
		DropBorder: black,
		DropText:   black,
		DropArrow:  black,
		DropItemBG: navy,

		MenuHoverBG:   navy,  // классика: подсветка пункта меню — navy
		MenuHoverText: white, // …с белым текстом
		MenuBG:        face,  // меню классики — на «лице», не белые

		CheckBG:      white,
		CheckBorder:  gray,
		CheckMark:    black,
		CheckHoverBG: white,
		CheckText:    black,

		TabBG:         face,
		TabActiveBG:   face,
		TabBorder:     gray,
		TabText:       black,
		TabActiveText: black,
		TabContentBG:  face,

		SliderTrackBG: white,
		SliderFill:    navy,
		SliderThumb:   face,
		SliderBorder:  gray,

		ToggleBG:     white,
		ToggleOnBG:   navy,
		ToggleThumb:  face,
		ToggleBorder: gray,

		ScrollTrackBG:  color.RGBA{R: 232, G: 228, B: 220, A: 255},
		ScrollThumbBG:  face,
		ListItemHover:  color.RGBA{R: 182, G: 189, B: 210, A: 255}, // #B6BDD2
		ListItemSelect: navy,

		TreeText:  black,
		TreeArrow: black,

		DialogBG:      face,
		DialogTitleBG: navy,
		DialogDim:     color.RGBA{R: 0, G: 0, B: 0, A: 90},

		SplitterBG:      face,
		SplitterHoverBG: gray,

		StatusBarBG:   face,
		StatusBarText: black,

		HeaderBG:   face,
		HeaderText: black,

		Accent:    navy,
		Scrollbar: face,
		Disabled:  gray,
	}
}

// MacTheme — macOS (светлая): мягкая серая палитра, сильное скругление,
// синий акцент #007AFF, зелёный ToggleSwitch (#34C759).
func MacTheme() *Theme {
	t := LightTheme()
	t.Style = ThemeStyle{Name: "Mac", ControlCorner: 8, WindowCorner: 10, MacTitleBar: true}

	t.WindowBG = color.RGBA{R: 236, G: 236, B: 236, A: 255} // #ECECEC
	t.PanelBG = color.RGBA{R: 246, G: 246, B: 246, A: 255}  // #F6F6F6
	t.TitleBG = color.RGBA{R: 232, G: 232, B: 232, A: 255}  // #E8E8E8
	t.TitleText = color.RGBA{R: 58, G: 58, B: 58, A: 255}
	t.Border = color.RGBA{R: 209, G: 209, B: 209, A: 255} // #D1D1D1

	t.BtnBG = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	t.BtnBorder = color.RGBA{R: 208, G: 208, B: 208, A: 255}
	t.BtnHoverBG = color.RGBA{R: 245, G: 245, B: 245, A: 255}
	t.BtnPressedBG = color.RGBA{R: 232, G: 232, B: 232, A: 255}
	t.BtnText = color.RGBA{R: 26, G: 26, B: 26, A: 255}

	t.Accent = color.RGBA{R: 0, G: 122, B: 255, A: 255} // #007AFF
	t.InputFocus = t.Accent
	t.InputCaret = t.Accent
	t.ProgressFill = t.Accent
	t.SliderFill = t.Accent
	t.SplitterHoverBG = t.Accent
	t.ToggleOnBG = color.RGBA{R: 52, G: 199, B: 89, A: 255} // #34C759 — зелёный mac-switch
	t.DropItemBG = color.RGBA{R: 0, G: 122, B: 255, A: 230}
	t.MenuHoverBG = color.RGBA{R: 0, G: 122, B: 255, A: 255} // mac: синяя подсветка меню
	t.MenuHoverText = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	t.MenuBG = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	t.ListItemHover = color.RGBA{R: 240, G: 240, B: 240, A: 255}
	t.ListItemSelect = color.RGBA{R: 179, G: 215, B: 255, A: 255} // #B3D7FF
	t.StatusBarBG = color.RGBA{R: 236, G: 236, B: 236, A: 255}
	t.StatusBarText = color.RGBA{R: 70, G: 70, B: 70, A: 255}
	t.DialogTitleBG = t.TitleBG
	return t
}

// ─── Реестр пресетов ─────────────────────────────────────────────────────────

var themePresets = []struct {
	Name string
	New  func() *Theme
}{
	{"Win10 Dark", Win10DarkTheme},
	{"Win10 Light", Win10LightTheme},
	{"Win11 Dark", Win11DarkTheme},
	{"Win11 Light", Win11LightTheme},
	{"Win2000", Win2000Theme},
	{"Mac", MacTheme},
}

// ThemeNames возвращает имена преднастроенных тем (в фиксированном порядке).
func ThemeNames() []string {
	out := make([]string, len(themePresets))
	for i, p := range themePresets {
		out[i] = p.Name
	}
	return out
}

// ThemeByName возвращает свежую копию преднастроенной темы по имени
// (без учёта регистра) или nil, если темы с таким именем нет.
// Алиасы: «Dark» → Win10 Dark, «Light» → Win10 Light (базовые темы).
func ThemeByName(name string) *Theme {
	switch {
	case equalFoldASCII(name, "Dark"):
		return Win10DarkTheme()
	case equalFoldASCII(name, "Light"):
		return Win10LightTheme()
	}
	for _, p := range themePresets {
		if equalFoldASCII(p.Name, name) {
			return p.New()
		}
	}
	return nil
}

// contrastText возвращает чёрный или белый текст в зависимости от яркости
// фона bg (для читаемости подписи на цветной подсветке/выделении).
func contrastText(bg color.RGBA) color.RGBA {
	// Воспринимаемая яркость (Rec. 601), 0..255.
	lum := (299*int(bg.R) + 587*int(bg.G) + 114*int(bg.B)) / 1000
	if lum > 140 {
		return color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}
	return color.RGBA{R: 255, G: 255, B: 255, A: 255}
}

// equalFoldASCII — сравнение без учёта регистра (ASCII достаточно для имён тем).
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
