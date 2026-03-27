// Package widget — определения тем и переключение тем.
package widget

import "image/color"

// Theme содержит полную цветовую палитру виджетов.
type Theme struct {
	// Окна / панели
	WindowBG    color.RGBA
	PanelBG     color.RGBA
	TitleBG     color.RGBA
	TitleText   color.RGBA
	Border      color.RGBA
	ShadowColor color.RGBA

	// Кнопки
	BtnBG        color.RGBA
	BtnBorder    color.RGBA
	BtnHoverBG   color.RGBA
	BtnPressedBG color.RGBA
	BtnText      color.RGBA

	// Текстовые поля
	InputBG          color.RGBA
	InputBorder      color.RGBA
	InputFocus       color.RGBA
	InputText        color.RGBA
	InputCaret       color.RGBA
	InputPlaceholder color.RGBA

	// Метки
	LabelText color.RGBA
	LabelBG   color.RGBA

	// Прогресс-бар
	ProgressBG   color.RGBA
	ProgressFill color.RGBA

	// Выпадающий список
	DropBG     color.RGBA
	DropBorder color.RGBA
	DropText   color.RGBA
	DropArrow  color.RGBA
	DropItemBG color.RGBA

	// CheckBox / RadioButton
	CheckBG       color.RGBA // фон квадратика/кружка
	CheckBorder   color.RGBA // рамка
	CheckMark     color.RGBA // галочка / точка
	CheckHoverBG  color.RGBA // фон при hover
	CheckText     color.RGBA // текст метки

	// TabControl
	TabBG         color.RGBA // фон неактивной вкладки
	TabActiveBG   color.RGBA // фон активной вкладки
	TabBorder     color.RGBA // рамка
	TabText       color.RGBA // текст вкладки
	TabActiveText color.RGBA // текст активной вкладки
	TabContentBG  color.RGBA // фон области содержимого

	// Slider
	SliderTrackBG color.RGBA // фон дорожки
	SliderFill    color.RGBA // заполненная часть
	SliderThumb   color.RGBA // ползунок
	SliderBorder  color.RGBA // рамка дорожки

	// ToggleSwitch
	ToggleBG     color.RGBA // фон выключенного переключателя
	ToggleOnBG   color.RGBA // фон включённого
	ToggleThumb  color.RGBA // кружок
	ToggleBorder color.RGBA // рамка

	// ScrollView / ListView
	ScrollTrackBG  color.RGBA // фон трека скроллбара
	ScrollThumbBG  color.RGBA // ползунок скроллбара
	ListItemHover  color.RGBA // hover по элементу списка
	ListItemSelect color.RGBA // выделенный элемент списка

	// Системные
	Accent    color.RGBA
	Scrollbar color.RGBA
	Disabled  color.RGBA
}

