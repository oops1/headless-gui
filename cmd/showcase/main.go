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
	"log"
	"time"

	"github.com/oops1/headless-gui/engine"
	"github.com/oops1/headless-gui/widget"
	"github.com/oops1/headless-gui/window"
)

func main() {
	const (
		screenW = 1280
		screenH = 900
	)

	// ─── Движок ─────────────────────────────────────────────────────────────
	eng := engine.New(screenW, screenH, 30)

	// ─── Загрузка UI из XAML ────────────────────────────────────────────────
	root, reg, err := widget.LoadUIFromXAMLFile("../assets/ui/showcase.xaml")
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
	win := window.New(eng, "GuiEngine — Widget Showcase")
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
