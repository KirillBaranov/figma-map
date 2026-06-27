package service

import (
	"context"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// PagesResult is the `pages` output — the discovery entry point: an agent
// with no node id yet calls this first to get oriented, then `find`/`inspect`
// to drill into a specific page/frame. Intentionally tiny: no styles, no tree.
type PagesResult struct {
	FileName        string       `json:"fileName"`
	CurrentPageID   string       `json:"currentPageId"`
	CurrentPageName string       `json:"currentPageName"`
	Pages           []figma.Page `json:"pages"`
}

// Pages returns the file's page list, with no node tree or styles.
func (s *Service) Pages(ctx context.Context, fileKey string) (PagesResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return PagesResult{}, err
	}
	meta, err := s.src.Metadata(ctx, key)
	if err != nil {
		return PagesResult{}, err
	}
	return PagesResult{
		FileName:        meta.FileName,
		CurrentPageID:   meta.CurrentPageID,
		CurrentPageName: meta.CurrentPageName,
		Pages:           meta.Pages,
	}, nil
}

// VariablesResult is the `variables` output — the file's full local-variable
// catalog (every collection/variable/mode). This is "what tokens exist",
// independent of any specific node's bindings; see `tokens` for what a given
// node is actually bound to.
type VariablesResult struct {
	Collections []figma.VariableCollection `json:"collections"`
}

// Variables returns every local variable collection defined in the file.
func (s *Service) Variables(ctx context.Context, fileKey string) (VariablesResult, error) {
	key, err := s.resolveFileKey(ctx, fileKey)
	if err != nil {
		return VariablesResult{}, err
	}
	defs, err := s.src.VariableDefs(ctx, key)
	if err != nil {
		return VariablesResult{}, err
	}
	return VariablesResult{Collections: defs.Collections}, nil
}
