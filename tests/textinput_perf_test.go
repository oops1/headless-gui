package tests

// Регрессионные тесты оптимизации PERF-11: кэш раскладки строки в TextInput и
// экономная мигающая каретка.

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// posCtx — DrawContext, считающий вызовы MeasureRunePositions.
type posCtx struct {
	opCtx
	measures int
}

func (c *posCtx) MeasureRunePositions(text string, sizePt float64) []int {
	c.measures++
	return c.opCtx.MeasureRunePositions(text, sizePt)
}

// TestTextInputLayoutCached — позиции рун считаются один раз, а не на каждом
// Draw; смена текста, маски пароля или ревизии метрик пересчитывает заново.
func TestTextInputLayoutCached(t *testing.T) {
	ti := widget.NewTextInput("подсказка")
	ti.SetBounds(image.Rect(0, 0, 200, 28))
	ti.SetText("некоторый текст поля")

	ctx := &posCtx{}
	ti.Draw(ctx)
	if ctx.measures != 1 {
		t.Fatalf("первый Draw должен измерить строку один раз, измерений: %d", ctx.measures)
	}

	ti.Draw(ctx)
	ti.Draw(ctx)
	ti.Draw(ctx)
	if ctx.measures != 1 {
		t.Errorf("повторные Draw пересчитали раскладку: %d измерений (кэш не работает)", ctx.measures)
	}

	// Смена текста инвалидирует кэш.
	ti.SetText("другой текст")
	ti.Draw(ctx)
	if ctx.measures != 2 {
		t.Errorf("после смены текста раскладка не пересчитана: %d измерений", ctx.measures)
	}

	// Ревизия метрик (смена DPI/HiDPI-масштаба движком) сбрасывает кэш.
	widget.BumpTextMetricsRev()
	ti.Draw(ctx)
	if ctx.measures != 3 {
		t.Errorf("после смены ревизии метрик раскладка не пересчитана: %d измерений", ctx.measures)
	}

	// Переключение маски пароля меняет отображаемую строку → пересчёт.
	ti.SetPasswordMode(true)
	ti.Draw(ctx)
	if ctx.measures != 4 {
		t.Errorf("после включения маски пароля раскладка не пересчитана: %d измерений", ctx.measures)
	}
	ti.Draw(ctx)
	if ctx.measures != 4 {
		t.Errorf("маскированная раскладка не закэширована: %d измерений", ctx.measures)
	}
}

// TestTextInputCaretNeedsAnimationOnlyOnPhaseChange — NeedsAnimation не тянет
// кадр, пока фаза мигания каретки не изменилась (PERF-11).
func TestTextInputCaretNeedsAnimationOnlyOnPhaseChange(t *testing.T) {
	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 200, 28))

	// Без фокуса анимация не нужна никогда.
	if ti.NeedsAnimation() {
		t.Fatal("без фокуса NeedsAnimation должен быть false")
	}

	// Считаем точечные инвалидации виджета.
	var rects []image.Rectangle
	widget.SetUIRectChangeNotifier(func(r image.Rectangle) { rects = append(rects, r) })
	defer widget.SetUIRectChangeNotifier(nil)

	ti.SetFocused(true)
	ti.Draw(&posCtx{}) // зафиксировали текущую фазу каретки
	rects = rects[:0]

	// Сразу после отрисовки фаза та же — кадров быть не должно.
	for i := 0; i < 20; i++ {
		if ti.NeedsAnimation() {
			t.Fatal("NeedsAnimation вернул true — это заставило бы движок рендерить полный кадр")
		}
	}
	if len(rects) != 0 {
		t.Fatalf("в пределах одной фазы виджет инвалидировался %d раз", len(rects))
	}

	// Дожидаемся смены фазы (полупериод 530 мс) — должна прийти РОВНО одна
	// точечная инвалидация по bounds поля.
	deadline := time.Now().Add(2 * time.Second)
	for len(rects) == 0 && time.Now().Before(deadline) {
		if ti.NeedsAnimation() {
			t.Fatal("даже на смене фазы NeedsAnimation обязан вернуть false (кадр даёт damage)")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(rects) == 0 {
		t.Fatal("смена фазы каретки не инвалидировала виджет — каретка перестанет мигать")
	}
	if len(rects) != 1 {
		t.Errorf("на одну смену фазы пришло %d инвалидаций, ожидалась 1", len(rects))
	}
	if rects[0] != ti.Bounds() {
		t.Errorf("инвалидирован %v вместо прямоугольника поля %v", rects[0], ti.Bounds())
	}
}
