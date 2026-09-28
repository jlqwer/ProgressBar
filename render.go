package ProgressBar

import (
	"bytes"
	"fmt"
	"time"
)

// render 渲染一帧（须在 mu 内调用）。
// final=true 表示收尾帧：跳过"内容未变化则跳过输出"的优化。
func (p *ProgressBar) render(final bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.finished.Load() && !final {
		return
	}
	p.renders++

	cur := p.current.Load()
	total := p.total.Load()
	now := time.Now()

	var elapsed time.Duration
	if start := p.startNano.Load(); start != 0 {
		elapsed = now.Sub(time.Unix(0, start))
	}
	speed := p.sampler.sample(cur, now)

	unknown := total <= 0
	percent := 0.0
	var eta time.Duration
	if !unknown {
		percent = float64(cur) / float64(total) * 100
		if speed > 0 && cur < total {
			eta = time.Duration(float64(total-cur)/speed) * time.Second
		}
	}
	st := Stats{
		Current: cur,
		Total:   total,
		Percent: percent,
		Speed:   speed,
		Elapsed: elapsed,
		ETA:     eta,
	}

	// 前缀装饰
	pre := &p.preBuf
	pre.Reset()
	for _, fn := range p.prependFns {
		pre.WriteString(fn(st))
	}

	// 信息段：百分比 / 进度值 / 速度 / 时间
	ib := &p.infoBuf
	ib.Reset()
	if p.showPercent && !unknown {
		fmt.Fprintf(ib, " %.1f%%", percent)
	}
	if p.showProgress {
		if p.showPercent && !unknown {
			ib.WriteString(" (")
		} else {
			ib.WriteString(" ")
		}
		ib.WriteString(p.formatValue(cur))
		ib.WriteByte('/')
		if unknown {
			ib.WriteByte('?')
		} else {
			ib.WriteString(p.formatValue(total))
		}
		if p.showPercent && !unknown {
			ib.WriteByte(')')
		}
	}
	if p.showSpeed && speed > 0 {
		if p.unit == UnitBytes {
			fmt.Fprintf(ib, " (%s/s)", formatBytes(int64(speed)))
		} else {
			fmt.Fprintf(ib, " (%.2f items/s)", speed)
		}
	}
	if !unknown && percent > 0 && p.showUsedTime && p.showLastTime {
		fmt.Fprintf(ib, " [%s/%s]", formatTime(elapsed.Milliseconds()), formatTime(eta.Milliseconds()))
	} else {
		if p.showUsedTime {
			fmt.Fprintf(ib, " [已用:%s]", formatTime(elapsed.Milliseconds()))
		}
		if p.showLastTime && !unknown && percent > 0 {
			fmt.Fprintf(ib, " [剩余:%s]", formatTime(eta.Milliseconds()))
		}
	}
	for _, fn := range p.appendFns {
		ib.WriteString(fn(st))
	}

	// 终端宽度：每次渲染时获取，天然响应窗口变化（替代 SIGWINCH goroutine）
	termW := p.width
	if termW <= 0 {
		termW = getTerminalWidth()
	}

	// 信息段按显示宽度计宽（中文/全角算 2），保证条宽不溢出
	barW := termW - displayWidth(pre.String()) - displayWidth(ib.String()) -
		displayWidth(p.theme.BarStart) - displayWidth(p.theme.BarEnd)
	if barW < 0 {
		barW = 0
	}

	// 组装最终帧
	b := &p.buf
	b.Reset()
	if p.isTTY {
		b.WriteByte('\r')
		if p.vtOK {
			b.WriteString("\x1b[K") // 清除整行，避免残留旧字符
		}
	}
	b.Write(pre.Bytes())
	b.WriteString(p.theme.BarStart)
	p.writeBar(b, barW, percent, unknown)
	b.WriteString(p.theme.BarEnd)
	b.Write(ib.Bytes())

	if !p.isTTY {
		// 非 TTY（重定向/管道）：纯文本 + 换行，不含 ANSI 序列
		b.WriteByte('\n')
	}
	// 首次输出前隐藏光标，避免其在条上闪烁（Finish 时恢复）
	if p.isTTY && p.vtOK && !p.cursorHidden {
		fmt.Fprint(p.out, "\x1b[?25l")
		p.cursorHidden = true
	}
	// 非 TTY 或 VT 不可用：剔除所有 ANSI 码（含主题/装饰中的颜色）
	frame := b.String()
	if !p.isTTY || !p.vtOK {
		frame = stripANSI(frame)
	}
	// 帧内容未变化则跳过终端 IO（TTY/非 TTY 均适用，避免 Finish 与档位触发重复输出）
	if frame != p.lastFrame {
		fmt.Fprint(p.out, frame)
		p.lastFrame = frame
	}

	// 进度自然到达终点时自动收尾
	if final || (!unknown && cur >= total) {
		p.finishLocked()
	}
}

// writeBar 将进度条写入 b（须在 mu 内调用）
func (p *ProgressBar) writeBar(b *bytes.Buffer, barW int, percent float64, unknown bool) {
	if barW <= 0 {
		return
	}
	filler := p.theme.Filler
	padding := p.theme.Padding

	if unknown {
		// 未知总量：弹跳动画（三角波往复）
		headW := barW / 5
		if headW < 1 {
			headW = 1
		}
		span := barW - headW
		period := 2 * span
		if period <= 0 {
			period = 1
		}
		step := int(p.renders % uint64(period))
		if step > span {
			step = period - step
		}
		tip := p.theme.Tip
		if tip == "" {
			tip = filler
		}
		tipW := maxInt(displayWidth(tip), 1)
		for i := 0; i < barW; {
			switch {
			case i >= step && i < step+headW-tipW && step+headW < barW:
				b.WriteString(filler)
				i += maxInt(displayWidth(filler), 1)
			case i >= step && i < step+headW && step+headW < barW:
				b.WriteString(tip) // 头部末格
				i += maxInt(displayWidth(tip), 1)
			default:
				b.WriteString(padding)
				i += maxInt(displayWidth(padding), 1)
			}
		}
		return
	}

	// 已知总量：确定头部字符（动画帧优先）
	tip := p.theme.Tip
	if frames := p.theme.TipFrames; len(frames) > 0 {
		tip = frames[int(p.renders)%len(frames)]
	}
	tipW := maxInt(displayWidth(tip), 1)

	filled := float64(barW) * percent / 100
	if filled >= float64(barW) {
		writeCells(b, filler, barW) // 已完成：无头部
		return
	}
	boundary := int(filled)
	writeCells(b, filler, boundary)

	switch {
	case tip != "" && boundary+tipW <= barW:
		b.WriteString(tip)
		boundary += tipW
	case len(p.theme.PartialRunes) > 0:
		// 亚字符精度：用部分块字符(▏▎▍...)表示不足一格的进度
		frac := filled - float64(boundary)
		if idx := int(frac * float64(len(p.theme.PartialRunes)+1)); idx > 0 {
			b.WriteRune(p.theme.PartialRunes[idx-1])
			boundary++
		}
	}
	writeCells(b, padding, barW-boundary)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
