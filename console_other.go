//go:build !windows

package ProgressBar

// enableVT 非 Windows 平台默认支持 ANSI 转义
func enableVT() bool { return true }
