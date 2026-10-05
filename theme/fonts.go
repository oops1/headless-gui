package theme

// GetFont возвращает именованный шрифт активной темы: "default", "caption",
// "title", "clock.large" — как объявил профиль. Второе значение false —
// темы нет или она такого шрифта не объявила; вызывающий берёт шрифт стиля.
func (m *Manager) GetFont(k Key) (FontSpec, bool) {
	m.mu.RLock()
	t := m.active
	m.mu.RUnlock()
	if t == nil {
		return FontSpec{}, false
	}
	return t.Font(k)
}
