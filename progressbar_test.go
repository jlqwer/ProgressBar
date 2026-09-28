package ProgressBar

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// 验证：TTY 渲染、清行前缀、光标隐藏、自动收尾、宽度约束
func TestTTYRender(t *testing.T) {
	var buf bytes.Buffer
	p := New(100)
	p.isTTY = true
	p.vtOK = true
	p.started.Store(true) // 阻止 onUpdate 启动真实后台循环，保证测试确定性
	p.out = &buf
	p.ShowPercent(true).ShowUsedTime(true).ShowLastTime(true)
	p.PrependFunc(func(Stats) string { return "中文前缀" })

	// 首帧：隐藏光标 + \r + 清行
	p.Add(50)
	p.render(false)
	frame1 := buf.String()
	if !strings.HasPrefix(frame1, "\x1b[?25l") {
		t.Fatalf("首帧前应隐藏光标, got %q", frame1)
	}
	if !strings.Contains(frame1, "\r\x1b[K") {
		t.Fatalf("TTY 帧应包含 \\r\\x1b[K, got %q", frame1)
	}
	if !strings.Contains(frame1, "50.0%") || !strings.Contains(frame1, " 50/100") {
		t.Fatalf("帧内容缺失: %q", frame1)
	}
	// 行宽约束：剔除前缀后显示宽度不得超过终端回退宽度
	core := strings.TrimPrefix(strings.TrimPrefix(frame1, "\x1b[?25l"), "\r\x1b[K")
	if w := displayWidth(core); w > defaultTermFallback {
		t.Fatalf("帧宽 %d 超过终端宽度 %d", w, defaultTermFallback)
	}

	// 相同内容不重复输出
	buf.Reset()
	p.render(false)
	if buf.Len() != 0 {
		t.Fatalf("内容未变不应输出, got %q", buf.String())
	}

	// 进度推进后输出新帧，到达 total 自动收尾并恢复光标
	time.Sleep(300 * time.Millisecond) // 让速度采样有数据
	buf.Reset()
	p.Add(50)
	p.render(false)
	frame2 := buf.String()
	if !strings.Contains(frame2, "100.0%") {
		t.Fatalf("应包含 100.0%%: %q", frame2)
	}
	if !strings.Contains(frame2, "\x1b[?25h") {
		t.Fatalf("收尾时应恢复光标: %q", frame2)
	}
	if !strings.HasSuffix(frame2, "\n") {
		t.Fatalf("收尾帧后应换行: %q", frame2)
	}
	if !p.finished.Load() {
		t.Fatal("到达 total 应自动标记完成")
	}

	// 完成后不再输出
	buf.Reset()
	p.Add(1) // finished 后无效
	p.render(false)
	if buf.Len() != 0 {
		t.Fatalf("完成后不应输出, got %q", buf.String())
	}
}

// 验证：主题字符与亚字符精度填充
func TestWriteBarTheme(t *testing.T) {
	p := New(100)

	// 经典样式：填充 + 静态 Tip + 圆点空白
	p.theme = normalizeTheme(ThemeASCII)
	var b bytes.Buffer
	p.writeBar(&b, 10, 25.0, false)
	if got, want := b.String(), "=="+">"+strings.Repeat(".", 7); got != want {
		t.Errorf("ASCII 25%% = %q, want %q", got, want)
	}

	// 块字符 + 亚字符精度：2.5 格 → ██▍(idx=4→第4个部分rune)
	p.theme = normalizeTheme(ThemeBlock)
	b.Reset()
	p.writeBar(&b, 10, 25.0, false)
	if got, want := b.String(), "██"+string(ThemeBlock.PartialRunes[3])+strings.Repeat(" ", 7); got != want {
		t.Errorf("Block 25%% = %q, want %q", got, want)
	}

	// 无 Tip 时整格进度不产生部分字符
	b.Reset()
	p.writeBar(&b, 10, 20.0, false)
	if got, want := b.String(), "██"+strings.Repeat(" ", 8); got != want {
		t.Errorf("Block 20%% = %q, want %q", got, want)
	}

	// 100% 全填充且无 Tip
	b.Reset()
	p.writeBar(&b, 10, 100.0, false)
	if got := b.String(); got != strings.Repeat("█", 10) {
		t.Errorf("Block 100%% = %q", got)
	}

	// TipFrames 按帧号轮换
	p.theme = normalizeTheme(ThemeASCII)
	p.theme.TipFrames = []string{"▶", "◆"}
	p.renders = 0
	b.Reset()
	p.writeBar(&b, 10, 25.0, false)
	if !strings.HasPrefix(b.String(), "==▶") {
		t.Errorf("TipFrames 第 0 帧应为 ▶: %q", b.String())
	}
	p.renders = 1
	b.Reset()
	p.writeBar(&b, 10, 25.0, false)
	if !strings.HasPrefix(b.String(), "==◆") {
		t.Errorf("TipFrames 第 1 帧应为 ◆: %q", b.String())
	}
}

// 验证：非 TTY 输出自动剔除颜色码
func TestColorsStrippedOnNonTTY(t *testing.T) {
	var buf bytes.Buffer
	p := New(100)
	p.isTTY = false
	p.vtOK = false
	p.out = &buf
	p.SetTheme(Theme{Filler: FgRed + "█" + Reset, Padding: " ", BarStart: "[", BarEnd: "]"})
	p.PrependFunc(func(Stats) string { return FgCyan + "彩色" + Reset })
	p.Add(50) // 非 TTY：跨过 10% 档位同步渲染
	if strings.ContainsRune(buf.String(), 0x1b) {
		t.Fatalf("非 TTY 输出不应包含 ANSI 码: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "彩色") {
		t.Fatalf("剔除颜色后文本应保留: %q", buf.String())
	}
}

// 验证：速度采样平稳性
func TestSpeedSampler(t *testing.T) {
	s := newSpeedSampler()
	now := time.Now()
	for i := 0; i < 6; i++ {
		s.sample(int64(i*1000), now.Add(time.Duration(i)*500*time.Millisecond))
	}
	if got := s.sample(6000, now.Add(3*time.Second)); got < 1900 || got > 2100 {
		t.Fatalf("采样速度应接近 2000/s, got %f", got)
	}
	// 间隔不足时返回缓存
	if got := s.sample(9999, now.Add(3100*time.Millisecond)); got != s.lastSpeed {
		t.Fatalf("采样间隔不足应返回缓存值")
	}
}

// 验证：宽度计算与 ANSI 剔除
func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"abc", 3},
		{"中文", 4},
		{"\x1b[31m红色\x1b[0m", 4},
		{"ａｂ", 4},
	}
	for _, c := range cases {
		if got := displayWidth(c.s); got != c.want {
			t.Errorf("displayWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
	if got := stripANSI("\x1b[1m\x1b[32mOK\x1b[0m"); got != "OK" {
		t.Errorf("stripANSI = %q, want OK", got)
	}
}
