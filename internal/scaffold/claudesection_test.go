package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteClaudeSection(t *testing.T) {
	t.Run("missing file is created with just the block", func(t *testing.T) {
		target := t.TempDir()
		if _, err := WriteClaudeSection(target, false); err != nil {
			t.Fatal(err)
		}
		got := readClaudeMD(t, target)
		if !strings.Contains(got, sectionStart) || !strings.Contains(got, sectionEnd) {
			t.Fatalf("missing markers in %q", got)
		}
	})

	t.Run("unrelated content is appended to, not replaced", func(t *testing.T) {
		target := t.TempDir()
		original := "# My Project\n\nSome instructions that have nothing to do with figma-map.\n"
		mustWrite(t, filepath.Join(target, claudeFile), original)

		if _, err := WriteClaudeSection(target, false); err != nil {
			t.Fatal(err)
		}
		got := readClaudeMD(t, target)
		if !strings.HasPrefix(got, original) {
			t.Fatalf("original content was not preserved verbatim at the top:\n%s", got)
		}
		if !strings.Contains(got, sectionStart) {
			t.Fatal("expected the section to be appended")
		}
	})

	t.Run("existing markers are updated in place, rest untouched", func(t *testing.T) {
		target := t.TempDir()
		original := "# My Project\n\n" +
			sectionStart + "\nold stale content\n" + sectionEnd +
			"\n\n## Unrelated later section\nkeep me\n"
		mustWrite(t, filepath.Join(target, claudeFile), original)

		if _, err := WriteClaudeSection(target, false); err != nil {
			t.Fatal(err)
		}
		got := readClaudeMD(t, target)
		if strings.Contains(got, "old stale content") {
			t.Fatal("stale section content should have been replaced")
		}
		if !strings.Contains(got, "## Unrelated later section") || !strings.Contains(got, "keep me") {
			t.Fatalf("content outside the markers was disturbed:\n%s", got)
		}
		if !strings.HasPrefix(got, "# My Project") {
			t.Fatalf("content before the markers was disturbed:\n%s", got)
		}
	})

	t.Run("rerun is idempotent", func(t *testing.T) {
		target := t.TempDir()
		if _, err := WriteClaudeSection(target, false); err != nil {
			t.Fatal(err)
		}
		first := readClaudeMD(t, target)
		if _, err := WriteClaudeSection(target, false); err != nil {
			t.Fatal(err)
		}
		second := readClaudeMD(t, target)
		if first != second {
			t.Fatalf("rerunning should be a no-op:\nfirst:\n%s\nsecond:\n%s", first, second)
		}
	})

	t.Run("preview never writes", func(t *testing.T) {
		target := t.TempDir()
		if _, err := WriteClaudeSection(target, true); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(target, claudeFile)); !os.IsNotExist(err) {
			t.Fatal("preview should not have created CLAUDE.md")
		}
	})
}

func readClaudeMD(t *testing.T, target string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(target, claudeFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
