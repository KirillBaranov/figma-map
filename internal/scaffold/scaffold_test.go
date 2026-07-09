package scaffold

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		skillAssetPath:  {Data: []byte("skill v1\n")},
		configAssetPath: {Data: []byte("config v1\n")},
	}
}

func TestWriteSkill(t *testing.T) {
	assets := testAssets()

	t.Run("missing target writes", func(t *testing.T) {
		target := t.TempDir()
		status, err := WriteSkill(assets, target, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if status == "" {
			t.Fatal("expected non-empty status")
		}
		got, err := os.ReadFile(filepath.Join(target, skillTargetPath))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "skill v1\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("identical existing file is skipped", func(t *testing.T) {
		target := t.TempDir()
		dst := filepath.Join(target, skillTargetPath)
		mustMkdirAll(t, filepath.Dir(dst))
		mustWrite(t, dst, "skill v1\n")
		// Make the file read-only so a stray write (rather than a true skip)
		// fails loudly instead of silently succeeding with identical bytes.
		if err := os.Chmod(dst, 0o444); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(dst, 0o644); err != nil {
				t.Log(err)
			}
		})

		if _, err := WriteSkill(assets, target, false, false); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("differing file without force is skipped", func(t *testing.T) {
		target := t.TempDir()
		dst := filepath.Join(target, skillTargetPath)
		mustMkdirAll(t, filepath.Dir(dst))
		mustWrite(t, dst, "locally edited\n")

		if _, err := WriteSkill(assets, target, false, false); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(dst)
		if string(got) != "locally edited\n" {
			t.Fatalf("differing file should not be overwritten without --force, got %q", got)
		}
	})

	t.Run("differing file with force is overwritten", func(t *testing.T) {
		target := t.TempDir()
		dst := filepath.Join(target, skillTargetPath)
		mustMkdirAll(t, filepath.Dir(dst))
		mustWrite(t, dst, "locally edited\n")

		if _, err := WriteSkill(assets, target, true, false); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(dst)
		if string(got) != "skill v1\n" {
			t.Fatalf("expected overwrite with --force, got %q", got)
		}
	})

	t.Run("preview never writes", func(t *testing.T) {
		target := t.TempDir()
		dst := filepath.Join(target, skillTargetPath)
		if _, err := WriteSkill(assets, target, false, true); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Fatal("preview should not have written the file")
		}
	})
}

func TestWriteConfig(t *testing.T) {
	assets := testAssets()

	t.Run("missing target writes", func(t *testing.T) {
		target := t.TempDir()
		if _, err := WriteConfig(assets, target, false); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(target, configTargetPath))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "config v1\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("existing config is never touched", func(t *testing.T) {
		target := t.TempDir()
		dst := filepath.Join(target, configTargetPath)
		mustWrite(t, dst, "custom config\n")

		if _, err := WriteConfig(assets, target, false); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(dst)
		if string(got) != "custom config\n" {
			t.Fatalf("existing config must never be overwritten, got %q", got)
		}
	})
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
