# 用法

## 同一个 gin server 上启动多个代理

```go
package main

import (
    "time"
    "github.com/gin-gonic/gin"
    "github.com/bagaking/openapi-proxy/proxy"
)

func main() {
    r := gin.Default()

    // OpenAI 代理配置
    openaiConf := proxy.Config{
        TargetURL:  "https://ark.cn-beijing.volces.com/api/v3",
        PathPrefix: "/openai", // 设置与路由组相同的前缀
        Headers: map[string]string{
            "Authorization": "Bearer your-token",
        },
        Models: []proxy.ModelInfo{
            {
                ID:      "gpt-4o",
                Object:  "model",
                Created: time.Now().Unix(),
                OwnedBy: "organization",
            },
        },
    }

    // 获取处理函数
    openaiHandler, err := proxy.StartCursorProxy(openaiConf)
    if err != nil {
        panic(err)
    }

    // 在现有 gin 应用中使用
    r.Group(openaiConf.PathPrefix).Any("/*path", openaiHandler)

    // 可以添加多个不同的 AI 服务代理
    anthropicConf := proxy.Config{
        TargetURL:  "https://api.anthropic.com",
        PathPrefix: "/anthropic",
        Headers: map[string]string{  // optional
            "x-api-key": "your-anthropic-key",
        },
    }
    anthropicHandler, err := proxy.StartCursorProxy(anthropicConf)
    if err != nil {
        panic(err)
    }
    r.Group(anthropicConf.PathPrefix).Any("/*path", anthropicHandler)

    // 其他路由
    r.GET("/ping", func(c *gin.Context) {
        c.JSON(200, gin.H{
            "message": "pong",
        })
    })

    // 启动服务器
    r.Run(":8080")
}
```

## 通过代理创建服务 

```go
package main

import (
    "time"
    "github.com/bagaking/openapi-proxy/proxy"
)

func main() {
    conf := proxy.Config{
        ListenAddr: ":8899",
        TargetURL:  "https://ark.cn-beijing.volces.com/api/v3",
        Headers: map[string]string{ // optional
            "Authorization": "Bearer your-token",
        },
    }

    // 启动独立服务器
    _, err := proxy.StartCursorProxy(conf)
    if err != nil {
        panic(err)
    }

    // 保持主程序运行
    select {}
}

```

## 直接提供服务，可以用于 localhost

```
go install github.com/bagaking/openapi-proxy
openapi-proxy
```