// Package ProgressBar 提供一个高性能的终端进度条。
//
// 核心设计（借鉴 mpb / pb / progressbar 等成熟库）：
//   - 更新与渲染解耦：Add/Update 仅做原子操作，TTY 模式下由后台
//     goroutine 按 refreshRate 定时重绘，高频调用不会产生额外终端 IO；
//   - 非 TTY（重定向到文件/管道）自动降级：无后台 goroutine，
//     仅在跨越 10% 档位时输出纯文本行；
//   - 每帧复用 bytes.Buffer 组装后单次写出，用 \r\033[K 整行清除。
package ProgressBar

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

// Unit 进度数值的显示单位
type Unit int

const (
	UnitRaw   Unit = iota // 原始数值
	UnitBytes             // 字节友好换算(KB/MB/...)
)

const (
	defaultRefreshRate  = 150 * time.Millisecond // 渲染节流间隔
	minRefreshRate      = 30 * time.Millisecond
	defaultTermFallback = 100 // 终端宽度获取失败时的回退值
)

// Stats 传递给自定义装饰函数的进度快照
type Stats struct {
	Current int64
	Total   int64         // 未知总量时为 0
	Percent float64       // 未知总量时为 0
	Speed   float64       // 单位/秒（滚动窗口平均）
	Elapsed time.Duration // 已用时间
	ETA     time.Duration // 预计剩余时间（未知总量或速度为 0 时为 0）
}

// DecoratorFunc 自定义装饰函数，返回的文本渲染到进度条行内
type DecoratorFunc func(Stats) string

type ProgressBar struct {
	// ---- 原子状态：更新路径无锁 ----
	current    atomic.Int64
	total      atomic.Int64
	started    atomic.Bool
	finished   atomic.Bool
	startNano  atomic.Int64 // 首次更新时间(UnixNano)，0 表示未开始
	lastBucket atomic.Int64 // 非 TTY 模式下已输出的百分比档位(0~10)

	// ---- 配置与渲染状态：mu 保护 ----
	mu           sync.Mutex
	refreshRate  time.Duration
	width        int // 固定宽度；0 表示自适应终端
	unit         Unit
	padWidth     int // UnitRaw 时按总位数补齐，避免宽度抖动
	out          io.Writer
	isTTY        bool // 是否终端输出（决定渲染模式）
	vtOK         bool // 终端是否支持 ANSI 转义（Windows VT 启用失败时为 false，颜色将被剔除）
	theme        Theme
	cursorHidden bool
	showProgress bool
	showPercent  bool
	showSpeed    bool
	showUsedTime bool
	showLastTime bool
	finalMsg     string
	prependFns   []DecoratorFunc
	appendFns    []DecoratorFunc

	sampler speedSampler

	// 复用缓冲，避免每帧分配
	buf       bytes.Buffer // 最终帧
	infoBuf   bytes.Buffer // 信息段
	preBuf    bytes.Buffer // 前缀装饰段
	lastFrame string       // 上一帧内容，用于跳过无变化的重绘
	renders   uint64       // 渲染帧号：驱动弹跳动画与 Tip 动画帧轮换

	stopCh chan struct{} // 关闭以停止后台渲染 goroutine
}

// New 创建进度条。total <= 0 表示总量未知，显示为 "?" 并以弹跳动画代替百分比。
func New(total int64) *ProgressBar {
	p := &ProgressBar{
		refreshRate:  defaultRefreshRate,
		out:          os.Stdout,
		isTTY:        term.IsTerminal(int(os.Stdout.Fd())),
		showProgress: true,
		stopCh:       make(chan struct{}),
	}
	p.total.Store(total)
	p.padWidth = len(strconv.FormatInt(total, 10))
	p.sampler = *newSpeedSampler()
	p.theme = normalizeTheme(ThemeBlock)
	p.vtOK = enableVT() // Windows 传统控制台启用 ANSI 转义序列
	return p
}

// ---- 链式配置（均可在启动前后调用，线程安全）----

// ShowProgress 是否显示进度值(x/y)
func (p *ProgressBar) ShowProgress(flag bool) *ProgressBar {
	p.mu.Lock()
	p.showProgress = flag
	p.mu.Unlock()
	return p
}

// ShowPercent 是否显示百分比
func (p *ProgressBar) ShowPercent(flag bool) *ProgressBar {
	p.mu.Lock()
	p.showPercent = flag
	p.mu.Unlock()
	return p
}

// ShowSpeed 是否显示速度
func (p *ProgressBar) ShowSpeed(flag bool) *ProgressBar {
	p.mu.Lock()
	p.showSpeed = flag
	p.mu.Unlock()
	return p
}

// ShowUsedTime 是否显示已用时间
func (p *ProgressBar) ShowUsedTime(flag bool) *ProgressBar {
	p.mu.Lock()
	p.showUsedTime = flag
	p.mu.Unlock()
	return p
}

// ShowLastTime 是否显示剩余时间
func (p *ProgressBar) ShowLastTime(flag bool) *ProgressBar {
	p.mu.Lock()
	p.showLastTime = flag
	p.mu.Unlock()
	return p
}

// SetUnit 设置显示单位
func (p *ProgressBar) SetUnit(unit Unit) *ProgressBar {
	p.mu.Lock()
	p.unit = unit
	p.mu.Unlock()
	return p
}

// SetTheme 设置进度条字符主题
func (p *ProgressBar) SetTheme(t Theme) *ProgressBar {
	p.mu.Lock()
	p.theme = normalizeTheme(t)
	p.mu.Unlock()
	return p
}

