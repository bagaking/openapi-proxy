package plugin

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type captureLogger struct {
	entries []string
}

func (l *captureLogger) Debug(args ...interface{}) {
	l.entries = append(l.entries, fmt.Sprint(args...))
}

func (l *captureLogger) Info(args ...interface{}) {
	l.entries = append(l.entries, fmt.Sprint(args...))
}

func (l *captureLogger) Error(args ...interface{}) {
	l.entries = append(l.entries, fmt.Sprint(args...))
}

func (l *captureLogger) joined() string {
	return strings.Join(l.entries, "\n")
}

func TestMockPluginLogsSummariesWithoutPromptContent(t *testing.T) {
	logger := &captureLogger{}
	plugin := NewMockPlugin(logger)
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"system","content":"system-secret"},{"role":"user","content":"Testing. Just say hi and nothing else."}]}`)
	req := newChatRequest(t, body)

	if err := plugin.BeforeRequest(req); err != nil {
		t.Fatalf("BeforeRequest returned error: %v", err)
	}

	logged := logger.joined()
	assertDoesNotContain(t, logged, "system-secret")
	assertDoesNotContain(t, logged, "Testing. Just say hi and nothing else.")
	assertDoesNotContain(t, logged, "Hi")
	assertContains(t, logged, "message_count")
	assertContains(t, logged, "choice_count")
}

func TestMockPluginAfterResponseLogsPayloadSizes(t *testing.T) {
	logger := &captureLogger{}
	plugin := NewMockPlugin(logger)
	mockResp := `{"id":"mock","object":"chat.completion","created":1,"model":"gpt-4o","choices":[{"index":0,"message":[{"role":"assistant","content":"response-secret"}],"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	req, err := http.NewRequest(http.MethodPost, "http://example.test/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Mock-Response", mockResp)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Request:    req,
	}

	if err := plugin.AfterResponse(resp); err != nil {
		t.Fatalf("AfterResponse returned error: %v", err)
	}

	logged := logger.joined()
	assertContains(t, logged, "bytes")
	assertDoesNotContain(t, logged, "response-secret")
}

func assertContains(t *testing.T, value, want string) {
	t.Helper()

	if !strings.Contains(value, want) {
		t.Fatalf("log output %q does not contain %q", value, want)
	}
}

func assertDoesNotContain(t *testing.T, value, forbidden string) {
	t.Helper()

	if strings.Contains(value, forbidden) {
		t.Fatalf("log output %q contains %q", value, forbidden)
	}
}
