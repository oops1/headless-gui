// webshowcase — ПОЛНАЯ витрина виджетов в браузере, без единого окна ОС.
//
//	go run ./cmd/webshowcase          # из корня репозитория
//	→ открыть http://localhost:8091
//
// Это тот же showcase, что и в нативном окне: грузится ТА ЖЕ разметка
// assets/ui/showcase.xaml с теми же вкладками, виджетами, темами и
// локализацией. Разница только в выводе — вместо окна ОС кадры уходят
// дельта-тайлами по WebSocket, а мышь и клавиатура возвращаются из браузера.
//
// Что тут интересно посмотреть:
//
//   - Сервер не открывает окон вообще: HEADLESS_GUI_*-бэкенды не участвуют,
//     работает только движок. Ту же программу можно крутить в контейнере.
//   - Диалоги (MessageBox, ввод, прогресс) и ФАЙЛОВЫЕ диалоги рисует движок,
//     поэтому они прекрасно живут в стриме — и показывают файловую систему
//     СЕРВЕРА, а не браузера.
//   - Смена темы и языка меняет картинку у всех подключённых вьюверов сразу:
//     состояние живёт на сервере, браузер лишь рисует тайлы.
//   - Несколько вкладок браузера = несколько зрителей одного UI.
//
// Вкладки «Система» (трей, balloon, превью в панели задач) в браузере
// неактивны: это возможности окна ОС, и сервер о них честно сообщает.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/cmd/internal/showcasestrings"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output/webstream"
	"github.com/oops1/headless-gui/v3/widget"
	dg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// Order — строка списка ордеров (DataTemplate на вкладке 3.2.4).
type Order struct {
	Side   string
	Symbol string
	Price  float64
}

// Trader — элемент для CollectionView + виртуализации (вкладка 3.2.5).
type Trader struct {
	Name string
	Age  int
}

// showcaseVM — модель данных разметки: живые привязки, триггеры, команды.
type showcaseVM struct {
	dg.PropertyNotifier
	username    string
	status      string
	Items       *dg.ObservableCollection
	SaveCommand *widget.RelayCommand

	Email  string
	People *widget.CollectionView
}

func (v *showcaseVM) GetUsername() string  { return v.username }
func (v *showcaseVM) SetUsername(s string) { v.username = s; v.NotifyPropertyChanged(v, "Username") }
func (v *showcaseVM) GetStatus() string    { return v.status }
func (v *showcaseVM) SetStatus(s string)   { v.status = s; v.NotifyPropertyChanged(v, "Status") }

// DataError — валидация поля E-mail (widget.DataErrorInfo, аналог IDataErrorInfo).
func (v *showcaseVM) DataError(prop string) string {
	if prop == "Email" {
		if v.Email == "" {
			return widget.Tr("E-mail cannot be empty")
		}
		if !strings.Contains(v.Email, "@") {
			return widget.Tr("E-mail must contain @")
		}
	}
	return ""
}

// pctConv — конвертер доли 0..1 → проценты (для {Binding …, Converter=Pct}).
type pctConv struct{}

func (pctConv) Convert(v interface{}) interface{} {
	f, _ := v.(float64)
	return fmt.Sprintf("%.0f%%", f*100)
}
func (pctConv) ConvertBack(v interface{}) interface{} { return v }

func main() {
	addr := flag.String("addr", ":8091", "адрес HTTP-сервера")
	xamlPath := flag.String("xaml", "./assets/ui/showcase.xaml", "путь к разметке витрины")
	lang := flag.String("lang", "RU", "язык интерфейса: RU или EN")
	theme := flag.String("theme", "", "тема оформления (Win10 Dark, Win11 Light, Win2000, macOS…)")
	flag.Parse()

	app, err := buildShowcase(*xamlPath, *lang)
	if err != nil {
		log.Fatalf("%v (запускайте из корня репозитория)", err)
	}
	if *theme != "" {
		app.applyTheme(*theme)
	}

	app.eng.Start()
	srv := webstream.New(app.eng)
	go srv.Run()
	go app.tick()

	log.Printf("webshowcase: http://localhost%s — витрина целиком, окон ОС не открыто", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv))
}

