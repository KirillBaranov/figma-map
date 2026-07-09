package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPingBridge(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	if err := pingBridge(context.Background(), ok.URL); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if err := pingBridge(context.Background(), bad.URL); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}

	if err := pingBridge(context.Background(), "http://127.0.0.1:1"); err == nil {
		t.Fatal("expected an error for an unreachable address")
	}
}

// TestPIDFileRoundTrip exercises write/read/remove against the real
// ~/.figma-map/bridge.pid path (there's no injectable state dir), so it
// saves and restores whatever was there before — a live `bridge up` may
// have a real pidfile in place when this runs.
func TestPIDFileRoundTrip(t *testing.T) {
	path, err := pidFilePath()
	if err != nil {
		t.Fatal(err)
	}
	original, readErr := os.ReadFile(path)
	t.Cleanup(func() {
		if readErr == nil {
			_ = os.WriteFile(path, original, 0o644)
		} else {
			_ = removePIDFile()
		}
	})

	if err := removePIDFile(); err != nil {
		t.Fatalf("remove (pre-clean): %v", err)
	}
	if _, err := readPID(); err == nil {
		t.Fatal("expected an error reading a missing pidfile")
	}

	if err := writePID(12345); err != nil {
		t.Fatal(err)
	}
	got, err := readPID()
	if err != nil {
		t.Fatal(err)
	}
	if got != 12345 {
		t.Fatalf("got pid %d, want 12345", got)
	}

	if err := removePIDFile(); err != nil {
		t.Fatal(err)
	}
	if err := removePIDFile(); err != nil {
		t.Fatalf("removing an already-missing pidfile should be a no-op, got %v", err)
	}
}
