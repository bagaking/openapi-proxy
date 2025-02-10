package proxy

import (
	"bufio"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

// streamResponseWriter 包装 gin.ResponseWriter 以支持流式响应
type streamResponseWriter struct {
	gin.ResponseWriter
	http.Flusher
	written    bool
	size       int
	statusCode int
}

// 创建新的 streamResponseWriter
func newStreamResponseWriter(original gin.ResponseWriter) *streamResponseWriter {
	flusher, _ := original.(http.Flusher)
	return &streamResponseWriter{
		ResponseWriter: original,
		Flusher:        flusher,
		statusCode:     http.StatusOK, // 默认状态码
	}
}

// Write 实现 io.Writer
func (w *streamResponseWriter) Write(data []byte) (int, error) {
	w.written = true
	n, err := w.ResponseWriter.Write(data)
	w.size += n
	if err == nil && w.Flusher != nil {
		w.Flusher.Flush()
	}
	return n, err
}

// WriteString 实现 io.StringWriter
func (w *streamResponseWriter) WriteString(s string) (int, error) {
	w.written = true
	n, err := w.ResponseWriter.WriteString(s)
	w.size += n
	if err == nil && w.Flusher != nil {
		w.Flusher.Flush()
	}
	return n, err
}

// Written 实现 gin.ResponseWriter
func (w *streamResponseWriter) Written() bool {
	return w.written
}

// WriteHeader 实现 http.ResponseWriter
func (w *streamResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// Status 实现 gin.ResponseWriter
func (w *streamResponseWriter) Status() int {
	return w.statusCode
}

// Size 实现 gin.ResponseWriter
func (w *streamResponseWriter) Size() int {
	return w.size
}

// Hijack 实现 http.Hijacker (如果原始 writer 支持)
func (w *streamResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("streaming response writer doesn't support hijacking")
}

// CloseNotify 实现 http.CloseNotifier (如果原始 writer 支持)
func (w *streamResponseWriter) CloseNotify() <-chan bool {
	if cn, ok := w.ResponseWriter.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	return nil
}

// Flush 实现 http.Flusher
func (w *streamResponseWriter) Flush() {
	if w.Flusher != nil {
		w.Flusher.Flush()
	}
}

// Pusher 实现 http.Pusher (如果原始 writer 支持)
func (w *streamResponseWriter) Pusher() http.Pusher {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher
	}
	return nil
}
