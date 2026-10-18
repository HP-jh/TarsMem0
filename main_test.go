package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestHealthHandler(t *testing.T) {
	client := NewClient("", "")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	withClient(client, healthHandler)(w, req)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil { t.Fatal(err) }
	if resp["status"] != "ok" { t.Fatalf("expected ok, got %v", resp["status"]) }
}

func TestToolsHandler(t *testing.T) {
	client := NewClient("", "")
	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	w := httptest.NewRecorder()
	withClient(client, toolsHandler)(w, req)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil { t.Fatal(err) }
	tools, ok := resp["tools"].([]interface{})
	if !ok || len(tools) == 0 { t.Fatal("expected tools") }
}

func TestInvokeHandler_MethodNotAllowed(t *testing.T) {
	client := NewClient("", "")
	req := httptest.NewRequest(http.MethodGet, "/invoke", nil)
	w := httptest.NewRecorder()
	withClient(client, invokeHandler)(w, req)
	if w.Code != http.StatusMethodNotAllowed { t.Fatalf("expected 405, got %d", w.Code) }
}

func TestInvokeHandler_UnknownTool(t *testing.T) {
	client := NewClient("", "")
	body, _ := json.Marshal(map[string]interface{}{ "tool": "xxx_unknown", "params": map[string]interface{}{} })
	req := httptest.NewRequest(http.MethodPost, "/invoke", bytes.NewReader(body))
	w := httptest.NewRecorder()
	withClient(client, invokeHandler)(w, req)
	if w.Code != http.StatusBadRequest { t.Fatalf("expected 400, got %d", w.Code) }
}

func TestClientReq_MockServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{ "status": "ok" })
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := NewClient(srv.URL, "test-key")
	res, err := client.req(context.Background(), "GET", "/health", nil)
	if err != nil { t.Fatalf("unexpected err: %v", err) }
	if res["status"] != "ok" { t.Fatalf("expected ok, got %v", res["status"]) }
}

func TestVersionHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	versionHandler(w, req)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil { t.Fatal(err) }
	if resp["name"] != RepoName { t.Fatalf("expected %s, got %v", RepoName, resp["name"]) }
}

func TestMetricsHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	metricsHandler(w, req)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil { t.Fatal(err) }
	if resp["memory"] == nil { t.Fatal("expected memory metrics") }
}

func TestValidateConfig(t *testing.T) {
	if err := validateConfig(map[string]string{"endpoint": ""}); err == nil {
		t.Fatal("expected error for empty endpoint")
	}
	if err := validateConfig(map[string]string{"endpoint": "ftp://bad"}); err == nil {
		t.Fatal("expected error for bad scheme")
	}
	if err := validateConfig(map[string]string{"endpoint": "https://api.example.com"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTimeout(t *testing.T) {
	os.Setenv("TEST_TIMEOUT", "5s")
	defer os.Unsetenv("TEST_TIMEOUT")
	if d := parseTimeout("TEST_TIMEOUT", 10*time.Second); d != 5*time.Second {
		t.Fatalf("expected 5s, got %v", d)
	}
	if d := parseTimeout("TEST_TIMEOUT_MISSING", 10*time.Second); d != 10*time.Second {
		t.Fatalf("expected 10s, got %v", d)
	}
}

func TestParseRetry(t *testing.T) {
	os.Setenv("TARS_MCP_MAX_RETRY", "5")
	defer os.Unsetenv("TARS_MCP_MAX_RETRY")
	if n := parseRetry(); n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}
}

func TestRetryBackoff(t *testing.T) {
	mux := http.NewServeMux()
	attempts := 0
	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := NewClient(srv.URL, "")
	client.MaxRetry = 2
	_, _ = client.req(context.Background(), "GET", "/fail", nil)
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestInvokeHandler_mem0_get(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/memories/abc123", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"id": "abc123", "data": "test"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := NewClient(srv.URL, "")
	body, _ := json.Marshal(map[string]interface{}{"tool": "mem0_get", "params": map[string]interface{}{"memory_id": "abc123"}})
	req := httptest.NewRequest(http.MethodPost, "/invoke", bytes.NewReader(body))
	w := httptest.NewRecorder()
	withClient(client, invokeHandler)(w, req)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
}

func TestInvokeHandler_mem0_delete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/memories/del456", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := NewClient(srv.URL, "")
	body, _ := json.Marshal(map[string]interface{}{"tool": "mem0_delete", "params": map[string]interface{}{"memory_id": "del456"}})
	req := httptest.NewRequest(http.MethodPost, "/invoke", bytes.NewReader(body))
	w := httptest.NewRecorder()
	withClient(client, invokeHandler)(w, req)
	if w.Code != http.StatusOK { t.Fatalf("expected 200, got %d", w.Code) }
}

func TestHandleStdioTool_mem0_get(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/memories/sid789", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"id": "sid789"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := NewClient(srv.URL, "")
	res := handleStdioTool(context.Background(), client, "mem0_get", map[string]interface{}{"memory_id": "sid789"})
	content, ok := res["content"].([]map[string]string)
	if !ok || len(content) == 0 { t.Fatal("expected content") }
}

func TestHandleStdioTool_mem0_delete(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/memories/sdel99", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := NewClient(srv.URL, "")
	res := handleStdioTool(context.Background(), client, "mem0_delete", map[string]interface{}{"memory_id": "sdel99"})
	content, ok := res["content"].([]map[string]string)
	if !ok || len(content) == 0 { t.Fatal("expected content") }
}
