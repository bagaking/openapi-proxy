package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	pluginPKG "github.com/bagaking/openapi-proxy/plugin"
)

// Proxy OpenAI 协议代理
type Proxy struct {
	config  Config
	plugins []pluginPKG.Plugin
	logger  Logger
	mu      sync.RWMutex
}

// 创建新的代理实例
func NewProxy(cfg Config) *Proxy {
	return &Proxy{
		config:  cfg,
		plugins: make([]pluginPKG.Plugin, 0),
		logger:  NewDefaultLogger(),
	}
}

// RegisterPlugin 注册插件
func (p *Proxy) RegisterPlugin(plugin pluginPKG.Plugin) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.plugins = append(p.plugins, plugin)
}

// corsMiddleware 创建一个统一处理 CORS 的中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 清除任何可能已经设置的 CORS 头部
		c.Writer.Header().Del("Access-Control-Allow-Origin")
		c.Writer.Header().Del("Access-Control-Allow-Methods")
		c.Writer.Header().Del("Access-Control-Allow-Headers")
		c.Writer.Header().Del("Access-Control-Allow-Credentials")

		// 设置 CORS 头部
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, Authorization, accept, origin, Cache-Control, X-Requested-With, baggage, sentry-trace")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		// 处理 OPTIONS 请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()

		// 确保响应中的 CORS 头部是正确的（可能被其他中间件修改）
		c.Writer.Header().Del("Access-Control-Allow-Origin")
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	}
}

// 启动代理服务
func (p *Proxy) Start() error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 使用自定义的 recovery 中间件
	r.Use(p.customRecovery())

	// 添加 CORS 中间件
	r.Use(corsMiddleware())

	// 所有请求都转发
	r.Any("/*path", p.handleRequest)

	return r.Run(p.config.ListenAddr)
}

// 自定义 recovery 中间件
func (p *Proxy) customRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 如果是正常的流式响应中断，忽略它
				if err == http.ErrAbortHandler {
					p.logger.Info("Stream completed normally")
					return
				}

				// 其他 panic 才记录错误
				stack := debug.Stack()
				p.logger.Error(fmt.Sprintf("Panic recovered: %v\n%s", err, string(stack)))
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

