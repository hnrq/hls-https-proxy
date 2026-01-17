package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
)

func TestRewriteManifest(t *testing.T) {
	// Mock response
	manifest := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1280000\nchunk.m3u8"

	// Create context with dynamic values (mimicking what Director does)
	ctx := context.WithValue(context.Background(), proxyHostKey, "proxy.com")
	ctx = context.WithValue(ctx, proxySchemeKey, "https")

	resp := &http.Response{
		Request: (&http.Request{
			Method: "GET",
			URL:    &url.URL{Scheme: "http", Host: "target.com", Path: "/playlist.m3u8"},
			Host:   "target.com",
			Header: make(http.Header),
		}).WithContext(ctx),
		Body:          io.NopCloser(strings.NewReader(manifest)),
		Header:        make(http.Header),
		ContentLength: int64(len(manifest)),
	}
	resp.Header.Set("Content-Type", "application/vnd.apple.mpegurl")

	// Call with keys (keys must match the type defined in main, but since they are private in main,
	// we essentially rely on the fact that rewriteManifest in main uses the keys passed to it.
	// However, since we can't import private types from main, we will pass the keys defined here
	// assuming rewriteManifest uses the interface{} arguments as keys.)
	err := rewriteManifest(resp, proxySchemeKey, proxyHostKey)
	if err != nil {
		t.Fatalf("rewriteManifest failed: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	rewritten := string(body)

	t.Logf("Rewritten manifest:\n%s", rewritten)

	expectedBase := "https://proxy.com/?url="
	if !strings.Contains(rewritten, expectedBase) {
		t.Errorf("Expected proxy base %q not found in rewritten manifest.\nGot: %s", expectedBase, rewritten)
	}
}

type mockTransport struct{}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("OK"))}, nil
}

func TestHandleRequest_Headers(t *testing.T) {
	// Setup globals
	allowedOrigins = map[string]bool{"http://test.com": true}
	proxy = &httputil.ReverseProxy{
		Director:  func(req *http.Request) {},
		Transport: &mockTransport{},
	}

	tests := []struct {
		name           string
		headers        map[string]string
		query          string
		method         string
		expectedStatus int
	}{
		{
			name:           "Missing Header",
			headers:        map[string]string{"Origin": "http://test.com"},
			query:          "?url=http://target.com",
			method:         "GET",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Wrong Header Value",
			headers:        map[string]string{"Origin": "http://test.com", "X-Terms-Accepted": "false"},
			query:          "?url=http://target.com",
			method:         "GET",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Correct Header",
			headers:        map[string]string{"Origin": "http://test.com", "X-Terms-Accepted": "true"},
			query:          "?url=http://target.com",
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Options Request Skip Check",
			headers:        map[string]string{"Origin": "http://test.com"},
			query:          "?url=http://target.com",
			method:         "OPTIONS",
			expectedStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://proxy.com/"+tt.query, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			handleRequest(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}
