// datagrid — демо DataGrid: полноценный табличный виджет с биндингом, сортировкой и редактированием.
//
// Демонстрирует все возможности DataGrid:
//   - Загрузка XAML с объявлением DataGrid и колонок
//   - Data Binding: ObservableCollection + INotifyPropertyChanged
//   - Сортировка по клику на заголовок колонки (▲/▼)
//   - Inline-редактирование ячеек (двойной клик / Enter, Esc — отмена)
//   - Выделение строк (Single, Extended: Ctrl+Click, Shift+Click)
//   - Навигация клавиатурой (стрелки, Tab, Home/End, PageUp/PageDown)
//   - Resize колонок перетаскиванием границы заголовка
//   - Авто-генерация колонок из структуры
//   - Observable: динамическое добавление/удаление строк
//   - Переключение тем (Dark/Light) через меню
//   - Виртуализация строк (рисуются только видимые)
//
// XAML layout:  assets/ui/datagrid.xaml (не изменяется)
//
// Запуск:
//
//	go run ./cmd/datagrid
package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
	"github.com/oops1/headless-gui/v3/window"
)

// ─── Модель данных ─────────────────────────────────────────────────────────

// User — модель строки таблицы (структура с экспортированными полями).
// DataGrid обращается к полям через reflection по Binding Path.
type User struct {
	datagrid.PropertyNotifier // встраиваем для INotifyPropertyChanged

	Name     string
	Age      int
	IsActive bool
	Email    string
	Role     string
	City     string
}

// sampleUsers возвращает набор демо-данных.
func sampleUsers() []*User {
	return []*User{
		{Name: "Алексей Петров", Age: 32, IsActive: true, Email: "alex@example.com", Role: "Developer", City: "Москва"},
		{Name: "Мария Иванова", Age: 28, IsActive: true, Email: "maria@example.com", Role: "Designer", City: "Санкт-Петербург"},
		{Name: "Дмитрий Козлов", Age: 45, IsActive: false, Email: "dmitry@example.com", Role: "Manager", City: "Казань"},
		{Name: "Елена Сидорова", Age: 35, IsActive: true, Email: "elena@example.com", Role: "QA Engineer", City: "Новосибирск"},
		{Name: "Игорь Николаев", Age: 29, IsActive: true, Email: "igor@example.com", Role: "Developer", City: "Екатеринбург"},
		{Name: "Анна Волкова", Age: 41, IsActive: false, Email: "anna@example.com", Role: "Team Lead", City: "Москва"},
		{Name: "Сергей Морозов", Age: 33, IsActive: true, Email: "sergey@example.com", Role: "DevOps", City: "Нижний Новгород"},
		{Name: "Ольга Кузнецова", Age: 27, IsActive: true, Email: "olga@example.com", Role: "Developer", City: "Воронеж"},
		{Name: "Андрей Лебедев", Age: 38, IsActive: false, Email: "andrey@example.com", Role: "Architect", City: "Краснодар"},
		{Name: "Наталья Новикова", Age: 31, IsActive: true, Email: "natasha@example.com", Role: "Analyst", City: "Самара"},
		{Name: "Павел Соколов", Age: 26, IsActive: true, Email: "pavel@example.com", Role: "Intern", City: "Москва"},
		{Name: "Виктория Попова", Age: 44, IsActive: true, Email: "vika@example.com", Role: "HR Manager", City: "Казань"},
		{Name: "Роман Васильев", Age: 30, IsActive: false, Email: "roman@example.com", Role: "Developer", City: "Санкт-Петербург"},
		{Name: "Татьяна Михайлова", Age: 36, IsActive: true, Email: "tanya@example.com", Role: "PM", City: "Москва"},
		{Name: "Кирилл Федоров", Age: 25, IsActive: true, Email: "kirill@example.com", Role: "Developer", City: "Тюмень"},
	}
}

// ─── Helper ────────────────────────────────────────────────────────────────

