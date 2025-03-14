package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
