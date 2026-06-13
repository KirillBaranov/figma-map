// Package storybook builds a catalog of code components by reading a running
// Storybook's index.json, screenshotting each story, and parsing the real
// import statement from the story source file.
package storybook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Story is one catalog entry: a single Storybook story (component + variant)
// plus the data needed to import the component and a screenshot of it.
type Story struct {
	ID           string `json:"id"`           // e.g. "ui-button--primary"
	Component    string `json:"component"`    // e.g. "Button" (from title)
	Variant      string `json:"variant"`      // e.g. "Primary" (from name)
	ImportSymbol string `json:"importSymbol"` // e.g. "Button"
	ImportFrom   string `json:"importFrom"`   // e.g. "@/components/ui/button"
	PNGPath      string `json:"pngPath"`      // relative path to screenshot in catalog dir
}

// Catalog is the persisted result of a scan.
type Catalog struct {
	Storybook string  `json:"storybook"`
	Stories   []Story `json:"stories"`
}

// indexJSON mirrors the parts of Storybook's index.json (v5) we consume.
type indexJSON struct {
	V       int `json:"v"`
	Entries map[string]struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Name       string `json:"name"`
		ImportPath string `json:"importPath"`
		Type       string `json:"type"`
	} `json:"entries"`
}

// titlePrefix selects which stories enter the catalog. Stories whose title does
// not start with this prefix (e.g. Storybook's own "Configure your project")
// are skipped.
const titlePrefix = "UI/"

// ParseIndex turns a raw index.json payload into Stories, filtered to titles
// under titlePrefix. Import fields are left empty; ResolveImports fills them by
// reading the story source files relative to projectRoot.
func ParseIndex(raw []byte) ([]Story, error) {
	var idx indexJSON
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parse index.json: %w", err)
	}

	var stories []Story
	for _, e := range idx.Entries {
		if e.Type != "story" || !strings.HasPrefix(e.Title, titlePrefix) {
			continue
		}
		stories = append(stories, Story{
			ID:         e.ID,
			Component:  strings.TrimPrefix(e.Title, titlePrefix),
			Variant:    e.Name,
			ImportFrom: e.ImportPath, // provisional; resolved later
		})
	}
	return stories, nil
}

// importRe matches a named import of the component symbol in a story file, e.g.
//
//	import { Button } from '../components/ui/button';
//	import { Card, CardHeader } from "@/components/ui/card"
var importRe = regexp.MustCompile(`import\s*\{([^}]*)\}\s*from\s*['"]([^'"]+)['"]`)

// ResolveImports reads each story's source file (importPath is relative to
// projectRoot) and fills ImportSymbol/ImportFrom with the real component import.
// The symbol is matched against the component name; the first import whose
// braces contain that name wins.
func ResolveImports(stories []Story, projectRoot string) error {
	// Cache file contents so multiple variants of one component read once.
	cache := map[string]string{}

	for i := range stories {
		s := &stories[i]
		rel := s.ImportFrom // currently the story importPath
		if rel == "" {
			continue
		}
		abs := filepath.Join(projectRoot, rel)

		src, ok := cache[abs]
		if !ok {
			data, err := os.ReadFile(abs)
			if err != nil {
				return fmt.Errorf("read story source %s: %w", rel, err)
			}
			src = string(data)
			cache[abs] = src
		}

		sym, from := findImport(src, s.Component)
		if from == "" {
			return fmt.Errorf("could not resolve import for %s in %s", s.Component, rel)
		}
		s.ImportSymbol = sym
		s.ImportFrom = from
	}
	return nil
}

// findImport returns the import symbol and module path for a component by
// scanning import statements for one whose braces include component.
func findImport(src, component string) (symbol, from string) {
	for _, m := range importRe.FindAllStringSubmatch(src, -1) {
		names := m[1]
		path := m[2]
		for _, n := range strings.Split(names, ",") {
			if strings.TrimSpace(n) == component {
				return component, path
			}
		}
	}
	return "", ""
}

// Save writes the catalog as catalog.json in dir.
func (c Catalog) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "catalog.json"), data, 0o644)
}

// LoadCatalog reads catalog.json from dir.
func LoadCatalog(dir string) (Catalog, error) {
	var c Catalog
	data, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("parse catalog.json: %w", err)
	}
	return c, nil
}

// PNG loads a story's screenshot bytes from the catalog dir.
func (s Story) PNG(catalogDir string) ([]byte, error) {
	return os.ReadFile(filepath.Join(catalogDir, s.PNGPath))
}
