package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Движок, который не рисует, не копит объявления о переносе бесконечно.
//
// Перетаскивание объявляет перенос шестьдесят раз в секунду, а движок,
// созданный и оставленный без кадров (окно свёрнуто, потребитель кадров не
// запрашивает), их не разбирает: приёмник только складывает, забирает —
// сборка кадра. Без ограничения список рос бы всё время работы приложения.
func TestNoteMove_IdleEngineDoesNotHoard(t *testing.T) {
	e := New(600, 400, 60)
	e.SetRenderOnDemand(true)

	r := image.Rect(10, 10, 110, 80)
	for i := 0; i < 20*maxPendingMoves; i++ {
		e.noteMove(widget.MoveNotice{From: r.Min, Rect: r.Add(image.Pt(i+1, 0))})
	}

	e.moveMu.Lock()
	n := len(e.pendingMoves)
	e.moveMu.Unlock()
	if n > maxPendingMoves {
		t.Errorf("накоплено %d объявлений при пределе %d", n, maxPendingMoves)
	}
	if n == 0 {
		t.Error("не принято ни одного объявления — предел съел и нужные")
	}
}

// Чужие объявления движок не берёт: перенос целиком за пределами его холста
// заставил бы копировать пиксели с места, где ничего не переезжало.
func TestNoteMove_IgnoresWhatIsNotOnThisCanvas(t *testing.T) {
	e := New(200, 150, 60)

	off := image.Rect(400, 300, 600, 450)
	e.noteMove(widget.MoveNotice{From: off.Min, Rect: off.Add(image.Pt(10, 10))})
	if got := len(e.takeMoves()); got != 0 {
		t.Errorf("движок 200x150 принял %d чужих объявлений", got)
	}

	// А своё — берёт, в том числе приезжающее на холст со стороны.
	in := image.Rect(-50, 20, 60, 90)
	e.noteMove(widget.MoveNotice{From: in.Min, Rect: in.Add(image.Pt(60, 0))})
	if got := len(e.takeMoves()); got != 1 {
		t.Errorf("движок не взял объявление на своём холсте: %d", got)
	}

	// takeMoves забирает список целиком: второй кадр не должен получить
	// объявления первого.
	if got := len(e.takeMoves()); got != 0 {
		t.Errorf("после takeMoves осталось %d объявлений", got)
	}
}
