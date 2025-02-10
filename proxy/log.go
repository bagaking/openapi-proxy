package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Logger 接口定义
type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Error(args ...interface{})
}

// DefaultLogger 实现
type DefaultLogger struct {
	debug *log.Logger
	info  *log.Logger
	error *log.Logger
}

func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		debug: log.New(os.Stdout, "[DEBUG] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile),
		info:  log.New(os.Stdout, "[INFO] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile),
		error: log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile),
	}
}

func (l *DefaultLogger) Debug(args ...interface{}) {
	l.debug.Println(args...)
}

func (l *DefaultLogger) Info(args ...interface{}) {
	l.info.Println(args...)
}

func (l *DefaultLogger) Error(args ...interface{}) {
	l.error.Println(args...)
}

// LoggingTransport 自定义传输层
type LoggingTransport struct {
	Transport http.RoundTripper
	Logger    Logger
}

func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// 记录请求详情
	t.Logger.Info(fmt.Sprintf("[Request] %s %s", req.Method, req.URL))
	t.Logger.Debug("Request Headers:", req.Header)

	if req.Body != nil && !strings.Contains(req.Header.Get("Content-Type"), "text/event-stream") {
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(body))
		t.Logger.Debug("Request Body:", string(body))
	}

	// 执行请求
	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		t.Logger.Error("Request failed:", err)
		return nil, err
	}

	duration := time.Since(start)

	// 记录响应信息，但不读取流式响应的内容
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Logger.Info(fmt.Sprintf("[Response] Streaming response started - Status: %d, Duration: %v",
			resp.StatusCode, duration))
		t.Logger.Debug("Response Headers:", resp.Header)
	} else {
		t.Logger.Info(fmt.Sprintf("[Response] Complete - Status: %d, Duration: %v",
			resp.StatusCode, duration))
		t.Logger.Debug("Response Headers:", resp.Header)

		if resp.Body != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body = io.NopCloser(bytes.NewBuffer(body))
			t.Logger.Debug("Response Body:", string(body))
		}
	}

	return resp, nil
}
