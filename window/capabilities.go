package window

// capabilities.go — возможности окна (ADR-0013 §3), которые фасад window
// подключает пустыми импортами: каждая регистрируется в platform/native из
// своего init. window/minimal — тот же фасад без этого файла.

import (
	_ "github.com/oops1/headless-gui/v3/platform/a11y"
	_ "github.com/oops1/headless-gui/v3/platform/dnd"
	_ "github.com/oops1/headless-gui/v3/platform/shell"
	_ "github.com/oops1/headless-gui/v3/platform/tray"
)
