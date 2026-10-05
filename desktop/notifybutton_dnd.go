package desktop

// notifybutton_dnd.go — колокольчик и общая модель «Не беспокоить».
//
// Режим «Не беспокоить» показывают три места: центр уведомлений (колокольчик в
// заголовке), тост (молчит) и кнопка в трее (перечёркнутый колокольчик). Раньше
// у кнопки трея было только SetDoNotDisturb(bool) — потребителю приходилось
// самому пересылать ей каждое переключение, а центр и тост уже читали модель
// DoNotDisturb с подпиской. Здесь кнопка привязывается к той же модели.

// BindDoNotDisturb привязывает кнопку к модели «Не беспокоить»: состояние
// берётся сразу и обновляется при каждом переключении модели — тем же
// объектом, что отдан центру (NotificationCenter.SetDoNotDisturb) и тосту.
// nil снимает привязку (ручное значение SetDoNotDisturb остаётся прежним).
// Привязка снимается и в Close.
func (b *NotificationButton) BindDoNotDisturb(d DoNotDisturb) {
	b.dndMu.Lock()
	if b.dndUnsub != nil {
		b.dndUnsub()
		b.dndUnsub = nil
	}
	b.dndMu.Unlock()
	if d == nil {
		return
	}
	b.SetDoNotDisturb(d.Enabled())
	unsub := d.Subscribe(func() { b.SetDoNotDisturb(d.Enabled()) })
	b.dndMu.Lock()
	b.dndUnsub = unsub
	b.dndMu.Unlock()
}

// unbindDoNotDisturb снимает подписку на модель (из Close).
func (b *NotificationButton) unbindDoNotDisturb() {
	b.dndMu.Lock()
	u := b.dndUnsub
	b.dndUnsub = nil
	b.dndMu.Unlock()
	if u != nil {
		u()
	}
}
