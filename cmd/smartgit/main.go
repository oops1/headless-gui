// smartgit — демо SmartGit-подобного UI: Window + TreeView + StackPanel + DataGrid.
//
// Демонстрирует работу новых виджетов:
//   - Window (корневой элемент нативного окна с заголовком Win-стиля)
//   - StackPanel (горизонтальная панель инструментов)
//   - TreeView (дерево репозиториев)
//   - DataGrid → ListView (лог коммитов)
//   - StatusBar → Panel (строка состояния)
//
// Запуск (из директории GuiEngine/window):
//
//	go run ../cmd/smartgit
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/window"
)

// findAsset ищет файл относительно текущей директории и относительно
// директории исполняемого файла — чтобы запуск работал из любой папки.
func findAsset(rel string) string {
	// 1. Относительно cwd
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	// 2. Относительно исполняемого файла
	exe, _ := os.Executable()
	if exe != "" {
		p := filepath.Join(filepath.Dir(exe), rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 3. Предположим запуск из корня проекта (go run ./cmd/smartgit)
	alt := filepath.Join("assets", "ui", filepath.Base(rel))
	if _, err := os.Stat(alt); err == nil {
		return alt
	}
	return rel // вернём как есть — ошибка будет при открытии
}

func main() {
	const (
		screenW = 1200
		screenH = 800
	)

	// ─── Движок ─────────────────────────────────────────────────────────────
	eng := engine.New(screenW, screenH, 30)

	// ─── Загрузка UI из XAML ────────────────────────────────────────────────
	root, reg, err := widget.LoadUIFromXAMLFile(findAsset("../assets/ui/smartgit.xaml"))
	if err != nil {
		log.Fatalf("ошибка загрузки smartgit.xaml: %v", err)
	}

	// ─── Вспомогательные функции ────────────────────────────────────────────
	btn := func(id string) *widget.Button {
		if w, ok := reg[id].(*widget.Button); ok {
			return w
		}
		return nil
	}
	lbl := func(id string) *widget.Label {
		if w, ok := reg[id].(*widget.Label); ok {
			return w
		}
		return nil
	}

	// ─── Toolbar кнопки ─────────────────────────────────────────────────────
	actions := []struct {
		id, name string
	}{
		{"btnPull", "Pull"},
		{"btnPush", "Push"},
		{"btnCommit", "Commit"},
		{"btnBranch", "Branch"},
		{"btnMerge", "Merge"},
		{"btnStash", "Stash"},
		{"btnFetch", "Fetch"},
	}
	for _, a := range actions {
		name := a.name
		if b := btn(a.id); b != nil {
			b.OnClick = func() {
				log.Printf("Action: %s", name)
				if l := lbl("lblStatus"); l != nil {
					l.SetText(fmt.Sprintf("Executing %s...", name))
				}
			}
		}
	}

	// ─── TreeView обработчик ────────────────────────────────────────────────
	if tv, ok := reg["repoTree"].(*widget.TreeView); ok {
		tv.OnSelect = func(node *widget.TreeNode) {
			log.Printf("TreeView select: %s", node.Text)
			if l := lbl("lblStatus"); l != nil {
				l.SetText(fmt.Sprintf("Selected: %s", node.Text))
			}
		}
	}

	// ─── Commit Log (ListView из DataGrid) ──────────────────────────────────
	if lv, ok := reg["commitLog"].(*widget.ListView); ok {
		// Добавляем демо-коммиты
		commits := []string{
			"*  |  feat: add Window widget with Win/Mac title bars  |  Валерий  |  2026-04-03  |  a1b2c3d",
			"*  |  feat: add StackPanel with auto-layout  |  Валерий  |  2026-04-03  |  e4f5g6h",
			"*  |  feat: add TreeView with expand/collapse  |  Валерий  |  2026-04-02  |  i7j8k9l",
			"|  |  fix: grid layout star sizing  |  Валерий  |  2026-04-01  |  m0n1o2p",
			"|  |  refactor: extract themes to separate file  |  Валерий  |  2026-03-31  |  q3r4s5t",
			"*  |  feat: add modal dialog support  |  Валерий  |  2026-03-30  |  u6v7w8x",
			"|  |  docs: update README with XAML examples  |  Валерий  |  2026-03-29  |  y9z0a1b",
		}
		for _, c := range commits {
			lv.AddItem(c)
		}

		lv.OnSelect = func(idx int, text string) {
			log.Printf("Commit selected: [%d] %s", idx, text)
			if ti, ok := reg["diffView"].(*widget.TextInput); ok {
				ti.SetText(fmt.Sprintf("diff --git for commit %d\n--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,5 @@\n+// New code added\n func main() {\n+    fmt.Println(\"hello\")\n }", idx))
			}
		}
	}

	// ─── Обработчик меню ───────────────────────────────────────────────────
	if menu, ok := reg["mainMenu"].(*widget.MenuBar); ok {
		menu.OnSelect = func(topIdx, subIdx int, text string) {
			log.Printf("Menu: [%d][%d] %s", topIdx, subIdx, text)

			switch text {
			// ── Тема ────────────────────────────────────────────────
			case "Dark":
				eng.SetTheme(widget.DarkTheme())
				log.Println("Theme: Dark")
			case "Light":
				eng.SetTheme(widget.LightTheme())
				log.Println("Theme: Light")

			// ── Стиль заголовка ─────────────────────────────────────
			case "Windows Style":
				if ww, ok := eng.Root().(*widget.Window); ok {
					ww.TitleStyle = widget.WindowTitleWin
					log.Println("TitleStyle: Windows")
				}
			case "Mac Style":
				if ww, ok := eng.Root().(*widget.Window); ok {
					ww.TitleStyle = widget.WindowTitleMac
					log.Println("TitleStyle: Mac")
				}

			// ── Выход ───────────────────────────────────────────────
			case "Exit":
				os.Exit(0)
			}
		}
	}

	// ─── Запуск движка ──────────────────────────────────────────────────────
	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	// ─── Живые данные ───────────────────────────────────────────────────────
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if l := lbl("lblBranch"); l != nil {
				l.SetText(fmt.Sprintf("HEAD: a1b2c3d | %s", time.Now().Format("15:04:05")))
			}
		}
	}()

	// ─── Нативное окно ──────────────────────────────────────────────────────
	win := window.New(eng, "SmartGit — Repository Browser")
	win.SetMaxFPS(60)

	if err := win.Run(); err != nil {
		log.Fatal(err)
	}
}
