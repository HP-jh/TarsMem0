package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var Version = "1.10.0"
const RepoName = "TarsMem0"

var (
	requestCount int64
	errorCount   int64
)

type Client struct {
	Endpoint string
	APIKey   string
	Client   *http.Client
	MaxRetry int
}

func parseTimeout(envKey string, fallback time.Duration) time.Duration {
	if v := os.Getenv(envKey); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func parseRetry() int {
	if v := os.Getenv("TARS_MCP_MAX_RETRY"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n >= 0 {
			return n
		}
	}
	return 3
}

func NewClient(endpoint, apiKey string) *Client {
	if endpoint == "" {
		endpoint = "https://api.example.com"
	}
	timeout := parseTimeout("TARS_MCP_TIMEOUT", 30*time.Second)
	return &Client{
		Endpoint: strings.TrimRight(endpoint, "/"),
		APIKey:   apiKey,
		Client:   &http.Client{Timeout: timeout},
		MaxRetry: parseRetry(),
	}
}

func (c *Client) req(ctx context.Context, method, path string, body interface{}) (map[string]interface{}, error) {
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetry; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
			if backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		result, err := c.doReq(ctx, method, path, body)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func (c *Client) doReq(ctx context.Context, method, path string, body interface{}) (map[string]interface{}, error) {
	var bodyR io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyR = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Endpoint+path, bodyR)
	if err != nil { return nil, err }
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("User-Agent", RepoName+"/"+Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return map[string]interface{}{"raw": "non-JSON response"}, nil
	}
	return result, nil
}

func loadConfig() map[string]string {
	return map[string]string{
		"endpoint": os.Getenv("TARS_MCP_ENDPOINT"),
		"api_key":  os.Getenv("TARS_MCP_API_KEY"),
	}
}

func validateConfig(cfg map[string]string) error {
	endpoint := cfg["endpoint"]
	if endpoint == "" {
		return fmt.Errorf("TARS_MCP_ENDPOINT 还没设置")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return fmt.Errorf("TARS_MCP_ENDPOINT 必须以 http:// 或 https:// 开头")
	}
	return nil
}

func main() {
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *version {
		fmt.Printf("%s v%s\n", RepoName, Version)
		return
	}

	cfg := loadConfig()
	if err := validateConfig(cfg); err != nil {
		log.Fatalf("[FATAL] 配置检查没通过: %v", err)
	}
	client := NewClient(cfg["endpoint"], cfg["api_key"])
	log.Printf("[%s] starting v%s endpoint=%s", RepoName, Version, cfg["endpoint"])
	mode := os.Getenv("TARS_MCP_MODE")
	if mode == "" { mode = "stdio" }
	srv := &http.Server{Addr: ":8080", ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { withClient(client, healthHandler)(w, r) })
	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) { versionHandler(w, r) })
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) { metricsHandler(w, r) })
	http.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) { withClient(client, toolsHandler)(w, r) })
	http.HandleFunc("/invoke", func(w http.ResponseWriter, r *http.Request) { withClient(client, invokeHandler)(w, r) })
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("[%s] shutting down", RepoName)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if mode == "stdio" {
		runStdioServer(client)
		return
	}
	log.Printf("[%s] HTTP listening on %s", RepoName, srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func withClient(client *Client, fn func(http.ResponseWriter, *http.Request, *Client)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions { w.WriteHeader(http.StatusNoContent); return }
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		start := time.Now()
		fn(w, r, client)
		log.Printf("[%s] %s %s %s %s", RepoName, r.Method, r.URL.Path, time.Since(start).String(), r.UserAgent())
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request, client *Client) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	up := map[string]interface{}{"healthy": false}
	if _, err := client.req(ctx, "GET", "/health", nil); err == nil { up["healthy"] = true }
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok", "version": Version, "name": RepoName, "upstream": up,
		"time": time.Now().UTC().Format(time.RFC3339),
	})
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"name": RepoName, "version": Version, "go": runtime.Version(),
	})
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"requests": requestCount, "errors": errorCount,
		"memory": map[string]interface{}{
			"alloc":        m.Alloc,
			"sys":          m.Sys,
			"goroutines":   runtime.NumGoroutine(),
		},
	})
}

