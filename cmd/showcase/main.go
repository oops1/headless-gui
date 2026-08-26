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
	"image/color"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/cmd/internal/showcasestrings"
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
	username    string
	status      string
	Items       *dg.ObservableCollection
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
			return "E-mail cannot be empty"
		}
		if !strings.Contains(v.Email, "@") {
			return "E-mail must contain @"
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

	// ЛОКАЛИЗАЦИЯ ВСЕГО ПРИЛОЖЕНИЯ. Две таблицы: строки разметки
	// (strings_ru.go, собрана из showcase.xaml) и строки, которые showcase
	// складывает в коде (strings_code_ru.go). Ключ — английская строка,
	// поэтому английской таблицы не нужно: Tr не находит перевод и возвращает
	// сам ключ. Разметка ссылается на ключи через {Loc …} и обновляется
	// вживую, код зовёт Tr/Trf и перечитывает строки в relocalize (ниже).
	showcasestrings.Register()
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
		status:   widget.Tr("Ready — Ctrl+S to save"),
		Items:    items,
		Email:    "user@example.com",
		People:   people,
	}
	// Статус в строке состояния хранится ключом и аргументами — при смене
	// языка он пересобирается, а не остаётся на прежнем (см. relocalize).
	statusKey, statusArgs := "Ready — Ctrl+S to save", []any(nil)
	setVMStatus := func(key string, args ...any) {
		statusKey, statusArgs = key, args
		vm.SetStatus(widget.Trf(key, args...))
	}
	vm.SaveCommand = widget.NewRelayCommand(func() {
		setVMStatus("Saved at %s", time.Now().Format("15:04:05"))
	})

	// ─── Загрузка UI из XAML (с DataContext для живых привязок) ──────────────
	root, reg, _, err := widget.LoadUIFromXAMLFileBindings("./assets/ui/showcase.xaml", vm)
	if err != nil {
		log.Fatalf("cannot load showcase.xaml: %v", err)
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

	// relocalizers — тексты, которые showcase ставит ИЗ КОДА. Разметка при
	// смене языка обновляется сама (привязки {Loc}), а вот содержимое,
	// собранное в Go, нужно перечитать: каждый такой участок регистрирует
	// здесь замыкание, и relocalize (ниже) прогоняет их разом.
	// prevLanguage нужен тем из них, кто должен отличить «текст остался
	// нашим демонстрационным» от «пользователь его поправил».
	var relocalizers []func()
	prevLanguage := widget.Language()

	// addLog пишет строку в журнал событий. Формат — это КЛЮЧ перевода:
	// сообщение собирается на текущем языке интерфейса (Trf = Sprintf поверх
	// Tr), поэтому журнал говорит на том же языке, что и остальной UI.
	addLog := func(format string, args ...any) {
		msg := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), widget.Trf(format, args...))
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
			addLog("Menu: %s (section %d, item %d)", text, topIdx, subIdx)
			// Пункт меню приходит уже переведённым, поэтому сравниваем с
			// переводом ключа, а не с английской строкой.
			if text == widget.Tr("Exit") && win != nil {
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
			Title:  "System properties",
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

	// Демо «Вкладки в заголовке» (Windows 11 Terminal style): настоящий
	// widget.Window с EnableTitleTabs внутри showcase — вкладки, «+», «×»,
	// шеврон-меню; отрисовка следует активной теме (Win10/11, Win2000, Mac).
	if mount, ok := reg["tabWinMount"].(*widget.Panel); ok {
		tw := &widget.Window{
			Title:      "Terminal",
			Style:      widget.WindowStyleSingleBorder,
			Resize:     widget.ResizeModeCanResize,
			MainWindow: false,
			Background: color.RGBA{R: 12, G: 12, B: 12, A: 255},
		}
		mb := mount.Bounds()
		tw.SetBounds(image.Rect(mb.Min.X+50, mb.Min.Y+40, mb.Max.X-50, mb.Max.Y-40))

		tabSeq := 0
		newTabContent := func(name string) widget.Widget {
			p := widget.NewPanel(color.RGBA{R: 12, G: 12, B: 12, A: 255})
			p.ShowHeader = false
			// Координаты относительно (0,0): SetActiveTitleTab выставит
			// панели ContentBounds, а Panel.SetBounds сдвинет детей на дельту.
			lbl := widget.NewLabel(`PS C:\> `+name, color.RGBA{R: 204, G: 204, B: 204, A: 255})
			lbl.SetBounds(image.Rect(12, 10, 400, 30))
			p.AddChild(lbl)
			return p
		}
		// Иконка вкладки — квадратик-«терминал» (генерируется кодом, как
		// иконка трея): тёмный фон + зелёный ">" в углу.
		tabIcon := func() image.Image {
			img := image.NewRGBA(image.Rect(0, 0, 16, 16))
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					img.SetRGBA(x, y, color.RGBA{R: 30, G: 30, B: 34, A: 255})
				}
			}
			green := color.RGBA{R: 80, G: 220, B: 120, A: 255}
			for i := 0; i < 4; i++ { // глиф ">"
				img.SetRGBA(3+i, 5+i, green)
				img.SetRGBA(3+i, 11-i, green)
			}
			for x := 8; x < 13; x++ { // подчёркивание
				img.SetRGBA(x, 12, green)
			}
			return img
		}()
		addTab := func(name string) int {
			tabSeq++
			idx := tw.AddTitleTab(name, newTabContent(name))
			tw.SetTitleTabIcon(idx, tabIcon)
			return idx
		}
		addTab("PowerShell")
		addTab("cmd")
		addTab("bash")

		tw.OnTitleTabNew = func() {
			idx := addTab(fmt.Sprintf("Вкладка %d", tabSeq))
			tw.SetActiveTitleTab(idx)
			addLog("Title tab: + (new tab)")
		}
		tw.OnTitleTabChange = func(idx int, header string) {
			addLog("Title tab: %s", header)
		}
		tw.OnTitleTabClosed = func(idx int, header string) {
			addLog("Title tab closed: %s", header)
		}
		// Закрыты все вкладки (или «×» окна) — демо возрождается с одной.
		tw.OnClose = func() {
			addTab("PowerShell")
			addLog("Title tabs: demo window restored")
		}

		menu := widget.NewPopupMenu()
		for _, prof := range []string{"Windows PowerShell", "Командная строка", "Ubuntu"} {
			prof := prof
			menu.AddItem(prof, func() {
				idx := addTab(prof)
				tw.SetActiveTitleTab(idx)
			})
		}
		menu.AddSeparator()
		menu.AddItem("О программе", func() { addLog("Title tabs: about") })
		tw.SetTitleTabsMenu(menu)

		mount.AddChild(tw)
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
				addLog("Theme: %s", name)
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
			addLog("Sign in: user=%s", login)
		}
	}
	if b := btn("btnDefault"); b != nil {
		b.OnClick = func() {
			addLog("Cancel pressed")
		}
	}
	if b := btn("btnDanger"); b != nil {
		b.OnClick = func() {
			addLog("Delete pressed (dangerous action)")
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
			addLog("Form reset")
		}
	}
	if b := btn("btnExport"); b != nil {
		b.OnClick = func() {
			addLog("Exporting settings...")
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
				addLog("PopupMenu opened (ShowBelow)")
			}
		}
		if b := btn("btnShowPopup2"); b != nil {
			b.OnClick = func() {
				pm.ShowRight(b)
				addLog("PopupMenu opened (ShowRight)")
			}
		}
	}

	// ─── TAB 2: Элементы управления ──────────────────────────────────────────

	// CheckBox
	for _, pair := range [][2]string{
		{"cbRemember", "Remember"},
		{"cbAutoConnect", "Auto-connect"},
		{"cbVerbose", "Verbose"},
		{"cbCompress", "Compression"},
	} {
		id, name := pair[0], pair[1]
		if c := cb(id); c != nil {
			n := name // capture
			c.OnChange = func(checked bool) {
				// Имя виджета — тоже ключ перевода: в журнал оно должно
				// попасть на языке интерфейса, а не на языке исходника.
				addLog("CheckBox «%s»: %v", widget.Tr(n), checked)
			}
		}
	}

	// RadioButton
	for _, pair := range [][2]string{
		{"rbLDAP", "LDAP"},
		{"rbOTP", "OTP"},
		{"rbCert", "Certificate"},
		{"rbHigh", "High"},
		{"rbMedium", "Medium"},
		{"rbLow", "Low"},
	} {
		id, name := pair[0], pair[1]
		if rb, ok := reg[id].(*widget.RadioButton); ok {
			n := name
			rb.OnChange = func(selected bool) {
				if selected {
					addLog("RadioButton: %s", widget.Tr(n))
				}
			}
		}
	}

	// ToggleSwitch
	for _, pair := range [][2]string{
		{"tsAutoRefresh", "Auto-update"},
		{"tsNotify", "Notifications"},
		{"tsDarkMode", "Dark theme"},
		{"tsFullscreen", "Full screen"},
		{"tsSmooth", "Smoothing"},
		{"tsAudio", "Audio"},
	} {
		id, name := pair[0], pair[1]
		if ts := toggle(id); ts != nil {
			n := name
			ts.OnChange = func(on bool) {
				state := "OFF"
				if on {
					state = "ON"
				}
				addLog("Toggle «%s»: %s", widget.Tr(n), widget.Tr(state))
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
			addLog("Playback: %.0f%%", v)
		}
	}
	// Slider: запись
	if s := slider("sliderRecord"); s != nil {
		s.OnChange = func(v float64) {
			addLog("Recording: %.0f%%", v)
		}
	}

	// ─── TAB 3: Данные ───────────────────────────────────────────────────────

	// ListView кнопки
	if b := btn("btnAddEvent"); b != nil {
		b.OnClick = func() {
			addLog("Event added by the user")
		}
	}
	if b := btn("btnClearLog"); b != nil {
		b.OnClick = func() {
			if eventLog != nil {
				eventLog.Clear()
				addLog("Log cleared")
			}
		}
	}

	// TabControl обработчик
	if tc, ok := reg["mainTabs"].(*widget.TabControl); ok {
		tc.OnTabChange = func(idx int, header string) {
			addLog("Tab: %s (%d)", header, idx)
		}
	}
	if tc, ok := reg["innerTabs"].(*widget.TabControl); ok {
		tc.OnTabChange = func(idx int, header string) {
			addLog("Inner tab: %s", header)
		}
	}

	// Dropdown
	if dd, ok := reg["ddRole"].(*widget.Dropdown); ok {
		dd.OnChange = func(idx int, text string) {
			addLog("Role: %s", text)
		}
	}
	if dd, ok := reg["ddProtocol"].(*widget.Dropdown); ok {
		dd.OnChange = func(idx int, text string) {
			addLog("Protocol: %s", text)
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
			addLog("3.2.4: status → ERROR (DataTrigger)")
		}
	}
	if b := btn("addBtn324"); b != nil {
		b.OnClick = func() {
			vm.Items.Add(Order{"BUY", "NEWPAIR", 1.23}) // ItemsControl обновится вживую
			addLog("3.2.4: row added (live ItemsControl)")
		}
	}
	if b := btn("saveBtn324"); b != nil {
		b.AddClickHandler(func() { addLog("3.2.4: Save command executed") })
	}

	// ─── TAB 3.2.5: локализация / Tier B ─────────────────────────────────────
	if b := btn("langRu325"); b != nil {
		b.OnClick = func() {
			widget.SetLanguage("RU")
			addLog("3.2.5: UI language → RU ({Loc} updated live)")
		}
	}
	if b := btn("langEn325"); b != nil {
		b.OnClick = func() {
			widget.SetLanguage("EN")
			addLog("3.2.5: UI language → EN ({Loc} updated live)")
		}
	}
	if b := btn("save325"); b != nil {
		b.OnClick = func() { addLog("3.2.5: Save (localised button)") }
	}
	if nud, ok := reg["qty325"].(*widget.NumericUpDown); ok {
		nud.OnChange = func(v float64) { addLog("3.2.5: quantity = %.0f", v) }
	}
	if nud, ok := reg["price325"].(*widget.NumericUpDown); ok {
		nud.OnChange = func(v float64) { addLog("3.2.5: price = %.2f", v) }
	}
	if b := btn("vfilter325"); b != nil {
		b.OnClick = func() {
			vm.People.SetFilter(func(it interface{}) bool { return it.(*Trader).Age >= 30 })
			addLog("3.2.5: filter age ≥ 30 (%d visible)", vm.People.Count())
		}
	}
	if b := btn("vsort325"); b != nil {
		b.OnClick = func() {
			vm.People.SetSort(widget.SortDescription{Property: "Name"})
			addLog("3.2.5: sorted by name")
		}
	}
	if b := btn("vreset325"); b != nil {
		b.OnClick = func() {
			vm.People.SetFilter(nil)
			vm.People.ClearSort()
			addLog("3.2.5: filter and sorting reset (%d)", vm.People.Count())
		}
	}
	if ti, ok := reg["maxlen325"].(*widget.TextInput); ok {
		ti.OnChange = func(s string) {
			addLog("3.2.5: TextBox = %q (%d chars)", s, len([]rune(s)))
		}
	}

	// ─── Кнопка переключения ЯЗЫКА ИНТЕРФЕЙСА в шапке ────────────────────────
	// Важно: язык интерфейса (надписи) НЕ связан с раскладкой клавиатуры —
	// приложение может быть на русском, а ввод вестись на любом языке.
	updateLangBtn := func() {
		if b := btn("langToggle"); b != nil {
			b.Text = widget.Tr("Language:") + " " + widget.Language()
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
	// relocalize пересобирает всё, что задано из кода. Разметка обновляется
	// сама, поэтому здесь только «ручные» тексты: кнопка языка, строка
	// состояния, демонстрационное содержимое редакторов, трей-меню.
	relocalize := func() {
		updateLangBtn()
		setVMStatus(statusKey, statusArgs...)
		for _, fn := range relocalizers {
			fn()
		}
	}
	widget.AddLanguageListener(func(code string) {
		relocalize()
		prevLanguage = code
		addLog("UI language → %s", code)
	})
	updateLangBtn()

	// Светящаяся полоса: кнопка переключает определённый режим и «бегунок».
	if b := btn("pbGlowToggle"); b != nil {
		if pbg, ok := reg["pbGlow"].(*widget.ProgressBar); ok {
			b.OnClick = func() {
				on := !pbg.IsIndeterminate()
				pbg.SetIndeterminate(on)
				if on {
					b.SetText(widget.Tr("Determinate mode"))
				} else {
					b.SetText(widget.Tr("Indeterminate mode"))
					pbg.AnimateValue(0.62)
				}
				addLog("Glow bar: indeterminate=%v", on)
			}
		}
	}

	// ─── TAB: Диалоги ────────────────────────────────────────────────────────
	mbox := widget.NewMessageBox(eng)
	setResult := func(format string, args ...any) {
		if l := lbl("dlgResult"); l != nil {
			l.SetText(widget.Trf(format, args...))
		}
	}
	if b := btn("dlgInfo"); b != nil {
		b.OnClick = func() { mbox.ShowInfo("", widget.Tr("Document saved. A backup copy has been created.")) }
	}
	if b := btn("dlgQuestion"); b != nil {
		b.OnClick = func() {
			mbox.ShowQuestion("", widget.Tr("Save the changes before exiting?"), func(r widget.MessageBoxResult) {
				setResult("Question → %v", r)
			})
		}
	}
	if b := btn("dlgWarn"); b != nil {
		b.OnClick = func() { mbox.ShowWarning("", widget.Tr("Unsaved changes will be lost.")) }
	}
	if b := btn("dlgError"); b != nil {
		b.OnClick = func() { mbox.ShowError("", widget.Tr("Cannot open the file: access denied (EACCES).")) }
	}
	if b := btn("dlgInput"); b != nil {
		b.OnClick = func() {
			id := mbox.ShowInput("", widget.Tr("New item name:"), "report-2026.xlsx",
				func(s string) string {
					if strings.ContainsAny(s, `/\:*?`) {
						return "The name cannot contain / \\ : * ?"
					}
					return ""
				},
				func(text string, ok bool) {
					if ok {
						setResult("Input → %q", text)
					} else {
						setResult("Input cancelled")
					}
				})
			id.SetHint(widget.Tr("The name cannot contain / \\ : * ?"))
		}
	}
	if b := btn("dlgProgress"); b != nil {
		b.OnClick = func() {
			pd := mbox.ShowProgress(widget.Tr("Copying files"), "backup/photos/IMG_2647.jpg", func() { setResult("Progress cancelled") })
			go func() {
				for i := 0; i <= 100; i += 4 {
					time.Sleep(60 * time.Millisecond)
					pd.SetProgress(float64(i) / 100)
					pd.SetDetail(widget.Trf("%d of 120 files · 61.4 MB/s", i*120/100))
				}
				pd.SetIndeterminate(true)
				pd.SetStatus(widget.Tr("Verifying checksums…"))
				pd.SetDetail("")
				time.Sleep(1200 * time.Millisecond)
				pd.Close()
				setResult("Copying finished")
			}()
		}
	}
	// Диалог ожидания: заголовок по центру, светящаяся полоса, нижняя
	// строка-предупреждение. Сперва неопределённый режим («не знаем, сколько
	// осталось»), затем переход на реальный прогресс — так это и выглядит в
	// жизни, когда работа сначала непредсказуема, а потом считается.
	if b := btn("dlgBusy"); b != nil {
		b.OnClick = func() {
			bd := mbox.ShowBusy(
				widget.Tr("Processing data"),
				widget.Tr("Please wait…"),
				widget.Tr("Do not close this window"),
				func() { setResult("Processing cancelled") },
			)
			go func() {
				time.Sleep(1600 * time.Millisecond)
				bd.SetSubtitle(widget.Tr("Building the index…"))
				for i := 0; i <= 100; i += 3 {
					time.Sleep(45 * time.Millisecond)
					bd.SetProgress(float64(i) / 100)
				}
				bd.Close()
				setResult("Processing finished")
			}()
		}
	}

	fileOpts := func() widget.FileDialogOptions {
		return widget.FileDialogOptions{
			Filters: []widget.FileFilter{
				{Label: widget.Tr("All files"), Exts: nil},
				{Label: widget.Tr("Spreadsheets"), Exts: []string{".xlsx", ".csv"}},
				{Label: widget.Tr("Images"), Exts: []string{".png", ".jpg", ".jpeg"}},
			},
		}
	}
	if b := btn("dlgOpen"); b != nil {
		b.OnClick = func() {
			mbox.ShowOpenFile(fileOpts(), func(path string, ok bool) {
				if ok {
					setResult("Open → %s", path)
				}
			})
		}
	}
	if b := btn("dlgSave"); b != nil {
		b.OnClick = func() {
			o := fileOpts()
			o.InitialName = "report-final.xlsx"
			mbox.ShowSaveFile(o, func(path string, ok bool) {
				if ok {
					setResult("Save → %s", path)
				}
			})
		}
	}
	if b := btn("dlgFolder"); b != nil {
		b.OnClick = func() {
			mbox.ShowPickFolder(widget.FileDialogOptions{}, func(path string, ok bool) {
				if ok {
					setResult("Folder → %s", path)
				}
			})
		}
	}

	// ─── TAB: Анимации ────────────────────────────────────────────────────────
	// «Диалог больше окна» — крупный диалог (1000×700). В нативном режиме
	// (Windows/X11) он открывается в СОБСТВЕННОМ окне ОС и может выходить за
	// пределы главного окна; в headless — рисуется поверх затемнения.
	if b := btn("dlgBig"); b != nil {
		b.OnClick = func() {
			const dw, dh = 1000, 700
			dlg := widget.NewDialog(widget.Tr("Large dialog in its own window"), dw, dh)
			dlg.SetBounds(image.Rect(0, 0, dw, dh))
			info := widget.NewWin10Label(widget.Tr("This dialog is 1000×700. Since v3.10 modal dialogs open in a separate native OS window, so they can be larger than the main window and can be dragged outside it."))
			info.SetBounds(image.Rect(40, 60, dw-40, 120))
			dlg.AddChild(info)
			okBtn := widget.NewWin10AccentButton(widget.Tr("Close"))
			okBtn.SetBounds(image.Rect(dw-160, dh-60, dw-40, dh-24))
			okBtn.OnClick = func() { eng.CloseModal(dlg) }
			dlg.AddChild(okBtn)
			eng.ShowModal(dlg)
			addLog("Dialogs: opened the large 1000x700 dialog")
		}
	}

	// ─── TAB: Система (трей / уведомления / превью) ──────────────────────────
	// На платформах кроме Windows функции трея — no-op; кнопки показывают
	// сообщение «только Windows».
	sysOnly := func() bool {
		if runtime.GOOS != "windows" {
			mbox.ShowInfo("", widget.Tr("This feature is available on Windows only."))
			return false
		}
		return true
	}
	if b := btn("sysBalloonInfo"); b != nil {
		b.OnClick = func() {
			if !sysOnly() {
				return
			}
			if err := win.ShowBalloon(widget.Tr("Information"), widget.Tr("The operation completed successfully."), widget.SeverityInfo); err != nil {
				addLog("Balloon: error — %v", err)
				return
			}
			addLog("Balloon: info shown")
		}
	}
	if b := btn("sysBalloonWarn"); b != nil {
		b.OnClick = func() {
			if !sysOnly() {
				return
			}
			if err := win.ShowBalloon(widget.Tr("Warning"), widget.Tr("There are unsaved changes."), widget.SeverityWarning); err != nil {
				addLog("Balloon: error — %v", err)
				return
			}
			addLog("Balloon: warning shown")
		}
	}
	if b := btn("sysBalloonErr"); b != nil {
		b.OnClick = func() {
			if !sysOnly() {
				return
			}
			if err := win.ShowBalloon(widget.Tr("Error"), widget.Tr("Cannot open the file (EACCES)."), widget.SeverityError); err != nil {
				addLog("Balloon: error — %v", err)
				return
			}
			addLog("Balloon: error shown")
		}
	}
	if b := btn("sysHide"); b != nil {
		b.OnClick = func() {
			if !sysOnly() {
				return
			}
			win.HideToTray()
			addLog("Window minimised to the tray (double left click on the icon restores it)")
		}
	}

	// ─── TAB: Анимации ────────────────────────────────────────────────────────
	animBars := []struct {
		name  string
		curve widget.Easing
	}{
		{"animBar0", widget.EaseLinear},
		{"animBar1", widget.EaseOutQuad},
		{"animBar2", widget.EaseOutCubic},
		{"animBar3", widget.EaseInOutSine},
		{"animBar4", widget.EaseOutBounce},
		{"animBar5", widget.EaseOutElastic},
	}
	if b := btn("animRun"); b != nil {
		b.OnClick = func() {
			for _, def := range animBars {
				pb, ok := reg[def.name].(*widget.ProgressBar)
				if !ok {
					continue
				}
				pb.SetValue(0)
				curve := def.curve // capture
				widget.AnimateOwned(pb, "race", 900*time.Millisecond, curve,
					func(t float64) { pb.SetValue(t) })
			}
		}
	}
	if b := btn("animMove"); b != nil {
		boxRight := false
		b.OnClick = func() {
			box, ok := reg["animBox"]
			if !ok {
				return
			}
			cur := box.Bounds()
			dx := 500
			if boxRight {
				dx = -500
			}
			boxRight = !boxRight
			widget.AnimateRect(box, cur.Add(image.Pt(dx, 0)),
				450*time.Millisecond, widget.EaseOutBack)
		}
	}
	if b := btn("animValue"); b != nil {
		next := 0.9
		b.OnClick = func() {
			pb, ok := reg["animValueBar"].(*widget.ProgressBar)
			if !ok {
				return
			}
			target := next
			next = 1.3 - next // чередуем 0.9 ↔ 0.4
			pb.AnimateValue(target)
			if l := lbl("animValueLbl"); l != nil {
				l.SetText(fmt.Sprintf("→ %d%%", int(target*100)))
			}
		}
	}
	if pb, ok := reg["animValueBar"].(*widget.ProgressBar); ok {
		pb.SetValue(0.4)
	}

	// ─── Многострочный TextBox (вкладка «Диалоги») ────────────────────────────
	if tb, ok := reg["editBox"].(*widget.TextBox); ok {
		const editDemo = "The multiline editor of the engine.\n\nWord wrapping, vertical scrolling with the wheel and PgUp/PgDn, selection with the mouse and Shift+arrows, Ctrl+arrows — by words, Ctrl+Home/End — document bounds, Ctrl+C/X/V and Ctrl+Z/Y.\n\nMixed content: English, Russian and digits 1234567890."
		tb.SetText(widget.Tr(editDemo))
		// Демонстрационный текст перечитывается при смене языка — но только
		// если пользователь его не правил (иначе затёрли бы его ввод).
		relocalizers = append(relocalizers, func() {
			if tb.GetText() == widget.TrIn(prevLanguage, editDemo) {
				tb.SetText(widget.Tr(editDemo))
			}
		})
		tb.OnChange = func(text string) {
			if l := lbl("editStats"); l != nil {
				l.SetText(widget.Trf("Characters: %d · lines (visual): %d",
					len([]rune(text)), tb.LineCount()))
			}
		}
	}
	if ro, ok := reg["editBoxRO"].(*widget.TextBox); ok {
		const roDemo = "This field is ReadOnly: the text can be selected and copied (Ctrl+C, context menu) but not edited.\n\nThe editor works headless too: input arrives through SendKeyEvent and layout is computed without a window."
		ro.SetText(widget.Tr(roDemo))
		relocalizers = append(relocalizers, func() { ro.SetText(widget.Tr(roDemo)) })
	}

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

			// Вкладка «Текст и графика»: текущий HiDPI-масштаб.
			if l := lbl("lblScale"); l != nil {
				pw, ph := eng.PhysicalSize()
				lw, lh := eng.CanvasSize()
				l.SetText(fmt.Sprintf("Scale: %.0f%%  ·  logical %d×%d → physical %d×%d",
					eng.Scale()*100, lw, lh, pw, ph))
			}
		}
	}()

	// ─── Вкладка «Компоновка»: SplitPanel + SVGIcon ──────────────────────────
	if sp, ok := reg["splitOuter"].(*widget.SplitPanel); ok {
		posLbl, _ := reg["splitPosLbl"].(*widget.Label)
		sp.OnPositionChanged = func(pos float64) {
			if posLbl != nil {
				posLbl.SetText(fmt.Sprintf("Position: %.2f", pos))
			}
		}
		if btn, ok := reg["splitCollapse"].(*widget.Button); ok {
			btn.OnClick = func() {
				sp.ToggleCollapse()
				addLog("SplitPanel: collapsed=%v", sp.IsCollapsed())
			}
		}
	}

	// ─── Вкладка «Докинг»: DockManager — сохранение/восстановление раскладки ─
	if dm, ok := reg["dockDemo"].(*widget.DockManager); ok {
		var savedLayout []byte
		statusLbl, _ := reg["dockLayoutStatus"].(*widget.Label)
		// Подпись хранится КЛЮЧОМ и аргументами, а не готовой строкой: при
		// смене языка relocalize пересобирает её заново (см. ниже).
		var statusKey string
		var statusArgs []any
		setStatus := func(key string, args ...any) {
			statusKey, statusArgs = key, args
			if statusLbl != nil {
				statusLbl.SetText(widget.Trf(key, args...))
			}
		}
		relocalizers = append(relocalizers, func() {
			if statusKey != "" {
				setStatus(statusKey, statusArgs...)
			}
		})
		if btn, ok := reg["dockSaveLayout"].(*widget.Button); ok {
			btn.OnClick = func() {
				savedLayout = dm.SaveLayout()
				setStatus("Layout saved (%d bytes of JSON)", len(savedLayout))
				addLog("DockManager: layout saved (%d bytes)", len(savedLayout))
			}
		}
		if btn, ok := reg["dockRestoreLayout"].(*widget.Button); ok {
			btn.OnClick = func() {
				if savedLayout == nil {
					setStatus("Save the layout first")
					addLog("DockManager: restore cancelled — no saved layout")
					return
				}
				if err := dm.RestoreLayout(savedLayout); err != nil {
					setStatus("Restore failed: %s", err.Error())
					addLog("DockManager: cannot restore the layout: %v", err)
					return
				}
				setStatus("Layout restored")
				addLog("DockManager: layout restored")
			}
		}
	}

	// ─── Нативное окно ──────────────────────────────────────────────────────
	win = window.New(eng, "GuiEngine — Widget Showcase")
	win.SetMaxFPS(60)

	// ─── Трей: иконка + контекстное меню (Windows; на прочих ОС — no-op) ─────
	// Иконку и меню задаём ДО Run(): состояние буферизуется и применяется при
	// создании окна. Двойной левый клик по иконке восстанавливает окно (дефолт).
	if err := win.SetTrayIcon(makeTrayIcon(), "GuiEngine — Widget Showcase"); err != nil {
		log.Printf("tray icon unavailable: %v", err)
	}
	// Трей-меню собирается заново при смене языка: его пункты — обычные
	// строки, а не привязки, поэтому сами по себе они не переведутся.
	trayMenu := widget.NewPopupMenu()
	buildTrayMenu := func() {
		trayMenu.SetItems(nil) // собираем список заново на текущем языке
		trayMenu.AddItem(widget.Tr("Show"), func() { win.RestoreFromTray() })
		trayMenu.AddItem(widget.Tr("Minimise"), func() { win.HideToTray() })
		trayMenu.AddSeparator()
		trayMenu.AddItem("Balloon", func() {
			win.ShowBalloon("GuiEngine", widget.Tr("A notification from the tray menu."), widget.SeverityInfo)
		})
		trayMenu.AddSeparator()
		trayMenu.AddItem(widget.Tr("Exit"), func() { win.Close() })
	}
	buildTrayMenu()
	relocalizers = append(relocalizers, buildTrayMenu)
	trayMenu.OnSelect = func(idx int, text string) { addLog("Tray menu: %s", text) }
	win.SetTrayMenu(trayMenu)
	win.SetOnBalloonClick(func() { addLog("Click on the balloon notification") })

	// ─── Drag & Drop файлов из ОС ────────────────────────────────────────────
	// Перетаскивание файлов из проводника в окно: выводим их пути в drop-зону
	// на вкладке «Система». Координаты (x,y) — логические, приходят из бэкенда
	// (Win32 WM_DROPFILES / X11 XDND / Wayland). Green — цвет успешного дропа.
	if dropLbl, ok := reg["dropLabel"].(*widget.Label); ok {
		win.SetOnFilesDropped(func(paths []string, x, y int) {
			addLog("Drop: %d file(s) at (%d,%d)", len(paths), x, y)
			text := widget.Trf("Dropped %d at (%d,%d):", len(paths), x, y)
			for i, p := range paths {
				if i >= 4 {
					text += widget.Trf("  …and %d more", len(paths)-4)
					break
				}
				text += "  " + p
			}
			dropLbl.SetText(text)
			dropLbl.TextColor = color.RGBA{R: 166, G: 227, B: 161, A: 255}
			dropLbl.Invalidate()
		})
	}

	if err := win.Run(); err != nil {
		log.Fatal(err)
	}
}