// SetTipFrames 设置头部动画帧（非空时逐帧轮换，优先于主题的 Tip）
func (p *ProgressBar) SetTipFrames(frames ...string) *ProgressBar {
	if len(frames) == 0 {
		return p
	}
	p.mu.Lock()
	p.theme.TipFrames = frames
	p.mu.Unlock()
	return p
}

// SetWidth 固定进度条总宽度；w <= 0 表示每次渲染自适应终端宽度
func (p *ProgressBar) SetWidth(w int) *ProgressBar {
	p.mu.Lock()
	p.width = w
	p.mu.Unlock()
	return p
}

// SetRefreshRate 设置后台渲染间隔（建议在首次更新前调用）
func (p *ProgressBar) SetRefreshRate(d time.Duration) *ProgressBar {
	if d < minRefreshRate {
		d = minRefreshRate
	}
	p.mu.Lock()
	p.refreshRate = d
	p.mu.Unlock()
	return p
}

// SetTotal 修改总量（如动态发现新任务）；t <= 0 切换为未知总量模式
func (p *ProgressBar) SetTotal(t int64) *ProgressBar {
	p.mu.Lock()
	p.padWidth = len(strconv.FormatInt(t, 10))
	p.mu.Unlock()
	p.total.Store(t)
	return p
}

// SetFinalMessage 设置完成后的提示行（Finish 时输出）
func (p *ProgressBar) SetFinalMessage(msg string) *ProgressBar {
	p.mu.Lock()
	p.finalMsg = msg
	p.mu.Unlock()
	return p
}

// PrependFunc 注册渲染在进度条行首的自定义装饰函数
func (p *ProgressBar) PrependFunc(fns ...DecoratorFunc) *ProgressBar {
	p.mu.Lock()
	p.prependFns = append(p.prependFns, fns...)
	p.mu.Unlock()
	return p
}

// AppendFunc 注册渲染在行尾的自定义装饰函数
func (p *ProgressBar) AppendFunc(fns ...DecoratorFunc) *ProgressBar {
	p.mu.Lock()
	p.appendFns = append(p.appendFns, fns...)
	p.mu.Unlock()
	return p
}

// ---- 更新（可从任意 goroutine 调用）----

// Add 相对推进 n 个单位。成本仅为原子操作，渲染由后台定时完成。
func (p *ProgressBar) Add(n int64) {
	if n <= 0 || p.finished.Load() {
		return
	}
	for {
		old := p.current.Load()
		nv := old + n
		if t := p.total.Load(); t > 0 && nv > t {
			nv = t
		}
		if p.current.CompareAndSwap(old, nv) {
			break
		}
	}
	p.onUpdate()
}

// Update 设置当前进度为 n（保持单调不减，兼容旧版语义）
func (p *ProgressBar) Update(n int64) {
	if p.finished.Load() {
		return
	}
	for {
		old := p.current.Load()
		nv := n
		if nv < old {
			nv = old
		}
		if t := p.total.Load(); t > 0 && nv > t {
			nv = t
		}
		if nv == old || p.current.CompareAndSwap(old, nv) {
			break
		}
	}
	p.onUpdate()
}

// Increment 推进 1 个单位
func (p *ProgressBar) Increment() { p.Add(1) }

// onUpdate 首次调用时启动渲染；TTY 走后台节流，非 TTY 仅在跨过 10% 档位时同步输出
func (p *ProgressBar) onUpdate() {
	if p.isTTY {
		if p.started.Swap(true) {
			return // 已启动，交给后台 ticker
		}
		p.startNano.CompareAndSwap(0, time.Now().UnixNano())
		go p.loop()
		return
	}
	// 非 TTY：无后台 goroutine
	if p.started.CompareAndSwap(false, true) {
		p.startNano.CompareAndSwap(0, time.Now().UnixNano())
	}
	t := p.total.Load()
	if t <= 0 {
		return
	}
	bucket := p.current.Load() * 100 / t / 10
	if bucket == p.lastBucket.Swap(bucket) {
		return // 档位未跨过，零成本返回
	}
	p.render(false)
}

// Finish 立即输出最终帧并结束（幂等）。进度自然到达 total 时会自动触发。
func (p *ProgressBar) Finish() { p.render(true) }

// Refresh 手动立即重绘一次（仅 TTY 模式有效；一般无需调用）
func (p *ProgressBar) Refresh() {
	if !p.isTTY {
		return
	}
	p.render(false)
}

// loop 后台渲染循环：按 refreshRate 定时重绘
func (p *ProgressBar) loop() {
	p.mu.Lock()
	rate := p.refreshRate
	p.mu.Unlock()
	ticker := time.NewTicker(rate)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.render(false)
		}
	}
}

// finishLocked 输出收尾（须在 mu 内调用），幂等
func (p *ProgressBar) finishLocked() {
	if p.finished.Swap(true) {
		return
	}
	if p.cursorHidden {
		fmt.Fprint(p.out, "\x1b[?25h") // 恢复光标
		p.cursorHidden = false
	}
	if p.isTTY {
		fmt.Fprintln(p.out)
	}
	if p.finalMsg != "" {
		fmt.Fprintln(p.out, p.finalMsg)
	}
	close(p.stopCh)
}

// formatValue 按单位格式化数值；UnitRaw 时按总位数右对齐补齐，避免宽度抖动
func (p *ProgressBar) formatValue(v int64) string {
	if p.unit == UnitBytes {
		return formatBytes(v)
	}
	if p.padWidth > 0 {
		return fmt.Sprintf("%*d", p.padWidth, v)
	}
	return strconv.FormatInt(v, 10)
}

// getTerminalWidth 获取终端宽度，失败回退默认值
func getTerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return defaultTermFallback
	}
	return w
}
