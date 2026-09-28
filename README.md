# ProgressBar

一个高性能的 Go 终端进度条库。

```bash
go get github.com/jlqwer/ProgressBar
```

## 特性

- **更新与渲染解耦**：`Add/Update` 仅做原子操作，后台 goroutine 定时（默认 150ms）重绘，高频调用（如字节级下载回调）不产生额外终端 IO
- **多种样式主题**：块字符 / 经典 / 全 ASCII 预设，支持自定义全部字符与头部动画帧
- **亚字符精度**：内置 `▏▎▍▌▋▊▉` 部分块填充，进度以 1/8 字符粒度平滑推进
- **颜色支持**：内置 16 色 ANSI 常量，可用于主题字符和自定义装饰；非 TTY 自动剔除
- **速度采样**：滚动窗口平均（10 样本 × 250ms），速度显示平稳不跳动
- **并发安全**：`Add/Update/Increment` 可从任意 goroutine 调用
- **io 流集成**：`ProxyReader/ProxyWriter` 包装后配合 `io.Copy` 零侵入显示进度
- **未知总量**：`total <= 0` 时显示 `n/?` 并以弹跳动画代替百分比
- **终端自适应**：每次渲染获取终端宽度，窗口缩放即时生效；中文/全角/emoji 按宽度 2 计算
- **非 TTY 降级**：重定向到文件/管道时自动输出纯文本（无 ANSI 码，仅跨 10% 档位时输出一行）

## 快速开始

```go
package main

import (
	"time"

	pb "github.com/jlqwer/ProgressBar"
)

func main() {
	p := pb.New(10000).
		ShowPercent(true).
		ShowSpeed(true).
		ShowUsedTime(true).
		ShowLastTime(true)

	for i := 0; i < 10000; i++ {
		time.Sleep(1 * time.Millisecond)
		p.Increment()
	}
	p.Finish()
}
```

更多示例见 [example/main.go](example/main.go)，运行 `go run ./example`。

## 用法说明

### 更新进度

```go
p.Add(1024)    // 相对推进 n 个单位（推荐，成本为原子操作）
p.Update(512)  // 设置绝对进度（保持单调不减，兼容旧版语义）
p.Increment()  // 推进 1
```

进度自然到达 `total` 时自动收尾（输出最终帧并换行）；`Finish()` 可提前结束，幂等可重复调用。

### 显示选项

```go
p.ShowProgress(true)  // 进度值 (x/y)
p.ShowPercent(true)   // 百分比 50.0%
p.ShowSpeed(true)     // 速度 (12.34 MB/s 或 120.00 items/s)
p.ShowUsedTime(true)  // 已用时间 [已用:00:01:23]
p.ShowLastTime(true)  // 剩余时间（按当前速度估算）
p.SetUnit(pb.UnitBytes)   // 数值按字节友好换算；默认 UnitRaw
```

`ShowUsedTime` 和 `ShowLastTime` 同时开启时合并显示为 `[已用/剩余]`。

### 主题与样式

```go
p.SetTheme(pb.ThemeBlock)   // [████▍    ] 默认，含亚字符精度
p.SetTheme(pb.ThemeClassic) // [===>    ]
p.SetTheme(pb.ThemeASCII)   // [===>....]

// 完全自定义
p.SetTheme(pb.Theme{
	BarStart: "<", Filler: "#", Tip: "@", Padding: "-", BarEnd: ">",
})

// 头部动画帧：逐帧轮换，优先于主题的 Tip
p.SetTipFrames("▶", "◆", "●", "◆")
```

### 颜色

零依赖的 ANSI 常量，可直接用于主题字符或装饰函数：

```go
p.SetTheme(pb.Theme{
	Filler:  pb.FgGreen + "█" + pb.Reset,
	Padding: pb.Faint + "░" + pb.Reset,
})
p.PrependFunc(func(s pb.Stats) string {
	return pb.FgCyan + "下载中" + pb.Reset + " "
})
```

可用常量：`FgBlack~FgWhite`、`FgHiBlack~FgHiWhite`、`Bold`、`Faint`、`Italic`、`Underline`、`Reset`。
重定向到文件/管道或终端不支持 ANSI 时自动剔除颜色码。

### 自定义装饰

```go
p.PrependFunc(func(s pb.Stats) string {  // 渲染在行首
	return fmt.Sprintf("[%d/%d]", s.Current, s.Total)
})
p.AppendFunc(func(s pb.Stats) string {   // 渲染在行尾
	return fmt.Sprintf("%.1f/s", s.Speed)
})
```

`Stats` 包含 `Current`、`Total`、`Percent`、`Speed`（单位/秒）、`Elapsed`、`ETA`。

### 文件流集成

```go
resp, _ := http.Get(url)
p := pb.New(resp.ContentLength).
	ShowPercent(true).
	SetUnit(pb.UnitBytes)

io.Copy(dst, p.NewProxyReader(resp.Body)) // 读取自动推进，Close 自动 Finish
```

`NewProxyWriter` 同理，适用于上传。

### 其他配置

```go
p.SetTotal(2048)                    // 动态修改总量（如分块下载发现新块）
p.SetWidth(80)                      // 固定总宽度；默认 0 自适应终端
p.SetRefreshRate(100 * time.Millisecond) // 渲染间隔，最低 30ms
p.SetFinalMessage("✓ 下载完成")      // Finish 时额外输出的终态行
p.Refresh()                         // 手动立即重绘（一般无需调用）
```

## 显示效果

```
[█████████████████████▌          ] 60.0% (  30.0 MB/  50.0 MB) ( 24.5 MB/s) [00:00:01/00:00:00]
```

## 设计说明

- **渲染节流**：更新仅写原子变量，TTY 模式下由后台 goroutine 按 `refreshRate` 重绘；帧内容未变化时跳过终端 IO
- **单帧单写**：每帧复用 `bytes.Buffer` 组装后一次性写出，帧间无内存分配
- **非 TTY**：无后台 goroutine，仅在跨过 10% 档位时输出一行纯文本（已剔除全部 ANSI 码）
- **Windows 兼容**：自动启用传统控制台的 VT 转义；失败时降级为纯 `\r` 覆盖并剔除颜色

## License

Apache 2.0
