// Демонстрация Headless GUI Engine — все виджеты
//
// UI загружается из assets/ui/demo.xaml.
// Фон — gui/background.png, масштабируется под заданное разрешение.
// Кадры сохраняются в out_test/frame_XXXXXX.png (только изменившиеся тайлы 64×64).
//
// Запуск:
//
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

func main() {
	const (
		screenW = 1920
		screenH = 1024
		fps     = 20
	)

	// ── Движок ──────────────────────────────────────────────────────────────
	eng := engine.New(screenW, screenH, fps)

	// ── Фон ─────────────────────────────────────────────────────────────────
	//if err := eng.SetBackgroundFile("gui/149013-1920x1200.jpg"); err != nil {
	//	log.Printf("предупреждение: фон не загружен: %v", err)
	//}

	// ── Загрузка UI из XAML ──────────────────────────────────────────────────
	root, registry, err := widget.LoadUIFromXAMLFile("assets/ui/demo.xaml")
	if err != nil {
		log.Fatalf("ошибка загрузки UI: %v", err)
	}

	// ── Получаем виджеты из реестра ─────────────────────────────────────────
	// Существующие виджеты
	loginInput := getWidget[*widget.TextInput](registry, "loginInput")
	roleDD := getWidget[*widget.Dropdown](registry, "roleDD")
	btnLogin := getWidget[*widget.Button](registry, "btnLogin")
	pb := getWidget[*widget.ProgressBar](registry, "pb")
	pbPct := getWidget[*widget.Label](registry, "pbPct")
	frameLabel := getWidget[*widget.Label](registry, "frameLabel")
	tilesLabel := getWidget[*widget.Label](registry, "tilesLabel")
	bytesLabel := getWidget[*widget.Label](registry, "bytesLabel")
	statusLabel := getWidget[*widget.Label](registry, "statusLabel")

	// ── Новые виджеты ───────────────────────────────────────────────────────
	cbRemember := getWidget[*widget.CheckBox](registry, "cbRemember")
	rbLDAP := getWidget[*widget.RadioButton](registry, "rbLDAP")
	rbOTP := getWidget[*widget.RadioButton](registry, "rbOTP")
	rbCert := getWidget[*widget.RadioButton](registry, "rbCert")

	tsAutoRefresh := getWidget[*widget.ToggleSwitch](registry, "tsAutoRefresh")
	tsNotify := getWidget[*widget.ToggleSwitch](registry, "tsNotify")

	sliderSpeed := getWidget[*widget.Slider](registry, "sliderSpeed")
	lblSpeedVal := getWidget[*widget.Label](registry, "lblSpeedVal")
	sliderQuality := getWidget[*widget.Slider](registry, "sliderQuality")
	lblQualityVal := getWidget[*widget.Label](registry, "lblQualityVal")

	eventLog := getWidget[*widget.ListView](registry, "eventLog")

	cbVerbose := getWidget[*widget.CheckBox](registry, "cbVerbose")
	cbCompress := getWidget[*widget.CheckBox](registry, "cbCompress")

	// ── Предустановка данных ────────────────────────────────────────────────
	if loginInput != nil {
		loginInput.SetText("admin")
	}
	if roleDD != nil {
		roleDD.SetSelected(0)
	}

	// ── Обработчики: CheckBox ───────────────────────────────────────────────
	if cbRemember != nil {
		cbRemember.OnChange = func(checked bool) {
			state := "выключено"
			if checked {
				state = "включено"
			}
			logEvent(eventLog, "CheckBox «Запомнить»: %s", state)
		}
	}
	if cbVerbose != nil {
		cbVerbose.OnChange = func(checked bool) {
			logEvent(eventLog, "Verbose: %v", checked)
		}
	}
	if cbCompress != nil {
		cbCompress.OnChange = func(checked bool) {
			logEvent(eventLog, "Сжатие: %v", checked)
		}
	}

	// ── Обработчики: RadioButton ────────────────────────────────────────────
	if rbLDAP != nil {
		rbLDAP.OnChange = func(selected bool) {
			if selected {
				logEvent(eventLog, "Аутентификация: LDAP / AD")
			}
		}
	}
	if rbOTP != nil {
		rbOTP.OnChange = func(selected bool) {
			if selected {
				logEvent(eventLog, "Аутентификация: OTP")
			}
		}
	}
	if rbCert != nil {
		rbCert.OnChange = func(selected bool) {
			if selected {
				logEvent(eventLog, "Аутентификация: Сертификат")
			}
		}
	}

	// ── Обработчики: ToggleSwitch ───────────────────────────────────────────
	if tsAutoRefresh != nil {
		tsAutoRefresh.OnChange = func(on bool) {
			state := "ВЫКЛ"
			if on {
				state = "ВКЛ"
			}
			logEvent(eventLog, "Авто-обновление: %s", state)
		}
	}
	if tsNotify != nil {
		tsNotify.OnChange = func(on bool) {
			state := "ВЫКЛ"
			if on {
				state = "ВКЛ"
			}
			logEvent(eventLog, "Уведомления: %s", state)
		}
	}

	// ── Обработчики: Slider ─────────────────────────────────────────────────
	if sliderSpeed != nil {
		sliderSpeed.OnChange = func(v float64) {
			if lblSpeedVal != nil {
				lblSpeedVal.SetText(fmt.Sprintf("%.0f FPS", v))
			}
		}
	}
	if sliderQuality != nil {
		sliderQuality.OnChange = func(v float64) {
			if lblQualityVal != nil {
				lblQualityVal.SetText(fmt.Sprintf("%.0f%%", v))
			}
		}
	}

	// ── Обработчики: ListView ───────────────────────────────────────────────
	if eventLog != nil {
		eventLog.OnSelect = func(idx int, text string) {
			log.Printf("ListView: выбран [%d] %q", idx, text)
		}
		// Начальные записи журнала
		eventLog.AddItem(fmt.Sprintf("[%s] Движок запущен", timeNow()))
		eventLog.AddItem(fmt.Sprintf("[%s] UI загружен из XAML", timeNow()))
		eventLog.AddItem(fmt.Sprintf("[%s] Разрешение: %d×%d", timeNow(), screenW, screenH))
	}

	// ── Обработчики: Кнопки ─────────────────────────────────────────────────
	if btnLogin != nil {
		btnLogin.OnClick = func() {
			login := ""
			if loginInput != nil {
				login = loginInput.GetText()
			}
			authType := "LDAP"
			if rbOTP != nil && rbOTP.IsSelected() {
				authType = "OTP"
			} else if rbCert != nil && rbCert.IsSelected() {
				authType = "Cert"
			}
			remember := false
			if cbRemember != nil {
				remember = cbRemember.IsChecked()
			}

			logEvent(eventLog, "ВХОД: user=%s auth=%s remember=%v", login, authType, remember)
			log.Printf("[ Войти ] user=%s auth=%s remember=%v", login, authType, remember)
		}
	}
	if btn, ok := registry["btnCancel"].(*widget.Button); ok {
		btn.OnClick = func() {
			logEvent(eventLog, "Отмена нажата")
			log.Println("[ Отмена ] нажата")
		}
	}
	if btn, ok := registry["btnReset"].(*widget.Button); ok {
		btn.OnClick = func() {
			if pb != nil {
				pb.SetValue(0)
			}
			logEvent(eventLog, "Прогресс сброшен")
		}
	}

	// Переключение темы
	var darkMode = true
	if btn, ok := registry["btnThemeToggle"].(*widget.Button); ok {
		btn.OnClick = func() {
			darkMode = !darkMode
			if darkMode {
				eng.SetTheme(widget.DarkTheme())
				btn.Text = "Светлая тема"
			} else {
				eng.SetTheme(widget.LightTheme())
				btn.Text = "Тёмная тема"
			}
			logEvent(eventLog, "Тема: dark=%v", darkMode)
		}
	}

	// Кнопка экспорта
	if btn, ok := registry["btnExport"].(*widget.Button); ok {
		btn.OnClick = func() {
			fmtName := "PNG"
			if rbJPEG := getWidget[*widget.RadioButton](registry, "rbJPEG"); rbJPEG != nil && rbJPEG.IsSelected() {
				fmtName = "JPEG"
			} else if rbRAW := getWidget[*widget.RadioButton](registry, "rbRAW"); rbRAW != nil && rbRAW.IsSelected() {
				fmtName = "RAW"
			}
			quality := 80.0
			if sliderQuality != nil {
				quality = sliderQuality.Value()
			}
			logEvent(eventLog, "Экспорт: формат=%s качество=%.0f%%", fmtName, quality)
		}
	}

	// Передаём фокус полю логина
	eng.SetFocus(loginInput)

	// ── Запуск ──────────────────────────────────────────────────────────────
	eng.SetRoot(root)
	eng.SaveFrames("out_test")
	eng.Start()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// ── Анимация (имитация живых данных) ────────────────────────────────────
	go func() {
		t := time.NewTicker(50 * time.Millisecond)
		defer t.Stop()
		var step float64
		var tick int
		for {
			select {
			case <-t.C:
				step += 0.006
				if step > 1.0 {
					step = 0
					tick++
					if roleDD != nil {
						roleDD.SetSelected(tick % 4)
					}
					if btnLogin != nil {
						btnLogin.SetPressed(tick%8 == 0)
					}
					// Имитация переключения CheckBox каждый 3-й цикл
					if cbVerbose != nil && tick%3 == 0 {
						cbVerbose.SetChecked(!cbVerbose.IsChecked())
						logEvent(eventLog, "Auto: Verbose=%v", cbVerbose.IsChecked())
					}
					// Имитация переключения ToggleSwitch
					if tsNotify != nil && tick%5 == 0 {
						tsNotify.SetOn(!tsNotify.IsOn())
						logEvent(eventLog, "Auto: Уведомления=%v", tsNotify.IsOn())
					}
				}
				if pb != nil {
					pb.SetValue(step)
				}
				if pbPct != nil {
					pbPct.SetText(fmt.Sprintf("%.0f%%", step*100))
				}
				// Обновляем Slider скорости анимационно
				if sliderSpeed != nil && tick < 3 {
					sv := sliderSpeed.Value() + 0.1
					if sv > 60 {
						sv = 1
					}
					sliderSpeed.SetValue(sv)
					if lblSpeedVal != nil {
						lblSpeedVal.SetText(fmt.Sprintf("%.0f FPS", sv))
					}
				}
				if statusLabel != nil {
					elapsed := time.Since(start).Truncate(time.Millisecond)
					autoRefresh := "вкл"
					if tsAutoRefresh != nil && !tsAutoRefresh.IsOn() {
						autoRefresh = "выкл"
					}
					statusLabel.SetText(fmt.Sprintf(
						"Status: running | elapsed: %s | %.1f%% | auto: %s",
						elapsed, step*100, autoRefresh,
					))
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// ── Потребитель кадров ───────────────────────────────────────────────────
	var totalFrames, totalTiles int
	var totalBytes int64

	const tilesTotal = ((screenW + 63) / 64) * ((screenH + 63) / 64)

	fmt.Printf("Разрешение: %d×%d  |  FPS: %d  |  Тайлов на холсте: %d\n",
		screenW, screenH, fps, tilesTotal)
	fmt.Printf("%-8s  %-5s  %-9s  %s\n", "Кадр", "Тайлы", "Байт", "Время")
	fmt.Println("─────────────────────────────────────────────────────")

	for {
		select {
		case frame, ok := <-eng.Frames():
			if !ok {
				goto done
			}
			totalFrames++
			var nb int
			for _, t := range frame.Tiles {
				nb += len(t.Data)
			}
			totalTiles += len(frame.Tiles)
			totalBytes += int64(nb)

			if frameLabel != nil {
				frameLabel.SetText(fmt.Sprintf("Кадр:    %d", frame.Seq))
			}
			if tilesLabel != nil {
				tilesLabel.SetText(fmt.Sprintf("Тайлов:  %d из %d",
					len(frame.Tiles), tilesTotal))
			}
			if bytesLabel != nil {
				bytesLabel.SetText(fmt.Sprintf("Байт/кадр: %d", nb))
			}

			fmt.Printf("%-8d  %-5d  %-9d  %s\n",
				frame.Seq, len(frame.Tiles), nb,
				frame.Timestamp.Format("15:04:05.000"))

		case <-ctx.Done():
			eng.Stop()
			goto done
		}
	}

done:
	duration := time.Since(start)
	fmt.Println("\n─── Итоги ─────────────────────────────────────────────────")
	fmt.Printf("Разрешение:       %d×%d\n", screenW, screenH)
	fmt.Printf("Продолжительность: %s\n", duration.Truncate(time.Millisecond))
	fmt.Printf("Кадров:   %d  (%.1f FPS)\n", totalFrames, float64(totalFrames)/duration.Seconds())
	fmt.Printf("Тайлов:   %d  (всего %d на холсте)\n", totalTiles, tilesTotal)
	fmt.Printf("Байт:     %d  (%.1f KB/s)\n", totalBytes, float64(totalBytes)/1024/duration.Seconds())
}

// ─── Утилиты ────────────────────────────────────────────────────────────────

// getWidget извлекает типизированный виджет из registry по ID.
func getWidget[T widget.Widget](registry map[string]widget.Widget, id string) T {
	var zero T
	w, ok := registry[id]
	if !ok {
		return zero
	}
	typed, ok := w.(T)
	if !ok {
		return zero
	}
	return typed
}

// logEvent добавляет запись в ListView журнала событий.
func logEvent(lv *widget.ListView, format string, args ...any) {
	if lv == nil {
		return
	}
	msg := fmt.Sprintf("[%s] %s", timeNow(), fmt.Sprintf(format, args...))
	lv.AddItem(msg)
	log.Println(msg)
}

// timeNow возвращает текущее время в формате HH:MM:SS.
func timeNow() string {
	return time.Now().Format("15:04:05")
}