func toolsHandler(w http.ResponseWriter, r *http.Request, client *Client) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"tools": []map[string]interface{}{
		{
			"name":        "mem0_search",
			"description": "Search memories by query",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"query": map[string]interface{}{"type": "string", "description": "Parameter query"}},
				"required":   []interface{}{"query"},
			},
		},
		{
			"name":        "mem0_add",
			"description": "Add a new memory entry",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"messages": map[string]interface{}{"type": "array", "description": "Parameter messages"}, "user_id": map[string]interface{}{"type": "string", "description": "Parameter user_id"}},
				"required":   []interface{}{"messages", "user_id"},
			},
		},
		{
			"name":        "mem0_get",
			"description": "Get a memory by ID",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"memory_id": map[string]interface{}{"type": "string", "description": "Memory ID"}},
				"required":   []interface{}{"memory_id"},
			},
		},
		{
			"name":        "mem0_delete",
			"description": "Delete a memory by ID",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"memory_id": map[string]interface{}{"type": "string", "description": "Memory ID"}},
				"required":   []interface{}{"memory_id"},
			},
		},
		},
	})
}

func invokeHandler(w http.ResponseWriter, r *http.Request, client *Client) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "这边只支持 POST 请求哦"})
		return
	}
	var req struct { Tool string `json:"tool"`; Params map[string]interface{} `json:"params"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("没看懂请求内容：%v", err)})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	switch req.Tool {
	case "mem0_search":
		if req.Params["query"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "需要参数：query"}); return }
		result, err := client.req(ctx, "GET", "/v1/memories/?search="+fmt.Sprintf("%v", req.Params["query"]), nil)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("请求没成功：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	case "mem0_add":
		if req.Params["messages"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "需要参数：messages"}); return }
		if req.Params["user_id"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "需要参数：user_id"}); return }
		body := map[string]interface{}{"messages": req.Params["messages"], "user_id": req.Params["user_id"]}
		result, err := client.req(ctx, "POST", "/v1/memories/", body)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("请求没成功：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	case "mem0_get":
		if req.Params["memory_id"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "需要参数：memory_id"}); return }
		result, err := client.req(ctx, "GET", "/v1/memories/"+fmt.Sprintf("%v", req.Params["memory_id"]), nil)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("请求没成功：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	case "mem0_delete":
		if req.Params["memory_id"] == nil { w.WriteHeader(http.StatusBadRequest); _ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "需要参数：memory_id"}); return }
		result, err := client.req(ctx, "DELETE", "/v1/memories/"+fmt.Sprintf("%v", req.Params["memory_id"]), nil)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("请求没成功：%v", err)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": result})
	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": fmt.Sprintf("不认识的工具：%s", req.Tool)})
	}
}

func runStdioServer(client *Client) {
	log.Printf("[%s] stdio mode", RepoName)
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for {
		var msg map[string]interface{}
		if err := dec.Decode(&msg); err != nil { return }
		id := msg["id"]
		method, _ := msg["method"].(string)
		var result map[string]interface{}
		switch method {
		case "tools/list":
			result = map[string]interface{}{"tools": []map[string]interface{}{{"name": "mem0_search", "description": "Search memories by query"}, {"name": "mem0_add", "description": "Add a new memory entry"}, {"name": "mem0_get", "description": "Get a memory by ID"}, {"name": "mem0_delete", "description": "Delete a memory by ID"}}}
		case "tools/call":
			params, _ := msg["params"].(map[string]interface{})
			toolName, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]interface{})
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			result = handleStdioTool(ctx, client, toolName, args)
			cancel()
		default:
			result = map[string]interface{}{"message": fmt.Sprintf("%s v%s ready", RepoName, Version)}
		}
		_ = enc.Encode(map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": result})
	}
}

func handleStdioTool(ctx context.Context, client *Client, toolName string, args map[string]interface{}) map[string]interface{} {
	switch toolName {
	case "mem0_search":
		if args["query"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "需要参数：query"}}} }
		result, err := client.req(ctx, "GET", "/v1/memories/?search="+fmt.Sprintf("%v", args["query"]), nil)
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("请求没成功：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "mem0_add":
		if args["messages"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "需要参数：messages"}}} }
		if args["user_id"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "需要参数：user_id"}}} }
		result, err := client.req(ctx, "POST", "/v1/memories/", map[string]interface{}{"messages": args["messages"], "user_id": args["user_id"]})
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("请求没成功：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "mem0_get":
		if args["memory_id"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "需要参数：memory_id"}}} }
		result, err := client.req(ctx, "GET", "/v1/memories/"+fmt.Sprintf("%v", args["memory_id"]), nil)
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("请求没成功：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	case "mem0_delete":
		if args["memory_id"] == nil { return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "需要参数：memory_id"}}} }
		result, err := client.req(ctx, "DELETE", "/v1/memories/"+fmt.Sprintf("%v", args["memory_id"]), nil)
		if err != nil {
			return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("请求没成功：%v", err)}}}
		}
		data, _ := json.Marshal(result)
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	default:
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": fmt.Sprintf("不认识的工具：%s", toolName)}}}
	}
}
