package widget

// clipboard.go — системный буфер обмена (ADR-0012, 8.7): реализации по ОС
// живут в platform/clipboard и ставят провайдер при загрузке; фасад
// импортирует пакет, чтобы буфер был у всех программ на widget, как прежде.

import _ "github.com/oops1/headless-gui/v4/platform/clipboard"
