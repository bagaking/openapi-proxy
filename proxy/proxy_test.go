package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
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

	prefix := "/" + "openai"
	router := newProxyTestRouter(Config{
		TargetURL:  backend.URL,
		PathPrefix: prefix,
	})

	pathPrefix := "/"
	tests := []struct {
		path string
	}{
		{path: pathPrefix + "v1/models"},
		{path: pathPrefix + "openaiish/v1/models"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusNotFound {
				t.Fatalf("GET %s status = %d, want %d; body = %s", tt.path, resp.Code, http.StatusNotFound, resp.Body.String())
			}
		})
	}
	if calledBackend {
		t.Fatal("path outside prefix called backend, want prefix boundary rejection")
	}
}

func TestHandleRequestAcceptsPathPrefixBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	calledBackend := false
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledBackend = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(backend.Close)

	prefix := "/" + "openai"
	router := newProxyTestRouter(Config{
		TargetURL:  backend.URL,
		PathPrefix: prefix,
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
	modelsPath := prefix + "/" + "v1/models"
	req := httptest.NewRequest(http.MethodGet, modelsPath, nil)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d; body = %s", modelsPath, resp.Code, http.StatusOK, resp.Body.String())
	}
	if calledBackend {
		t.Fatalf("GET %s called backend, want local models response after prefix trim", modelsPath)
	}

	frontend := httptest.NewServer(router)
	t.Cleanup(frontend.Close)

	exactResp, err := http.Get(frontend.URL + prefix)
	if err != nil {
		t.Fatalf("GET %s returned error: %v", prefix, err)
	}
	t.Cleanup(func() { exactResp.Body.Close() })

	if exactResp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET %s status = %d, want accepted exact prefix boundary", prefix, exactResp.StatusCode)
	}
	if !calledBackend {
		t.Fatalf("GET %s did not call backend, want exact prefix accepted and proxied", prefix)
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

func TestHandleRequestRewritePreservesTrailingSlash(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pathSep := "/"
	targetBasePath := pathSep + "api" + pathSep + "v3" + pathSep
	requestPath := pathSep + "v1" + pathSep + "files" + pathSep
	wantPath := pathSep + "api" + pathSep + "v3" + pathSep + "files" + pathSep

	paths := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(backend.Close)

	router := newProxyTestRouter(Config{
		TargetURL: backend.URL + targetBasePath,
		Headers: map[string]string{
			"Authorization": "Bearer configured-value",
		},
	})
	frontend := httptest.NewServer(router)
	t.Cleanup(frontend.Close)

	resp, err := http.Post(frontend.URL+requestPath, "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST %s returned error: %v", requestPath, err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s status = %d, want %d; body = %s", requestPath, resp.StatusCode, http.StatusNoContent, respBody)
	}
	got := <-paths
	if got != wantPath {
		t.Errorf("backend path = %q, want %q", got, wantPath)
	}
}

func TestHandleRequestRunsAfterResponsePlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend":true}`))
	}))
	t.Cleanup(backend.Close)

	plugin := &rewriteResponsePlugin{
		body:        []byte(`{"plugin":true}`),
		contentType: "application/json",
	}
	proxy := NewProxy(Config{TargetURL: backend.URL})
	proxy.RegisterPlugin(plugin)

	pathSep := "/"
	chatPath := pathSep + "v1" + pathSep + "chat" + pathSep + "completions"
	router := gin.New()
	router.Any(pathSep+"*path", proxy.handleRequest)
	frontend := httptest.NewServer(router)
	t.Cleanup(frontend.Close)

	resp, err := http.Post(frontend.URL+chatPath, "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("POST %s returned error: %v", chatPath, err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, respBody)
	}
	gotBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if !plugin.called.Load() {
		t.Fatal("AfterResponse plugin was not called")
	}
	if !bytes.Equal(gotBody, plugin.body) {
		t.Fatalf("response body = %s, want %s", gotBody, plugin.body)
	}
	if got := resp.Header.Get("Content-Type"); got != plugin.contentType {
		t.Fatalf("Content-Type = %q, want %q", got, plugin.contentType)
	}
}

func TestJoinProxyPathBoundaries(t *testing.T) {
	apiBase := strings.Join([]string{"", "api", "v3"}, "/")
	filesPath := strings.Join([]string{"", "files", ""}, "/")
	nestedFilesPath := strings.Join([]string{"", "", "", "files", "", "x", ""}, "/")

	tests := []struct {
		name        string
		basePath    string
		requestPath string
		want        string
	}{
		{
			name:        "empty request keeps base",
			basePath:    apiBase + "/",
			requestPath: "",
			want:        apiBase + "/",
		},
		{
			name:        "empty base keeps request",
			basePath:    "",
			requestPath: filesPath,
			want:        filesPath,
		},
		{
			name:        "root base keeps request",
			basePath:    "/",
			requestPath: filesPath,
			want:        filesPath,
		},
		{
			name:        "trim boundary slashes only",
			basePath:    apiBase + "///",
			requestPath: nestedFilesPath,
			want:        apiBase + "/" + strings.TrimLeft(nestedFilesPath, "/"),
		},
		{
			name:        "bare root request joins under base",
			basePath:    apiBase,
			requestPath: "/",
			want:        apiBase + "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinProxyPath(tt.basePath, tt.requestPath)
			if got != tt.want {
				t.Errorf("joinProxyPath(%q, %q) = %q, want %q", tt.basePath, tt.requestPath, got, tt.want)
			}
		})
	}
}

type rewriteResponsePlugin struct {
	called      atomic.Bool
	body        []byte
	contentType string
}

func (p *rewriteResponsePlugin) BeforeRequest(req *http.Request) error {
	return nil
}

func (p *rewriteResponsePlugin) AfterResponse(resp *http.Response) error {
	p.called.Store(true)
	resp.Body = io.NopCloser(bytes.NewBuffer(p.body))
	resp.ContentLength = int64(len(p.body))
	resp.Header.Set("Content-Type", p.contentType)
	resp.Header.Set("Content-Length", strconv.Itoa(len(p.body)))
	return nil
}

func (p *rewriteResponsePlugin) Configure(config json.RawMessage) error {
	return nil
}

func newProxyTestRouter(cfg Config) *gin.Engine {
	router := gin.New()
	proxy := NewProxy(cfg)
	router.Any("/*path", proxy.handleRequest)
	return router
}
