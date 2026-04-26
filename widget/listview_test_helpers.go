package widget

// ListViewScrollY — test-only геттер текущего вертикального смещения
// ListView. Не предназначен для прикладного кода: внутреннее поле
// scrollY является деталью реализации и может изменяться. Используется
// в тестах пакета tests/ для проверки поведения SetItems/AddItem
// и AutoScrollToBottom.
func ListViewScrollY(lv *ListView) int {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	return lv.scrollY
}
