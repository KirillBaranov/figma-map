package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsProjectRoot(t *testing.T) {
	dir := t.TempDir()
	if IsProjectRoot(dir) {
		t.Fatal("empty dir should not be a project root")
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsProjectRoot(dir) {
		t.Fatal("dir with .git should be a project root")
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()

	// root/proj-a is a project (has go.mod); root/plain is not;
	// root/proj-a/nested-proj (also has go.mod) should NOT be found,
	// since Discover stops descending once it finds a project root.
	mustMkdirAll(t, filepath.Join(root, "proj-a"))
	mustTouch(t, filepath.Join(root, "proj-a", "go.mod"))
	mustMkdirAll(t, filepath.Join(root, "proj-a", "nested-proj"))
	mustTouch(t, filepath.Join(root, "proj-a", "nested-proj", "go.mod"))
	mustMkdirAll(t, filepath.Join(root, "plain"))
	mustMkdirAll(t, filepath.Join(root, ".hidden"))
	mustTouch(t, filepath.Join(root, ".hidden", "go.mod"))

	found := Discover([]string{root}, 3)

	if len(found) != 1 || found[0] != filepath.Join(root, "proj-a") {
		t.Fatalf("expected exactly [proj-a], got %v", found)
	}
}

func TestDiscoverDepthLimit(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	mustMkdirAll(t, deep)
	mustTouch(t, filepath.Join(deep, "go.mod"))

	if found := Discover([]string{root}, 1); len(found) != 0 {
		t.Fatalf("expected nothing found within depth 1, got %v", found)
	}
	if found := Discover([]string{root}, 3); len(found) != 1 {
		t.Fatalf("expected 1 match within depth 3, got %v", found)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustTouch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