// DarkTheme возвращает тему Windows 10 Dark Mode.
func DarkTheme() *Theme {
	return &Theme{
		WindowBG:    color.RGBA{R: 32, G: 32, B: 32, A: 245},
		PanelBG:     color.RGBA{R: 43, G: 43, B: 43, A: 220},
		TitleBG:     color.RGBA{R: 0, G: 120, B: 215, A: 255},
		TitleText:   color.RGBA{R: 255, G: 255, B: 255, A: 255},
		Border:      color.RGBA{R: 76, G: 76, B: 76, A: 255},
		ShadowColor: color.RGBA{R: 0, G: 0, B: 0, A: 80},

		BtnBG:        color.RGBA{R: 58, G: 58, B: 58, A: 255},
		BtnBorder:    color.RGBA{R: 100, G: 100, B: 100, A: 255},
		BtnHoverBG:   color.RGBA{R: 80, G: 130, B: 200, A: 255},
		BtnPressedBG: color.RGBA{R: 0, G: 84, B: 153, A: 255},
		BtnText:      color.RGBA{R: 240, G: 240, B: 240, A: 255},

		InputBG:          color.RGBA{R: 25, G: 25, B: 25, A: 255},
		InputBorder:      color.RGBA{R: 100, G: 100, B: 100, A: 255},
		InputFocus:       color.RGBA{R: 0, G: 120, B: 215, A: 255},
		InputText:        color.RGBA{R: 220, G: 220, B: 220, A: 255},
		InputCaret:       color.RGBA{R: 0, G: 120, B: 215, A: 255},
		InputPlaceholder: color.RGBA{R: 120, G: 120, B: 120, A: 255},

		LabelText: color.RGBA{R: 220, G: 220, B: 220, A: 255},
		LabelBG:   color.RGBA{R: 0, G: 0, B: 0, A: 0},

		ProgressBG:   color.RGBA{R: 30, G: 30, B: 30, A: 255},
		ProgressFill: color.RGBA{R: 0, G: 120, B: 215, A: 255},

		DropBG:     color.RGBA{R: 58, G: 58, B: 58, A: 255},
		DropBorder: color.RGBA{R: 100, G: 100, B: 100, A: 255},
		DropText:   color.RGBA{R: 220, G: 220, B: 220, A: 255},
		DropArrow:  color.RGBA{R: 180, G: 180, B: 180, A: 255},
		DropItemBG: color.RGBA{R: 0, G: 120, B: 215, A: 200},

		CheckBG:      color.RGBA{R: 25, G: 25, B: 25, A: 255},
		CheckBorder:  color.RGBA{R: 140, G: 140, B: 140, A: 255},
		CheckMark:    color.RGBA{R: 255, G: 255, B: 255, A: 255},
		CheckHoverBG: color.RGBA{R: 45, G: 45, B: 45, A: 255},
		CheckText:    color.RGBA{R: 220, G: 220, B: 220, A: 255},

		TabBG:         color.RGBA{R: 45, G: 45, B: 45, A: 255},
		TabActiveBG:   color.RGBA{R: 32, G: 32, B: 32, A: 255},
		TabBorder:     color.RGBA{R: 76, G: 76, B: 76, A: 255},
		TabText:       color.RGBA{R: 160, G: 160, B: 160, A: 255},
		TabActiveText: color.RGBA{R: 255, G: 255, B: 255, A: 255},
		TabContentBG:  color.RGBA{R: 32, G: 32, B: 32, A: 255},

		SliderTrackBG: color.RGBA{R: 55, G: 55, B: 55, A: 255},
		SliderFill:    color.RGBA{R: 0, G: 120, B: 215, A: 255},
		SliderThumb:   color.RGBA{R: 220, G: 220, B: 220, A: 255},
		SliderBorder:  color.RGBA{R: 76, G: 76, B: 76, A: 255},

		ToggleBG:     color.RGBA{R: 55, G: 55, B: 55, A: 255},
		ToggleOnBG:   color.RGBA{R: 0, G: 120, B: 215, A: 255},
		ToggleThumb:  color.RGBA{R: 255, G: 255, B: 255, A: 255},
		ToggleBorder: color.RGBA{R: 140, G: 140, B: 140, A: 255},

		ScrollTrackBG:  color.RGBA{R: 40, G: 40, B: 40, A: 255},
		ScrollThumbBG:  color.RGBA{R: 80, G: 80, B: 80, A: 255},
		ListItemHover:  color.RGBA{R: 55, G: 55, B: 55, A: 255},
		ListItemSelect: color.RGBA{R: 0, G: 120, B: 215, A: 100},

		Accent:    color.RGBA{R: 0, G: 120, B: 215, A: 255},
		Scrollbar: color.RGBA{R: 80, G: 80, B: 80, A: 255},
		Disabled:  color.RGBA{R: 100, G: 100, B: 100, A: 255},
	}
}

// LightTheme возвращает тему Windows 10 Light Mode.
func LightTheme() *Theme {
	return &Theme{
		WindowBG:    color.RGBA{R: 243, G: 243, B: 243, A: 255},
		PanelBG:     color.RGBA{R: 255, G: 255, B: 255, A: 240},
		TitleBG:     color.RGBA{R: 0, G: 120, B: 215, A: 255},
		TitleText:   color.RGBA{R: 255, G: 255, B: 255, A: 255},
		Border:      color.RGBA{R: 200, G: 200, B: 200, A: 255},
		ShadowColor: color.RGBA{R: 0, G: 0, B: 0, A: 30},

		BtnBG:        color.RGBA{R: 225, G: 225, B: 225, A: 255},
		BtnBorder:    color.RGBA{R: 180, G: 180, B: 180, A: 255},
		BtnHoverBG:   color.RGBA{R: 200, G: 220, B: 240, A: 255},
		BtnPressedBG: color.RGBA{R: 0, G: 84, B: 153, A: 255},
		BtnText:      color.RGBA{R: 30, G: 30, B: 30, A: 255},

		InputBG:          color.RGBA{R: 255, G: 255, B: 255, A: 255},
		InputBorder:      color.RGBA{R: 180, G: 180, B: 180, A: 255},
		InputFocus:       color.RGBA{R: 0, G: 120, B: 215, A: 255},
		InputText:        color.RGBA{R: 30, G: 30, B: 30, A: 255},
		InputCaret:       color.RGBA{R: 0, G: 120, B: 215, A: 255},
		InputPlaceholder: color.RGBA{R: 160, G: 160, B: 160, A: 255},

		LabelText: color.RGBA{R: 30, G: 30, B: 30, A: 255},
		LabelBG:   color.RGBA{R: 0, G: 0, B: 0, A: 0},

		ProgressBG:   color.RGBA{R: 210, G: 210, B: 210, A: 255},
		ProgressFill: color.RGBA{R: 0, G: 120, B: 215, A: 255},

		DropBG:     color.RGBA{R: 250, G: 250, B: 250, A: 255},
		DropBorder: color.RGBA{R: 180, G: 180, B: 180, A: 255},
		DropText:   color.RGBA{R: 30, G: 30, B: 30, A: 255},
		DropArrow:  color.RGBA{R: 100, G: 100, B: 100, A: 255},
		DropItemBG: color.RGBA{R: 0, G: 120, B: 215, A: 200},

		CheckBG:      color.RGBA{R: 255, G: 255, B: 255, A: 255},
		CheckBorder:  color.RGBA{R: 140, G: 140, B: 140, A: 255},
		CheckMark:    color.RGBA{R: 255, G: 255, B: 255, A: 255},
		CheckHoverBG: color.RGBA{R: 240, G: 240, B: 240, A: 255},
		CheckText:    color.RGBA{R: 30, G: 30, B: 30, A: 255},

		TabBG:         color.RGBA{R: 235, G: 235, B: 235, A: 255},
		TabActiveBG:   color.RGBA{R: 255, G: 255, B: 255, A: 255},
		TabBorder:     color.RGBA{R: 200, G: 200, B: 200, A: 255},
		TabText:       color.RGBA{R: 100, G: 100, B: 100, A: 255},
		TabActiveText: color.RGBA{R: 30, G: 30, B: 30, A: 255},
		TabContentBG:  color.RGBA{R: 255, G: 255, B: 255, A: 255},

		SliderTrackBG: color.RGBA{R: 210, G: 210, B: 210, A: 255},
		SliderFill:    color.RGBA{R: 0, G: 120, B: 215, A: 255},
		SliderThumb:   color.RGBA{R: 0, G: 120, B: 215, A: 255},
		SliderBorder:  color.RGBA{R: 180, G: 180, B: 180, A: 255},

		ToggleBG:     color.RGBA{R: 210, G: 210, B: 210, A: 255},
		ToggleOnBG:   color.RGBA{R: 0, G: 120, B: 215, A: 255},
		ToggleThumb:  color.RGBA{R: 255, G: 255, B: 255, A: 255},
		ToggleBorder: color.RGBA{R: 160, G: 160, B: 160, A: 255},

		ScrollTrackBG:  color.RGBA{R: 240, G: 240, B: 240, A: 255},
		ScrollThumbBG:  color.RGBA{R: 200, G: 200, B: 200, A: 255},
		ListItemHover:  color.RGBA{R: 230, G: 230, B: 230, A: 255},
		ListItemSelect: color.RGBA{R: 0, G: 120, B: 215, A: 80},

		Accent:    color.RGBA{R: 0, G: 120, B: 215, A: 255},
		Scrollbar: color.RGBA{R: 180, G: 180, B: 180, A: 255},
		Disabled:  color.RGBA{R: 160, G: 160, B: 160, A: 255},
	}
}

