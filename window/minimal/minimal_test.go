package minimal

import (
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// window/minimal не подключает ни одной возможности окна.
func TestMinimal_NoCapabilities(t *testing.T) {
	if c := native.Capabilities(); len(c) != 0 {
		t.Errorf("window/minimal подключил возможности: %v", c)
	}
}
