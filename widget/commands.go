// commands.go — ICommand (WPF Commanding) и InputBindings/KeyBinding.
package widget

import "strings"

// ICommand — команда (WPF System.Windows.Input.ICommand).
type ICommand interface {
	Execute(parameter interface{})
	CanExecute(parameter interface{}) bool
}

// RelayCommand — простая реализация ICommand на функциях.
type RelayCommand struct {
	ExecuteFn    func(parameter interface{})
	CanExecuteFn func(parameter interface{}) bool
}

// Execute выполняет команду.
func (c *RelayCommand) Execute(p interface{}) {
	if c.ExecuteFn != nil {
		c.ExecuteFn(p)
	}
}

// CanExecute сообщает, доступна ли команда (по умолчанию true).
func (c *RelayCommand) CanExecute(p interface{}) bool {
	if c.CanExecuteFn != nil {
		return c.CanExecuteFn(p)
	}
	return true
}

// NewRelayCommand создаёт команду из функции без параметра.
func NewRelayCommand(fn func()) *RelayCommand {
	return &RelayCommand{ExecuteFn: func(interface{}) {
		if fn != nil {
			fn()
		}
	}}
}

// ─── InputBindings / KeyBinding ─────────────────────────────────────────────

// InputBinding — привязка горячей клавиши к команде (WPF KeyBinding).
type InputBinding struct {
	Key         KeyCode
	Mods        KeyMod
	Command     ICommand
	Param       interface{}
	CommandPath string // путь {Binding} к команде (резолвится после загрузки)
}

// matchInputBinding ищет InputBinding по коду клавиши и модификаторам.
func matchInputBinding(bindings []InputBinding, code KeyCode, mod KeyMod) (ICommand, interface{}, bool) {
	for _, b := range bindings {
		if b.Key == code && b.Mods == mod && b.Command != nil {
			return b.Command, b.Param, true
		}
	}
	return nil, nil, false
}

// parseKeyName переводит имя клавиши (XAML Key) в KeyCode.
func parseKeyName(s string) KeyCode {
	s = strings.TrimSpace(s)
	if len(s) == 1 {
		r := s[0]
		if r >= 'A' && r <= 'Z' {
			return KeyCode(r)
		}
		if r >= 'a' && r <= 'z' {
			return KeyCode(r - 'a' + 'A')
		}
		// Цифровой ряд: коды клавиш здесь — виртуальные клавиши Windows, у
		// цифр это 0x30..0x39, то есть код самого символа.
		if r >= '0' && r <= '9' {
			return KeyCode(r)
		}
	}

	lower := strings.ToLower(s)

	// Функциональные клавиши разбираем расчётом, а не двумя дюжинами строк в
	// switch: ряд непрерывен, и список пришлось бы дополнять при каждой новой.
	if k := parseFunctionKey(lower); k != KeyUnknown {
		return k
	}

	switch lower {
	case "enter", "return":
		return KeyEnter
	case "escape", "esc":
		return KeyEscape
	case "space", "spacebar":
		return KeySpace
	case "tab":
		return KeyTab
	case "delete", "del":
		return KeyDelete
	case "insert", "ins":
		return KeyInsert
	case "back", "backspace":
		return KeyBackspace
	case "home":
		return KeyHome
	case "end":
		return KeyEnd
	case "pageup", "pgup", "prior":
		return KeyPageUp
	case "pagedown", "pgdn", "pgdown", "next":
		return KeyPageDown
	case "left":
		return KeyLeft
	case "right":
		return KeyRight
	case "up":
		return KeyUp
	case "down":
		return KeyDown
	case "capslock", "caps":
		return KeyCapsLock
	case "numlock":
		return KeyNumLock
	case "scrolllock", "scroll":
		return KeyScrollLock
	case "printscreen", "prtsc", "snapshot":
		return KeyPrintScreen
	case "pause", "break":
		return KeyPause
	case "apps", "menu", "contextmenu":
		return KeyMenu
	// Цифровая клавиатура — отдельно от цифрового ряда: калькулятору важно,
	// с какой стороны нажали.
	case "numpad0", "num0":
		return KeyNumpad0
	case "numpad1", "num1":
		return KeyNumpad1
	case "numpad2", "num2":
		return KeyNumpad2
	case "numpad3", "num3":
		return KeyNumpad3
	case "numpad4", "num4":
		return KeyNumpad4
	case "numpad5", "num5":
		return KeyNumpad5
	case "numpad6", "num6":
		return KeyNumpad6
	case "numpad7", "num7":
		return KeyNumpad7
	case "numpad8", "num8":
		return KeyNumpad8
	case "numpad9", "num9":
		return KeyNumpad9
	case "add", "numpadadd", "numpadplus":
		return KeyAdd
	case "subtract", "numpadsubtract", "numpadminus":
		return KeySubtract
	case "multiply", "numpadmultiply":
		return KeyMultiply
	case "divide", "numpaddivide":
		return KeyDivide
	case "decimal", "numpaddecimal":
		return KeyDecimal
	// OEM-клавиши названы и по-wpf-овски, и по тому, что на них написано в
	// раскладке США: разметке удобнее второе, совместимости — первое.
	case "oemplus", "plus", "=":
		return KeyOemPlus
	case "oemminus", "minus", "-":
		return KeyOemMinus
	case "oemcomma", "comma", ",":
		return KeyOemComma
	case "oemperiod", "period", ".":
		return KeyOemPeriod
	case "oem1", "semicolon", ";":
		return KeyOemSemicolon
	case "oem2", "slash", "/":
		return KeyOemSlash
	case "oem3", "tilde", "backquote", "`":
		return KeyOemTilde
	case "oem4", "openbrace", "[":
		return KeyOemOpenBrace
	case "oem5", "backslash", "\\":
		return KeyOemBackslash
	case "oem6", "closebrace", "]":
		return KeyOemCloseBrace
	case "oem7", "quote", "'":
		return KeyOemQuote
	}
	return KeyUnknown
}

// parseFunctionKey разбирает «f5», «F12», «f24» — весь ряд F1..F24.
//
// KeyUnknown означает «это не функциональная клавиша»: и «f», и «f0», и «f25»,
// и «file». Именно поэтому имя проверяется целиком, а не по одному префиксу:
// «f» с мусором дальше не должно становиться F-чем-нибудь.
func parseFunctionKey(lower string) KeyCode {
	if len(lower) < 2 || lower[0] != 'f' {
		return KeyUnknown
	}
	n := 0
	for _, r := range lower[1:] {
		if r < '0' || r > '9' {
			return KeyUnknown
		}
		n = n*10 + int(r-'0')
		if n > 24 {
			return KeyUnknown
		}
	}
	if n < 1 {
		return KeyUnknown
	}
	return KeyF1 + KeyCode(n-1)
}

// parseModifiers переводит WPF Modifiers ("Ctrl+Shift") в KeyMod.
func parseModifiers(s string) KeyMod {
	var m KeyMod
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '+' || r == ',' || r == ' ' }) {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "ctrl", "control":
			m |= ModCtrl
		case "shift":
			m |= ModShift
		case "alt":
			m |= ModAlt
		case "win", "windows", "meta", "cmd":
			m |= ModMeta
		}
	}
	return m
}
