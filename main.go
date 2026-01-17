package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

func main() {
	originsEnv := os.Getenv("ALLOWED_ORIGINS")
	if originsEnv == "" {
		originsEnv = "http://localhost:5173"
	}

	allowedList := strings.Split(originsEnv, ",")
	allowedOrigins := make(map[string]bool)
	for _, origin := range allowedList {
		allowedOrigins[strings.TrimSpace(origin)] = true
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			targetParam := req.URL.Query().Get("url")
			target, err := url.Parse(targetParam)
			if err != nil || target.Scheme == "" {
				return
			}
			req.URL = target
			req.Host = target.Host
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		},
		ModifyResponse: func(resp *http.Response) error {
			contentType := resp.Header.Get("Content-Type")
			if strings.Contains(contentType, "mpegurl") || strings.Contains(contentType, "x-mpegurl") {
				return rewriteManifest(resp)
			}
			return nil
		},
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
		} else if origin != "" {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.URL.Query().Get("url") == "" {
			http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
			return
		}

		proxy.ServeHTTP(w, r)
	})

	port := os.Getenv("PROXY_PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("HLS Proxy started on :%s\n", port)
	fmt.Printf("Allowed Origins: %s\n", originsEnv)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		panic(err)
	}
}

func rewriteManifest(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()

	origURL := resp.Request.URL
	baseDir := origURL.Scheme + "://" + origURL.Host + origURL.Path[:strings.LastIndex(origURL.Path, "/")+1]

	scheme := "https"
	if resp.Request.TLS == nil && resp.Request.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	proxyBase := fmt.Sprintf("%s://%s/?url=", scheme, resp.Request.Host)

	var buffer bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			fullURL := line
			if !strings.HasPrefix(line, "http") {
				fullURL = baseDir + line
			}
			line = proxyBase + url.QueryEscape(fullURL)
		}
		buffer.WriteString(line + "\n")
	}

	newBody := buffer.Bytes()
	resp.Body = io.NopCloser(bytes.NewReader(newBody))
	resp.ContentLength = int64(len(newBody))
	resp.Header.Set("Content-Length", fmt.Sprint(len(newBody)))

	return nil
}
