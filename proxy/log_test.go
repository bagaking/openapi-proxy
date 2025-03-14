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
		"X-Mock-Response":        []string{`{"content":"private"}`},
	}
	headers.Set("Forwarded", "for=203.0.113.10;proto=https")
	headers.Set("X-Client-IP", "203.0.113.20")
	headers.Add("X-Forwarded-For", "203.0.113.30")
	headers.Add("X-Forwarded-For", "198.51.100.10")
	headers.Set("X-Real-IP", "203.0.113.40")

	got := redactedHeader(headers)

	for _, name := range []string{
		"Authorization",
		"Cookie",
		"Forwarded",
		"X-Api-Key",
		"X-Client-IP",
		"X-Forwarded-For",
		"X-Mock-Direct-Response",
		"X-Mock-Response",
		"X-Real-IP",
	} {
		t.Run(name, func(t *testing.T) {
			if got.Get(name) != "[REDACTED]" {
				t.Errorf("redactedHeader(%q) = %q, want %q", name, got.Get(name), "[REDACTED]")
			}
		})
	}
	if got.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want preserved", got.Get("Content-Type"))
	}
	if headers.Get("Authorization") != "Bearer configured-value" {
		t.Errorf("source Authorization mutated to %q", headers.Get("Authorization"))
	}
	if len(headers.Values("X-Forwarded-For")) != 2 {
		t.Errorf("source X-Forwarded-For value count = %d, want 2", len(headers.Values("X-Forwarded-For")))
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