// buildShowcase собирает витрину: движок, разметку, модель и обработчики.
// Вынесено из main, чтобы тест мог поднять то же самое и постучаться в UI
// настоящим вводом, без сети и браузера.
func buildShowcase(xamlPath, lang string) (*webShowcase, error) {
	const screenW, screenH = 1280, 900
	eng := engine.New(screenW, screenH, 30)

	// Локализация — общая с оконной витриной (cmd/internal/showcasestrings).
	widget.RegisterValueConverter("Pct", pctConv{})
	showcasestrings.Register()
	registerWebStrings()
	widget.SetLanguage(lang)
	widget.SetLocale(lang)

	vm := newVM()
	root, reg, _, err := widget.LoadUIFromXAMLFileBindings(xamlPath, vm)
	if err != nil {
		return nil, fmt.Errorf("не удалось загрузить %s: %w", xamlPath, err)
	}
	eng.SetRoot(root)

	app := &webShowcase{eng: eng, reg: reg, vm: vm}
	app.wire()
	return app, nil
}

// newVM собирает модель данных с тем же наполнением, что и оконная витрина.
func newVM() *showcaseVM {
	items := dg.NewObservableCollection()
	items.Add(Order{"BUY", "BTCUSDT", 64231.5})
	items.Add(Order{"SELL", "ETHUSDT", 3120.0})
	items.Add(Order{"BUY", "SOLUSDT", 148.25})

	traders := make([]interface{}, 1000)
	names := []string{"Alice", "Bob", "Carol", "Dmitry", "Elena", "Igor", "Olga", "Pavel"}
	for i := range traders {
		traders[i] = &Trader{Name: fmt.Sprintf("%s #%d", names[i%len(names)], i), Age: 18 + (i*7)%50}
	}

	vm := &showcaseVM{
		username: "trader",
		status:   widget.Tr("Ready — Ctrl+S to save"),
		Items:    items,
		Email:    "user@example.com",
		People:   widget.NewCollectionView(dg.NewObservableCollectionFrom(traders)),
	}
	vm.SaveCommand = widget.NewRelayCommand(func() {
		vm.SetStatus(widget.Trf("Saved at %s", time.Now().Format("15:04:05")))
	})
	return vm
}

// webShowcase — обработчики витрины для браузерного режима.
type webShowcase struct {
	eng *engine.Engine
	reg map[string]widget.Widget
	vm  *showcaseVM

	mbox  *widget.MessageBox
	log   *widget.ListView
	langs []func() // тексты, заданные из кода, — перечитать при смене языка
}

// ─── Мелкие помощники доступа к разметке ─────────────────────────────────────

func (a *webShowcase) btn(id string) *widget.Button {
	b, _ := a.reg[id].(*widget.Button)
	return b
}
func (a *webShowcase) lbl(id string) *widget.Label {
	l, _ := a.reg[id].(*widget.Label)
	return l
}
func (a *webShowcase) slider(id string) *widget.Slider {
	s, _ := a.reg[id].(*widget.Slider)
	return s
}

// addLog пишет строку в журнал событий витрины. Формат — ключ перевода.
func (a *webShowcase) addLog(format string, args ...any) {
	msg := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), widget.Trf(format, args...))
	if a.log != nil {
		a.log.AddItem(msg)
	}
}

// onClick вешает обработчик на кнопку разметки (если она есть).
func (a *webShowcase) onClick(id string, fn func()) {
	if b := a.btn(id); b != nil {
		b.OnClick = fn
	}
}

// ─── Подключение обработчиков ────────────────────────────────────────────────

func (a *webShowcase) wire() {
	a.mbox = widget.NewMessageBox(a.eng)
	a.log, _ = a.reg["eventLog"].(*widget.ListView)

	a.wireHeader()
	a.wireButtons()
	a.wireControls()
	a.wireDialogs()
	a.wireWindowOnly()
	a.wireWebNote()

	a.addLog("Web showcase started: the server has no OS window")
}

