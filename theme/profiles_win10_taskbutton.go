package theme

// Кнопки приложений и кнопка «Пуск» панели Windows 10.
//
// Вынесено из profiles.go по той же причине, что и profiles_win10_shell.go:
// набор объявляется дважды — для тёмной панели и для светлой (флаг
// taskbar.light), и вести два списка вручную значило бы разводить их при
// первой правке.

// declareWin10TaskButtons объявляет метрики и признаки кнопок приложений
// Windows 10. Стили состояний — declareWin10TaskButtonStyles.
//
// Как устроена кнопка в Windows 10:
//
//   - 48 px в ширину, значок 24 px, кнопки стоят вплотную;
//   - несколько окон одного приложения — одна кнопка (taskbutton.group);
//   - запущено — короткая тонкая линия снизу, активно — линия во всю ширину
//     и подсветка фона; закреплённое незапущенное линии не имеет;
//   - несколько окон — за значком видны торцы «стопки» (taskbutton.stack);
//   - наведение — прямоугольная плёнка без скруглений;
//   - свёрнутое окно и незапущенное приложение не «выключены»: наведение
//     подсвечивает их так же, как остальные (taskbutton.muted = false).
func declareWin10TaskButtons(p *Profile) {
	p.SetMetric("taskbutton.icon.size", 24).
		SetMetric("taskbutton.gap", 0).
		SetMetric("taskbutton.underline", 2).
		SetMetric("taskbutton.underline.len", 1).
		// Запущенное, но не активное окно — короткая линия в треть кнопки.
		SetMetric("taskbutton.underline.idle", 2).
		SetMetric("taskbutton.underline.idle.len", 0.33).
		// Торцы стопки: две тонкие чёрточки правее значка, каждая короче
		// предыдущей.
		SetMetric("taskbutton.stack", 2).
		SetMetric("taskbutton.stack.width", 1).
		SetMetric("taskbutton.stack.gap", 2).
		SetMetric("taskbutton.stack.offset", 3)

	p.SetFlag("taskbutton.group", true).
		SetFlag("taskbutton.muted", false)
}

// declareWin10TaskButtonStyles объявляет стили, которых у других тем нет:
// кнопка «Пуск» в состоянии «меню открыто» (StateActive). Плёнка та же, что
// у активного окна, — на акриле панели видно, что меню принадлежит кнопке.
func declareWin10TaskButtonStyles(p *Profile) {
	active, lightActive := RGBA(255, 255, 255, 38), RGBA(0, 0, 0, 34)
	p.SetStyle("startbutton", "", StateActive, StyleDelta{Fill: C(active)})
	p.SetStyleWhen(KeyTaskbarLight, "startbutton", "", StateActive, StyleDelta{Fill: C(lightActive)})

	declareWin10Preview(func(comp, part string, st State, d StyleDelta) {
		p.SetStyle(comp, part, st, d)
	}, win10ShellDark)
	declareWin10Preview(func(comp, part string, st State, d StyleDelta) {
		p.SetStyleWhen(KeyTaskbarLight, comp, part, st, d)
	}, win10ShellLight)
}

// declareWin10Preview объявляет вид панели предпросмотра окон: акрил, как у
// панели задач и «Пуска», заголовок строки текстом панели, подложка под
// миниатюрой — слабая плёнка. Без этих стилей предпросмотр наследует светлую
// поверхность темы и на тёмной панели Windows 10 выглядит чужим.
func declareWin10Preview(set func(comp, part string, st State, d StyleDelta), pal win10Shell) {
	clear := C(RGBA(0, 0, 0, 0))
	set("preview", "", StateNormal, StyleDelta{
		Backdrop: win10Acrylic(pal.tint, pal.fallback), Text: C(pal.text),
		Border: C(pal.border), BorderWidth: N(1), Elevation: N(0), Shadow: clear,
		// Поля даёт метрика preview.pad; отступ стиля сузил бы содержимое
		// внутри панели, размер которой его не учитывает.
		PadX: N(0), PadY: N(0),
	})
	set("preview", "header", StateNormal, StyleDelta{Text: C(pal.text), Border: clear, BorderWidth: N(0)})
	set("preview", "thumb", StateNormal, StyleDelta{Fill: C(pal.action), Border: clear, BorderWidth: N(0)})
}
