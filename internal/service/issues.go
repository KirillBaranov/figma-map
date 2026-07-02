package service

import (
	"context"
	"fmt"

	"github.com/kirillbaranov/figma-map/internal/figma"
)

// IssuesResult is the `capture issues` output.
type IssuesResult struct {
	Issues []figma.Issue `json:"issues"`
}

// AckResult is the `capture ack` output.
type AckResult struct {
	ID string `json:"id"`
}

// issueSource type-asserts the service's Source to the bridge-specific
// IssueSource capability. Issues are page captures relayed by the bridge
// process, not Figma data, so they live outside the Source interface proper
// — only a real bridge connection supports them.
func (s *Service) issueSource() (figma.IssueSource, error) {
	is, ok := s.src.(figma.IssueSource)
	if !ok {
		return nil, fmt.Errorf("issue inbox requires a bridge connection")
	}
	return is, nil
}

// ListIssues returns pending issues flagged by the browser extension,
// optionally filtered to a single file.
func (s *Service) ListIssues(ctx context.Context, fileKey string) (IssuesResult, error) {
	is, err := s.issueSource()
	if err != nil {
		return IssuesResult{}, err
	}
	issues, err := is.ListIssues(ctx, fileKey)
	if err != nil {
		return IssuesResult{}, err
	}
	return IssuesResult{Issues: issues}, nil
}

// AckIssue marks a flagged issue as handled.
func (s *Service) AckIssue(ctx context.Context, id string) (AckResult, error) {
	is, err := s.issueSource()
	if err != nil {
		return AckResult{}, err
	}
	if err := is.AckIssue(ctx, id); err != nil {
		return AckResult{}, err
	}
	return AckResult{ID: id}, nil
}
