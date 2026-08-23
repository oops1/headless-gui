// Тесты TabControl (ENGINE_ISSUES tts-studio #9):
// тултипы заголовков вкладок и разделители групп.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestTabControl_TabToolTip — GetToolTip отдаёт подсказку вкладки под курсором.
func TestTabControl_TabToolTip(t *testing.T) {
	tc := widget.NewTabControl()
	tc.SetBounds(image.Rect(0, 0, 400, 200))
	tc.AddTab("aa", nil)
	tc.AddTab("bb", nil)
	tc.SetTabToolTip(1, "вторая вкладка")
	tc.ToolTip = "общий"

	// Без Draw хит-тест берёт fallback-ширину len(header)*8 + TabPadH*2.
	tabW := 2*8 + tc.TabPadH*2

	tc.OnMouseMove(tabW+5, 10) // внутри второй вкладки
	if got := tc.GetToolTip(); got != "вторая вкладка" {
		t.Errorf("GetToolTip над вкладкой 1 = %q", got)
	}

	tc.OnMouseMove(5, 10) // первая вкладка — своей подсказки нет
	if got := tc.GetToolTip(); got != "общий" {
		t.Errorf("GetToolTip над вкладкой 0 = %q, ждали общий", got)
	}

	tc.OnMouseMove(5, 150) // вне полосы вкладок
	if got := tc.GetToolTip(); got != "общий" {
		t.Errorf("GetToolTip вне полосы = %q, ждали общий", got)
	}
}

// TestTabControl_SeparatorShiftsHitTest — разделитель занимает место в полосе
// и сдвигает хит-тест последующих вкладок.
func TestTabControl_SeparatorShiftsHitTest(t *testing.T) {
	tc := widget.NewTabControl()
	tc.SetBounds(image.Rect(0, 0, 400, 200))
	tc.AddTab("aa", nil)
	tc.AddTab("bb", nil)
	tc.SetTabSeparator(1, true)

	// Без Draw хит-тест клика берёт fallback-ширину TabPadH*2 + 80.
	tabW := tc.TabPadH*2 + 80
	const sepW = 9

	// Клик в зазор разделителя — не попадает ни в одну вкладку.
	if tc.OnMouseButton(widget.MouseEvent{Button: widget.MouseLeft, Pressed: false, X: tabW + sepW/2, Y: 10}) {
		t.Error("клик в зазор разделителя засчитался как вкладка")
	}

	// Клик правее зазора — вторая вкладка.
	if !tc.OnMouseButton(widget.MouseEvent{Button: widget.MouseLeft, Pressed: false, X: tabW + sepW + 5, Y: 10}) {
		t.Fatal("клик по второй вкладке не засчитался")
	}
	if got := tc.Active(); got != 1 {
		t.Errorf("Active() = %d, ждали 1", got)
	}

	// Разделитель перед ПЕРВОЙ видимой вкладкой места не занимает.
	tc2 := widget.NewTabControl()
	tc2.SetBounds(image.Rect(0, 0, 400, 200))
	tc2.AddTab("aa", nil)
	tc2.SetTabSeparator(0, true)
	if !tc2.OnMouseButton(widget.MouseEvent{Button: widget.MouseLeft, Pressed: false, X: 3, Y: 10}) {
		t.Error("первая вкладка с SeparatorBefore сдвинулась, хотя не должна")
	}
}
