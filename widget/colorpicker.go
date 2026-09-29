package widget

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"sync"
)

// colorpicker.go — поле цвета с выпадающей палитрой (пара к DatePicker).
//
// Цвет в настройках — это всегда одно и то же: образец, код `#RRGGBB` и
// несколько готовых цветов рядом. Пока такого поля не было, приложение
// набирало код в обычный TextBox и ставило рядом панель нужного цвета —
// человек правил тему, не видя, что получится, а опечатка в коде оборачивалась
// невидимой ошибкой.
//
// Устройство то же, что у DatePicker: значение можно набрать с клавиатуры
// прямо в поле, а можно раскрыть палитру и выбрать образец или подвинуть
// каналы. Набранное применяется по Enter и откатывается по Esc — до тех пор
// поле показывает набираемый текст, а не значение.
//
// Прозрачность не поддерживается намеренно: цвет интерфейса — это цвет, а не
// плёнка поверх соседа. Полупрозрачные образцы читаются по-разному на светлом
// и тёмном фоне, и «тот же самый» цвет в двух местах выглядел бы разным.

// ColorPicker — поле цвета с выпадающей палитрой.
type ColorPicker struct {
	Base
	mu sync.Mutex

	value   color.RGBA
	palette []color.RGBA

	open    bool
	focused bool

	text      []rune // набираемый код; актуален, пока editing
	editing   bool
	selectAll bool // первый символ заменяет весь текст
	invalid   bool // набранное не разобралось

	hoverSwatch int // образец под мышью; -1 — нет
	cursor      int // образец под клавиатурным курсором
	dragChannel int // канал, который тянут мышью (0..2); -1 — не тянут
	hoverBand   int // канал под мышью; -1 — нет

	// FontSize — кегль поля и палитры; 0 — общий размер.
	FontSize float64

	pal      cpPalette
	commands map[string]ICommand

	// OnChanged вызывается при смене цвета — и выбором, и набором кода.
	// Зовётся без замков контрола.
	OnChanged func(c color.RGBA)
}

// cpPalette — цвета поля и выпадающей палитры из темы.
type cpPalette struct {
	bg, border, focus, text, sel, err          color.RGBA
	dropBG, dropBorder, dropText, muted, glyph color.RGBA
	hoverBG, accent                            color.RGBA
}

// DefaultColorPalette — образцы по умолчанию: восемь оттенков серого и два
// ряда основных тонов.
//
// Набор небольшой намеренно: палитра нужна, чтобы быстро взять «примерно
// такой» цвет, а точный подбирают каналами. Своя палитра — SetPalette.
func DefaultColorPalette() []color.RGBA {
	hex := []uint32{
		0x000000, 0x1F1F1F, 0x3C3C3C, 0x5A5A5A, 0x808080, 0xA6A6A6, 0xD4D4D4, 0xFFFFFF,
		0xC42B1C, 0xE81123, 0xF7630C, 0xFFB900, 0x107C10, 0x00B294, 0x0078D7, 0x8764B8,
		0x744DA9, 0xB146C2, 0xE3008C, 0x877B70, 0x486860, 0x2D7D9A, 0x004E8C, 0x4A5459,
	}
	out := make([]color.RGBA, 0, len(hex))
	for _, h := range hex {
		out = append(out, dvRGB(h))
	}
	return out
}

// cpPaletteCols — сколько образцов в ряду. Восемь: ряд серых задаёт ширину.
const cpPaletteCols = 8

// NewColorPicker создаёт поле с чёрным цветом и палитрой по умолчанию.
func NewColorPicker() *ColorPicker {
	p := &ColorPicker{
		value:       color.RGBA{A: 255},
		palette:     DefaultColorPalette(),
		hoverSwatch: -1,
		hoverBand:   -1,
		dragChannel: -1,
	}
	if t := CurrentTheme(); t != nil {
		p.pal = cpPaletteFrom(t)
	} else {
		p.pal = cpPaletteFrom(Win11LightTheme())
	}
	return p
}

// Value возвращает выбранный цвет.
func (p *ColorPicker) Value() color.RGBA {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.value
}

