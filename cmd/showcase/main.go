// showcase — полная демонстрация всех виджетов GuiEngine в нативном окне.
//
// Загружает showcase.xaml и подключает обработчики событий
// ко всем интерактивным виджетам.
//
// Запуск (из директории GuiEngine/window):
//
//	go run ../cmd/showcase
//
// Бинарник без консоли (Windows):
//
//	go build -ldflags="-H windowsgui" -o showcase.exe ../cmd/showcase
package main

import (
	"fmt"
	"image"
	"log"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	dg "github.com/oops1/headless-gui/v3/widget/datagrid"
	"github.com/oops1/headless-gui/v3/window"
)

// ─── DataContext для вкладки «3.2.4» (новый функционал) ──────────────────────

// Order — элемент списка для ItemsControl + DataTemplate.
type Order struct {
	Side   string
	Symbol string
	Price  float64
}

// Trader — элемент для CollectionView + VirtualizingItemsControl (вкладка 3.2.5).
type Trader struct {
	Name string
	Age  int
}

// showcaseVM — модель с уведомлениями для живого биндинга/триггеров/команд.
type showcaseVM struct {
	dg.PropertyNotifier
	username string
	status   string
	Items    *dg.ObservableCollection
	SaveCommand *widget.RelayCommand

	// Вкладка 3.2.5
	Email  string                 // валидируется через DataError (IDataErrorInfo)
	People *widget.CollectionView // источник для CollectionView/виртуализации
}

func (v *showcaseVM) GetUsername() string  { return v.username }
func (v *showcaseVM) SetUsername(s string) { v.username = s; v.NotifyPropertyChanged(v, "Username") }
func (v *showcaseVM) GetStatus() string    { return v.status }
func (v *showcaseVM) SetStatus(s string)   { v.status = s; v.NotifyPropertyChanged(v, "Status") }

// DataError реализует widget.DataErrorInfo (аналог WPF IDataErrorInfo).
func (v *showcaseVM) DataError(prop string) string {
	if prop == "Email" {
		if v.Email == "" {
			return "E-mail не может быть пустым"
		}
		if !strings.Contains(v.Email, "@") {
			return "E-mail должен содержать «@»"
		}
	}
	return ""
}

// pctConv — конвертер доли 0..1 → проценты (для {Binding ..., Converter=}).
type pctConv struct{}

func (pctConv) Convert(v interface{}) interface{} {
	f, _ := v.(float64)
	return fmt.Sprintf("%.0f%%", f*100)
}
func (pctConv) ConvertBack(v interface{}) interface{} { return v }

