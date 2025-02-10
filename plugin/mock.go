package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ChatMessage 定义聊天消息结构
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 定义请求结构
type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
	Model    string        `json:"model"`
	Stream   bool          `json:"stream"`
}

type ChatChoice struct {
	Index        int           `json:"index"`
	Message      []ChatMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// ChatResponse 定义响应结构
type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// MockRule 定义匹配规则
type MockRule struct {
	// 匹配条件
	Condition func(req *ChatRequest) bool
	// 生成响应
	Response func(req *ChatRequest) (*ChatResponse, error)
}

// MockPlugin Mock插件
type MockPlugin struct {
	rules  []MockRule
	logger Logger
}

// NewMockPlugin 创建新的Mock插件
func NewMockPlugin(logger Logger) *MockPlugin {
	p := &MockPlugin{
		logger: logger,
	}

	// 添加默认的测试规则
	p.AddRule(
		// 匹配条件
		func(req *ChatRequest) bool {
			if req.Model != "gpt-4o" {
				return false
			}
			if len(req.Messages) < 2 {
				return false
			}
			lastMsg := req.Messages[len(req.Messages)-1]
			return lastMsg.Role == "user" &&
				lastMsg.Content == "Testing. Just say hi and nothing else."
		},
		// 响应生成器
		func(req *ChatRequest) (*ChatResponse, error) {
			return &ChatResponse{
				ID:      "mock-response",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   req.Model,
				Choices: []ChatChoice{
					{
						Index: 0,
						Message: []ChatMessage{
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

	return p
}

// AddRule 添加匹配规则
func (p *MockPlugin) AddRule(condition func(req *ChatRequest) bool, response func(req *ChatRequest) (*ChatResponse, error)) {
	p.rules = append(p.rules, MockRule{
		Condition: condition,
		Response:  response,
	})
}

// 在 plugin/mock.go 中的 BeforeRequest
func (p *MockPlugin) BeforeRequest(req *http.Request) error {
	// 只处理 chat/completions 请求
	if !strings.Contains(req.URL.Path, "/chat/completions") {
		return nil
	}

	// 读取请求体
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	// 解析请求
	var chatReq ChatRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		req.Body = io.NopCloser(bytes.NewBuffer(body))
		return nil
	}

	// 恢复请求体
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	// 添加调试日志
	p.logger.Debug("Mock plugin: Checking request",
		"model", chatReq.Model,
		"messages", chatReq.Messages)

	// 检查是否匹配任何规则
	for _, rule := range p.rules {
		if rule.Condition(&chatReq) {
			p.logger.Info("Mock: matched request, generating mock response")

			// 生成响应
			resp, err := rule.Response(&chatReq)
			if err != nil {
				return err
			}

			// 补充响应字段
			resp.ID = fmt.Sprintf("mock-%d", time.Now().Unix())
			resp.Object = "chat.completion"
			resp.Created = time.Now().Unix()
			resp.Model = chatReq.Model

			// 序列化响应
			respData, err := json.Marshal(resp)
			if err != nil {
				return err
			}

			// 将响应直接写入请求的上下文
			req.Header.Set("X-Mock-Direct-Response", string(respData))

			// 添加调试日志
			p.logger.Debug("Mock plugin: Generated response", string(respData))
			break
		}
	}

	return nil
}

func (p *MockPlugin) AfterResponse(resp *http.Response) error {
	// 检查是否需要mock响应
	mockResp := resp.Request.Header.Get("X-Mock-Response")
	if mockResp == "" {
		return nil
	}

	p.logger.Info("Mock: intercepting response")

	// 获取流式标志
	isStream := resp.Request.Header.Get("X-Mock-Stream") == "true"

	// 创建新的响应
	if isStream {
		// 流式响应
		var chatResp ChatResponse
		if err := json.Unmarshal([]byte(mockResp), &chatResp); err != nil {
			p.logger.Error("Mock: Failed to unmarshal response", err)
			return err
		}

		newBody := &bytes.Buffer{}
		for _, choice := range chatResp.Choices {
			chunk := map[string]interface{}{
				"id":      chatResp.ID,
				"object":  "chat.completion.chunk",
				"created": chatResp.Created,
				"model":   chatResp.Model,
				"choices": []map[string]interface{}{
					{
						"index":         choice.Index,
						"delta":         choice.Message[0],
						"finish_reason": choice.FinishReason,
					},
				},
			}
			jsonData, _ := json.Marshal(chunk)
			newBody.WriteString("data: ")
			newBody.Write(jsonData)
			newBody.WriteString("\n\n")
		}
		newBody.WriteString("data: [DONE]\n\n")

		p.logger.Debug("Mock: Generated stream response", newBody.String())

		resp.Body = io.NopCloser(newBody)
		resp.Header.Set("Content-Type", "text/event-stream")
	} else {
		p.logger.Debug("Mock: Using normal response", mockResp)
		resp.Body = io.NopCloser(bytes.NewBufferString(mockResp))
		resp.Header.Set("Content-Type", "application/json")
	}

	// 设置成功状态码
	resp.StatusCode = http.StatusOK
	resp.Status = http.StatusText(http.StatusOK)

	return nil
}

// Configure 配置插件
func (p *MockPlugin) Configure(config json.RawMessage) error {
	// 可以添加配置逻辑
	return nil
}
