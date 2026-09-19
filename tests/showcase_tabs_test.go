package tests

import (
	"os"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// Витрина showcase показывает КАЖДЫЙ контрол движка — иначе о нём не узнают.
// Тест держит на месте вкладки «Панели инструментов», «Деревья и таблицы» и
// «Сравнение»: разметка собирается, контролы те, что заявлены, а файлы-образцы
// сравнения найдены (ошибку чтения разметка глотает молча, и пустой DiffView
// выглядел бы как «так и было задумано»).

func showcaseReg(t *testing.T) map[string]widget.Widget {
	t.Helper()
	const path = "../assets/ui/showcase.xaml"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("разметки нет: %v", err)
	}
	root, reg, err := widget.LoadUIFromXAMLFile(path)
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	t.Cleanup(func() { widget.ReleaseXAML(root) })
	return reg
}

func TestShowcase_ToolbarTab(t *testing.T) {
	reg := showcaseReg(t)

	tb, ok := reg["tbMain"].(*widget.ToolBar)
	if !ok {
		t.Fatalf("tbMain — %T, ждал *widget.ToolBar", reg["tbMain"])
	}
	if tb.OverflowCount() != 0 {
		t.Errorf("панель переполнена на пустом месте: %d", tb.OverflowCount())
	}
	push, ok := reg["tbPush"].(*widget.MenuButton)
	if !ok || !push.Split {
		t.Fatalf("tbPush — %T (разделённая: %v)", reg["tbPush"], ok && push.Split)
	}
	if len(push.Items) != 2 {
		t.Errorf("пунктов меню у tbPush: %d, ждал 2", len(push.Items))
	}
	flow, ok := reg["tbFlow"].(*widget.MenuButton)
	if !ok || flow.Split {
		t.Fatalf("tbFlow — %T (должна открывать меню целиком)", reg["tbFlow"])
	}

	// Распорка прижала правую кнопку к краю панели.
	settings, ok := reg["tbSettings"].(*widget.Button)
	if !ok {
		t.Fatalf("tbSettings — %T", reg["tbSettings"])
	}
	if got, want := settings.Bounds().Max.X, tb.Bounds().Max.X-tb.Padding; got != want {
		t.Errorf("правая кнопка кончается на %d, ждал %d — распорка не сработала", got, want)
	}

	if _, ok := reg["dockPanelDemo"].(*widget.DockPanel); !ok {
		t.Errorf("dockPanelDemo — %T, ждал *widget.DockPanel", reg["dockPanelDemo"])
	}
}

func TestShowcase_TreesAndTablesTab(t *testing.T) {
	reg := showcaseReg(t)

	tw, ok := reg["treeDemo"].(*widget.TreeViewWidget)
	if !ok {
		t.Fatalf("treeDemo — %T", reg["treeDemo"])
	}
	if tw.Tree.SelectionMode != treeview.SelectionExtended {
		t.Error("дерево показывает выбор набора — нужен SelectionMode=Extended")
	}
	if len(tw.Tree.Roots()) != 2 {
		t.Errorf("корней в дереве: %d, ждал 2", len(tw.Tree.Roots()))
	}

	lv, ok := reg["listReorder"].(*widget.ListView)
	if !ok {
		t.Fatalf("listReorder — %T", reg["listReorder"])
	}
	if !lv.Reorderable {
		t.Error("список показывает перестановку строк — нужен Reorderable")
	}

	if _, ok := reg["gridDemo"].(*widget.DataGridWidget); !ok {
		t.Errorf("gridDemo — %T, ждал *widget.DataGridWidget", reg["gridDemo"])
	}
	if _, ok := reg["dateDemo"].(*widget.DatePicker); !ok {
		t.Errorf("dateDemo — %T, ждал *widget.DatePicker", reg["dateDemo"])
	}
}

func TestShowcase_CompareTab(t *testing.T) {
	reg := showcaseReg(t)

	dv, ok := reg["diffDemo"].(*widget.DiffView)
	if !ok {
		t.Fatalf("diffDemo — %T", reg["diffDemo"])
	}
	if len(dv.Lines(widget.DiffLeft)) < 5 || len(dv.Lines(widget.DiffRight)) < 5 {
		t.Fatalf("стороны сравнения пусты — файлы-образцы не найдены: %d и %d строк",
			len(dv.Lines(widget.DiffLeft)), len(dv.Lines(widget.DiffRight)))
	}
	if dv.ChangeCount() == 0 {
		t.Error("в образцах нет различий — сравнивать нечего")
	}

	mv, ok := reg["mergeDemo"].(*widget.MergeView)
	if !ok {
		t.Fatalf("mergeDemo — %T", reg["mergeDemo"])
	}
	chunks := mv.Chunks()
	if len(chunks) == 0 {
		t.Fatal("блоков слияния нет — файлы-образцы не найдены")
	}
	conflicts := 0
	for _, c := range chunks {
		if c.Conflict {
			conflicts++
		}
	}
	if conflicts == 0 {
		t.Error("в образцах нет конфликта — показывать нечего")
	}
}
