package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("# comment\nFOO=bar\nQUOTED=\"baz\"\n\nEMPTY=\nALREADY_SET=should-not-override\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ALREADY_SET", "from-shell")
	for _, k := range []string{"FOO", "QUOTED", "EMPTY"} {
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}

	if got := os.Getenv("FOO"); got != "bar" {
		t.Errorf("FOO = %q, want %q", got, "bar")
	}
	if got := os.Getenv("QUOTED"); got != "baz" {
		t.Errorf("QUOTED = %q, want %q (quotes stripped)", got, "baz")
	}
	if got := os.Getenv("ALREADY_SET"); got != "from-shell" {
		t.Errorf("ALREADY_SET = %q, want shell value to win, got overridden", got)
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	if err := LoadEnvFile(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Errorf("missing .env should not error, got %v", err)
	}
}
