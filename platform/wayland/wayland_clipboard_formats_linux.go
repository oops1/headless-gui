//go:build linux && !android

package wayland

// wayland_clipboard_formats_linux.go — какие виды содержимого лежат в буфере
// обмена Wayland (base.ClipboardFormats).
//
// Типы выбранного offer компоновщик присылает событиями offer ДО самого
// selection, и handleSelection уже разложил их по полям (offerText, offerHTML,
// offerURIList/offerGnome, offerImage). Так что ответ — чтение этих полей под
// замком: ни канала, ни ожидания владельца.

import (
	"github.com/oops1/headless-gui/v4/widget/base"

	"github.com/oops1/headless-gui/v4/platform/native"
)

var _ base.ClipboardFormatsProvider = (*wlClipboard)(nil)

// ClipboardFormats возвращает виды содержимого буфера: свои, пока владеем
// им сами, иначе — объявленные текущим offer'ом. Реализует
// base.ClipboardFormatsProvider. Мгновенно и безопасно из любой горутины.
func (c *wlClipboard) ClipboardFormats() base.ClipboardFormatSet {
	if c == nil || c.w == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.source != 0 {
		// Чужой selection стирает наши поля раньше, чем компоновщик отменит
		// источник (handleSelection): пустое — не наше, смотрим offer.
		if s := native.OwnedFormats(c.owned, c.ownedHTML, len(c.ownedFiles) > 0 || c.ownedVirt != nil, c.ownedImg != nil); s != 0 {
			return s
		}
	}
	if c.offer == 0 {
		return 0
	}
	var s base.ClipboardFormatSet
	if c.offerText {
		s |= base.ClipboardFormatText
	}
	if c.offerHTML != "" {
		s |= base.ClipboardFormatHTML
	}
	if c.offerURIList || c.offerGnome {
		s |= base.ClipboardFormatFiles
	}
	if c.offerImage != "" {
		s |= base.ClipboardFormatImage
	}
	return s
}
