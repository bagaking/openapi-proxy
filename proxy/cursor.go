package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bagaking/openapi-proxy/plugin"
	"github.com/gin-gonic/gin"
)

func StartCursorProxy(conf Config, mappings map[string]string) (gin.HandlerFunc, error) {

	// 创建代理实例
	proxy := NewProxy(conf)

	// 创建并配置模型映射插件
	modelMapPlugin := plugin.NewModelMapPlugin(proxy.logger)

	// 通过方法添加映射
	// for key, val := range mappings {
	// 	modelMapPlugin.AddMapping(key, val)
	// }

	// 或者通过配置文件加载
	pluginConfig := plugin.ModelMapConfig{
		Mappings: mappings,
	}

	configBytes, _ := json.Marshal(pluginConfig)
	modelMapPlugin.Configure(configBytes)

	// 注册插件 Mock 插件
	// 创建 Mock 插件
	mockPlugin := plugin.NewMockPlugin(proxy.logger)

	// 添加自定义规则
	// 添加测试规则
	mockPlugin.AddRule(
		// 匹配条件 - 精确匹配
		func(req *plugin.ChatRequest) bool {
			if len(req.Messages) == 0 {
				return false
			}
			lastContent := req.Messages[len(req.Messages)-1].Content
			return req.Model == "gpt-4o" &&
				len(req.Messages) > 0 &&
				(lastContent == "Testing. Just say hi and nothing else." || strings.Contains(lastContent, "Test prompt using"))
		},
		// 响应生成器
		func(req *plugin.ChatRequest) (*plugin.ChatResponse, error) {
			return &plugin.ChatResponse{
				ID:      fmt.Sprintf("mock-%d", time.Now().Unix()),
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []plugin.ChatChoice{
					{
						Index: 0,
						Message: []plugin.ChatMessage{
							{
								Role:    "assistant",
								Content: "Hi",
							},
						},
						FinishReason: "stop",
					},
				},
				Usage: struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				}{
					PromptTokens:     len(req.Messages),
					CompletionTokens: 1,
					TotalTokens:      len(req.Messages) + 1,
				},
			}, nil
		},
	)

	// 注册插件
	proxy.RegisterPlugin(mockPlugin)

	// 如果配置了 ListenAddr，则启动独立服务器
	if conf.ListenAddr != "" {
		go func() {
			proxy.logger.Info("Starting proxy server on ", conf.ListenAddr)
			if err := proxy.Start(); err != nil {
				proxy.logger.Error("Failed to start proxy:", err)
			}
		}()
	}

	// 返回处理函数
	return proxy.handleRequest, nil
}
