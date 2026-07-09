// Package scaffold bootstraps figma-map into a target project: the skill,
// a starter config, MCP server registration, and a CLAUDE.md section. It
// backs the `figma-map init` command.
package scaffold

import (
	"os"
	"path/filepath"
	"sort"
)

// projectMarkers are files whose presence in a directory marks it as a
// project root, for the purpose of suggesting init targets.
var projectMarkers = []string{".git", "go.mod", "package.json", "Cargo.toml", "pyproject.toml"}

// IsProjectRoot reports whether dir directly contains any projectMarkers.
func IsProjectRoot(dir string) bool {
	for _, m := range projectMarkers {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return true
		}
	}
	return false
}

// CandidateRoots returns the directories to search for project candidates:
// the current directory, its siblings, and well-known project roots —
// filtered to those that actually exist.
func CandidateRoots() []string {
	var roots []string

	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
		if parent := filepath.Dir(cwd); parent != cwd {
			roots = append(roots, parent)
		}
	}

	home, err := os.UserHomeDir()
	if err == nil {
		for _, d := range []string{"Desktop", "Projects", "dev", "work", "code"} {
			roots = append(roots, filepath.Join(home, d))
		}
	}

	return existingDirs(roots)
}

// Discover walks each root up to maxDepth levels looking for directories
// matching IsProjectRoot, returning a deduplicated, sorted list.
func Discover(roots []string, maxDepth int) []string {
	seen := map[string]bool{}
	var found []string

	for _, root := range roots {
		walkForProjects(root, maxDepth, seen, &found)
	}

	sort.Strings(found)
	return found
}

func walkForProjects(dir string, depth int, seen map[string]bool, found *[]string) {
	if depth < 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if IsProjectRoot(dir) && !seen[dir] {
		seen[dir] = true
		*found = append(*found, dir)
		// Don't descend into a project root looking for nested projects.
		return
	}
	for _, e := range entries {
		if !e.IsDir() || hasDotPrefix(e.Name()) {
			continue
		}
		walkForProjects(filepath.Join(dir, e.Name()), depth-1, seen, found)
	}
}

func hasDotPrefix(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

func existingDirs(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		if seen[p] {
			continue
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
