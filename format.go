package ProgressBar

import "fmt"

// formatTime 毫秒转为 时:分:秒
func formatTime(ms int64) string {
	seconds := ms / 1000
	hours := seconds / 3600
	seconds = seconds % 3600
	minutes := seconds / 60
	seconds = seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// formatBytes 字节数转友好格式(KB/MB/...)
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%3d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%6.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// displayWidth 计算字符串在终端上的显示宽度：
// 先剔除 ANSI 转义序列，再按字符宽度累计（东亚宽字符算 2）。
func displayWidth(s string) int {
	w := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			// CSI 序列以字母结束
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			continue
		}
		w += runeWidth(r)
	}
	return w
}

func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 32 || (r >= 0x7f && r < 0xa0):
		return 0 // 控制字符
	case isWideRune(r):
		return 2
	}
	return 1
}

// isWideRune 判断东亚宽字符（覆盖 CJK/全角/emoji 常用区段）
func isWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK 部首、符号
		r >= 0x3041 && r <= 0x33FF, // 假名、注音
		r >= 0x3400 && r <= 0x4DBF, // CJK 扩展 A
		r >= 0x4E00 && r <= 0x9FFF, // CJK 基本
		r >= 0xA000 && r <= 0xA4CF, // 彝文
		r >= 0xAC00 && r <= 0xD7A3, // Hangul 音节
		r >= 0xF900 && r <= 0xFAFF, // CJK 兼容表意
		r >= 0xFE30 && r <= 0xFE4F, // CJK 兼容形式
		r >= 0xFF00 && r <= 0xFF60, // 全角形式
		r >= 0xFFE0 && r <= 0xFFE6, // 全角符号
		r >= 0x1F300 && r <= 0x1F64F, // emoji
		r >= 0x20000 && r <= 0x3FFFD: // CJK 扩展 B+
		return true
	}
	return false
}
