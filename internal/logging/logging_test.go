package logging

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureWritesJSONLogToXDGStatePath(t *testing.T) {
	stateHome := t.TempDir()

	configured, err := Configure(Options{
		StateHome: stateHome,
		Level:     "debug",
	})
	if err != nil {
		t.Fatalf("Configure returned error: %v", err)
	}
	defer func() {
		if err := configured.Close(); err != nil {
			t.Fatalf("deferred close log: %v", err)
		}
	}()

	configured.Logger.DebugContext(context.Background(), "trace enabled", slog.String("method", "GET"), slog.Int("status", 304))
	if err := configured.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}

	wantPath := filepath.Join(stateHome, "gh-hound", "gh-hound.log")
	if configured.Path != wantPath {
		t.Fatalf("log path = %q, want %q", configured.Path, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("log is not JSON: %v\n%s", err, string(data))
	}
	if decoded["msg"] != "trace enabled" || decoded["method"] != "GET" || decoded["status"].(float64) != 304 {
		t.Fatalf("unexpected log payload: %#v", decoded)
	}
}
