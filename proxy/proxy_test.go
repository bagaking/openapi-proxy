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

func TestHandleRequestServesConfiguredModelsLocally(t *testing.T) {
	gin.SetMode(gin.TestMode)

	calledBackend := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledBackend = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(backend.Close)

	router := newProxyTestRouter(Config{
		TargetURL: backend.URL,
		Models: []ModelInfo{
			{
				ID:      "test-model",
				Object:  "model",
				Created: 123,
				OwnedBy: "test-owner",
			},
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET /v1/models status = %d, want %d; body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if calledBackend {
		t.Fatal("GET /v1/models called backend, want local response")
	}

	var got ModelsResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET /v1/models body is not JSON: %v; body = %s", err, resp.Body.String())
	}
	want := ModelsResponse{
		Object: "list",
		Data: []ModelInfo{
			{
				ID:      "test-model",
				Object:  "model",
				Created: 123,
				OwnedBy: "test-owner",
			},
		},
	}
	if got.Object != want.Object || len(got.Data) != len(want.Data) || got.Data[0] != want.Data[0] {
		t.Fatalf("GET /v1/models response = %+v, want %+v", got, want)
	}
}

func TestHandleRequestRejectsPathsOutsidePrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	calledBackend := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledBackend = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	router := newProxyTestRouter(Config{
		TargetURL:  backend.URL,
		PathPrefix: "/openai",
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("GET /v1/models status = %d, want %d; body = %s", resp.Code, http.StatusNotFound, resp.Body.String())
	}
	if calledBackend {
		t.Fatal("GET /v1/models called backend, want prefix boundary rejection")
	}
}

func TestHandleRequestRewritesOpenAIPathAndUsesConfiguredAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	type backendRequest struct {
		path          string
		authorization string
		origin        string
		referer       string
		body          []byte
	}
	requests := make(chan backendRequest, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("backend ReadAll error = %v, want nil", err)
		}
		requests <- backendRequest{
			path:          r.URL.Path,
			authorization: r.Header.Get("Authorization"),
			origin:        r.Header.Get("Origin"),
			referer:       r.Header.Get("Referer"),
			body:          body,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backend.Close)

	router := newProxyTestRouter(Config{
		TargetURL: backend.URL + "/api/v3",
		Headers: map[string]string{
			"Authorization": "Bearer configured-value",
		},
	})
	frontend := httptest.NewServer(router)
	t.Cleanup(frontend.Close)

	reqBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)
	req, err := http.NewRequest(http.MethodPost, frontend.URL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("Authorization", "Bearer")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://client.example")
	req.Header.Set("Referer", "https://client.example/app")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /v1/chat/completions status = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, respBody)
	}

	got := <-requests
	if got.path != "/api/v3/chat/completions" {
		t.Errorf("backend path = %q, want %q", got.path, "/api/v3/chat/completions")
	}
	if got.authorization != "Bearer configured-value" {
		t.Errorf("backend Authorization = %q, want %q", got.authorization, "Bearer configured-value")
	}
	if got.origin != "" {
		t.Errorf("backend Origin = %q, want empty", got.origin)
	}
	if got.referer != "" {
		t.Errorf("backend Referer = %q, want empty", got.referer)
	}
	if !bytes.Equal(got.body, reqBody) {
		t.Errorf("backend body = %s, want %s", got.body, reqBody)
	}
}

func newProxyTestRouter(cfg Config) *gin.Engine {
	router := gin.New()
	proxy := NewProxy(cfg)
	router.Any("/*path", proxy.handleRequest)
	return router
}
