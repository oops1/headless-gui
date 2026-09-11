// locale.go — строки приложения и выбор подсветки по расширению.
package main

import (
	"path/filepath"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"
)

func init() {
	widget.RegisterStrings("EN", map[string]string{
		"app.open.left":    "Left…",
		"app.open.right":   "Right…",
		"app.save":         "Save",
		"app.save.tip":     "Save modified sides (Ctrl+S — active side)",
		"app.title":        "File comparison — headless-gui",
		"app.undo":         "Undo",
		"app.redo":         "Redo",
		"app.undo.tip":     "Undo (Ctrl+Z)",
		"app.redo.tip":     "Redo (Ctrl+Y)",
		"app.prev.tip":     "Previous change (Shift+F7)",
		"app.next.tip":     "Next change (F7)",
		"app.hide":         "Hide identical",
		"app.ignoreWS":     "Ignore whitespace",
		"app.changes.0":    "Files are identical",
		"app.changes.1":    "%d change",
		"app.changes.2":    "%d changes",
		"app.changes.5":    "%d changes",
		"app.pick.left":    "Left file",
		"app.pick.right":   "Right file",
		"app.saveAs.left":  "Save left side as",
		"app.saveAs.right": "Save right side as",
		"app.err":          "Error",
		"app.saved":        "Saved: %s",
		"app.nothing":      "Nothing to save — no changes",
		"app.reloaded":     "Reloaded from disk: %s",
		"app.disk.title":   "File changed on disk",
		"app.disk.msg":     "%s was changed by another program.\nReload it? Unsaved edits will be lost.",
		"app.disk.deleted": "%s was deleted from disk",
		"app.side.left":    "Left",
		"app.side.right":   "Right",
		"app.status.pos":   "%s: line %d, col %d",
		"app.hint":         "F7 / Shift+F7 — changes · Alt+→ / Alt+← — copy block · Ctrl+S — save · drop files onto a pane",
	})
	widget.RegisterStrings("RU", map[string]string{
		"app.open.left":    "Левый…",
		"app.open.right":   "Правый…",
		"app.save":         "Сохранить",
		"app.save.tip":     "Сохранить изменённые стороны (Ctrl+S — активную)",
		"app.title":        "Сравнение файлов — headless-gui",
		"app.undo":         "Отменить",
		"app.redo":         "Повторить",
		"app.undo.tip":     "Отменить (Ctrl+Z)",
		"app.redo.tip":     "Повторить (Ctrl+Y)",
		"app.prev.tip":     "Предыдущее изменение (Shift+F7)",
		"app.next.tip":     "Следующее изменение (F7)",
		"app.hide":         "Скрыть совпадающие",
		"app.ignoreWS":     "Без учёта пробелов",
		"app.changes.0":    "Файлы совпадают",
		"app.changes.1":    "%d изменение",
		"app.changes.2":    "%d изменения",
		"app.changes.5":    "%d изменений",
		"app.pick.left":    "Левый файл",
		"app.pick.right":   "Правый файл",
		"app.saveAs.left":  "Сохранить левую сторону как",
		"app.saveAs.right": "Сохранить правую сторону как",
		"app.err":          "Ошибка",
		"app.saved":        "Сохранено: %s",
		"app.nothing":      "Нечего сохранять — изменений нет",
		"app.reloaded":     "Перечитан с диска: %s",
		"app.disk.title":   "Файл изменён на диске",
		"app.disk.msg":     "%s изменён другой программой.\nПеречитать? Несохранённые правки пропадут.",
		"app.disk.deleted": "%s удалён с диска",
		"app.side.left":    "Левый",
		"app.side.right":   "Правый",
		"app.status.pos":   "%s: стр. %d, кол. %d",
		"app.hint":         "F7 / Shift+F7 — изменения · Alt+→ / Alt+← — перенести блок · Ctrl+S — сохранить · файлы можно бросить на панель",
	})
}

var codeExt = map[string]bool{
	".go": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cs": true,
	".java": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true, ".py": true,
	".rs": true, ".kt": true, ".swift": true, ".php": true, ".rb": true, ".sh": true,
	".ps1": true, ".sql": true, ".lua": true, ".dart": true, ".scala": true,
}

func isCode(path string) bool { return codeExt[strings.ToLower(filepath.Ext(path))] }
