package main

import (
	"fmt"
	"time"

	pb "github.com/jlqwer/ProgressBar"
)

func main() {
	downloadDemo()
	fileCounterDemo()
	unknownTotalDemo()
	styledDemo()
}

// 示例 1：字节单位 + 速度/已用/剩余时间（模拟下载 50MB）
func downloadDemo() {
	fmt.Println("示例 1：50MB 下载（字节单位）")
	total := int64(50 << 20)
	p := pb.New(total).
		ShowPercent(true).
		ShowSpeed(true).
		ShowUsedTime(true).
		ShowLastTime(true).
		SetUnit(pb.UnitBytes)

	chunk := int64(512 << 10)
	for cur := int64(0); cur < total; cur += chunk {
		time.Sleep(20 * time.Millisecond)
		p.Add(chunk)
	}
	p.Finish()
	fmt.Println()
}

// 示例 2：普通计数 + 自定义前缀装饰
func fileCounterDemo() {
	fmt.Println("示例 2：处理 200 个文件（自定义前缀装饰）")
	p := pb.New(200).
		ShowPercent(true).
		ShowSpeed(true).
		PrependFunc(func(s pb.Stats) string { return "处理文件 " })
	for i := 0; i < 200; i++ {
		time.Sleep(8 * time.Millisecond)
		p.Increment()
	}
	p.Finish()
	fmt.Println()
}

// 示例 3：未知总量（弹跳动画）+ 完成消息
func unknownTotalDemo() {
	fmt.Println("示例 3：未知总量（弹跳动画）")
	p := pb.New(-1).
		ShowSpeed(true).
		SetFinalMessage("扫描完成!")
	for i := 0; i < 60; i++ {
		time.Sleep(30 * time.Millisecond)
		p.Add(int64(1 + i%7))
	}
	p.Finish()
	fmt.Println()
}

// 示例 4：自定义主题 + 颜色装饰 + 头部动画帧
func styledDemo() {
	fmt.Println("示例 4：主题/颜色/头部动画")
	p := pb.New(300).
		ShowPercent(true).
		ShowSpeed(true).
		SetTheme(pb.ThemeASCII).
		SetTipFrames("▶", "◆", "●", "◆")
	p.PrependFunc(func(s pb.Stats) string {
		return pb.FgCyan + "上传备份" + pb.Reset + " "
	})
	for i := 0; i < 300; i++ {
		time.Sleep(6 * time.Millisecond)
		p.Increment()
	}
	p.Finish()
}
