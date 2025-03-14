package plugin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type testLogger struct{}

func (testLogger) Debug(args ...interface{}) {}
func (testLogger) Info(args ...interface{})  {}
func (testLogger) Error(args ...interface{}) {}

func TestModelMapPluginBeforeRequestPreservesBodyWithoutMapping(t *testing.T) {
	originalBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)
	req := newChatRequest(t, originalBody)
	plugin := NewModelMapPlugin(testLogger{})

	if err := plugin.BeforeRequest(req); err != nil {
		t.Fatalf("BeforeRequest returned error: %v", err)
	}

	got := readRequestBody(t, req)
	if !bytes.Equal(got, originalBody) {
		t.Fatalf("body changed without mapping:\nwant %s\n got %s", originalBody, got)
	}
	if req.ContentLength != int64(len(originalBody)) {
		t.Fatalf("content length changed without mapping: want %d, got %d", len(originalBody), req.ContentLength)
	}
}

func TestModelMapPluginBeforeRequestRewritesMappedModel(t *testing.T) {
	req := newChatRequest(t, []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	plugin := NewModelMapPlugin(testLogger{})
	plugin.AddMapping("gpt-4o", "provider-model")

	if err := plugin.BeforeRequest(req); err != nil {
		t.Fatalf("BeforeRequest returned error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(readRequestBody(t, req), &got); err != nil {
		t.Fatalf("rewritten body is not JSON: %v", err)
	}
	if got["model"] != "provider-model" {
		t.Fatalf("model was not rewritten: got %v", got["model"])
	}
}

func newChatRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, "http://example.test/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func readRequestBody(t *testing.T, req *http.Request) []byte {
	t.Helper()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return body
}