func (p *Proxy) handleRequest(c *gin.Context) {
	// OPTIONS 请求已经在 CORS 中间件中处理了
	if c.Request.Method == "OPTIONS" {
		return
	}

	// 包装响应写入器以支持流式响应
	wrappedWriter := newStreamResponseWriter(c.Writer)
	c.Writer = wrappedWriter

	// 处理路径前缀
	requestPath := c.Request.URL.Path
	if p.config.PathPrefix != "" {
		// 如果请求路径不以配置的前缀开头，返回 404
		if !strings.HasPrefix(requestPath, p.config.PathPrefix) {
			c.Status(http.StatusNotFound)
			return
		}
		// 移除前缀，这样后续代码可以正常处理
		c.Request.URL.Path = strings.TrimPrefix(requestPath, p.config.PathPrefix)
	}

	// 3. 检查是否是 models 请求
	if c.Request.URL.Path == "/v1/models" {
		p.handleModelsRequest(c)
		return
	}

	// 4. 解析目标 URL
	targetURL, err := url.Parse(p.config.TargetURL)
	if err != nil {
		p.logger.Error("Failed to parse target URL:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 5. 读取请求体
	reqBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		p.logger.Error("Failed to read request body:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(reqBody))

	// 6. 检查是否是流式请求
	var isStreamRequest bool
	var requestBody map[string]interface{}
	if err := json.NewDecoder(bytes.NewBuffer(reqBody)).Decode(&requestBody); err == nil {
		if stream, ok := requestBody["stream"].(bool); ok && stream {
			isStreamRequest = true
		}
	}

	// 7. 记录请求信息
	p.logger.Info(fmt.Sprintf("Incoming request: %s %s", c.Request.Method, c.Request.URL.Path))
	p.logger.Debug("Request headers:", c.Request.Header)
	if len(reqBody) > 0 {
		p.logger.Debug("Request body:", string(reqBody))
	}

	// 8. 创建反向代理
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			p.logger.Info("Proxying request to:", targetURL.String())

			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.Host = targetURL.Host

			// 删除可能导致目标服务器添加 CORS 头部的请求头
			req.Header.Del("Origin")
			req.Header.Del("Referer")

			// 修改路径处理逻辑
			originalPath := req.URL.Path
			if strings.HasPrefix(originalPath, "/v1/") {
				newPath := strings.TrimPrefix(originalPath, "/v1")
				req.URL.Path = path.Join(targetURL.Path, newPath)
				p.logger.Debug("Rewritten path:", req.URL.Path)
			}

			// 处理认证头
			clientAuth := c.GetHeader("Authorization")
			if clientAuth != "" && clientAuth != "Bearer" {
				req.Header.Set("Authorization", clientAuth)
				p.logger.Debug("Using client Authorization token")
			} else if p.config.Headers["Authorization"] != "" {
				req.Header.Set("Authorization", p.config.Headers["Authorization"])
				p.logger.Debug("Using configured Authorization token")
			} else {
				p.logger.Error("No valid Authorization token available")
			}

			// 复制必要的 headers
			copyHeaders := []string{
				"Content-Type",
				"Accept",
				"Accept-Encoding",
				"User-Agent",
				"Content-Length",
			}
			for _, h := range copyHeaders {
				if v := c.GetHeader(h); v != "" {
					req.Header.Set(h, v)
				}
			}

			// 设置 X-Forwarded-For
			if clientIP, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil {
				if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
					clientIP = prior + ", " + clientIP
				}
				req.Header.Set("X-Forwarded-For", clientIP)
			}

			p.logger.Debug("Final request path:", req.URL.Path)
			p.logger.Debug("Final request headers:", req.Header)
		},
		Transport: &LoggingTransport{
			Transport: &http.Transport{
				ResponseHeaderTimeout: 30 * time.Second,
				DisableKeepAlives:     false,
				MaxIdleConnsPerHost:   100,
				IdleConnTimeout:       90 * time.Second,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				TLSHandshakeTimeout:   10 * time.Second,
			},
			Logger: p.logger,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// 检查是否是正常的流式响应结束
			if err == http.ErrAbortHandler {
				p.logger.Info("Stream completed normally")
				return
			}

			// 忽略已经关闭的连接错误
			if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
				p.logger.Info("Client closed connection")
				return
			}

			// 检查上下文取消
			if err == context.Canceled || err == context.DeadlineExceeded {
				p.logger.Info("Request context canceled:", err)
				return
			}

			// 其他错误才记录
			p.logger.Error("Proxy error:", err)
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(fmt.Sprintf("Proxy Error: %v", err)))
		},
		ModifyResponse: func(resp *http.Response) error {
			p.logger.Info("Received response:", resp.Status)

			// 处理流式响应
			if isStreamRequest {
				// 设置 SSE headers
				resp.Header.Set("Content-Type", "text/event-stream")
				resp.Header.Set("Cache-Control", "no-cache")
				resp.Header.Set("Connection", "keep-alive")
				resp.Header.Set("Transfer-Encoding", "chunked")
				resp.Header.Set("X-Accel-Buffering", "no")
			}

			// 确保删除所有可能的 CORS 头部
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Methods")
			resp.Header.Del("Access-Control-Allow-Headers")
			resp.Header.Del("Access-Control-Allow-Credentials")
			resp.Header.Del("Access-Control-Max-Age")
			resp.Header.Del("Access-Control-Expose-Headers")
			resp.Header.Del("Access-Control-Request-Method")

			return nil
		},
	}

	// 9. 执行请求前的插件
	p.mu.RLock()
	for _, plugin := range p.plugins {
		if err := plugin.BeforeRequest(c.Request); err != nil {
			p.logger.Error("Plugin error:", err)
			p.mu.RUnlock()
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	p.mu.RUnlock()

	// 10. 检查是否有 Mock 直接响应
	if mockResp := c.Request.Header.Get("X-Mock-Direct-Response"); mockResp != "" {
		p.logger.Info("Using mock response directly")
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, mockResp)
		return
	}

	// 11. 执行代理转发
	proxy.ServeHTTP(c.Writer, c.Request)

	// 注意: 这里不会继续执行，因为 ServeHTTP 已经写入了响应
}

// 处理 models 请求
func (p *Proxy) handleModelsRequest(c *gin.Context) {
	// 如果配置中没有模型列表，使用默认值
	models := p.config.Models
	if len(models) == 0 {
		models = []ModelInfo{
			{
				ID:      "gpt-4o",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "organization",
			},
			{
				ID:      "ep-20250208163847-fv7w8",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "organization",
			},
		}
	}

	response := ModelsResponse{
		Object: "list",
		Data:   models,
	}

	c.JSON(http.StatusOK, response)
}