// Themeable — виджет, поддерживающий применение темы.
type Themeable interface {
	ApplyTheme(t *Theme)
}

// ApplyGlobalTheme обновляет глобальные цвета по умолчанию (используются в New*-конструкторах).
// Вызывается engine.SetTheme перед рекурсивным обходом дерева виджетов.
func ApplyGlobalTheme(t *Theme) {
	win10.WindowBG = t.WindowBG
	win10.PanelBG = t.PanelBG
	win10.TitleBG = t.TitleBG
	win10.TitleText = t.TitleText
	win10.Border = t.Border
	win10.ShadowColor = t.ShadowColor
	win10.BtnBG = t.BtnBG
	win10.BtnBorder = t.BtnBorder
	win10.BtnHoverBG = t.BtnHoverBG
	win10.BtnPressedBG = t.BtnPressedBG
	win10.BtnText = t.BtnText
	win10.InputBG = t.InputBG
	win10.InputBorder = t.InputBorder
	win10.InputFocus = t.InputFocus
	win10.InputText = t.InputText
	win10.InputCaret = t.InputCaret
	win10.InputPlaceholder = t.InputPlaceholder
	win10.LabelText = t.LabelText
	win10.LabelBG = t.LabelBG
	win10.ProgressBG = t.ProgressBG
	win10.ProgressFill = t.ProgressFill
	win10.DropBG = t.DropBG
	win10.DropBorder = t.DropBorder
	win10.DropText = t.DropText
	win10.DropArrow = t.DropArrow
	win10.DropItemBG = t.DropItemBG
	win10.CheckBG = t.CheckBG
	win10.CheckBorder = t.CheckBorder
	win10.CheckMark = t.CheckMark
	win10.CheckHoverBG = t.CheckHoverBG
	win10.CheckText = t.CheckText
	win10.TabBG = t.TabBG
	win10.TabActiveBG = t.TabActiveBG
	win10.TabBorder = t.TabBorder
	win10.TabText = t.TabText
	win10.TabActiveText = t.TabActiveText
	win10.TabContentBG = t.TabContentBG
	win10.SliderTrackBG = t.SliderTrackBG
	win10.SliderFill = t.SliderFill
	win10.SliderThumb = t.SliderThumb
	win10.SliderBorder = t.SliderBorder
	win10.ToggleBG = t.ToggleBG
	win10.ToggleOnBG = t.ToggleOnBG
	win10.ToggleThumb = t.ToggleThumb
	win10.ToggleBorder = t.ToggleBorder
	win10.ScrollTrackBG = t.ScrollTrackBG
	win10.ScrollThumbBG = t.ScrollThumbBG
	win10.ListItemHover = t.ListItemHover
	win10.ListItemSelect = t.ListItemSelect
	win10.Accent = t.Accent
	win10.Scrollbar = t.Scrollbar
	win10.Disabled = t.Disabled
}