func findAsset(rel string) string {
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	exe, _ := os.Executable()
	if exe != "" {
		p := filepath.Join(filepath.Dir(exe), rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	alt := filepath.Join("assets", "ui", filepath.Base(rel))
	if _, err := os.Stat(alt); err == nil {
		return alt
	}
	return rel
}

// ─── main ──────────────────────────────────────────────────────────────────

func main() {
	const (
		screenW = 600
		screenH = 400
	)

	// ─── Движок ─────────────────────────────────────────────────────────────
	eng := engine.New(screenW, screenH, 30)

	// ─── Загрузка UI из XAML ────────────────────────────────────────────────
	root, _, err := widget.LoadUIFromXAMLFile(findAsset("../assets/ui/datagrid.xaml"))
	if err != nil {
		log.Fatalf("ошибка загрузки datagrid.xaml: %v", err)
	}

	// ─── Подготовка данных ──────────────────────────────────────────────────

	// Создаём ObservableCollection из демо-данных
	users := sampleUsers()
	collection := datagrid.NewObservableCollection()
	for _, u := range users {
		collection.Add(u)
	}

	// ─── Привязка данных к DataGrid ─────────────────────────────────────────

	// Ищем DataGridWidget в реестре виджетов.
	// XAML не задаёт Name для DataGrid, поэтому обходим дерево.
	dgWidget := findDataGrid(root)
	if dgWidget != nil {
		// Привязываем данные
		dgWidget.Grid.SetItemsSource(collection)

		// ── Обработчик выделения ────────────────────────────────────────
		dgWidget.Grid.OnSelectionChanged = func(e datagrid.SelectionChangedEvent) {
			if u, ok := e.SelectedItem.(*User); ok {
				log.Printf("Выделен: %s (%s, %s)", u.Name, u.Role, u.City)
			}
		}

		// ── Обработчик сортировки ───────────────────────────────────────
		dgWidget.Grid.OnSorting = func(e *datagrid.SortingEvent) {
			dir := "▲"
			if e.Direction == datagrid.SortDescending {
				dir = "▼"
			}
			log.Printf("Сортировка: %s %s", e.Column.Header(), dir)
			// Handled=false → DataGrid выполнит стандартную сортировку
		}

		// ── Обработчик редактирования ───────────────────────────────────
		dgWidget.Grid.OnCellEditEnding = func(e *datagrid.CellEditEndingEvent) {
			log.Printf("Ячейка изменена: строка %d, колонка '%s', новое значение: '%s'",
				e.RowIndex, e.Column.Header(), e.NewValue)
			// Cancel = false → значение будет записано в модель
		}

		dgWidget.Grid.OnRowEditEnding = func(rowIndex int, item interface{}) {
			if u, ok := item.(*User); ok {
				log.Printf("Строка сохранена: %s (возраст: %d, активен: %v)", u.Name, u.Age, u.IsActive)
			}
		}

		// Устанавливаем фокус на DataGrid
		eng.SetFocus(dgWidget)

		log.Printf("DataGrid: %d колонок, %d строк", len(dgWidget.Grid.Columns()), collection.Count())
	} else {
		log.Println("WARN: DataGridWidget не найден в дереве виджетов")
	}

	// ─── Запуск движка ──────────────────────────────────────────────────────
	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	// ─── Динамическое обновление данных ─────────────────────────────────────
	// Каждые 3 секунды добавляем нового пользователя (демонстрация ObservableCollection)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		counter := 0
		cities := []string{"Москва", "СПб", "Казань", "Сочи", "Омск", "Пермь"}
		roles := []string{"Developer", "QA", "Designer", "PM", "DevOps", "Analyst"}

		for range ticker.C {
			counter++
			newUser := &User{
				Name:     fmt.Sprintf("Новый_%d", counter),
				Age:      20 + rand.Intn(30),
				IsActive: rand.Intn(2) == 1,
				Email:    fmt.Sprintf("new%d@example.com", counter),
				Role:     roles[rand.Intn(len(roles))],
				City:     cities[rand.Intn(len(cities))],
			}
			collection.Add(newUser)
			log.Printf("Добавлен: %s (%s) — всего строк: %d", newUser.Name, newUser.City, collection.Count())
		}
	}()

	// ─── Нативное окно ──────────────────────────────────────────────────────
	win := window.New(eng, "DataGrid Demo — все возможности")
	win.SetMaxFPS(60)

	log.Println("═══════════════════════════════════════════════════════")
	log.Println(" DataGrid Demo")
	log.Println("═══════════════════════════════════════════════════════")
	log.Println(" Управление:")
	log.Println("   ↑↓        — навигация по строкам")
	log.Println("   ←→        — навигация по колонкам")
	log.Println("   Tab       — следующая ячейка")
	log.Println("   Home/End  — первая/последняя строка")
	log.Println("   PgUp/PgDn — страница вверх/вниз")
	log.Println("   Enter     — редактирование ячейки")
	log.Println("   Esc       — отмена редактирования")
	log.Println("   Клик      — заголовок → сортировка")
	log.Println("   Двойной клик — редактирование")
	log.Println("   Drag      — граница заголовка → resize колонки")
	log.Println("   Скроллбар — вертикальная прокрутка")
	log.Println("═══════════════════════════════════════════════════════")
	log.Println(" Каждые 3 сек добавляется новый пользователь (Observable)")
	log.Println("═══════════════════════════════════════════════════════")

	if err := win.Run(); err != nil {
		log.Fatal(err)
	}
}

// findDataGrid рекурсивно ищет DataGridWidget в дереве виджетов.
func findDataGrid(w widget.Widget) *widget.DataGridWidget {
	if dg, ok := w.(*widget.DataGridWidget); ok {
		return dg
	}
	for _, child := range w.Children() {
		if found := findDataGrid(child); found != nil {
			return found
		}
	}
	return nil
}
