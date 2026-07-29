//go:build !windows

package tray

// HideFromTaskbar не поддерживается на этой платформе: у Windows-специфичного трюка
// (WS_EX_TOOLWINDOW) нет общего аналога вне Windows.
func HideFromTaskbar(windowClassName string) {}
