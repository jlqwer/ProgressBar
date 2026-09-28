package ProgressBar

import "time"

const (
	maxSpeedSamples   = 10                      // 滚动窗口样本数
	minSampleInterval = 250 * time.Millisecond // 最小采样间隔
)

type speedSample struct {
	t time.Time
	v int64
}

// speedSampler 滚动窗口速度采样：
// 对窗口首尾做差求平均，避免按"相邻两次调用间隔"计算导致的剧烈抖动。
type speedSampler struct {
	samples   []speedSample
	lastSpeed float64
}

func newSpeedSampler() *speedSampler {
	return &speedSampler{samples: make([]speedSample, 0, maxSpeedSamples)}
}

// sample 记录一次采样并返回当前平均速度（单位/秒）。
// 距上次采样不足 minSampleInterval 时直接返回缓存值。
func (s *speedSampler) sample(cur int64, now time.Time) float64 {
	if n := len(s.samples); n > 0 && now.Sub(s.samples[n-1].t) < minSampleInterval {
		return s.lastSpeed
	}
	s.samples = append(s.samples, speedSample{now, cur})
	if len(s.samples) > maxSpeedSamples {
		copy(s.samples, s.samples[1:])
		s.samples = s.samples[:maxSpeedSamples-1]
	}
	if len(s.samples) < 2 {
		return s.lastSpeed
	}
	first := s.samples[0]
	dt := now.Sub(first.t).Seconds()
	if dt <= 0 {
		return s.lastSpeed
	}
	s.lastSpeed = float64(cur-first.v) / dt
	if s.lastSpeed < 0 {
		s.lastSpeed = 0
	}
	return s.lastSpeed
}
