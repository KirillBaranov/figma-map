package service

import (
	"context"
	"fmt"

	"github.com/kirillbaranov/figma-map/internal/storybook"
)

// ScanResult summarizes a scan.
type ScanResult struct {
	Storybook string `json:"storybook"`
	Stories   int    `json:"stories"`
	Out       string `json:"out"`
}

// Scan reads a running Storybook's index.json, screenshots each UI story,
// resolves each component's real import, and writes a catalog. Deterministic.
func (s *Service) Scan(ctx context.Context, storybookURL, project, out string) (ScanResult, error) {
	p := progressFrom(ctx)
	if storybookURL == "" {
		storybookURL = s.cfg.Storybook
	}

	p.emit(fmt.Sprintf("Fetching index.json from %s …", storybookURL))
	raw, err := storybook.FetchIndex(storybookURL)
	if err != nil {
		return ScanResult{}, err
	}

	stories, err := storybook.ParseIndex(raw)
	if err != nil {
		return ScanResult{}, err
	}
	if len(stories) == 0 {
		return ScanResult{}, fmt.Errorf("no UI/* stories found in index.json")
	}
	p.emit(fmt.Sprintf("Found %d stories. Resolving imports …", len(stories)))

	if err := storybook.ResolveImports(stories, project); err != nil {
		return ScanResult{}, err
	}

	p.emit("Capturing screenshots …")
	if err := storybook.NewCapturer(storybookURL).CaptureAll(ctx, stories, out); err != nil {
		return ScanResult{}, err
	}

	catalog := storybook.Catalog{Storybook: storybookURL, Stories: stories}
	if err := catalog.Save(out); err != nil {
		return ScanResult{}, err
	}
	return ScanResult{Storybook: storybookURL, Stories: len(stories), Out: out}, nil
}
