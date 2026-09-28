//go:build windows

package ProgressBar

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT 为 Windows 传统控制台启用 ANSI 转义序列支持
// （Windows Terminal 默认支持）。返回是否支持 ANSI 转义。
func enableVT() bool {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) == nil {
		if windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil {
			return true
		}
	}
	return false
}
