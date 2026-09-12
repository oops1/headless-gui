package widget

// mergeview_theme.go — палитра контрола слияния.
//
// Палитра та же, что у контрола сравнения (dvPaletteFrom): окна должны
// выглядеть роднёй, а цвета сторон — совпадать с цветами удаления и вставки,
// к которым человек уже привык в сравнении. Своего набора полей в Theme у
// слияния нет: конфликт рисуется цветом «изменено» (Theme через p.dirty),
// решённый блок — цветом вставки.

// ApplyTheme — контракт Themeable: движок зовёт его при смене темы.
func (m *MergeView) ApplyTheme(t *Theme) {
	m.mu.Lock()
	m.pal = dvPaletteFrom(t)
	m.mu.Unlock()
	m.menu.ApplyTheme(t)
	m.Invalidate()
}
