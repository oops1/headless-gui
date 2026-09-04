package tests

import (
	"bytes"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Кнопка-переключатель — запрос GG-28.
//
// `<ToggleButton>` разбирался тем же сборщиком, что и `<Button>`, и давал
// обычную кнопку: состояния «нажата» у неё нет, а вид нажатия держится лишь
// пока держат мышь. Панель из восьми кнопок-фильтров этим выразить было
// нечем — приложение хранило состояние у себя и подделывало вид, перекрашивая
// Background на каждый клик и на каждую смену темы.

func TestToggleButton_ClickKeepsTheState(t *testing.T) {
	btn := widget.NewButton("Фильтр")
	btn.SetToggle(true)
	btn.SetBounds(image.Rect(0, 0, 80, 30))

	var seen []bool
	btn.OnCheckedChanged = func(v bool) { seen = append(seen, v) }

	press := func() {
		btn.OnMouseButton(widget.MouseEvent{X: 10, Y: 10, Button: widget.MouseLeft, Pressed: true})
		btn.OnMouseButton(widget.MouseEvent{X: 10, Y: 10, Button: widget.MouseLeft, Pressed: false})
	}

	press()
	if !btn.IsChecked() {
		t.Error("после щелчка переключатель не включился")
	}
	press()
	if btn.IsChecked() {
		t.Error("после второго щелчка переключатель не выключился")
	}
	if len(seen) != 2 || !seen[0] || seen[1] {
		t.Errorf("о смене состояния сообщили как %v, ждали [true false]", seen)
	}
}

// Обычная кнопка состояние не держит: включение — свойство переключателя.
func TestToggleButton_PlainButtonStaysStateless(t *testing.T) {
	btn := widget.NewButton("Обычная")
	btn.SetBounds(image.Rect(0, 0, 80, 30))
	btn.OnMouseButton(widget.MouseEvent{X: 10, Y: 10, Button: widget.MouseLeft, Pressed: true})
	btn.OnMouseButton(widget.MouseEvent{X: 10, Y: 10, Button: widget.MouseLeft, Pressed: false})

	if btn.IsChecked() {
		t.Error("обычная кнопка осталась «нажатой» после клика")
	}
}

// Клавиатура включает переключатель так же, как мышь: иначе он слушался бы
// только мыши.
func TestToggleButton_KeyboardTogglesToo(t *testing.T) {
	btn := widget.NewButton("Фильтр")
	btn.SetToggle(true)
	btn.SetBounds(image.Rect(0, 0, 80, 30))

	btn.OnKeyEvent(widget.KeyEvent{Code: widget.KeySpace, Pressed: true})
	if !btn.IsChecked() {
		t.Error("пробел не включил переключатель")
	}
}

// SetChecked не зовёт обработчик: программная установка — не действие
// пользователя, и обработчик, синхронизирующий группу кнопок, ушёл бы в
// рекурсию.
func TestToggleButton_SetCheckedIsSilent(t *testing.T) {
	btn := widget.NewButton("Фильтр")
	btn.SetToggle(true)
	fired := 0
	btn.OnCheckedChanged = func(bool) { fired++ }

	btn.SetChecked(true)
	btn.SetChecked(false)
	if fired != 0 {
		t.Errorf("SetChecked позвал обработчик %d раз", fired)
	}
}

// Снятие режима переключателя гасит состояние: «нажатая» кнопка, переставшая
// быть переключателем, осталась бы нажатой навсегда.
func TestToggleButton_TurningOffClearsTheState(t *testing.T) {
	btn := widget.NewButton("Фильтр")
	btn.SetToggle(true)
	btn.SetChecked(true)
	btn.SetToggle(false)

	if btn.IsChecked() {
		t.Error("кнопка осталась включённой, перестав быть переключателем")
	}
}

// Разметка: тег даёт переключатель, атрибут — начальное состояние.
func TestToggleButton_FromXAML(t *testing.T) {
	xaml := `<Window Width="200" Height="100">
	  <Canvas>
	    <ToggleButton x:Name="filter" Left="10" Top="10" Width="80" Height="28"
	                  Content="Изменённые" IsChecked="True"/>
	    <Button x:Name="plain" Left="10" Top="50" Width="80" Height="28" Content="Обычная"/>
	  </Canvas>
	</Window>`

	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}

	tb, ok := reg["filter"].(*widget.Button)
	if !ok {
		t.Fatalf("ToggleButton собрался как %T", reg["filter"])
	}
	if !tb.IsToggle() {
		t.Error("<ToggleButton> не стал переключателем")
	}
	if !tb.IsChecked() {
		t.Error(`IsChecked="True" не включило состояние`)
	}

	pb, ok := reg["plain"].(*widget.Button)
	if !ok {
		t.Fatalf("Button собрался как %T", reg["plain"])
	}
	if pb.IsToggle() {
		t.Error("<Button> стал переключателем")
	}
}

// Включённый переключатель ВИДНО: он рисуется не так, как выключенный.
//
// Проверка по пикселям, а не по полю: приложение обходилось перекраской
// Background именно потому, что состояние без вида ему не помогало.
func TestToggleButton_LooksPressedWhenChecked(t *testing.T) {
	root := widget.NewPanel(widget.Theme{}.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 120, 60))

	btn := widget.NewButton("Фильтр")
	btn.SetToggle(true)
	btn.SetBounds(image.Rect(10, 10, 110, 44))
	root.AddChild(btn)

	eng := engine.New(120, 60, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	off := snapshotRGBA(eng.RenderOnce())

	btn.SetChecked(true)
	eng.Invalidate()
	on := snapshotRGBA(eng.RenderOnce())

	if bytes.Equal(off.Pix, on.Pix) {
		t.Error("включённый переключатель выглядит так же, как выключенный")
	}
}
