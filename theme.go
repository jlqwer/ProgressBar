package ProgressBar

import (
	"bytes"
	"strings"
)

// Theme 进度条字符样式。所有字符按显示宽度参与布局，可内嵌 ANSI 颜色码
// （非 TTY 输出时会自动剔除颜色码）。
type Theme struct {
	BarStart     string   // 左边界
	Filler       string   // 已完成填充符
	Tip          string   // 静态头部字符；为空表示无头部
	TipFrames    []string // 头部动画帧：非空时逐帧轮换并优先于 Tip（建议等宽）
	PartialRunes []rune   // 亚字符精度填充序列(如 ▏▎▍▌▋▊▉)，在 Tip/TipFrames 为空时生效
	Padding      string   // 未完成区填充符
	BarEnd       string   // 右边界
}

// 内置主题预设
var (
	// ThemeBlock 块字符 + 亚字符精度填充（默认）：[████▍    ]
	ThemeBlock = Theme{
		BarStart:     "[",
		Filler:       "█",
		PartialRunes: []rune("▏▎▍▌▋▊▉"),
		Padding:      " ",
		BarEnd:       "]",
	}
	// ThemeClassic 经典样式：[===>    ]
	ThemeClassic = Theme{BarStart: "[", Filler: "=", Tip: ">", Padding: " ", BarEnd: "]"}
	// ThemeASCII 全 ASCII：[===>....]
	ThemeASCII = Theme{BarStart: "[", Filler: "=", Tip: ">", Padding: ".", BarEnd: "]"}
)

// ANSI 颜色/属性码。可直接用在主题字符或装饰函数返回值中；
// 非 TTY（或 Windows VT 启用失败）输出时会被自动剔除。
const (
	Reset     = "\x1b[0m"
	Bold      = "\x1b[1m"
	Faint     = "\x1b[2m"
	Italic    = "\x1b[3m"
	Underline = "\x1b[4m"

	FgBlack   = "\x1b[30m"
	FgRed     = "\x1b[31m"
	FgGreen   = "\x1b[32m"
	FgYellow  = "\x1b[33m"
	FgBlue    = "\x1b[34m"
	FgMagenta = "\x1b[35m"
	FgCyan    = "\x1b[36m"
	FgWhite   = "\x1b[37m"

	FgHiBlack   = "\x1b[90m"
	FgHiRed     = "\x1b[91m"
	FgHiGreen   = "\x1b[92m"
	FgHiYellow  = "\x1b[93m"
	FgHiBlue    = "\x1b[94m"
	FgHiMagenta = "\x1b[95m"
	FgHiCyan    = "\x1b[96m"
	FgHiWhite   = "\x1b[97m"
)

// stripANSI 剔除字符串中的 ANSI 转义序列
func stripANSI(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
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
		b.WriteRune(r)
	}
	return b.String()
}

// normalizeTheme 补全必要字段，避免零宽填充导致布局错误
func normalizeTheme(t Theme) Theme {
	if t.Filler == "" {
		t.Filler = "="
	}
	if t.Padding == "" {
		t.Padding = " "
	}
	return t
}

// writeCells 按显示宽度重复写入 s，凑满 cells 个单元格
func writeCells(b *bytes.Buffer, s string, cells int) {
	w := displayWidth(s)
	if w < 1 {
		w = 1
	}
	for ; cells >= w; cells -= w {
		b.WriteString(s)
	}
}
