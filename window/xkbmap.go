package window

// xkbmap.go — парсер текстового xkb-keymap. Платформонезависим (чистый
// разбор строк) — тестируется на всех ОС; используется Linux-бэкендами.
//
// Далее — парсер текстового xkb-keymap (формат xkb_v1, который Wayland
// передаёт в wl_keyboard.keymap). Извлекается только то, что нужно для
// ввода символов: keycode → группы (раскладки) → уровни (Shift) → руна.
// Типы, actions, compat и виртуальные модификаторы игнорируются.
//
// Структура файла (упрощённо):
//
//	xkb_keymap {
//	    xkb_keycodes "..." { <AE01> = 10; alias <LatQ> = <AD01>; ... };
//	    xkb_types    "..." { ... };
//	    xkb_compat   "..." { ... };
//	    xkb_symbols  "..." {
//	        key <AE01> { [ 1, exclam ] };
//	        key <AD01> { [ q, Q ], [ Cyrillic_shorti, Cyrillic_SHORTI ] };
//	        key <AC01> { symbols[Group1]= [ a, A ], symbols[Group2]= [ ... ] };
//	    };
//	};

import "strings"

// xkbKeymap — распарсенная таблица символов.
type xkbKeymap struct {
	// keys: xkb-keycode → группа → уровень → руна.
	// Wayland-событие key несёт evdev-код; xkb-код = evdev + 8.
	keys map[uint32][][]rune
}

// runeFor возвращает руну для xkb-кода с учётом группы (раскладки),
// Shift и CapsLock. 0 — непечатаемая клавиша.
func (m *xkbKeymap) runeFor(xkbCode uint32, group int, shift, caps bool) rune {
	if m == nil {
		return 0
	}
	groups := m.keys[xkbCode]
	if len(groups) == 0 {
		return 0
	}
	if group >= len(groups) || group < 0 {
		group = 0
	}
	levels := groups[group]
	if len(levels) == 0 {
		return 0
	}
	level := 0
	if shift && len(levels) > 1 {
		level = 1
	}
	r := levels[level]
	// CapsLock: для букв действует как инверсия Shift.
	if caps && isLetterRune(r) {
		alt := 0
		if level == 0 {
			alt = 1
		}
		if alt < len(levels) && isLetterRune(levels[alt]) {
			r = levels[alt]
		}
	}
	return r
}

func isLetterRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= 0x00C0 && r <= 0x024F) || // Latin ext
		(r >= 0x0400 && r <= 0x04FF) || // кириллица
		(r >= 0x0370 && r <= 0x03FF) // греческий
}

// parseXkbKeymap разбирает текстовый keymap. nil — разобрать не удалось.
func parseXkbKeymap(data string) *xkbKeymap {
	keycodes := parseXkbSection(data, "xkb_keycodes")
	symbols := parseXkbSection(data, "xkb_symbols")
	if keycodes == "" || symbols == "" {
		return nil
	}

	// ── keycodes: <AE01> = 10; и alias <LatQ> = <AD01>; ─────────────────────
	nameToCode := make(map[string]uint32, 256)
	aliases := make(map[string]string)
	for _, line := range strings.Split(keycodes, ";") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "alias") {
			// alias <LatQ> = <AD01>
			parts := strings.SplitN(line[5:], "=", 2)
			if len(parts) == 2 {
				from := strings.TrimSpace(parts[0])
				to := strings.TrimSpace(parts[1])
				aliases[trimAngle(from)] = trimAngle(to)
			}
			continue
		}
		if !strings.HasPrefix(line, "<") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := trimAngle(strings.TrimSpace(parts[0]))
		code := parseUintPrefix(strings.TrimSpace(parts[1]))
		if name != "" && code > 0 {
			nameToCode[name] = code
		}
	}
	for from, to := range aliases {
		if c, ok := nameToCode[to]; ok {
			nameToCode[from] = c
		}
	}

	// ── symbols: key <NAME> { ... }; ─────────────────────────────────────────
	km := &xkbKeymap{keys: make(map[uint32][][]rune, 256)}
	rest := symbols
	for {
		i := strings.Index(rest, "key ")
		if i < 0 {
			break
		}
		rest = rest[i+4:]
		// имя клавиши
		lt := strings.IndexByte(rest, '<')
		gt := strings.IndexByte(rest, '>')
		if lt < 0 || gt < lt {
			continue
		}
		name := rest[lt+1 : gt]
		rest = rest[gt+1:]
		// тело до закрывающей '}' верхнего уровня (внутри бывают вложенные [])
		ob := strings.IndexByte(rest, '{')
		if ob < 0 {
			break
		}
		depth := 0
		end := -1
		for j := ob; j < len(rest); j++ {
			switch rest[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = j
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			break
		}
		body := rest[ob+1 : end]
		rest = rest[end+1:]

		code, ok := nameToCode[name]
		if !ok {
			continue
		}
		groups := parseXkbKeyBody(body)
		if len(groups) > 0 {
			km.keys[code] = groups
		}
	}
	if len(km.keys) == 0 {
		return nil
	}
	return km
}

// parseXkbKeyBody извлекает группы символов из тела key {...}:
// каждая группа — это [ sym1, sym2, ... ] (в порядке появления, включая
// формы symbols[GroupN]= [...]). Скобки-индексы (symbols[Group1], actions[…])
// отличаются по контексту: перед ними стоит идентификатор.
func parseXkbKeyBody(body string) [][]rune {
	var groups [][]rune
	for {
		ob := strings.IndexByte(body, '[')
		if ob < 0 {
			break
		}
		// Скобка-индекс? Предыдущий непробельный символ — буква/цифра.
		isIndex := false
		for k := ob - 1; k >= 0; k-- {
			c := body[k]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				continue
			}
			isIndex = c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
			break
		}
		cb := strings.IndexByte(body[ob:], ']')
		if cb < 0 {
			break
		}
		list := body[ob+1 : ob+cb]
		body = body[ob+cb+1:]
		if isIndex {
			continue // symbols[Group1] и т.п. — пропускаем
		}

		var levels []rune
		for _, sym := range strings.Split(list, ",") {
			levels = append(levels, keysymNameToRune(strings.TrimSpace(sym)))
		}
		groups = append(groups, levels)
	}
	return groups
}

// parseXkbSection возвращает содержимое секции name {...} (между скобками).
func parseXkbSection(data, name string) string {
	i := strings.Index(data, name)
	if i < 0 {
		return ""
	}
	ob := strings.IndexByte(data[i:], '{')
	if ob < 0 {
		return ""
	}
	start := i + ob + 1
	depth := 1
	for j := start; j < len(data); j++ {
		switch data[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return data[start:j]
			}
		}
	}
	return ""
}

func trimAngle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return s
}

func parseUintPrefix(s string) uint32 {
	var v uint32
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + uint32(c-'0')
	}
	return v
}
