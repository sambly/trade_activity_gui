package tray

import (
	"runtime"

	"fyne.io/systray"
)

// Callbacks описывают действия, которые должны выполняться при взаимодействии с иконкой в трее.
type Callbacks struct {
	OnShow func()
	OnHide func()
	OnQuit func()
}

// Run запускает системный трей и блокирует вызывающую горутину до вызова systray.Quit().
// Предполагается, что Run вызывается из отдельной горутины (не из main()), поскольку wails.Run
// должен оставаться на "основном" потоке ради стабильности своего цикла сообщений WebView2.
// systray устроен так же — его циклу сообщений тоже нужен один закреплённый OS-поток, поэтому
// закрепляем именно эту горутину явным runtime.LockOSThread().
func Run(iconBytes []byte, cb Callbacks) {
	runtime.LockOSThread()
	systray.Run(func() { onReady(iconBytes, cb) }, func() {})
}

func onReady(iconBytes []byte, cb Callbacks) {
	systray.SetIcon(iconBytes)
	systray.SetTooltip("Trade Activity")

	mShow := systray.AddMenuItem("Показать", "Показать окно")
	mHide := systray.AddMenuItem("Свернуть", "Свернуть в трей")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Выход", "Закрыть приложение")

	// Левый клик по иконке — показать окно. Правый клик открывает меню выше (поведение по умолчанию).
	systray.SetOnTapped(func() {
		if cb.OnShow != nil {
			cb.OnShow()
		}
	})

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				if cb.OnShow != nil {
					cb.OnShow()
				}
			case <-mHide.ClickedCh:
				if cb.OnHide != nil {
					cb.OnHide()
				}
			case <-mQuit.ClickedCh:
				if cb.OnQuit != nil {
					cb.OnQuit()
				}
				systray.Quit()
				return
			}
		}
	}()
}
