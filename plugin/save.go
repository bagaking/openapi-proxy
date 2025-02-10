package plugin

import "net/http"

// SavePlugin 保存对话记录插件
type SavePlugin struct {
	StoragePath string
}

func (p *SavePlugin) BeforeRequest(req *http.Request) error {
	// 可以在这里对请求做处理
	return nil
}

func (p *SavePlugin) AfterResponse(resp *http.Response) error {
	// 保存对话内容
	return nil
}
