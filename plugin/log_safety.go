package plugin

import "fmt"

func payloadLogSummary(body []byte) string {
	return fmt.Sprintf("%d bytes", len(body))
}

func chatRequestLogSummary(req *ChatRequest) map[string]interface{} {
	return map[string]interface{}{
		"model":         req.Model,
		"message_count": len(req.Messages),
		"stream":        req.Stream,
	}
}

func chatResponseLogSummary(resp *ChatResponse) map[string]interface{} {
	if resp == nil {
		return map[string]interface{}{"response": "nil"}
	}
	return map[string]interface{}{
		"id":                resp.ID,
		"model":             resp.Model,
		"choice_count":      len(resp.Choices),
		"prompt_tokens":     resp.Usage.PromptTokens,
		"completion_tokens": resp.Usage.CompletionTokens,
		"total_tokens":      resp.Usage.TotalTokens,
	}
}
