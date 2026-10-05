// startmodel.go — данные меню «Пуск» с боковой панелью и плитками.
//
// Меню не знает, откуда берутся приложения, плитки и пункты боковой панели:
// всё приходит от потребителя типами и интерфейсами этого файла. Здесь же
// чистая арифметика, не привязанная к рисованию — разбиение списка по буквам и
// раскладка плиток по сетке, — чтобы её можно было проверять без кадра.
package desktop

import (
	"image"
	"image/color"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ─── Плитки ──────────────────────────────────────────────────────────────────

// TileSize — размер плитки в единицах сетки.
type TileSize int

const (
	// TileSmall — 1×1.
	TileSmall TileSize = iota
	// TileMedium — 2×2.
	TileMedium
	// TileWide — 4×2.
	TileWide
	// TileLarge — 4×4.
	TileLarge
)

// Units возвращает ширину и высоту плитки в единицах сетки. Неизвестный размер
// считается средним: плитка с неверным значением не должна исчезать.
func (s TileSize) Units() (cols, rows int) {
	switch s {
	case TileSmall:
		return 1, 1
	case TileWide:
		return 4, 2
	case TileLarge:
		return 4, 4
	}
	return 2, 2
}

// String — имя размера для сообщений и тестов.
func (s TileSize) String() string {
	switch s {
	case TileSmall:
		return "small"
	case TileWide:
		return "wide"
	case TileLarge:
		return "large"
	}
	return "medium"
}

// TileID — идентификатор плитки. Уникален в пределах меню: по нему обновляется
// содержимое (SetTileContent) и сообщается новый порядок.
type TileID string

// TileContent — что показывает плитка. Модель, а не виджет: потребитель
// обновляет содержимое, не трогая раскладку меню.
type TileContent struct {
	Title    string
	Subtitle string
	Icon     image.Image
	// IconAt — необязательный источник значка по размеру в физических
	// пикселях (как AppInfo.IconAt); при его наличии побеждает Icon.
	IconAt func(size int) image.Image
	// Badge — короткая метка в нижнем правом углу («3», «99+»).
	Badge string
	// Background — цвет плитки; нулевой — акцент темы.
	Background color.RGBA
}

// Tile — плитка в группе. Размер и порядок задаёт потребитель.
type Tile struct {
	ID TileID
	// App — приложение, которое запускает плитка (через AppCatalog.Launch).
	// Пустое — плитка запускается обработчиком StartMenu.OnTileLaunch.
	App     AppID
	Size    TileSize
	Content TileContent
}

// TileGroup — именованная группа плиток.
type TileGroup struct {
	ID    string
	Title string
	Tiles []Tile
}

// tilePlace — место плитки в сетке группы, в единицах.
type tilePlace struct{ Col, Row, Cols, Rows int }

// placeTiles раскладывает плитки по сетке шириной columns единиц: каждая
// следующая плитка занимает первое свободное место, куда она вмещается,
// просматривая сетку слева направо и сверху вниз, — так плитки переносятся на
// новую строку внутри группы, а мелкие заполняют пустоты. Возвращает места в
// порядке плиток и число занятых строк.
//
// Плитка шире сетки сжимается до её ширины: узкое меню не должно терять плитки.
func placeTiles(sizes []TileSize, columns int) (places []tilePlace, rows int) {
	if columns < 1 {
		columns = 1
	}
	var occ [][]bool // [строка][столбец]
	grow := func(n int) {
		for len(occ) < n {
			occ = append(occ, make([]bool, columns))
		}
	}
	fits := func(col, row, w, h int) bool {
		if col+w > columns {
			return false
		}
		grow(row + h)
		for y := row; y < row+h; y++ {
			for x := col; x < col+w; x++ {
				if occ[y][x] {
					return false
				}
			}
		}
		return true
	}
	places = make([]tilePlace, 0, len(sizes))
	for _, sz := range sizes {
		w, h := sz.Units()
		if w > columns {
			w = columns
		}
	search:
		for row := 0; ; row++ {
			for col := 0; col+w <= columns; col++ {
				if fits(col, row, w, h) {
					for y := row; y < row+h; y++ {
						for x := col; x < col+w; x++ {
							occ[y][x] = true
						}
					}
					places = append(places, tilePlace{col, row, w, h})
					break search
				}
			}
		}
	}
	for _, p := range places {
		if p.Row+p.Rows > rows {
			rows = p.Row + p.Rows
		}
	}
	return places, rows
}

// ─── Список приложений ───────────────────────────────────────────────────────

// StartEntry — строка списка приложений: приложение или папка.
type StartEntry struct {
	ID    AppID
	Title string
	// Subtitle — необязательная вторая строка («Система»).
	Subtitle string
	Icon     image.Image
	// IconAt — значок по размеру (см. AppInfo.IconAt).
	IconAt func(size int) image.Image
	// Folder — строка раскрывается на месте и показывает Children. Папка без
	// дочерних строк (Children пуст) тоже считается папкой, если Folder
	// истинно.
	Folder   bool
	Children []StartEntry
	// Key — идентификатор папки (раскрытие переживает пересборку списка).
	// Пустой у папки — берётся Title.
	Key string
}

// IsFolder сообщает, раскрывается ли строка.
func (e StartEntry) IsFolder() bool { return e.Folder || len(e.Children) > 0 }

// folderKey — устойчивое имя папки для запоминания раскрытия.
func (e StartEntry) folderKey() string {
	if e.Key != "" {
		return e.Key
	}
	return e.Title
}

// StartLetterGroup — алфавитная группа списка: заголовок-буква и её строки.
type StartLetterGroup struct {
	// Letter — заголовок группы: «#», «A», «Я».
	Letter  string
	Entries []StartEntry
}

// StartMenuSource — данные списка приложений меню «Пуск» с плитками.
//
// Необязателен: без него меню строит группы само из AppCatalog (по первой
// букве названия), без раздела «Недавно добавленные». Реализуется, когда
// потребитель хочет свои группы, папки, вторую строку или недавние
// приложения. Subscribe зовёт замыкание из горутины потребителя — см. раздел
// «Из какой горутины что зовётся» в описании пакета.
type StartMenuSource interface {
	// Recent — раздел «Недавно добавленные» (пусто — раздела нет).
	Recent() []StartEntry
	// Groups — алфавитные группы в порядке показа.
	Groups() []StartLetterGroup
	Subscribe(func()) func()
}

// StartLetter возвращает букву-заголовок для названия: «#» для цифр и знаков,
// заглавная буква для остальных.
func StartLetter(title string) string {
	for _, r := range strings.TrimSpace(title) {
		if unicode.IsLetter(r) {
			return string(unicode.ToUpper(r))
		}
		return "#"
	}
	return "#"
}

// letterRank упорядочивает заголовки как в Windows: «#», латиница, кириллица,
// остальное.
func letterRank(letter string) int {
	r, _ := utf8.DecodeRuneInString(letter)
	switch {
	case letter == "" || letter == "#":
		return 0
	case r >= 'A' && r <= 'Z':
		return 1
	case r >= 'А' && r <= 'Я', r == 'Ё':
		return 2
	}
	return 3
}

// GroupByLetter разбивает записи по первой букве названия: группы
// отсортированы («#», A–Z, А–Я, прочие), внутри группы записи упорядочены по
// названию без учёта регистра. Исходный срез не меняется.
func GroupByLetter(entries []StartEntry) []StartLetterGroup {
	by := map[string][]StartEntry{}
	for _, e := range entries {
		l := StartLetter(e.Title)
		by[l] = append(by[l], e)
	}
	letters := make([]string, 0, len(by))
	for l := range by {
		letters = append(letters, l)
	}
	sort.Slice(letters, func(i, j int) bool {
		ri, rj := letterRank(letters[i]), letterRank(letters[j])
		if ri != rj {
			return ri < rj
		}
		return letters[i] < letters[j]
	})
	out := make([]StartLetterGroup, 0, len(letters))
	for _, l := range letters {
		list := by[l]
		sort.SliceStable(list, func(i, j int) bool {
			return strings.ToLower(list[i].Title) < strings.ToLower(list[j].Title)
		})
		out = append(out, StartLetterGroup{Letter: l, Entries: list})
	}
	return out
}

// catalogSource — источник по умолчанию: каталог приложений, сгруппированный по
// буквам. Недавние берёт у каталога, если тот умеет их назвать.
type catalogSource struct{ cat AppCatalog }

// NewCatalogSource строит источник меню из AppCatalog: алфавитные группы по
// названиям приложений. Если каталог реализует Recent() []AppID, из этих
// приложений составляется раздел «Недавно добавленные»; если Subscribe(func())
// func(), меню узнаёт об изменениях каталога.
func NewCatalogSource(cat AppCatalog) StartMenuSource { return catalogSource{cat: cat} }

func (s catalogSource) entries() []StartEntry {
	if s.cat == nil {
		return nil
	}
	apps := s.cat.Apps()
	out := make([]StartEntry, 0, len(apps))
	for _, a := range apps {
		out = append(out, StartEntry{ID: a.ID, Title: a.Title, Icon: a.Icon, IconAt: a.IconAt})
	}
	return out
}

func (s catalogSource) Recent() []StartEntry {
	r, ok := s.cat.(interface{ Recent() []AppID })
	if !ok {
		return nil
	}
	byID := map[AppID]StartEntry{}
	for _, e := range s.entries() {
		byID[e.ID] = e
	}
	var out []StartEntry
	for _, id := range r.Recent() {
		if e, ok := byID[id]; ok {
			out = append(out, e)
		}
	}
	return out
}

func (s catalogSource) Groups() []StartLetterGroup { return GroupByLetter(s.entries()) }

func (s catalogSource) Subscribe(fn func()) func() {
	if sub, ok := s.cat.(interface{ Subscribe(func()) func() }); ok {
		return sub.Subscribe(fn)
	}
	return func() {}
}

// ─── Боковая панель ──────────────────────────────────────────────────────────

// StartSidebarItem — пункт боковой панели: пользователь, документы, параметры,
// питание. Состав и порядок — от потребителя. Гамбургер в начале панели есть
// всегда и в список не входит.
type StartSidebarItem struct {
	ID    string
	Title string
	// Icon, IconAt — собственная картинка пункта; Glyph — встроенный значок, если
	// картинки нет.
	Icon   image.Image
	IconAt func(size int) image.Image
	Glyph  StartGlyph
	// KeepOpen — меню не закрывается после выбора пункта (например, у «Питания»,
	// которое открывает собственный список).
	KeepOpen bool
}

// ─── Цели действий ───────────────────────────────────────────────────────────

// StartTargetKind — на что пришлось действие пользователя.
type StartTargetKind int

const (
	// StartTargetApp — строка приложения списка.
	StartTargetApp StartTargetKind = iota
	// StartTargetFolder — строка папки.
	StartTargetFolder
	// StartTargetTile — плитка.
	StartTargetTile
	// StartTargetSidebar — пункт боковой панели.
	StartTargetSidebar
	// StartTargetResult — результат поиска.
	StartTargetResult
	// StartTargetPinned — ячейка закреплённого в меню Windows 11.
	StartTargetPinned
	// StartTargetRecommended — строка «Рекомендуем» в меню Windows 11.
	StartTargetRecommended
	// StartTargetUser — пользователь нижней полосы меню Windows 11.
	StartTargetUser
	// StartTargetPower — кнопка питания нижней полосы меню Windows 11.
	StartTargetPower
)

// StartTarget описывает объект, на котором пользователь нажал правую кнопку:
// по нему потребитель собирает контекстное меню.
type StartTarget struct {
	Kind StartTargetKind
	// App — приложение (строка списка или плитка с приложением).
	App AppID
	// Tile — плитка.
	Tile TileID
	// Folder — ключ папки.
	Folder string
	// Sidebar — идентификатор пункта боковой панели.
	Sidebar string
	// Result — результат поиска.
	Result SearchResult
	// Pinned — идентификатор закреплённого (StartPinned.ID); App — его
	// приложение, если оно есть.
	Pinned string
	// Recommended — идентификатор строки «Рекомендуем»
	// (StartRecommendedItem.ID).
	Recommended string
}
