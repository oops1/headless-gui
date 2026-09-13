package tests

import (
	"image"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-61: у панели итога есть свои заголовок и примечание. GG-65: у шапки панели
// есть вторая строка — пояснение мелким шрифтом, — а подписи сторон можно
// прочитать обратно.

func mergeHeadersView() *widget.MergeView {
	mv := widget.NewMergeView("", "")
	mv.SetBounds(image.Rect(0, 0, 1000, 600))
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	return mv
}

// GG-61: SetResultInfo доходит до шапки итога (видно скринридеру) и читается
// обратно; пустой заголовок — прежний, из ключа merge.side.result.
func TestMergeView_ResultInfo(t *testing.T) {
	mv := mergeHeadersView()

	panes := mergePanes(widget.BuildAccessTree(mv, mv))
	if !strings.Contains(panes[3].Name, widget.Tr("merge.side.result")) {
		t.Fatalf("итог без заголовка должен подписываться ключом: %q", panes[3].Name)
	}

	info := widget.MergeSideInfo{Title: "итог слияния", Note: "config.go"}
	mv.SetResultInfo(info)
	if got := mv.ResultInfo(); got != info {
		t.Fatalf("ResultInfo = %+v, хочу %+v", got, info)
	}
	panes = mergePanes(widget.BuildAccessTree(mv, mv))
	if !strings.Contains(panes[3].Name, "итог слияния") {
		t.Fatalf("заголовок итога не дошёл до панели: %q", panes[3].Name)
	}

	mv.SetResultInfo(widget.MergeSideInfo{})
	panes = mergePanes(widget.BuildAccessTree(mv, mv))
	if !strings.Contains(panes[3].Name, widget.Tr("merge.side.result")) {
		t.Fatalf("сброс заголовка не вернул ключ: %q", panes[3].Name)
	}
}

// GG-65: подписи сторон читаются обратно.
func TestMergeView_SidesGetter(t *testing.T) {
	mv := mergeHeadersView()
	ours := widget.MergeSideInfo{Title: "main", Note: "config.go", Hint: "ваша ветка, HEAD"}
	base := widget.MergeSideInfo{Title: "merge-base", Hint: "общий предок"}
	theirs := widget.MergeSideInfo{Title: "feature", Hint: "вливаемая ветка"}
	mv.SetSides(ours, base, theirs)

	got := mv.Sides()
	if got[0] != ours || got[1] != base || got[2] != theirs {
		t.Fatalf("Sides() = %+v", got)
	}
}

// GG-65: пояснение растит шапку — содержимое панелей опускается ниже, — и
// уходит скринридеру описанием панели. У сторон и у итога шапки растут
// независимо: пояснение у итога не сдвигает верхние панели.
func TestMergeView_HintGrowsHeader(t *testing.T) {
	mv := mergeHeadersView()
	plain := mergePanes(widget.BuildAccessTree(mv, mv))

	mv.SetSides(
		widget.MergeSideInfo{Title: "main", Hint: "ваша ветка, HEAD"},
		widget.MergeSideInfo{Title: "base"},
		widget.MergeSideInfo{Title: "feature"},
	)
	hinted := mergePanes(widget.BuildAccessTree(mv, mv))
	for i := 0; i < 3; i++ {
		if hinted[i].Bounds.Min.Y <= plain[i].Bounds.Min.Y {
			t.Fatalf("панель %d: пояснение у стороны не опустило содержимое (%d → %d)",
				i, plain[i].Bounds.Min.Y, hinted[i].Bounds.Min.Y)
		}
	}
	if hinted[0].Description != "ваша ветка, HEAD" {
		t.Fatalf("пояснение не дошло до скринридера: %q", hinted[0].Description)
	}
	if hinted[1].Description != "" {
		t.Fatalf("у стороны без пояснения описание %q", hinted[1].Description)
	}

	topBefore := hinted[0].Bounds.Min.Y
	resBefore := hinted[3].Bounds.Min.Y
	mv.SetResultInfo(widget.MergeSideInfo{Hint: "запишется в файл по «Сохранить»"})
	after := mergePanes(widget.BuildAccessTree(mv, mv))
	if after[3].Bounds.Min.Y <= resBefore {
		t.Fatalf("пояснение у итога не опустило его содержимое (%d → %d)", resBefore, after[3].Bounds.Min.Y)
	}
	if after[0].Bounds.Min.Y != topBefore {
		t.Fatalf("пояснение у итога сдвинуло верхние панели (%d → %d)", topBefore, after[0].Bounds.Min.Y)
	}
}