// SetValue задаёт цвет. Возвращает false, если цвет тот же.
//
// Непрозрачность выставляется сама: поле работает с цветом, а не с плёнкой
// (см. комментарий к файлу), и цвет с нулевой альфой означал бы «невидимо».
func (p *ColorPicker) SetValue(c color.RGBA) bool {
	return p.setValue(c, true)
}

// SetValueQuiet задаёт цвет БЕЗ OnChanged и ValueChangedCommand.
//
// Нужен тому, кто расставляет цвета сам: в окне настроек пустое поле значит
// «берём из системной темы», и показанный там цвет темы немедленно вернулся
// бы обратно как «человек выбрал цвет» — пустое значение превратилось бы в
// заданное. То же при смене темы и при сбросе. Пара к SetValue — как
// SetSelectedIndexQuiet у таблицы.
func (p *ColorPicker) SetValueQuiet(c color.RGBA) bool {
	return p.setValue(c, false)
}

// setValue — общий путь обоих сеттеров: notify решает, узнает ли об этом
// приложение.
func (p *ColorPicker) setValue(c color.RGBA, notify bool) bool {
	c.A = 255
	p.mu.Lock()
	if p.value == c {
		p.mu.Unlock()
		return false
	}
	p.value = c
	p.editing, p.invalid = false, false
	p.text = nil
	cb, cmd := p.OnChanged, p.commands["ValueChangedCommand"]
	p.mu.Unlock()

	p.Invalidate()
	if !notify {
		return true
	}
	if cb != nil {
		cb(c)
	}
	if cmd != nil && cmd.CanExecute(c) {
		cmd.Execute(c)
	}
	return true
}

// SetPalette задаёт свои образцы. Пустой список возвращает набор по умолчанию:
// палитра без образцов — это выпадающий список, из которого нечего выбрать.
func (p *ColorPicker) SetPalette(cols []color.RGBA) {
	p.mu.Lock()
	if len(cols) == 0 {
		p.palette = DefaultColorPalette()
	} else {
		p.palette = append([]color.RGBA(nil), cols...)
		for i := range p.palette {
			p.palette[i].A = 255
		}
	}
	p.cursor = 0
	p.mu.Unlock()
	p.Invalidate()
}

// Palette возвращает текущие образцы.
func (p *ColorPicker) Palette() []color.RGBA {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]color.RGBA(nil), p.palette...)
}

// Text возвращает то, что показано в поле: набираемый код или код значения.
func (p *ColorPicker) Text() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.textLocked()
}

func (p *ColorPicker) textLocked() string {
	if p.editing {
		return string(p.text)
	}
	return HexColor(p.value)
}

// IsDropDownOpen сообщает, раскрыта ли палитра.
func (p *ColorPicker) IsDropDownOpen() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.open
}

// SetDropDownOpen раскрывает или закрывает палитру.
func (p *ColorPicker) SetDropDownOpen(v bool) {
	p.mu.Lock()
	changed := p.setOpenLocked(v)
	p.mu.Unlock()
	if changed {
		// Палитра лежит ВНЕ границ поля: пометить к перерисовке одно поле
		// мало — там, где палитра появилась или исчезла, кадр не обновится.
		notifyUIChanged()
	}
}

// setOpenLocked меняет состояние палитры. Вызывать под p.mu.
func (p *ColorPicker) setOpenLocked(v bool) bool {
	if p.open == v {
		return false
	}
	p.open = v
	if v {
		// Курсор встаёт на образец, совпадающий со значением: стрелками
		// человек продолжает с того места, где он сейчас.
		p.cursor = 0
		for i, c := range p.palette {
			if c == p.value {
				p.cursor = i
				break
			}
		}
	} else {
		p.hoverSwatch, p.hoverBand, p.dragChannel = -1, -1, -1
	}
	return true
}

// commitLocked применяет набранный код. Вызывать под p.mu; сам колбэк
// отправляется наружу вызывающим (см. commit).
func (p *ColorPicker) commitLocked() (color.RGBA, bool) {
	if !p.editing {
		return p.value, false
	}
	c, ok := ParseHexColor(string(p.text))
	if !ok {
		p.invalid = true
		return p.value, false
	}
	p.editing, p.invalid = false, false
	p.text = nil
	if p.value == c {
		return c, false
	}
	p.value = c
	return c, true
}

