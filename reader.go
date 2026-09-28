package ProgressBar

import "io"

// ProxyReader 包装 io.Reader，每次读取自动推进进度；Close 时自动 Finish。
// 典型用法：
//
//	resp, _ := http.Get(url)
//	bar := ProgressBar.New(resp.ContentLength).ShowPercent(true).SetUnit(UnitBytes)
//	io.Copy(dst, bar.NewProxyReader(resp.Body))
type ProxyReader struct {
	r   io.Reader
	bar *ProgressBar
}

// NewProxyReader 创建代理 Reader，读取时自动推进进度
func (p *ProgressBar) NewProxyReader(r io.Reader) *ProxyReader {
	return &ProxyReader{r: r, bar: p}
}

func (pr *ProxyReader) Read(b []byte) (int, error) {
	n, err := pr.r.Read(b)
	if n > 0 {
		pr.bar.Add(int64(n))
	}
	return n, err
}

func (pr *ProxyReader) Close() error {
	if c, ok := pr.r.(io.Closer); ok {
		err := c.Close()
		pr.bar.Finish()
		return err
	}
	pr.bar.Finish()
	return nil
}

// ProxyWriter 包装 io.Writer，每次写入自动推进进度；Close 时自动 Finish。
type ProxyWriter struct {
	w   io.Writer
	bar *ProgressBar
}

// NewProxyWriter 创建代理 Writer，写入时自动推进进度
func (p *ProgressBar) NewProxyWriter(w io.Writer) *ProxyWriter {
	return &ProxyWriter{w: w, bar: p}
}

func (pw *ProxyWriter) Write(b []byte) (int, error) {
	n, err := pw.w.Write(b)
	if n > 0 {
		pw.bar.Add(int64(n))
	}
	return n, err
}

func (pw *ProxyWriter) Close() error {
	if c, ok := pw.w.(io.Closer); ok {
		err := c.Close()
		pw.bar.Finish()
		return err
	}
	pw.bar.Finish()
	return nil
}
