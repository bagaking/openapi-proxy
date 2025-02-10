package plugin

import (
	"encoding/json"
	"net/http"
)

// Plugin 接口定义
type Plugin interface {
	BeforeRequest(*http.Request) error
	AfterResponse(*http.Response) error
	Configure(json.RawMessage) error // 添加配置方法
}

// Logger 日志接口定义
type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Error(args ...interface{})
}