// commit применяет набранное и уведомляет подписчиков.
func (p *ColorPicker) commit() {
	p.mu.Lock()
	c, changed := p.commitLocked()
	cb, cmd := p.OnChanged, p.commands["ValueChangedCommand"]
	p.mu.Unlock()
	p.Invalidate()
	if !changed {
		return
	}
	if cb != nil {
		cb(c)
	}
	if cmd != nil && cmd.CanExecute(c) {
		cmd.Execute(c)
	}
}

// revertLocked отбрасывает набранное. Вызывать под p.mu.
func (p *ColorPicker) revertLocked() {
	p.editing, p.invalid, p.selectAll = false, false, false
	p.text = nil
}

// setChannel меняет один канал цвета: 0 — красный, 1 — зелёный, 2 — синий.
func (p *ColorPicker) setChannel(ch int, v int) {
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	p.mu.Lock()
	c := p.value
	switch ch {
	case 0:
		c.R = uint8(v)
	case 1:
		c.G = uint8(v)
	case 2:
		c.B = uint8(v)
	}
	p.mu.Unlock()
	p.SetValue(c)
}

// channel возвращает значение канала цвета.
func cpChannel(c color.RGBA, ch int) int {
	switch ch {
	case 0:
		return int(c.R)
	case 1:
		return int(c.G)
	case 2:
		return int(c.B)
	}
	return 0
}

// SetCommand подключает команду: ValueChangedCommand получает color.RGBA.
func (p *ColorPicker) SetCommand(name string, cmd ICommand) bool {
	if name != "ValueChangedCommand" {
		return false
	}
	p.mu.Lock()
	if p.commands == nil {
		p.commands = map[string]ICommand{}
	}
	p.commands[name] = cmd
	p.mu.Unlock()
	return true
}

// ApplyTheme обновляет цвета поля и палитры.
func (p *ColorPicker) ApplyTheme(t *Theme) {
	p.mu.Lock()
	p.pal = cpPaletteFrom(t)
	p.mu.Unlock()
}

func cpPaletteFrom(t *Theme) cpPalette {
	or := func(c, def color.RGBA) color.RGBA {
		if c.A == 0 {
			return def
		}
		return c
	}
	var p cpPalette
	p.bg = or(t.InputBG, dvRGB(0xFFFFFF))
	p.border = or(t.InputBorder, or(t.Border, dvRGB(0x8A8A8A)))
	p.accent = or(t.Accent, dvRGB(0x0078D7))
	p.accent.A = 255
	p.focus = or(t.InputFocus, p.accent)
	p.text = or(t.InputText, dvRGB(0x1F1F1F))
	p.sel = or(t.TextSelectionBG, mixRGBA(p.bg, p.accent, 0.3))
	// Ошибка набора — тем же красным, что текст удалённого в сравнении: тема
	// уже подобрала его читаемым на поле ввода.
	p.err = or(t.DiffDelText, dvRGB(0xCF222E))
	p.dropBG = or(t.DropBG, p.bg)
	p.dropBorder = or(t.DropBorder, p.border)
	p.dropText = or(t.DropText, p.text)
	p.muted = or(t.SecondaryText, mixRGBA(p.dropText, p.dropBG, 0.45))
	p.hoverBG = or(t.MenuHoverBG, or(t.ListItemHover, mixRGBA(p.dropBG, p.accent, 0.15)))
	p.glyph = or(t.DropArrow, p.muted)
	return p
}

// ─── Код цвета ──────────────────────────────────────────────────────────────

// HexColor форматирует цвет как «#RRGGBB».
//
// Заглавными: код цвета читают и сверяют глазами, а вперемешку регистр мешает.
func HexColor(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

// ParseHexColor разбирает код цвета: «#RRGGBB», «RRGGBB», «#RGB», «RGB».
//
// Короткая запись раскрывается удвоением знаков (#0AF → #00AAFF) — так её
// понимают и CSS, и разметка. Всё прочее — не цвет: ok=false, и поле
// показывает набранное как ошибку, а не подставляет чёрный.
func ParseHexColor(s string) (color.RGBA, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	switch len(s) {
	case 3:
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	case 6:
	default:
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, true
}