func main() {
	const (
		screenW = 1280
		screenH = 900
	)

	// ─── Движок ─────────────────────────────────────────────────────────────
	eng := engine.New(screenW, screenH, 30)

	// ─── DataContext + локаль + конвертер (для вкладки «3.2.4») ──────────────
	widget.RegisterValueConverter("Pct", pctConv{})

	// Локализованные строки для вкладки 3.2.5 (динамическое переключение ЯЗЫКА
	// ИНТЕРФЕЙСА — независимо от раскладки клавиатуры).
	widget.SetFallbackLanguage("EN")
	widget.RegisterStrings("EN", map[string]string{
		"T325_Header":  "Localization (dynamic {Loc})",
		"T325_Hello":   "Hello! Switch the language →",
		"T325_Save":    "Save",
		"T325_Hint":    "Buttons change widget.Locale — {Loc} strings update live",
		"T325_LangTip": "Switch UI language",
	})
	widget.RegisterStrings("RU", map[string]string{
		"T325_Header":  "Локализация (динамические {Loc})",
		"T325_Hello":   "Привет! Переключите язык →",
		"T325_Save":    "Сохранить",
		"T325_Hint":    "Кнопки меняют язык интерфейса (widget.Language) — строки {Loc} обновляются вживую",
		"T325_LangTip": "Сменить язык интерфейса (не раскладку клавиатуры)",
	})
	widget.SetLanguage("RU") // язык интерфейса (надписи)
	widget.SetLocale("RU")   // раскладка клавиатуры (индикатор) — независимо

	items := dg.NewObservableCollection()
	items.Add(Order{"BUY", "BTCUSDT", 64231.5})
	items.Add(Order{"SELL", "ETHUSDT", 3120.0})
	items.Add(Order{"BUY", "SOLUSDT", 148.25})

	// 1000 трейдеров для CollectionView + виртуализации (вкладка 3.2.5).
	traders := make([]interface{}, 1000)
	firstNames := []string{"Alice", "Bob", "Carol", "Dmitry", "Elena", "Igor", "Olga", "Pavel"}
	for i := range traders {
		traders[i] = &Trader{
			Name: fmt.Sprintf("%s #%d", firstNames[i%len(firstNames)], i),
			Age:  18 + (i*7)%50,
		}
	}
	people := widget.NewCollectionView(dg.NewObservableCollectionFrom(traders))

	vm := &showcaseVM{
		username: "trader",
		status:   "Готов — Ctrl+S для сохранения",
		Items:    items,
		Email:    "user@example.com",
		People:   people,
	}
	vm.SaveCommand = widget.NewRelayCommand(func() {
		vm.SetStatus("Сохранено в " + time.Now().Format("15:04:05"))
	})

	// ─── Загрузка UI из XAML (с DataContext для живых привязок) ──────────────
	root, reg, _, err := widget.LoadUIFromXAMLFileBindings("./assets/ui/showcase.xaml", vm)
	if err != nil {
		log.Fatalf("ошибка загрузки showcase.xaml: %v", err)
	}

	// ─── Вспомогательные функции ────────────────────────────────────────────
	get := func(id string) widget.Widget {
		if w, ok := reg[id]; ok {
			return w
		}
		return nil
	}
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
	slider := func(id string) *widget.Slider {
		if w, ok := reg[id].(*widget.Slider); ok {
			return w
		}
		return nil
	}
	toggle := func(id string) *widget.ToggleSwitch {
		if w, ok := reg[id].(*widget.ToggleSwitch); ok {
			return w
		}
		return nil
	}
	cb := func(id string) *widget.CheckBox {
		if w, ok := reg[id].(*widget.CheckBox); ok {
			return w
		}
		return nil
	}

	eventLog, _ := reg["eventLog"].(*widget.ListView)
	_ = get // suppress unused

	addLog := func(format string, args ...any) {
		msg := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
		if eventLog != nil {
			eventLog.AddItem(msg)
		}
		log.Println(msg)
	}

	// win заполняется ниже (перед Run); нужен обработчикам меню для «Выход».
	var win *window.Window

	// ─── MenuBar ────────────────────────────────────────────────────────────
	if menu, ok := reg["mainMenu"].(*widget.MenuBar); ok {
		menu.OnSelect = func(topIdx, subIdx int, text string) {
			addLog("Меню: %s (раздел %d, пункт %d)", text, topIdx, subIdx)
			if text == "Выход" && win != nil {
				win.Close() // штатное закрытие нативного окна → Run() вернётся
			}
		}
	}

	// ─── Смена темы (Win10/Win11/Win2000/Mac) ────────────────────────────────
	// applyHeaderStyle подгоняет шапку приложения под тему: в классике —
	// градиентный заголовок Win2000 (navy→голубой) с белым жирным текстом.
	applyHeaderStyle := func(t *widget.Theme) {
		hdr, _ := reg["header"].(*widget.Panel)
		title, _ := reg["headerTitle"].(*widget.Label)
		if hdr == nil || title == nil {
			return
		}
		if t.Style.Classic3D {
			hdr.Gradient = &widget.LinearGradient{
				Horizontal: true,
				Stops: []widget.GradientStop{
					{Offset: 0, Color: t.TitleBG},
					{Offset: 1, Color: t.TitleBG2},
				},
			}
			title.TextColor = t.TitleText
			title.Bold = true
		} else {
			hdr.Gradient = nil
			title.TextColor = t.TitleText
			title.Bold = false
		}
		if clock, ok := reg["headerClock"].(*widget.Label); ok {
			clock.TextColor = t.TitleText
		}
	}
	// Живое превью оформления окна (вкладка 3.2.5): настоящий widget.Window
	// внутри showcase, который перерисовывается вместе с темой — видно
	// скругление (Win11/Mac), traffic-lights (Mac), кнопки/градиент (Win2000).
	if mount, ok := reg["winPreviewMount"].(*widget.Panel); ok {
		prev := &widget.Window{
			Title:  "Свойства системы",
			Style:  widget.WindowStyleSingleBorder,
			Resize: widget.ResizeModeCanResize,
		}
		mb := mount.Bounds()
		prev.SetBounds(mb)
		info := widget.NewLabel("Microsoft Windows · GuiEngine", widget.CurrentThemeStyle().BevelDark)
		info.SetBounds(image.Rect(mb.Min.X+14, mb.Min.Y+44, mb.Max.X-14, mb.Min.Y+64))
		prev.AddChild(info)
		okBtn := widget.NewButton("OK")
		okBtn.SetBounds(image.Rect(mb.Max.X-100, mb.Max.Y-40, mb.Max.X-12, mb.Max.Y-12))
		prev.AddChild(okBtn)
		mount.AddChild(prev)
	}

	if dd, ok := reg["themeSelect"].(*widget.Dropdown); ok {
		dd.OnChange = func(_ int, name string) {
			if t := widget.ThemeByName(name); t != nil {
				eng.SetTheme(t)
				applyHeaderStyle(t)
				// Скругление НАСТОЯЩЕГО окна ОС по теме (Win11/Mac — скруглённое).
				if win != nil {
					win.SetCornerRadius(t.Style.WindowCorner)
				}
				addLog("Тема: %s", name)
			}
		}
	}

	// ─── TAB 1: Ввод данных — Кнопки ────────────────────────────────────────
	if b := btn("btnAccent"); b != nil {
		b.OnClick = func() {
			login := ""
			if ti, ok := reg["txtLogin"].(*widget.TextInput); ok {
				login = ti.GetText()
			}
			addLog("Вход: user=%s", login)
		}
	}
	if b := btn("btnDefault"); b != nil {
		b.OnClick = func() {
			addLog("Отмена нажата")
		}
	}
	if b := btn("btnDanger"); b != nil {
		b.OnClick = func() {
			addLog("Удалить нажата (опасное действие)")
		}
	}
	if b := btn("btnDisabled"); b != nil {
		b.OnClick = func() {
			// Сброс формы
			if ti, ok := reg["txtLogin"].(*widget.TextInput); ok {
				ti.SetText("")
			}
			if ti, ok := reg["txtPassword"].(*widget.TextInput); ok {
				ti.SetText("")
			}
			if ti, ok := reg["txtComment"].(*widget.TextInput); ok {
				ti.SetText("")
			}
			addLog("Форма сброшена")
		}
	}
	if b := btn("btnExport"); b != nil {
		b.OnClick = func() {
			addLog("Экспорт настроек...")
		}
	}

	// ─── PopupMenu ──────────────────────────────────────────────────────────
	if pm, ok := reg["ctxMenu"].(*widget.PopupMenu); ok {
		pm.OnSelect = func(idx int, text string) {
			addLog("PopupMenu: «%s» (idx=%d)", text, idx)
		}

		if b := btn("btnShowPopup"); b != nil {
			b.OnClick = func() {
				pm.ShowBelow(b)
				addLog("PopupMenu открыто (ShowBelow)")
			}
		}
		if b := btn("btnShowPopup2"); b != nil {
			b.OnClick = func() {
				pm.ShowRight(b)
				addLog("PopupMenu открыто (ShowRight)")
			}
		}
	}

	// ─── TAB 2: Элементы управления ──────────────────────────────────────────

	// CheckBox
	for _, pair := range [][2]string{
		{"cbRemember", "Запомнить"},
		{"cbAutoConnect", "Автоподключение"},
		{"cbVerbose", "Verbose"},
		{"cbCompress", "Сжатие"},
	} {
		id, name := pair[0], pair[1]
		if c := cb(id); c != nil {
			n := name // capture
			c.OnChange = func(checked bool) {
				addLog("CheckBox «%s»: %v", n, checked)
			}
		}
	}

	// RadioButton
	for _, pair := range [][2]string{
		{"rbLDAP", "LDAP"},
		{"rbOTP", "OTP"},
		{"rbCert", "Сертификат"},
		{"rbHigh", "Высокое"},
		{"rbMedium", "Среднее"},
		{"rbLow", "Низкое"},
	} {
		id, name := pair[0], pair[1]
		if rb, ok := reg[id].(*widget.RadioButton); ok {
			n := name
			rb.OnChange = func(selected bool) {
				if selected {
					addLog("RadioButton: %s", n)
				}
			}
		}
	}

	// ToggleSwitch
	for _, pair := range [][2]string{
		{"tsAutoRefresh", "Авто-обновление"},
		{"tsNotify", "Уведомления"},
		{"tsDarkMode", "Тёмная тема"},
		{"tsFullscreen", "Полноэкранный режим"},
		{"tsSmooth", "Сглаживание"},
		{"tsAudio", "Аудио"},
	} {
		id, name := pair[0], pair[1]
		if ts := toggle(id); ts != nil {
			n := name
			ts.OnChange = func(on bool) {
				state := "ВЫКЛ"
				if on {
					state = "ВКЛ"
				}
				addLog("Toggle «%s»: %s", n, state)
			}
		}
	}

	// Slider: скорость
	if s := slider("sliderSpeed"); s != nil {
		s.OnChange = func(v float64) {
			if l := lbl("lblSpeedVal"); l != nil {
				l.SetText(fmt.Sprintf("%.0f FPS", v))
			}
		}
	}
	// Slider: качество
	if s := slider("sliderQuality"); s != nil {
		s.OnChange = func(v float64) {
			if l := lbl("lblQualityVal"); l != nil {
				l.SetText(fmt.Sprintf("%.0f%%", v))
			}
		}
	}
	// Slider: громкость
	if s := slider("sliderVolume"); s != nil {
		s.OnChange = func(v float64) {
			if l := lbl("lblVolumeVal"); l != nil {
				l.SetText(fmt.Sprintf("%.0f", v))
			}
		}
	}
	// Slider: воспроизведение (внутри таба)
	if s := slider("sliderPlayback"); s != nil {
		s.OnChange = func(v float64) {
			addLog("Воспроизведение: %.0f%%", v)
		}
	}
	// Slider: запись
	if s := slider("sliderRecord"); s != nil {
		s.OnChange = func(v float64) {
			addLog("Запись: %.0f%%", v)
		}
	}

	// ─── TAB 3: Данные ───────────────────────────────────────────────────────

	// ListView кнопки
	if b := btn("btnAddEvent"); b != nil {
		b.OnClick = func() {
			addLog("Событие добавлено пользователем")
		}
	}
	if b := btn("btnClearLog"); b != nil {
		b.OnClick = func() {
			if eventLog != nil {
				eventLog.Clear()
				addLog("Журнал очищен")
			}
		}
	}

	// TabControl обработчик
	if tc, ok := reg["mainTabs"].(*widget.TabControl); ok {
		tc.OnTabChange = func(idx int, header string) {
			addLog("Вкладка: %s (%d)", header, idx)
		}
	}
	if tc, ok := reg["innerTabs"].(*widget.TabControl); ok {
		tc.OnTabChange = func(idx int, header string) {
			addLog("Внутренняя вкладка: %s", header)
		}
	}

	// Dropdown
	if dd, ok := reg["ddRole"].(*widget.Dropdown); ok {
		dd.OnChange = func(idx int, text string) {
			addLog("Роль: %s", text)
		}
	}
	if dd, ok := reg["ddProtocol"].(*widget.Dropdown); ok {
		dd.OnChange = func(idx int, text string) {
			addLog("Протокол: %s", text)
		}
	}

	// ListView select
	if eventLog != nil {
		eventLog.OnSelect = func(idx int, text string) {
			log.Printf("ListView select: [%d] %q", idx, text)
		}
	}

	// ─── TAB 3.2.4: новый функционал ─────────────────────────────────────────
	if b := btn("errBtn324"); b != nil {
		b.OnClick = func() {
			vm.SetStatus("ERROR") // DataTrigger покрасит статус в красный
			addLog("3.2.4: статус → ERROR (DataTrigger)")
		}
	}
	if b := btn("addBtn324"); b != nil {
		b.OnClick = func() {
			vm.Items.Add(Order{"BUY", "NEWPAIR", 1.23}) // ItemsControl обновится вживую
			addLog("3.2.4: строка добавлена (live ItemsControl)")
		}
	}
	if b := btn("saveBtn324"); b != nil {
		b.AddClickHandler(func() { addLog("3.2.4: команда Save выполнена") })
	}

	// ─── TAB 3.2.5: локализация / Tier B ─────────────────────────────────────
	if b := btn("langRu325"); b != nil {
		b.OnClick = func() {
			widget.SetLanguage("RU")
			addLog("3.2.5: язык интерфейса → RU ({Loc} обновлены вживую)")
		}
	}
	if b := btn("langEn325"); b != nil {
		b.OnClick = func() {
			widget.SetLanguage("EN")
			addLog("3.2.5: язык интерфейса → EN ({Loc} обновлены вживую)")
		}
	}
	if b := btn("save325"); b != nil {
		b.OnClick = func() { addLog("3.2.5: Save (локализованная кнопка)") }
	}
	if nud, ok := reg["qty325"].(*widget.NumericUpDown); ok {
		nud.OnChange = func(v float64) { addLog("3.2.5: количество = %.0f", v) }
	}
	if nud, ok := reg["price325"].(*widget.NumericUpDown); ok {
		nud.OnChange = func(v float64) { addLog("3.2.5: цена = %.2f", v) }
	}
	if b := btn("vfilter325"); b != nil {
		b.OnClick = func() {
			vm.People.SetFilter(func(it interface{}) bool { return it.(*Trader).Age >= 30 })
			addLog("3.2.5: фильтр возраст ≥ 30 (видно %d)", vm.People.Count())
		}
	}
	if b := btn("vsort325"); b != nil {
		b.OnClick = func() {
			vm.People.SetSort(widget.SortDescription{Property: "Name"})
			addLog("3.2.5: сортировка по имени")
		}
	}
	if b := btn("vreset325"); b != nil {
		b.OnClick = func() {
			vm.People.SetFilter(nil)
			vm.People.ClearSort()
			addLog("3.2.5: фильтр/сортировка сброшены (%d)", vm.People.Count())
		}
	}
	if ti, ok := reg["maxlen325"].(*widget.TextInput); ok {
		ti.OnChange = func(s string) {
			addLog("3.2.5: TextBox = %q (%d симв.)", s, len([]rune(s)))
		}
	}

	// ─── Кнопка переключения ЯЗЫКА ИНТЕРФЕЙСА в шапке ────────────────────────
	// Важно: язык интерфейса (надписи) НЕ связан с раскладкой клавиатуры —
	// приложение может быть на русском, а ввод вестись на любом языке.
	updateLangBtn := func() {
		if b := btn("langToggle"); b != nil {
			b.Text = "Язык: " + widget.Language()
		}
	}
	if b := btn("langToggle"); b != nil {
		b.OnClick = func() {
			if widget.Language() == "RU" {
				widget.SetLanguage("EN")
			} else {
				widget.SetLanguage("RU")
			}
		}
	}
	widget.AddLanguageListener(func(code string) {
		updateLangBtn()
		addLog("Язык интерфейса → %s", code)
	})
	updateLangBtn()

	// Фокус на поле логина
	if ti, ok := reg["txtLogin"].(*widget.TextInput); ok {
		eng.SetFocus(ti)
	}

	// ─── Запуск ─────────────────────────────────────────────────────────────
	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	// ─── Живые данные (анимация) ────────────────────────────────────────────
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		var progress float64
		for range ticker.C {
			// Часы в хедере
			if l := lbl("headerClock"); l != nil {
				l.SetText(time.Now().Format("15:04:05"))
			}

			// Анимация прогресса
			progress += 0.003
			if progress > 1.0 {
				progress = 0
			}
			if pb, ok := reg["pbMain"].(*widget.ProgressBar); ok {
				pb.SetValue(progress)
			}
			if l := lbl("lblPbVal"); l != nil {
				l.SetText(fmt.Sprintf("%.0f%%", progress*100))
			}

			// Имитация CPU/RAM/Disk
			cpuVal := 0.20 + 0.15*sinWave(time.Now(), 3*time.Second)
			ramVal := 0.65 + 0.05*sinWave(time.Now(), 7*time.Second)
			diskVal := 0.44 + 0.02*sinWave(time.Now(), 11*time.Second)

			if pb, ok := reg["pbCPU"].(*widget.ProgressBar); ok {
				pb.SetValue(cpuVal)
			}
			if l := lbl("lblCPU"); l != nil {
				l.SetText(fmt.Sprintf("%.0f%%", cpuVal*100))
			}
			if pb, ok := reg["pbRAM"].(*widget.ProgressBar); ok {
				pb.SetValue(ramVal)
			}
			if l := lbl("lblRAM"); l != nil {
				l.SetText(fmt.Sprintf("%.0f%%", ramVal*100))
			}
			if pb, ok := reg["pbDisk"].(*widget.ProgressBar); ok {
				pb.SetValue(diskVal)
			}
			if l := lbl("lblDisk"); l != nil {
				l.SetText(fmt.Sprintf("%.0f%%", diskVal*100))
			}

			// Status bar FPS
			if l := lbl("lblFPS"); l != nil {
				l.SetText(fmt.Sprintf("%.0f FPS", 60.0))
			}
		}
	}()

	// ─── Нативное окно ──────────────────────────────────────────────────────
	win = window.New(eng, "GuiEngine — Widget Showcase")
	win.SetMaxFPS(60)

	if err := win.Run(); err != nil {
		log.Fatal(err)
	}
}

// sinWave возвращает sin-волну в диапазоне [-1, 1] с заданным периодом.
func sinWave(now time.Time, period time.Duration) float64 {
	phase := float64(now.UnixNano()%int64(period)) / float64(period)
	return sinApprox(phase * 2 * 3.14159265)
}

// sinApprox — приближение sin(x) без импорта math.
func sinApprox(x float64) float64 {
	// Нормализация в [-π, π]
	for x > 3.14159265 {
		x -= 2 * 3.14159265
	}
	for x < -3.14159265 {
		x += 2 * 3.14159265
	}
	// Taylor: sin(x) ≈ x - x³/6 + x⁵/120
	x3 := x * x * x
	x5 := x3 * x * x
	return x - x3/6.0 + x5/120.0
}
