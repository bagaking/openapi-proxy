package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStartCursorProxyAppliesModelMappings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const chatCompletionsPath = "v1/chat/completions"

	var gotBody []byte
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("backend read body error = %v, want nil", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backend.Close)

	handler, err := StartCursorProxy(Config{
		TargetURL: backend.URL,
	}, map[string]string{
		"gpt-4o": "provider-model",
	})
	if err != nil {
		t.Fatalf("StartCursorProxy returned error: %v", err)
	}

	router := gin.New()
	router.POST("/"+chatCompletionsPath, handler)
	frontend := httptest.NewServer(router)
	t.Cleanup(frontend.Close)

	reqBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)
	req, err := http.NewRequest(http.MethodPost, frontend.URL+"/"+chatCompletionsPath, bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("StartCursorProxy response code = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, respBody)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("backend body is not JSON: %v; body = %s", err, gotBody)
	}
	if got["model"] != "provider-model" {
		t.Fatalf("backend model = %v, want %q; body = %s", got["model"], "provider-model", gotBody)
	}
}

func TestStartCursorProxyLogsDoNotExposeMockPrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const chatCompletionsPath = "v1/chat/completions"

	tests := []struct {
		name      string
		prompt    string
		body      []byte
		forbidden []string
	}{
		{
			name:   "cursor proxy rule",
			prompt: "Test prompt using private context",
			body:   []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"Test prompt using private context"}]}`),
			forbidden: []string{
				"Test prompt using private context",
				"private context",
			},
		},
		{
			name:   "built in mock rule",
			prompt: "Testing. Just say hi and nothing else.",
			body: []byte(`{"model":"gpt-4o","messages":[{"role":"system","content":"system private instruction"},` +
				`{"role":"user","content":"Testing. Just say hi and nothing else."}]}`),
			forbidden: []string{
				"system private instruction",
				"Testing. Just say hi and nothing else.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(t, func() {
				handler, err := StartCursorProxy(Config{
					TargetURL: "http://example.test",
				}, nil)
				if err != nil {
					t.Fatalf("StartCursorProxy returned error: %v", err)
				}

				router := gin.New()
				router.POST("/"+chatCompletionsPath, handler)

				resp := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/"+chatCompletionsPath, bytes.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(resp, req)

				if resp.Code != http.StatusOK {
					t.Fatalf("StartCursorProxy response code = %d, want %d; body = %s", resp.Code, http.StatusOK, resp.Body.String())
				}
			})

			for _, forbidden := range tt.forbidden {
				if strings.Contains(output, forbidden) {
					t.Fatalf("captured logs leaked %q:\n%s", forbidden, output)
				}
			}
		})
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	done := make(chan string, 1)
	go func() {
		var output bytes.Buffer
		_, _ = io.Copy(&output, reader)
		done <- output.String()
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	output := <-done
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return output
}
