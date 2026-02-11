package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"
)

// Unit 单位枚举
type Unit int

const (
	UnitRaw   Unit = iota // 0: 仅数值
	UnitBytes             // 1: 字节友好换算
)

type Config struct {
	current      int64
	total        int64
	width        int    //进度条宽度
	showProgress bool   //是否显示进度(x/y)
	showPercent  bool   //是否显示百分比
	showSpeed    bool   //是否显示速度
	showUsedTime bool   //是否显示耗时
	showLastTime bool   //是否显示剩余时间
	startTime    int64  //开始时间(毫秒)
	last         int64  //计算速度用
	lastTime     int64  //计算速度用
	unit         Unit   // 单位
	totalStr     string // 缓存格式化后的总数
}

// 获取终端宽度的函数
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 100
	}
	// 默认返回值
	return width
}

func ProgressBar(total int64) *Config {
	c := &Config{
		current:      0,
		startTime:    time.Now().UnixNano() / int64(time.Millisecond),
		total:        total,
		width:        getTerminalWidth(), // 获取终端宽度
		showProgress: true,
		showPercent:  false,
		showSpeed:    false,
		last:         0,
		lastTime:     0,
		unit:         UnitRaw,                  // 默认单位为原始数值
		totalStr:     fmt.Sprintf("%d", total), // 默认单位0时直接格式化
	}
	// 监听窗口大小变化信号（SIGWINCH）
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)

	go func() {
		for {
			select {
			case <-sigwinch:
				c.width = getTerminalWidth()
			}
		}
	}()
	return c
}

func (c *Config) ShowProgress(flag bool) *Config {
	c.showProgress = flag
	return c
}

func (c *Config) ShowPercent(flag bool) *Config {
	c.showPercent = flag
	return c
}

func (c *Config) ShowSpeed(flag bool) *Config {
	c.showSpeed = flag
	return c
}

func (c *Config) SetUnit(unit Unit) *Config {
	c.unit = unit
	// 一次性计算完成，不关心后续变动
	if unit == UnitBytes {
		c.totalStr = formatBytes(c.total)
	} else {
		c.totalStr = fmt.Sprintf("%d", c.total)
	}
	return c
}

func (c *Config) Update(current int64) {
	if current > c.current && current <= c.total {
		c.current = current
	}
	c.ShowProgressBar()
}

func (c *Config) Increment() {
	if c.current < c.total {
		c.current++
	}
	c.ShowProgressBar()
}

func (c *Config) ShowProgressBar() {
	// 计算进度百分比
	var percent float64
	if c.total > 0 {
		percent = float64(c.current) / float64(c.total) * 100
	}

	// 计算时间相关数据
	currentTime := time.Now().UnixNano() / int64(time.Millisecond)
	usedTime := currentTime - c.startTime // 已用时间(毫秒)
	var lastTime int64
	if percent > 0 {
		lastTime = int64(float64(usedTime)*(100/percent) - float64(usedTime))
	}
	if c.total > 0 {
		percent = float64(c.current) / float64(c.total) * 100
	}

	// 格式化当前数值
	var currentStr string
	if c.unit == UnitBytes {
		currentStr = formatBytes(c.current)
	} else {
		currentStrLength := len(c.totalStr)
		format := fmt.Sprintf("%%%dd", currentStrLength)
		currentStr = fmt.Sprintf(format, c.current)
	}

	output := ""

	// 添加百分比(紧跟在进度条后面)
	if c.showPercent {
		output += fmt.Sprintf(" %.1f%%", percent)
	}

	// 添加进度(x/y) - 可独立控制
	if c.showProgress {
		if c.showPercent {
			output += fmt.Sprintf(" (%s/%s)", currentStr, c.totalStr)
		} else {
			output += fmt.Sprintf(" %s/%s", currentStr, c.totalStr)
		}
	}

	// 添加速度
	if c.showSpeed {
		now := time.Now().UnixNano() / int64(time.Millisecond)
		if c.lastTime > 0 {
			duration := now - c.lastTime
			if duration > 0 {
				speed := float64(c.current-c.last) / (float64(duration) / 1000.0)
				if c.unit == UnitBytes {
					speedBytes := int64(speed * 1024) // 将KB/s转换为B/s
					output += fmt.Sprintf(" (%s/s)", formatBytes(speedBytes))
				} else {
					output += fmt.Sprintf(" (%7.2f items/s)", speed)
				}
			}
		}
		c.last = c.current
		c.lastTime = now
	}

	// 添加时间信息
	if c.showUsedTime && c.showLastTime && percent > 0 {
		output += fmt.Sprintf(" [%s/%s]", formatTime(usedTime), formatTime(lastTime))
	} else {
		if c.showUsedTime {
			output += fmt.Sprintf(" [已用:%s]", formatTime(usedTime))
		}
		if c.showLastTime && percent > 0 {
			output += fmt.Sprintf(" [剩余:%s]", formatTime(lastTime))
		}
	}
	// 计算进度条长度
	progressWidth := c.width - len(output) - 2
	progressLength := int(float64(progressWidth) * percent / 100)

	// 构建进度条字符串
	bar := ""
	for i := 0; i < progressWidth; i++ {
		if i < progressLength {
			bar += "="
		} else if i == progressLength && progressLength < progressWidth {
			bar += ">"
		} else {
			bar += " "
		}
	}

	// 构建输出字符串
	output = "\r[" + bar + "]" + output

	// 输出进度条
	fmt.Print(output)

	// 如果完成，则换行
	if c.current >= c.total {
		fmt.Println()
	}
}

// 辅助函数：格式化时间(毫秒转为 时:分:秒)
func formatTime(ms int64) string {
	seconds := ms / 1000
	hours := seconds / 3600
	seconds = seconds % 3600
	minutes := seconds / 60
	seconds = seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// 辅助函数：将字节数转换为友好格式
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

func (c *Config) ShowUsedTime(flag bool) {
	c.showUsedTime = flag
}

func (c *Config) ShowLastTime(flag bool) {
	c.showLastTime = flag
}

// 示例用法
func main() {
	// 创建一个总大小为10000的进度条
	pb := ProgressBar(10000)

	// 显示百分比和速度
	pb.ShowProgress(true)
	pb.ShowPercent(true)
	pb.ShowSpeed(true)
	pb.ShowUsedTime(true)
	pb.ShowLastTime(true)
	pb.SetUnit(UnitBytes) // 使用字节单位

	// 模拟进度更新
	for i := 0; i <= 10000; i++ {
		pb.Update(int64(i))
		time.Sleep(1 * time.Millisecond) // 模拟处理时间
	}

	fmt.Println("完成!")
}