// wireHeader — смена темы и языка. И то и другое живёт на сервере, поэтому
// применяется сразу ко всем подключённым вьюверам.
func (a *webShowcase) wireHeader() {
	if dd, ok := a.reg["themeSelect"].(*widget.Dropdown); ok {
		dd.OnChange = func(_ int, name string) { a.applyTheme(name) }
	}

	updateLangBtn := func() {
		if b := a.btn("langToggle"); b != nil {
			b.Text = widget.Tr("Language:") + " " + widget.Language()
		}
	}
	a.langs = append(a.langs, updateLangBtn)
	a.onClick("langToggle", func() {
		if widget.Language() == "RU" {
			widget.SetLanguage("EN")
		} else {
			widget.SetLanguage("RU")
		}
	})
	widget.AddLanguageListener(func(code string) {
		for _, fn := range a.langs {
			fn()
		}
		a.addLog("UI language → %s", code)
	})
	updateLangBtn()

	// Вкладка 3.2.5: кнопки прямого выбора языка.
	a.onClick("langRu325", func() { widget.SetLanguage("RU") })
	a.onClick("langEn325", func() { widget.SetLanguage("EN") })
}

func (a *webShowcase) applyTheme(name string) {
	t := widget.ThemeByName(name)
	if t == nil {
		return
	}
	a.eng.SetTheme(t)
	if hdr, ok := a.reg["header"].(*widget.Panel); ok {
		if title, ok := a.reg["headerTitle"].(*widget.Label); ok {
			if t.Style.Classic3D {
				hdr.Gradient = &widget.LinearGradient{Horizontal: true, Stops: []widget.GradientStop{
					{Offset: 0, Color: t.TitleBG}, {Offset: 1, Color: t.TitleBG2},
				}}
				title.TextColor, title.Bold = t.TitleText, true
			} else {
				hdr.Gradient = nil
				title.TextColor, title.Bold = t.TitleText, false
			}
		}
	}
	a.addLog("Theme: %s", name)
}

// wireButtons — кнопки формы на первой вкладке.
func (a *webShowcase) wireButtons() {
	a.onClick("btnAccent", func() {
		login := ""
		if ti, ok := a.reg["txtLogin"].(*widget.TextInput); ok {
			login = ti.GetText()
		}
		a.addLog("Sign in: user=%s", login)
	})
	a.onClick("btnDefault", func() { a.addLog("Cancel pressed") })
	a.onClick("btnDanger", func() { a.addLog("Delete pressed (dangerous action)") })
	a.onClick("btnAddEvent", func() { a.addLog("Event added by the user") })
	// В разметке кнопка сброса формы называется btnDisabled (историческое имя).
	a.onClick("btnDisabled", func() {
		for _, id := range []string{"txtLogin", "txtPassword", "txtComment"} {
			if ti, ok := a.reg[id].(*widget.TextInput); ok {
				ti.SetText("")
			}
		}
		a.addLog("Form reset")
	})
	a.onClick("btnExport", func() { a.addLog("Exporting settings...") })
	a.onClick("btnClearLog", func() {
		if a.log != nil {
			a.log.SetItems(nil)
		}
		a.addLog("Log cleared")
	})
}

