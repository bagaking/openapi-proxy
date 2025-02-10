package proxy

// Config 配置结构
type Config struct {
	ListenAddr string            // 监听地址
	TargetURL  string            // 目标服务地址
	PathPrefix string            // 路由前缀，如 "/openai"
	Headers    map[string]string // 需要添加的 header
	Models     []ModelInfo       // 支持的模型列表
}

// ModelInfo 模型信息
type ModelInfo struct {
	ID      string `json:"id"`       // 模型ID
	Object  string `json:"object"`   // 对象类型，固定为 "model"
	Created int64  `json:"created"`  // 创建时间
	OwnedBy string `json:"owned_by"` // 所有者
}

// ModelsResponse models API 的响应格式
type ModelsResponse struct {
	Object string      `json:"object"` // 固定为 "list"
	Data   []ModelInfo `json:"data"`   // 模型列表
}
