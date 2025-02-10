package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ModelMapConfig 模型映射插件的配置
type ModelMapConfig struct {
	Mappings map[string]string `json:"mappings"` // 模型名称映射
}

// ModelMapPlugin 模型映射插件
type ModelMapPlugin struct {
	config ModelMapConfig
	logger Logger
}

// NewModelMapPlugin 创建新的模型映射插件
func NewModelMapPlugin(logger Logger) *ModelMapPlugin {
	return &ModelMapPlugin{
		config: ModelMapConfig{
			Mappings: make(map[string]string),
		},
		logger: logger,
	}
}

// Configure 配置插件
func (p *ModelMapPlugin) Configure(config json.RawMessage) error {
	return json.Unmarshal(config, &p.config)
}

// AddMapping 添加模型映射
func (p *ModelMapPlugin) AddMapping(from, to string) {
	p.config.Mappings[from] = to
}

func (p *ModelMapPlugin) BeforeRequest(req *http.Request) error {
	// 只处理 chat/completions 请求
	if !strings.Contains(req.URL.Path, "/chat/completions") {
		return nil
	}

	// 读取请求体
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	defer req.Body.Close()

	// 解析请求体
	var requestBody map[string]interface{}
	if err := json.Unmarshal(body, &requestBody); err != nil {
		return err
	}

	// 检查是否存在模型字段
	if model, ok := requestBody["model"].(string); ok {
		// 如果存在映射，则替换模型名称
		if mappedModel, exists := p.config.Mappings[model]; exists {
			p.logger.Info(fmt.Sprintf("Mapping model from %s to %s", model, mappedModel))
			requestBody["model"] = mappedModel

			// 重新编码请求体
			newBody, err := json.Marshal(requestBody)
			if err != nil {
				return err
			}

			// 替换请求体
			req.Body = io.NopCloser(bytes.NewBuffer(newBody))
			req.ContentLength = int64(len(newBody))
		}
	}

	return nil
}

func (p *ModelMapPlugin) AfterResponse(resp *http.Response) error {
	return nil
}