// wireControls — переключатели, ползунки и вкладки: журналируем изменения,
// чтобы в браузере было видно, что ввод доезжает до сервера.
func (a *webShowcase) wireControls() {
	for id, name := range map[string]string{
		"cbRemember": "Remember", "cbAutoConnect": "Auto-connect",
		"cbVerbose": "Verbose", "cbCompress": "Compression",
	} {
		if c, ok := a.reg[id].(*widget.CheckBox); ok {
			n := name
			c.OnChange = func(checked bool) { a.addLog("CheckBox «%s»: %v", widget.Tr(n), checked) }
		}
	}
	for id, name := range map[string]string{
		"tsAutoRefresh": "Auto-update", "tsNotify": "Notifications",
		"tsDarkMode": "Dark theme", "tsFullscreen": "Full screen",
	} {
		if ts, ok := a.reg[id].(*widget.ToggleSwitch); ok {
			n := name
			ts.OnChange = func(on bool) {
				state := "OFF"
				if on {
					state = "ON"
				}
				a.addLog("Toggle «%s»: %s", widget.Tr(n), widget.Tr(state))
			}
		}
	}
	if s := a.slider("sliderSpeed"); s != nil {
		s.OnChange = func(v float64) {
			if l := a.lbl("lblSpeedVal"); l != nil {
				l.SetText(fmt.Sprintf("%.0f FPS", v))
			}
		}
	}
	if s := a.slider("sliderQuality"); s != nil {
		s.OnChange = func(v float64) {
			if l := a.lbl("lblQualityVal"); l != nil {
				l.SetText(fmt.Sprintf("%.0f%%", v))
			}
		}
	}
	if s := a.slider("sliderVolume"); s != nil {
		s.OnChange = func(v float64) {
			if l := a.lbl("lblVolumeVal"); l != nil {
				l.SetText(fmt.Sprintf("%.0f", v))
			}
		}
	}
	if tc, ok := a.reg["mainTabs"].(*widget.TabControl); ok {
		tc.OnTabChange = func(idx int, header string) { a.addLog("Tab: %s (%d)", header, idx) }
	}
}

// wireDialogs — диалоги движка. В браузере это самое показательное: их рисует
// сервер поверх затемнения, а файловые диалоги ходят по ЕГО файловой системе.
func (a *webShowcase) wireDialogs() {
	setResult := func(format string, args ...any) {
		if l := a.lbl("dlgResult"); l != nil {
			l.SetText(widget.Trf(format, args...))
		}
	}

	a.onClick("dlgInfo", func() {
		a.mbox.ShowInfo("", widget.Tr("Document saved. A backup copy has been created."))
	})
	a.onClick("dlgQuestion", func() {
		a.mbox.ShowQuestion("", widget.Tr("Save the changes before exiting?"),
			func(r widget.MessageBoxResult) { setResult("Question → %v", r) })
	})
	a.onClick("dlgWarn", func() {
		a.mbox.ShowWarning("", widget.Tr("Unsaved changes will be lost."))
	})
	a.onClick("dlgError", func() {
		a.mbox.ShowError("", widget.Tr("Cannot open the file: access denied (EACCES)."))
	})
	a.onClick("dlgInput", func() {
		a.mbox.ShowInput("", widget.Tr("New item name:"), "report-2026.xlsx", nil,
			func(text string, ok bool) {
				if ok {
					setResult("Input → %q", text)
					return
				}
				setResult("Input cancelled")
			})
	})
	a.onClick("dlgProgress", func() {
		pd := a.mbox.ShowProgress(widget.Tr("Copying files"), "backup/photos/IMG_2647.jpg",
			func() { setResult("Progress cancelled") })
		go func() {
			for i := 0; i <= 100; i += 4 {
				time.Sleep(60 * time.Millisecond)
				pd.SetProgress(float64(i) / 100)
				pd.SetDetail(widget.Trf("%d of 120 files · 61.4 MB/s", i*120/100))
			}
			pd.Close()
			setResult("Copying finished")
		}()
	})

	// Файловые диалоги — файловая система СЕРВЕРА (в браузере это особенно
	// наглядно: видно не диск пользователя, а диск процесса).
	opts := func() widget.FileDialogOptions {
		return widget.FileDialogOptions{Filters: []widget.FileFilter{
			{Label: widget.Tr("All files")},
			{Label: widget.Tr("Spreadsheets"), Exts: []string{".xlsx", ".csv"}},
			{Label: widget.Tr("Images"), Exts: []string{".png", ".jpg", ".jpeg"}},
		}}
	}
	a.onClick("dlgOpen", func() {
		a.mbox.ShowOpenFile(opts(), func(path string, ok bool) {
			if ok {
				setResult("Open → %s", path)
			}
		})
	})
	a.onClick("dlgSave", func() {
		o := opts()
		o.InitialName = "report-2026.xlsx"
		a.mbox.ShowSaveFile(o, func(path string, ok bool) {
			if ok {
				setResult("Save → %s", path)
			}
		})
	})
	a.onClick("dlgFolder", func() {
		a.mbox.ShowPickFolder(widget.FileDialogOptions{}, func(path string, ok bool) {
			if ok {
				setResult("Folder → %s", path)
			}
		})
	})
	a.onClick("dlgBig", func() {
		const dw, dh = 1000, 700
		dlg := widget.NewDialog(widget.Tr("Large dialog in its own window"), dw, dh)
		dlg.SetBounds(image.Rect(0, 0, dw, dh))
		info := widget.NewWin10Label(widget.Tr("In the browser a large dialog is drawn over the dimmed canvas: there is no OS window to be larger than."))
		info.SetBounds(image.Rect(40, 60, dw-40, 120))
		dlg.AddChild(info)
		ok := widget.NewWin10AccentButton(widget.Tr("Close"))
		ok.SetBounds(image.Rect(dw-160, dh-60, dw-40, dh-24))
		ok.OnClick = func() { a.eng.CloseModal(dlg) }
		dlg.AddChild(ok)
		a.eng.ShowModal(dlg)
		a.addLog("Dialogs: opened the large dialog over the stream")
	})
}

