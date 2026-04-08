package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryBannedList_Success(t *testing.T) {
	expected := []*BannedHostInfo{
		{ID: 1, Type: "vmess", IP: "1.2.3.4", Port: 443},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/internal/QueryBannedList" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		resp := RepQueryBannedList{Data: expected}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(&Config{APIHost: server.URL, Token: "test"})
	result, err := client.QueryBannedList([]string{"tag1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].IP != "1.2.3.4" {
		t.Errorf("expected IP 1.2.3.4, got %s", result[0].IP)
	}
}

func TestQueryBannedList_HTTP400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := New(&Config{APIHost: server.URL, Token: "test"})
	_, err := client.QueryBannedList([]string{"tag1"})
	if err == nil {
		t.Fatal("expected error for HTTP 400, got nil")
	}
}

func TestQueryBannedList_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := New(&Config{APIHost: server.URL, Token: "test"})
	_, err := client.QueryBannedList([]string{"tag1"})
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
}

func TestTestPing_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RepTestPing{Data: true}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(&Config{APIHost: server.URL, Token: "test"})
	result, err := client.TestPing("1.2.3.4", 443)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bool(result) {
		t.Error("expected ping result true, got false")
	}
}

func TestTestPing_HTTP400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	}))
	defer server.Close()

	client := New(&Config{APIHost: server.URL, Token: "test"})
	_, err := client.TestPing("1.2.3.4", 443)
	if err == nil {
		t.Fatal("expected error for HTTP 400, got nil")
	}
}

func TestTestPing_NetworkError(t *testing.T) {
	client := New(&Config{APIHost: "http://127.0.0.1:1", Token: "test", Timeout: 1})
	_, err := client.TestPing("1.2.3.4", 443)
	if err == nil {
		t.Fatal("expected error for network failure, got nil")
	}
}

func TestChangeIP_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RepChangIpResult{Data: true}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(&Config{APIHost: server.URL, Token: "test"})
	err := client.ChangeIP("vmess", 1, "1.2.3.4", "5.6.7.8")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChangeIP_HTTP400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	}))
	defer server.Close()

	client := New(&Config{APIHost: server.URL, Token: "test"})
	err := client.ChangeIP("vmess", 1, "1.2.3.4", "5.6.7.8")
	if err == nil {
		t.Fatal("expected error for HTTP 400, got nil")
	}
}

func TestChangeIP_NetworkError(t *testing.T) {
	client := New(&Config{APIHost: "http://127.0.0.1:1", Token: "test", Timeout: 1})
	err := client.ChangeIP("vmess", 1, "1.2.3.4", "5.6.7.8")
	if err == nil {
		t.Fatal("expected error for network failure, got nil")
	}
}
