package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	logger            *zap.Logger
	allowedOrigins    map[string]bool
	proxy             *httputil.ReverseProxy
	connectionLimiter = make(chan struct{}, 5)
)

type contextKey string

const (
	proxyHostKey   contextKey = "proxyHost"
	proxySchemeKey contextKey = "proxyScheme"
)

func initLogger() {
	logPath := os.Getenv("LOG_PATH")
	if logPath == "" {
		logPath = "/app/logs/proxy.log"
	}

	w := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	})

	core := zapcore.NewTee(
		zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			w,
			zap.InfoLevel,
		),
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
			zapcore.AddSync(os.Stdout),
			zap.InfoLevel,
		),
	)

	logger = zap.New(core)
}

func main() {
	initLogger()
	defer logger.Sync()

	originsEnv := os.Getenv("ALLOWED_ORIGINS")
	if originsEnv == "" {
		originsEnv = "http://localhost:5173"
	}

	allowedList := strings.Split(originsEnv, ",")
	allowedOrigins = make(map[string]bool)
	for _, origin := range allowedList {
		allowedOrigins[strings.TrimSpace(origin)] = true
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}

	httpClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	proxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			targetParam := req.URL.Query().Get("url")
			target, err := url.Parse(targetParam)
			if err != nil || target.Scheme == "" {
				logger.Warn("Malformed target URL", zap.String("url", targetParam))
				return
			}

			// Capture original host and scheme
			originalHost := req.Host
			originalScheme := "http"
			if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
				originalScheme = "https"
			}

			// Inject into context
			ctx := context.WithValue(req.Context(), proxyHostKey, originalHost)
			ctx = context.WithValue(ctx, proxySchemeKey, originalScheme)
			*req = *req.WithContext(ctx)

			req.URL = target
			req.Host = target.Host
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			if resp.StatusCode >= 300 && resp.StatusCode <= 308 {
				location := resp.Header.Get("Location")
				if location != "" {
					targetURL, _ := url.Parse(location)
					originalURL := resp.Request.URL
					resolvedURL := originalURL.ResolveReference(targetURL)

					req, _ := http.NewRequestWithContext(resp.Request.Context(), "GET", resolvedURL.String(), nil)
					newResp, err := httpClient.Do(req)
					if err == nil {
						*resp = *newResp
					}
				}
			}

			contentType := resp.Header.Get("Content-Type")
			resp.Header.Del("Access-Control-Allow-Origin") // Remove CORS header to avoid conflicts

			if strings.Contains(contentType, "mpegurl") || strings.Contains(contentType, "x-mpegurl") {
				return rewriteManifest(resp, proxySchemeKey, proxyHostKey)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("Proxy error", zap.Error(err), zap.String("url", r.URL.Query().Get("url")))
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	http.HandleFunc("/", handleRequest)

	port := os.Getenv("PROXY_PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("Server running", zap.String("port", port))
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Fatal("Fatal error", zap.Error(err))
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if allowedOrigins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range, X-Terms-Accepted")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	} else if origin != "" {
		logger.Warn("Unauthorized origin blocked", zap.String("origin", origin))
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Header.Get("X-Terms-Accepted") != "true" {
		http.Error(w, "Terms of Service not accepted", http.StatusForbidden)
		return
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Missing url", http.StatusBadRequest)
		return
	}

	connectionLimiter <- struct{}{}
	defer func() { <-connectionLimiter }()

	proxy.ServeHTTP(w, r)
}

func rewriteManifest(resp *http.Response, schemeKey, hostKey interface{}) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()

	baseLoc := resp.Request.URL

	scheme, _ := resp.Request.Context().Value(schemeKey).(string)
	if scheme == "" {
		scheme = "http"
	}

	host, _ := resp.Request.Context().Value(hostKey).(string)
	if host == "" {
		host = resp.Request.Host
	}

	proxyBase := fmt.Sprintf("%s://%s/?url=", scheme, host)

	var buffer bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			lineURL, err := url.Parse(line)
			if err == nil {
				resolvedURL := baseLoc.ResolveReference(lineURL)
				line = proxyBase + url.QueryEscape(resolvedURL.String())
			}
		}
		buffer.WriteString(line + "\n")
	}

	newBody := buffer.Bytes()
	resp.Body = io.NopCloser(bytes.NewReader(newBody))
	resp.ContentLength = int64(len(newBody))
	resp.Header.Set("Content-Length", fmt.Sprint(len(newBody)))

	return nil
}
