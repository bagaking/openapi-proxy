package plugin

// import (
// 	"fmt"
// 	"net/http"
// 	"sync"
// )

// // 示例插件：指标收集
// type MetricsPlugin struct {
// 	logger  Logger
// 	metrics map[string]int
// 	mu      sync.RWMutex
// }

// func NewMetricsPlugin(logger Logger) *MetricsPlugin {
// 	return &MetricsPlugin{
// 		logger:  logger,
// 		metrics: make(map[string]int),
// 	}
// }

// func (p *MetricsPlugin) BeforeRequest(req *http.Request) error {
// 	p.mu.Lock()
// 	defer p.mu.Unlock()

// 	key := fmt.Sprintf("%s %s", req.Method, req.URL.Path)
// 	p.metrics[key]++

// 	p.logger.Info("MetricsPlugin: Request count for", key, ":", p.metrics[key])
// 	return nil
// }

// func (p *MetricsPlugin) AfterResponse(resp *http.Response) error {
// 	p.mu.Lock()
// 	defer p.mu.Unlock()

// 	key := fmt.Sprintf("status_%d", resp.StatusCode)
// 	p.metrics[key]++

// 	p.logger.Info("MetricsPlugin: Response count for", key, ":", p.metrics[key])
// 	return nil
// }
