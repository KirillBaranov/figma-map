// Package service holds all of figma-map's logic behind one type. The CLI and
// the MCP server are thin wrappers over it (see internal/op), so behavior is
// identical regardless of how an operation is invoked.
//
// Per ADR-0001 the service is deterministic-first: most methods need no API key.
// The LLM client is built lazily and only the fuzzy operations require it.
package service

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
)

// Service is the entry point for every operation.
type Service struct {
	cfg    config.Config
	bridge *figma.Bridge
	llm    *llm.Client // built lazily by llmClient
}

// New constructs a Service. It never requires an API key — deterministic
// operations run without one.
func New(cfg config.Config) *Service {
	return &Service{cfg: cfg, bridge: figma.NewBridge(cfg.Bridge)}
}

// Config exposes the loaded configuration (read-only use by callers).
func (s *Service) Config() config.Config { return s.cfg }

// Progress reports human-facing progress for long operations. The CLI wires it
// to stderr; the MCP server omits it (no-op).
type Progress func(string)

type progressKey struct{}

// WithProgress attaches a progress reporter to ctx. Operations that report
// progress read it via progressFrom; absent reporter = silent.
func WithProgress(ctx context.Context, p Progress) context.Context {
	return context.WithValue(ctx, progressKey{}, p)
}

func progressFrom(ctx context.Context) Progress {
	if p, ok := ctx.Value(progressKey{}).(Progress); ok {
		return p
	}
	return nil
}

func (p Progress) emit(msg string) {
	if p != nil {
		p(msg)
	}
}

// llmClient lazily builds the vision client, erroring if no API key is set.
func (s *Service) llmClient() (*llm.Client, error) {
	if s.llm != nil {
		return s.llm, nil
	}
	key, ok := s.cfg.APIKey()
	if !ok {
		return nil, fmt.Errorf("API key not set; export $%s", s.cfg.LLM.APIKeyEnv)
	}
	s.llm = llm.New(llm.Options{
		APIKey:  key,
		BaseURL: s.cfg.LLM.BaseURL,
		Model:   s.cfg.LLM.Model,
	})
	return s.llm, nil
}

// resolveFileKey returns the file to operate on: explicit flag, then config,
// else the single connected file (erroring if ambiguous).
func (s *Service) resolveFileKey(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if s.cfg.FileKey != "" {
		return s.cfg.FileKey, nil
	}
	files, err := s.bridge.Files()
	if err != nil {
		return "", err
	}
	switch len(files) {
	case 0:
		return "", fmt.Errorf("no Figma files connected to the bridge (open the file and run the plugin)")
	case 1:
		return files[0].FileKey, nil
	default:
		return "", fmt.Errorf("multiple files connected; pass --file <fileKey>")
	}
}

// jsonObjRe extracts the first {...} block from an LLM reply.
var jsonObjRe = regexp.MustCompile(`(?s)\{.*\}`)

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
