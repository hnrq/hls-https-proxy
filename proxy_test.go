package main

import (
	"context"
	"io"
	"net/http"
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