// makeTrayIcon рисует 32×32 иконку приложения: синий квадрат со скруглёнными
// углами и белой буквой «G» (кольцо с разрывом справа + горизонтальная
// перекладина). Чистый image/draw, без внешних ассетов.
func makeTrayIcon() image.Image {
	const n = 32
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	blue := color.RGBA{R: 0, G: 120, B: 215, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	// Фон: синий квадрат со скруглёнными углами (радиус 6).
	const r = 6
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if roundedInside(x, y, n, n, r) {
				img.SetRGBA(x, y, blue)
			}
		}
	}

	// Буква «G»: кольцо (6 ≤ dist ≤ 10.5) вокруг центра, с разрывом справа
	// (устье буквы) и перекладиной от центра вправо.
	cx, cy := 15.5, 16.0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := dx*dx + dy*dy
			ring := dist >= 6.0*6.0 && dist <= 10.5*10.5
			mouth := dx > 3.0 && dy > -3.0 && dy < 3.0 // разрыв справа
			bar := dy > 1.0 && dy < 4.0 && dx > 0.0 && dx < 8.0
			if (ring && !mouth) || bar {
				img.SetRGBA(x, y, white)
			}
		}
	}
	return img
}

// roundedInside сообщает, попадает ли пиксель (x,y) внутрь прямоугольника
// w×h со скруглёнными углами радиуса r.
func roundedInside(x, y, w, h, r int) bool {
	// Внутри вертикальной/горизонтальной «крестовины» — всегда да.
	if x >= r && x < w-r {
		return true
	}
	if y >= r && y < h-r {
		return true
	}
	// Углы: расстояние до центра ближайшего скругления.
	cx, cy := r, r
	if x >= w-r {
		cx = w - r - 1
	}
	if y >= h-r {
		cy = h - r - 1
	}
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= r*r
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
