package theme

// trayExtraComponents — компоненты трея, у которых нет собственного оформления
// в встроенных профилях: обобщённый значок, кнопка центра уведомлений и полоска
// «Показать рабочий стол». Выглядят они как значки состояния рядом, поэтому
// наследуют стили «tray.volume» — по всем состояниям. Без этого стиль по
// умолчанию дал бы им заливку общей поверхности: на панели проступали бы
// прямоугольники чужого оттенка (то же, от чего профили освобождают значки
// состояния).
var trayExtraComponents = []string{"tray.icon", "tray.notifications", "tray.showdesktop"}

// inheritTrayStyles копирует стили «tray.volume» компонентам trayExtraComponents,
// не трогая уже заданные профилем явно. Зовётся последней строкой каждого
// базового профиля, когда стили значков состояния окончательны.
func inheritTrayStyles(p *Profile) {
	type pair struct {
		key StyleKey
		d   StyleDelta
	}
	var src []pair
	for k, d := range p.Styles {
		if k.Component == "tray.volume" {
			src = append(src, pair{k, d})
		}
	}
	// Условные правила (светлая панель и подобные) копируются так же: иначе под
	// флагом значок, кнопка центра и полоска остаются в цветах тёмной панели.
	var cond []ConditionalStyle
	have := map[condID]bool{}
	for _, c := range p.Conditional {
		if c.Key.Component == "tray.volume" {
			cond = append(cond, c)
		}
		have[condID{c.Flag, c.Key}] = true
	}
	for _, comp := range trayExtraComponents {
		for _, e := range src {
			nk := e.key
			nk.Component = comp
			if _, ok := p.Styles[nk]; !ok {
				p.Styles[nk] = e.d
			}
		}
		for _, c := range cond {
			nc := c
			nc.Key.Component = comp
			// Явное правило профиля с тем же флагом и ключом не перетирается.
			if !have[condID{nc.Flag, nc.Key}] {
				p.Conditional = append(p.Conditional, nc)
			}
		}
	}
}

// condID — флаг и ключ условного правила: по ним inheritTrayStyles
// узнаёт, что профиль уже объявил своё правило.
type condID struct {
	Flag Key
	Key  StyleKey
}