// wireWindowOnly — кнопки, которым нужно настоящее окно ОС (трей, balloon,
// сворачивание). В браузере честно объясняем, почему они не работают, вместо
// того чтобы молча ничего не делать.
func (a *webShowcase) wireWindowOnly() {
	const note = "This needs a real OS window: the server runs headless and has no tray."
	for _, id := range []string{
		"sysBalloonInfo", "sysBalloonWarn", "sysBalloonErr", "sysHide",
	} {
		a.onClick(id, func() {
			a.mbox.ShowInfo("", widget.Tr(note))
			a.addLog("Window-only feature requested in the browser")
		})
	}
}

// wireWebNote — заменяет подпись drop-зоны: перетаскивание файлов из ОС в
// браузерный холст движку недоступно.
func (a *webShowcase) wireWebNote() {
	if l := a.lbl("dropLabel"); l != nil {
		const note = "Drag and drop from the OS works in the native window; in the browser the canvas receives mouse and keyboard only."
		l.SetText(widget.Tr(note))
		a.langs = append(a.langs, func() { l.SetText(widget.Tr(note)) })
	}
}

// tick обновляет часы, прогресс и индикаторы — картинка в браузере должна
// жить, а не быть статичной.
func (a *webShowcase) tick() {
	start := time.Now()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for now := range t.C {
		if l := a.lbl("headerClock"); l != nil {
			l.SetText(now.Format("15:04:05"))
		}
		if pb, ok := a.reg["pbStatus"].(*widget.ProgressBar); ok {
			pb.SetValue(0.5 + 0.5*sinApprox(float64(now.Unix()%60)/4))
		}
		// Индикаторы «загрузки» — плавные пилы разной длины.
		secs := time.Since(start).Seconds()
		set := func(barID, lblID string, period float64) {
			v := 0.5 + 0.5*sinApprox(secs/period)
			if pb, ok := a.reg[barID].(*widget.ProgressBar); ok {
				pb.SetValue(v)
			}
			if l := a.lbl(lblID); l != nil {
				l.SetText(fmt.Sprintf("%.0f%%", v*100))
			}
		}
		set("pbCPU", "lblCPU", 3.1)
		set("pbRAM", "lblRAM", 5.7)
		set("pbDisk", "lblDisk", 8.3)
	}
}

// sinApprox — синус в диапазоне [-1,1] без импорта math (ряд Тейлора после
// приведения аргумента к [-π,π]); точности для индикаторов достаточно.
func sinApprox(x float64) float64 {
	const pi = 3.14159265358979
	for x > pi {
		x -= 2 * pi
	}
	for x < -pi {
		x += 2 * pi
	}
	x2 := x * x
	return x * (1 - x2/6*(1-x2/20*(1-x2/42)))
}
