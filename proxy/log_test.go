package proxy

import (
	"net/http"
	"strings"
	"testing"
)

func TestRedactedHeaderMasksSensitiveValues(t *testing.T) {
	headers := http.Header{
		"Authorization":          []string{"Bearer configured-value"},
		"Content-Type":           []string{"application/json"},
		"Cookie":                 []string{"session=private"},
		"X-Api-Key":              []string{"private-api-key"},
		"X-Mock-Direct-Response": []string{`{"content":"private"}`},
	}

	got := redactedHeader(headers)

	if got.Get("Authorization") != "[REDACTED]" {
		t.Errorf("Authorization = %q, want redacted", got.Get("Authorization"))
	}
	if got.Get("Cookie") != "[REDACTED]" {
		t.Errorf("Cookie = %q, want redacted", got.Get("Cookie"))
	}
	if got.Get("X-Api-Key") != "[REDACTED]" {
		t.Errorf("X-Api-Key = %q, want redacted", got.Get("X-Api-Key"))
	}
	if got.Get("X-Mock-Direct-Response") != "[REDACTED]" {
		t.Errorf("X-Mock-Direct-Response = %q, want redacted", got.Get("X-Mock-Direct-Response"))
	}
	if got.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want preserved", got.Get("Content-Type"))
	}
	if headers.Get("Authorization") != "Bearer configured-value" {
		t.Errorf("source Authorization mutated to %q", headers.Get("Authorization"))
	}
}

func TestBodyLogSummaryDoesNotExposePayload(t *testing.T) {
	body := []byte(`{"password":"secret","prompt":"private prompt"}`)

	got := bodyLogSummary(body)

	if !strings.Contains(got, "bytes") {
		t.Fatalf("summary = %q, want byte count", got)
	}
	for _, leaked := range []string{"password", "secret", "private prompt"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("summary = %q, leaked %q", got, leaked)
		}
	}
}
