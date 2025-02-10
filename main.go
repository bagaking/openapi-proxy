package main

import (
	"os"
	"time"

	"github.com/bagaking/openapi-proxy/proxy"
)

var (
	VolcEnging = proxy.Config{
		TargetURL: "https://ark.cn-beijing.volces.com/api/v3",
	}

	TencentCloud = proxy.Config{
		TargetURL: "https://api.lkeap.cloud.tencent.com/api/v1",
	}
)

func main() {

	conf := proxy.Config{
		ListenAddr: ":8899",
		TargetURL:  VolcEnging.TargetURL,
		Headers: map[string]string{
			"Authorization": "Bearer " + os.Getenv("OPENAI_API_KEY"),
		},
		// 配置支持的模型
		Models: []proxy.ModelInfo{
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
			{
				ID:      "deepseek-r1",
				Object:  "model",
				Created: time.Now().Unix(),
				OwnedBy: "organization",
			},
		},
	}
	if _, err := proxy.StartCursorProxy(conf, map[string]string{
		"gpt-4":         "ep-20250208163847-fv7w8",
		"gpt-4o":        "ep-20250208163847-fv7w8",
		"gpt-3.5-turbo": "ep-20250208163847-fv7w8",
	}); err != nil {
		os.Exit(1)
	}
}
