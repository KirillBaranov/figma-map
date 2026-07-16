package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMigrateMissingFile(t *testing.T) {
	applied, err := Migrate(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Migrate on missing file: %v", err)
	}
	if applied != nil {
		t.Fatalf("expected no migrations applied, got %v", applied)
	}
}

func TestMigrateWritesCurrentSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "figma-map.yaml")
	original := "bridge: http://localhost:1994\n# a comment worth keeping\nstorybook: http://localhost:6007\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := Migrate(path); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	cfg := Config{}
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatalf("parse migrated file: %v", err)
	}
	if cfg.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", cfg.SchemaVersion, CurrentSchemaVersion)
	}
	if !strings.Contains(string(out), "a comment worth keeping") {
		t.Fatal("migration must not drop existing comments")
	}
	if cfg.Bridge != "http://localhost:1994" || cfg.Storybook != "http://localhost:6007" {
		t.Fatalf("existing fields lost after migration: %+v", cfg)
	}

	// Re-running on an already-current file must be a no-op (no migrations
	// applied, since schemaVersion already matches).
	applied, err := Migrate(path)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if applied != nil {
		t.Fatalf("expected no-op on already-current file, got %v", applied)
	}
}
