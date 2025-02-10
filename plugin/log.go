package plugin

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// LogPlugin 日志插件
type LogPlugin struct {
	LogFile string
}

func (p *LogPlugin) BeforeRequest(req *http.Request) error {
	// 记录请求信息
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	fmt.Printf("[Request] %s %s: %s\n", req.Method, req.URL, string(body))
	return nil
}

func (p *LogPlugin) AfterResponse(resp *http.Response) error {
	// 记录响应信息
	body, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewBuffer(body))

	fmt.Printf("[Response] Status: %d, Body: %s\n", resp.StatusCode, string(body))
	return nil
}
