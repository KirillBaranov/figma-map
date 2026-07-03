package figma

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBridge_ListIssues(t *testing.T) {
	want := []Issue{{
		ID:               "abc",
		TabURL:           "https://example.com",
		Selector:         "#hero",
		ScreenshotBase64: "Zm9v",
		Bounds:           IssueBBox{X: 1, Y: 2, Width: 3, Height: 4},
		CreatedAt:        "2026-01-01T00:00:00Z",
	}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/issues" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("fileKey"); got != "file1" {
			t.Fatalf("fileKey query = %q, want file1", got)
		}
		data, _ := json.Marshal(want)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":` + string(data) + `}`))
	}))
	defer srv.Close()

	b := NewBridge(srv.URL)
	got, err := b.ListIssues(context.Background(), "file1")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(got) != 1 || got[0].ID != "abc" || got[0].Selector != "#hero" {
		t.Fatalf("ListIssues = %+v, want %+v", got, want)
	}
}

func TestBridge_AckIssue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/issues/abc" || r.Method != http.MethodDelete {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"abc"}}`))
	}))
	defer srv.Close()

	b := NewBridge(srv.URL)
	if err := b.AckIssue(context.Background(), "abc"); err != nil {
		t.Fatalf("AckIssue: %v", err)
	}
}

func TestBridge_AckIssue_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	b := NewBridge(srv.URL)
	if err := b.AckIssue(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing issue, got nil")
	}
}
