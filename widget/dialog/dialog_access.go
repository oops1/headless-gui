package dialog

import "github.com/oops1/headless-gui/v3/widget/base"

// dialog_access.go — диалог в дереве доступности.
//
// Контейнер диалога раньше попадал в дерево безымянной группой неизвестной
// роли: скринридер, зайдя в MessageBox, не мог сказать «диалог „Ошибка“» и
// читал только отдельные надписи и кнопки. Теперь у контейнера есть роль,
// имя — заголовок и описание — текст сообщения, и скринридер объявляет диалог
// целиком при его появлении.

// AccessInfo — контракт Accessible: роль «диалог», имя — заголовок, описание —
// Description (MessageBox кладёт туда текст сообщения), иначе подсказка.
func (d *Dialog) AccessInfo() base.AccessInfo {
	info := base.AccessInfo{
		Role:        base.RoleDialog,
		Name:        d.Title,
		Description: d.Description,
		Bounds:      d.Bounds(),
	}
	if info.Description == "" {
		info.Description = d.GetToolTip()
	}
	if !d.IsEnabled() {
		info.States = append(info.States, base.StateDisabled)
	}
	return info
}

// AccessInfo — кнопка ✕ в заголовке диалога: кнопка с локализованным именем,
// а не безымянный элемент неизвестной роли.
func (cb *dialogCloseBtn) AccessInfo() base.AccessInfo {
	return base.AccessInfo{Role: base.RoleButton, Name: base.Tr("dlg.close"), Bounds: cb.Bounds()}
}
